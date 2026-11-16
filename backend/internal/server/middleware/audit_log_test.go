package middleware

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestDeriveAuditAction(t *testing.T) {
	cases := []struct {
		method string
		path   string
		want   string
	}{
		{"PUT", "/api/v1/admin/accounts/:id", "admin.accounts.update"},
		{"POST", "/api/v1/admin/accounts", "admin.accounts.create"},
		{"DELETE", "/api/v1/admin/backups/:id", "admin.backups.delete"},
		{"GET", "/api/v1/admin/users/:id/api-keys", "admin.users.api_keys.read"},
		{"POST", "/api/v1/admin/redeem-codes/batch", "admin.redeem_codes.batch.create"},
	}
	for _, tc := range cases {
		if got := deriveAuditAction(tc.method, tc.path); got != tc.want {
			t.Fatalf("deriveAuditAction(%q, %q) = %q, want %q", tc.method, tc.path, got, tc.want)
		}
	}
}

func TestGPTDirectAuditNeverCapturesCDKSessionOrTaskToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &auditCaptureRepository{}
	audit := service.NewAuditLogService(repository, nil)
	audit.Start()
	router := gin.New()
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(audit)))
	for _, operation := range []string{"verify", "query", "redeem", "subscription", "task"} {
		path := "/api/v1/gpt-direct/" + operation
		router.POST(path, func(c *gin.Context) {
			var input map[string]string
			require.NoError(t, c.ShouldBindJSON(&input))
			require.Equal(t, "audit-canary-session", input["session"])
			c.Status(200)
		})
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"code":"audit-canary-cdk","session":"audit-canary-session","token":"audit-canary-token"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, 200, recorder.Code)
	}
	audit.Stop()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	require.Len(t, repository.logs, 5)
	for _, entry := range repository.logs {
		require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
	}
}

type auditCaptureRepository struct {
	mu   sync.Mutex
	logs []*service.AuditLog
}

func (r *auditCaptureRepository) BatchInsert(_ context.Context, logs []*service.AuditLog) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, logs...)
	return int64(len(logs)), nil
}
func (r *auditCaptureRepository) Insert(_ context.Context, log *service.AuditLog) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.logs = append(r.logs, log)
	return nil
}
func (r *auditCaptureRepository) List(context.Context, *service.AuditLogFilter) (*service.AuditLogList, error) {
	return &service.AuditLogList{}, nil
}
func (r *auditCaptureRepository) GetByID(context.Context, int64) (*service.AuditLog, error) {
	return nil, service.ErrAuditLogNotFound
}
func (r *auditCaptureRepository) Count(context.Context) (int64, error) { return 0, nil }
func (r *auditCaptureRepository) TruncateAll(context.Context) error    { return nil }
func (r *auditCaptureRepository) DeleteBefore(context.Context, time.Time, int) (int64, error) {
	return 0, nil
}

func TestPasskeyLoginAuditUsesCanonicalLoginActionAndOmitsCredentialBody(t *testing.T) {
	route := "POST /api/v1/auth/passkey/login/finish"
	require.Equal(t, service.AuditActionLogin, auditActionOverrides[route])
	require.Contains(t, auditBodyOmittedRoutes, route)
}

// Ollama 会话保存的请求体整体就是浏览器 Cookie 明文，键级脱敏清单曾漏掉裸键
// "session"，必须走整体不入库路径，防止会话凭证长期留存在 audit_logs。
func TestOllamaCloudUsageSessionRouteOmitsAuditBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	require.Contains(t, auditBodyOmittedRoutes, "PUT /api/v1/admin/accounts/:id/ollama-cloud-usage/session")

	repository := &auditCaptureRepository{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 77})
		c.Set(string(ContextKeyUserRole), "admin")
		c.Next()
	})
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(auditService)))
	router.PUT("/api/v1/admin/accounts/:id/ollama-cloud-usage/session", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	request := httptest.NewRequest(http.MethodPut, "/api/v1/admin/accounts/7/ollama-cloud-usage/session",
		bytes.NewBufferString(`{"session":"wos-session=audit-canary-cookie; __Secure-authjs.session-token.0=audit-canary-shard"}`))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	require.Equal(t, http.StatusOK, recorder.Code)
	auditService.Stop()

	repository.mu.Lock()
	logs := append([]*service.AuditLog(nil), repository.logs...)
	repository.mu.Unlock()
	require.Len(t, logs, 1)
	require.Equal(t, "<credential-bearing body omitted>", logs[0].RequestBody)
	require.NotContains(t, logs[0].RequestBody, "audit-canary")
}

// MCP OAuth 是机器凭证：只读工具同样可以批量读取邮箱等数据，必须全部入审计并带上 grant。
func TestAuditRecordsEveryMCPOAuthReadWithGrant(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &auditCaptureRepository{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()

	for _, authMethod := range []string{service.AuditAuthMethodMCPOAuth, ""} {
		router := gin.New()
		router.Use(func(c *gin.Context) {
			c.Set(string(ContextKeyUser), AuthSubject{UserID: 77})
			c.Set(string(ContextKeyUserRole), service.RoleAdmin)
			if authMethod != "" {
				c.Set("auth_method", authMethod)
				SetAuditExtra(c, map[string]any{"mcp_grant_id": "grant-1", "mcp_tool": "admin_list_users"})
			}
			c.Next()
		})
		router.Use(gin.HandlerFunc(NewAuditLogMiddleware(auditService)))
		router.GET("/api/v1/admin/users", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"items": []any{}}) })
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/admin/users?search=a", nil))
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	auditService.Stop()

	repository.mu.Lock()
	logs := append([]*service.AuditLog(nil), repository.logs...)
	repository.mu.Unlock()
	require.Len(t, logs, 1, "browser reads stay outside the audit log")
	require.Equal(t, service.AuditAuthMethodMCPOAuth, logs[0].AuthMethod)
	require.Equal(t, "admin.users.read", logs[0].Action)
	require.Equal(t, "grant-1", logs[0].Extra["mcp_grant_id"])
	require.Equal(t, "admin_list_users", logs[0].Extra["mcp_tool"])
}

// A VM2-managed login passes the account's password and 2FA secret through
// Sub2API. Its audit record must say that it happened, never what was typed.
func TestOpenAIVMManagedOperationRouteOmitsAuditBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const route = "/api/v1/admin/accounts/:id/vm-managed/op/*operation"
	require.Contains(t, auditBodyOmittedRoutes, "POST "+route)
	const batchRoute = "/api/v1/admin/openai-vm/managed-batches"
	require.Contains(t, auditBodyOmittedRoutes, "POST "+batchRoute)
	require.Contains(t, auditBodyOmittedRoutes, "POST "+batchRoute+"/preview")

	repository := &auditCaptureRepository{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()

	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 77})
		c.Set(string(ContextKeyUserRole), "admin")
		c.Next()
	})
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(auditService)))
	router.POST(route, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	router.POST(batchRoute, func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	router.POST(batchRoute+"/preview", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// Values a key-level redactor cannot recognise: an "email"/"code"/"raw"
	// field is not a sensitive key, and the password hides in free text.
	for _, tc := range []struct{ operation, body string }{
		{"login/password", `{"email":"canary-owner@example.test","password":"audit-canary-password","mfa_mode":"secret","mfa_secret":"AUDITCANARYSECRET"}`},
		{"login/password/code", `{"job_id":"j1","code":"audit-canary-otp"}`},
		{"login/oauth-code", `{"session_id":"s1","code":"audit-canary-oauth-code"}`},
		{"login/import", `{"raw":"{\"tokens\":{\"refresh\":\"audit-canary-refresh\"}}"}`},
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/admin/accounts/7/vm-managed/op/"+tc.operation, bytes.NewBufferString(tc.body))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	// A batch body is one login per line inside a single "text" value.
	batchBody := `{"text":"owner@example.test---audit-canary-batch-password---AUDITCANARYSECRET","proxy_ids":[1],"per_proxy":1}`
	for _, path := range []string{batchRoute, batchRoute + "/preview"} {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(batchBody))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	auditService.Stop()

	repository.mu.Lock()
	logs := append([]*service.AuditLog(nil), repository.logs...)
	repository.mu.Unlock()
	require.Len(t, logs, 6, "the operations themselves are still audited")
	for _, entry := range logs {
		require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
		require.NotContains(t, entry.RequestBody, "canary")
	}
}

func TestOpenAIPasswordLoginRouteOmitsAuditBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repository := &auditCaptureRepository{}
	auditService := service.NewAuditLogService(repository, nil)
	auditService.Start()
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set(string(ContextKeyUser), AuthSubject{UserID: 77})
		c.Set(string(ContextKeyUserRole), "admin")
		c.Next()
	})
	router.Use(gin.HandlerFunc(NewAuditLogMiddleware(auditService)))
	routes := []string{
		"/api/v1/admin/openai/password-login/tasks",
		"/api/v1/admin/openai/password-login/tasks/:jobID/code",
		"/api/v1/admin/openai/password-login/tasks/:jobID/exchange",
	}
	for _, route := range routes {
		router.POST(route, func(c *gin.Context) { c.JSON(200, gin.H{"ok": true}) })
		request := httptest.NewRequest(http.MethodPost, strings.ReplaceAll(route, ":jobID", "synthetic-job"), bytes.NewBufferString(`{"email":"audit-canary@example.test","password":"audit-canary-password","mfa_secret":"AUDITCANARYSECRET","code":"987654"}`))
		request.Header.Set("Content-Type", "application/json")
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusOK, recorder.Code)
	}
	auditService.Stop()
	repository.mu.Lock()
	defer repository.mu.Unlock()
	require.Len(t, repository.logs, len(routes))
	for _, entry := range repository.logs {
		require.Equal(t, "<credential-bearing body omitted>", entry.RequestBody)
		require.NotContains(t, entry.RequestBody, "canary")
		require.NotContains(t, entry.RequestBody, "987654")
	}
}
