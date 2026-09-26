package service

import (
	"bytes"
	"testing"
)

var usageAuditPerformanceSink *UsageAudit

// BenchmarkBuildUsageAuditPerformance uses synthetic prose only. Request
// construction and tokenizer initialization are outside the measured interval.
// Use -benchtime=1x for Large25MiB so the original unbounded implementation is
// evaluated exactly once instead of repeatedly sizing an expensive benchmark.
func BenchmarkBuildUsageAuditPerformance(b *testing.B) {
	for _, tc := range []struct {
		name string
		size int
	}{
		{"Short1KiB", 1024},
		{"Within256KiB", 256*1024 - 1024},
		{"Large1MiB", 1024 * 1024},
		{"Large25MiB", 25 * 1024 * 1024},
	} {
		b.Run(tc.name, func(b *testing.B) {
			prefix := []byte(`{"model":"gpt-5.4","input":"`)
			suffix := []byte(`"}`)
			phrase := []byte("A bounded queue keeps independent work responsive. Each sample is synthetic text. ")
			payloadSize := tc.size - len(prefix) - len(suffix)
			body := make([]byte, 0, tc.size)
			body = append(body, prefix...)
			body = append(body, bytes.Repeat(phrase, payloadSize/len(phrase)+1)[:payloadSize]...)
			body = append(body, suffix...)
			input := UsageAuditBuildInput{
				Protocol:     UsageAuditProtocolOpenAIResponses,
				Model:        "gpt-5.4",
				RequestBody:  body,
				ResponseBody: []byte(`{"output":[{"type":"message","content":[{"type":"output_text","text":"A bounded queue limits retained work."}]}],"usage":{"input_tokens":9,"output_tokens":8,"input_tokens_details":{"cached_tokens":0}}}`),
				Upstream: UsageAuditTokenSnapshot{
					InputTokens:      usageAuditIntPtr(9),
					TotalInputTokens: usageAuditIntPtr(9),
					OutputTokens:     usageAuditIntPtr(8),
					CacheReadTokens:  usageAuditIntPtr(0),
				},
			}
			if _, err := openAIInputTokensCodecForModel(input.Model); err != nil {
				b.Fatal(err)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				usageAuditPerformanceSink = BuildUsageAudit(input)
			}
			b.StopTimer()
			b.ReportMetric(float64(len(body)), "request_bytes")
		})
	}
}
