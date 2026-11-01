package reauthruntime

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

//go:embed bps_connection_login.mjs
var bpsConnectionFlow []byte

// BPSConnectionLogin returns only a callback for the API-owned PKCE session.
// It cannot mint tokens, select a connection, or change parent credentials.
func (m *Manager) BPSConnectionLogin(ctx context.Context, email, password, totp, proxy, authURL, workspace string) (string, error) {
	if ctx.Err() != nil {
		return "", ctx.Err()
	}
	if proxy != "" {
		u, err := url.Parse(proxy)
		if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "socks5" && u.Scheme != "socks5h") {
			return "", errors.New("invalid login proxy")
		}
	}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	select {
	case m.loginSlots <- struct{}{}:
		defer func() { <-m.loginSlots }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	m.Ensure()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for m.Status().State != "running" {
		if m.Status().State == "unavailable" || m.Status().State == "stopped" {
			return "", &LoginError{Progress: LoginProgress{Status: "failed", Stage: "protocol_login", Code: "login_runtime_dependency_failed"}}
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
	dir := filepath.Join(m.root, m.version+"-linux-"+runtime.GOARCH)
	return bpsConnectionProcess(ctx, dir, email, password, totp, proxy, authURL, workspace)
}

func prepareBPSConnectionSource(dir, tmp string, direct bool) error {
	if err := preparePinnedLoginSource(dir, tmp); err != nil {
		return err
	}
	path := filepath.Join(tmp, "src", "protocol-login.mjs")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	patched, err := patchBPSConnectionSource(string(data))
	if err != nil {
		return err
	}
	if err = os.WriteFile(path, []byte(patched), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(tmp, "src", "bps_connection_login.mjs"), bpsConnectionFlow, 0600); err != nil {
		return err
	}
	if direct {
		path = filepath.Join(tmp, "src", "tls-transport.mjs")
		data, err = os.ReadFile(path)
		if err != nil {
			return err
		}
		old := "  const selected = process.env.CHATGPT_PROXY_URL;"
		if strings.Count(string(data), old) != 1 {
			return errors.New("unsupported direct login layout")
		}
		// Explicit direct mode is allowed only when the account has no proxy. A
		// selected proxy never enters this branch and can never fall back to direct.
		data = []byte(strings.Replace(string(data), old, old+"\n  if (!selected) { if (proxy) throw new Error('UNEXPECTED_LOGIN_PROXY'); return; }", 1))
		if err = os.WriteFile(path, data, 0600); err != nil {
			return err
		}
	}
	return nil
}

func patchBPSConnectionSource(source string) (string, error) {
	replacements := [][3]string{
		{`import crypto from "node:crypto";`, `import crypto from "node:crypto";` + "\nimport {loginBPSConnection} from './bps_connection_login.mjs';", "1"},
		{`async function loginChatgptWeb(client, { chatgptBase, authBase, email, rl, password, totpSecret }) {`, `async function loginChatgptWeb(client, { chatgptBase, authBase, email, rl, password, totpSecret }) {
  return loginBPSConnection(client,{authBase,email,rl,password,totpSecret},
    {isPasswordLoginPage,isAuthLoginPage,createSentinelHeaders,authJsonStep,getContinueUrl,verifyPassword,
     isMfaChallengePayload,completeTotpMfaIfNeeded,pickWorkspaceId,selectChatgptLoginWorkspaceIfNeeded,continueFlow});`, "1"},
		{`async function runCodexOauth(client, options) {`, `async function runCodexOauth(client, options) {
  if (client.bpsConnection) return client.bpsConnection;
  throw new Error("BPS_CALLBACK_MISSING");`, "1"},
		{`      console.log(codex.callbackUrl);`, `      console.log("[bps] Callback received; query withheld.");`, "1"},
		{`    if (codex?.callbackUrl && codex?.codeVerifier && !args.noSub2apiExport`, `    if (codex?.callbackUrl) {
      await writeJsonAtomic(sub2apiOutPath, {callback_url:codex.callbackUrl});
      await removeProtocolCheckpoint(checkpointPath);
      return;
    }
    if (codex?.callbackUrl && codex?.codeVerifier && !args.noSub2apiExport`, "1"},
		{`    maxProxySessionAttempts: process.env.CHATGPT_PROXY_MAX_ATTEMPTS || 10,`, `    maxProxySessionAttempts: 1,
    sameProxyRiskRetries: 0,
    cloudflareSolver: false,`, "1"},
		{"await transport.prepareProxy(proxyTemplate, `${chatgptBase}/`);", "await transport.prepareProxy(proxyTemplate, `${authBase}/`);", "3"},
		{`async function ask(rl, prompt) {`, `async function ask(rl, prompt) {
  throw new Error("BPS_INTERACTION_REQUIRED");`, "1"},
		{`  const organization = workspaces.find((item) => item?.kind === "organization" && item?.id);`, `  const expected = process.env.KITTY_BPS_WORKSPACE;
  if (expected) {
    const matched = workspaces.find(item => item?.id === expected);
    if (!matched) throw new Error("BPS_WORKSPACE_MISMATCH");
    return matched.id;
  }
  const organization = workspaces.find((item) => item?.kind === "organization" && item?.id);`, "1"},
		{"(parsed.hostname === \"localhost\" || parsed.hostname === \"127.0.0.1\") &&\n      parsed.pathname === \"/auth/callback\"", `parsed.origin === "https://bps.openai.com" &&
      parsed.pathname === "/basispoints/extension/360590d7-f8f9-4d88-bf75-0edfe0a4b9f3/auth/callback" && !parsed.username && !parsed.password`, "1"},
	}
	for _, r := range replacements {
		count := 1
		if r[2] == "3" {
			count = 3
		}
		if strings.Count(source, r[0]) != count {
			return "", errors.New("unsupported BPS connection login layout")
		}
		source = strings.ReplaceAll(source, r[0], r[1])
	}
	return source, nil
}

func bpsConnectionProcess(ctx context.Context, dir, email, password, totp, proxy, authURL, workspace string) (string, error) {
	tmp, err := os.MkdirTemp("", "sub2api-bps-connection-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err = prepareBPSConnectionSource(dir, tmp, proxy == ""); err != nil {
		return "", &LoginError{Progress: LoginProgress{Status: "failed", Stage: "protocol_login", Code: "login_runtime_dependency_failed"}}
	}
	cmd := passwordLoginCommand(ctx, dir, tmp, email, password, totp, proxy)
	cmd.Env = append(cmd.Env, "KITTY_BPS_AUTH_URL="+authURL, "KITTY_BPS_WORKSPACE="+workspace)
	diagnostic := &passwordLoginDiagnostic{}
	cmd.Stdout = diagnostic
	cmd.Stderr = diagnostic
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		return "", diagnostic.failure()
	}
	f, err := os.Open(filepath.Join(tmp, "oauth.json"))
	if err != nil {
		return "", errors.New("BPS callback unavailable")
	}
	defer f.Close()
	var result struct {
		CallbackURL string `json:"callback_url"`
	}
	if err = json.NewDecoder(io.LimitReader(f, 16384)).Decode(&result); err != nil || result.CallbackURL == "" || len(result.CallbackURL) > 4096 {
		return "", errors.New("invalid BPS callback")
	}
	return result.CallbackURL, nil
}
