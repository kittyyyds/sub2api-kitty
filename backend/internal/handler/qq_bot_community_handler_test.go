package handler

import (
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/Wei-Shaw/sub2api/internal/qqbot"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestQQCommunityGuidePublicReadOnlyAndSanitized(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	bot := qqbot.DefaultConfig()
	bot.Enabled, bot.AppID, bot.AppSecret = true, "private-app-id", "private-app-secret"
	bot.AllowedGroups, bot.APIKey = []string{"private-group-id"}, "private-api-key"
	raw, err := json.Marshal(bot)
	require.NoError(t, err)
	mock.ExpectBegin()
	mock.ExpectQuery("(?s)SELECT jsonb_build_object.*FROM settings").WithArgs("qq_bot_config").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(string(raw)))
	for _, key := range []string{"qq_bot_checkin_config", "qq_bot_lottery_config", "qq_bot_ranking_config:" + bot.AppID} {
		mock.ExpectQuery("SELECT value FROM settings").WithArgs(key).WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow(`{}`))
	}
	mock.ExpectCommit()
	r := gin.New()
	r.GET("/guide", NewQQBotHandler(qqbot.New(db, qqTestEncryptor{}, true, nil)).CommunityGuide)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/guide", nil))
	require.Equal(t, 200, w.Code)
	require.Equal(t, "no-store", w.Header().Get("Cache-Control"))
	for _, secret := range []string{bot.AppID, bot.AppSecret, bot.APIKey, bot.AllowedGroups[0]} {
		require.NotContains(t, w.Body.String(), secret)
	}
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestQQCommunityGuideFailureIsGenericAndRetryable(t *testing.T) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	mock.ExpectBegin().WillReturnError(errors.New("private database detail"))
	r := gin.New()
	r.GET("/guide", NewQQBotHandler(qqbot.New(db, qqTestEncryptor{}, true, nil)).CommunityGuide)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/guide", nil))
	require.Equal(t, 503, w.Code)
	require.NotContains(t, w.Body.String(), "private database detail")
	require.NoError(t, mock.ExpectationsWereMet())
}
