package service

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/openai"
	"github.com/gin-gonic/gin"
)

// Recognize retired Excel credentials only to prevent them from being sent
// through the standard Codex protocol.
func (a *Account) IsExcelOAuth() bool {
	return a != nil && a.IsOpenAIOAuthLike() && strings.TrimSpace(a.GetCredential("client_id")) == openai.ExcelClientID
}

var errExcelOAuthRouteUnavailable = errors.New("legacy Excel OAuth credentials are no longer supported; import a standard Codex OAuth account")

const excelOAuthRouteErrorCode = "excel_oauth_route_unavailable"

func (s *OpenAIGatewayService) validateExcelOAuthRoute(ctx context.Context, account *Account, model string) error {
	if account.IsExcelOAuth() {
		return errExcelOAuthRouteUnavailable
	}
	return nil
}

func writeExcelOAuthRouteError(c *gin.Context) error {
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
		"type": "invalid_request_error", "code": excelOAuthRouteErrorCode,
		"message": errExcelOAuthRouteUnavailable.Error(),
	}})
	return errExcelOAuthRouteUnavailable
}
