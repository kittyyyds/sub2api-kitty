package service

import (
	"crypto/sha256"
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

// Only metadata is retained. Image bytes, URLs and prompts are never stored here.
type ImageOutputObservation struct {
	DeclaredSize string `json:"declared_size,omitempty"`
	MeasuredSize string `json:"measured_size,omitempty"`
}
type ImageOutputEvidence struct {
	Version        int                      `json:"version"`
	RequestedModel string                   `json:"requested_model,omitempty"`
	UpstreamModel  string                   `json:"upstream_model,omitempty"`
	ReportedModel  string                   `json:"reported_model,omitempty"`
	ModelStatus    string                   `json:"model_status"`
	RequestedSize  string                   `json:"requested_size,omitempty"`
	SizeStatus     string                   `json:"size_status"`
	SizeEvidence   string                   `json:"size_evidence"`
	Outputs        []ImageOutputObservation `json:"outputs,omitempty"`
	Postprocessed  *bool                    `json:"postprocessed,omitempty"`
}

const imageOutputObservationKey = "kitty_image_output_observations"
const imageOutputObservationSeenKey = "kitty_image_output_observation_seen"

func setImageOutputObservations(c *gin.Context, results []openAIResponsesImageResult) {
	if c == nil {
		return
	}
	out := make([]ImageOutputObservation, 0, len(results))
	for _, result := range results {
		if len(out) >= 64 {
			break
		}
		out = append(out, ImageOutputObservation{DeclaredSize: result.DeclaredSize, MeasuredSize: result.MeasuredSize})
	}
	c.Set(imageOutputObservationKey, out)
}
func observeRawImageOutput(c *gin.Context, body []byte) {
	if c == nil {
		return
	}
	root := gjson.ParseBytes(body)
	out := imageOutputObservations(c)
	value, _ := c.Get(imageOutputObservationSeenKey)
	seen, _ := value.(map[string]struct{})
	if seen == nil {
		seen = make(map[string]struct{})
		c.Set(imageOutputObservationSeenKey, seen)
	}
	add := func(item gjson.Result, position string) {
		if len(out) >= 64 {
			return
		}
		encoded := item.Get("b64_json").String()
		if encoded == "" {
			encoded = item.Get("result").String()
		}
		if encoded == "" && item.Get("url").String() == "" {
			return
		}
		// Retain only a bounded fingerprint in memory. Repeated final SSE
		// events must not replace earlier images or duplicate their evidence.
		fingerprint := sha256.Sum256([]byte(encoded + "\x00" + item.Get("url").String()))
		key := fmt.Sprintf("%s:%s:%x", position, item.Get("id").String(), fingerprint)
		if _, ok := seen[key]; ok {
			return
		}
		seen[key] = struct{}{}
		declared := item.Get("size").String()
		if declared == "" {
			declared = root.Get("size").String()
		}
		out = append(out, ImageOutputObservation{DeclaredSize: declared, MeasuredSize: detectOpenAIImageResultSize(encoded)})
	}
	if root.Get("data").IsArray() {
		for index, item := range root.Get("data").Array() {
			add(item, fmt.Sprint(index))
		}
	} else if strings.HasSuffix(root.Get("type").String(), ".completed") {
		add(root, root.Get("output_index").String())
	}
	c.Set(imageOutputObservationKey, out)
}
func imageOutputObservations(c *gin.Context) []ImageOutputObservation {
	if c == nil {
		return nil
	}
	v, ok := c.Get(imageOutputObservationKey)
	if !ok {
		return nil
	}
	out, _ := v.([]ImageOutputObservation)
	return out
}
func buildImageOutputEvidence(requested, upstream, reported, size string, outputs []ImageOutputObservation, knownUnprocessed bool) *ImageOutputEvidence {
	e := &ImageOutputEvidence{Version: 1, RequestedModel: requested, UpstreamModel: upstream, ReportedModel: reported, ModelStatus: "missing", RequestedSize: size, SizeStatus: "unverified", SizeEvidence: "unavailable", Outputs: outputs}
	if reported != "" {
		e.ModelStatus = "different"
		if strings.EqualFold(strings.TrimSpace(upstream), strings.TrimSpace(reported)) {
			e.ModelStatus = "consistent"
		}
	}
	if knownUnprocessed {
		value := false
		e.Postprocessed = &value
	}
	measured := 0
	different := false
	for _, o := range outputs {
		if o.MeasuredSize != "" {
			measured++
			if size != "" && size != "auto" && strings.Contains(size, "x") && !strings.EqualFold(size, o.MeasuredSize) {
				different = true
			}
		}
	}
	if measured > 0 {
		e.SizeEvidence = "image_header"
		switch {
		case size == "" || size == "auto":
			e.SizeStatus = "auto"
		case different:
			e.SizeStatus = "different"
		case measured == len(outputs) && strings.Contains(size, "x"):
			e.SizeStatus = "consistent"
		}
	}
	return e
}
func attachUsageImageOutputEvidence(log *UsageLog, outputs []ImageOutputObservation, knownUnprocessed bool) {
	if log == nil || log.ImageCount <= 0 {
		return
	}
	str := func(v *string) string {
		if v == nil {
			return ""
		}
		return strings.TrimSpace(*v)
	}
	requested := log.Model
	if log.RequestedModel != "" {
		requested = log.RequestedModel
	}
	if log.UsageAudit == nil {
		log.UsageAudit = NewNotApplicableUsageAudit("non_token_billing")
	}
	log.UsageAudit.ImageOutputEvidence = buildImageOutputEvidence(requested, str(log.UpstreamModel), str(log.UpstreamResponseModel), str(log.ImageInputSize), outputs, knownUnprocessed)
}
