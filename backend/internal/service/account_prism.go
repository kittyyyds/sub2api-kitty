package service

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
)

const prismCookieCipherVersion = "prism:v1:"

// PrismCredentialsUpdate records the editor's baseline and explicitly supplied
// secret fields. Repositories merge this edit into the locked current row so
// an OAuth refresh completed after the account read cannot be undone.
type PrismCredentialsUpdate struct {
	Original         map[string]any
	ExplicitKeys     []string
	OriginalPlatform string
	OriginalType     string
}

type AccountPrismSettingsRepository interface {
	UpdateWithPrismAccountSettings(context.Context, *Account, PrismCredentialsUpdate, *bool, *bool, *float64) error
}

func isPrismBindingOnlyCredentials(incoming map[string]any) bool {
	if len(incoming) == 0 {
		return false
	}
	for key := range incoming {
		if key != PrismCookieKey && key != PrismUserIDKey {
			return false
		}
	}
	return true
}

func prismCredentialsUpdate(previous, desired *Account, incoming map[string]any) (*PrismCredentialsUpdate, error) {
	relevant := false
	for _, a := range []*Account{previous, desired} {
		for _, key := range []string{PrismCookieKey, PrismCookieEncryptedKey, PrismUserIDKey} {
			_, present := a.Credentials[key]
			relevant = relevant || present
		}
		for _, key := range []string{PrismEnabledExtraKey, PrismModelsExtraKey} {
			_, present := a.Extra[key]
			relevant = relevant || present
		}
	}
	if _, supplied := incoming[PrismCookieKey]; supplied {
		relevant = true
	}
	if !relevant {
		return nil, nil
	}
	original, err := cloneAccountJSONMap(previous.Credentials)
	if err != nil {
		return nil, err
	}
	update := &PrismCredentialsUpdate{Original: original, OriginalPlatform: previous.Platform, OriginalType: previous.Type}
	for key := range incoming {
		if IsSensitiveCredentialKey(key) || key == PrismUserIDKey {
			update.ExplicitKeys = append(update.ExplicitKeys, key)
		}
	}
	if _, supplied := incoming[PrismCookieKey]; supplied {
		// Empty Cookie is an explicit unlink even when the read snapshot did
		// not yet contain a concurrently created binding.
		update.ExplicitKeys = append(update.ExplicitKeys, PrismCookieEncryptedKey)
	}
	return update, nil
}

// PrismFeatureEnabled is an explicit opt-in for the local experimental adapter.
// An account flag alone cannot activate the private upstream protocol.
func PrismFeatureEnabled() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("SUB2API_PRISM_ENABLED")), "true")
}

func prismAccountEligible(a *Account) bool {
	return a != nil && a.Platform == PlatformOpenAI && a.Type == AccountTypeOAuth &&
		!a.IsShadow() && !a.IsOpenAIAgentIdentity() && !a.IsOpenAIPersonalAccessToken()
}

func prismCookieCipher(cfg *config.Config) (cipher.AEAD, error) {
	if cfg == nil || !cfg.Totp.EncryptionKeyConfigured {
		return nil, infraerrors.BadRequest("PRISM_ENCRYPTION_KEY_REQUIRED", "Prism requires a persistent configured encryption key")
	}
	key, err := hex.DecodeString(strings.TrimSpace(cfg.Totp.EncryptionKey))
	if err != nil || len(key) != 32 {
		return nil, infraerrors.BadRequest("PRISM_ENCRYPTION_KEY_INVALID", "Prism encryption key is invalid")
	}
	// Separate the Prism secret domain from TOTP and every other use of the root key.
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("sub2api:prism-cookie:v1"))
	block, err := aes.NewCipher(mac.Sum(nil))
	if err != nil {
		return nil, errors.New("Prism encryption is unavailable")
	}
	return cipher.NewGCM(block)
}

func encryptPrismCookie(cookie string, cfg *config.Config) (string, error) {
	aead, err := prismCookieCipher(cfg)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", errors.New("Prism encryption failed")
	}
	sealed := aead.Seal(nonce, nonce, []byte(cookie), []byte(prismCookieCipherVersion))
	return prismCookieCipherVersion + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// DecryptPrismCookie returns the bound session only to the selected-account
// forwarder. Do not put this value into account snapshots, logs or API output.
func DecryptPrismCookie(a *Account, cfg *config.Config) (string, error) {
	if !prismAccountEligible(a) {
		return "", infraerrors.BadRequest("PRISM_ACCOUNT_INVALID", "Prism requires a regular ChatGPT OAuth account")
	}
	ciphertext := a.GetPrismCookieCiphertext()
	if !strings.HasPrefix(ciphertext, prismCookieCipherVersion) {
		return "", infraerrors.BadRequest("PRISM_COOKIE_REQUIRED", "Bind this GPT account's Prism session first")
	}
	aead, err := prismCookieCipher(cfg)
	if err != nil {
		return "", err
	}
	sealed, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(ciphertext, prismCookieCipherVersion))
	if err != nil || len(sealed) < aead.NonceSize()+aead.Overhead() {
		return "", errors.New("Prism session could not be decrypted")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], []byte(prismCookieCipherVersion))
	if err != nil || len(plain) == 0 || strings.ContainsAny(string(plain), "\r\n") {
		return "", errors.New("Prism session could not be decrypted")
	}
	return string(plain), nil
}

func validatePrismInputCredentials(incoming map[string]any) error {
	if _, ok := incoming[PrismCookieEncryptedKey]; ok {
		return infraerrors.BadRequest("PRISM_MANAGED_CREDENTIAL", "Prism encrypted session is managed by the server; submit a new session instead")
	}
	for _, key := range []string{PrismBaseURLKey, PrismProjectIDKey} {
		if v, ok := incoming[key]; ok && v != nil && v != "" {
			return infraerrors.BadRequest("PRISM_FIXED_SESSION_POLICY", "Prism uses its fixed official origin and a separate project for every request")
		}
	}
	if raw, ok := incoming[PrismCookieKey]; ok {
		cookie, valid := raw.(string)
		if !valid || len(cookie) > 64<<10 || strings.ContainsAny(cookie, "\r\n") {
			return infraerrors.BadRequest("PRISM_COOKIE_INVALID", "Prism session must be a bounded single-line Cookie string")
		}
	}
	return nil
}

// Internal re-auth/import callers may pass a merged copy of stored credentials.
// Accept only an unchanged ciphertext, then let the normal sensitive merge retain
// it. A client can never replace or inject server-managed ciphertext this way.
func prismCredentialsForUpdate(incoming, existing map[string]any) (map[string]any, error) {
	if raw, supplied := incoming[PrismCookieEncryptedKey]; supplied {
		value, ok := raw.(string)
		stored, storedOK := existing[PrismCookieEncryptedKey].(string)
		if !ok || !storedOK || stored == "" || value != stored {
			return nil, infraerrors.BadRequest("PRISM_MANAGED_CREDENTIAL", "Prism encrypted session is managed by the server; submit a new session instead")
		}
		incoming = shallowCopyMap(incoming)
		delete(incoming, PrismCookieEncryptedKey)
	}
	return incoming, validatePrismInputCredentials(incoming)
}

func validatePrismExtraSecrets(extra map[string]any) error {
	for _, key := range []string{PrismCookieKey, PrismCookieEncryptedKey, PrismBaseURLKey, PrismProjectIDKey, PrismUserIDKey} {
		if _, ok := extra[key]; ok {
			return infraerrors.BadRequest("PRISM_EXTRA_INVALID", "Prism session fields cannot be stored in extra")
		}
	}
	return nil
}

func normalizePrismModelScope(extra map[string]any) error {
	if raw, ok := extra[PrismEnabledExtraKey]; ok {
		if _, valid := raw.(bool); !valid {
			return infraerrors.BadRequest("PRISM_SETTINGS_INVALID", "openai_prism must be a boolean")
		}
	}
	raw, ok := extra[PrismModelsExtraKey]
	if !ok {
		return nil
	}
	var values []string
	switch list := raw.(type) {
	case []string:
		values = list
	case []any:
		for _, item := range list {
			v, valid := item.(string)
			if !valid {
				return infraerrors.BadRequest("PRISM_SETTINGS_INVALID", "openai_prism_models must be an array of model names")
			}
			values = append(values, v)
		}
	default:
		return infraerrors.BadRequest("PRISM_SETTINGS_INVALID", "openai_prism_models must be an array of model names")
	}
	normalized := make([]string, 0, len(values))
	seen := make(map[string]bool)
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		if len(value) > 200 || strings.ContainsAny(value, "\r\n") {
			return infraerrors.BadRequest("PRISM_SETTINGS_INVALID", "Invalid Prism model name")
		}
		seen[value] = true
		normalized = append(normalized, value)
	}
	extra[PrismModelsExtraKey] = normalized
	return nil
}

func validatePrismProtocolConflict(a *Account) error {
	if !a.IsPrismEnabled() || !a.IsExcelBPSEnabled() {
		return nil
	}
	prismScope, prismScoped := a.Extra[PrismModelsExtraKey]
	bpsScope, bpsScoped := a.Extra["openai_excel_bps_models"]
	scopeValues := func(raw any) []string {
		switch values := raw.(type) {
		case []string:
			return values
		case []any:
			out := make([]string, 0, len(values))
			for _, v := range values {
				if s, ok := v.(string); ok {
					out = append(out, s)
				}
			}
			return out
		}
		return nil
	}
	p, b := scopeValues(prismScope), scopeValues(bpsScope)
	if (prismScoped && len(p) == 0) || (bpsScoped && len(b) == 0) {
		return nil
	}
	overlap := !prismScoped || !bpsScoped
	for _, model := range p {
		if a.isExcelBPSUpstreamModelEnabled(strings.TrimSpace(model)) {
			overlap = true
		}
	}
	if overlap {
		return infraerrors.BadRequest("PRISM_PROTOCOL_CONFLICT", "Prism and Excel/BPS cannot both route the same model; disable one or use disjoint model scopes")
	}
	return nil
}

// preparePrismAccountForSave runs before a repository write and never mutates
// incoming. Ordinary OAuth token refresh preserves only the encrypted field.
func preparePrismAccountForSave(a, previous *Account, incoming map[string]any, cfg *config.Config) error {
	if err := validatePrismInputCredentials(incoming); err != nil {
		return err
	}
	if err := validatePrismExtraSecrets(a.Extra); err != nil {
		return err
	}
	if err := normalizePrismModelScope(a.Extra); err != nil {
		return err
	}
	enabled, _ := a.Extra[PrismEnabledExtraKey].(bool)
	_, newCookie := incoming[PrismCookieKey]
	hasBinding := a.GetPrismCookieCiphertext() != "" || a.GetCredential(PrismCookieKey) != ""
	if (enabled || hasBinding || newCookie) && !prismAccountEligible(a) {
		return infraerrors.BadRequest("PRISM_ACCOUNT_INVALID", "Prism requires a regular ChatGPT OAuth account")
	}
	for _, key := range []string{PrismBaseURLKey, PrismProjectIDKey} {
		if a.GetCredential(key) != "" {
			return infraerrors.BadRequest("PRISM_FIXED_SESSION_POLICY", "Prism uses its fixed official origin and a separate project for every request")
		}
	}
	if userID, ok := a.Credentials[PrismUserIDKey]; ok {
		value, valid := userID.(string)
		if !valid || len(value) > 512 || strings.ContainsAny(value, "\r\n") {
			return infraerrors.BadRequest("PRISM_USER_ID_INVALID", "Invalid Prism user ID")
		}
	}
	if newCookie {
		cookie := strings.TrimSpace(incoming[PrismCookieKey].(string))
		if cookie != "" && !PrismFeatureEnabled() {
			return infraerrors.BadRequest("PRISM_FEATURE_DISABLED", "Prism is disabled; enable SUB2API_PRISM_ENABLED for the local trial")
		}
		a.Credentials = shallowCopyMap(a.Credentials)
		delete(a.Credentials, PrismCookieKey)
		delete(a.Credentials, PrismCookieEncryptedKey)
		if cookie != "" {
			ciphertext, err := encryptPrismCookie(cookie, cfg)
			if err != nil {
				return err
			}
			a.Credentials[PrismCookieEncryptedKey] = ciphertext
		}
	} else if a.GetCredential(PrismCookieKey) != "" {
		return infraerrors.BadRequest("PRISM_PLAINTEXT_REJECTED", "Rebind the Prism session through the account editor before enabling it")
	}
	if enabled {
		if !PrismFeatureEnabled() && (previous == nil || !previous.IsPrismEnabled()) {
			return infraerrors.BadRequest("PRISM_FEATURE_DISABLED", "Prism is disabled; enable SUB2API_PRISM_ENABLED for the local trial")
		}
		if _, err := DecryptPrismCookie(a, cfg); err != nil {
			return err
		}
	}
	return validatePrismProtocolConflict(a)
}

func rejectPrismBulkFields(credentials, extra map[string]any) error {
	if err := validatePrismExtraSecrets(extra); err != nil {
		return err
	}
	for _, key := range []string{PrismCookieKey, PrismCookieEncryptedKey, PrismBaseURLKey, PrismProjectIDKey, PrismUserIDKey} {
		if _, ok := credentials[key]; ok {
			return infraerrors.BadRequest("PRISM_ACCOUNT_EDITOR_REQUIRED", "Bind Prism sessions one GPT account at a time in the account editor")
		}
	}
	for _, key := range []string{PrismEnabledExtraKey, PrismModelsExtraKey} {
		if _, ok := extra[key]; ok {
			return infraerrors.BadRequest("PRISM_ACCOUNT_EDITOR_REQUIRED", "Configure Prism one GPT account at a time in the account editor")
		}
	}
	return nil
}
