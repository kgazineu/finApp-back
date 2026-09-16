package user

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type UpdateInput struct {
	ID    uuid.UUID
	Name  *string
	Email *string
}

func (s *Service) Update(
	ctx context.Context,
	input UpdateInput,
) (User, error) {
	if err := ctx.Err(); err != nil {
		return User{}, err
	}

	if input.ID == uuid.Nil {
		return User{}, ErrInvalidUserID
	}

	if input.Name == nil && input.Email == nil {
		return User{}, ErrNoFieldsToUpdate
	}

	current, err := s.repo.FindByID(ctx, input.ID)
	if err != nil {
		return User{}, fmt.Errorf("buscar usuário: %w", err)
	}

	name := current.Name
	email := current.Email

	if input.Name != nil {
		name = *input.Name
	}

	if input.Email != nil {
		email = *input.Email
	}

	if err := validateProfile(name, email); err != nil {
		return User{}, err
	}

	if err := ctx.Err(); err != nil {
		return User{}, err
	}

	current.Name = name
	current.Email = email
	current.UpdatedAt = time.Now().UTC()

	updated, err := s.repo.Update(ctx, current)
	if err != nil {
		return User{}, fmt.Errorf("atualizar usuário: %w", err)
	}

	return updated, nil
}
