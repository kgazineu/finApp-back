package migrations_test

import (
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
	if version != 10 || dirty {
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
	if version, dirty, err := migrations.Version(dsn); err != nil || version != 8 || dirty {
		t.Fatalf("depois do down: versão %d, dirty=%v, erro %v", version, dirty, err)
	}
	if err := migrations.Up(dsn); err != nil {
		t.Fatal(err)
	}
	if version, _, err := migrations.Version(dsn); err != nil || version != 10 {
		t.Fatalf("depois do up: versão %d, erro %v", version, err)
	}
}
