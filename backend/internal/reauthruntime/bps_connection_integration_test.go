//go:build linux

package reauthruntime

import (
	"context"
	"fmt"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// Uses the installed Node/Python transport against a local fixture only.
func TestBPSLoginInstalledTransportModes(t *testing.T) {
	dir := os.Getenv("PINNED_RUNTIME_FIXTURE")
	if dir == "" {
		t.Skip("isolated installed runtime required")
	}
	for _, direct := range []bool{false, true} {
		t.Run(fmt.Sprint("direct=", direct), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			tmp := t.TempDir()
			require.NoError(t, prepareBPSConnectionSource(dir, tmp, direct))
			target, proxy := "http://fixture.invalid/probe", server.URL
			if direct {
				target, proxy = server.URL, ""
			}
			module := (&url.URL{Scheme: "file", Path: filepath.Join(tmp, "src", "tls-transport.mjs")}).String()
			script := fmt.Sprintf(`const {TlsFingerprintTransport}=await import(%q);const t=new TlsFingerprintTransport({enabled:true,profile:'chrome146'});try{await t.prepareProxy(process.env.CHATGPT_PROXY_URL||null,%q);const r=await t.request('GET',%q);if(r.status!==200)throw Error('missing fixture response');let rejected=false;try{await t.configure('http://127.0.0.1:1')}catch{rejected=true}if(!rejected)throw Error('changed proxy accepted')}finally{await t.close()}`, module, target, target)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, filepath.Join(dir, "node"), "--input-type=module", "-e", script)
			cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + tmp, "TMPDIR=" + tmp, "TOSUB2_PYTHON=" + filepath.Join(dir, "python"), "NODE_EXECUTABLE=" + filepath.Join(dir, "node"), "CHATGPT_PROXY_URL=" + proxy}
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			require.Positive(t, calls.Load())
		})
	}
}
