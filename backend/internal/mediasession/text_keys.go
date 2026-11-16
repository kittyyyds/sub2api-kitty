package mediasession

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

func (a *Authority) TextKeys(ctx context.Context, handle, subject string, keyID int64) (*Identity, []service.MediaTextKey, error) {
	r, err := a.load(ctx, handle, subject)
	if err != nil {
		return nil, nil, err
	}
	reader, ok := a.cfg.Permissions.(interface {
		MediaTextKeys(context.Context, int64, int64) ([]service.MediaTextKey, error)
	})
	if !ok {
		return nil, nil, ErrUnavailable
	}
	keys, err := reader.MediaTextKeys(ctx, r.Source.UserID, keyID)
	if err != nil {
		return nil, nil, ErrUnavailable
	}
	id := a.identity(*r)
	return &id, keys, nil
}
