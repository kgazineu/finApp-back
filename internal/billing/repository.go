package billing

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

type billingRegistration struct {
	ID        int64     `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	Delta     int64     `db:"delta"`
	Total     int64     `db:"total"`
	CreatedAt time.Time `db:"created_at"`
}

type billingEntry struct {
	ID                    int64  `db:"id"`
	BillingRegistrationID int64  `db:"billing_registration_id"`
	AccountID             int64  `db:"account_id"`
	AccountName           string `db:"account_name"`
	Amount                int64  `db:"amount"`
}

func (row billingRegistration) toBillingRegistration() *BillingRegistration {
	return &BillingRegistration{
		ID:        row.ID,
		delta:     row.Delta,
		total:     row.Total,
		CreatedAt: row.CreatedAt,
	}
}

func (row billingEntry) toBillingEntry() *BillingEntry {
	return &BillingEntry{
		ID:                    row.ID,
		BillingRegistrationID: row.BillingRegistrationID,
		AccountID:             row.AccountID,
		AccountName:           row.AccountName,
		Amount:                row.Amount,
	}
}

func (r *Repository) FindManyBillingRegistrations(ctx context.Context, userID uuid.UUID) ([]*BillingRegistration, error) {
	var regRows []billingRegistration
	err := r.db.SelectContext(ctx, &regRows, `
		select * from billing_registrations where user_id = $1 order by id
	`, userID)
	if err != nil {
		return nil, err
	}

	var entryRows []billingEntry
	err = r.db.SelectContext(ctx, &entryRows, `
		select e.* from billing_entries e
		join billing_registrations r on r.id = e.billing_registration_id
		where r.user_id = $1
		order by e.id
	`, userID)
	if err != nil {
		return nil, err
	}

	entriesByReg := make(map[int64][]*BillingEntry)
	for _, row := range entryRows {
		entriesByReg[row.BillingRegistrationID] = append(entriesByReg[row.BillingRegistrationID], row.toBillingEntry())
	}

	billings := make([]*BillingRegistration, len(regRows))
	for i, row := range regRows {
		billings[i] = row.toBillingRegistration()
		billings[i].entries = entriesByReg[row.ID]
	}

	return billings, nil
}

func (r *Repository) findLastBillingRegistration(ctx context.Context, userID uuid.UUID) (*BillingRegistration, error) {
	var row billingRegistration

	err := r.db.QueryRowxContext(ctx, `
		select * from billing_registrations where user_id = $1 order by id desc limit 1
	`, userID).StructScan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return row.toBillingRegistration(), nil
}

func (r *Repository) createBillingRegistration(ctx context.Context, userID uuid.UUID, billing *BillingRegistration) (*BillingRegistration, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var regRow billingRegistration
	err = tx.QueryRowxContext(ctx, `
		insert into billing_registrations (user_id, delta, total) values ($1, $2, $3) returning *
	`, userID, billing.delta, billing.total).StructScan(&regRow)
	if err != nil {
		return nil, err
	}

	n := len(billing.entries)
	accountIDs := make([]int64, n)
	accountNames := make([]string, n)
	amounts := make([]int64, n)
	for i, e := range billing.entries {
		accountIDs[i] = e.AccountID
		accountNames[i] = e.AccountName
		amounts[i] = e.Amount
	}

	var entryRows []billingEntry
	err = tx.SelectContext(ctx, &entryRows, `
		insert into billing_entries (billing_registration_id, account_id, account_name, amount)
		select $1, unnest($2::bigint[]), unnest($3::text[]), unnest($4::bigint[])
		returning *
	`, regRow.ID, accountIDs, accountNames, amounts)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	result := regRow.toBillingRegistration()
	result.entries = make([]*BillingEntry, len(entryRows))
	for i, row := range entryRows {
		result.entries[i] = row.toBillingEntry()
	}

	return result, nil
}
