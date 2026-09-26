package main

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func migrationTestConfig() *config.Config {
	return &config.Config{
		Database: config.DatabaseConfig{
			Host: "db", Port: 5432, User: "sub2api", DBName: "sub2api", SSLMode: "disable",
		},
		Timezone: "UTC",
	}
}

func TestRunMigrationsOnlyWithSuccess(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()

	opened := false
	err = runMigrationsOnlyWith(
		func() (*config.Config, error) { return migrationTestConfig(), nil },
		func(driverName, dataSourceName string) (*sql.DB, error) {
			opened = true
			require.Equal(t, "postgres", driverName)
			require.Contains(t, dataSourceName, "host=db")
			require.Contains(t, dataSourceName, "TimeZone=UTC")
			return db, nil
		},
		func(ctx context.Context, gotDB *sql.DB) error {
			require.Same(t, db, gotDB)
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, time.Now().Add(10*time.Minute), deadline, 2*time.Second)
			return nil
		},
	)
	require.NoError(t, err)
	require.True(t, opened)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestRunMigrationsOnlyWithLoadFailure(t *testing.T) {
	wantErr := errors.New("invalid bootstrap config")
	err := runMigrationsOnlyWith(
		func() (*config.Config, error) { return nil, wantErr },
		func(string, string) (*sql.DB, error) {
			t.Fatal("database must not be opened after a config error")
			return nil, nil
		},
		func(context.Context, *sql.DB) error {
			t.Fatal("migrations must not run after a config error")
			return nil
		},
	)
	require.ErrorIs(t, err, wantErr)
	require.ErrorContains(t, err, "load bootstrap config")
}

func TestRunMigrationsOnlyWithEmptyConfig(t *testing.T) {
	err := runMigrationsOnlyWith(
		func() (*config.Config, error) { return nil, nil },
		func(string, string) (*sql.DB, error) {
			t.Fatal("database must not be opened for an empty config")
			return nil, nil
		},
		func(context.Context, *sql.DB) error { return nil },
	)
	require.EqualError(t, err, "load bootstrap config: empty config")
}

func TestRunMigrationsOnlyWithOpenFailure(t *testing.T) {
	wantErr := errors.New("driver unavailable")
	err := runMigrationsOnlyWith(
		func() (*config.Config, error) { return migrationTestConfig(), nil },
		func(string, string) (*sql.DB, error) { return nil, wantErr },
		func(context.Context, *sql.DB) error {
			t.Fatal("migrations must not run after an open error")
			return nil
		},
	)
	require.ErrorIs(t, err, wantErr)
	require.ErrorContains(t, err, "open database")
}

func TestRunMigrationsOnlyWithApplyFailure(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	mock.ExpectClose()
	wantErr := errors.New("migration checksum mismatch")

	err = runMigrationsOnlyWith(
		func() (*config.Config, error) { return migrationTestConfig(), nil },
		func(string, string) (*sql.DB, error) { return db, nil },
		func(context.Context, *sql.DB) error { return wantErr },
	)
	require.ErrorIs(t, err, wantErr)
	require.ErrorContains(t, err, "apply migrations")
	require.NoError(t, mock.ExpectationsWereMet())
}
