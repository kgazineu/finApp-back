package account

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgerrcode"
	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/postgres"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

type accountRow struct {
	ID         int64      `db:"id"`
	UserID     uuid.UUID  `db:"user_id"`
	Name       string     `db:"name"`
	Kind       Kind       `db:"kind"`
	HasYield   bool       `db:"has_yield"`
	ArchivedAt *time.Time `db:"archived_at"`
	CreatedAt  time.Time  `db:"created_at"`
}

func (row accountRow) toAccount() *Account {
	return &Account{
		ID:         row.ID,
		Name:       row.Name,
		Kind:       row.Kind,
		HasYield:   row.HasYield,
		ArchivedAt: row.ArchivedAt,
		CreatedAt:  row.CreatedAt,
	}
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, a *Account) (*Account, error) {
	var row accountRow

	err := r.db.QueryRowxContext(ctx, `
		insert into accounts (user_id, name, kind, has_yield) values ($1, $2, $3, $4) returning *
	`, userID, a.Name, a.Kind, a.HasYield).StructScan(&row)
	if err != nil {
		return nil, err
	}

	return row.toAccount(), nil
}

func (r *Repository) FindActive(ctx context.Context, userID uuid.UUID) ([]*Account, error) {
	var rows []accountRow

	err := r.db.SelectContext(ctx, &rows, `
		select * from accounts where user_id = $1 and archived_at is null order by id
	`, userID)
	if err != nil {
		return nil, err
	}

	accounts := make([]*Account, len(rows))
	for i, row := range rows {
		accounts[i] = row.toAccount()
	}

	return accounts, nil
}

func (r *Repository) FindByID(ctx context.Context, userID uuid.UUID, id int64) (*Account, error) {
	var row accountRow

	err := r.db.QueryRowxContext(ctx, `
		select * from accounts where id = $1 and user_id = $2
	`, id, userID).StructScan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	return row.toAccount(), nil
}

func (r *Repository) Update(ctx context.Context, userID uuid.UUID, a *Account) (*Account, error) {
	var row accountRow

	err := r.db.QueryRowxContext(ctx, `
		update accounts set name = $1, kind = $2, has_yield = $3 where id = $4 and user_id = $5 returning *
	`, a.Name, a.Kind, a.HasYield, a.ID, userID).StructScan(&row)

	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, err
	}

	return row.toAccount(), nil
}

type DeleteResult struct {
	Archived bool
}

func (r *Repository) Delete(ctx context.Context, userID uuid.UUID, id int64) (*DeleteResult, error) {
	var archived bool = false
	res, err := r.db.ExecContext(ctx, `delete from accounts where id = $1 and user_id = $2`, id, userID)

	if postgres.ErrorCode(err) == pgerrcode.ForeignKeyViolation {
		archived = true
		res, err = r.db.ExecContext(ctx, `
			update accounts set archived_at = now() where id = $1 and user_id = $2
			`, id, userID)
	}

	if err != nil {
		return nil, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return nil, err
	}

	if n == 0 {
		return nil, ErrNotFound
	}

	return &DeleteResult{Archived: archived}, nil
}
