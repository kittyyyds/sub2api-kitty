package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Like VM2's saved-login store, these records are encrypted separately from
// account credentials. No password or TOTP is returned by account APIs/export.
var savedOpenAILoginMu sync.Mutex

type savedOpenAILoginStore struct{ dir string }
type savedOpenAILoginRecord struct {
	Scope       string                   `json:"scope"`
	Input       OpenAIPasswordLoginInput `json:"input"`
	ExpiresAt   int64                    `json:"expires_at,omitempty"`
	Authorized  bool                     `json:"authorized,omitempty"`
	WorkspaceID string                   `json:"workspace_id,omitempty"`
}
type SavedOpenAILoginSummary struct {
	Email        string `json:"email"`
	HasPassword  bool   `json:"has_password"`
	HasMFASecret bool   `json:"has_mfa_secret"`
	MFAMode      string `json:"mfa_mode"`
}

func openAISavedLoginStore() savedOpenAILoginStore {
	root := strings.TrimSpace(os.Getenv("DATA_DIR"))
	if root == "" {
		root = "data"
	}
	return savedOpenAILoginStore{dir: filepath.Join(root, "openai-saved-logins")}
}
func (s savedOpenAILoginStore) file(scope string) string {
	h := sha256.Sum256([]byte(scope))
	prefix := ""
	if strings.HasPrefix(scope, "pending:") {
		prefix = "pending-"
	}
	return filepath.Join(s.dir, prefix+hex.EncodeToString(h[:])+".enc")
}
func (s savedOpenAILoginStore) key(create bool) ([]byte, error) {
	path := filepath.Join(s.dir, ".key")
	if create {
		if err := os.MkdirAll(s.dir, 0700); err != nil {
			return nil, err
		}
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			key := make([]byte, 32)
			_, err = rand.Read(key)
			if err == nil {
				_, err = f.Write(key)
			}
			closeErr := f.Close()
			if err != nil {
				_ = os.Remove(path)
				return nil, err
			}
			if closeErr != nil {
				return nil, closeErr
			}
		} else if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
	}
	key, err := os.ReadFile(path)
	if err != nil || len(key) != 32 {
		return nil, fmt.Errorf("saved login key unavailable")
	}
	return key, nil
}
func (s savedOpenAILoginStore) read(scope string) (*savedOpenAILoginRecord, error) {
	data, err := os.ReadFile(s.file(scope))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	key, err := s.key(false)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(data) < aead.NonceSize() {
		return nil, fmt.Errorf("invalid saved login")
	}
	plain, err := aead.Open(nil, data[:aead.NonceSize()], data[aead.NonceSize():], []byte(scope))
	if err != nil {
		return nil, err
	}
	var record savedOpenAILoginRecord
	if json.Unmarshal(plain, &record) != nil || record.Scope != scope {
		return nil, fmt.Errorf("invalid saved login scope")
	}
	if record.ExpiresAt > 0 && record.ExpiresAt <= time.Now().Unix() {
		_ = os.Remove(s.file(scope))
		return nil, nil
	}
	return &record, nil
}
func (s savedOpenAILoginStore) write(record savedOpenAILoginRecord) error {
	key, err := s.key(true)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = rand.Read(nonce); err != nil {
		return err
	}
	plain, err := json.Marshal(record)
	if err != nil {
		return err
	}
	data := aead.Seal(nonce, nonce, plain, []byte(record.Scope))
	f, err := os.CreateTemp(s.dir, ".pending-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(f.Name(), s.file(record.Scope))
}
func pendingOpenAILoginScope(owner int64, job string) string {
	return fmt.Sprintf("pending:%d:%s", owner, job)
}
func accountOpenAILoginScope(account *Account) string {
	return fmt.Sprintf("account:%d:%s", account.ID, account.CreatedAt.UTC().Format(time.RFC3339Nano))
}
func savedLoginAccountEmail(account *Account) string {
	if account == nil {
		return ""
	}
	email := account.GetCredential("email")
	if email == "" {
		email, _ = account.Extra["email"].(string)
	}
	return strings.ToLower(strings.TrimSpace(email))
}
func eligibleSavedOpenAILogin(account *Account) bool {
	return account != nil && account.Platform == PlatformOpenAI && account.Type == AccountTypeOAuth &&
		account.ParentAccountID == nil && !account.IsOpenAIVMManaged()
}
func (s savedOpenAILoginStore) prunePending() {
	paths, _ := filepath.Glob(filepath.Join(s.dir, "pending-*.enc"))
	for _, path := range paths {
		if info, err := os.Stat(path); err == nil && time.Since(info.ModTime()) > 15*time.Minute {
			_ = os.Remove(path)
		}
	}
}
func (s *OpenAIOAuthService) RememberPasswordLogin(owner int64, job string, account *Account) error {
	if owner <= 0 || !passwordLoginJobID.MatchString(job) || !eligibleSavedOpenAILogin(account) {
		return passwordLoginError(400, "invalid_input")
	}
	savedOpenAILoginMu.Lock()
	defer savedOpenAILoginMu.Unlock()
	store := openAISavedLoginStore()
	record, err := store.read(pendingOpenAILoginScope(owner, job))
	if err != nil {
		return passwordLoginError(503, "saved_login_unavailable")
	}
	if record == nil || !record.Authorized {
		return passwordLoginError(404, "job_missing")
	}
	if savedLoginAccountEmail(account) != strings.ToLower(record.Input.Email) {
		return passwordLoginError(400, "identity_mismatch")
	}
	if record.WorkspaceID == "" || record.WorkspaceID != account.GetCredential("chatgpt_account_id") {
		return passwordLoginError(400, "workspace_mismatch")
	}
	record.Scope = accountOpenAILoginScope(account)
	record.ExpiresAt = 0
	record.Input.ProxyID = nil
	record.Input.Remember = false
	if err = store.write(*record); err != nil {
		return passwordLoginError(503, "saved_login_unavailable")
	}
	_ = os.Remove(store.file(pendingOpenAILoginScope(owner, job)))
	return nil
}
func (s *OpenAIOAuthService) SavedPasswordLogin(account *Account) (*SavedOpenAILoginSummary, error) {
	input, err := s.savedPasswordLoginInput(account)
	if err != nil || input == nil {
		return nil, err
	}
	return &SavedOpenAILoginSummary{Email: input.Email, HasPassword: true, HasMFASecret: input.MFASecret != "", MFAMode: input.MFAMode}, nil
}
func (s *OpenAIOAuthService) savedPasswordLoginInput(account *Account) (*OpenAIPasswordLoginInput, error) {
	if !eligibleSavedOpenAILogin(account) {
		return nil, passwordLoginError(400, "invalid_input")
	}
	savedOpenAILoginMu.Lock()
	defer savedOpenAILoginMu.Unlock()
	record, err := openAISavedLoginStore().read(accountOpenAILoginScope(account))
	if err != nil {
		return nil, passwordLoginError(503, "saved_login_unavailable")
	}
	if record == nil || savedLoginAccountEmail(account) != strings.ToLower(record.Input.Email) || record.WorkspaceID != account.GetCredential("chatgpt_account_id") {
		return nil, nil
	}
	record.Input.ProxyID = account.ProxyID
	return &record.Input, nil
}
func (s *OpenAIOAuthService) ForgetPasswordLogin(account *Account) error {
	if !eligibleSavedOpenAILogin(account) {
		return passwordLoginError(400, "invalid_input")
	}
	savedOpenAILoginMu.Lock()
	defer savedOpenAILoginMu.Unlock()
	err := os.Remove(openAISavedLoginStore().file(accountOpenAILoginScope(account)))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return passwordLoginError(503, "saved_login_unavailable")
	}
	return nil
}
func (s *OpenAIOAuthService) StartSavedPasswordLogin(ctx context.Context, owner int64, account *Account) (*OpenAIPasswordLoginJob, error) {
	input, err := s.savedPasswordLoginInput(account)
	if err != nil {
		return nil, err
	}
	if input == nil {
		return nil, passwordLoginError(404, "saved_login_missing")
	}
	input.Remember = false
	return s.StartPasswordLogin(ctx, owner, *input)
}
