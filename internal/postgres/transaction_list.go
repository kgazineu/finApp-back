package postgres

import (
	"context"
	"fmt"

	"github.com/kgazineu/finApp-back/internal/transaction"
)

func (r *TransactionRepository) List(ctx context.Context, input transaction.ListInput) ([]transaction.Transaction, error) {
	var records []transactionModel
	err := r.db.WithContext(ctx).
		Where("user_id = ?", input.UserID).
		Order("occurred_at DESC").Order("id DESC").
		Limit(input.Limit).Offset(input.Offset).
		Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("consultar transações: %w", err)
	}
	items := make([]transaction.Transaction, 0, len(records))
	for _, record := range records {
		items = append(items, record.transaction())
	}
	return items, nil
}
