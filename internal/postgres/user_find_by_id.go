package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/user"
	"gorm.io/gorm"
)

func (r *UserRepository) FindByID(
	ctx context.Context,
	id uuid.UUID,
) (user.User, error) {
	var record userModel

	err := r.db.
		WithContext(ctx).
		Where("id = ?", id).
		First(&record).
		Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return user.User{}, user.ErrUserNotFound
		}

		return user.User{}, fmt.Errorf(
			"consultar usuário por ID: %w",
			err,
		)
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
