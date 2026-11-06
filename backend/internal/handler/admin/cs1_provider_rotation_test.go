package admin

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCS1ProviderProbePreservesRotationAfterMetricsFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/auth/refresh":
			writeProviderJSON(w, 200, `{"code":0,"data":{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer"}}`)
		case "/api/v1/user/profile":
			writeProviderJSON(w, 200, `{"code":0,"data":{"balance":9}}`)
		default:
			writeProviderJSON(w, 502, `{"code":502,"message":"temporary failure"}`)
		}
	}))
	defer server.Close()
	useProviderTestServer(t, server)
	secret, e := encodeProviderToken("", "one-use-refresh", "Bearer")
	require.NoError(t, e)
	h := newProviderKeyTestHandler(t, providerRecord{ID: "stored", Name: "Original", SiteType: "sub2api", AuthMode: "token", BaseURL: server.URL}, secret)

	var pending []providerRecord
	require.NoError(t, h.providerSettings.LoadProviderRecords(context.Background(), &pending))
	err := h.probeRecordContext(context.Background(), &pending[0], "")
	require.Error(t, err)
	require.NoError(t, h.saveProviderRecord(context.Background(), &pending[0]))

	var records []providerRecord
	require.NoError(t, h.providerSettings.LoadProviderRecords(context.Background(), &records))
	require.Len(t, records, 1)
	require.Equal(t, "Original", records[0].Name)
	stored, e := h.providerEncryptor.Decrypt(records[0].SecretEncrypted)
	require.NoError(t, e)
	require.Contains(t, stored, "new-refresh")
}

func TestCS1ProviderPublicViewOmitsCredentials(t *testing.T) {
	b, e := json.Marshal(providerRecord{ID: "public", SecretEncrypted: "must-stay-server-side"}.view())
	require.NoError(t, e)
	require.NotContains(t, string(b), "must-stay-server-side")
	require.NotContains(t, string(b), "secret_encrypted")
}
