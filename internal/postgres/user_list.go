package postgres

import (
	"context"
	"fmt"

	"github.com/kgazineu/finApp-back/internal/user"
)

func (r *UserRepository) List(ctx context.Context, input user.ListInput) ([]user.User, error) {
	var records []userModel
	err := r.db.WithContext(ctx).
		Select("id", "name", "email", "created_at", "updated_at").
		Order("created_at ASC").Order("id ASC").
		Limit(input.Limit).Offset(input.Offset).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("consultar usuários: %w", err)
	}

	users := make([]user.User, 0, len(records))
	for _, record := range records {
		users = append(users, user.User{
			ID: record.ID, Name: record.Name, Email: record.Email,
			CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
		})
	}
	return users, nil
}
