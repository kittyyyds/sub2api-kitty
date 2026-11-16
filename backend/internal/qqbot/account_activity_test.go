package qqbot

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/require"
)

func TestPersonalInfoMetricFailureDoesNotHideProfileOrBecomeZero(t *testing.T) {
	for _, failure := range []bool{false, true} {
		db, mock, err := sqlmock.New()
		require.NoError(t, err)
		c := accountQueryConfig()
		expectAccountQueryConfig(t, mock, c)
		mock.ExpectQuery("SELECT b.user_id FROM qq_bot_bindings").WithArgs(c.AppID, "member").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(42))
		mock.ExpectQuery("SELECT username,balance,concurrency FROM users").WithArgs(int64(42)).WillReturnRows(sqlmock.NewRows([]string{"username", "balance", "concurrency"}).AddRow("Kitty", 12.5, 4))
		q := mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM usage_logs").WithArgs(int64(42), sqlmock.AnyArg(), sqlmock.AnyArg())
		if failure {
			q.WillReturnError(errors.New("unavailable"))
		} else {
			q.WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
		}
		s := New(db, nil, false, nil)
		s.SetUserConcurrencyReader(func(_ context.Context, id int64, limit int) (*int, error) {
			require.Equal(t, int64(42), id)
			require.Equal(t, 4, limit)
			if failure {
				return nil, errors.New("unavailable")
			}
			n := 0
			return &n, nil
		})
		text, err := s.personalInfo(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "member"}, time.Now())
		require.NoError(t, err)
		require.Contains(t, text, "12.500000")
		require.Contains(t, text, "用户 ID：42")
		if failure {
			require.Contains(t, text, "当前并发：暂不可用")
			require.Contains(t, text, "当前 RPM（近60秒）：暂不可用")
		} else {
			require.Contains(t, text, "当前并发：0")
			require.Contains(t, text, "当前 RPM（近60秒）：0")
		}
		require.NoError(t, mock.ExpectationsWereMet())
		db.Close()
	}
}

func TestPersonalInfoRejectsForgedOrDisabledIdentityBeforeDataRead(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := accountQueryConfig()
	_, err = s.personalInfo(context.Background(), c, Job{AppID: "another-app", GroupID: "group", MemberID: "member"}, time.Now())
	require.ErrorIs(t, err, ErrDisabled)
	current := c
	current.Enabled = false
	expectAccountQueryConfig(t, mock, current)
	_, err = s.personalInfo(context.Background(), c, Job{AppID: c.AppID, GroupID: "group", MemberID: "member"}, time.Now())
	require.ErrorIs(t, err, ErrDisabled)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestKeyUsageArgumentsAreBoundedAndUseBeijingDay(t *testing.T) {
	now := time.Date(2026, 11, 11, 17, 0, 0, 0, time.UTC)
	id, start, err := keyUsageArgs("7", now)
	require.NoError(t, err)
	require.Equal(t, int64(7), id)
	require.Equal(t, "2026-11-12T00:00:00+08:00", start.Format(time.RFC3339))
	for _, arg := range []string{"", "0", "-1", "1 2026-11-13", "1 2026-01-01", "1 2026-02-30", "1 2026-11-11 extra", "1;SELECT"} {
		_, _, err = keyUsageArgs(arg, now)
		require.Error(t, err, arg)
	}
}

func TestPrivateIdentityAndUnknownMessageNeverEnterGroupOrAIQueue(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	defer db.Close()
	s := New(db, nil, false, nil)
	c := accountQueryConfig()
	for _, raw := range []string{
		`{"id":"x","content":"个人信息","author":{"member_openid":"member"}}`,
		`{"id":"x","content":"生图 a cat","author":{"user_openid":"member"}}`,
		`{"id":"x","content":"转账 100 other@example.invalid","author":{"user_openid":"member"}}`,
	} {
		require.NoError(t, s.Enqueue(context.Background(), c, Payload{Type: privateMessageEvent, Data: []byte(raw)}))
	}
	require.NoError(t, mock.ExpectationsWereMet())
	require.False(t, isPrivateJob(Job{EventType: groupAtMessageCreate, GroupID: privateJobGroup, MemberID: "member"}))
}

func TestBalanceAlertBackoffIsBounded(t *testing.T) {
	require.Equal(t, time.Minute, balanceAlertRetry(1))
	require.Equal(t, 2*time.Minute, balanceAlertRetry(2))
	require.Equal(t, time.Hour, balanceAlertRetry(1000))
}
