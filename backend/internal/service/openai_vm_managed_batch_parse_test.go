package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const batchSecret = "JBSWY3DPEHPK3PXP"

func TestParseOpenAIVMBatchKeepsPasswordsIntactAndSplitsFromBothEnds(t *testing.T) {
	text := strings.Join([]string{
		"one@example.test---plain-password---" + batchSecret,
		"  two@example.test---pass---with---dashes---jbsw y3dp ehpk 3pxp  ",
		"",
		"three@example.test----four-dash-typo----" + batchSecret,
		"four@example.test---p4---" + batchSecret + "---socks5://user:proxy-pass@exit.example:1080",
		"five@example.test---has://scheme-inside---" + batchSecret,
		"six@example.test--- leading and trailing spaces kept ---" + batchSecret + "===",
	}, "\r\n")
	lines, problems := ParseOpenAIVMBatch(text)
	require.Empty(t, problems)
	require.Len(t, lines, 6)
	want := []OpenAIVMBatchLine{
		{LineNo: 1, Email: "one@example.test", Password: "plain-password", MFASecret: batchSecret},
		{LineNo: 2, Email: "two@example.test", Password: "pass---with---dashes", MFASecret: batchSecret},
		{LineNo: 4, Email: "three@example.test", Password: "four-dash-typo", MFASecret: batchSecret},
		{LineNo: 5, Email: "four@example.test", Password: "p4", MFASecret: batchSecret, ProxyURL: "socks5://user:proxy-pass@exit.example:1080"},
		{LineNo: 6, Email: "five@example.test", Password: "has://scheme-inside", MFASecret: batchSecret},
		{LineNo: 7, Email: "six@example.test", Password: " leading and trailing spaces kept ", MFASecret: batchSecret + "==="},
	}
	require.Equal(t, want, lines)
}

func TestParseOpenAIVMBatchReportsBadLinesWithoutQuotingThem(t *testing.T) {
	text := strings.Join([]string{
		"ok@example.test---canary-good---" + batchSecret,                                             // 1
		"only-two-fields@example.test---canary-two-fields",                                           // 2
		"canary-no-separators",                                                                       // 3
		"---canary-starts---" + batchSecret,                                                          // 4
		"canary-not-an-email---canary-value---" + batchSecret,                                        // 5
		"blank@example.test---   ---" + batchSecret,                                                  // 6
		"short@example.test---canary-pw-a---CANARYABC",                                               // 7
		"digits@example.test---canary-pw-b---0123456789012345",                                       // 8
		"OK@example.test---canary-another---" + batchSecret,                                          // 9: same account as line 1
		"http@example.test---canary-pw---" + batchSecret + "---http://canary-proxy.example:8080",     // 10
		"noport@example.test---canary-pw---" + batchSecret + "---socks5://canary-proxy.example",      // 11
		"path@example.test---canary-pw---" + batchSecret + "---socks5://canary-proxy.example:1080/x", // 12
		"two@at@example.test---canary-pw---" + batchSecret,                                           // 13
		"nodot@localhost---canary-pw---" + batchSecret,                                               // 14
	}, "\n")
	lines, problems := ParseOpenAIVMBatch(text)
	require.Len(t, lines, 1)
	require.Equal(t, "ok@example.test", lines[0].Email)
	got := map[int]string{}
	for _, problem := range problems {
		got[problem.LineNo] = problem.Code
	}
	require.Equal(t, map[int]string{
		2: "invalid_format", 3: "invalid_format", 4: "invalid_format", 5: "invalid_email", 6: "missing_password",
		7: "invalid_mfa_secret", 8: "invalid_mfa_secret", 9: "duplicate_in_batch", 10: "invalid_proxy", 11: "invalid_proxy",
		12: "invalid_proxy", 13: "invalid_email", 14: "invalid_email",
	}, got)
	// What goes back to the browser (and into the database) never holds a
	// password, a 2FA secret or a proxy credential.
	report, err := json.Marshal(problems)
	require.NoError(t, err)
	for _, secret := range []string{"canary", "CANARY", batchSecret, "0123456789012345"} {
		require.NotContainsf(t, string(report), secret, "a line error quoted %q", secret)
	}
	require.Contains(t, string(report), "short@example.test", "a well-formed email identifies the bad line")
}

func TestParseOpenAIVMBatchProxy(t *testing.T) {
	proxy, ok := ParseOpenAIVMBatchProxy(" socks5h://User:p%40ss@EXIT.example:1080 ")
	require.True(t, ok)
	require.Equal(t, OpenAIVMBatchProxy{Protocol: "socks5h", Host: "exit.example", Port: 1080, Username: "User", Password: "p@ss"}, proxy)
	proxy, ok = ParseOpenAIVMBatchProxy("socks5://10.0.0.8:1080/")
	require.True(t, ok)
	require.Equal(t, OpenAIVMBatchProxy{Protocol: "socks5", Host: "10.0.0.8", Port: 1080}, proxy)
	for _, bad := range []string{"", "exit.example:1080", "http://exit.example:8080", "https://exit.example:443", "socks4://exit.example:1080",
		"socks5://exit.example", "socks5://exit.example:0", "socks5://exit.example:70000", "socks5://:1080", "socks5://exit.example:1080/path",
		"socks5://exit.example:1080?x=1", "socks5://exit.example:1080#f"} {
		_, ok := ParseOpenAIVMBatchProxy(bad)
		require.Falsef(t, ok, "%q accepted", bad)
	}
}

func TestAssignOpenAIVMBatchProxiesSpreadsAndNeverOverfills(t *testing.T) {
	line := func(n int, own string) OpenAIVMBatchLine { return OpenAIVMBatchLine{LineNo: n, ProxyURL: own} }
	lines := []OpenAIVMBatchLine{line(1, ""), line(2, ""), line(3, "socks5://own.example:1080"), line(4, ""), line(5, ""), line(6, ""), line(7, "")}

	// Two proxies, two accounts each: lines spread across both before either fills.
	assigned, unplaced := AssignOpenAIVMBatchProxies(lines, []int64{10, 20}, nil, 2)
	require.Equal(t, map[int]int64{1: 10, 2: 20, 4: 10, 5: 20}, assigned)
	require.Equal(t, []int{6, 7}, unplaced, "lines beyond the capacity are not created")
	require.NotContains(t, assigned, 3, "a line with its own proxy takes no slot on the selected ones")

	// Accounts a proxy already carries count against its limit.
	assigned, unplaced = AssignOpenAIVMBatchProxies(lines, []int64{10, 20}, map[int64]int{10: 2, 20: 1}, 2)
	require.Equal(t, map[int]int64{1: 20}, assigned)
	require.Equal(t, []int{2, 4, 5, 6, 7}, unplaced)

	// No proxy selected: nothing is placed, nothing falls back to "any proxy".
	assigned, unplaced = AssignOpenAIVMBatchProxies(lines, nil, nil, 5)
	require.Empty(t, assigned)
	require.Equal(t, []int{1, 2, 4, 5, 6, 7}, unplaced)

	// A nonsensical limit means one account per proxy, never unlimited.
	assigned, unplaced = AssignOpenAIVMBatchProxies(lines, []int64{10}, nil, 0)
	require.Equal(t, map[int]int64{1: 10}, assigned)
	require.Len(t, unplaced, 5)

	// The caller's bookkeeping is not modified.
	used := map[int64]int{10: 1}
	AssignOpenAIVMBatchProxies(lines, []int64{10}, used, 3)
	require.Equal(t, map[int64]int{10: 1}, used)
}
