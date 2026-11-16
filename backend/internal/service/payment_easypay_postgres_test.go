//go:build unit

package service

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/payment"
	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
)

// Explicitly opt in with a disposable local PostgreSQL instance, never production.
func TestEasyPayPostgresConcurrentTradeClaim(t *testing.T) {
	dsn := os.Getenv("EASYPAY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires disposable local PostgreSQL")
	}
	require.Contains(t, dsn, "host=127.0.0.1")
	require.Contains(t, dsn, "dbname=easypay_security_test")
	ctx := context.Background()
	admin, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	defer admin.Close()
	schema := fmt.Sprintf("easypay_test_%d", time.Now().UnixNano())
	_, err = admin.ExecContext(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)
	db, err := sql.Open("postgres", dsn+" search_path="+schema)
	require.NoError(t, err)
	defer db.Close()
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	defer client.Close()
	require.NoError(t, client.Schema.Create(ctx))
	u, err := client.User.Create().SetEmail("pg@example.test").SetPasswordHash("hash").Save(ctx)
	require.NoError(t, err)
	orders := make([]*dbent.PaymentOrder, 2)
	for i := range orders {
		orders[i], err = client.PaymentOrder.Create().SetUserID(u.ID).SetUserEmail(u.Email).SetUserName("test").SetAmount(10).SetPayAmount(10).SetFeeRate(0).SetRechargeCode(fmt.Sprintf("PG%d", i)).SetOutTradeNo(fmt.Sprintf("sub2_pg%d", i)).SetPaymentType(payment.TypeEasyPay).SetPaymentTradeNo("").SetOrderType(payment.OrderTypeBalance).SetStatus(OrderStatusPending).SetExpiresAt(time.Now().Add(time.Hour)).SetClientIP("127.0.0.1").SetSrcHost("test.invalid").Save(ctx)
		require.NoError(t, err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, o := range orders {
		wg.Add(1)
		go func(o *dbent.PaymentOrder) {
			defer wg.Done()
			<-start
			s := &PaymentService{entClient: client}
			_, err := s.easyPayPaidTransition(ctx, o, "same-real-trade", 10, func(tx *dbent.Client) (int, error) {
				time.Sleep(30 * time.Millisecond)
				_, err := tx.PaymentOrder.UpdateOneID(o.ID).SetStatus(OrderStatusPaid).SetPaymentTradeNo("same-real-trade").Save(ctx)
				return 1, err
			})
			results <- err
		}(o)
	}
	close(start)
	wg.Wait()
	close(results)
	winners := 0
	for err := range results {
		if err == nil {
			winners++
		} else {
			require.True(t, strings.Contains(err.Error(), "already bound"), err.Error())
		}
	}
	require.Equal(t, 1, winners)
	// A failed state update must release its DB lock and leave no transaction number.
	s := &PaymentService{entClient: client}
	_, err = s.easyPayPaidTransition(ctx, orders[0], "rolled-back-trade", 10, func(tx *dbent.Client) (int, error) {
		_, e := tx.PaymentOrder.UpdateOneID(orders[0].ID).SetPaymentTradeNo("rolled-back-trade").Save(ctx)
		if e != nil {
			return 0, e
		}
		return 0, fmt.Errorf("test rollback")
	})
	require.Error(t, err)
	got, err := client.PaymentOrder.Get(ctx, orders[0].ID)
	require.NoError(t, err)
	require.NotEqual(t, "rolled-back-trade", got.PaymentTradeNo)
}
