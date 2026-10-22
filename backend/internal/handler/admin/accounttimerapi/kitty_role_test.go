package accounttimerapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKittySuperAdminCanReadTimers(t *testing.T) {
	repo := &fakeRepo{}
	w := httptest.NewRecorder()
	router(repo, "super_admin", 1).ServeHTTP(w, httptest.NewRequest("GET", "/accounts/7/timers", nil))
	if w.Code != http.StatusOK || repo.calls != 1 || repo.id != 7 {
		t.Fatalf("status=%d calls=%d account=%d", w.Code, repo.calls, repo.id)
	}
}
