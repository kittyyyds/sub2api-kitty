//go:build unit

package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestKittySuanliAuthorizationRemainsProviderScoped(t *testing.T) {
	req := &OpenAIImagesRequest{Suanli: true, Model: "nano-banana-custom", Endpoint: openAIImagesGenerationsEndpoint}
	good := &Account{Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://suanliai.top/v1"}}
	require.NoError(t, req.ValidateAccountModels(good, ""))
	ctx := req.WithModelAuthorization(context.Background(), "")
	require.False(t, extraImagesAccountRejected(ctx, good))
	for _, a := range []*Account{nil, {Platform: PlatformOpenAI, Type: AccountTypeOAuth}, {Platform: PlatformOpenAI, Type: AccountTypeAPIKey, Credentials: map[string]any{"base_url": "https://other.example"}}} {
		require.Error(t, req.ValidateAccountModels(a, ""))
		require.True(t, extraImagesAccountRejected(ctx, a))
	}
	require.False(t, good.AllowsExtraImageModel(req.Model), "Suanli authorization must not grant unrelated generic image access")
	req.Suanli = false
	require.Error(t, req.ValidateAccountModels(good, ""))
}
