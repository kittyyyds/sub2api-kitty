//go:build unit

package middleware

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"testing"
)

type canvasQueueKeyRepo struct {
	service.APIKeyRepository
	key *service.APIKey
}

func (r *canvasQueueKeyRepo) GetByID(_ context.Context, id int64) (*service.APIKey, error) {
	if r.key == nil || r.key.ID != id {
		return nil, service.ErrAPIKeyNotFound
	}
	c := *r.key
	return &c, nil
}
func TestCanvasQueueRevalidationUsesOwnerAndFreshPermissionsWithoutCredential(t *testing.T) {
	key := canvasTestKey()
	key.ConcurrencyLimit = 2
	repo := &canvasQueueKeyRepo{key: key}
	users := &stubUserRepo{getByID: func(context.Context, int64) (*service.User, error) { return key.User, nil }}
	svc := service.NewAPIKeyService(repo, users, nil, nil, nil, nil, &config.Config{})
	revalidate := newCanvasQueueAuthRevalidator(svc, "", key)
	limit, err := revalidate(context.Background())
	require.NoError(t, err)
	require.Equal(t, 2, limit)
	require.Empty(t, key.Key, "Canvas works with hash-only keys")
	key.Status = service.StatusDisabled
	_, err = revalidate(context.Background())
	require.Error(t, err)
	key.Status = service.StatusActive
	key.UserID++
	_, err = revalidate(context.Background())
	require.Error(t, err, "ownership cannot change during the queue wait")
}
