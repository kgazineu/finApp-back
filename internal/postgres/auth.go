package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/user"
	"gorm.io/gorm"
)

// sessionHashValue prevents GORM from expanding byte slices into multiple SQL bind parameters
// when the placeholder follows an opening parenthesis (e.g. INSERT ... VALUES (?)).
type sessionHashValue [32]byte

func (hash sessionHashValue) Value() (driver.Value, error) {
	return hash[:], nil
}

type AuthRepository struct {
	db *gorm.DB
}

var _ auth.Repository = (*AuthRepository)(nil)

func NewAuthRepository(db *gorm.DB) *AuthRepository {
	return &AuthRepository{db: db}
}

func (r *AuthRepository) FindByEmail(ctx context.Context, email string) (user.User, error) {
	var record userModel
	err := r.db.WithContext(ctx).Where("email = ?", email).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return user.User{}, user.ErrUserNotFound
	}
	if err != nil {
		return user.User{}, fmt.Errorf("find user by email: %w", err)
	}
	return user.User{
		ID: record.ID, Name: record.Name, Email: record.Email,
		PasswordHash: record.PasswordHash, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}, nil
}

func (r *AuthRepository) SaveSession(ctx context.Context, tokenHash [32]byte, userID uuid.UUID, expiresAt time.Time) error {
	err := r.db.WithContext(ctx).Exec(
		"INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)",
		sessionHashValue(tokenHash), userID, expiresAt,
	).Error
	if err != nil {
		return fmt.Errorf("save session: %w", err)
	}
	return nil
}

func (r *AuthRepository) FindSession(ctx context.Context, tokenHash [32]byte) (uuid.UUID, time.Time, error) {
	var userID uuid.UUID
	var expiresAt time.Time
	err := r.db.WithContext(ctx).Raw(
		"SELECT user_id, expires_at FROM sessions WHERE token_hash = ?", sessionHashValue(tokenHash),
	).Row().Scan(&userID, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return uuid.Nil, time.Time{}, auth.ErrInvalidSession
	}
	if err != nil {
		return uuid.Nil, time.Time{}, fmt.Errorf("find session: %w", err)
	}
	return userID, expiresAt, nil
}
