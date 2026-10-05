package recurring

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
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

type transactionRow struct {
	ID             int64      `db:"id"`
	UserID         uuid.UUID  `db:"user_id"`
	Description    string     `db:"description"`
	Kind           Kind       `db:"kind"`
	IsFixed        bool       `db:"is_fixed"`
	Amount         int64      `db:"amount"`
	StartMonth     time.Time  `db:"start_month"`
	IntervalMonths int        `db:"interval_months"`
	EndMonth       *time.Time `db:"end_month"`
	DayOfMonth     *int       `db:"day_of_month"`
	CreatedAt      time.Time  `db:"created_at"`
	ArchivedAt     *time.Time `db:"archived_at"`
}

type installmentRow struct {
	ID            int64      `db:"id"`
	TransactionID int64      `db:"transaction_id"`
	Number        int        `db:"number"`
	Amount        int64      `db:"amount"`
	DueDate       time.Time  `db:"due_date"`
	PaidAt        *time.Time `db:"paid_at"`
}

type pendingInstallmentRow struct {
	installmentRow
	Description string `db:"description"`
	Kind        Kind   `db:"kind"`
	IsFixed     bool   `db:"is_fixed"`
}

func (row transactionRow) toTransaction() *Transaction {
	return &Transaction{
		ID:             row.ID,
		Description:    row.Description,
		Kind:           row.Kind,
		IsFixed:        row.IsFixed,
		Amount:         row.Amount,
		StartMonth:     row.StartMonth,
		IntervalMonths: row.IntervalMonths,
		EndMonth:       row.EndMonth,
		DayOfMonth:     row.DayOfMonth,
		CreatedAt:      row.CreatedAt,
		ArchivedAt:     row.ArchivedAt,
	}
}

func (row installmentRow) toInstallment() *Installment {
	return &Installment{
		ID:            row.ID,
		TransactionID: row.TransactionID,
		Number:        row.Number,
		Amount:        row.Amount,
		DueDate:       row.DueDate,
		PaidAt:        row.PaidAt,
	}
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, t *Transaction) (*Transaction, error) {
	var endMonth any
	if t.EndMonth != nil {
		endMonth = t.EndMonth.Format(time.DateOnly)
	}

	var row transactionRow
	err := r.db.QueryRowxContext(ctx, `
		insert into recurring_transactions (user_id, description, kind, is_fixed, amount, start_month, interval_months, end_month, day_of_month)
		values ($1, $2, $3, $4, $5, $6, $7, $8, $9) returning *
	`, userID, t.Description, t.Kind, t.IsFixed, t.Amount, t.StartMonth.Format(time.DateOnly), // datas como "2026-10-01": só a data, sem fuso
		t.IntervalMonths, endMonth, t.DayOfMonth).StructScan(&row)
	if err != nil {
		return nil, err
	}

	return row.toTransaction(), nil
}

func (r *Repository) FindAll(ctx context.Context, userID uuid.UUID) ([]*Transaction, error) {
	var rows []transactionRow

	err := r.db.SelectContext(ctx, &rows, `
		select * from recurring_transactions
		where user_id = $1 and archived_at is null
		order by day_of_month nulls last, id
	`, userID)
	if err != nil {
		return nil, err
	}

	transactions := make([]*Transaction, len(rows))
	for i, row := range rows {
		transactions[i] = row.toTransaction()
	}

	return transactions, nil
}

// deleteUnpaidInstallments só apaga parcelas de transactions do próprio usuário.
func (r *Repository) deleteUnpaidInstallments(ctx context.Context, userID uuid.UUID, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		delete from recurring_transaction_installments i
		using recurring_transactions t
		where t.id = i.transaction_id and t.id = $1 and t.user_id = $2 and i.paid_at is null
	`, id, userID)
	return err
}

func (r *Repository) Update(ctx context.Context, userID uuid.UUID, t *Transaction) (*Transaction, error) {
	// apaga antes de atualizar: parcela não paga sempre pode ser gerada de novo, já com os valores novos
	if err := r.deleteUnpaidInstallments(ctx, userID, t.ID); err != nil {
		return nil, err
	}

	var endMonth any
	if t.EndMonth != nil {
		endMonth = t.EndMonth.Format(time.DateOnly)
	}

	var row transactionRow
	err := r.db.QueryRowxContext(ctx, `
		update recurring_transactions set description = $1, amount = $2, day_of_month = $3, end_month = $4
		where id = $5 and user_id = $6 and archived_at is null
		returning *
	`, t.Description, t.Amount, t.DayOfMonth, endMonth, t.ID, userID).StructScan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if postgres.ErrorCode(err) == pgerrcode.CheckViolation {
		return nil, fmt.Errorf("%w: o mês final não pode ser antes do mês de início", ErrInvalidTransaction)
	}
	if err != nil {
		return nil, err
	}

	return row.toTransaction(), nil
}

func (r *Repository) Delete(ctx context.Context, userID uuid.UUID, id int64) (bool, error) {
	if err := r.deleteUnpaidInstallments(ctx, userID, id); err != nil {
		return false, err
	}

	archived := false
	res, err := r.db.ExecContext(ctx, `delete from recurring_transactions where id = $1 and user_id = $2`, id, userID)
	if postgres.ErrorCode(err) == pgerrcode.ForeignKeyViolation {
		archived = true
		res, err = r.db.ExecContext(ctx, `
			update recurring_transactions set archived_at = now()
			where id = $1 and user_id = $2 and archived_at is null
		`, id, userID)
	}
	if err != nil {
		return false, err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	if n == 0 {
		return false, ErrNotFound
	}

	return archived, nil
}

// InsertInstallments recebe só parcelas geradas a partir das transactions do usuário (FindAll já filtra).
func (r *Repository) InsertInstallments(ctx context.Context, installments []*Installment) error {
	if len(installments) == 0 {
		return nil
	}

	n := len(installments)
	transactionIDs := make([]int64, n)
	numbers := make([]int64, n)
	amounts := make([]int64, n)
	dueDates := make([]time.Time, n)
	for i, inst := range installments {
		transactionIDs[i] = inst.TransactionID
		numbers[i] = int64(inst.Number)
		amounts[i] = inst.Amount
		dueDates[i] = inst.DueDate
	}

	// parcela que já existe fica como está, inclusive se já foi paga
	_, err := r.db.ExecContext(ctx, `
		insert into recurring_transaction_installments (transaction_id, number, amount, due_date)
		select unnest($1::bigint[]), unnest($2::int[]), unnest($3::bigint[]), unnest($4::date[])
		on conflict (transaction_id, number) do nothing
	`, transactionIDs, numbers, amounts, dueDates)

	return err
}

func (r *Repository) FindPendingInstallments(ctx context.Context, userID uuid.UUID, until time.Time) ([]*PendingInstallment, error) {
	var rows []pendingInstallmentRow

	err := r.db.SelectContext(ctx, &rows, `
		select i.*, t.description, t.kind, t.is_fixed
		from recurring_transaction_installments i
		join recurring_transactions t on t.id = i.transaction_id
		where t.user_id = $1 and i.paid_at is null and i.due_date <= $2
		order by i.due_date, i.id
	`, userID, until.Format(time.DateOnly))
	if err != nil {
		return nil, err
	}

	return toPendingInstallments(rows), nil
}

func (r *Repository) FindPaidInstallments(ctx context.Context, userID uuid.UUID, since time.Time) ([]*PendingInstallment, error) {
	var rows []pendingInstallmentRow

	err := r.db.SelectContext(ctx, &rows, `
		select i.*, t.description, t.kind, t.is_fixed
		from recurring_transaction_installments i
		join recurring_transactions t on t.id = i.transaction_id
		where t.user_id = $1 and i.paid_at >= $2
		order by i.paid_at desc
	`, userID, since)
	if err != nil {
		return nil, err
	}

	return toPendingInstallments(rows), nil
}

func toPendingInstallments(rows []pendingInstallmentRow) []*PendingInstallment {
	pending := make([]*PendingInstallment, len(rows))
	for i, row := range rows {
		pending[i] = &PendingInstallment{
			Installment: *row.toInstallment(),
			Description: row.Description,
			Kind:        row.Kind,
			IsFixed:     row.IsFixed,
		}
	}

	return pending
}

func (r *Repository) SetInstallmentPaid(ctx context.Context, userID uuid.UUID, id int64, paid bool) (*Installment, error) {
	var row installmentRow

	err := r.db.QueryRowxContext(ctx, `
		update recurring_transaction_installments i
		set paid_at = case when $1 then coalesce(i.paid_at, now()) end
		from recurring_transactions t
		where i.id = $2 and t.id = i.transaction_id and t.user_id = $3
		returning i.*
	`, paid, id, userID).StructScan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInstallmentNotFound
	}
	if err != nil {
		return nil, err
	}

	return row.toInstallment(), nil
}
