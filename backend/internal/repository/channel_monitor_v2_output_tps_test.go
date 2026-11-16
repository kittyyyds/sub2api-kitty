//go:build unit

package repository

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestChannelMonitorV2OutputTPSIsGenerationWeightedNotFleetThroughput(t *testing.T) {
	a := newMetricAccumulator()
	a.addFact(channelMonitorV2Fact{Success: 1, Output: 100, TPSOutputTokens: 100, TPSGenerationMS: 1000, TPSSamples: 1})
	a.addFact(channelMonitorV2Fact{Success: 1, Output: 400, TPSOutputTokens: 400, TPSGenerationMS: 10000, TPSSamples: 1})
	// Existing history may contain tokens but has no measured generation interval.
	a.addFact(channelMonitorV2Fact{Success: 100, Output: 900000})
	m := a.metric(90, true)
	require.NotNil(t, m.OutputTPS)
	require.InDelta(t, 500.0/11, *m.OutputTPS, 0.000001)
	require.Equal(t, int64(2), m.OutputTPSSamples)
	require.NotEqual(t, m.TPM/60, *m.OutputTPS)
	require.Equal(t, m.OutputTPS, a.metric(1440, false).OutputTPS, "window length changes throughput, not measured generation speed")
}

func TestChannelMonitorV2OutputTPSUnknownWithoutValidObservations(t *testing.T) {
	for _, fact := range []channelMonitorV2Fact{{Output: 1000}, {TPSOutputTokens: 100, TPSGenerationMS: 0, TPSSamples: 1}, {TPSOutputTokens: 100, TPSGenerationMS: 1000, TPSSamples: 0}} {
		a := newMetricAccumulator()
		a.addFact(fact)
		require.Nil(t, a.metric(90, true).OutputTPS)
	}
}
