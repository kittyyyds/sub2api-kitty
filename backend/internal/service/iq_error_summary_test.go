package service

import (
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestIQErrorSummary(t *testing.T) {
	for _, message := range []string{ErrAccountTestBusy.Error(), "API returned 403", "context deadline exceeded", "代理连接失败"} {
		require.Equal(t, message, IQErrorSummary(message))
	}
	require.Equal(t, "请求失败，请查看测试详情", IQErrorSummary("\n "))
	raw := `proxy socks5://alice:secret@proxy.invalid:1080 failed; Authorization: Bearer private-token; {"refresh_token":"private-refresh","password":"secret-pass"}`
	safe := IQErrorSummary(raw)
	for _, secret := range []string{"alice", "secret@", "private-token", "private-refresh", "secret-pass"} {
		require.NotContains(t, safe, secret)
	}
	long := IQErrorSummary(strings.Repeat("错误", 400))
	require.True(t, utf8.ValidString(long))
	require.Contains(t, long, "详见测试记录")
}
