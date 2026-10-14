package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityPolicyTranscriptCompatibilityDoesNotCreateCyberIdentity(t *testing.T) {
	body := []byte(`{"messages":[{"role":"user","content":"setup"},{"role":"assistant","content":"ready"},{"role":"user","content":"next"}]}`)
	keys := SecurityPolicyTranscriptKeys(ContentModerationProtocolOpenAIChat, 77, body)
	require.Len(t, keys, 2)
	require.NotEqual(t, keys, SecurityPolicyTranscriptKeys(ContentModerationProtocolOpenAIChat, 78, body))

	// The optional Kitty policy retains its existing lineage contract, while
	// ranxi admission still requires an explicit client conversation identity.
	identity := ResolveCyberSessionIdentity(77, nil, body)
	require.False(t, identity.Resolved())
	require.Empty(t, identity.LookupKeys)
	require.Empty(t, CyberSessionExplicitBlockKey(77, nil, body))
}
