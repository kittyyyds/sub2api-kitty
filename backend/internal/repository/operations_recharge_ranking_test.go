package repository

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
	"testing"
	"time"
)

func TestOperationsRechargeRankingCompletedCreditBoundaries(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	require.NoError(t, err)
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{
		`CREATE TABLE users(id INTEGER PRIMARY KEY,email TEXT,role TEXT)`,
		`CREATE TABLE payment_orders(user_id INTEGER,amount NUMERIC,completed_at DATETIME,paid_at DATETIME,order_type TEXT,status TEXT)`,
		`CREATE TABLE redeem_codes(used_by INTEGER,value NUMERIC,used_at DATETIME,status TEXT,type TEXT,code TEXT)`,
	} {
		_, err = db.Exec(q)
		require.NoError(t, err)
	}
	start := time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC)
	end := start.AddDate(0, 0, 1)
	for i := 1; i <= 63; i++ {
		_, err = db.Exec(`INSERT INTO users VALUES(?,?,'user')`, i, fmt.Sprintf("user%d@example.com", i))
		require.NoError(t, err)
		_, err = db.Exec(`INSERT INTO payment_orders VALUES(?,?,?,?,?,?)`, i, i, start.Add(time.Hour), start.Add(-time.Hour), "balance", "COMPLETED")
		require.NoError(t, err)
	}
	exec := func(q string, args ...any) { _, err := db.Exec(q, args...); require.NoError(t, err) }
	exec(`INSERT INTO users VALUES(100,'admin@example.com','admin')`)
	exec(`INSERT INTO redeem_codes VALUES(1,1,?,'used','balance','PAY-AUTO')`, start)
	exec(`INSERT INTO redeem_codes VALUES(1,20,?,'used','balance','REDEEM')`, start)
	exec(`INSERT INTO redeem_codes VALUES(1,30,?,'used','admin_balance','MANUAL')`, start)
	exec(`INSERT INTO redeem_codes VALUES(1,-5,?,'used','admin_balance','DEDUCT')`, start)
	exec(`INSERT INTO redeem_codes VALUES(100,999,?,'used','balance','ADMIN')`, start)
	exec(`INSERT INTO redeem_codes VALUES(1,999,?,'used','balance','TOMORROW')`, end)
	exec(`INSERT INTO payment_orders VALUES(1,999,NULL,?,'balance','PAID')`, start)
	exec(`INSERT INTO payment_orders VALUES(1,999,?,?,'subscription','COMPLETED')`, start, start)
	exec(`INSERT INTO payment_orders VALUES(1,999,?,?,'balance','COMPLETED')`, end, end)
	exec(`INSERT INTO payment_orders VALUES(1,999,?,?,'balance','COMPLETED')`, start.Add(-time.Second), start)
	// Preserve the pre-existing gross-credit definition across refund states.
	exec(`UPDATE payment_orders SET status='REFUNDED' WHERE user_id=2`)
	repo := newUsageLogRepositoryWithSQL(nil, db)
	for _, limit := range []int{3, 50} {
		data, err := repo.GetOperationsRechargeRanking(context.Background(), start, end, limit)
		require.NoError(t, err)
		require.Len(t, data.Users, limit)
		require.Equal(t, int64(63), data.Users[0].UserID)
		require.InDelta(t, 2066, data.TotalAmount, 1e-9)
		require.Equal(t, int64(63), data.TotalUsers)
		require.Equal(t, int64(65), data.TotalCount)
	}
	empty, err := repo.GetOperationsRechargeRanking(context.Background(), end, end.AddDate(0, 0, 1), 50)
	require.NoError(t, err)
	// The tomorrow fixture is a real next-day credit; an empty day remains []/0.
	require.Equal(t, int64(2), empty.TotalCount)
	empty, err = repo.GetOperationsRechargeRanking(context.Background(), end.AddDate(0, 0, 1), end.AddDate(0, 0, 2), 50)
	require.NoError(t, err)
	require.NotNil(t, empty.Users)
	require.Empty(t, empty.Users)
	require.Zero(t, empty.TotalAmount)
	require.Zero(t, empty.TotalUsers)
	require.Zero(t, empty.TotalCount)
}
