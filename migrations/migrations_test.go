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
	if version != 1 || dirty {
		t.Fatalf("versão inesperada: %d, dirty=%v", version, dirty)
	}
	var count int
	if err := db.Raw("SELECT count(*) FROM users").Row().Scan(&count); err != nil {
		t.Fatal(err)
	}
}
