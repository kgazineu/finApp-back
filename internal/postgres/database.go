package postgres

import (
	"context"
	"fmt"
	"time"

	pgdriver "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Open opens the application's pool and verifies the database is reachable.
func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	db, err := gorm.Open(pgdriver.Open(dsn), &gorm.Config{
		DisableAutomaticPing: true,
		// SQL error logs can contain credential values; report failures at the application boundary.
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("abrir PostgreSQL: %w", err)
	}
	pool, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("obter pool PostgreSQL: %w", err)
	}
	pool.SetMaxOpenConns(10)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(30 * time.Minute)
	if err := pool.PingContext(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("conectar ao PostgreSQL: %w", err)
	}
	return db, nil
}
