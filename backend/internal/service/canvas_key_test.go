//go:build unit

package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type canvasServiceKeyRepo struct {
	APIKeyRepository
	key *APIKey
}

func (r *canvasServiceKeyRepo) GetByID(context.Context, int64) (*APIKey, error) {
	copy := *r.key
	return &copy, nil
}

type canvasServiceUserRepo struct {
	UserRepository
	calls int
}

func (r *canvasServiceUserRepo) GetByID(context.Context, int64) (*User, error) {
	r.calls++
	return &User{ID: 42, AllowedGroups: []int64{3}}, nil
}

func TestCanvasOwnedKeyLoadsCurrentPermissionsAndHashOnly(t *testing.T) {
	u := &canvasServiceUserRepo{}
	s := &APIKeyService{apiKeyRepo: &canvasServiceKeyRepo{key: &APIKey{ID: 7, UserID: 42, KeyHash: "fixture-hash", IPWhitelist: []string{"203.0.113.0/24"}, User: &User{ID: 42}}}, userRepo: u}
	key, err := s.GetOwnedForCanvas(context.Background(), 42, 7)
	require.NoError(t, err)
	require.Empty(t, key.Key)
	require.Equal(t, "fixture-hash", key.KeyHash)
	require.Equal(t, []int64{3}, key.User.AllowedGroups)
	require.NotNil(t, key.CompiledIPWhitelist)
	_, err = s.GetOwnedForCanvas(context.Background(), 43, 7)
	require.ErrorIs(t, err, ErrAPIKeyNotFound)
	require.Equal(t, 1, u.calls)
}
