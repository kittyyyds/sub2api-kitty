// Package kittyrisk contains the explicitly selected risk features from
// zuchengchen/sub2api czc-v2026.09.08.3, isolated from Kitty's existing audit.
package kittyrisk

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type SettingRepository = service.SettingRepository
type GroupRepository = service.GroupRepository
type UserRepository = service.UserRepository
type ProxyRepository = service.ProxyRepository
type APIKeyRepository = service.APIKeyRepository
type APIKeyAuthCacheInvalidator = service.APIKeyAuthCacheInvalidator
type EmailService = service.EmailService
type APIKey = service.APIKey
type User = service.User
type Group = service.Group
type UserUpdateFields = service.UserUpdateFields
type APIKeyUpdateFields = service.APIKeyUpdateFields
type APIKeyListFilters = service.APIKeyListFilters

var ErrUserNotFound = service.ErrUserNotFound
var ErrSettingNotFound = service.ErrSettingNotFound

const SettingKeyContentModerationConfig = "kitty_risk_config"
const SettingKeyRiskControlEnabled = "kitty_risk_enabled"
const SettingKeySiteName = service.SettingKeySiteName
const StatusDisabled = service.StatusDisabled
const StatusActive = service.StatusActive
const StatusAPIKeyDisabled = service.StatusAPIKeyDisabled
const RoleAdmin = service.RoleAdmin

func notificationEmailHash(value string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(strings.TrimSpace(value))))
	return hex.EncodeToString(hash[:])
}

func sanitizeEmailHeader(value string) string {
	return strings.NewReplacer("\r", "", "\n", "").Replace(value)
}

func paginationResultFromTotal(total int64, params pagination.PaginationParams) *pagination.PaginationResult {
	return &pagination.PaginationResult{Total: total, Page: params.Page, PageSize: params.Limit(), Pages: int((total + int64(params.Limit()) - 1) / int64(params.Limit()))}
}
