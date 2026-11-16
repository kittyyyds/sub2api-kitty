//go:build integration

package repository

import (
	"context"
	"log"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	redisclient "github.com/redis/go-redis/v9"
)

// runWithExternalServices runs the suite against a PostgreSQL (and Redis) that
// already exist instead of starting containers. It is for hosts where the test
// binary must not be handed the Docker daemon. The database must be
// disposable: the migration chain is applied to it and tests write to it.
func runWithExternalServices(ctx context.Context, m *testing.M, dsn, redisAddr string) int {
	var err error
	integrationDatabaseDSN = dsn
	if integrationDB, err = openSQLWithRetry(ctx, dsn, 30*time.Second); err != nil {
		log.Printf("failed to open the supplied test database: %v", err)
		return 1
	}
	defer func() { _ = integrationDB.Close() }()
	if err = ApplyMigrations(ctx, integrationDB); err != nil {
		log.Printf("failed to apply db migrations: %v", err)
		return 1
	}
	integrationEntClient = dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, integrationDB)))
	if redisAddr != "" {
		integrationRedis = redisclient.NewClient(&redisclient.Options{Addr: redisAddr})
		defer func() { _ = integrationRedis.Close() }()
		if err = integrationRedis.Ping(ctx).Err(); err != nil {
			log.Printf("failed to ping the supplied test redis: %v", err)
			return 1
		}
	}
	return m.Run()
}
