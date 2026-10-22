package accounttimer

import (
	"context"
	"database/sql"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

func TestTimedOpenCannotBypassAdmissionGate(t *testing.T) {
	s, m := testStore(t)
	called := false
	s.CheckOpening = func(_ context.Context, tx *sql.Tx, id int64) (string, error) {
		if tx == nil || id != 7 {
			t.Fatal("gate must run under the locked account transaction")
		}
		called = true
		return "admission_pending", nil
	}
	s.AfterCommit = func(context.Context, int64) { t.Fatal("blocked account must not be published as schedulable") }
	now := s.now()
	task := Task{AccountID: 7, Action: "open", Version: 1, ExecuteAt: now, Timezone: "UTC", RequestedBy: 9, Status: "pending"}
	m.ExpectBegin()
	expectAccount(m, "active", false)
	m.ExpectQuery("SELECT "+regexp.QuoteMeta(taskColumns)).WithArgs(int64(7), "open", now).WillReturnRows(taskRows().AddRow(7, "open", 1, now, "UTC", 9, "pending", 0, nil, ""))
	// No accounts UPDATE is expected: a timer cannot release this account.
	m.ExpectExec("UPDATE account_schedulable_timers").WithArgs(int64(7), "open", "skipped", now, "admission_pending", int64(1)).WillReturnResult(sqlmock.NewResult(0, 1))
	m.ExpectExec("INSERT INTO account_schedulable_timer_audit").WithArgs(int64(7), "open", int64(1), int64(9), now, "UTC", now, "skipped", "admission_pending").WillReturnResult(sqlmock.NewResult(1, 1))
	m.ExpectCommit()
	if err := s.execute(context.Background(), task); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("admission gate was bypassed")
	}
}
