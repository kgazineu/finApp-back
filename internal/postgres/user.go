package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kgazineu/finApp-back/internal/user"
	"gorm.io/gorm"
)

type UserRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

var _ user.Repository = (*UserRepository)(nil)

func (r *UserRepository) Create(
	ctx context.Context,
	u user.User,
) (user.User, error) {
	record := userModel{
		ID:           u.ID,
		Name:         u.Name,
		Email:        u.Email,
		PasswordHash: u.PasswordHash,
		CreatedAt:    u.CreatedAt,
		UpdatedAt:    u.UpdatedAt,
	}

	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		var pgErr *pgconn.PgError

		if errors.As(err, &pgErr) &&
			pgErr.Code == "23505" &&
			pgErr.ConstraintName == "users_email_unique" {
			return user.User{}, user.ErrEmailAlreadyExists
		}

		return user.User{}, fmt.Errorf("persistir usuário: %w", err)
	}

	return user.User{
		ID:           record.ID,
		Name:         record.Name,
		Email:        record.Email,
		PasswordHash: record.PasswordHash,
		CreatedAt:    record.CreatedAt,
		UpdatedAt:    record.UpdatedAt,
	}, nil
}
