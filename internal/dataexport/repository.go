package dataexport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db *sqlx.DB
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{db: db}
}

// Export lê tudo do usuário numa transação só de leitura: o arquivo é uma foto consistente.
// Inclui registros arquivados, porque o histórico (lançamentos, parcelas pagas) aponta para eles.
func (r *Repository) Export(ctx context.Context, userID uuid.UUID) (*Document, error) {
	tx, err := r.db.BeginTxx(ctx, &sql.TxOptions{ReadOnly: true, Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	doc := &Document{Format: Format, Version: Version, ExportedAt: time.Now().UTC()}
	var entries []BillingEntry
	var recurringInstallments, receivableInstallments []Installment

	queries := []struct {
		dest  any
		query string
	}{
		{&doc.Accounts, `select id, name, kind, has_yield, archived_at, created_at from accounts where user_id = $1 order by id`},
		{&doc.BillingRegistrations, `select id, delta, total, created_at from billing_registrations where user_id = $1 order by id`},
		{&entries, `select e.billing_registration_id, e.account_id, e.account_name, e.amount
			from billing_entries e join billing_registrations r on r.id = e.billing_registration_id
			where r.user_id = $1 order by e.id`},
		{&doc.RecurringTransactions, `select id, description, kind, is_fixed, amount, start_month::text as start_month, interval_months,
			end_month::text as end_month, day_of_month, archived_at, created_at
			from recurring_transactions where user_id = $1 order by id`},
		{&recurringInstallments, `select i.transaction_id as parent_id, i.number, i.amount, i.due_date::text as due_date, i.paid_at
			from recurring_transaction_installments i join recurring_transactions t on t.id = i.transaction_id
			where t.user_id = $1 order by i.transaction_id, i.number`},
		{&doc.Receivables, `select id, kind, debtor, description, amount, amount_mode, interest_rate, archived_at, created_at
			from receivables where user_id = $1 order by id`},
		{&receivableInstallments, `select i.receivable_id as parent_id, i.number, i.amount, i.due_date::text as due_date, i.paid_at
			from receivable_installments i join receivables r on r.id = i.receivable_id
			where r.user_id = $1 order by i.receivable_id, i.number`},
		{&doc.Transactions, `select kind, amount_minor, necessity_level, description, category, payment_method, installments, occurred_at, created_at
			from transactions where user_id = $1 order by occurred_at, id`},
		{&doc.Goals, `select name, target_minor, saved_minor, created_at, updated_at from goals where user_id = $1 order by created_at, id`},
		{&doc.Targets, `select name, amount, deadline::text as deadline, account_id, created_at from targets where user_id = $1 order by id`},
	}
	for _, q := range queries {
		if err := tx.SelectContext(ctx, q.dest, q.query, userID); err != nil {
			return nil, err
		}
	}
	var savings SavingsGoal
	err = tx.GetContext(ctx, &savings, `select percent, amount from savings_goals where user_id = $1`, userID)
	if err == nil {
		doc.SavingsGoal = &savings
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	// listas sempre presentes no JSON ([] em vez de null) e filhos dentro do pai
	nonNil(&doc.Accounts)
	nonNil(&doc.BillingRegistrations)
	nonNil(&doc.RecurringTransactions)
	nonNil(&doc.Receivables)
	nonNil(&doc.Transactions)
	nonNil(&doc.Goals)
	nonNil(&doc.Targets)

	byRegistration := group(entries, func(e BillingEntry) int64 { return e.RegistrationID })
	for i := range doc.BillingRegistrations {
		doc.BillingRegistrations[i].Entries = orEmpty(byRegistration[doc.BillingRegistrations[i].ID])
	}
	byTransaction := group(recurringInstallments, func(i Installment) int64 { return i.ParentID })
	for i := range doc.RecurringTransactions {
		doc.RecurringTransactions[i].Installments = orEmpty(byTransaction[doc.RecurringTransactions[i].ID])
	}
	byReceivable := group(receivableInstallments, func(i Installment) int64 { return i.ParentID })
	for i := range doc.Receivables {
		doc.Receivables[i].Installments = orEmpty(byReceivable[doc.Receivables[i].ID])
	}

	return doc, tx.Commit()
}

// Import grava o arquivo inteiro ou nada (uma transação). Nunca mistura com dados existentes:
// conta com dados só aceita replace, que apaga tudo antes de gravar. Arquivo com erro desfaz
// também a limpeza, então os dados antigos continuam lá.
// ponytail: um INSERT por linha; troque por unnest em lote se os arquivos ficarem grandes
func (r *Repository) Import(ctx context.Context, userID uuid.UUID, doc *Document, replace bool) (string, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()

	// trava o usuário: duas importações simultâneas não passam juntas pela checagem de conta vazia
	if _, err := tx.ExecContext(ctx, `select 1 from users where id = $1 for update`, userID); err != nil {
		return "", err
	}
	var used bool
	err = tx.GetContext(ctx, &used, `select
		exists(select 1 from accounts where user_id = $1) or
		exists(select 1 from billing_registrations where user_id = $1) or
		exists(select 1 from recurring_transactions where user_id = $1) or
		exists(select 1 from receivables where user_id = $1) or
		exists(select 1 from transactions where user_id = $1) or
		exists(select 1 from goals where user_id = $1) or
		exists(select 1 from targets where user_id = $1)`, userID)
	if err != nil {
		return "", err
	}
	if used && !replace {
		return "", ErrNotEmpty
	}
	if used {
		for _, query := range []string{
			`delete from billing_registrations where user_id = $1`, // os lançamentos vão junto (cascade)
			`delete from targets where user_id = $1`,
			`delete from accounts where user_id = $1`,
			`delete from recurring_transaction_installments i using recurring_transactions t where t.id = i.transaction_id and t.user_id = $1`,
			`delete from recurring_transactions where user_id = $1`,
			`delete from receivable_installments i using receivables r where r.id = i.receivable_id and r.user_id = $1`,
			`delete from receivables where user_id = $1`,
			`delete from transactions where user_id = $1`,
			`delete from goals where user_id = $1`,
			`delete from savings_goals where user_id = $1`,
		} {
			if _, err := tx.ExecContext(ctx, query, userID); err != nil {
				return "", err
			}
		}
	}

	insertID := func(query string, args ...any) (int64, error) {
		var id int64
		err := tx.QueryRowxContext(ctx, query, args...).Scan(&id)
		return id, invalidData(err)
	}
	exec := func(query string, args ...any) error {
		_, err := tx.ExecContext(ctx, query, args...)
		return invalidData(err)
	}

	accountIDs := make(map[int64]int64, len(doc.Accounts))
	for _, a := range doc.Accounts {
		id, err := insertID(`insert into accounts (user_id, name, kind, has_yield, archived_at, created_at)
			values ($1, $2, $3, $4, $5, $6) returning id`, userID, a.Name, a.Kind, a.HasYield, a.ArchivedAt, a.CreatedAt)
		if err != nil {
			return "", err
		}
		accountIDs[a.ID] = id
	}

	for _, reg := range doc.BillingRegistrations {
		id, err := insertID(`insert into billing_registrations (user_id, delta, total, created_at)
			values ($1, $2, $3, $4) returning id`, userID, reg.Delta, reg.Total, reg.CreatedAt)
		if err != nil {
			return "", err
		}
		for _, e := range reg.Entries {
			accountID, ok := accountIDs[e.AccountID]
			if !ok {
				return "", fmt.Errorf("%w: registro de saldo aponta para a conta %d, que não está no arquivo", ErrInvalidFile, e.AccountID)
			}
			if err := exec(`insert into billing_entries (billing_registration_id, account_id, account_name, amount)
				values ($1, $2, $3, $4)`, id, accountID, e.AccountName, e.Amount); err != nil {
				return "", err
			}
		}
	}

	for _, t := range doc.RecurringTransactions {
		id, err := insertID(`insert into recurring_transactions
			(user_id, description, kind, is_fixed, amount, start_month, interval_months, end_month, day_of_month, archived_at, created_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) returning id`,
			userID, t.Description, t.Kind, t.IsFixed, t.Amount, t.StartMonth, t.IntervalMonths, t.EndMonth, t.DayOfMonth, t.ArchivedAt, t.CreatedAt)
		if err != nil {
			return "", err
		}
		for _, i := range t.Installments {
			if err := exec(`insert into recurring_transaction_installments (transaction_id, number, amount, due_date, paid_at)
				values ($1, $2, $3, $4, $5)`, id, i.Number, i.Amount, i.DueDate, i.PaidAt); err != nil {
				return "", err
			}
		}
	}

	for _, rec := range doc.Receivables {
		if rec.AmountMode == "" {
			rec.AmountMode = "total"
		}
		id, err := insertID(`insert into receivables (user_id, kind, debtor, description, amount, amount_mode, interest_rate, archived_at, created_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9) returning id`,
			userID, rec.Kind, rec.Debtor, rec.Description, rec.Amount, rec.AmountMode, rec.InterestRate, rec.ArchivedAt, rec.CreatedAt)
		if err != nil {
			return "", err
		}
		for _, i := range rec.Installments {
			if err := exec(`insert into receivable_installments (receivable_id, number, amount, due_date, paid_at)
				values ($1, $2, $3, $4, $5)`, id, i.Number, i.Amount, i.DueDate, i.PaidAt); err != nil {
				return "", err
			}
		}
	}

	for _, t := range doc.Transactions {
		if err := exec(`insert into transactions
			(id, user_id, kind, amount_minor, necessity_level, description, category, payment_method, installments, occurred_at, created_at)
			values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			uuid.New(), userID, t.Kind, t.AmountMinor, t.NecessityLevel, t.Description, t.Category, t.PaymentMethod, t.Installments, t.OccurredAt, t.CreatedAt); err != nil {
			return "", err
		}
	}

	for _, t := range doc.Targets {
		var accountID *int64
		if t.AccountID != nil {
			id, ok := accountIDs[*t.AccountID]
			if !ok {
				return "", fmt.Errorf("%w: a meta %q aponta para a conta %d, que não está no arquivo", ErrInvalidFile, t.Name, *t.AccountID)
			}
			accountID = &id
		}
		if err := exec(`insert into targets (user_id, name, amount, deadline, account_id, created_at) values ($1, $2, $3, $4, $5, $6)`,
			userID, t.Name, t.Amount, t.Deadline, accountID, t.CreatedAt); err != nil {
			return "", err
		}
	}

	for _, g := range doc.Goals {
		if err := exec(`insert into goals (id, user_id, name, target_minor, saved_minor, created_at, updated_at)
			values ($1, $2, $3, $4, $5, $6, $7)`, uuid.New(), userID, g.Name, g.TargetMinor, g.SavedMinor, g.CreatedAt, g.UpdatedAt); err != nil {
			return "", err
		}
	}

	// meta é configuração, não conta como dado: numa conta vazia que já tinha meta, a do arquivo vale
	if s := doc.SavingsGoal; s != nil && (s.Percent != nil || s.Amount != nil) {
		if err := exec(`insert into savings_goals (user_id, percent, amount) values ($1, $2, $3)
			on conflict (user_id) do update set percent = excluded.percent, amount = excluded.amount, updated_at = now()`,
			userID, s.Percent, s.Amount); err != nil {
			return "", err
		}
	}

	if err := tx.Commit(); err != nil {
		return "", err
	}

	summary := fmt.Sprintf("Importados: %d contas, %d registros de saldo, %d transações planejadas, %d valores a receber, %d lançamentos, %d metas e %d envelopes.",
		len(doc.Accounts), len(doc.BillingRegistrations), len(doc.RecurringTransactions), len(doc.Receivables), len(doc.Transactions), len(doc.Targets), len(doc.Goals))
	if used {
		summary = "Seus dados anteriores foram substituídos. " + summary
	}
	return summary, nil
}

// invalidData converte violação de regra do banco (classes 22 e 23: tipo, enum, CHECK, NOT NULL...)
// em erro de arquivo; o resto continua erro interno.
func invalidData(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (strings.HasPrefix(pgErr.Code, "22") || strings.HasPrefix(pgErr.Code, "23")) {
		detail := pgErr.ConstraintName
		if detail == "" {
			detail = pgErr.ColumnName
		}
		if detail == "" {
			return fmt.Errorf("%w: algum registro não respeita as regras do FinApp", ErrInvalidFile)
		}
		return fmt.Errorf("%w: algum registro não respeita as regras do FinApp (%s)", ErrInvalidFile, detail)
	}
	return err
}

func group[T any](items []T, key func(T) int64) map[int64][]T {
	m := make(map[int64][]T)
	for _, item := range items {
		m[key(item)] = append(m[key(item)], item)
	}
	return m
}

func orEmpty[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func nonNil[T any](items *[]T) {
	*items = orEmpty(*items)
}
