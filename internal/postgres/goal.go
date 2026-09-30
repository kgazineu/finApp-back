package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/kgazineu/finApp-back/internal/goal"
	"gorm.io/gorm"
)

type goalModel struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey"`
	UserID      uuid.UUID `gorm:"type:uuid"`
	Name        string
	TargetMinor int64
	SavedMinor  int64
	CreatedAt   time.Time `gorm:"autoCreateTime:false"`
	UpdatedAt   time.Time `gorm:"autoUpdateTime:false"`
}

func (goalModel) TableName() string { return "goals" }

func (record goalModel) domain() goal.Goal {
	return goal.Goal{
		ID: record.ID, UserID: record.UserID, Name: record.Name,
		TargetMinor: record.TargetMinor, SavedMinor: record.SavedMinor,
		CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt,
	}
}

type GoalRepository struct{ db *gorm.DB }

func NewGoalRepository(db *gorm.DB) *GoalRepository { return &GoalRepository{db: db} }

var _ goal.Repository = (*GoalRepository)(nil)

func (r *GoalRepository) Create(ctx context.Context, item goal.Goal) (goal.Goal, error) {
	var record goalModel
	result := r.db.WithContext(ctx).Raw(`
		INSERT INTO goals (id, user_id, name, target_minor)
		VALUES (?, ?, ?, ?)
		RETURNING id, user_id, name, target_minor, saved_minor, created_at, updated_at`,
		item.ID, item.UserID, item.Name, item.TargetMinor).Scan(&record)
	if result.Error != nil {
		return goal.Goal{}, fmt.Errorf("insert goal: %w", result.Error)
	}
	return record.domain(), nil
}

func (r *GoalRepository) List(ctx context.Context, input goal.ListInput) ([]goal.Goal, error) {
	var records []goalModel
	err := r.db.WithContext(ctx).Where("user_id = ?", input.UserID).
		Order("created_at DESC").Order("id DESC").
		Limit(input.Limit).Offset(input.Offset).Find(&records).Error
	if err != nil {
		return nil, fmt.Errorf("query goals: %w", err)
	}
	items := make([]goal.Goal, 0, len(records))
	for _, record := range records {
		items = append(items, record.domain())
	}
	return items, nil
}

func (r *GoalRepository) Deposit(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (goal.Goal, error) {
	return r.adjust(ctx, userID, id, amountMinor, true)
}

func (r *GoalRepository) Withdraw(ctx context.Context, userID, id uuid.UUID, amountMinor int64) (goal.Goal, error) {
	return r.adjust(ctx, userID, id, amountMinor, false)
}

func (r *GoalRepository) adjust(ctx context.Context, userID, id uuid.UUID, amountMinor int64, deposit bool) (goal.Goal, error) {
	if err := ctx.Err(); err != nil {
		return goal.Goal{}, err
	}
	if amountMinor <= 0 {
		return goal.Goal{}, goal.ErrInvalidAmount
	}
	var record goalModel
	var query string
	if deposit {
		query = `UPDATE goals SET saved_minor = saved_minor + ?, updated_at = NOW()
			WHERE id = ? AND user_id = ? AND saved_minor <= 9223372036854775807 - ?
			RETURNING id, user_id, name, target_minor, saved_minor, created_at, updated_at`
	} else {
		query = `UPDATE goals SET saved_minor = saved_minor - ?, updated_at = NOW()
			WHERE id = ? AND user_id = ? AND saved_minor >= ?
			RETURNING id, user_id, name, target_minor, saved_minor, created_at, updated_at`
	}
	result := r.db.WithContext(ctx).Raw(query, amountMinor, id, userID, amountMinor).Scan(&record)
	if result.Error != nil {
		return goal.Goal{}, fmt.Errorf("update goal balance: %w", result.Error)
	}
	if result.RowsAffected == 1 {
		return record.domain(), nil
	}
	// A failed guard is indistinguishable from a missing row; only inspect the owner's row.
	var owned goalModel
	err := r.db.WithContext(ctx).Select("id").Where("id = ? AND user_id = ?", id, userID).First(&owned).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return goal.Goal{}, goal.ErrNotFound
	}
	if err != nil {
		return goal.Goal{}, fmt.Errorf("check goal ownership: %w", err)
	}
	if deposit {
		return goal.Goal{}, goal.ErrInvalidAmount
	}
	return goal.Goal{}, goal.ErrInsufficientFunds
}
