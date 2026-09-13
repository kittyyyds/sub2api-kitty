package service

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/tidwall/gjson"
)

const candyTestResponseLimit = 2 << 20

func (s *CandyTestService) execute(ctx context.Context, c CandyTestConfig) CandyTestRun {
	run := CandyTestRun{Status: "error"}
	key, err := s.testKey(ctx, c)
	if err != nil {
		run.Message = "测试 Key 不可用，请检查分组、余额、到期时间和管理员归属"
		return run
	}
	group, err := s.groups.GetByID(ctx, c.GroupID)
	if err != nil || group == nil || group.Platform != PlatformOpenAI || group.Status != StatusActive {
		run.Message = "分组已停用或不再是 GPT 分组"
		return run
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint, bytes.NewReader(candyTestPayload(c)))
	if err != nil {
		run.Message = "无法创建测试请求"
		return run
	}
	req.Header.Set("Authorization", "Bearer "+key.Key)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Kitty-Candy-Test/1.0")
	// This is a normal authenticated customer API request: its owner's balance,
	// group policies, model mapping, scheduling and usage logs all apply.
	resp, err := s.client.Do(req)
	if err != nil {
		run.Message = "分组请求超时或连接中断；未自动重新测试"
		return run
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		run.Message = fmt.Sprintf("分组接口返回 HTTP %d；请在使用记录中检查 Key 余额、访问限制及上游状态", resp.StatusCode)
		return run
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, candyTestResponseLimit+1))
	if err != nil {
		run.Message = "响应接收中断，未取得完整答案"
		return run
	}
	if len(body) > candyTestResponseLimit {
		run.Status = "inconclusive"
		run.Message = "响应超过保存上限，不能判定答案"
		return run
	}
	run = parseCandyTestResponse(body, resp.Header.Get("Content-Type"), c.ExpectedAnswers)
	// Upstream content is untrusted; never return credentials even if echoed.
	for _, field := range []*string{&run.Output, &run.Answer, &run.ResponseModel, &run.ResponseID} {
		if key.Key != "" {
			*field = strings.ReplaceAll(*field, key.Key, "[已脱敏]")
		}
	}
	return run
}

func parseCandyTestResponse(body []byte, contentType string, expected []string) CandyTestRun {
	run := CandyTestRun{Status: "error"}
	var final []byte
	if strings.Contains(contentType, "text/event-stream") {
		scanner := bufio.NewScanner(bytes.NewReader(body))
		scanner.Buffer(make([]byte, 4096), candyTestResponseLimit)
		var data []string
		consume := func() {
			frame := strings.Join(data, "\n")
			data = nil
			if !gjson.Valid(frame) {
				return
			}
			kind := gjson.Get(frame, "type").String()
			if kind == "response.completed" || kind == "response.incomplete" || kind == "response.failed" {
				final = []byte(gjson.Get(frame, "response").Raw)
			}
		}
		for scanner.Scan() {
			line := strings.TrimSuffix(scanner.Text(), "\r")
			if line == "" {
				consume()
			} else if strings.HasPrefix(line, "data:") {
				data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
		consume()
		if scanner.Err() != nil {
			run.Message = "响应事件格式异常"
			return run
		}
	} else {
		final = body
	}
	if !json.Valid(final) {
		run.Message = "未收到可核对的完整响应"
		return run
	}
	run.ResponseModel = gjson.GetBytes(final, "model").String()
	run.ResponseID = gjson.GetBytes(final, "id").String()
	if len(run.ResponseModel) > 200 {
		run.ResponseModel = ""
	}
	if len(run.ResponseID) > 200 {
		run.ResponseID = ""
	}
	status := gjson.GetBytes(final, "status").String()
	var texts []string
	for _, item := range gjson.GetBytes(final, "output").Array() {
		if item.Get("type").String() != "message" {
			continue
		}
		for _, part := range item.Get("content").Array() {
			if part.Get("type").String() == "output_text" {
				texts = append(texts, part.Get("text").String())
			}
		}
	}
	run.Output = strings.Join(texts, "\n")
	for path, target := range map[string]**int{"usage.input_tokens": &run.InputTokens, "usage.output_tokens": &run.OutputTokens, "usage.input_tokens_details.cached_tokens": &run.CacheReadTokens} {
		v := gjson.GetBytes(final, path)
		if v.Type == gjson.Number && v.Int() >= 0 {
			n := int(v.Int())
			*target = &n
		}
	}
	if len(run.Output) > 64<<10 {
		cut := 64 << 10
		for cut > 0 && !utf8.ValidString(run.Output[:cut]) {
			cut--
		}
		run.Output = run.Output[:cut]
		run.Status = "inconclusive"
		run.Message = "回答超过文本保存上限，仅保留前 64 KiB，本轮不判对错"
		return run
	}
	if status == "incomplete" {
		run.Status = "inconclusive"
		run.Message = "输出未完成（可能达到 Token 上限），本轮不判对错"
		return run
	}
	if status != "completed" {
		run.Message = "接口未确认回复完整完成，本轮不判对错"
		return run
	}
	if strings.TrimSpace(run.Output) == "" {
		run.Status = "inconclusive"
		run.Message = "请求完成但没有文本答案，本轮不判对错"
		return run
	}
	run.Status, run.Answer = judgeCandyAnswer(run.Output, expected)
	if run.Status == "inconclusive" {
		run.Message = "没有明确的最终答案，或答案相互冲突，请查看原文"
	}
	return run
}
