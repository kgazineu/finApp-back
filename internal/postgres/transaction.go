package postgres

import (
	"context"
	"fmt"

	"github.com/kgazineu/finApp-back/internal/transaction"
	"gorm.io/gorm"
)

type TransactionRepository struct {
	db *gorm.DB
}

func NewTransactionRepository(db *gorm.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

var _ transaction.Repository = (*TransactionRepository)(nil)

func (r *TransactionRepository) Create(ctx context.Context, tx transaction.Transaction) (transaction.Transaction, error) {
	record := transactionModel{
		ID:             tx.ID,
		UserID:         tx.UserID,
		AmountMinor:    tx.AmountMinor,
		NecessityLevel: tx.NecessityLevel,
		CreatedAt:      tx.CreatedAt,
	}
	if err := r.db.WithContext(ctx).Create(&record).Error; err != nil {
		return transaction.Transaction{}, fmt.Errorf("persistir transação: %w", err)
	}
	return transaction.Transaction{
		ID:             record.ID,
		UserID:         record.UserID,
		AmountMinor:    record.AmountMinor,
		NecessityLevel: record.NecessityLevel,
		CreatedAt:      record.CreatedAt,
	}, nil
}
