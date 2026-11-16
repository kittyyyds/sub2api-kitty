package service

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"math"
	"net/mail"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	upstreamRateNotifyDelay      = 5 * time.Minute
	upstreamRateNotifyRetryDelay = time.Minute
	upstreamRateNotifyLease      = 2 * time.Minute
)

// UpstreamRateNotification is stored inside the account's probe snapshot. A
// snapshot and its notification state therefore pass the same repository CAS.
// Baseline survives restarts and remains fixed until the upstream returns to it.
type UpstreamRateNotification struct {
	Epoch          string     `json:"epoch"`
	Recipient      string     `json:"recipient"`
	SourceHash     string     `json:"source_hash"`
	Baseline       float64    `json:"baseline"`
	Candidate      *float64   `json:"candidate,omitempty"`
	ChangedAt      *time.Time `json:"changed_at,omitempty"`
	LastObservedAt *time.Time `json:"last_observed_at,omitempty"`
	SentAt         *time.Time `json:"sent_at,omitempty"`
	RetryAt        *time.Time `json:"retry_at,omitempty"`
	LeaseToken     string     `json:"lease_token,omitempty"`
	LeaseUntil     *time.Time `json:"lease_until,omitempty"`
	Attempts       int        `json:"attempts,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
}

type upstreamRateEmailSender interface {
	SendEmail(context.Context, string, string, string) error
}

// rateNotifyAccountRepository decorates only the probe's repository. It keeps
// the probe's normal CAS write authoritative and leaves all other account
// services and the shared probe implementation unchanged.
type rateNotifyAccountRepository struct {
	AccountRepository
	newAPIRepo     NewAPIAuthorizationRepository
	settingService *SettingService
	sender         upstreamRateEmailSender
	now            func() time.Time
	async          bool
	slots          chan struct{}
}

func (r *rateNotifyAccountRepository) currentTime() time.Time {
	if r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}

func (r *rateNotifyAccountRepository) getSettings(ctx context.Context) (*UpstreamBillingProbeSettings, error) {
	if r.settingService == nil {
		return defaultUpstreamBillingProbeSettings(), nil
	}
	return r.settingService.GetUpstreamBillingProbeSettings(ctx)
}

func (r *rateNotifyAccountRepository) UpdateUpstreamBillingProbeSnapshot(ctx context.Context, account *Account, snapshot *UpstreamBillingProbeSnapshot, rate *float64) error {
	r.prepareRateNotification(ctx, account, snapshot, nil)
	writer, ok := r.AccountRepository.(upstreamBillingProbeSnapshotWriter)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	if err := writer.UpdateUpstreamBillingProbeSnapshot(ctx, account, snapshot, rate); err != nil {
		return err
	}
	r.afterSnapshot(ctx, account.ID, snapshot)
	return nil
}

func (r *rateNotifyAccountRepository) ListDueUpstreamBillingProbeAccounts(ctx context.Context, now time.Time, limit int) ([]Account, error) {
	if lister, ok := r.AccountRepository.(upstreamBillingProbeDueAccountLister); ok {
		return lister.ListDueUpstreamBillingProbeAccounts(ctx, now, limit)
	}
	return r.FindByExtraField(ctx, UpstreamBillingProbeEnabledExtraKey, true)
}

// rateNotifyNewAPIRepository preserves the dedicated binding/revision CAS while
// sharing the same notification state machine as the generic billing probe.
type rateNotifyNewAPIRepository struct {
	NewAPIAuthorizationRepository
	notifier *rateNotifyAccountRepository
}

func (r *rateNotifyNewAPIRepository) WriteSnapshot(ctx context.Context, account *Account, binding *NewAPIAccountBinding, snapshot *UpstreamBillingProbeSnapshot) error {
	r.notifier.prepareRateNotification(ctx, account, snapshot, binding)
	if err := r.NewAPIAuthorizationRepository.WriteSnapshot(ctx, account, binding, snapshot); err != nil {
		return err
	}
	r.notifier.afterSnapshot(ctx, account.ID, snapshot)
	return nil
}

// SetRateNotificationSender is called by the production provider after both
// repository paths are configured. Constructors retain their old behavior.
func (s *UpstreamBillingProbeService) SetRateNotificationSender(sender upstreamRateEmailSender) {
	wrapped := &rateNotifyAccountRepository{AccountRepository: s.accountRepo, newAPIRepo: s.newAPIRepo, settingService: s.settingService, sender: sender, now: s.currentTime, async: true, slots: make(chan struct{}, upstreamBillingProbeConcurrency)}
	s.accountRepo = wrapped
	if s.newAPIRepo != nil {
		s.newAPIRepo = &rateNotifyNewAPIRepository{NewAPIAuthorizationRepository: s.newAPIRepo, notifier: wrapped}
	}
}

func (r *rateNotifyAccountRepository) afterSnapshot(ctx context.Context, accountID int64, snapshot *UpstreamBillingProbeSnapshot) {
	if r.sender == nil || !upstreamRateNotificationReady(snapshot.RateNotification, r.currentTime()) {
		return
	}
	if r.async {
		// Mail delivery must not create unbounded work or hold up probing. A
		// busy sender leaves the persisted candidate for a later normal probe.
		select {
		case r.slots <- struct{}{}:
		default:
			return
		}
		go func() {
			defer func() { <-r.slots }()
			notifyCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
			defer cancel()
			r.deliverRateNotification(notifyCtx, accountID)
		}()
	} else {
		r.deliverRateNotification(ctx, accountID)
	}
}

// writeSnapshot never bypasses the current NewAPI binding or its revision CAS.
func (r *rateNotifyAccountRepository) writeSnapshot(ctx context.Context, account *Account, snapshot *UpstreamBillingProbeSnapshot) error {
	if snapshot.RateSource == "bound_group_pricing" {
		if r.newAPIRepo == nil {
			return ErrUpstreamBillingProbeIdentityChanged
		}
		binding, err := r.newAPIRepo.GetBinding(ctx, account.ID)
		if err != nil || binding == nil || binding.Fingerprint != NewAPIAccountFingerprint(account) {
			return ErrUpstreamBillingProbeIdentityChanged
		}
		if snapshot.RateNotification == nil || snapshot.RateNotification.SourceHash != upstreamRateNotifySourceHash(account, binding) {
			return ErrUpstreamBillingProbeIdentityChanged
		}
		return r.newAPIRepo.WriteSnapshot(ctx, account, binding, snapshot)
	}
	if snapshot.RateNotification == nil || snapshot.RateNotification.SourceHash != upstreamRateNotifySourceHash(account, nil) {
		return ErrUpstreamBillingProbeIdentityChanged
	}
	writer, ok := r.AccountRepository.(upstreamBillingProbeSnapshotWriter)
	if !ok {
		return ErrUpstreamBillingProbeUnavailable
	}
	return writer.UpdateUpstreamBillingProbeSnapshot(ctx, account, snapshot, nil)
}

func validateUpstreamRateNotifyEmail(email string) error {
	if email == "" {
		return errors.New("notification email is required when rate change notification is enabled")
	}
	if len(email) > 254 || strings.ContainsAny(email, "\r\n") {
		return errors.New("invalid notification email")
	}
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || address.Name != "" {
		return errors.New("invalid notification email")
	}
	return nil
}

// advanceUpstreamRateNotification observes a fresh declared multiplier. A
// second change restarts the five-minute window; returning to baseline clears
// the candidate. A sent candidate is never mailed twice during one excursion.
func advanceUpstreamRateNotification(previous *UpstreamRateNotification, epoch, recipient, sourceHash string, observed float64, at time.Time) *UpstreamRateNotification {
	if previous == nil || previous.Epoch != epoch || previous.Recipient != recipient || previous.SourceHash != sourceHash {
		previous = &UpstreamRateNotification{Epoch: epoch, Recipient: recipient, SourceHash: sourceHash, Baseline: observed}
	}
	state := *previous
	state.LastObservedAt = upstreamRateNotifyTimePtr(at)
	if equalBillingMultiplier(observed, state.Baseline) {
		state.Candidate = nil
		state.ChangedAt = nil
		state.SentAt = nil
		state.RetryAt = nil
		state.LeaseToken = ""
		state.LeaseUntil = nil
		state.Attempts = 0
		state.LastError = ""
		return &state
	}
	if state.Candidate == nil || !equalBillingMultiplier(*state.Candidate, observed) {
		state.Candidate = upstreamRateNotifyFloatPtr(observed)
		state.ChangedAt = upstreamRateNotifyTimePtr(at)
		state.SentAt = nil
		state.RetryAt = nil
		state.LeaseToken = ""
		state.LeaseUntil = nil
		state.Attempts = 0
		state.LastError = ""
	}
	return &state
}

func upstreamRateNotifySourceHash(account *Account, binding *NewAPIAccountBinding) string {
	// Hash the existing transport identity. No raw key, proxy credentials or
	// management secret is persisted in notification state or email.
	identity := NewAPIAccountFingerprint(account)
	if binding != nil {
		return fmt.Sprintf("%s:%d:%d:%d", identity, binding.Profile.ID, binding.Profile.Revision, binding.TokenID)
	}
	return identity
}

func upstreamRateNotifyTimePtr(value time.Time) *time.Time { return &value }
func upstreamRateNotifyFloatPtr(value float64) *float64    { return &value }

func upstreamRateNotificationReady(state *UpstreamRateNotification, now time.Time) bool {
	return state != nil && state.Candidate != nil && state.ChangedAt != nil &&
		state.LastObservedAt != nil && state.SentAt == nil &&
		!now.Before(state.ChangedAt.Add(upstreamRateNotifyDelay)) &&
		!state.LastObservedAt.Before(state.ChangedAt.Add(upstreamRateNotifyDelay)) &&
		(state.RetryAt == nil || !now.Before(*state.RetryAt)) &&
		(state.LeaseUntil == nil || !now.Before(*state.LeaseUntil))
}

func upstreamRateNotifyObserved(snapshot *UpstreamBillingProbeSnapshot) (float64, bool) {
	if snapshot == nil || snapshot.Kind != "" || snapshot.Status != UpstreamBillingProbeStatusOK || snapshot.Data == nil || snapshot.ReceivedAt == nil || !snapshot.ReceivedAt.Equal(snapshot.LastAttemptAt) {
		return 0, false
	}
	// Multiple routes have no single declared rate. Scheduled peak factors are
	// intentionally ignored; notify only on the declared base multiplier.
	if snapshot.Data["billing_mode"] == "multi_group" {
		return 0, false
	}
	value, ok := resolveAccountExtraNumber(snapshot.Data, "resolved_rate_multiplier")
	return value, ok && value >= 0 && !math.IsNaN(value) && !math.IsInf(value, 0)
}

// prepareRateNotification runs before the snapshot CAS. Notification errors
// never prevent the upstream multiplier from being written back.
func (r *rateNotifyAccountRepository) prepareRateNotification(ctx context.Context, account *Account, snapshot *UpstreamBillingProbeSnapshot, binding *NewAPIAccountBinding) {
	if account == nil || snapshot == nil {
		return
	}
	if snapshot.Kind != "" {
		// Media catalog observations are not scalar billing declarations.
		snapshot.RateNotification = nil
		return
	}
	previous := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if previous != nil {
		snapshot.RateNotification = previous.RateNotification
	}
	settings, err := r.getSettings(ctx)
	if err != nil {
		slog.Error("upstream_rate_notification_settings_failed", "account_id", account.ID, "error", err)
		return
	}
	if !settings.RateChangeNotifyEnabled || settings.RateChangeNotifyEmail == "" || !upstreamBillingProbeEnabled(account) {
		snapshot.RateNotification = nil
		return
	}
	observed, ok := upstreamRateNotifyObserved(snapshot)
	if !ok {
		return
	}
	state := advanceUpstreamRateNotification(snapshot.RateNotification, settings.RateChangeNotifyEpoch,
		settings.RateChangeNotifyEmail, upstreamRateNotifySourceHash(account, binding), observed, snapshot.LastAttemptAt)
	snapshot.RateNotification = state
	// Keep the user configured probe cadence; confirmation happens on a later
	// normal probe at least five minutes after the first changed observation.
}

// deliverRateNotification uses a persisted lease to suppress ordinary repeats
// across instances. A crash between SMTP acceptance and recording success can
// still yield an at-least-once retry, as SMTP has no transaction with the DB.
func (r *rateNotifyAccountRepository) deliverRateNotification(ctx context.Context, accountID int64) {
	if r.sender == nil || r.AccountRepository == nil {
		return
	}
	account, err := r.GetByID(ctx, accountID)
	if err != nil || !upstreamBillingProbeEnabled(account) {
		return
	}
	settings, err := r.getSettings(ctx)
	if err != nil || !settings.RateChangeNotifyEnabled {
		return
	}
	snapshot := decodeUpstreamBillingProbeSnapshot(account.Extra)
	if snapshot == nil || snapshot.RateNotification == nil ||
		snapshot.RateNotification.Epoch != settings.RateChangeNotifyEpoch ||
		snapshot.RateNotification.Recipient != settings.RateChangeNotifyEmail {
		return
	}
	now := r.currentTime()
	state := snapshot.RateNotification
	if !upstreamRateNotificationReady(state, now) {
		return
	}
	// Only a confirmed fresh probe may trigger delivery. Failed probes preserve
	// the pending state but cannot assert that the changed rate still holds.
	observed, ok := upstreamRateNotifyObserved(snapshot)
	if !ok || !equalBillingMultiplier(observed, *state.Candidate) ||
		snapshot.LastAttemptAt.Before(state.ChangedAt.Add(upstreamRateNotifyDelay)) {
		return
	}
	token := uuid.NewString()
	state.LeaseToken = token
	state.LeaseUntil = upstreamRateNotifyTimePtr(now.Add(upstreamRateNotifyLease))
	state.Attempts++
	if err := r.writeSnapshot(ctx, account, snapshot); err != nil {
		slog.Warn("upstream_rate_notification_claim_failed", "account_id", accountID, "error", err)
		return
	}
	freshSettings, err := r.getSettings(ctx)
	if err != nil {
		slog.Error("upstream_rate_notification_settings_recheck_failed", "account_id", accountID, "error", err)
		return
	}
	if !freshSettings.RateChangeNotifyEnabled || freshSettings.RateChangeNotifyEpoch != state.Epoch || freshSettings.RateChangeNotifyEmail != state.Recipient {
		return
	}
	// An administrator may have disabled or rebound the account after the
	// claim. Recheck the current lease/identity before the external side effect.
	freshAccount, err := r.GetByID(ctx, accountID)
	if err != nil || !upstreamBillingProbeEnabled(freshAccount) {
		return
	}
	freshSnapshot := decodeUpstreamBillingProbeSnapshot(freshAccount.Extra)
	if freshSnapshot == nil || freshSnapshot.RateNotification == nil || freshSnapshot.RateNotification.LeaseToken != token {
		return
	}
	if snapshot.RateSource == "bound_group_pricing" {
		if r.newAPIRepo == nil {
			return
		}
		binding, err := r.newAPIRepo.GetBinding(ctx, accountID)
		if err != nil || binding == nil || binding.Fingerprint != NewAPIAccountFingerprint(freshAccount) || state.SourceHash != upstreamRateNotifySourceHash(freshAccount, binding) {
			return
		}
	} else if state.SourceHash != upstreamRateNotifySourceHash(freshAccount, nil) {
		return
	}
	subject, body := buildUpstreamRateNotificationEmail(account, state, now)
	sendCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	sendErr := r.sender.SendEmail(sendCtx, state.Recipient, subject, body)
	cancel()
	if sendErr != nil {
		slog.Error("upstream_rate_notification_send_failed", "account_id", accountID,
			"attempt", state.Attempts)
	}
	latest, err := r.GetByID(ctx, accountID)
	if err != nil {
		slog.Error("upstream_rate_notification_state_reload_failed", "account_id", accountID, "error", err)
		return
	}
	current := decodeUpstreamBillingProbeSnapshot(latest.Extra)
	if current == nil || current.RateNotification == nil || current.RateNotification.LeaseToken != token {
		return
	}
	currentState := current.RateNotification
	currentState.LeaseToken = ""
	currentState.LeaseUntil = nil
	if sendErr == nil {
		currentState.SentAt = upstreamRateNotifyTimePtr(r.currentTime())
		currentState.RetryAt = nil
		currentState.LastError = ""
		slog.Info("upstream_rate_notification_sent", "account_id", accountID, "recipient", currentState.Recipient,
			"baseline", currentState.Baseline, "current", *currentState.Candidate)
	} else {
		currentState.RetryAt = upstreamRateNotifyTimePtr(r.currentTime().Add(upstreamRateNotifyRetryDelay))
		currentState.LastError = "email_send_failed"
	}
	if err := r.writeSnapshot(ctx, latest, current); err != nil {
		slog.Error("upstream_rate_notification_state_save_failed", "account_id", accountID, "error", err)
	}
}

func buildUpstreamRateNotificationEmail(account *Account, state *UpstreamRateNotification, detectedAt time.Time) (string, string) {
	groupLines := make([]string, 0, len(account.Groups))
	for _, group := range account.Groups {
		if group != nil {
			groupLines = append(groupLines, fmt.Sprintf("%s (当前倍率 %s)", group.Name, formatUpstreamRate(group.RateMultiplier)))
		}
	}
	sort.Strings(groupLines)
	if len(groupLines) == 0 {
		groupLines = append(groupLines, "未关联分组")
	}
	baseURL := strings.TrimSpace(account.GetCredential("base_url"))
	if parsed, err := url.Parse(baseURL); err == nil && parsed.Host != "" {
		parsed.User, parsed.RawQuery, parsed.Fragment = nil, "", ""
		baseURL = parsed.String()
	} else {
		baseURL = "未提供可公开的站点地址"
	}
	source := "API Key 导入：" + baseURL
	if binding, ok := account.Extra["provider_binding"].(map[string]any); ok {
		if name, ok := binding["provider_name"].(string); ok && strings.TrimSpace(name) != "" {
			source = "服务商导入：" + strings.TrimSpace(name) + " (baseURL: " + baseURL + ")"
		}
	}
	safeSubjectName := strings.NewReplacer("\r", " ", "\n", " ").Replace(account.Name)
	subject := fmt.Sprintf("上游倍率持续变化：%s %s → %s", safeSubjectName, formatUpstreamRate(state.Baseline), formatUpstreamRate(*state.Candidate))
	lines := []string{
		"对应分组：" + strings.Join(groupLines, "、"),
		fmt.Sprintf("对应账号：%s (ID %d)", account.Name, account.ID),
		fmt.Sprintf("上游声明倍率：%s → %s", formatUpstreamRate(state.Baseline), formatUpstreamRate(*state.Candidate)),
		"站点来源：" + source,
		"本提醒不会启用或改变自动跟价、账号倍率及分组价格。",
		"检测时间：" + detectedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		"开始变化：" + state.ChangedAt.UTC().Format("2006-01-02 15:04:05 UTC"),
		"持续时长：" + detectedAt.Sub(*state.ChangedAt).Round(time.Second).String(),
	}
	var body strings.Builder
	_, _ = body.WriteString("<p>上游声明倍率持续变化，请核对以下账号：</p><ul>")
	for _, line := range lines {
		_, _ = body.WriteString("<li>")
		_, _ = body.WriteString(html.EscapeString(line))
		_, _ = body.WriteString("</li>")
	}
	_, _ = body.WriteString("</ul>")
	return subject, body.String()
}

func formatUpstreamRate(rate float64) string { return strconv.FormatFloat(rate, 'f', -1, 64) }
