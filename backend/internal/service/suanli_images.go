package service

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	_ "golang.org/x/image/webp"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

func configuredSuanliImage(c *gin.Context, model string) *ImageModelConfig {
	if c == nil {
		return nil
	}
	value, ok := c.Get("api_key")
	if !ok {
		return nil
	}
	key, ok := value.(*APIKey)
	if !ok {
		return nil
	}
	return ImageGroupModelConfig(key.Group, model)
}
func isSuanliImageAccount(a *Account) bool {
	if a == nil || a.Type != AccountTypeAPIKey || a.Platform != PlatformOpenAI {
		return false
	}
	u, err := url.Parse(a.GetOpenAIBaseURL())
	return err == nil && u.Scheme == "https" && strings.EqualFold(u.Hostname(), "suanliai.top") && (u.Port() == "" || u.Port() == "443") && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/" || u.Path == "/v1" || u.Path == "/v1/")
}
func suanliImageRequest(parsed *OpenAIImagesRequest, model string) (string, []byte, error) {
	if strings.TrimSpace(parsed.Prompt) == "" {
		return "", nil, errors.New("请输入图片提示词")
	}
	if parsed.N != 1 {
		return "", nil, errors.New("算力图片当前每次生成一张，请将 n 设为1")
	}
	if parsed.Stream {
		return "", nil, errors.New("算力图片请使用非流式或本站异步图片接口")
	}
	if parsed.HasMask {
		return "", nil, errors.New("当前上游图片文档未说明蒙版支持")
	}
	images := append([]string(nil), parsed.InputImageURLs...)
	if !parsed.Multipart && len(images) == 0 {
		for _, item := range gjson.GetBytes(parsed.Body, "image").Array() {
			images = append(images, item.String())
		}
	}
	for _, u := range parsed.Uploads {
		images = append(images, u.ImageDataURL())
	}
	for _, ref := range images {
		if !strings.HasPrefix(ref, "data:image/") {
			u, e := url.Parse(ref)
			if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
				return "", nil, errors.New("参考图请使用图片文件、data URI或HTTPS图片地址")
			}
		}
	}
	endpoint := openAIImagesGenerationsEndpoint
	payload := map[string]any{"model": model, "prompt": parsed.Prompt, "n": 1, "response_format": "b64_json"}
	if parsed.Size != "" {
		payload["size"] = parsed.Size
	}
	if parsed.Quality != "" {
		payload["quality"] = parsed.Quality
	}
	if len(images) > 0 {
		payload["image"] = images
	}
	// These provider families explicitly use the Chat compatibility protocol.
	// The original, case-sensitive display ID is kept intact.
	if strings.Contains(strings.ToLower(model), "banana") {
		endpoint = "/v1/chat/completions"
		content := []map[string]any{{"type": "text", "text": parsed.Prompt}}
		for _, ref := range images {
			content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": ref}})
		}
		delete(payload, "image")
		delete(payload, "prompt")
		delete(payload, "response_format")
		payload["messages"] = []map[string]any{{"role": "user", "content": content}}
		payload["stream"] = false
	}
	raw, err := json.Marshal(payload)
	return endpoint, raw, err
}

type suanliImageItem struct {
	URL    string `json:"url,omitempty"`
	Base64 string `json:"b64_json,omitempty"`
}

var suanliMarkdownImage = regexp.MustCompile(`!\[[^\]]*\]\(([^\s)]+)\)`)
var errSuanliImageFailed = errors.New("上游图片任务失败")

func validateSuanliImageBytes(raw []byte) error {
	conf, _, err := image.DecodeConfig(bytes.NewReader(raw))
	if err != nil || conf.Width <= 0 || conf.Height <= 0 || conf.Width > 32768 || conf.Height > 32768 {
		return errors.New("上游返回的内容不是有效图片")
	}
	return nil
}

func suanliImageResult(body []byte, contentType string) ([]byte, string, error) {
	var images []suanliImageItem
	add := func(value string, b64 bool) error {
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if b64 {
			raw, e := base64.StdEncoding.DecodeString(value)
			if e != nil || validateSuanliImageBytes(raw) != nil {
				return errors.New("图片编码无效")
			}
			images = append(images, suanliImageItem{Base64: value})
			return nil
		}
		if strings.HasPrefix(value, "data:image/") {
			i := strings.Index(value, ";base64,")
			if i < 0 {
				return errors.New("图片编码无效")
			}
			data := value[i+8:]
			raw, e := base64.StdEncoding.DecodeString(data)
			if e != nil || validateSuanliImageBytes(raw) != nil {
				return errors.New("图片编码无效")
			}
			images = append(images, suanliImageItem{Base64: data})
			return nil
		}
		u, e := url.Parse(value)
		if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
			return errors.New("上游图片地址无效")
		}
		images = append(images, suanliImageItem{URL: value})
		return nil
	}
	if strings.HasPrefix(strings.ToLower(contentType), "image/") {
		if err := validateSuanliImageBytes(body); err != nil {
			return nil, "", err
		}
		images = append(images, suanliImageItem{Base64: base64.StdEncoding.EncodeToString(body)})
	} else {
		if !gjson.ValidBytes(body) {
			return nil, "", errors.New("上游图片响应不是有效JSON")
		}
		if gjson.GetBytes(body, "error").Exists() {
			return nil, "", errSuanliImageFailed
		}
		if code := gjson.GetBytes(body, "code"); code.Exists() && code.Int() != 0 {
			return nil, "", errSuanliImageFailed
		}
		status := strings.ToLower(gjson.GetBytes(body, "data.status").String())
		if status == "failed" || status == "error" || status == "cancelled" {
			return nil, "", errSuanliImageFailed
		}
		if status == "running" || status == "pending" || status == "processing" {
			id := gjson.GetBytes(body, "data.id").String()
			if id == "" {
				id = "pending"
			}
			return nil, id, nil
		}
		data := gjson.GetBytes(body, "data")
		items := data.Array()
		if !data.IsArray() {
			items = gjson.GetBytes(body, "data.results").Array()
		}
		for _, item := range items {
			if v := item.Get("b64_json").String(); v != "" {
				if e := add(v, true); e != nil {
					return nil, "", e
				}
			} else if v := item.Get("url").String(); v != "" {
				if e := add(v, false); e != nil {
					return nil, "", e
				}
			}
		}
		if len(images) == 0 {
			content := strings.TrimSpace(gjson.GetBytes(body, "choices.0.message.content").String())
			matches := suanliMarkdownImage.FindAllStringSubmatch(content, -1)
			for _, m := range matches {
				if e := add(m[1], false); e != nil {
					return nil, "", e
				}
			}
			if len(matches) == 0 && (strings.HasPrefix(content, "https://") || strings.HasPrefix(content, "data:image/")) {
				if e := add(content, false); e != nil {
					return nil, "", e
				}
			}
		}
		if len(images) == 0 {
			if id := gjson.GetBytes(body, "data.id").String(); id != "" && len(id) <= 256 && !strings.ContainsAny(id, "\r\n") {
				return nil, id, nil
			}
			if status == "running" || status == "pending" || status == "processing" {
				return nil, "pending", nil
			}
			return nil, "", errors.New("上游未返回图片或可查询任务，未按成功扣费")
		}
	}
	if len(images) != 1 {
		return nil, "", errors.New("上游返回图片数量与每次一张的约定不符，待人工核实")
	}
	raw, err := json.Marshal(map[string]any{"created": time.Now().Unix(), "data": images})
	return raw, "", err
}

// All paid submissions have a durable record before contacting the supplier.
func (s *OpenAIGatewayService) forwardSuanliImages(ctx context.Context, c *gin.Context, a *Account, parsed *OpenAIImagesRequest, requestModel, model string) (*OpenAIForwardResult, error) {
	if !isSuanliImageAccount(a) {
		return nil, errors.New("算力图片配置与绑定账号不匹配")
	}
	value, _ := c.Get("api_key")
	key, ok := value.(*APIKey)
	if !ok || key.Group == nil {
		return nil, errors.New("缺少图片分组")
	}
	price := LookupImageModelPrice(key.Group.ImageModelPrices, requestModel, parsed.SizeTier)
	if price == nil || !validVideoMoney(*price) {
		return nil, errors.New("图片型号尚未配置有效售价")
	}
	endpoint, body, err := suanliImageRequest(parsed, model)
	if err != nil {
		return nil, err
	}
	ct := "application/json"
	if parsed.Multipart && !strings.Contains(strings.ToLower(model), "banana") {
		endpoint = parsed.Endpoint
		body, ct, err = rewriteOpenAIImagesModel(parsed.Body, parsed.ContentType, model)
		if err != nil {
			return nil, err
		}
	}
	upstreamCtx, release := detachUpstreamContext(ctx)
	defer release()
	upstreamCtx, cancel := context.WithTimeout(upstreamCtx, 5*time.Minute)
	defer cancel()
	task, created, err := s.beginSuanliImageTask(upstreamCtx, key, a, parsed, model, c.GetHeader("Idempotency-Key"), *price)
	if err != nil {
		return nil, err
	}
	c.Header("X-Image-Task-ID", task.ID)
	if !created {
		return suanliTaskResult(task), nil
	}
	repo := s.suanliImageTaskRepository()
	save := func(status string) error {
		persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 8*time.Second)
		defer stop()
		return s.finishSuanliImageTask(persist, task, status)
	}
	raw, contentType, err := s.sendSuanliImageRequest(upstreamCtx, a, endpoint, body, ct)
	if err != nil {
		_ = save("unknown")
		return suanliTaskResult(task), nil
	}
	normalized, remote, err := suanliImageResult(raw, contentType)
	if err != nil {
		if errors.Is(err, errSuanliImageFailed) {
			_ = save("failed")
		} else {
			_ = save("unknown")
		}
		return suanliTaskResult(task), nil
	}
	if remote != "" {
		if remote == "pending" {
			_ = save("unknown")
			return suanliTaskResult(task), nil
		}
		task.UpstreamTaskID = remote
		task.Status = "pending"
		// A foreground request owns a six-minute polling lease. A crash expires
		// this lease and the worker resumes read-only polling, never submission.
		if err = repo.UpdateSuanliImageTask(upstreamCtx, task, 6*time.Minute); err != nil {
			return nil, errors.New("图片任务已提交但保存失败，请勿重新下单")
		}
		if strings.Contains(strings.ToLower(c.GetHeader("Prefer")), "respond-async") {
			if err = repo.UpdateSuanliImageTask(upstreamCtx, task, 0); err != nil {
				return nil, err
			}
			return suanliTaskResult(task), nil
		}
		for delay := 2 * time.Second; ; {
			timer := time.NewTimer(delay)
			select {
			case <-upstreamCtx.Done():
				timer.Stop()
				persist, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
				_ = repo.UpdateSuanliImageTask(persist, task, 0)
				stop()
				return suanliTaskResult(task), nil
			case <-timer.C:
			}
			if delay < 10*time.Second {
				delay += time.Second
			}
			query, _ := json.Marshal(map[string]string{"id": remote})
			raw, contentType, err = s.sendSuanliImageRequest(upstreamCtx, a, "/v1/images/result", query, "application/json")
			if err != nil {
				continue
			}
			var next string
			normalized, next, err = suanliImageResult(raw, contentType)
			if err != nil {
				if errors.Is(err, errSuanliImageFailed) {
					_ = save("failed")
					return suanliTaskResult(task), nil
				}
				continue
			}
			if next == "" {
				break
			}
		}
	}
	task.Result = normalized
	if err = save("ready"); err != nil {
		return nil, errors.New("图片结果保存失败，请查询任务状态，勿重新下单")
	}
	return suanliTaskResult(task), nil
}
func (s *OpenAIGatewayService) sendSuanliImageRequest(ctx context.Context, a *Account, path string, payload []byte, ct string) ([]byte, string, error) {
	ctx = WithHTTPUpstreamRedirectsDisabled(ctx)
	if !isSuanliImageAccount(a) {
		return nil, "", errors.New("图片账号地址无效")
	}
	token, _, err := s.GetAccessToken(ctx, a)
	if err != nil {
		return nil, "", errors.New("图片账号授权不可用")
	}
	proxy := ""
	if a.ProxyID != nil && a.Proxy != nil {
		proxy = a.Proxy.URL()
	}
	target := strings.TrimSuffix(strings.TrimSuffix(a.GetOpenAIBaseURL(), "/"), "/v1") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(payload))
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", ct)
	resp, err := s.doOpenAIUpstream(req, proxy, a)
	if err != nil {
		return nil, "", errors.New("图片上游连接未完成")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("图片上游返回HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, (32<<20)+1))
	if err != nil || len(data) > 32<<20 {
		return nil, "", errors.New("图片响应读取失败或超过32MB")
	}
	return data, resp.Header.Get("Content-Type"), nil
}
