package postgres

import (
	"time"

	"github.com/google/uuid"
)

type transactionModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID `gorm:"type:uuid"`
	AmountMinor    int64
	NecessityLevel int
	CreatedAt      time.Time `gorm:"autoCreateTime:false"`
}

func (transactionModel) TableName() string {
	return "transactions"
}
