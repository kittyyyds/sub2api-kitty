package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/tlsfingerprint"
)

var ErrProviderRemote = infraerrors.BadRequest("PROVIDER_REMOTE_FAILED", "Supplier authorization or request failed; review the wallet connection")

type providerHubRemote struct {
	repo    ProviderHubRepository
	service *UpstreamBillingProbeService
	wallet  *ProviderHubWalletAuthorization
	account *Account
	token   string
	refresh string
}
type ProviderHubRemoteKey struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Group     string `json:"group"`
	Available bool   `json:"available"`
	MaskedKey string `json:"masked_key"`
	Secret    string `json:"-"`
}
type ProviderHubRemoteGroup struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type ProviderHubImport struct {
	KeyIDs   []string `json:"key_ids"`
	Platform string   `json:"platform"`
	GroupIDs []int64  `json:"group_ids"`
	Prefix   string   `json:"prefix"`
	Revision int64    `json:"revision"`
}

func hubRemoteText(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}
func hubRemoteNumber(v any) (float64, bool) {
	switch x := v.(type) {
	case json.Number:
		n, e := x.Float64()
		return n, e == nil && !math.IsNaN(n) && !math.IsInf(n, 0) && math.Abs(n) <= 1e15
	case float64:
		return x, !math.IsNaN(x) && !math.IsInf(x, 0) && math.Abs(x) <= 1e15
	default:
		return 0, false
	}
}
func hubRemoteRows(data any) []map[string]any {
	var rows []any
	switch x := data.(type) {
	case []any:
		rows = x
	case map[string]any:
		for _, key := range []string{"items", "tokens", "data", "keys"} {
			if v, ok := x[key].([]any); ok {
				rows = v
				break
			}
		}
	}
	out := []map[string]any{}
	for _, row := range rows {
		if v, ok := row.(map[string]any); ok {
			out = append(out, v)
		}
	}
	return out
}
func (s *ChannelService) openProviderHubRemote(ctx context.Context, id int64) (*providerHubRemote, error) {
	repo, e := s.providerHubRepo()
	if e != nil {
		return nil, e
	}
	w, e := repo.ProviderHubAuthorization(ctx, id)
	if e != nil {
		return nil, ErrProviderHubInput
	}
	p := s.providerProbe
	if p == nil || p.accountTestService == nil || p.accountTestService.httpUpstream == nil || p.newAPIEncryptor == nil || !p.newAPIFixedKey || w.Cipher == "" || w.AuthMode == "none" || w.SiteType == "manual" {
		return nil, ErrProviderRemote
	}
	secret, e := p.newAPIEncryptor.Decrypt(w.Cipher)
	if e != nil {
		return nil, ErrProviderRemote
	}
	remote := &providerHubRemote{repo: repo, service: p, wallet: w, account: &Account{ID: -id, Concurrency: 1}}
	if len(w.AccountIDs) > 0 {
		a, err := p.accountRepo.GetByID(ctx, w.AccountIDs[0])
		if err != nil {
			return nil, ErrProviderRemote
		}
		remote.account = a
	}
	if w.AuthMode == "password" {
		if w.SiteType != "sub2api" {
			return nil, ErrProviderRemote
		}
		data, err := remote.call(ctx, http.MethodPost, "/api/v1/auth/login", map[string]string{"email": w.Username, "password": secret}, "")
		if err != nil {
			return nil, err
		}
		m, ok := data.(map[string]any)
		if !ok {
			return nil, ErrProviderRemote
		}
		remote.token = hubRemoteText(m["access_token"])
	} else {
		remote.token = secret
		if strings.HasPrefix(strings.TrimSpace(secret), "{") {
			var pair struct {
				Access  string `json:"access_token"`
				Refresh string `json:"refresh_token"`
			}
			if json.Unmarshal([]byte(secret), &pair) != nil {
				return nil, ErrProviderRemote
			}
			remote.token, remote.refresh = pair.Access, pair.Refresh
		}
	}
	if remote.token == "" || strings.ContainsAny(remote.token, "\r\n") {
		return nil, ErrProviderRemote
	}
	return remote, nil
}
func (r *providerHubRemote) call(ctx context.Context, method, path string, body any, operation string) (any, error) {
	s := r.service
	if _, e := s.accountTestService.validateUpstreamBaseURL(r.wallet.BaseURL); e != nil {
		return nil, ErrProviderRemote
	}
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") {
		return nil, ErrProviderRemote
	}
	raw, e := json.Marshal(body)
	if e != nil {
		return nil, ErrProviderRemote
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, e := http.NewRequestWithContext(WithHTTPUpstreamRedirectsDisabled(WithHTTPUpstreamProfile(callCtx, HTTPUpstreamProfileDefault)), method, r.wallet.BaseURL+path, bytes.NewReader(raw))
	if e != nil {
		return nil, ErrProviderRemote
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if r.token != "" {
		req.Header.Set("Authorization", "Bearer "+r.token)
	}
	if r.wallet.SiteType == "newapi" && r.wallet.RemoteUserID != "" {
		req.Header.Set("New-Api-User", r.wallet.RemoteUserID)
	}
	if operation != "" {
		req.Header.Set("Idempotency-Key", operation)
	}
	proxy := ""
	a := r.account
	if a.ProxyID != nil {
		if a.Proxy == nil || a.Proxy.ID != *a.ProxyID {
			return nil, ErrProviderRemote
		}
		proxy = a.Proxy.URL()
	}
	var profile *tlsfingerprint.Profile
	if s.accountTestService.tlsFPProfileService != nil {
		profile = s.accountTestService.tlsFPProfileService.ResolveTLSProfile(a)
	}
	resp, e := s.accountTestService.httpUpstream.DoWithTLS(req, proxy, a.ID, 1, profile)
	if e != nil || resp == nil || resp.Body == nil {
		return nil, ErrProviderRemote
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized && r.refresh != "" && path != "/api/v1/auth/refresh" && r.wallet.SiteType == "sub2api" {
		_ = resp.Body.Close()
		refresh := r.refresh
		r.refresh = ""
		fresh, e := r.call(ctx, http.MethodPost, "/api/v1/auth/refresh", map[string]string{"refresh_token": refresh}, "")
		if e != nil {
			return nil, ErrProviderRemote
		}
		m, ok := fresh.(map[string]any)
		if !ok {
			return nil, ErrProviderRemote
		}
		r.token = hubRemoteText(m["access_token"])
		r.refresh = hubRemoteText(m["refresh_token"])
		if r.refresh == "" {
			r.refresh = refresh
		}
		if r.token == "" || strings.ContainsAny(r.token+r.refresh, "\r\n") {
			return nil, ErrProviderRemote
		}
		pair, _ := json.Marshal(map[string]string{"access_token": r.token, "refresh_token": r.refresh})
		cipher, e := s.newAPIEncryptor.Encrypt(string(pair))
		if e != nil {
			return nil, ErrProviderRemote
		}
		if e = r.repo.RotateProviderHubCredential(ctx, r.wallet, cipher); e != nil {
			return nil, ErrProviderHubConflict
		}
		// One refresh per operation; a second rejection is surfaced immediately.
		savedRefresh := r.refresh
		r.refresh = ""
		data, e := r.call(ctx, method, path, body, operation)
		r.refresh = savedRefresh
		return data, e
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, ErrProviderRemote
	}
	limited := io.LimitReader(resp.Body, 2*1024*1024+1)
	encoded, e := io.ReadAll(limited)
	if e != nil || len(encoded) > 2*1024*1024 {
		return nil, ErrProviderRemote
	}
	var envelope map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&envelope) != nil {
		return nil, ErrProviderRemote
	}
	if ok, exists := envelope["success"].(bool); exists && !ok {
		return nil, ErrProviderRemote
	}
	if code, exists := envelope["code"]; exists {
		v := hubRemoteText(code)
		if v != "0" && v != "200" {
			return nil, ErrProviderRemote
		}
	}
	if data, ok := envelope["data"]; ok {
		return data, nil
	}
	return envelope, nil
}
func (s *ChannelService) SyncProviderHubWallet(ctx context.Context, id int64) error {
	repo, e := s.providerHubRepo()
	if e != nil {
		return e
	}
	w, e := repo.ProviderHubAuthorization(ctx, id)
	if e != nil {
		return ErrProviderHubInput
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	var balance *float64
	var observedAt *time.Time
	unit, source, reason := "", "", ""
	if w.AuthMode == "none" {
		// Reuse one explicitly bound account's wallet snapshot, not a sum of key limits.
		if s.providerProbe != nil {
			for _, accountID := range w.AccountIDs {
				a, err := s.providerProbe.accountRepo.GetByID(ctx, accountID)
				if err != nil {
					continue
				}
				snap := decodeUpstreamBillingProbeSnapshot(a.Extra)
				if snap == nil || snap.Balance == nil {
					continue
				}
				b := snap.Balance
				valid, hasValidity := b.Data["is_valid"].(bool)
				wallet, hasWallet := opsNumber(b.Data["wallet_balance"])
				if b.Status == "ok" && opsFresh(b.ReceivedAt, b.FreshUntil, time.Now()) && (!hasValidity || valid) && hasWallet {
					balance = &wallet
					observedAt = b.ReceivedAt
					unit, _ = b.Data["unit"].(string)
					source = "linked_account_snapshot"
					break
				}
			}
		}
		if balance == nil {
			reason = "no_fresh_wallet_snapshot"
		}
	} else {
		remote, err := s.openProviderHubRemote(ctx, id)
		if err != nil {
			reason = "authorization_or_connection_failed"
		} else {
			path := "/api/v1/user/profile"
			if w.SiteType == "newapi" {
				path = "/api/user/self"
			}
			data, err := remote.call(ctx, http.MethodGet, path, nil, "")
			if err != nil {
				reason = "profile_unavailable"
			} else if profile, ok := data.(map[string]any); ok {
				key := "balance"
				if w.SiteType == "newapi" {
					key = "quota"
				}
				if n, ok := hubRemoteNumber(profile[key]); ok {
					balance = &n
					unit = "USD"
					source = w.SiteType + "_profile"
				}
				if w.SiteType == "newapi" && balance != nil {
					status, err := remote.call(ctx, http.MethodGet, "/api/status", nil, "")
					if err != nil {
						reason = "quota_conversion_unavailable"
					} else if m, ok := status.(map[string]any); ok {
						if factor, ok := hubRemoteNumber(m["quota_per_unit"]); ok && factor > 0 {
							n := *balance / factor
							balance = &n
						} else {
							reason = "quota_conversion_unavailable"
						}
					} else {
						reason = "invalid_status"
					}
				}
			} else {
				reason = "invalid_profile"
			}
			if balance == nil && reason == "" {
				reason = "balance_not_reported"
			}
		}
	}
	if reason == "" && unit != w.CreditCurrency {
		reason = "wallet_currency_mismatch"
	}
	if err := repo.SaveProviderHubObservation(ctx, w, balance, unit, source, reason, observedAt); err != nil {
		return err
	}
	if reason != "" {
		return ErrProviderRemote
	}
	return nil
}
func (r *providerHubRemote) keys(ctx context.Context) ([]ProviderHubRemoteKey, error) {
	out := []ProviderHubRemoteKey{}
	seen := map[string]bool{}
	legacy := false
	for page := 1; page <= 50; page++ {
		path := fmt.Sprintf("/api/v1/keys?page=%d&page_size=100", page)
		if r.wallet.SiteType == "newapi" {
			path = fmt.Sprintf("/api/token/?p=%d&page_size=100", page)
		}
		data, e := r.call(ctx, http.MethodGet, path, nil, "")
		if e != nil {
			return nil, e
		}
		if _, bare := data.([]any); bare && r.wallet.SiteType == "newapi" && !legacy {
			legacy = true
			page = 0
			data, e = r.call(ctx, http.MethodGet, "/api/token/?p=0&page_size=100", nil, "")
			if e != nil {
				return nil, e
			}
		}
		rows := hubRemoteRows(data)
		if len(rows) == 0 {
			switch m := data.(type) {
			case []any:
				return out, nil
			case map[string]any:
				if _, ok := m["items"].([]any); ok {
					return out, nil
				}
				if n, ok := hubRemoteNumber(m["total"]); ok && n == 0 {
					return out, nil
				}
			}
			return nil, ErrProviderRemote
		}
		before := len(out)
		for _, row := range rows {
			id := hubRemoteText(row["id"])
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			key := ProviderHubRemoteKey{ID: id, Name: sanitizeUpstreamBalanceText(hubRemoteText(row["name"]), 120), Group: hubRemoteText(row["group_id"]), Secret: hubRemoteText(row["key"])}
			if r.wallet.SiteType == "newapi" {
				key.Group = hubRemoteText(row["group"])
				key.Available = hubRemoteText(row["status"]) == "1"
			} else {
				key.Available = hubRemoteText(row["status"]) == "active"
			}
			if expires := hubRemoteText(row["expires_at"]); expires != "" {
				if at, e := time.Parse(time.RFC3339, expires); e == nil && !at.After(time.Now()) {
					key.Available = false
				}
			}
			if expiry, ok := hubRemoteNumber(row["expired_time"]); ok && expiry > 0 && expiry <= float64(time.Now().Unix()) {
				key.Available = false
			}
			if quota, ok := hubRemoteNumber(row["quota"]); ok && quota > 0 {
				if used, ok := hubRemoteNumber(row["quota_used"]); ok && used >= quota {
					key.Available = false
				}
			}
			if remain, ok := hubRemoteNumber(row["remain_quota"]); ok && remain <= 0 {
				if unlimited, _ := row["unlimited_quota"].(bool); !unlimited {
					key.Available = false
				}
			}
			key.MaskedKey = maskNewAPIKey(key.Secret)
			out = append(out, key)
		}
		if len(out) == before {
			return nil, ErrProviderRemote
		}
		if len(out) > 1000 {
			break
		}
		if m, ok := data.(map[string]any); ok {
			if total, ok := hubRemoteNumber(m["total"]); ok && float64(len(out)) >= total {
				return out, nil
			}
		}
		if !legacy && len(rows) < 100 {
			return out, nil
		}
	}
	return nil, infraerrors.BadRequest("PROVIDER_KEY_LIMIT", "Supplier has more than 1000 keys; select a narrower supplier login")
}
func (s *ChannelService) ProviderHubRemoteKeys(ctx context.Context, id int64) ([]ProviderHubRemoteKey, error) {
	remote, e := s.openProviderHubRemote(ctx, id)
	if e != nil {
		return nil, e
	}
	return remote.keys(ctx)
}
func (s *ChannelService) ProviderHubRemoteGroups(ctx context.Context, id int64) ([]ProviderHubRemoteGroup, error) {
	remote, e := s.openProviderHubRemote(ctx, id)
	if e != nil {
		return nil, e
	}
	return remote.groups(ctx)
}
func (remote *providerHubRemote) groups(ctx context.Context) ([]ProviderHubRemoteGroup, error) {
	path := "/api/v1/groups/available"
	if remote.wallet.SiteType == "newapi" {
		path = "/api/user/self/groups"
	}
	data, e := remote.call(ctx, http.MethodGet, path, nil, "")
	if e != nil {
		return nil, e
	}
	out := []ProviderHubRemoteGroup{}
	if remote.wallet.SiteType == "newapi" {
		if m, ok := data.(map[string]any); ok {
			for name := range m {
				out = append(out, ProviderHubRemoteGroup{ID: name, Name: name})
			}
		}
	} else {
		for _, row := range hubRemoteRows(data) {
			out = append(out, ProviderHubRemoteGroup{ID: hubRemoteText(row["id"]), Name: sanitizeUpstreamBalanceText(hubRemoteText(row["name"]), 120)})
		}
	}
	return out, nil
}
func (s *ChannelService) CreateProviderHubRemoteKey(ctx context.Context, id int64, name, group, operation string) error {
	if len(strings.TrimSpace(name)) < 1 || len(name) > 100 || len(group) < 1 || len(group) > 128 || len(operation) < 12 || len(operation) > 128 || strings.ContainsAny(name+group+operation, "\r\n\x00") {
		return ErrProviderHubInput
	}
	remote, e := s.openProviderHubRemote(ctx, id)
	if e != nil {
		return e
	}
	groups, e := remote.groups(ctx)
	if e != nil {
		return e
	}
	valid := false
	for _, g := range groups {
		if g.ID == group {
			valid = true
		}
	}
	if !valid {
		return ErrProviderHubInput
	}
	var body any
	path := "/api/v1/keys"
	if remote.wallet.SiteType == "newapi" {
		path = "/api/token/"
		body = map[string]any{"name": name, "group": group, "remain_quota": 0, "unlimited_quota": true, "expired_time": -1, "model_limits_enabled": false, "model_limits": "", "allow_ips": "", "cross_group_retry": false}
	} else {
		n, e := strconv.ParseInt(group, 10, 64)
		if e != nil || n <= 0 {
			return ErrProviderHubInput
		}
		body = map[string]any{"name": name, "group_id": n}
	}
	start, e := remote.repo.BeginProviderHubKeyCreation(ctx, id, operation, name, group)
	if e != nil {
		return e
	}
	if !start {
		return nil
	}
	_, e = remote.call(ctx, http.MethodPost, path, body, operation)
	finishCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	finishError := remote.repo.FinishProviderHubKeyCreation(finishCtx, operation, e == nil)
	if e != nil {
		return e
	}
	return finishError
}

// Import uses the normal account creation service and keeps all upstream secrets
// on the server. Already-imported keys are matched through explicit metadata.
func (s *ChannelService) ImportProviderHubKeys(ctx context.Context, id int64, req ProviderHubImport, admin AdminService) ([]int64, error) {
	if admin == nil || len(req.KeyIDs) < 1 || len(req.KeyIDs) > 20 || len(req.Prefix) > 60 || !slicesContains([]string{"openai", "anthropic", "gemini", "grok", "deepseek", "qwen", "minimax", "zhipu"}, req.Platform) {
		return nil, ErrProviderHubInput
	}
	repo, e := s.providerHubRepo()
	if e != nil {
		return nil, e
	}
	release, e := repo.LockProviderHubImport(ctx, id)
	if e != nil {
		return nil, e
	}
	defer release()
	remote, e := s.openProviderHubRemote(ctx, id)
	if e != nil {
		return nil, e
	}
	if req.Revision != remote.wallet.Revision {
		return nil, ErrProviderHubConflict
	}
	keys, e := remote.keys(ctx)
	if e != nil {
		return nil, e
	}
	byID := map[string]ProviderHubRemoteKey{}
	for _, k := range keys {
		byID[k.ID] = k
	}
	out := []int64{}
	seen := map[string]bool{}
	for _, keyID := range req.KeyIDs {
		if seen[keyID] {
			return nil, ErrProviderHubInput
		}
		seen[keyID] = true
		key, ok := byID[keyID]
		if !ok || !key.Available {
			return nil, ErrProviderHubInput
		}
		marker := fmt.Sprintf("%d:%s", id, keyID)
		previous, e := s.providerProbe.accountRepo.FindByExtraField(ctx, "supplier_key_binding", marker)
		if e != nil {
			return nil, e
		}
		if len(previous) > 0 {
			a := previous[0]
			_, e = s.MutateProviderHub(ctx, ProviderHubCommand{Action: "bindings", ID: id, Revision: remote.wallet.Revision, AccountIDs: []int64{a.ID}})
			if e != nil {
				return nil, e
			}
			remote.wallet.Revision++
			out = append(out, a.ID)
			continue
		}
		if remote.wallet.SiteType == "newapi" && (key.Secret == "" || strings.Contains(key.Secret, "*")) {
			path := "/api/token/" + url.PathEscape(keyID) + "/key"
			data, e := remote.call(ctx, http.MethodPost, path, map[string]any{}, "")
			if e != nil {
				return nil, e
			}
			if m, ok := data.(map[string]any); ok {
				key.Secret = hubRemoteText(m["key"])
			} else {
				key.Secret = hubRemoteText(data)
			}
		}
		if key.Secret == "" || strings.Contains(key.Secret, "*") || strings.ContainsAny(key.Secret, "\r\n") {
			return nil, ErrProviderRemote
		}
		if remote.wallet.SiteType == "newapi" && !strings.HasPrefix(key.Secret, "sk-") {
			key.Secret = "sk-" + key.Secret
		}
		name := strings.TrimSpace(req.Prefix + key.Name)
		if name == "" {
			name = "Supplier key " + keyID
		}
		probeEnabled := false
		account, e := admin.CreateAccount(ctx, &CreateAccountInput{Name: name, Platform: req.Platform, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": remote.wallet.BaseURL, "api_key": key.Secret}, Extra: map[string]any{"supplier_key_binding": marker}, Concurrency: 1, Priority: 50, GroupIDs: req.GroupIDs, SkipDefaultGroupBind: true, ProbeEnabled: &probeEnabled})
		if e != nil {
			return nil, e
		}
		_, e = s.MutateProviderHub(ctx, ProviderHubCommand{Action: "bindings", ID: id, Revision: remote.wallet.Revision, AccountIDs: []int64{account.ID}})
		if e != nil {
			return nil, e
		}
		remote.wallet.Revision++
		out = append(out, account.ID)
	}
	return out, nil
}
