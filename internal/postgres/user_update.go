package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/kgazineu/finApp-back/internal/user"
)

func (r *UserRepository) Update(
	ctx context.Context,
	u user.User,
) (user.User, error) {
	record := userModel{
		Name:      u.Name,
		Email:     u.Email,
		UpdatedAt: u.UpdatedAt,
	}

	result := r.db.
		WithContext(ctx).
		Model(&userModel{}).
		Where("id = ?", u.ID).
		Select("name", "email", "updated_at").
		Updates(&record)

	if result.Error != nil {
		var pgErr *pgconn.PgError

		if errors.As(result.Error, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_unique" {
			return user.User{}, user.ErrEmailAlreadyExists
		}

		return user.User{}, fmt.Errorf(
			"persistir atualização do usuário: %w",
			result.Error,
		)
	}

	if result.RowsAffected == 0 {
		return user.User{}, user.ErrUserNotFound
	}

	return u, nil
}
