package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

type AccountImportTicket struct {
	ID           string    `json:"id"`
	Platform     string    `json:"platform"`
	Source       string    `json:"source"`
	ProxyID      *int64    `json:"proxy_id"`
	IdentityKey  string    `json:"-"`
	State        string    `json:"state"`
	AccountID    *int64    `json:"account_id,omitempty"`
	AutoAssigned bool      `json:"auto_assigned"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type AccountImportStore interface {
	BeginAccountImport(context.Context, string, string, *int64, string) (*AccountImportTicket, error)
	ReadyAccountImport(context.Context, string, string) error
	FinalizeOAuthImport(context.Context, string, string, string) error
	ReleaseAccountImport(context.Context, string) error
	ResolveAccountImport(context.Context, string, string) (*AccountImportTicket, error)
}
type AccountImportCoordinator interface {
	PrepareAccountImport(context.Context, string, string, *int64, string) (*AccountImportTicket, error)
	CompleteAccountImport(context.Context, string, map[string]any) error
	CancelAccountImport(context.Context, string) error
}

func ImportCredentialFingerprint(credentials map[string]any) string {
	value, _ := credentials["access_token"].(string)
	if strings.TrimSpace(value) == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(digest[:])
}
func (s *adminServiceImpl) PrepareAccountImport(ctx context.Context, platform, source string, proxyID *int64, identity string) (*AccountImportTicket, error) {
	store, ok := s.accountRepo.(AccountImportStore)
	if !ok {
		return nil, errors.New("账号导入仓储未就绪")
	}
	policy, err := s.GetProxyBindingPolicy(ctx)
	if err != nil {
		return nil, err
	}
	if policy.Matches(platform, AccountTypeOAuth, source) && policy.AutoAssign {
		if err = s.refreshProxyBindingExits(ctx); err != nil {
			return nil, err
		}
	}
	return store.BeginAccountImport(ctx, platform, source, proxyID, strings.ToLower(strings.TrimSpace(identity)))
}
func (s *adminServiceImpl) CompleteAccountImport(ctx context.Context, id string, credentials map[string]any) error {
	store, ok := s.accountRepo.(AccountImportStore)
	if !ok {
		return errors.New("账号导入仓储未就绪")
	}
	fp := ImportCredentialFingerprint(credentials)
	if fp == "" {
		return errors.New("登录未返回完整访问凭据")
	}
	return store.ReadyAccountImport(ctx, id, fp)
}
func (s *adminServiceImpl) CancelAccountImport(ctx context.Context, id string) error {
	store, ok := s.accountRepo.(AccountImportStore)
	if !ok {
		return errors.New("账号导入仓储未就绪")
	}
	return store.ReleaseAccountImport(ctx, id)
}
func (s *adminServiceImpl) resolveVerifiedImport(ctx context.Context, input *CreateAccountInput) (*Account, error) {
	input.VerifiedImportSource = ""
	if input.ImportTicket == "" {
		r, ok := s.accountRepo.(ProxyBindingStore)
		if !ok {
			return nil, nil
		}
		p, err := r.GetProxyBindingPolicy(ctx)
		if err != nil {
			return nil, err
		}
		if p.Matches(input.Platform, input.Type, "oauth") && p.AutoAssign {
			ticket, prepareErr := s.PrepareAccountImport(ctx, input.Platform, "oauth", input.ProxyID, "")
			if prepareErr != nil {
				return nil, prepareErr
			}
			input.ImportTicket = ticket.ID
			input.ProxyID = ticket.ProxyID
			return s.resolveVerifiedImport(ctx, input)
		}
		return nil, nil
	}
	store, ok := s.accountRepo.(AccountImportStore)
	if !ok {
		return nil, errors.New("账号导入仓储未就绪")
	}
	identity := ""
	for _, key := range []string{"email", "chatgpt_user_id", "user_id", "sub", "account_id"} {
		if v, ok := input.Credentials[key].(string); ok && strings.TrimSpace(v) != "" {
			identity = strings.ToLower(strings.TrimSpace(v))
			break
		}
	}
	if identity == "" {
		identity = "credential:" + ImportCredentialFingerprint(input.Credentials)
	}
	if err := store.FinalizeOAuthImport(ctx, input.ImportTicket, ImportCredentialFingerprint(input.Credentials), identity); err != nil {
		return nil, err
	}
	ticket, err := store.ResolveAccountImport(ctx, input.ImportTicket, ImportCredentialFingerprint(input.Credentials))
	if err != nil {
		return nil, err
	}
	if ticket.Platform != input.Platform || input.Type != AccountTypeOAuth {
		return nil, errors.New("导入票据与账号类型不匹配")
	}
	if ticket.AccountID != nil {
		return s.accountRepo.GetByID(ctx, *ticket.AccountID)
	}
	if (ticket.ProxyID == nil) != (input.ProxyID == nil) || (ticket.ProxyID != nil && *ticket.ProxyID != *input.ProxyID) {
		return nil, errors.New("登录代理与保存代理不一致，请使用本次登录已分配的代理")
	}
	input.VerifiedImportSource = ticket.Source
	return nil, nil
}

type accountImportContextKey struct{}

func WithAccountImportTicket(ctx context.Context, ticket *AccountImportTicket) context.Context {
	return context.WithValue(ctx, accountImportContextKey{}, ticket)
}
func (s *AccountTokenGuardService) PrepareTwoFAImport(ctx context.Context, entry AccountTokenGuardReloginAccount, proxyID *int64) (*AccountImportTicket, error) {
	coordinator, ok := s.admin.(AccountImportCoordinator)
	if !ok {
		return nil, errors.New("账号导入协调服务未就绪")
	}
	return coordinator.PrepareAccountImport(ctx, PlatformOpenAI, "twofa", proxyID, entry.Email)
}
func (s *AccountTokenGuardService) CancelImportTicket(id string) {
	if id == "" {
		return
	}
	if coordinator, ok := s.admin.(AccountImportCoordinator); ok {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = coordinator.CancelAccountImport(ctx, id)
	}
}
