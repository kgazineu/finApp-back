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
func Up(dsn string) error {
	return run(dsn, "aplicar migrations", func(m *migrate.Migrate) error { return m.Up() })
}

// Down reverts the last n applied migrations.
func Down(dsn string, n int) error {
	if n < 1 {
		return errors.New("a quantidade de migrations para desfazer deve ser pelo menos 1")
	}
	return run(dsn, "desfazer migrations", func(m *migrate.Migrate) error { return m.Steps(-n) })
}

// Force sets the version without running SQL, to recover from a migration that failed
// midway (dirty). Fix the database by hand first, then force the last version that is correct.
func Force(dsn string, version int) error {
	return run(dsn, "forçar versão", func(m *migrate.Migrate) error { return m.Force(version) })
}

// Version reports the applied version (0 = none) and whether the last migration failed midway.
func Version(dsn string) (version uint, dirty bool, err error) {
	err = run(dsn, "ler versão", func(m *migrate.Migrate) error {
		v, d, verr := m.Version()
		if errors.Is(verr, migrate.ErrNilVersion) {
			return nil
		}
		version, dirty = v, d
		return verr
	})
	return version, dirty, err
}

func run(dsn, action string, do func(*migrate.Migrate) error) (err error) {
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
	if err := do(runner); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("%s: %w", action, err)
	}
	return nil
}
