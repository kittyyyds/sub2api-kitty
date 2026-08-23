//go:build unit

package service

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/xai"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestGrokOpenAICompatibleMediaModeIsAccountScoped(t *testing.T) {
	official := &Account{Platform: PlatformGrok}
	require.False(t, official.IsGrokOpenAICompatibleMedia())

	compatible := &Account{
		Platform: PlatformGrok,
		Extra:    map[string]any{GrokMediaCompatibilityExtraKey: GrokMediaCompatibilityOpenAI},
	}
	require.True(t, compatible.IsGrokOpenAICompatibleMedia())

	legacyCredential := &Account{
		Platform:    PlatformGrok,
		Credentials: map[string]any{GrokMediaCompatibilityExtraKey: "openai-compatible"},
	}
	require.True(t, legacyCredential.IsGrokOpenAICompatibleMedia())
	compatible.Platform = PlatformOpenAI
	require.False(t, compatible.IsGrokOpenAICompatibleMedia())
}

func TestNormalizeOpenAICompatibleGrokImagePayloadUsesUpstreamSizeContract(t *testing.T) {
	body, contentType, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesGenerations,
		[]byte(`{"model":"grok-imagine-image","prompt":"draw","n":2,"size":"1K","resolution":"1k","aspect_ratio":"16:9","quality":"high"}`),
		"application/json",
	)
	require.NoError(t, err)
	require.Equal(t, "application/json", contentType)
	require.Equal(t, "grok-imagine-image", gjson.GetBytes(body, "model").String())
	require.Equal(t, "draw", gjson.GetBytes(body, "prompt").String())
	require.Equal(t, int64(2), gjson.GetBytes(body, "n").Int())
	require.Equal(t, "1024x1024", gjson.GetBytes(body, "size").String())
	require.Equal(t, "1k", gjson.GetBytes(body, "resolution").String())
	require.Equal(t, "16:9", gjson.GetBytes(body, "aspect_ratio").String())
	require.Equal(t, "b64_json", gjson.GetBytes(body, "response_format").String())
	require.False(t, gjson.GetBytes(body, "quality").Exists())
}

func TestNormalizeOpenAICompatibleGrokImagePayloadAppliesEditRatioRules(t *testing.T) {
	single, _, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesEdits,
		[]byte(`{"model":"grok-imagine-image","prompt":"edit","resolution":"2k","aspect_ratio":"9:16","image":{"url":"https://cdn.example/one.png"}}`),
		"application/json",
	)
	require.NoError(t, err)
	require.Equal(t, "2048x2048", gjson.GetBytes(single, "size").String())
	require.Equal(t, "2k", gjson.GetBytes(single, "resolution").String())
	require.False(t, gjson.GetBytes(single, "aspect_ratio").Exists())

	multiple, _, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesEdits,
		[]byte(`{"model":"grok-imagine-image","prompt":"edit","resolution":"2k","aspect_ratio":"9:16","images":[{"url":"https://cdn.example/one.png"},{"url":"https://cdn.example/two.png"}]}`),
		"application/json",
	)
	require.NoError(t, err)
	require.Equal(t, "9:16", gjson.GetBytes(multiple, "aspect_ratio").String())
}

func TestNormalizeOpenAICompatibleGrokImagePayloadRejectsInvalidRatio(t *testing.T) {
	_, _, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesGenerations,
		[]byte(`{"model":"grok-imagine-image","prompt":"draw","aspect_ratio":"5:4"}`),
		"application/json",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "unsupported Grok image aspect ratio")
}

func TestNormalizeOpenAICompatibleGrokImagePayloadRejects4K(t *testing.T) {
	_, _, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesGenerations,
		[]byte(`{"model":"grok-imagine-image","prompt":"draw","resolution":"4k"}`),
		"application/json",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "only 1K or 2K")
}

func TestNormalizeOpenAICompatibleGrokImageMultipartBodyUsesIndexedImageFields(t *testing.T) {
	var input bytes.Buffer
	writer := multipart.NewWriter(&input)
	require.NoError(t, writer.WriteField("model", "grok-imagine-image"))
	require.NoError(t, writer.WriteField("prompt", "edit one"))
	require.NoError(t, writer.WriteField("size", "2048x2048"))
	require.NoError(t, writer.WriteField("aspect_ratio", "4:3"))
	for index, data := range [][]byte{{0x89, 0x50, 0x4e, 0x47}, {0xff, 0xd8, 0xff, 0xe0}} {
		header := textproto.MIMEHeader{}
		header.Set("Content-Disposition", `form-data; name="image"; filename="input`+string(rune('0'+index))+`.bin"`)
		header.Set("Content-Type", "image/png")
		part, err := writer.CreatePart(header)
		require.NoError(t, err)
		_, err = part.Write(data)
		require.NoError(t, err)
	}
	require.NoError(t, writer.Close())

	body, contentType, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesEdits,
		input.Bytes(),
		writer.FormDataContentType(),
	)
	require.NoError(t, err)
	require.Contains(t, contentType, "multipart/form-data")

	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	fields := map[string][]byte{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		fields[part.FormName()] = data
	}
	require.Equal(t, []byte{0x89, 0x50, 0x4e, 0x47}, fields["image[0]"])
	require.Equal(t, []byte{0xff, 0xd8, 0xff, 0xe0}, fields["image[1]"])
	require.Equal(t, []byte("grok-imagine-image"), fields["model"])
	require.Equal(t, []byte("edit one"), fields["prompt"])
	require.Equal(t, []byte("2048x2048"), fields["size"])
	require.Equal(t, []byte("2k"), fields["resolution"])
	require.Equal(t, []byte("4:3"), fields["aspect_ratio"])
	require.Equal(t, []byte("b64_json"), fields["response_format"])
}

func TestNormalizeOpenAICompatibleGrokImageMultipartBodyPreservesMaskFile(t *testing.T) {
	var input bytes.Buffer
	writer := multipart.NewWriter(&input)
	require.NoError(t, writer.WriteField("model", "grok-imagine-image"))
	require.NoError(t, writer.WriteField("prompt", "mask edit"))
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="image[0]"; filename="input.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write([]byte{1, 2, 3})
	require.NoError(t, err)
	header = textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="mask"; filename="mask.png"`)
	header.Set("Content-Type", "image/png")
	part, err = writer.CreatePart(header)
	require.NoError(t, err)
	_, err = part.Write([]byte{4, 5, 6})
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	body, contentType, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointImagesEdits,
		input.Bytes(),
		writer.FormDataContentType(),
	)
	require.NoError(t, err)
	mediaType, params, err := mime.ParseMediaType(contentType)
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(body), params["boundary"])
	seenMask := false
	seenImage := false
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		switch part.FormName() {
		case "image[0]":
			seenImage = true
			require.Equal(t, []byte{1, 2, 3}, data)
		case "mask":
			seenMask = true
			require.Equal(t, []byte{4, 5, 6}, data)
		}
	}
	require.True(t, seenImage)
	require.True(t, seenMask)
}

func TestForwardCompatibleGrokMediaImageEditPreservesMultipartContract(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	var input bytes.Buffer
	writer := multipart.NewWriter(&input)
	require.NoError(t, writer.WriteField("model", "grok-imagine-image"))
	require.NoError(t, writer.WriteField("prompt", "edit the product background"))
	require.NoError(t, writer.WriteField("size", "2K"))
	require.NoError(t, writer.WriteField("aspect_ratio", "16:9"))
	header := textproto.MIMEHeader{}
	header.Set("Content-Disposition", `form-data; name="image"; filename="product.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	require.NoError(t, err)
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a}
	_, err = part.Write(imageBytes)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/edits", bytes.NewReader(input.Bytes()))
	c.Request.Header.Set("Content-Type", writer.FormDataContentType())

	account := &Account{
		ID:          12287,
		Name:        "third-party-grok",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Extra:       map[string]any{GrokMediaCompatibilityExtraKey: GrokMediaCompatibilityOpenAI},
		Credentials: map[string]any{"api_key": "api-key", "base_url": "https://xai.test/v1"},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"ZWRpdGVk"}]}`)),
	}}
	service := &OpenAIGatewayService{httpUpstream: upstream}

	result, err := service.ForwardGrokMedia(
		context.Background(),
		c,
		account,
		GrokMediaEndpointImagesEdits,
		"",
		input.Bytes(),
		writer.FormDataContentType(),
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Contains(t, upstream.lastReq.Header.Get("Content-Type"), "multipart/form-data")

	mediaType, params, err := mime.ParseMediaType(upstream.lastReq.Header.Get("Content-Type"))
	require.NoError(t, err)
	require.Equal(t, "multipart/form-data", mediaType)
	reader := multipart.NewReader(bytes.NewReader(upstream.lastBody), params["boundary"])
	fields := map[string][]byte{}
	for {
		part, nextErr := reader.NextPart()
		if nextErr == io.EOF {
			break
		}
		require.NoError(t, nextErr)
		data, readErr := io.ReadAll(part)
		require.NoError(t, readErr)
		fields[part.FormName()] = data
	}
	require.Equal(t, imageBytes, fields["image[0]"])
	require.Equal(t, []byte("grok-imagine-image"), fields["model"])
	require.Equal(t, []byte("edit the product background"), fields["prompt"])
	require.Equal(t, []byte("2048x2048"), fields["size"])
	require.Equal(t, []byte("2k"), fields["resolution"])
	require.NotContains(t, fields, "aspect_ratio")
	require.Equal(t, []byte("b64_json"), fields["response_format"])
}

func TestForwardCompatibleGrokMediaPreservesExplicitMappedImageAlias(t *testing.T) {
	t.Setenv(xai.EnvAllowUnsafeURLOverrides, "true")
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	body := []byte(`{"model":"grok-imagine-image","prompt":"draw","size":"1024x1024","resolution":"1k","aspect_ratio":"1:1","background":"auto"}`)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	account := &Account{
		ID:          12351,
		Name:        "third-party-grok",
		Platform:    PlatformGrok,
		Type:        AccountTypeAPIKey,
		Concurrency: 1,
		Extra:       map[string]any{GrokMediaCompatibilityExtraKey: GrokMediaCompatibilityOpenAI},
		Credentials: map[string]any{
			"api_key":  "api-key",
			"base_url": "https://xai.test/v1",
			"model_mapping": map[string]any{
				"grok-imagine-image": "grok-imagine",
			},
		},
	}
	upstream := &httpUpstreamRecorder{resp: &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(`{"data":[{"b64_json":"aW1hZ2U="}]}`)),
	}}
	service := &OpenAIGatewayService{httpUpstream: upstream}

	result, err := service.ForwardGrokMedia(
		context.Background(),
		c,
		account,
		GrokMediaEndpointImagesGenerations,
		"",
		body,
		"application/json",
	)
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, "grok-imagine", gjson.GetBytes(upstream.lastBody, "model").String())
	require.Equal(t, "1024x1024", gjson.GetBytes(upstream.lastBody, "size").String())
	require.Equal(t, "1k", gjson.GetBytes(upstream.lastBody, "resolution").String())
	require.Equal(t, "b64_json", gjson.GetBytes(upstream.lastBody, "response_format").String())
	require.False(t, gjson.GetBytes(upstream.lastBody, "background").Exists())
	require.Equal(t, "grok-imagine-image", result.Model)
	require.Equal(t, "grok-imagine", result.UpstreamModel)
}

func TestNormalizeOpenAICompatibleGrokVideoPayloadValidatesRatioAndFiltersFields(t *testing.T) {
	body, _, err := normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointVideosGenerations,
		[]byte(`{"model":"grok-imagine-video","prompt":"waves","ratio":"3:2","duration":8,"resolution":"720p","unsupported":true}`),
		"application/json",
	)
	require.NoError(t, err)
	require.Equal(t, "3:2", gjson.GetBytes(body, "aspect_ratio").String())
	require.False(t, gjson.GetBytes(body, "ratio").Exists())
	require.False(t, gjson.GetBytes(body, "unsupported").Exists())

	_, _, err = normalizeOpenAICompatibleGrokMediaBody(
		GrokMediaEndpointVideosGenerations,
		[]byte(`{"model":"grok-imagine-video","prompt":"waves","aspect_ratio":"5:4"}`),
		"application/json",
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "16:9, 1:1, 9:16, 3:2, or 2:3")
}

func TestCompatibleGrok422PassthroughKeepsUpstreamBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/images/generations", nil)
	account := &Account{
		ID:       12286,
		Name:     "third-party-grok",
		Platform: PlatformGrok,
		Type:     AccountTypeAPIKey,
		Extra:    map[string]any{GrokMediaCompatibilityExtraKey: GrokMediaCompatibilityOpenAI},
	}
	body := `{"error":{"code":"invalid_size","message":"size must be 1024x1024 or 2048x2048"}}`
	service := &OpenAIGatewayService{}
	resp := &http.Response{
		StatusCode: http.StatusUnprocessableEntity,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}

	_, err := service.handleGrokMediaErrorResponse(context.Background(), resp, c, account, "upstream-request", "grok-imagine-image")
	require.Error(t, err)
	require.Equal(t, http.StatusUnprocessableEntity, recorder.Code)
	require.JSONEq(t, body, recorder.Body.String())
}
