package passwordreset

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Upsert replaces the user's code. created is false when the e-mail has no account
// or the current code was issued less than a minute ago.
func (r *Repository) Upsert(ctx context.Context, email string, codeHash [32]byte, expiresAt time.Time) (bool, error) {
	var userID uuid.UUID
	err := r.db.QueryRowxContext(ctx, `
		insert into password_resets (user_id, code_hash, expires_at)
		select id, $2, $3 from users where email = $1
		on conflict (user_id) do update
			set code_hash = excluded.code_hash, expires_at = excluded.expires_at, attempts = 0, created_at = now()
			where password_resets.created_at < now() - interval '1 minute'
		returning user_id
	`, email, codeHash[:], expiresAt).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}

	return err == nil, err
}

// UseAttempt spends one attempt of a live code and returns its hash; nil hash means
// there is no usable code (unknown e-mail, expired or out of attempts).
func (r *Repository) UseAttempt(ctx context.Context, email string, maxAttempts int) (uuid.UUID, []byte, error) {
	var userID uuid.UUID
	var codeHash []byte
	err := r.db.QueryRowxContext(ctx, `
		update password_resets p set attempts = p.attempts + 1
		from users u
		where u.id = p.user_id and u.email = $1 and p.expires_at > now() and p.attempts < $2
		returning p.user_id, p.code_hash
	`, email, maxAttempts).Scan(&userID, &codeHash)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, nil, nil
	}

	return userID, codeHash, err
}

// ResetPassword swaps the hash, burns the code and ends every open session.
func (r *Repository) ResetPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `update users set password_hash = $1, updated_at = now() where id = $2`, passwordHash, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from password_resets where user_id = $1`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `delete from sessions where user_id = $1`, userID); err != nil {
		return err
	}

	return tx.Commit()
}
