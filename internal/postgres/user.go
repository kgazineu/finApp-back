package postgres

import (
	"context"
	"errors"

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
	return user.User{}, errors.New("persistência não implementada")
}
