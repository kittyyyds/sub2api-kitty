package admin

import (
	"errors"
	"github.com/Wei-Shaw/sub2api/internal/pkg/response"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/gin-gonic/gin"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

func (h *ChannelHandler) ListContextRules(c *gin.Context) {
	store := h.billingService.GlobalContextRules()
	if store == nil {
		response.Error(c, http.StatusGone, "此全局功能已移除，请到智能运维→自动配置设置分组模型计价")
		return
	}
	rules, err := store.List(c.Request.Context())
	if err != nil {
		response.Error(c, 503, "读取上下文规则失败")
		return
	}
	response.Success(c, rules)
}
func (h *ChannelHandler) SaveContextRule(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8192)
	var rule service.GlobalContextRule
	if err := c.ShouldBindJSON(&rule); err != nil {
		response.BadRequest(c, "规则格式不正确")
		return
	}
	if err := rule.Validate(); err != nil {
		response.BadRequest(c, err.Error())
		return
	}
	store := h.billingService.GlobalContextRules()
	if store == nil {
		response.Error(c, http.StatusGone, "此全局功能已移除，请到智能运维→自动配置设置分组模型计价")
		return
	}
	saved, err := store.Save(c.Request.Context(), rule)
	if errors.Is(err, service.ErrGlobalContextConflict) {
		response.Error(c, 409, err.Error())
		return
	}
	if err != nil {
		response.Error(c, 503, "保存规则失败，未确认生效")
		return
	}
	response.Success(c, saved)
}
func (h *ChannelHandler) DeleteContextRule(c *gin.Context) {
	id, e1 := strconv.ParseInt(c.Param("ruleID"), 10, 64)
	revision, e2 := strconv.ParseInt(c.Query("revision"), 10, 64)
	if e1 != nil || e2 != nil || id <= 0 || revision <= 0 {
		response.BadRequest(c, "规则 ID 或版本错误")
		return
	}
	store := h.billingService.GlobalContextRules()
	if store == nil {
		response.Error(c, http.StatusGone, "此全局功能已移除，请到智能运维→自动配置设置分组模型计价")
		return
	}
	err := store.Delete(c.Request.Context(), id, revision)
	if errors.Is(err, service.ErrGlobalContextConflict) {
		response.Error(c, 409, err.Error())
		return
	}
	if err != nil {
		response.Error(c, 503, "删除规则失败")
		return
	}
	response.Success(c, gin.H{"deleted": true})
}

func (h *ChannelHandler) ContextRuleModels(c *gin.Context) {
	platform := strings.ToLower(strings.TrimSpace(c.Query("platform")))
	provider, known := platformLiteLLMProvider(platform)
	if !known && platform != "openai-compatible" {
		response.BadRequest(c, "不支持的平台")
		return
	}
	store := h.billingService.GlobalContextRules()
	if store == nil {
		response.Error(c, http.StatusGone, "此全局功能已移除，请到智能运维→自动配置设置分组模型计价")
		return
	}
	names, err := store.ConfiguredModels(c.Request.Context(), platform)
	if err != nil {
		response.Error(c, 503, "读取已配置模型失败")
		return
	}
	if known && h.pricingService != nil {
		names = append(names, h.pricingService.ListModelNamesByProvider(provider)...)
	}
	seen := map[string]bool{}
	models := []string{}
	for _, name := range names {
		if name != "" && !seen[name] && !strings.ContainsAny(name, "*?") {
			seen[name] = true
			models = append(models, name)
		}
	}
	sort.Strings(models)
	response.Success(c, gin.H{"models": models})
}
