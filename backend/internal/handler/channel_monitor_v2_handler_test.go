package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type stubChannelMonitorGroupAuthorizer struct {
	groups []service.Group
	err    error
	calls  int
	userID int64
}

func (s *stubChannelMonitorGroupAuthorizer) GetAvailableGroups(_ context.Context, userID int64) ([]service.Group, error) {
	s.calls++
	s.userID = userID
	return s.groups, s.err
}

func TestChannelMonitorV2QueryListSupportsRepeatedAndCommaValues(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	c.Request = httptest.NewRequest("GET", "/?platform=openai,grok&platform=anthropic", nil)
	require.Equal(t, []string{"openai", "grok", "anthropic"}, queryList(c, "platform"))
}

func TestChannelMonitorV2GroupByQueryDefaultsAndRejectsInvalid(t *testing.T) {
	groupBy, err := service.ParseChannelMonitorV2GroupBy("")
	require.NoError(t, err)
	require.Equal(t, service.ChannelMonitorV2GroupByPlatformGroup, groupBy)
	_, err = service.ParseChannelMonitorV2GroupBy("invalid")
	require.Error(t, err)
}

func TestChannelMonitorV2MatrixHandlerRejectsInvalidGroupBy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/matrix?group_by=invalid", nil)
	h := NewChannelMonitorV2Handler(service.NewChannelMonitorV2Service(nil), nil)
	h.Matrix(c)
	require.Equal(t, http.StatusBadRequest, recorder.Code)
}

func TestChannelMonitorV2ScopeKeepsAdminGlobalWhenAuthorizationDependencyMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(string(middleware.ContextKeyUserRole), service.RoleAdmin)
	filter := service.ChannelMonitorV2Filter{GroupIDs: []int64{99}}
	authorizer := &stubChannelMonitorGroupAuthorizer{groups: []service.Group{{ID: 2}}}
	h := newChannelMonitorV2Handler(nil, authorizer)
	require.True(t, h.scopeFilter(c, &filter, true))
	require.Equal(t, []int64{99}, filter.GroupIDs)
	require.False(t, filter.RestrictGroups)
	require.Zero(t, authorizer.calls)
}

func TestChannelMonitorV2ScopeFailsClosedForOrdinaryViewerWithoutAuthorizationDependency(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Set(string(middleware.ContextKeyUserRole), "user")
	filter := service.ChannelMonitorV2Filter{}
	h := NewChannelMonitorV2Handler(nil, nil)
	require.False(t, h.scopeFilter(c, &filter, false))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.False(t, filter.RestrictGroups)
}

func TestChannelMonitorV2ScopeAppliesServerDerivedGroupsForOrdinaryViewer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/matrix", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 42})
	authorizer := &stubChannelMonitorGroupAuthorizer{groups: []service.Group{{ID: 2}, {ID: 4}}}
	filter := service.ChannelMonitorV2Filter{GroupIDs: []int64{4}}
	h := newChannelMonitorV2Handler(nil, authorizer)

	require.True(t, h.scopeFilter(c, &filter, false))
	require.True(t, filter.RestrictGroups)
	require.Equal(t, []int64{2, 4}, filter.AllowedGroupIDs)
	require.Equal(t, 1, authorizer.calls)
	require.Equal(t, int64(42), authorizer.userID)
}

func TestChannelMonitorV2ScopeAllowsExplicitlyEmptyOrdinaryViewerScope(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/matrix", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 43})
	authorizer := &stubChannelMonitorGroupAuthorizer{}
	filter := service.ChannelMonitorV2Filter{}
	h := newChannelMonitorV2Handler(nil, authorizer)

	require.True(t, h.scopeFilter(c, &filter, false))
	require.True(t, filter.RestrictGroups)
	require.Empty(t, filter.AllowedGroupIDs)
}

func TestChannelMonitorV2ScopeFailsClosedWithoutAuthenticatedSubject(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/matrix", nil)
	authorizer := &stubChannelMonitorGroupAuthorizer{groups: []service.Group{{ID: 2}}}
	filter := service.ChannelMonitorV2Filter{}
	h := newChannelMonitorV2Handler(nil, authorizer)

	require.False(t, h.scopeFilter(c, &filter, false))
	require.Equal(t, http.StatusUnauthorized, recorder.Code)
	require.Zero(t, authorizer.calls)
	require.False(t, filter.RestrictGroups)
}

func TestChannelMonitorV2ScopeFailsClosedWhenAuthorizationLookupFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodGet, "/channel-monitor-v2/matrix", nil)
	c.Set(string(middleware.ContextKeyUser), middleware.AuthSubject{UserID: 44})
	authorizer := &stubChannelMonitorGroupAuthorizer{err: errors.New("authorization unavailable")}
	filter := service.ChannelMonitorV2Filter{}
	h := newChannelMonitorV2Handler(nil, authorizer)

	require.False(t, h.scopeFilter(c, &filter, false))
	require.Equal(t, http.StatusInternalServerError, recorder.Code)
	require.Equal(t, 1, authorizer.calls)
	require.False(t, filter.RestrictGroups)
}
