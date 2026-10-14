package reauthruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Only the reviewed release is adapted. Changes in the upstream login engine
// require another review; a mismatch fails closed rather than silently changing
// the selected proxy. The original, verified runtime is never modified.
const pinnedTransportSHA = "d1986fa95a3fbbd88ff2c7253f6b59a6fe4884152356785762453f3baca592cf"
const pinnedProtocolSHA = "b7e5ed0a4dcc2955607815f01ea291d1708c661af0796b19dc5dab23ff9c4752"

func pinnedTransport(source []byte) ([]byte, error) {
	sum := sha256.Sum256(source)
	if hex.EncodeToString(sum[:]) != pinnedTransportSHA {
		return nil, errors.New("unsupported proxy login runtime revision")
	}
	return pinTransportSource(string(source))
}

func pinTransportSource(source string) ([]byte, error) {
	guard := `
function requireSelectedProxy(transport, proxy) {
  const selected = process.env.CHATGPT_PROXY_URL;
  if (!transport.enabled || !selected || !proxy ||
      new URL(proxy).href !== new URL(selected).href) {
    throw new Error("SELECTED_PROXY_REQUIRED_NO_DIRECT_FALLBACK");
  }
}
`
	replacements := [][2]string{
		{"  async configure(proxy, { force = false } = {}) {", "  async configure(proxy, { force = false } = {}) {\n    requireSelectedProxy(this, proxy);"},
		{"  async prepareProxy(proxyTemplate, validationUrl) {", "  async prepareProxy(proxyTemplate, validationUrl) {\n    requireSelectedProxy(this, proxyTemplate);"},
		{"const canRotate = proxySupportsSessionRotation(template);", "const canRotate = false; // Operator-selected session must not be rewritten."},
		{"verifyTls: shouldVerifyTlsForProxy(proxy),", "verifyTls: true,"},
		{"  async request(method, url, options = {}) {", "  async request(method, url, options = {}) {\n    requireSelectedProxy(this, Object.hasOwn(options, 'proxy') ? options.proxy : this.configuredProxy);"},
		{"const riskControl = isRiskControlResult(result);", "const riskControl = isRiskControlResult(result);\n      if (riskControl) throw new Error('PROXY_SECURITY_CHECK_REQUIRES_INTERACTION');"},
	}
	for _, pair := range replacements {
		if strings.Count(source, pair[0]) != 1 {
			return nil, errors.New("unsupported proxy login runtime layout")
		}
		source = strings.Replace(source, pair[0], pair[1], 1)
	}
	return []byte(guard + source), nil
}

func preparePinnedLoginSource(dir, tmp string) error {
	source := filepath.Join(dir, "tosub2", "src")
	target := filepath.Join(tmp, "src")
	var total int64
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		dest := filepath.Join(target, rel)
		if entry.Type()&os.ModeSymlink != 0 {
			return errors.New("login runtime source symlink rejected")
		}
		if entry.IsDir() {
			return os.MkdirAll(dest, 0700)
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		total += info.Size()
		if !info.Mode().IsRegular() || info.Size() > 2<<20 || total > 8<<20 {
			return errors.New("invalid login runtime source size")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if rel == "protocol-login.mjs" {
			sum := sha256.Sum256(data)
			if hex.EncodeToString(sum[:]) != pinnedProtocolSHA {
				return errors.New("unsupported login protocol revision")
			}
		}
		if rel == "tls-transport.mjs" {
			data, err = pinnedTransport(data)
			if err != nil {
				return err
			}
		}
		return os.WriteFile(dest, data, 0600)
	})
}
