package admin

import (
	"encoding/csv"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	"github.com/Wei-Shaw/sub2api/internal/pkg/usagestats"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
)

const (
	usageAuditExportPageSize = 500
	usageAuditExportMaxRows  = 100000
)

// AuditStats returns authenticity coverage, deviation metrics and dimensions.
// GET /api/v1/admin/usage/audit/stats
func (h *UsageHandler) AuditStats(c *gin.Context) {
	filters, ok := parseUsageAuditFilters(c)
	if !ok {
		return
	}
	stats, err := h.usageService.GetUsageAuditStats(c.Request.Context(), filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}
	response.Success(c, stats)
}

// AuditExport streams filtered audit rows without retaining the full export in memory.
// GET /api/v1/admin/usage/audit/export?format=csv|json
func (h *UsageHandler) AuditExport(c *gin.Context) {
	filters, ok := parseUsageAuditFilters(c)
	if !ok {
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "csv")))
	if format != "csv" && format != "json" {
		response.BadRequest(c, "Invalid format, use csv or json")
		return
	}
	filters.ExactTotal = true
	firstPage, page, err := h.usageService.ListWithFilters(c.Request.Context(), pagination.PaginationParams{
		Page:      1,
		PageSize:  usageAuditExportPageSize,
		SortBy:    "created_at",
		SortOrder: pagination.SortOrderDesc,
	}, filters)
	if err != nil {
		response.ErrorFrom(c, err)
		return
	}

	total := page.Total
	c.Header("X-Export-Total", strconv.FormatInt(total, 10))
	c.Header("X-Export-Limit", strconv.Itoa(usageAuditExportMaxRows))
	if total > usageAuditExportMaxRows {
		c.Header("X-Export-Truncated", "true")
	} else {
		c.Header("X-Export-Truncated", "false")
	}
	filename := "usage-audit-" + time.Now().Format("20060102-150405") + "." + format
	c.Header("Content-Disposition", `attachment; filename="`+filename+`"`)

	fetchPage := func(pageNumber int) ([]service.UsageLog, error) {
		if pageNumber == 1 {
			return firstPage, nil
		}
		logs, _, err := h.usageService.ListWithFilters(c.Request.Context(), pagination.PaginationParams{
			Page:      pageNumber,
			PageSize:  usageAuditExportPageSize,
			SortBy:    "created_at",
			SortOrder: pagination.SortOrderDesc,
		}, filters)
		return logs, err
	}

	if format == "json" {
		c.Header("Content-Type", "application/json; charset=utf-8")
		err = writeUsageAuditJSONExport(c, fetchPage)
	} else {
		c.Header("Content-Type", "text/csv; charset=utf-8")
		err = writeUsageAuditCSVExport(c, fetchPage)
	}
	if err != nil {
		if !c.Writer.Written() {
			response.ErrorFrom(c, err)
			return
		}
		_ = c.Error(err)
	}
}

type usageAuditExportRecord struct {
	ID                    int64               `json:"id"`
	CreatedAt             time.Time           `json:"created_at"`
	RequestID             string              `json:"request_id"`
	UserID                int64               `json:"user_id"`
	APIKeyID              int64               `json:"api_key_id"`
	AccountID             int64               `json:"account_id"`
	AccountName           string              `json:"account_name"`
	GroupID               *int64              `json:"group_id,omitempty"`
	GroupName             string              `json:"group_name,omitempty"`
	Platform              string              `json:"platform"`
	RequestedModel        string              `json:"requested_model"`
	UpstreamModel         string              `json:"upstream_model,omitempty"`
	UpstreamResponseModel string              `json:"upstream_response_model,omitempty"`
	InputTokens           int                 `json:"input_tokens"`
	OutputTokens          int                 `json:"output_tokens"`
	CacheCreationTokens   int                 `json:"cache_creation_tokens"`
	CacheReadTokens       int                 `json:"cache_read_tokens"`
	ActualCost            float64             `json:"actual_cost"`
	AccountCost           float64             `json:"account_cost"`
	Audit                 *service.UsageAudit `json:"usage_audit,omitempty"`
}

func writeUsageAuditJSONExport(c *gin.Context, fetchPage func(int) ([]service.UsageLog, error)) error {
	if _, err := c.Writer.WriteString("["); err != nil {
		return err
	}
	written := 0
	for pageNumber := 1; written < usageAuditExportMaxRows; pageNumber++ {
		logs, err := fetchPage(pageNumber)
		if err != nil {
			return err
		}
		if len(logs) == 0 {
			break
		}
		for i := range logs {
			if written >= usageAuditExportMaxRows {
				break
			}
			if written > 0 {
				if _, err := c.Writer.WriteString(","); err != nil {
					return err
				}
			}
			payload, err := json.Marshal(newUsageAuditExportRecord(&logs[i]))
			if err != nil {
				return err
			}
			if _, err = c.Writer.Write(payload); err != nil {
				return err
			}
			written++
		}
		if len(logs) < usageAuditExportPageSize {
			break
		}
	}
	_, err := c.Writer.WriteString("]")
	return err
}

func writeUsageAuditCSVExport(c *gin.Context, fetchPage func(int) ([]service.UsageLog, error)) error {
	writer := csv.NewWriter(c.Writer)
	if err := writer.Write([]string{
		"created_at", "id", "request_id", "user_id", "api_key_id", "platform", "account_id", "account_name", "group_id", "group_name",
		"requested_model", "upstream_model", "upstream_response_model", "status", "confidence", "evidence_source", "protocol", "estimator",
		"input_tokens", "output_tokens", "cache_creation_tokens", "cache_read_tokens", "local_input_tokens", "local_output_tokens",
		"input_delta", "output_delta", "input_deviation_pct", "output_deviation_pct", "cache_hit_rate", "actual_cost", "account_cost",
		"reasons", "raw_usage",
	}); err != nil {
		return err
	}
	written := 0
	for pageNumber := 1; written < usageAuditExportMaxRows; pageNumber++ {
		logs, err := fetchPage(pageNumber)
		if err != nil {
			return err
		}
		if len(logs) == 0 {
			break
		}
		for i := range logs {
			if written >= usageAuditExportMaxRows {
				break
			}
			if err := writer.Write(usageAuditCSVRow(newUsageAuditExportRecord(&logs[i]))); err != nil {
				return err
			}
			written++
		}
		writer.Flush()
		if err := writer.Error(); err != nil {
			return err
		}
		if len(logs) < usageAuditExportPageSize {
			break
		}
	}
	writer.Flush()
	return writer.Error()
}

func newUsageAuditExportRecord(log *service.UsageLog) usageAuditExportRecord {
	if log == nil {
		return usageAuditExportRecord{}
	}
	record := usageAuditExportRecord{
		ID:                  log.ID,
		CreatedAt:           log.CreatedAt,
		RequestID:           log.RequestID,
		UserID:              log.UserID,
		APIKeyID:            log.APIKeyID,
		AccountID:           log.AccountID,
		GroupID:             log.GroupID,
		RequestedModel:      usageAuditFirstNonEmpty(log.RequestedModel, log.Model),
		InputTokens:         log.InputTokens,
		OutputTokens:        log.OutputTokens,
		CacheCreationTokens: log.CacheCreationTokens,
		CacheReadTokens:     log.CacheReadTokens,
		ActualCost:          log.ActualCost,
		AccountCost:         log.TotalCost,
		Audit:               log.UsageAudit,
	}
	if log.UpstreamModel != nil {
		record.UpstreamModel = *log.UpstreamModel
	}
	if log.UpstreamResponseModel != nil {
		record.UpstreamResponseModel = *log.UpstreamResponseModel
	}
	if log.AccountRateMultiplier != nil {
		record.AccountCost *= *log.AccountRateMultiplier
	}
	if log.AccountStatsCost != nil {
		record.AccountCost = *log.AccountStatsCost
	}
	if log.Account != nil {
		record.AccountName = log.Account.Name
		record.Platform = log.Account.Platform
	}
	if log.Group != nil {
		record.GroupName = log.Group.Name
		if platform := strings.TrimSpace(log.Group.Platform); platform != "" && platform != service.PlatformComposite {
			record.Platform = platform
		}
	}
	return record
}

func usageAuditCSVRow(record usageAuditExportRecord) []string {
	audit := record.Audit
	status, confidence, evidenceSource, protocol, estimator, reasons, rawUsage := "", "", "", "", "", "", ""
	var localInput, localOutput, inputDelta, outputDelta, inputDeviation, outputDeviation, cacheHitRate string
	if audit != nil {
		status = string(audit.Status)
		confidence = string(audit.Confidence)
		evidenceSource = string(audit.EvidenceSource)
		protocol = audit.Protocol
		estimator = audit.Estimator
		reasons = strings.Join(audit.Reasons, "|")
		rawUsage = string(audit.UpstreamRawUsage)
		localInput = formatOptionalInt(audit.Local.TotalInputTokens)
		localOutput = formatOptionalInt(audit.Local.OutputTokens)
		inputDelta = formatOptionalInt(audit.Delta.InputTokens)
		outputDelta = formatOptionalInt(audit.Delta.OutputTokens)
		inputDeviation = formatOptionalFloat(audit.DeviationPct.InputTokens)
		outputDeviation = formatOptionalFloat(audit.DeviationPct.OutputTokens)
		cacheHitRate = formatOptionalFloat(audit.CacheHitRate)
	}
	return []string{
		record.CreatedAt.Format(time.RFC3339Nano), strconv.FormatInt(record.ID, 10), record.RequestID,
		strconv.FormatInt(record.UserID, 10), strconv.FormatInt(record.APIKeyID, 10), record.Platform,
		strconv.FormatInt(record.AccountID, 10), record.AccountName, formatOptionalInt64(record.GroupID), record.GroupName,
		record.RequestedModel, record.UpstreamModel, record.UpstreamResponseModel, status, confidence, evidenceSource, protocol, estimator,
		strconv.Itoa(record.InputTokens), strconv.Itoa(record.OutputTokens), strconv.Itoa(record.CacheCreationTokens), strconv.Itoa(record.CacheReadTokens),
		localInput, localOutput, inputDelta, outputDelta, inputDeviation, outputDeviation, cacheHitRate,
		strconv.FormatFloat(record.ActualCost, 'f', -1, 64), strconv.FormatFloat(record.AccountCost, 'f', -1, 64), reasons, rawUsage,
	}
}

func formatOptionalInt(value *int) string {
	if value == nil {
		return ""
	}
	return strconv.Itoa(*value)
}

func formatOptionalInt64(value *int64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatInt(*value, 10)
}

func formatOptionalFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', 2, 64)
}

func usageAuditFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func parseUsageAuditFilters(c *gin.Context) (usagestats.UsageLogFilters, bool) {
	var filters usagestats.UsageLogFilters
	parseID := func(name string) (int64, bool) {
		raw := strings.TrimSpace(c.Query(name))
		if raw == "" {
			return 0, true
		}
		value, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || value <= 0 {
			response.BadRequest(c, "Invalid "+name)
			return 0, false
		}
		return value, true
	}
	var ok bool
	if filters.UserID, ok = parseID("user_id"); !ok {
		return filters, false
	}
	if filters.APIKeyID, ok = parseID("api_key_id"); !ok {
		return filters, false
	}
	if filters.AccountID, ok = parseID("account_id"); !ok {
		return filters, false
	}
	if filters.GroupID, ok = parseID("group_id"); !ok {
		return filters, false
	}

	filters.RequestID = strings.TrimSpace(c.Query("request_id"))
	filters.Model = strings.TrimSpace(c.Query("model"))
	filters.ModelFilterSource = usagestats.ModelSourceRequested
	filters.BillingMode = strings.TrimSpace(c.Query("billing_mode"))
	if filters.UsageAuditStatus, ok = parseUsageAuditStatusQuery(c); !ok {
		return filters, false
	}

	if raw := strings.TrimSpace(c.Query("request_type")); raw != "" {
		requestType, err := service.ParseUsageRequestType(raw)
		if err != nil {
			response.BadRequest(c, err.Error())
			return filters, false
		}
		value := int16(requestType)
		filters.RequestType = &value
	} else if raw := strings.TrimSpace(c.Query("stream")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid stream value, use true or false")
			return filters, false
		}
		filters.Stream = &value
	}
	if raw := strings.TrimSpace(c.Query("billing_type")); raw != "" {
		value, err := strconv.ParseInt(raw, 10, 8)
		if err != nil {
			response.BadRequest(c, "Invalid billing_type")
			return filters, false
		}
		parsed := int8(value)
		filters.BillingType = &parsed
	}
	if raw := strings.TrimSpace(c.Query("upstream_model_mismatch")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			response.BadRequest(c, "Invalid upstream_model_mismatch value, use true or false")
			return filters, false
		}
		filters.UpstreamModelMismatch = &value
	}

	userTZ := c.Query("timezone")
	now := timezone.NowInUserLocation(userTZ)
	startDate := strings.TrimSpace(c.Query("start_date"))
	endDate := strings.TrimSpace(c.Query("end_date"))
	var startTime, endTime time.Time
	if startDate != "" || endDate != "" {
		if startDate == "" || endDate == "" {
			response.BadRequest(c, "start_date and end_date must be provided together")
			return filters, false
		}
		var err error
		startTime, err = timezone.ParseInUserLocation("2006-01-02", startDate, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid start_date format, use YYYY-MM-DD")
			return filters, false
		}
		endTime, err = timezone.ParseInUserLocation("2006-01-02", endDate, userTZ)
		if err != nil {
			response.BadRequest(c, "Invalid end_date format, use YYYY-MM-DD")
			return filters, false
		}
		endTime = endTime.AddDate(0, 0, 1)
	} else {
		switch c.DefaultQuery("period", "week") {
		case "today":
			startTime = timezone.StartOfDayInUserLocation(now, userTZ)
		case "month":
			startTime = now.AddDate(0, -1, 0)
		default:
			startTime = now.AddDate(0, 0, -7)
		}
		endTime = now
	}
	filters.StartTime = &startTime
	filters.EndTime = &endTime
	return filters, true
}

func parseUsageAuditStatusQuery(c *gin.Context) (string, bool) {
	status := strings.ToLower(strings.TrimSpace(c.Query("audit_status")))
	switch status {
	case "", "all":
		return "", true
	case string(service.UsageAuditStatusPass),
		string(service.UsageAuditStatusWarning),
		string(service.UsageAuditStatusAnomaly),
		string(service.UsageAuditStatusUnverifiable),
		string(service.UsageAuditStatusNotApplicable),
		service.UsageAuditFilterMissing:
		return status, true
	default:
		response.BadRequest(c, "Invalid audit_status")
		return "", false
	}
}
