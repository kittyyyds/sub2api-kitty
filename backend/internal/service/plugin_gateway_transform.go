package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const GatewayPromptMaxBody = 32 * 1024 * 1024

// ErrGatewayPromptInvalidRequest marks deterministic client input failures.
// Temporary plugin/runtime failures deliberately do not wrap this sentinel.
var ErrGatewayPromptInvalidRequest = errors.New("请求正文必须是有效的 JSON 对象")

var gatewayPromptSlots = make(chan struct{}, 64)

func (m *PluginManager) HasGatewayPrompt() bool {
	if m == nil {
		return false
	}
	route := m.transformRoute.Load()
	return route != nil && (route.runtime == nil || !route.runtime.promptNoop.Load())
}

func gatewayPromptConfigEmpty(raw []byte) bool {
	var cfg map[string]json.RawMessage
	if json.Unmarshal(raw, &cfg) != nil || cfg == nil {
		return false
	}
	for key, value := range cfg {
		switch key {
		case "rules":
			var rows []json.RawMessage
			if json.Unmarshal(value, &rows) != nil || len(rows) > 0 {
				return false
			}
		case "profiles":
			var rows map[string]json.RawMessage
			if json.Unmarshal(value, &rows) != nil || len(rows) > 0 {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// TransformGatewayRequest runs before model mapping and upstream retries. Only
// authenticated group metadata and the body are sent; credentials never are.
func (m *PluginManager) TransformGatewayRequest(ctx context.Context, groupID, keyID int64, protocol, model string, body []byte) ([]byte, error) {
	if m == nil || groupID <= 0 {
		return body, nil
	}
	route := m.transformRoute.Load()
	if route == nil || int(stablePluginBucket(keyID)) >= route.rolloutPercent {
		return body, nil
	}
	if route.runtime != nil && route.runtime.promptNoop.Load() {
		return body, nil
	}
	if len(body) > GatewayPromptMaxBody {
		return nil, &http.MaxBytesError{Limit: GatewayPromptMaxBody}
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return nil, ErrGatewayPromptInvalidRequest
	}
	field := ""
	switch protocol {
	case "anthropic":
		field = "system"
	case "responses":
		field = "instructions"
	case "chat_completions":
		field = "messages"
	default:
		return body, nil
	}
	if route.runtime == nil || route.runtime.client.Exited() {
		return nil, errors.New("分组提示词插件暂不可用")
	}
	callCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	select {
	case gatewayPromptSlots <- struct{}{}:
		defer func() { <-gatewayPromptSlots }()
	case <-callCtx.Done():
		return nil, errors.New("分组提示词插件繁忙，请稍后重试")
	}
	payload := body
	// A follow-up WS turn may inherit its model without carrying it in the frame.
	// Supply that name solely for rule matching, and remove it from the result.
	insertedModel := !gjson.GetBytes(body, "model").Exists() && model != ""
	var err error
	if insertedModel {
		payload, err = sjson.SetBytes(body, "model", model)
		if err != nil {
			return nil, err
		}
	}
	req, err := http.NewRequestWithContext(callCtx, http.MethodPost, "http://plugin.invalid/transform", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Sub2API-Plugin-Group-ID", strconv.FormatInt(groupID, 10))
	req.Header.Set("X-Sub2API-Plugin-Protocol", protocol)
	req.Header.Set("Content-Type", "application/json")
	runtime := route.runtime
	if !runtime.beginRequest() {
		return nil, errors.New("分组提示词插件正在切换")
	}
	resp, err := runtime.roundTrip(callCtx, req, "", &Account{ID: keyID, Platform: "gateway", Type: "any"})
	if err != nil {
		runtime.finishRequest()
		return nil, errors.New("分组提示词处理失败，请检查插件配置")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("分组提示词插件返回无效状态")
	}
	limit := int64(len(payload) + 256*1024)
	out, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil || int64(len(out)) > limit {
		return nil, errors.New("分组提示词插件响应无效或过大")
	}
	if err := validateGatewayPromptMutation(payload, out, field); err != nil {
		return nil, err
	}
	if insertedModel {
		out, err = sjson.DeleteBytes(out, "model")
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// A prompt plugin is not allowed to alter routing, tools, token limits, stream
// flags, or metadata. Preserve exact JSON number values during comparison.
func validateGatewayPromptMutation(before, after []byte, field string) error {
	decode := func(raw []byte) (map[string]any, error) {
		d := json.NewDecoder(bytes.NewReader(raw))
		d.UseNumber()
		token, err := d.Token()
		if err != nil || token != json.Delim('{') {
			return nil, errors.New("invalid object")
		}
		value := make(map[string]any)
		for d.More() {
			key, err := d.Token()
			if err != nil {
				return nil, err
			}
			name, ok := key.(string)
			if !ok {
				return nil, errors.New("invalid key")
			}
			if _, exists := value[name]; exists {
				return nil, errors.New("duplicate request field")
			}
			var item any
			if err := d.Decode(&item); err != nil {
				return nil, err
			}
			value[name] = item
		}
		if _, err := d.Token(); err != nil {
			return nil, err
		}
		var trailing any
		if d.Decode(&trailing) != io.EOF {
			return nil, errors.New("trailing JSON")
		}
		return value, nil
	}
	a, err := decode(before)
	if err != nil {
		return ErrGatewayPromptInvalidRequest
	}
	b, err := decode(after)
	if err != nil {
		return errors.New("提示词插件返回的 JSON 无效")
	}
	if !validPromptPrepend(a[field], b[field], field) {
		return errors.New("提示词插件未保留原始提示词或消息")
	}
	delete(a, field)
	delete(b, field)
	if !reflect.DeepEqual(a, b) {
		return errors.New("提示词插件修改了未授权的请求字段")
	}
	return nil
}

func validPromptPrepend(before, after any, field string) bool {
	if reflect.DeepEqual(before, after) {
		return true
	}
	if field == "messages" {
		a, aOK := before.([]any)
		b, bOK := after.([]any)
		if !aOK || !bOK || len(b) != len(a)+1 || !reflect.DeepEqual(a, b[1:]) {
			return false
		}
		message, ok := b[0].(map[string]any)
		_, textOK := message["content"].(string)
		return ok && len(message) == 2 && message["role"] == "system" && textOK
	}
	if a, ok := before.([]any); ok && field == "system" {
		b, ok := after.([]any)
		if !ok || len(b) != len(a)+1 || !reflect.DeepEqual(a, b[1:]) {
			return false
		}
		block, ok := b[0].(map[string]any)
		_, textOK := block["text"].(string)
		return ok && len(block) == 2 && block["type"] == "text" && textOK
	}
	b, ok := after.(string)
	if !ok {
		return false
	}
	if before == nil || before == "" {
		return true
	}
	a, ok := before.(string)
	return ok && strings.HasSuffix(b, "\n\n"+a)
}

// Used only by the authenticated administrator model picker.
func (s *GatewayService) GetPluginGroup(ctx context.Context, id int64) (*Group, error) {
	return s.groupRepo.GetByIDLite(ctx, id)
}
