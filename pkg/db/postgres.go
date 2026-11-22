package db

import (
	"context"
	"database/sql"
	"fmt"
	"io/fs"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
)

// Bundle holds both a pgxpool (for CopyFrom / raw high-throughput queries) and a Bun ORM DB.
type Bundle struct {
	Pool  *pgxpool.Pool
	SQLDB *sql.DB
	Bun   *bun.DB
}

// Connect opens a pgxpool + Bun ORM handle against PostgreSQL.
func Connect(ctx context.Context, databaseURL string) (*Bundle, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 16
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open pgxpool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	sqlDB := stdlib.OpenDBFromPool(pool)
	bunDB := bun.NewDB(sqlDB, pgdialect.New())

	return &Bundle{
		Pool:  pool,
		SQLDB: sqlDB,
		Bun:   bunDB,
	}, nil
}

// Close shuts down the Bun/SQL handle and underlying pgxpool.
func (b *Bundle) Close() {
	if b.Bun != nil {
		_ = b.Bun.Close()
	}
	if b.Pool != nil {
		b.Pool.Close()
	}
}

// RunMigrations applies embedded SQL migrations using goose inside the service's own schema.
func (b *Bundle) RunMigrations(ctx context.Context, migrationsFS fs.FS, dir string) error {
	goose.SetBaseFS(migrationsFS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("set goose dialect: %w", err)
	}
	if err := goose.UpContext(ctx, b.SQLDB, dir); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	return nil
}
