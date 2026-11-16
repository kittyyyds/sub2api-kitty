package mediasession

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

type textKeyReader struct {
	assistantPermissions
	calls    int
	selected int64
}

func (p *textKeyReader) MediaTextKeys(_ context.Context, userID, keyID int64) ([]service.MediaTextKey, error) {
	if userID != 42 {
		return nil, service.ErrGroupNotAllowed
	}
	p.calls++
	p.selected = keyID
	return []service.MediaTextKey{{ID: 7, Name: "existing customer key", GroupID: 9}}, nil
}
func TestMediaTextKeyReadRequiresLiveOwnedSourceSession(t *testing.T) {
	a, _, source, _ := fixture(t)
	reader := &textKeyReader{}
	a.cfg.Permissions = reader
	grant := grantFixture(t, a)
	_, keys, err := a.TextKeys(context.Background(), grant.Handle, grant.Subject, 0)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	require.Empty(t, keys[0].Key)
	_, _, err = a.TextKeys(context.Background(), grant.Handle, strings.Repeat("x", 43), 7)
	require.Error(t, err)
	require.Equal(t, 1, reader.calls)
	source.err = service.ErrTokenRevoked
	_, _, err = a.TextKeys(context.Background(), grant.Handle, grant.Subject, 7)
	require.Error(t, err)
	require.Equal(t, 1, reader.calls)
}
