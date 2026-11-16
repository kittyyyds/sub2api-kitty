//go:build unit

package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVideoTaskWorkerCanRestartAfterDrain(t *testing.T) {
	svc, _, _, _, _ := newVideoTaskFixture()
	keys := &APIKeyService{}
	svc.StartVideoTaskWorker(keys)
	first := svc.videoTaskWorkerDone
	require.NotNil(t, first)
	svc.StartVideoTaskWorker(keys)
	require.Equal(t, first, svc.videoTaskWorkerDone, "repeated start must not duplicate settlement workers")
	svc.StopVideoTaskWorker()
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("worker did not drain")
	}
	svc.StopVideoTaskWorker()
	svc.StartVideoTaskWorker(keys)
	defer svc.StopVideoTaskWorker()
	require.NotNil(t, svc.videoTaskWorkerDone)
	require.NotEqual(t, first, svc.videoTaskWorkerDone, "reactivation must create a live worker")
}
