package migrations_test

import (
	"database/sql"
	"testing"

	"github.com/kgazineu/finApp-back/internal/testutil"
	"github.com/kgazineu/finApp-back/migrations"
)

func TestUpIsRepeatable(t *testing.T) {
	db, dsn := testutil.Database(t)
	if err := migrations.Up(dsn); err != nil {
		t.Fatal(err)
	}
	var version int
	var dirty bool
	if err := db.Raw("SELECT version, dirty FROM schema_migrations").Row().Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != 12 || dirty {
		t.Fatalf("versão inesperada: %d, dirty=%v", version, dirty)
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM users").Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT count(*) FROM transactions").Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err := db.Raw("SELECT count(*) FROM sessions").Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"goals", "accounts", "billing_registrations", "billing_entries", "recurring_transactions", "recurring_transaction_installments", "receivables", "receivable_installments", "password_resets"} {
		if err := db.Raw("SELECT count(*) FROM " + table).Row().Scan(&count); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDownRevertsAndUpReapplies(t *testing.T) {
	_, dsn := testutil.Database(t)
	if err := migrations.Down(dsn, 2); err != nil {
		t.Fatal(err)
	}
	if version, dirty, err := migrations.Version(dsn); err != nil || version != 10 || dirty {
		t.Fatalf("depois do down: versão %d, dirty=%v, erro %v", version, dirty, err)
	}
	if err := migrations.Up(dsn); err != nil {
		t.Fatal(err)
	}
	if version, _, err := migrations.Version(dsn); err != nil || version != 12 {
		t.Fatalf("depois do up: versão %d, erro %v", version, err)
	}
}

func TestFirstBillingDeltaBecomesNull(t *testing.T) {
	db, dsn := testutil.Database(t)
	if err := migrations.Down(dsn, 1); err != nil { // volta para antes da 000012
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`INSERT INTO users (id, name, email, password_hash, created_at, updated_at)
			VALUES ('00000000-0000-0000-0000-000000000001', 'Ana', 'ana@example.com', 'hash', now(), now())`,
		`INSERT INTO billing_registrations (user_id, delta, total) VALUES
			('00000000-0000-0000-0000-000000000001', 0, 5000),
			('00000000-0000-0000-0000-000000000001', -450, 4550)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := migrations.Up(dsn); err != nil {
		t.Fatal(err)
	}
	rows, err := db.Raw("SELECT delta FROM billing_registrations ORDER BY id").Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var deltas []sql.NullInt64
	for rows.Next() {
		var d sql.NullInt64
		if err := rows.Scan(&d); err != nil {
			t.Fatal(err)
		}
		deltas = append(deltas, d)
	}
	if len(deltas) != 2 || deltas[0].Valid || !deltas[1].Valid || deltas[1].Int64 != -450 {
		t.Fatalf("esperado [null, -450], veio %v", deltas)
	}
}
