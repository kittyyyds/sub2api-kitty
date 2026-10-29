//go:build media_postgres

package repository

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetainedMediaUploadAndDownload(t *testing.T) {
	db, r, _ := retainedFixture(t)
	ctx := context.Background()
	t.Setenv("KITTY_MEDIA_STORAGE_DIR", t.TempDir())
	t.Setenv("KITTY_MEDIA_STORAGE_CLEANUP", "")
	s, e := service.NewRetainedMediaService(r)
	require.NoError(t, e)
	defer s.Stop()
	mp4 := append([]byte{0, 0, 0, 24}, []byte("ftypmp42\x00\x00\x00\x00mp42isom")...)
	a, e := s.Upload(ctx, 1, "../video.mp4", bytes.NewReader(mp4))
	require.NoError(t, e)
	require.Equal(t, "video", a.Kind)
	require.Equal(t, "video.mp4", a.Name)
	require.Equal(t, 24*time.Hour, a.ExpiresAt.Sub(a.CreatedAt))
	replay, e := s.Upload(ctx, 1, "renamed.mp4", bytes.NewReader(mp4))
	require.NoError(t, e)
	require.Equal(t, a, replay)
	other, e := s.Upload(ctx, 2, "video.mp4", bytes.NewReader(mp4))
	require.NoError(t, e)
	require.NotEqual(t, a.ID, other.ID)
	_, f, e := s.Open(ctx, 1, a.ID)
	require.NoError(t, e)
	require.NoError(t, f.Close())
	_, e = s.Upload(ctx, 1, "not-video.mp4", bytes.NewBufferString("this is not a video"))
	require.ErrorIs(t, e, service.ErrStorageInvalid)
	_, e = db.Exec(`UPDATE kitty_retained_media SET expires_at=clock_timestamp()-INTERVAL '1 second' WHERE id=$1`, a.ID)
	require.NoError(t, e)
	_, e = s.Upload(ctx, 1, "same.mp4", bytes.NewReader(mp4))
	require.ErrorIs(t, e, service.ErrStorageExpired)
	_, _, e = s.Open(ctx, 1, a.ID)
	require.ErrorIs(t, e, service.ErrStorageExpired)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
}

func retainedFixture(t *testing.T) (*sql.DB, service.RetainedMediaRepository, *service.RetainedMedia) {
	db, _ := newMediaWalletFixture(t)
	raw, e := os.ReadFile("../../migrations/298_kitty_retained_media.sql")
	require.NoError(t, e)
	_, e = db.Exec(string(raw))
	require.NoError(t, e)
	r := NewRetainedMediaRepository(db)
	a, e := r.Create(context.Background(), &service.RetainedMedia{ID: uuid.NewString(), UserID: 1, SourceKey: "fixture", SHA256: "fixture", Kind: "video", Name: "test.mp4", MimeType: "video/mp4", Bytes: 24})
	require.NoError(t, e)
	require.Equal(t, 24*time.Hour, a.ExpiresAt.Sub(a.CreatedAt))
	return db, r, a
}
func TestRetainedMediaPricesIdempotencyAndOwnership(t *testing.T) {
	db, r, a := retainedFixture(t)
	ctx := context.Background()
	cmd := service.StorageExtension{RequestID: uuid.NewString(), Days: 3, ExpectedExpiry: a.ExpiresAt}
	_, e := r.Extend(ctx, 2, a.ID, cmd)
	require.ErrorIs(t, e, sql.ErrNoRows)
	o, e := r.Extend(ctx, 1, a.ID, cmd)
	require.NoError(t, e)
	require.Equal(t, "0.30000000", o.Amount)
	require.True(t, o.After.Equal(a.ExpiresAt.Add(72*time.Hour)))
	assertMediaBalance(t, db, "9.70000000", "0.00000000")
	for range 4 {
		replay, e := r.Extend(ctx, 1, a.ID, cmd)
		require.NoError(t, e)
		require.Equal(t, o, replay)
	}
	cmd.Days = 4
	_, e = r.Extend(ctx, 1, a.ID, cmd)
	require.ErrorIs(t, e, service.ErrStorageChanged)
	assertMediaBalance(t, db, "9.70000000", "0.00000000")
	a2, e := r.Create(ctx, &service.RetainedMedia{ID: uuid.NewString(), UserID: 1, SourceKey: "image", SHA256: "image", Kind: "image", Name: "test.png", MimeType: "image/png", Bytes: 30})
	require.NoError(t, e)
	require.Equal(t, 4*time.Hour, a2.ExpiresAt.Sub(a2.CreatedAt))
	imageReplay, e := r.Create(ctx, &service.RetainedMedia{ID: uuid.NewString(), UserID: 1, SourceKey: "image", SHA256: "image", Kind: "image", Name: "renamed.png", MimeType: "image/png", Bytes: 30})
	require.NoError(t, e)
	require.Equal(t, a2.ID, imageReplay.ID)
	require.Equal(t, a2.ExpiresAt, imageReplay.ExpiresAt)
	o, e = r.Extend(ctx, 1, a2.ID, service.StorageExtension{RequestID: uuid.NewString(), Days: 5, ExpectedExpiry: a2.ExpiresAt})
	require.NoError(t, e)
	require.Equal(t, "0.05000000", o.Amount)
	require.True(t, o.After.Equal(a2.ExpiresAt.Add(5*24*time.Hour)))
	assertMediaBalance(t, db, "9.65000000", "0.00000000")
	list, e := r.Orders(ctx, 2, 0)
	require.NoError(t, e)
	require.Empty(t, list)
}
func TestRetainedMediaConcurrentOneCharge(t *testing.T) {
	db, r, a := retainedFixture(t)
	var wg sync.WaitGroup
	var ok atomic.Int32
	for range 12 {
		wg.Go(func() {
			_, e := r.Extend(context.Background(), 1, a.ID, service.StorageExtension{RequestID: uuid.NewString(), Days: 1, ExpectedExpiry: a.ExpiresAt})
			if e == nil {
				ok.Add(1)
			} else {
				require.ErrorIs(t, e, service.ErrStorageChanged)
			}
		})
	}
	wg.Wait()
	require.EqualValues(t, 1, ok.Load())
	assertMediaBalance(t, db, "9.90000000", "0.00000000")
}
func TestRetainedMediaFailureRollback(t *testing.T) {
	for _, scenario := range []string{"expired", "insufficient", "insert_failure", "cross_expiry"} {
		t.Run(scenario, func(t *testing.T) {
			db, r, a := retainedFixture(t)
			ctx := context.Background()
			expected := "10.00000000"
			switch scenario {
			case "expired":
				_, e := db.Exec(`UPDATE kitty_retained_media SET expires_at=clock_timestamp()-INTERVAL '1 second'`)
				require.NoError(t, e)
			case "insufficient":
				_, e := db.Exec(`UPDATE users SET balance=0 WHERE id=1`)
				require.NoError(t, e)
				expected = "0.00000000"
			case "insert_failure":
				_, e := db.Exec(`ALTER TABLE kitty_media_storage_orders ADD CONSTRAINT forced_failure CHECK(days=999)`)
				require.NoError(t, e)
			case "cross_expiry":
				_, e := db.Exec(`UPDATE kitty_retained_media SET expires_at=clock_timestamp()+INTERVAL '200 milliseconds'; CREATE FUNCTION slow_storage() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN PERFORM pg_sleep(0.3); RETURN NEW; END $$; CREATE TRIGGER slow_storage BEFORE INSERT ON kitty_media_wallet_orders FOR EACH ROW EXECUTE FUNCTION slow_storage()`)
				require.NoError(t, e)
			}
			a, e := r.Get(ctx, 1, a.ID)
			require.NoError(t, e)
			_, e = r.Extend(ctx, 1, a.ID, service.StorageExtension{RequestID: uuid.NewString(), Days: 1, ExpectedExpiry: a.ExpiresAt})
			require.Error(t, e)
			assertMediaBalance(t, db, expected, "0.00000000")
			var n int
			require.NoError(t, db.QueryRow(`SELECT count(*) FROM kitty_media_wallet_orders`).Scan(&n))
			require.Zero(t, n)
			b, e := r.Get(ctx, 1, a.ID)
			require.NoError(t, e)
			require.True(t, b.ExpiresAt.Equal(a.ExpiresAt))
		})
	}
}
func TestRetainedMediaCleanupOnlyExpiredAndReceiptReplay(t *testing.T) {
	db, r, a := retainedFixture(t)
	ctx := context.Background()
	cmd := service.StorageExtension{RequestID: uuid.NewString(), Days: 1, ExpectedExpiry: a.ExpiresAt}
	o, e := r.Extend(ctx, 1, a.ID, cmd)
	require.NoError(t, e)
	n, e := r.Cleanup(ctx, func(string) error { t.Fatal("unexpired media must not be deleted"); return nil })
	require.NoError(t, e)
	require.Zero(t, n)
	_, e = db.Exec(`UPDATE kitty_retained_media SET expires_at=clock_timestamp()-INTERVAL '1 second'`)
	require.NoError(t, e)
	_, e = r.Cleanup(ctx, func(string) error { return fmt.Errorf("disk failure") })
	require.Error(t, e)
	got, e := r.Get(ctx, 1, a.ID)
	require.NoError(t, e)
	require.Equal(t, "active", got.Status)
	n, e = r.Cleanup(ctx, func(id string) error { require.Equal(t, a.ID, id); return nil })
	require.NoError(t, e)
	require.Equal(t, 1, n)
	n, e = r.Cleanup(ctx, func(string) error { t.Fatal("already deleted"); return nil })
	require.NoError(t, e)
	require.Zero(t, n)
	replay, e := r.Extend(ctx, 1, a.ID, cmd)
	require.NoError(t, e)
	require.Equal(t, o, replay)
	assertMediaBalance(t, db, "9.90000000", "0.00000000")
}
func TestRetainedMediaMissingFileDoesNotCharge(t *testing.T) {
	db, r, a := retainedFixture(t)
	t.Setenv("KITTY_MEDIA_STORAGE_DIR", t.TempDir())
	t.Setenv("KITTY_MEDIA_STORAGE_CLEANUP", "")
	s, e := service.NewRetainedMediaService(r)
	require.NoError(t, e)
	defer s.Stop()
	_, e = s.Extend(context.Background(), 1, a.ID, service.StorageExtension{RequestID: uuid.NewString(), Days: 1, ExpectedExpiry: a.ExpiresAt})
	require.ErrorIs(t, e, service.ErrStorageUnavailable)
	assertMediaBalance(t, db, "10.00000000", "0.00000000")
	require.NoError(t, os.WriteFile(filepath.Join(os.Getenv("KITTY_MEDIA_STORAGE_DIR"), a.ID+".blob"), make([]byte, 24), 0600))
	_, f, e := s.Open(context.Background(), 2, a.ID)
	require.True(t, errors.Is(e, sql.ErrNoRows))
	require.Nil(t, f)
}
