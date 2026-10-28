//go:build integration

package repository

import (
	"context"
	"database/sql"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"fmt"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/pkg/timezone"
	redisclient "github.com/redis/go-redis/v9"
	"net/url"
	"os"
	"testing"
	"time"
)

// Explicit, container-only rehearsal. Never reads production environment DSNs.
func kittyIsolatedBillingAuditMain(m *testing.M) int {
	dsn := os.Getenv("KITTY_ISOLATED_BILLING_DATABASE_URL")
	u, err := url.Parse(dsn)
	if err != nil || u.Host != "127.0.0.1:5432" || u.Path != "/kitty_v217_billing_test" || u.User == nil || u.User.Username() != "postgres" {
		fmt.Println("refusing non-isolated billing database")
		return 1
	}
	if err = timezone.Init("UTC"); err != nil {
		return 1
	}
	integrationPostgresDSN = dsn
	integrationDB, err = sql.Open("postgres", dsn)
	if err != nil {
		return 1
	}
	defer integrationDB.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err = ApplyMigrations(ctx, integrationDB); err != nil {
		fmt.Println(err)
		return 1
	}
	integrationEntClient = dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, integrationDB)))
	defer integrationEntClient.Close()
	integrationRedis = redisclient.NewClient(&redisclient.Options{Addr: "127.0.0.1:6379", DB: 0})
	defer integrationRedis.Close()
	if err = integrationRedis.Ping(ctx).Err(); err != nil {
		fmt.Println(err)
		return 1
	}
	return m.Run()
}
