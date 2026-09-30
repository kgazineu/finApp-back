package postgres

import (
	"context"
	"database/sql/driver"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestSessionHashIsSingleByteaParameter(t *testing.T) {
	db, err := gorm.Open(pgdriver.Open("postgres://test:test@localhost:5432/test?sslmode=disable"), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	hash := [32]byte{1, 2, 3}
	stmt := db.WithContext(context.Background()).Exec(
		"INSERT INTO sessions (token_hash, user_id, expires_at) VALUES (?, ?, ?)",
		sessionHashValue(hash), uuid.New(), time.Now().UTC(),
	).Statement
	if len(stmt.Vars) != 3 || !strings.Contains(stmt.SQL.String(), "VALUES ($1, $2, $3)") {
		t.Fatalf("expected three bind parameters for three columns, got %d: %s", len(stmt.Vars), stmt.SQL.String())
	}
	value, ok := stmt.Vars[0].(driver.Valuer)
	if !ok {
		t.Fatalf("hash argument must implement driver.Valuer, got %T", stmt.Vars[0])
	}
	encoded, err := value.Value()
	if err != nil {
		t.Fatal(err)
	}
	bytes, ok := encoded.([]byte)
	if !ok || len(bytes) != len(hash) || bytes[0] != hash[0] || bytes[1] != hash[1] || bytes[2] != hash[2] {
		t.Fatalf("session hash must stay a single 32-byte BYTEA value, got %T", encoded)
	}
	lookup := db.Raw("SELECT user_id, expires_at FROM sessions WHERE token_hash = ?", sessionHashValue(hash)).Statement
	if len(lookup.Vars) != 1 || !strings.Contains(lookup.SQL.String(), "token_hash = $1") {
		t.Fatalf("expected one hash bind parameter for lookup, got %d: %s", len(lookup.Vars), lookup.SQL.String())
	}
}
