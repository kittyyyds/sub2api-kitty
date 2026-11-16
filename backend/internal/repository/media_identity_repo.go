package repository

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"regexp"

	"github.com/Wei-Shaw/sub2api/internal/service"
)

type mediaWalletIdentityRepository struct{ db *sql.DB }

func NewMediaWalletIdentityRepository(db *sql.DB) service.MediaWalletIdentityRepository {
	return &mediaWalletIdentityRepository{db: db}
}

var mediaSubject = regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`)

func (r *mediaWalletIdentityRepository) EnsureSubject(ctx context.Context, verifiedUserID int64) (string, error) {
	if verifiedUserID <= 0 {
		return "", service.ErrMediaWalletSubjectNotFound
	}
	var random [32]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", err
	}
	candidate := base64.RawURLEncoding.EncodeToString(random[:])
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id=$1 AND status='active' AND deleted_at IS NULL FOR UPDATE`, verifiedUserID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", service.ErrMediaWalletSubjectNotFound
	}
	if err != nil {
		return "", err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO kitty_media_identities(user_id,subject) VALUES($1,$2) ON CONFLICT(user_id) DO NOTHING`, id, candidate)
	if err != nil {
		return "", err
	}
	var subject string
	err = tx.QueryRowContext(ctx, `SELECT subject FROM kitty_media_identities WHERE user_id=$1`, id).Scan(&subject)
	if err != nil {
		return "", err
	}
	if err = tx.Commit(); err != nil {
		return "", err
	}
	return subject, nil
}

func (r *mediaWalletIdentityRepository) ResolveSubject(ctx context.Context, subject string) (int64, error) {
	if !mediaSubject.MatchString(subject) {
		return 0, service.ErrMediaWalletSubjectNotFound
	}
	var id int64
	err := r.db.QueryRowContext(ctx, `SELECT user_id FROM kitty_media_identities WHERE subject=$1`, subject).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, service.ErrMediaWalletSubjectNotFound
	}
	return id, err
}
