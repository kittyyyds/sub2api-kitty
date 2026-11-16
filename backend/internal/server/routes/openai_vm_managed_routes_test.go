package routes

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	adminhandler "github.com/Wei-Shaw/sub2api/internal/handler/admin"
	servermiddleware "github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// Every managed-account route sits behind admin auth and the audit log, and
// every one that changes the slot or its login also needs the second factor.
func TestOpenAIVMManagedRoutesRequireStepUpForEveryMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	handlers := &handler.Handlers{Admin: &handler.AdminHandlers{
		Account: &adminhandler.AccountHandler{}, OpenAIOAuth: &adminhandler.OpenAIOAuthHandler{},
		OpenAIVM: &adminhandler.OpenAIVMHandler{}, OpenAIVMManaged: &adminhandler.OpenAIVMManagedHandler{},
	}}
	chains := map[string][]string{}
	adminAuth := servermiddleware.AdminAuthMiddleware(func(c *gin.Context) {
		names := c.HandlerNames()
		chains[c.Request.Method+" "+c.FullPath()] = names[:len(names)-1]
		servermiddleware.AbortWithError(c, http.StatusUnauthorized, "UNAUTHORIZED", "Authorization required")
	})
	auditLog := servermiddleware.AuditLogMiddleware(func(c *gin.Context) { c.Next() })
	stepUp := servermiddleware.StepUpAuthMiddleware(func(c *gin.Context) { c.Next() })
	RegisterAdminRoutes(router.Group("/api/v1"), handlers, adminAuth, auditLog, stepUp, nil, nil, nil)

	const account = "/api/v1/admin/accounts/:id/vm-managed"
	want := map[string]bool{ // route -> needs the second factor
		"POST /api/v1/admin/openai-vm/managed-accounts":                 true,
		"POST /api/v1/admin/openai-vm/managed-batches/preview":          true,
		"POST /api/v1/admin/openai-vm/managed-batches":                  true,
		"GET /api/v1/admin/openai-vm/managed-batches/:batch_id":         false,
		"POST /api/v1/admin/openai-vm/managed-batches/:batch_id/cancel": true,
		"GET " + account:                     false,
		"DELETE " + account:                  true,
		"POST " + account + "/retry":         true,
		"PUT " + account + "/proxy":          true,
		"POST " + account + "/schedulable":   true,
		"POST " + account + "/usage/refresh": false,
		"GET " + account + "/op/*operation":  false,
		"POST " + account + "/op/*operation": true,
	}
	var registered []string
	for _, route := range router.Routes() {
		if strings.Contains(route.Path, "vm-managed") || strings.Contains(route.Path, "managed-accounts") || strings.Contains(route.Path, "managed-batches") {
			registered = append(registered, route.Method+" "+route.Path)
		}
	}
	sort.Strings(registered)
	var expected []string
	for route := range want {
		expected = append(expected, route)
	}
	sort.Strings(expected)
	require.Equal(t, expected, registered, "the managed route set changed; decide its protection here")

	for route, needsStepUp := range want {
		method, path, _ := strings.Cut(route, " ")
		target := strings.NewReplacer(":id", "7", ":batch_id", "3", "*operation", "login/password").Replace(path)
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, httptest.NewRequest(method, target, strings.NewReader("{}")))
		require.Equalf(t, http.StatusUnauthorized, recorder.Code, "%s answered without admin auth", route)
		chain := strings.Join(chains[route], "\n")
		require.NotEmptyf(t, chain, "%s did not pass through admin auth", route)
		// The three wrappers this test passes in are auth, audit and step-up; they
		// appear in the chain as this test's func1, func2 and func3.
		wrappers := strings.Count(chain, "TestOpenAIVMManagedRoutesRequireStepUpForEveryMutation.func")
		if needsStepUp {
			require.Equalf(t, 3, wrappers, "%s must be behind auth, audit and step-up", route)
		} else {
			require.Equalf(t, 2, wrappers, "%s should be behind auth and audit only", route)
		}
	}
}
