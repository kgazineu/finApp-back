package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/overview"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"gorm.io/gorm"
)

type OverviewRepository struct{ db *gorm.DB }

func NewOverviewRepository(db *gorm.DB) *OverviewRepository { return &OverviewRepository{db: db} }

var _ overview.Repository = (*OverviewRepository)(nil)

func (r *OverviewRepository) Fetch(ctx context.Context, userID uuid.UUID, start, end time.Time) (overview.Snapshot, error) {
	result := overview.Snapshot{Month: start.UTC(), RecentTransactions: []transaction.Transaction{}, ExpensesByCategory: []overview.CategoryTotal{}}
	// PostgreSQL SUM(bigint) produces numeric. Casting to bigint fails rather than
	// silently losing precision when the aggregate exceeds Snapshot's int64 fields.
	err := r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(amount_minor) FILTER (WHERE kind = 'income'), 0)::bigint,
		       COALESCE(SUM(amount_minor) FILTER (WHERE kind = 'expense'), 0)::bigint
		FROM transactions WHERE user_id = ? AND occurred_at >= ? AND occurred_at < ?`,
		userID, start, end).Row().Scan(&result.IncomeMinor, &result.ExpenseMinor)
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("aggregate monthly transactions: %w", err)
	}
	err = r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(CASE WHEN kind = 'income' THEN amount_minor ELSE -amount_minor END), 0)::bigint
		FROM transactions WHERE user_id = ?`, userID).Row().Scan(&result.NetTrackedMinor)
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("aggregate tracked net: %w", err)
	}
	err = r.db.WithContext(ctx).Raw(`
		SELECT COALESCE(SUM(saved_minor), 0)::bigint, COALESCE(SUM(target_minor), 0)::bigint
		FROM goals WHERE user_id = ?`, userID).Row().Scan(&result.GoalsSavedMinor, &result.GoalsTargetMinor)
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("aggregate goals: %w", err)
	}

	rows, err := r.db.WithContext(ctx).Raw(`
		SELECT category, SUM(amount_minor)::bigint AS total_minor
		FROM transactions
		WHERE user_id = ? AND kind = 'expense' AND occurred_at >= ? AND occurred_at < ?
		GROUP BY category ORDER BY total_minor DESC, category ASC LIMIT 10`, userID, start, end).Rows()
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("query expense categories: %w", err)
	}
	for rows.Next() {
		var item overview.CategoryTotal
		if err := rows.Scan(&item.Category, &item.TotalMinor); err != nil {
			rows.Close()
			return overview.Snapshot{}, fmt.Errorf("scan expense category: %w", err)
		}
		result.ExpensesByCategory = append(result.ExpensesByCategory, item)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("iterate expense categories: %w", err)
	}

	var records []transactionModel
	err = r.db.WithContext(ctx).Where("user_id = ? AND occurred_at >= ? AND occurred_at < ?", userID, start, end).
		Order("occurred_at DESC").Order("id DESC").Limit(5).Find(&records).Error
	if err != nil {
		return overview.Snapshot{}, fmt.Errorf("query recent transactions: %w", err)
	}
	for _, record := range records {
		result.RecentTransactions = append(result.RecentTransactions, record.transaction())
	}
	return result, nil
}
