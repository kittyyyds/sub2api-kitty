package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (r openAIImagesFailoverAccountRepo) ListModelAvailabilityCandidates(_ context.Context, _ *int64, _ []string, _ bool) ([]service.Account, error) {
	return r.accounts, nil
}

func (r *openAIWSFailoverHandlerAccountRepoStub) ListModelAvailabilityCandidates(_ context.Context, _ *int64, _ []string, _ bool) ([]service.Account, error) {
	return r.accounts, nil
}

// The source fork's fixtures carry Tibo model-health metadata. Kitty has no
// corresponding model gating policy, so the synthetic extra map stays intact.
func astraHandlerExtra(_ string) map[string]any {
	return map[string]any{}
}
