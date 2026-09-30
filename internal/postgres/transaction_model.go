package postgres

import (
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/transaction"
)

type transactionModel struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID `gorm:"type:uuid"`
	AmountMinor    int64
	NecessityLevel *int
	Kind           string
	Description    string
	Category       string
	PaymentMethod  *string
	Installments   int
	OccurredAt     time.Time `gorm:"autoCreateTime:false"`
	CreatedAt      time.Time `gorm:"autoCreateTime:false"`
}

func (transactionModel) TableName() string {
	return "transactions"
}

func modelFromTransaction(tx transaction.Transaction) transactionModel {
	// Support callers that still construct the pre-enrichment Transaction directly.
	if tx.Kind == "" && tx.Description == "" && tx.Category == "" &&
		tx.PaymentMethod == "" && tx.Installments == 0 && tx.OccurredAt.IsZero() {
		tx.Kind = "expense"
		tx.Description = "Lançamento"
		tx.Category = "Outros"
		tx.Installments = 1
		tx.OccurredAt = tx.CreatedAt
	}
	record := transactionModel{
		ID: tx.ID, UserID: tx.UserID, AmountMinor: tx.AmountMinor,
		Kind: tx.Kind, Description: tx.Description, Category: tx.Category,
		Installments: tx.Installments, OccurredAt: tx.OccurredAt, CreatedAt: tx.CreatedAt,
	}
	if tx.Kind != "income" || tx.NecessityLevel != 0 {
		record.NecessityLevel = &tx.NecessityLevel
	}
	if tx.PaymentMethod != "" {
		record.PaymentMethod = &tx.PaymentMethod
	}
	return record
}

func (record transactionModel) transaction() transaction.Transaction {
	tx := transaction.Transaction{
		ID: record.ID, UserID: record.UserID, AmountMinor: record.AmountMinor,
		Kind: record.Kind, Description: record.Description, Category: record.Category,
		Installments: record.Installments, OccurredAt: record.OccurredAt, CreatedAt: record.CreatedAt,
	}
	if record.NecessityLevel != nil {
		tx.NecessityLevel = *record.NecessityLevel
	}
	if record.PaymentMethod != nil {
		tx.PaymentMethod = *record.PaymentMethod
	}
	return tx
}
