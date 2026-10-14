//go:build linux

package reauthruntime

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Opt-in, no real account and no real upstream. The .invalid destination cannot
// be reached directly. Every request must arrive at this controlled proxy.
func TestPinnedLoginActualProxyTransport(t *testing.T) {
	dir := os.Getenv("PINNED_RUNTIME_FIXTURE")
	if dir == "" {
		t.Skip("requires isolated verified runtime fixture")
	}
	var count atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		if r.URL.Host != "fixture.invalid" {
			w.WriteHeader(400)
			return
		}
		user, pass, ok := proxyCredentials(r)
		if !ok || user != "fixture-sid-original" || pass != "synthetic-password" {
			w.WriteHeader(407)
			return
		}
		if r.URL.Path == "/challenge" {
			w.Header().Set("cf-mitigated", "challenge")
			w.WriteHeader(403)
			io.WriteString(w, "security check")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	defer proxy.Close()
	u, err := url.Parse(proxy.URL)
	require.NoError(t, err)
	u.User = url.UserPassword("fixture-sid-original", "synthetic-password")
	tmp := t.TempDir()
	require.NoError(t, preparePinnedLoginSource(dir, tmp))
	module := (&url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(tmp, "src", "tls-transport.mjs"))}).String()
	script := fmt.Sprintf(`
 const {TlsFingerprintTransport}=await import(%q);
 const t=new TlsFingerprintTransport({enabled:true,profile:'chrome146'});
 try {
  await t.prepareProxy(process.env.CHATGPT_PROXY_URL,'http://fixture.invalid/probe');
  const r=await t.request('GET','http://fixture.invalid/login');
  if(r.status!==200)throw new Error('proxy response missing');
  for(const p of [null,'http://127.0.0.1:1']){
   let blocked=false;try{await t.configure(p)}catch{blocked=true}if(!blocked)throw new Error('fallback accepted');
  }
  let blocked=false;try{await t.request('GET','http://fixture.invalid/challenge')}catch(e){blocked=e.message==='PROXY_SECURITY_CHECK_REQUIRES_INTERACTION'}
  if(!blocked)throw new Error('challenge must stop');
 }finally{await t.close()}
 `, module)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join(dir, "node"), "--input-type=module", "-e", script)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + tmp, "TMPDIR=" + tmp, "TOSUB2_PYTHON=" + filepath.Join(dir, "python"), "NODE_EXECUTABLE=" + filepath.Join(dir, "node"), "CHATGPT_PROXY_URL=" + u.String()}
	output, err := cmd.CombinedOutput()
	// Only synthetic credentials can appear in this isolated test diagnostic.
	require.NoError(t, err, strings.ReplaceAll(string(output), "synthetic-password", "[redacted]"))
	require.Equal(t, int32(3), count.Load())
}
func proxyCredentials(r *http.Request) (string, string, bool) {
	copy := r.Clone(r.Context())
	copy.Header = copy.Header.Clone()
	copy.Header.Set("Authorization", r.Header.Get("Proxy-Authorization"))
	return copy.BasicAuth()
}
