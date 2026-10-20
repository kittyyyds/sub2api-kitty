package service

import (
	"context"
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/reauthruntime"
	"log/slog"
	"net/mail"
	"strings"
	"time"
)

// Login jobs keep token results in bounded, short-lived memory. Successful logins
// save the supplied password/2FA to the guard without enabling it or creating accounts.
type OpenAITwoFALoginJob struct {
	ID               string         `json:"id"`
	Status           string         `json:"status"`
	Credential       map[string]any `json:"credential,omitempty"`
	Stage            string         `json:"stage,omitempty"`
	ErrorCode        string         `json:"error_code,omitempty"`
	Message          string         `json:"message,omitempty"`
	HTTPStatus       int            `json:"http_status,omitempty"`
	Timezone         string         `json:"timezone,omitempty"`
	BrowserAvailable bool           `json:"browser_available"`
	UpdatedAt        time.Time      `json:"updated_at"`
	browserID        string
	cancel           context.CancelFunc
	expiry           *time.Timer
}

func ValidateOpenAITwoFALogin(entry AccountTokenGuardReloginAccount) error {
	email := strings.TrimSpace(entry.Email)
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 320 ||
		strings.TrimSpace(entry.Password) == "" || len(entry.Password) > 4096 ||
		strings.TrimSpace(entry.MFASecret) == "" || len(entry.MFASecret) > 4096 {
		return errors.New("请填写有效的邮箱、密码和 2FA 密钥")
	}
	return nil
}

func (s *AccountTokenGuardService) StartTwoFALogin(ctx context.Context, entry AccountTokenGuardReloginAccount) (*OpenAITwoFALoginJob, error) {
	return s.startTwoFALogin(ctx, entry, true)
}

// Operations imports save their login method through the encrypted per-account
// API after identity deduplication returns the account ID. Do not also enroll
// them in the legacy guard's plaintext settings.
func (s *AccountTokenGuardService) StartTwoFALoginForOperations(ctx context.Context, entry AccountTokenGuardReloginAccount) (*OpenAITwoFALoginJob, error) {
	return s.startTwoFALogin(ctx, entry, false)
}

func (s *AccountTokenGuardService) startTwoFALogin(ctx context.Context, entry AccountTokenGuardReloginAccount, saveToLegacyGuard bool, login ...func(context.Context) (map[string]any, error)) (*OpenAITwoFALoginJob, error) {
	if err := ValidateOpenAITwoFALogin(entry); err != nil {
		return nil, err
	}
	var execute func(context.Context) (map[string]any, error)
	if len(login) > 0 && login[0] != nil {
		execute = login[0]
	} else {
		cfg, err := s.GetConfig(ctx)
		if err != nil {
			return nil, errors.New("无法读取凭证守护配置")
		}
		if err := validateGuardHTTPURL(cfg.ReloginEndpoint, "relogin_endpoint"); err != nil {
			return nil, errors.New("请在凭证守护中配置有效的重登接口")
		}
		execute = func(c context.Context) (map[string]any, error) { return s.relogin(c, cfg, entry) }
	}
	entry.Email = strings.ToLower(strings.TrimSpace(entry.Email))
	entry.MFASecret = strings.TrimSpace(entry.MFASecret)
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	active := 0
	for _, existing := range s.logins {
		if existing.Status == "running" || existing.Status == "waiting_interaction" {
			active++
		}
	}
	if active >= 32 {
		return nil, errors.New("登录任务过多，请稍后重试")
	}
	// Terminal records must not consume live capacity during a large batch.
	// Evict only the oldest failed diagnostic; never evict uncollected credentials.
	for len(s.logins) >= 256 {
		var oldest *OpenAITwoFALoginJob
		for _, existing := range s.logins {
			if existing.Status == "failed" && (oldest == nil || existing.UpdatedAt.Before(oldest.UpdatedAt)) {
				oldest = existing
			}
		}
		if oldest == nil {
			return nil, errors.New("请先领取或清理已完成的登录任务")
		}
		oldest.cancel()
		oldest.expiry.Stop()
		delete(s.logins, oldest.ID)
	}
	if s.logins == nil {
		s.logins = make(map[string]*OpenAITwoFALoginJob)
	}
	loginCtx, cancel := context.WithTimeout(context.Background(), 25*time.Minute)
	job := &OpenAITwoFALoginJob{ID: guardRandomID(), Status: "running", Stage: "queued", UpdatedAt: time.Now(), cancel: cancel}
	s.logins[job.ID] = job
	job.expiry = time.AfterFunc(30*time.Minute, func() { s.DeleteTwoFALogin(job.ID) })
	go func() {
		defer cancel()
		progressCtx := reauthruntime.WithLoginProgress(loginCtx, func(p reauthruntime.LoginProgress) {
			p = reauthruntime.SafeLoginProgress(p)
			s.loginMu.Lock()
			defer s.loginMu.Unlock()
			if s.logins[job.ID] != job {
				return
			}
			job.Status, job.Stage, job.ErrorCode = p.Status, p.Stage, p.Code
			// The service owns final success only after validating/saving credentials.
			if job.Status == "succeeded" {
				job.Status = "running"
			}
			job.HTTPStatus, job.Timezone = p.HTTPStatus, p.Timezone
			job.browserID = p.BrowserID
			job.BrowserAvailable = p.BrowserID != "" && (p.Status == "waiting_interaction" || p.Status == "running")
			job.Message = ""
			if p.Code != "" {
				job.Message = reauthruntime.LoginMessage(p.Code)
			}
			job.UpdatedAt = time.Now()
		})
		credential, loginErr := execute(progressCtx)
		s.loginMu.Lock()
		defer s.loginMu.Unlock()
		if s.logins[job.ID] != job {
			return
		}
		if loginErr != nil {
			// Only allowlisted errors cross the admin API boundary.
			p := reauthruntime.LoginFailure(loginErr)
			job.Status, job.ErrorCode = "failed", p.Code
			if p.Stage != "" {
				job.Stage = p.Stage
			}
			job.HTTPStatus = p.HTTPStatus
			job.Message = reauthruntime.LoginMessage(p.Code)
			job.BrowserAvailable = false
			job.browserID = ""
			job.UpdatedAt = time.Now()
			return
		}
		// Persist verified login credentials before the client imports tokens and
		// clears its password/MFA input. Never report success on a failed save.
		if saveToLegacyGuard {
			if err := s.saveTwoFALoginAccount(loginCtx, entry); err != nil {
				slog.Warn("account_token_guard_two_fa_login_failed",
					"job_id", job.ID,
					"stage", "save_guard_credentials",
					"reason", safeTwoFALoginError(err, entry),
				)
				job.Status = "failed"
				job.ErrorCode = "credential_save_failed"
				job.Message = reauthruntime.LoginMessage(job.ErrorCode)
				job.BrowserAvailable = false
				job.browserID = ""
				return
			}
		}
		job.Credential = twoFALoginCredential(credential, entry.Email)
		job.Status = "succeeded"
		job.Stage = "succeeded"
		job.ErrorCode = ""
		job.Message = ""
		job.BrowserAvailable = false
		job.browserID = ""
		job.UpdatedAt = time.Now()
	}()
	return &OpenAITwoFALoginJob{ID: job.ID, Status: job.Status}, nil
}

// safeTwoFALoginError keeps asynchronous login diagnostics useful without
// allowing an upstream response or an accidentally wrapped secret to become a
// log entry. The relogin path already reduces network and provider failures to
// short error codes; this is a final redaction, length and control-character guard.
func safeTwoFALoginError(err error, entry AccountTokenGuardReloginAccount) string {
	if err == nil {
		return "unknown"
	}
	reason := strings.TrimSpace(err.Error())
	if reason == "" {
		return "unknown"
	}
	for _, secret := range []string{entry.Email, entry.Password, entry.MFASecret} {
		if secret != "" {
			reason = strings.ReplaceAll(reason, secret, "[redacted]")
		}
	}
	reason = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' {
			return ' '
		}
		if r < 0x20 || r == 0x7f {
			return -1
		}
		return r
	}, reason)
	if len(reason) > 160 {
		return reason[:160] + "..."
	}
	return reason
}

// Merge into the latest persisted config, not the snapshot from login start:
// logins can take minutes, and other imports/config edits may finish meanwhile.
func (s *AccountTokenGuardService) saveTwoFALoginAccount(ctx context.Context, entry AccountTokenGuardReloginAccount) error {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	cfg, err := s.getConfigLocked(ctx)
	if err != nil {
		return err
	}
	// GetConfig also caches this slice; do not mutate the published snapshot.
	accounts := make([]AccountTokenGuardReloginAccount, 0, len(cfg.ReloginAccounts)+1)
	for _, existing := range cfg.ReloginAccounts {
		if existing.Email != entry.Email {
			accounts = append(accounts, existing)
		}
	}
	cfg.ReloginAccounts = append(accounts, entry)
	_, err = s.saveConfigLocked(ctx, cfg)
	return err
}

// Only token/session fields enter the existing Session importer. Discard any
// echoed password, MFA secret, or unexpected service configuration fields.
func twoFALoginCredential(in map[string]any, email string) map[string]any {
	out := map[string]any{"email": email}
	for _, key := range []string{"access_token", "refresh_token", "id_token", "expires_at", "expired", "account_id", "chatgpt_account_id", "chatgpt_user_id", "user_id", "client_id", "plan_type"} {
		switch value := in[key].(type) {
		case string:
			out[key] = value
		case float64:
			if key == "expires_at" || key == "expired" {
				out[key] = value
			}
		}
	}
	return out
}

func (s *AccountTokenGuardService) TwoFALogin(id string) (*OpenAITwoFALoginJob, bool) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	job, ok := s.logins[id]
	if !ok {
		return nil, false
	}
	snapshot := &OpenAITwoFALoginJob{ID: job.ID, Status: job.Status, Stage: job.Stage, ErrorCode: job.ErrorCode, Message: job.Message, HTTPStatus: job.HTTPStatus, Timezone: job.Timezone, BrowserAvailable: job.BrowserAvailable, UpdatedAt: job.UpdatedAt}
	if job.Credential != nil {
		snapshot.Credential = make(map[string]any, len(job.Credential))
		for key, value := range job.Credential {
			snapshot.Credential[key] = value
		}
	}
	return snapshot, true
}

func (s *AccountTokenGuardService) DeleteTwoFALogin(id string) {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	if job, ok := s.logins[id]; ok {
		job.cancel()
		job.expiry.Stop()
		job.Credential = nil
		delete(s.logins, id)
	}
}

func (s *AccountTokenGuardService) clearTwoFALogins() {
	s.loginMu.Lock()
	defer s.loginMu.Unlock()
	for id, job := range s.logins {
		job.cancel()
		job.expiry.Stop()
		job.Credential = nil
		delete(s.logins, id)
	}
}

// Browser access always resolves an authenticated application job, never a client-supplied worker session ID.
func (s *AccountTokenGuardService) TwoFABrowserAction(ctx context.Context, id, action string, input any) ([]byte, error) {
	s.loginMu.Lock()
	job, ok := s.logins[id]
	browser := ""
	if ok && job.BrowserAvailable && (job.Status == "running" || job.Status == "waiting_interaction") {
		browser = job.browserID
	}
	s.loginMu.Unlock()
	if browser == "" {
		return nil, errors.New("登录窗口不存在或已结束")
	}
	return reauthruntime.BrowserAction(ctx, browser, action, input)
}
