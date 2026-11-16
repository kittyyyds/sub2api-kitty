package admin

import (
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/gin-gonic/gin"
	"log/slog"
	"strconv"
	"strings"
	"time"
)

// GetOperationsDashboard returns role-aware operational revenue and usage data.
// GET /api/v1/admin/dashboard/operations
func (h *DashboardHandler) GetOperationsDashboard(c *gin.Context) {
	startTime, endTime, rangeErr := parseTimeRange(c)
	if rangeErr != nil || endTime.Sub(startTime) > 93*24*time.Hour {
		response.BadRequest(c, "Invalid date range (maximum 93 days)")
		return
	}
	userTimezone := strings.TrimSpace(c.Query("timezone"))
	if _, err := time.LoadLocation(userTimezone); userTimezone == "" || err != nil {
		userTimezone = timezone.Name()
		if userTimezone == "Local" {
			userTimezone = "UTC"
		}
	}
	now := timezone.NowInUserLocation(userTimezone)
	todayStart := timezone.StartOfDayInUserLocation(now, userTimezone)
	todayEnd := todayStart.AddDate(0, 0, 1)
	trendDays := 15
	if raw := strings.TrimSpace(c.Query("trend_days")); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || (parsed != 7 && parsed != 15) {
			response.BadRequest(c, "Invalid trend_days value, use 7 or 15")
			return
		}
		trendDays = parsed
	}
	// 0 = all groups, -1 = unassigned usage, -2 = usage of deleted groups.
	groupID := int64(0)
	if raw := strings.TrimSpace(c.Query("group_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed < -2 {
			response.BadRequest(c, "Invalid group_id")
			return
		}
		groupID = parsed
	}
	trendStart := todayStart.AddDate(0, 0, -(trendDays - 1))
	// Today cards and recharge ranking share the existing site calendar day.
	accountingTodayStart, accountingTodayEnd := operationsAccountingDay()

	data, err := h.dashboardService.GetOperationsDashboardData(c.Request.Context(), accountingTodayStart, accountingTodayEnd, startTime, endTime, trendStart, todayEnd, userTimezone, groupID)
	if err != nil {
		slog.Error("operations dashboard query failed", "error", err)
		response.Error(c, 500, "Failed to get operations dashboard data")
		return
	}
	data.TodayDate = accountingTodayStart.Format("2006-01-02")
	data.TodayStart = accountingTodayStart
	data.SiteTimezone = operationsSiteTimezone()
	response.Success(c, data)
}

// GetOperationsUsers returns the ranked user spend list and an optional user's
// group/model breakdown for the selected date range.
// GET /api/v1/admin/dashboard/operations/users
func (h *DashboardHandler) GetOperationsUsers(c *gin.Context) {
	startTime, endTime, rangeErr := parseTimeRange(c)
	if rangeErr != nil || endTime.Sub(startTime) > 93*24*time.Hour {
		response.BadRequest(c, "Invalid date range (maximum 93 days)")
		return
	}
	search := strings.TrimSpace(c.Query("search"))
	if len([]rune(search)) > 100 {
		response.BadRequest(c, "Search is too long")
		return
	}
	var userID int64
	if raw := strings.TrimSpace(c.Query("user_id")); raw != "" {
		parsed, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || parsed <= 0 {
			response.BadRequest(c, "Invalid user_id")
			return
		}
		userID = parsed
	}
	data, err := h.dashboardService.GetOperationsUsersData(c.Request.Context(), startTime, endTime, search, userID)
	if err != nil {
		response.Error(c, 500, "Failed to get operations user spending")
		return
	}
	response.Success(c, data)
}

// GetOperationsUserSuggestions returns matching ordinary-user emails for the
// operations search combobox.
// GET /api/v1/admin/dashboard/operations/users/suggest
func (h *DashboardHandler) GetOperationsUserSuggestions(c *gin.Context) {
	search := strings.TrimSpace(c.Query("q"))
	if len([]rune(search)) > 100 {
		response.BadRequest(c, "Search is too long")
		return
	}
	users, err := h.dashboardService.GetOperationsUserSuggestions(c.Request.Context(), search)
	if err != nil {
		response.Error(c, 500, "Failed to search operations users")
		return
	}
	response.Success(c, users)
}

func operationsAccountingDay() (time.Time, time.Time) {
	start := timezone.StartOfDay(timezone.Now())
	return start, start.AddDate(0, 0, 1)
}
func operationsSiteTimezone() string {
	if name := timezone.Name(); name != "Local" {
		return name
	}
	return "UTC" + timezone.UTCOffset()
}
func (h *DashboardHandler) GetOperationsRechargeRanking(c *gin.Context) {
	start, end := operationsAccountingDay()
	data, err := h.dashboardService.GetOperationsRechargeRanking(c.Request.Context(), start, end, 50)
	if err != nil {
		response.Error(c, 500, "Failed to get operations recharge ranking")
		return
	}
	data.Date = start.Format("2006-01-02")
	data.StartTime = start
	data.EndTime = end
	data.SiteTimezone = operationsSiteTimezone()
	response.Success(c, data)
}
