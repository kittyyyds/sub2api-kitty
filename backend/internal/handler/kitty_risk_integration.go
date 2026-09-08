package handler

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/kittyrisk"
	"github.com/Wei-Shaw/sub2api/internal/securityaudit"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

func ProvideKittyRiskService(db *sql.DB, redisClient *redis.Client, settings service.SettingRepository,
	groups service.GroupRepository, users service.UserRepository, proxies service.ProxyRepository,
	invalidator service.APIKeyAuthCacheInvalidator, email *service.EmailService, keys service.APIKeyRepository,
	build BuildInfo, cfg *config.Config) (*kittyrisk.ContentModerationService, error) {
	return kittyrisk.NewManagedService(db, redisClient, settings, groups, users, proxies, invalidator, email, keys, build.Version+"@"+build.Commit, cfg.Pricing.DataDir)
}

// Only active while evidence/actions are explicitly enabled. The random local
// ID cannot collide with a client-supplied request ID. Cancellation ends both
// SSE and WebSocket upstream work through the existing request context.
func (h *GatewayHandler) KittyRiskLifecycle() gin.HandlerFunc {
	return func(c *gin.Context) {
		if h == nil || h.kittyRiskService == nil || !h.kittyRiskService.EvidenceEnabled(c.Request.Context()) {
			c.Next()
			return
		}
		key, ok := middleware.GetAPIKeyFromContext(c)
		if !ok || key == nil || key.UserID <= 0 {
			c.Next()
			return
		}
		ctx, cancel := context.WithCancel(c.Request.Context())
		registry := kittyrisk.DefaultGatewayKillSwitchRegistry()
		id := uuid.NewString()
		registry.Register(key.UserID, id, cancel)
		defer registry.Unregister(key.UserID, id)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

type kittyRiskCheckCache struct {
	stage string
	turn  int
	hash  [sha256.Size]byte
}

func runKittyRisk(c *gin.Context, svc *kittyrisk.ContentModerationService, key *service.APIKey,
	subject middleware.AuthSubject, protocol, model string, body []byte, stage string) *securityaudit.Decision {
	if c == nil || c.Request == nil || svc == nil || !svc.ReviewEnabled(c.Request.Context()) {
		return nil
	}
	turn, hasTurn := securityAuditWSTurn(c)
	cacheable := cachesSecurityAuditCompletion(stage) || (isSecurityAuditWebSocketStage(stage) && hasTurn)
	token := kittyRiskCheckCache{stage: stage, turn: turn, hash: sha256.Sum256(body)}
	if previous, ok := c.Get("kitty_risk.checked"); cacheable && ok && previous == token {
		return nil
	}
	reservation, reserved := svc.ReservePendingRequestBody(int64(len(body)))
	if !reserved {
		return &securityaudit.Decision{Kind: securityaudit.DecisionBlock, HTTPStatus: http.StatusServiceUnavailable,
			ErrorCode: "review_capacity_exceeded", ClientMessage: "Content review capacity temporarily exceeded", AllowNextStage: false}
	}
	defer reservation.Release()
	legacy := buildContentModerationInput(c, key, subject, protocol, model, body)
	input := kittyrisk.ContentModerationCheckInput{
		RequestID: legacy.RequestID, UserID: legacy.UserID, UserEmail: legacy.UserEmail,
		APIKeyID: legacy.APIKeyID, APIKeyName: legacy.APIKeyName, GroupID: legacy.GroupID, GroupName: legacy.GroupName,
		Endpoint: legacy.Endpoint, Provider: legacy.Provider, Model: legacy.Model, Protocol: protocol, Body: body,
		Reservation: reservation,
	}
	scope := kittyrisk.NewContentModerationScopeSnapshot(input.GroupID, input.GroupName)
	input.Scope = &scope
	if key != nil && key.User != nil {
		input.UserRole = key.User.Role
	}
	if svc.EvidenceEnabled(c.Request.Context()) {
		input.RawRequest = kittyRiskRawRequest(c, body, stage)
	}
	result, err := svc.Check(c.Request.Context(), input)
	if !svc.ReviewEnabled(c.Request.Context()) {
		return nil
	}
	if err != nil {
		// Disabling during an in-flight review must let the original gateway resume.
		if !svc.ReviewEnabled(c.Request.Context()) {
			return nil
		}
		return &securityaudit.Decision{Kind: securityaudit.DecisionBlock, HTTPStatus: http.StatusServiceUnavailable,
			ErrorCode: "review_unavailable", ClientMessage: "Content review temporarily unavailable", AllowNextStage: false}
	}
	if result != nil && result.Blocked {
		status := result.StatusCode
		if status < 400 || status > 599 {
			status = http.StatusForbidden
		}
		code := "content_policy_violation"
		if status >= 500 {
			code = "review_unavailable"
		}
		return &securityaudit.Decision{Kind: securityaudit.DecisionBlock, HTTPStatus: status,
			ErrorCode: code, ClientMessage: result.Message, AllowNextStage: false}
	}
	if cacheable {
		c.Set("kitty_risk.checked", token)
	}
	return nil
}

func kittyRiskRawRequest(c *gin.Context, body []byte, stage string) kittyrisk.ContentModerationRawRequest {
	transport := "http"
	if isSecurityAuditWebSocketStage(stage) {
		transport = "websocket"
	}
	// Authentication headers and query strings are never archived.
	headers := make(http.Header)
	for _, name := range []string{"Content-Type", "User-Agent", "OpenAI-Beta", "Anthropic-Version"} {
		if value := c.Request.Header.Get(name); value != "" {
			headers.Set(name, value)
		}
	}
	return kittyrisk.ContentModerationRawRequest{Method: c.Request.Method, Target: c.Request.URL.Path,
		Headers: headers, Body: body, Transport: transport, Stage: stage}
}

func (h *OpenAIGatewayHandler) recordKittyCyberPolicy(c *gin.Context, key *service.APIKey, model string, body []byte, mark *service.CyberPolicyMark) {
	svc := h.kittyRiskService
	if svc == nil || mark == nil || !svc.EvidenceEnabled(c.Request.Context()) {
		return
	}
	subject, _ := middleware.GetAuthSubjectFromContext(c)
	protocol := service.ContentModerationProtocolOpenAIResponses
	if strings.Contains(c.Request.URL.Path, "messages") {
		protocol = service.ContentModerationProtocolAnthropicMessages
	}
	legacy := buildContentModerationInput(c, key, subject, protocol, model, nil)
	scope := kittyrisk.NewContentModerationScopeSnapshot(legacy.GroupID, legacy.GroupName)
	stage := "http"
	if _, ok := securityAuditWSTurn(c); ok {
		stage = "subsequent_turn"
	}
	input := kittyrisk.CyberPolicyRecordInput{RequestID: legacy.RequestID, UserID: legacy.UserID, UserEmail: legacy.UserEmail,
		APIKeyID: legacy.APIKeyID, APIKeyName: legacy.APIKeyName, GroupID: legacy.GroupID, GroupName: legacy.GroupName,
		Endpoint: legacy.Endpoint, Model: model, Protocol: protocol, Scope: &scope,
		UpstreamMessage: mark.Message, UpstreamBody: mark.Body, UpstreamStatus: mark.UpstreamStatus,
		UpstreamInTok: mark.UpstreamInTok, UpstreamOutTok: mark.UpstreamOutTok,
		RawRequest: kittyRiskRawRequest(c, append([]byte(nil), body...), stage),
	}
	if key != nil && key.User != nil {
		input.UserRole = key.User.Role
		input.UserID = key.User.ID
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		svc.RecordCyberPolicyEvent(ctx, input)
	}()
}
