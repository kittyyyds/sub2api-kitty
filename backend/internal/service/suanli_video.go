package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
)

// SuanliSpecialVideoProvider is opt-in on a dedicated Grok API-key account.
// It never changes the contract of an existing Grok account.
const SuanliSpecialVideoProvider = "suanli_special"

func (a *Account) IsSuanliSpecialVideo() bool {
	return a != nil && (a.Platform == PlatformGrok || a.Platform == PlatformVideo) && a.Type == AccountTypeAPIKey &&
		strings.TrimSpace(a.GetCredential("grok_media_provider")) == SuanliSpecialVideoProvider
}

func (a *Account) VideoMediaProvider() string {
	if a.IsSuanliSpecialVideo() {
		return SuanliSpecialVideoProvider
	}
	return ""
}

type suanliVideoSpec struct {
	resolution    string
	fixedSeconds  int
	contractKnown bool
	seedance      bool
}

// These are exact supplier IDs, not aliases inferred from display names.
var suanliVideoModels = map[string]suanliVideoSpec{
	"官转稳定sd2.0(933原生过真人480P)":       {resolution: "480p", contractKnown: true, seedance: true},
	"官转稳定sd2.0(933原生过真人720P)":       {resolution: "720p", contractKnown: true, seedance: true},
	"官转稳定sd2.5(满血30-10-10不卡脸1080P)": {resolution: "1080p", contractKnown: true, seedance: true},
	"sd2.0(满血9-3-3不卡脸720P)":         {resolution: "720p", contractKnown: true, seedance: true},
	"sd2.5(满血30-10-10不卡脸720P)":      {resolution: "720p", contractKnown: true, seedance: true},
	"特价/sd-2.5":                     {resolution: "720p", fixedSeconds: 30, contractKnown: true},
	"特价/官方h3-2k":                    {resolution: "2k", contractKnown: true},
	"特价/官方h3-720p":                  {resolution: "720p", contractKnown: true},
	"特价/官方h3-1080p":                 {resolution: "1080p", contractKnown: true},
}

// normalizeSuanliVideoRequest validates before forwarding any paid request.
// Unverified contracts stay disabled instead of guessing supplier parameters.
func normalizeSuanliVideoRequest(endpoint GrokMediaEndpoint, body []byte, contentType string) ([]byte, string, error) {
	if endpoint.IsVideoLookupRequest() {
		return body, contentType, nil
	}
	if endpoint != GrokMediaEndpointVideosGenerations {
		return nil, "", newGrokMediaRequestValidationError("suanli_special only supports video generation and lookup")
	}
	if !strings.HasPrefix(strings.ToLower(contentType), "application/json") || !gjson.ValidBytes(body) {
		return nil, "", newGrokMediaRequestValidationError("suanli_special requires an application/json object")
	}
	payload, err := decodeGrokMediaJSONObject(body)
	if err != nil {
		return nil, "", newGrokMediaRequestValidationError("invalid video request object")
	}
	model, _ := payload["model"].(string)
	model = strings.TrimSpace(model)
	spec, ok := suanliVideoModels[model]
	if !ok {
		return nil, "", newGrokMediaRequestValidationError("unsupported suanli_special video model %q", model)
	}
	if !spec.contractKnown {
		return nil, "", newGrokMediaRequestValidationError("video model %q is not enabled: supplier parameter contract has not been verified", model)
	}
	prompt, _ := payload["prompt"].(string)
	if strings.TrimSpace(prompt) == "" {
		return nil, "", newGrokMediaRequestValidationError("prompt is required")
	}
	allowed := map[string]bool{"model": true, "prompt": true}
	out := map[string]any{"model": model, "prompt": prompt}
	if spec.fixedSeconds == 30 {
		for _, key := range []string{"resolution", "aspect_ratio", "images", "duration", "seconds"} {
			allowed[key] = true
		}
		if err := suanliCheckResolution(payload, spec.resolution); err != nil {
			return nil, "", err
		}
		for _, key := range []string{"duration", "seconds"} {
			if v, exists := payload[key]; exists {
				seconds, err := suanliPositiveInteger(v)
				if err != nil || seconds != 30 {
					return nil, "", newGrokMediaRequestValidationError("this model always generates 30 seconds")
				}
			}
		}
		images, err := suanliReferenceArray(payload["images"], 1, 10, false, false)
		if err != nil {
			return nil, "", err
		}
		out["images"] = images
		out["resolution"] = "720p"
		if ratio, exists := payload["aspect_ratio"]; exists {
			text, ok := ratio.(string)
			if !ok || !suanliValidRatio(text) {
				return nil, "", newGrokMediaRequestValidationError("aspect_ratio must contain two positive integers, e.g. 16:9")
			}
			out["aspect_ratio"] = text
		}
	} else {
		for _, key := range []string{"seconds", "duration", "size", "resolution", "reference_images", "reference_audios"} {
			allowed[key] = true
		}
		if err := suanliCheckResolution(payload, spec.resolution); err != nil {
			return nil, "", err
		}
		value, exists := payload["seconds"]
		if !exists {
			value = payload["duration"]
		}
		seconds, err := suanliPositiveInteger(value)
		if err != nil || seconds < 5 || seconds > 15 {
			return nil, "", newGrokMediaRequestValidationError("seconds must be an integer from 5 to 15")
		}
		if v, exists := payload["duration"]; exists {
			duration, err := suanliPositiveInteger(v)
			if err != nil || duration != seconds {
				return nil, "", newGrokMediaRequestValidationError("duration and seconds must agree")
			}
		}
		out["seconds"] = strconv.Itoa(seconds)
		size, _ := payload["size"].(string)
		if !suanliValidSize(size, spec.resolution) {
			return nil, "", newGrokMediaRequestValidationError("size must contain pixel dimensions matching %s", spec.resolution)
		}
		out["size"] = size
		maxImages := 9
		if spec.seedance {
			maxImages = 1
		}
		images, err := suanliReferenceArray(payload["reference_images"], 1, maxImages, true, false)
		if err != nil {
			return nil, "", err
		}
		out["reference_images"] = images
		if value, exists := payload["reference_audios"]; exists {
			if spec.seedance {
				return nil, "", newGrokMediaRequestValidationError("audio references are not enabled for the initial Seedance test")
			}
			audios, err := suanliReferenceArray(value, 0, 3, true, true)
			if err != nil {
				return nil, "", err
			}
			out["reference_audios"] = audios
		}
	}
	for key := range payload {
		if !allowed[key] {
			return nil, "", newGrokMediaRequestValidationError("unsupported parameter %q for model %q", key, model)
		}
	}
	encoded, err := json.Marshal(out)
	return encoded, "application/json", err
}

func suanliCheckResolution(payload map[string]any, expected string) error {
	if value, exists := payload["resolution"]; exists {
		resolution, ok := value.(string)
		if !ok || !strings.EqualFold(strings.TrimSpace(resolution), expected) {
			return newGrokMediaRequestValidationError("this model requires %s resolution", expected)
		}
	}
	return nil
}

func suanliPositiveInteger(value any) (int, error) {
	var text string
	switch v := value.(type) {
	case string:
		text = v
	case float64:
		if v <= 0 || v > 10000 || v != float64(int(v)) {
			return 0, fmt.Errorf("invalid positive integer")
		}
		return int(v), nil
	case json.Number:
		text = v.String()
	default:
		return 0, fmt.Errorf("integer required")
	}
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || n <= 0 || n > 10000 {
		return 0, fmt.Errorf("invalid positive integer")
	}
	return n, nil
}

func suanliValidRatio(value string) bool {
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return false
	}
	a, e1 := strconv.Atoi(parts[0])
	b, e2 := strconv.Atoi(parts[1])
	return e1 == nil && e2 == nil && a > 0 && b > 0 && a <= 100 && b <= 100
}

func suanliValidSize(value, resolution string) bool {
	parts := strings.Split(value, "x")
	if len(parts) != 2 {
		return false
	}
	w, e1 := strconv.Atoi(parts[0])
	h, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil || w <= 0 || h <= 0 || w > 8192 || h > 8192 {
		return false
	}
	edge := 720
	if resolution == "480p" {
		edge = 480
	} else if resolution == "1080p" {
		edge = 1080
	} else if resolution == "2k" {
		edge = 1440
	}
	return min(w, h) == edge
}

func suanliReferenceArray(value any, minimum, maximum int, objects, audio bool) ([]any, error) {
	items, ok := value.([]any)
	if !ok || len(items) < minimum || len(items) > maximum {
		return nil, newGrokMediaRequestValidationError("reference array requires %d to %d entries", minimum, maximum)
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		text, stringOK := item.(string)
		var object map[string]any
		if objects {
			object, ok = item.(map[string]any)
			if !ok {
				return nil, newGrokMediaRequestValidationError("reference entries must contain url")
			}
			text, stringOK = object["url"].(string)
		}
		if !stringOK || !(suanliPublicMediaURL(text, audio) || (!audio && suanliImageDataURL(text))) {
			return nil, newGrokMediaRequestValidationError("reference must be a public HTTP(S) URL or a valid media data URL")
		}
		if objects {
			normalized := map[string]any{"url": text}
			for key := range object {
				if key != "url" && (audio || key != "role") {
					return nil, newGrokMediaRequestValidationError("unsupported reference field %q", key)
				}
			}
			if !audio {
				role, _ := object["role"].(string)
				if role != "" && role != "first_frame" && role != "reference_image" {
					return nil, newGrokMediaRequestValidationError("reference image role must be first_frame or reference_image")
				}
				if role != "" {
					normalized["role"] = role
				}
			}
			out = append(out, normalized)
		} else {
			out = append(out, text)
		}
	}
	return out, nil
}

func suanliPublicMediaURL(value string, audio bool) bool {
	if audio && strings.HasPrefix(value, "data:audio/") {
		return suanliValidDataURL(value)
	}
	u, err := url.Parse(value)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()) {
		return false
	}
	return true
}

func suanliValidDataURL(value string) bool {
	parts := strings.SplitN(value, ";base64,", 2)
	if len(parts) != 2 || len(parts[1]) == 0 || len(parts[1]) > 28*1024*1024 {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(parts[1])
	return err == nil && len(decoded) > 0 && len(decoded) <= 20*1024*1024
}

func suanliImageDataURL(value string) bool {
	for _, media := range []string{"image/png", "image/jpeg", "image/webp"} {
		if strings.HasPrefix(value, "data:"+media+";base64,") {
			if !suanliValidDataURL(value) {
				return false
			}
			parts := strings.SplitN(value, ";base64,", 2)
			decoded, err := base64.StdEncoding.DecodeString(parts[1])
			return err == nil && http.DetectContentType(decoded) == media
		}
	}
	return false
}

func suanliRequestBillingInfo(body []byte, info *GrokMediaRequestInfo) {
	model := strings.TrimSpace(gjson.GetBytes(body, "model").String())
	if spec, ok := suanliVideoModels[model]; ok {
		info.Resolution = spec.resolution
		if spec.fixedSeconds > 0 {
			info.DurationSeconds = spec.fixedSeconds
		} else {
			info.DurationSeconds = int(gjson.GetBytes(body, "seconds").Int())
		}
	}
}

// Normalize to the existing asynchronous task contract. Only completion with
// an output URL is billable; queued progress values never imply completion.
func normalizeSuanliVideoResponse(endpoint GrokMediaEndpoint, body []byte, requestID string) ([]byte, error) {
	if !gjson.ValidBytes(body) {
		return nil, fmt.Errorf("suanli video returned invalid JSON")
	}
	id := ""
	for _, path := range []string{"id", "task_id", "data.id", "data.task_id"} {
		if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
			id = value
			break
		}
	}
	if endpoint.IsVideoLookupRequest() && strings.TrimSpace(requestID) != "" {
		// Ownership and dedup are keyed by the submitted gateway task ID.
		// Do not let an upstream internal ID change that identity on polling.
		id = strings.TrimSpace(requestID)
	}
	if id == "" {
		id = strings.TrimSpace(requestID)
	}
	if id == "" {
		return nil, fmt.Errorf("suanli video returned no task id; submission state is unknown, do not retry automatically")
	}
	status := strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "status").String()))
	if status == "" {
		status = strings.ToLower(strings.TrimSpace(gjson.GetBytes(body, "data.status").String()))
	}
	out := map[string]any{"request_id": id, "id": id}
	switch status {
	case "completed", "done", "succeeded", "success":
		out["status"] = "done"
	case "failed", "error", "cancelled", "canceled":
		out["status"] = "failed"
	case "expired":
		out["status"] = "expired"
	default:
		out["status"] = "pending"
	}
	for _, key := range []string{"model", "progress", "created_at", "error"} {
		if value := gjson.GetBytes(body, key); value.Exists() {
			out[key] = value.Value()
		}
	}
	if out["status"] == "done" {
		var videoURL string
		for _, path := range []string{"metadata.url", "video.url", "url", "result_url", "video_url", "data.metadata.url", "data.video.url", "data.url", "data.result_url", "data.video_url"} {
			if value := strings.TrimSpace(gjson.GetBytes(body, path).String()); value != "" {
				videoURL = value
				break
			}
		}
		if videoURL != "" {
			if !strings.HasPrefix(videoURL, "https://") || !suanliPublicMediaURL(videoURL, false) {
				return nil, fmt.Errorf("suanli returned invalid video URL")
			}
			video := map[string]any{"url": videoURL}
			for _, path := range []string{"video.duration", "duration", "seconds", "metadata.duration", "metadata.seconds", "data.duration", "data.seconds"} {
				if seconds, err := suanliPositiveInteger(gjson.GetBytes(body, path).Value()); err == nil && seconds <= 30 {
					video["duration"] = seconds
					break
				}
			}
			out["video"] = video
		}
	}
	if endpoint.IsGenerationRequest() {
		// Binding and the pricing snapshot are persisted after creation. Even
		// an instantly completed job must be polled before exposing its output.
		delete(out, "video")
		if out["status"] == "done" {
			out["status"] = "pending"
		}
	}
	return json.Marshal(out)
}

func NormalizeVideoBillingDurationForProvider(provider string, seconds int) int {
	if provider != SuanliSpecialVideoProvider {
		return NormalizeVideoBillingDurationSecondsOrDefault(seconds)
	}
	if seconds <= 0 {
		return 0
	}
	if seconds > 30 {
		return 30
	}
	return seconds
}

// Require explicit pricing before submitting paid supplier work. A supplier
// model must never inherit xAI's built-in fallback price.
func (s *OpenAIGatewayService) ValidateSuanliVideoPricing(ctx context.Context, apiKey *APIKey, model string) error {
	spec, ok := suanliVideoModels[strings.TrimSpace(model)]
	if !ok {
		return newGrokMediaRequestValidationError("unsupported suanli video model")
	}
	if apiKey != nil && apiKey.Group != nil && apiKey.Group.Platform == PlatformVideo {
		_, err := s.ResolveVideoGroupPricing(ctx, apiKey, model)
		return err
	}
	if apiKey == nil || apiKey.Group == nil {
		return newGrokMediaRequestValidationError("video group and explicit model pricing are required")
	}
	apiKey = s.apiKeyWithFreshGroupMediaPricing(ctx, apiKey)
	if _, ok := configuredVideoPriceModel(apiKey, spec.resolution, model); ok {
		return nil
	}
	resolved := s.resolveOpenAIChannelPricing(ctx, model, apiKey)
	if resolved != nil && (resolved.Source == PricingSourceGroup || resolved.Source == PricingSourceChannel) &&
		(resolved.Mode == BillingModeVideo || resolved.Mode == BillingModePerRequest) {
		return nil
	}
	return newGrokMediaRequestValidationError("configure explicit per-second or per-request video model pricing before submitting")
}
