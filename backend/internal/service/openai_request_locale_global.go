package service

// Upstream global fallback applies only to accounts without an explicit Kitty policy.
// Adapted from MACOS-DO/sub4api, LGPL-3.0-or-later.
import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/pkg/logger"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
	"strconv"
	"strings"
)

func normalizeUpstreamOpenAIRequestLocale(ctx context.Context, account *Account, body []byte, transport string) []byte {
	if account == nil || !account.IsOpenAI() {
		return body
	}
	return normalizeUpstreamOpenAIRequestLocaleTimezone(ctx, account, body, transport, account.OpenAIRequestTimezone())
}

func normalizeUpstreamOpenAIRequestLocaleTimezone(ctx context.Context, account *Account, body []byte, transport, timezone string) []byte {
	if account == nil || !account.IsOpenAI() || !openAIRequestTimezoneAllowed[timezone] || !gjson.ValidBytes(body) {
		return body
	}
	before := make([]*string, 0)
	after := make([]*string, 0)
	matched := 0
	replaced := 0
	invalidEnvironment := false
	rewriteText := func(path string, text gjson.Result) {
		if text.Type != gjson.String {
			return
		}
		next, previous, reason := rewriteOpenAIRequestEnvironment(text.String(), timezone)
		if reason == "no_environment_context" {
			return
		}
		matched++
		var prior, current *string
		switch reason {
		case "invalid_environment_context":
			invalidEnvironment = true
		case "replaced", "already_target":
			loggedPrevious := previous
			if len(loggedPrevious) > 128 {
				loggedPrevious = loggedPrevious[:128]
			}
			prior, current = &loggedPrevious, &loggedPrevious
			if reason == "replaced" {
				if updated, err := sjson.SetBytes(body, path, next); err == nil {
					body = updated
					current = &timezone
					replaced++
				} else {
					invalidEnvironment = true
				}
			}
		}
		before = append(before, prior)
		after = append(after, current)
	}
	input := gjson.GetBytes(body, "input")
	if input.IsArray() {
		for inputIndex, item := range input.Array() {
			if item.Get("role").String() != "user" {
				continue
			}
			content := item.Get("content")
			path := "input." + strconv.Itoa(inputIndex) + ".content"
			if content.Type == gjson.String {
				rewriteText(path, content)
			} else if content.IsArray() {
				for contentIndex, part := range content.Array() {
					if part.Get("type").String() == "input_text" {
						rewriteText(path+"."+strconv.Itoa(contentIndex)+".text", part.Get("text"))
					}
				}
			}
		}
	}
	webSearchBefore := make([]*string, 0)
	webSearchAfter := make([]*string, 0)
	tools := gjson.GetBytes(body, "tools")
	if tools.IsArray() {
		for index, tool := range tools.Array() {
			kind := tool.Get("type").String()
			if kind != "web_search" && !strings.HasPrefix(kind, "web_search_") {
				continue
			}
			value := tool.Get("user_location.timezone")
			if value.Type == gjson.String {
				previous := value.String()
				loggedPrevious := previous
				if len(loggedPrevious) > 128 {
					loggedPrevious = loggedPrevious[:128]
				}
				current := loggedPrevious
				if previous != timezone {
					if updated, err := sjson.SetBytes(body, "tools."+strconv.Itoa(index)+".user_location.timezone", timezone); err == nil {
						body = updated
						current = timezone
						replaced++
					}
				}
				webSearchBefore = append(webSearchBefore, &loggedPrevious)
				webSearchAfter = append(webSearchAfter, &current)
			}
		}
	}
	reason := "no_environment_context"
	if replaced > 0 {
		reason = "replaced"
	} else if invalidEnvironment {
		reason = "invalid_environment_context"
	} else if matched > 0 {
		reason = "no_timezone"
		for _, value := range before {
			if value != nil {
				reason = "already_target"
				break
			}
		}
	}
	logger.FromContext(ctx).Debug("openai request timezone normalization",
		zap.Int64("account_id", account.ID), zap.String("transport", transport),
		zap.Bool("timezone_replaced", replaced > 0), zap.Any("timezone_before", before),
		zap.Any("timezone_after", after), zap.Any("web_search_timezone_before", webSearchBefore),
		zap.Any("web_search_timezone_after", webSearchAfter), zap.String("target_timezone", timezone),
		zap.Int("matched_count", matched), zap.Int("replaced_count", replaced), zap.String("reason", reason))
	return body
}
