//go:build integration

package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/Wei-Shaw/sub2api/migrations"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

type qqKittyEncryptor struct{}

func (qqKittyEncryptor) Encrypt(s string) (string, error) { return s, nil }
func (qqKittyEncryptor) Decrypt(s string) (string, error) { return s, nil }

func TestQQBotKittyTransferPreservesFrozenAndBonusMoney(t *testing.T) {
	dsn := os.Getenv("QQBOT_TEST_DSN")
	if dsn == "" {
		t.Skip("isolated QQBOT_TEST_DSN required")
	}
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := "kitty_qq_funds_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	defer admin.Exec("DROP SCHEMA " + schema + " CASCADE")
	u, err := url.Parse(dsn)
	require.NoError(t, err)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	db, err := sql.Open("postgres", u.String())
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE settings(key TEXT PRIMARY KEY,value TEXT,updated_at TIMESTAMPTZ DEFAULT NOW());
CREATE TABLE users(id BIGINT PRIMARY KEY,email TEXT DEFAULT '',balance NUMERIC(20,8),frozen_balance NUMERIC(20,8),status TEXT DEFAULT 'active',deleted_at TIMESTAMPTZ,updated_at TIMESTAMPTZ DEFAULT NOW());
CREATE TABLE redeem_codes(id BIGSERIAL PRIMARY KEY,code VARCHAR(32) UNIQUE,type VARCHAR(20),value NUMERIC(20,8),status TEXT,used_by BIGINT REFERENCES users(id),used_at TIMESTAMPTZ,notes TEXT);
CREATE TABLE recharge_bonus_grants(id BIGSERIAL PRIMARY KEY,user_id BIGINT,remaining_amount NUMERIC(20,8),status TEXT,expires_at TIMESTAMPTZ);
CREATE TABLE kitty_permanent_usage(user_id BIGINT,input_tokens BIGINT,output_tokens BIGINT,cache_creation_tokens BIGINT,cache_read_tokens BIGINT,actual_cost NUMERIC,billing_type SMALLINT,observed_at TIMESTAMPTZ,invalid_metrics BOOLEAN);
INSERT INTO users(id,balance,frozen_balance) VALUES(1,3,7),(2,0,0);
INSERT INTO recharge_bonus_grants(user_id,remaining_amount,status,expires_at) VALUES(1,2,'active',NOW()-INTERVAL '1 minute');`)
	require.NoError(t, err)
	raw, err := migrations.FS.ReadFile("325_kitty_qq_bot.sql")
	require.NoError(t, err)
	_, err = db.Exec(string(raw))
	require.NoError(t, err)
	invalidations := map[int64]int{}
	s := qqbot.New(db, qqKittyEncryptor{}, true, func(_ context.Context, id int64) { invalidations[id]++ })
	s.SetTransferBalancePolicy(qqBotTransferableBalance)
	c := qqbot.DefaultConfig()
	c.Enabled = true
	c.AppID = "app"
	c.AppSecret = "synthetic-secret"
	c.AllowedGroups = []string{"group"}
	c.TransferEnabled = true
	_, err = s.SaveConfig(context.Background(), c)
	require.NoError(t, err)
	token := strings.Repeat("a", 48)
	h := sha256.Sum256([]byte(token))
	hash := hex.EncodeToString(h[:])
	_, err = db.Exec(`INSERT INTO qq_bot_transfer_requests(token_hash,app_id,from_member_id,from_user_id,to_user_id,from_email,to_email,amount,fee,net_amount,expires_at)
VALUES($1,'app','member',1,2,'from@example.invalid','to@example.invalid',2,0,2,NOW()+INTERVAL '10 minutes')`, hash)
	require.NoError(t, err)
	require.Error(t, s.ConfirmTransfer(context.Background(), token), "expired unsettled grants cannot be converted to transferable funds")
	_, err = db.Exec(`UPDATE recharge_bonus_grants SET remaining_amount=1`)
	require.NoError(t, err)
	require.NoError(t, s.ConfirmTransfer(context.Background(), token), "available balance already excludes frozen money; do not subtract twice")
	require.Error(t, s.ConfirmTransfer(context.Background(), token))
	var balance, frozen, recipient, grant string
	require.NoError(t, db.QueryRow(`SELECT balance::text,frozen_balance::text FROM users WHERE id=1`).Scan(&balance, &frozen))
	require.NoError(t, db.QueryRow(`SELECT balance::text FROM users WHERE id=2`).Scan(&recipient))
	require.NoError(t, db.QueryRow(`SELECT remaining_amount::text FROM recharge_bonus_grants`).Scan(&grant))
	require.Equal(t, "1.00000000", balance)
	require.Equal(t, "7.00000000", frozen)
	require.Equal(t, "2.00000000", recipient)
	require.Equal(t, "1.00000000", grant)
	require.Equal(t, map[int64]int{1: 1, 2: 1}, invalidations)
}
