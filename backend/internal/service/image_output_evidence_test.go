//go:build unit

package service

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"image"
	"image/png"
	"testing"
)

func TestImageOutputEvidenceMeasuredDiffersWithoutEditing(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 32))))
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	results := []openAIResponsesImageResult{{Result: encoded, Size: "1024x1024"}}
	reconcileOpenAIResponsesImageResultSizes(results, nil)
	require.Equal(t, "64x32", results[0].MeasuredSize)
	require.Equal(t, "1024x1024", results[0].DeclaredSize)
	require.Equal(t, encoded, results[0].Result)
	reconcileOpenAIResponsesImageResultSizes(results, nil)
	require.Equal(t, "1024x1024", results[0].DeclaredSize, "do not overwrite original upstream declaration")
	e := buildImageOutputEvidence("public", "upstream", "upstream", "1024x1024", []ImageOutputObservation{{DeclaredSize: results[0].DeclaredSize, MeasuredSize: results[0].MeasuredSize}}, true)
	require.Equal(t, "different", e.SizeStatus)
	require.Equal(t, "consistent", e.ModelStatus)
	require.Equal(t, "image_header", e.SizeEvidence)
	require.NotNil(t, e.Postprocessed)
	require.False(t, *e.Postprocessed)
}
func TestImageOutputEvidenceUnknownIsNeverInvented(t *testing.T) {
	e := buildImageOutputEvidence("image", "image", "", "auto", nil, false)
	require.Equal(t, "missing", e.ModelStatus)
	require.Equal(t, "unverified", e.SizeStatus)
	require.Nil(t, e.Postprocessed)
	e = buildImageOutputEvidence("image", "image", "different", "64x32", []ImageOutputObservation{{DeclaredSize: "64x32"}}, true)
	require.Equal(t, "different", e.ModelStatus)
	require.Equal(t, "unverified", e.SizeStatus, "declared dimensions are not measured evidence")
}
func TestImageOutputEvidenceRetainsMultipleStreamImagesWithoutDuplicates(t *testing.T) {
	c, _ := gin.CreateTestContext(nil)
	event := func(index int, width int) []byte {
		var buf bytes.Buffer
		require.NoError(t, png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, 32))))
		raw, err := json.Marshal(map[string]any{"type": "image_generation.completed", "output_index": index, "size": "1024x1024", "b64_json": base64.StdEncoding.EncodeToString(buf.Bytes())})
		require.NoError(t, err)
		return raw
	}
	first, second := event(0, 64), event(1, 128)
	observeRawImageOutput(c, first)
	observeRawImageOutput(c, second)
	observeRawImageOutput(c, first)
	outputs := imageOutputObservations(c)
	require.Len(t, outputs, 2)
	require.Equal(t, "64x32", outputs[0].MeasuredSize)
	require.Equal(t, "128x32", outputs[1].MeasuredSize)
	raw, err := json.Marshal(outputs)
	require.NoError(t, err)
	require.NotContains(t, string(raw), "b64_json")
}
