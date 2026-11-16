//go:build integration

package qqbot

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type kittyNoExternalTransport struct{ t *testing.T }

func (p kittyNoExternalTransport) RoundTrip(*http.Request) (*http.Response, error) {
	p.t.Error("disabled QQ bot made an external request")
	return nil, errors.New("external call forbidden")
}

func TestKittyDisabledRuntimeCanStopAndRestartWithoutExternalWork(t *testing.T) {
	db := testDB(t)
	s := New(db, testEncryptor{}, true, nil)
	s.http = &http.Client{Transport: kittyNoExternalTransport{t}}
	for range 2 {
		s.Start()
		s.Start() // A second lease notification cannot duplicate workers.
		time.Sleep(1200 * time.Millisecond)
		s.Stop()
		s.Stop()
	}
	for _, table := range []string{"qq_bot_jobs", "qq_bot_rewards", "qq_bot_transfer_history"} {
		var count int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM "+table).Scan(&count))
		require.Zero(t, count)
	}
}

func kittyTransferFixture(t *testing.T) (*Service, int64, int64, string) {
	t.Helper()
	s := savedBot(t, testDB(t))
	require.NoError(t, s.SaveTransferConfig(context.Background(), TransferConfig{TransferEnabled: true}))
	from, to := user(t, s.db), user(t, s.db)
	_, err := s.db.Exec(`UPDATE users SET balance=10 WHERE id=$1`, from)
	require.NoError(t, err)
	token := strings.Repeat("a", 48)
	_, err = s.db.Exec(`INSERT INTO qq_bot_transfer_requests(token_hash,app_id,from_member_id,from_user_id,to_user_id,from_email,to_email,amount,fee,net_amount,expires_at)
VALUES($1,'app','member',$2,$3,'from@example.invalid','to@example.invalid',5,1,4,NOW()+INTERVAL '10 minutes')`, codeHash(token), from, to)
	require.NoError(t, err)
	return s, from, to, token
}

func TestKittyTransferHistoryFailureRollsBackBothBalancesAndToken(t *testing.T) {
	s, from, to, token := kittyTransferFixture(t)
	_, err := s.db.Exec(`ALTER TABLE redeem_codes ADD CONSTRAINT reject_qq_transfer CHECK(type<>'qq_transfer')`)
	require.NoError(t, err)
	require.Error(t, s.ConfirmTransfer(context.Background(), token))
	var sender, recipient string
	require.NoError(t, s.db.QueryRow(`SELECT balance::text FROM users WHERE id=$1`, from).Scan(&sender))
	require.NoError(t, s.db.QueryRow(`SELECT balance::text FROM users WHERE id=$1`, to).Scan(&recipient))
	require.Equal(t, "10.00000000", sender)
	require.Equal(t, "0.00000000", recipient)
	var receipts, pending int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM qq_bot_transfer_history`).Scan(&receipts))
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM qq_bot_transfer_requests WHERE token_hash=$1`, codeHash(token)).Scan(&pending))
	require.Zero(t, receipts)
	require.Equal(t, 1, pending)
}

func TestKittyTransferConcurrentConfirmationHasOneReceiptAndTwoHistoryEntries(t *testing.T) {
	s, from, to, token := kittyTransferFixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for range 12 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- s.ConfirmTransfer(context.Background(), token) }()
	}
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			t.Logf("confirmation: %v", err)
		}
	}
	require.Equal(t, 1, succeeded)
	var sender, recipient string
	require.NoError(t, s.db.QueryRow(`SELECT balance::text FROM users WHERE id=$1`, from).Scan(&sender))
	require.NoError(t, s.db.QueryRow(`SELECT balance::text FROM users WHERE id=$1`, to).Scan(&recipient))
	require.Equal(t, "5.00000000", sender)
	require.Equal(t, "4.00000000", recipient)
	var receipts, entries int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM qq_bot_transfer_history`).Scan(&receipts))
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM redeem_codes WHERE type='qq_transfer'`).Scan(&entries))
	require.Equal(t, 1, receipts)
	require.Equal(t, 2, entries)
	var historyTotal string
	require.NoError(t, s.db.QueryRow(`SELECT SUM(value)::text FROM redeem_codes WHERE type='qq_transfer'`).Scan(&historyTotal))
	require.Equal(t, "-1.00000000", historyTotal)
	principal, err := rechargeSum(context.Background(), s.db, to, nil, false)
	require.NoError(t, err)
	require.True(t, principal.IsZero(), "transferred credits cannot become recharge eligibility")
}

func TestKittyTransferRechecksHostBalancePolicyAndFailsClosed(t *testing.T) {
	s, _, _, token := kittyTransferFixture(t)
	s.SetTransferBalancePolicy(func(context.Context, *sql.Tx, int64, string) (string, error) { return "4.99999999", nil })
	require.Error(t, s.ConfirmTransfer(context.Background(), token))
	s.SetTransferBalancePolicy(func(context.Context, *sql.Tx, int64, string) (string, error) {
		return "", errors.New("ledger unavailable")
	})
	require.Error(t, s.ConfirmTransfer(context.Background(), token))
	var pending int
	require.NoError(t, s.db.QueryRow(`SELECT COUNT(*) FROM qq_bot_transfer_requests`).Scan(&pending))
	require.Equal(t, 1, pending)
}
