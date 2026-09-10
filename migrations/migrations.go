// Package migrations embeds and applies the versioned SQL schema.
package migrations

import (
	"embed"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed *.sql
var files embed.FS

// Up applies pending migrations before the HTTP server starts.
// golang-migrate tracks versions and locks the database against concurrent migrations.
func Up(dsn string) (err error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return errors.New("URL PostgreSQL inválida para migrations")
	}
	u.Scheme = "pgx5"
	q := u.Query()
	if q.Get("connect_timeout") == "" {
		q.Set("connect_timeout", "5")
	}
	q.Set("x-statement-timeout", "30000")
	q.Set("lock_timeout", "10000")
	u.RawQuery = q.Encode()
	source, err := iofs.New(files, ".")
	if err != nil {
		return fmt.Errorf("ler migrations: %w", err)
	}
	runner, err := migrate.NewWithSourceInstance("iofs", source, u.String())
	if err != nil {
		source.Close()
		return fmt.Errorf("inicializar migrations: %w", err)
	}
	defer func() {
		sourceErr, databaseErr := runner.Close()
		err = errors.Join(err, sourceErr, databaseErr)
	}()
	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("aplicar migrations: %w", err)
	}
	return nil
}
