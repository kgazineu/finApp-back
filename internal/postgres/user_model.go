package postgres

import (
	"time"

	"github.com/google/uuid"
)

type userModel struct {
	ID           uuid.UUID `gorm:"type:uuid;primaryKey"`
	Name         string
	Email        string
	PasswordHash string
	CreatedAt    time.Time `gorm:"autoCreateTime:false"`
	UpdatedAt    time.Time `gorm:"autoUpdateTime:false"`
}

func (userModel) TableName() string {
	return "users"
}
