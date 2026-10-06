package receivable

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

type receivableRow struct {
	ID           int64      `db:"id"`
	UserID       uuid.UUID  `db:"user_id"`
	Kind         Kind       `db:"kind"`
	Debtor       string     `db:"debtor"`
	Description  string     `db:"description"`
	Amount       int64      `db:"amount"`
	AmountMode   AmountMode `db:"amount_mode"`
	InterestRate int        `db:"interest_rate"`
	CreatedAt    time.Time  `db:"created_at"`
	ArchivedAt   *time.Time `db:"archived_at"`
}

type installmentRow struct {
	ID           int64      `db:"id"`
	ReceivableID int64      `db:"receivable_id"`
	Number       int        `db:"number"`
	Amount       int64      `db:"amount"`
	DueDate      time.Time  `db:"due_date"`
	PaidAt       *time.Time `db:"paid_at"`
}

type pendingInstallmentRow struct {
	installmentRow
	Debtor      string `db:"debtor"`
	Description string `db:"description"`
}

func (row receivableRow) toReceivable() *Receivable {
	return &Receivable{
		ID:           row.ID,
		Kind:         row.Kind,
		Debtor:       row.Debtor,
		Description:  row.Description,
		Amount:       row.Amount,
		AmountMode:   row.AmountMode,
		InterestRate: row.InterestRate,
		CreatedAt:    row.CreatedAt,
	}
}

func (row installmentRow) toInstallment() *Installment {
	return &Installment{
		ID:           row.ID,
		ReceivableID: row.ReceivableID,
		Number:       row.Number,
		Amount:       row.Amount,
		DueDate:      row.DueDate,
		PaidAt:       row.PaidAt,
	}
}

func (r *Repository) Create(ctx context.Context, userID uuid.UUID, rec *Receivable) (*Receivable, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var recRow receivableRow
	err = tx.QueryRowxContext(ctx, `
		insert into receivables (user_id, kind, debtor, description, amount, amount_mode, interest_rate)
		values ($1, $2, $3, $4, $5, $6, $7) returning *
	`, userID, rec.Kind, rec.Debtor, rec.Description, rec.Amount, rec.AmountMode, rec.InterestRate).StructScan(&recRow)
	if err != nil {
		return nil, err
	}

	n := len(rec.Installments)
	numbers := make([]int64, n)
	amounts := make([]int64, n)
	dueDates := make([]time.Time, n)
	for i, inst := range rec.Installments {
		numbers[i] = int64(inst.Number)
		amounts[i] = inst.Amount
		dueDates[i] = inst.DueDate
	}

	var instRows []installmentRow
	err = tx.SelectContext(ctx, &instRows, `
		insert into receivable_installments (receivable_id, number, amount, due_date)
		select $1, unnest($2::int[]), unnest($3::bigint[]), unnest($4::date[])
		returning *
	`, recRow.ID, numbers, amounts, dueDates)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}

	result := recRow.toReceivable()
	result.Installments = make([]*Installment, len(instRows))
	for i, row := range instRows {
		result.Installments[i] = row.toInstallment()
	}

	return result, nil
}

func (r *Repository) FindPendingInstallments(ctx context.Context, userID uuid.UUID) ([]*PendingInstallment, error) {
	var rows []pendingInstallmentRow

	err := r.db.SelectContext(ctx, &rows, `
		select i.*, r.debtor, r.description
		from receivable_installments i
		join receivables r on r.id = i.receivable_id
		where r.user_id = $1 and i.paid_at is null
		order by i.due_date, i.id
	`, userID)
	if err != nil {
		return nil, err
	}

	return toPendingInstallments(rows), nil
}

func (r *Repository) FindPendingUntil(ctx context.Context, userID uuid.UUID, until time.Time) ([]*PendingInstallment, error) {
	var rows []pendingInstallmentRow

	err := r.db.SelectContext(ctx, &rows, `
		select i.*, r.debtor, r.description
		from receivable_installments i
		join receivables r on r.id = i.receivable_id
		where r.user_id = $1 and i.paid_at is null and i.due_date <= $2
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
		select i.*, r.debtor, r.description
		from receivable_installments i
		join receivables r on r.id = i.receivable_id
		where r.user_id = $1 and i.paid_at >= $2
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
			Debtor:      row.Debtor,
			Description: row.Description,
		}
	}

	return pending
}

// UpdateInstallment marca como recebida e/ou muda valor e vencimento de uma parcela do usuário.
// Com ApplyToFollowing, as parcelas seguintes ainda não recebidas ganham o mesmo valor e
// vencimentos mês a mês a partir da nova data. Parcelas recebidas nunca são alteradas em lote.
func (r *Repository) UpdateInstallment(ctx context.Context, userID uuid.UUID, id int64, ch InstallmentChanges) (*Installment, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var row installmentRow
	err = tx.QueryRowxContext(ctx, `
		select i.* from receivable_installments i
		join receivables r on r.id = i.receivable_id
		where i.id = $1 and r.user_id = $2
		for update of i
	`, id, userID).StructScan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	statements := []struct {
		apply bool
		query string
		args  []any
	}{
		{ch.Paid != nil, `update receivable_installments set paid_at = case when $1 then coalesce(paid_at, now()) end where id = $2`,
			[]any{ch.Paid, id}},
		{ch.Amount != nil, `update receivable_installments set amount = $1 where id = $2`,
			[]any{ch.Amount, id}},
		{ch.Amount != nil && ch.ApplyToFollowing, `
			update receivable_installments set amount = $1
			where receivable_id = $2 and number > $3 and paid_at is null`,
			[]any{ch.Amount, row.ReceivableID, row.Number}},
		{ch.DueDate != nil, `update receivable_installments set due_date = $1 where id = $2`,
			[]any{dateArg(ch.DueDate), id}},
		// "data + n meses" no Postgres parte sempre da data base e não transborda (31/01 + 1 mês = 28/02)
		{ch.DueDate != nil && ch.ApplyToFollowing, `
			update receivable_installments set due_date = ($1::date + (number - $2) * interval '1 month')::date
			where receivable_id = $3 and number > $2 and paid_at is null`,
			[]any{dateArg(ch.DueDate), row.Number, row.ReceivableID}},
	}
	for _, st := range statements {
		if !st.apply {
			continue
		}
		if _, err := tx.ExecContext(ctx, st.query, st.args...); err != nil {
			return nil, err
		}
	}

	if err := tx.QueryRowxContext(ctx, `select * from receivable_installments where id = $1`, id).StructScan(&row); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	return row.toInstallment(), nil
}

// dateArg manda só a data ("2026-10-15"), sem fuso.
func dateArg(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Format(time.DateOnly)
}

func (r *Repository) FindAll(ctx context.Context, userID uuid.UUID) ([]*Receivable, error) {
	var recRows []receivableRow
	err := r.db.SelectContext(ctx, &recRows, `
		select * from receivables where user_id = $1 and archived_at is null order by id desc
	`, userID)
	if err != nil {
		return nil, err
	}

	var instRows []installmentRow
	err = r.db.SelectContext(ctx, &instRows, `
		select i.* from receivable_installments i
		join receivables r on r.id = i.receivable_id
		where r.user_id = $1 and r.archived_at is null
		order by i.number
	`, userID)
	if err != nil {
		return nil, err
	}

	byReceivable := make(map[int64][]*Installment)
	for _, row := range instRows {
		byReceivable[row.ReceivableID] = append(byReceivable[row.ReceivableID], row.toInstallment())
	}

	receivables := make([]*Receivable, len(recRows))
	for i, row := range recRows {
		receivables[i] = row.toReceivable()
		receivables[i].Installments = byReceivable[row.ID]
	}

	return receivables, nil
}

func (r *Repository) Update(ctx context.Context, userID uuid.UUID, id int64, debtor, description string) (*Receivable, error) {
	var row receivableRow

	err := r.db.QueryRowxContext(ctx, `
		update receivables set debtor = $1, description = $2
		where id = $3 and user_id = $4 and archived_at is null
		returning *
	`, debtor, description, id, userID).StructScan(&row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReceivableNotFound
	}
	if err != nil {
		return nil, err
	}

	return row.toReceivable(), nil
}

func (r *Repository) Delete(ctx context.Context, userID uuid.UUID, id int64) (bool, error) {
	_, err := r.db.ExecContext(ctx, `
		delete from receivable_installments i
		using receivables r
		where r.id = i.receivable_id and r.id = $1 and r.user_id = $2 and i.paid_at is null
	`, id, userID)
	if err != nil {
		return false, err
	}

	archived := false
	res, err := r.db.ExecContext(ctx, `delete from receivables where id = $1 and user_id = $2`, id, userID)
	if postgres.ErrorCode(err) == pgerrcode.ForeignKeyViolation {
		archived = true
		res, err = r.db.ExecContext(ctx, `
			update receivables set archived_at = now() where id = $1 and user_id = $2 and archived_at is null
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
		return false, ErrReceivableNotFound
	}

	return archived, nil
}
