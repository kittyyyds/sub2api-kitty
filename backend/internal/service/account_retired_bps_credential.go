package service

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

// IsRetiredBPSCredential reports a primary OAuth credential that was only
// servable through the removed Excel BPS route or BPS plugin: a grant issued to
// the Excel add-in client, or a Kitty BPS plugin grant (auth_mode=bps). They are
// never valid for native Codex traffic, are kept out of scheduling (see
// isModelMappingSupported) and are rejected on every OpenAI route.
func (a *Account) IsRetiredBPSCredential() bool {
	if a == nil || !a.IsOpenAIOAuthLike() {
		return false
	}
	return strings.TrimSpace(a.GetCredential("client_id")) == openai.ExcelClientID ||
		strings.EqualFold(strings.TrimSpace(a.GetCredential("auth_mode")), "bps")
}

// RetiredBPSCredentialReason is shown to administrators for retired BPS credentials.
const RetiredBPSCredentialReason = "Excel OAuth / BPS credentials were only usable through Excel BPS or the BPS plugin, which have been removed; this account is never scheduled for GPT traffic. Re-authorize it with the Codex OAuth flow or delete it."

var errRetiredBPSCredential = errors.New(RetiredBPSCredentialReason)

const retiredBPSCredentialErrorCode = "excel_oauth_route_unavailable"

func writeRetiredBPSCredentialError(c *gin.Context) error {
	if c != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "code": retiredBPSCredentialErrorCode,
			"message": errRetiredBPSCredential.Error(),
		}})
	}
	return errRetiredBPSCredential
}

// The Excel ID token's sid identifies a session, not a workspace. Manual OAuth
// refresh of an existing Excel-client account must use canonical access-token
// claims and must not probe native ChatGPT/Codex endpoints with this grant.
func applyExcelAccessIdentity(info *OpenAITokenInfo) error {
	invalid := infraerrors.BadRequest("OPENAI_EXCEL_TOKEN_INVALID", "Excel OAuth returned invalid token identity")
	if info == nil {
		return invalid
	}
	parts := strings.Split(info.AccessToken, ".")
	if len(parts) != 3 {
		return invalid
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return invalid
	}
	var client struct {
		ClientID string `json:"client_id"`
		AZP      string `json:"azp"`
	}
	if json.Unmarshal(payload, &client) != nil || firstNonEmptyReauth(client.ClientID, client.AZP) != openai.ExcelClientID {
		return invalid
	}
	at, err := openai.DecodeIDToken(info.AccessToken)
	if err != nil || at.Exp <= time.Now().Unix() || at.OpenAIAuth == nil || at.OpenAIAuth.ChatGPTAccountID == "" {
		return invalid
	}
	if info.IDToken != "" {
		id, idErr := openai.DecodeIDToken(info.IDToken)
		if idErr != nil {
			return invalid
		}
		audienceOK := false
		for _, aud := range id.Aud {
			audienceOK = audienceOK || aud == openai.ExcelClientID
		}
		if !audienceOK {
			return invalid
		}
		info.Email = id.Email
	}
	info.ChatGPTAccountID = at.OpenAIAuth.ChatGPTAccountID
	info.ChatGPTUserID = firstNonEmptyReauth(at.OpenAIAuth.ChatGPTUserID, at.OpenAIAuth.UserID, info.ChatGPTUserID)
	info.PlanType = firstNonEmptyReauth(at.OpenAIAuth.ChatGPTPlanType, info.PlanType)
	info.ExpiresAt = at.Exp
	return nil
}
