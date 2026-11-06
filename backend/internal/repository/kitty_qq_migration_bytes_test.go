package repository

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/migrations"
)

// Migration hashes bind the embedded bytes, not Git's normalized text view.
// A CRLF checkout can otherwise pass git status yet break the recovery image.
func TestKittyQQMigrationCanonicalBytes(t *testing.T) {
	raw, err := migrations.FS.ReadFile("325_kitty_qq_bot.sql")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("\r\n")) {
		t.Fatal("QQ migration must use the committed LF bytes before embedding")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(string(raw))))
	if got := hex.EncodeToString(sum[:]); got != "9d35c13609c9dbe37ec9d520f115f84a10b7d99587db27f4339248e0dbbdbb50" {
		t.Fatalf("unreviewed QQ migration checksum: %s", got)
	}
}
