package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/igor/shelfy/internal/storage/postgres/sqlcgen"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool    *pgxpool.Pool
	queries *sqlcgen.Queries
	logger  *slog.Logger
}

func Open(ctx context.Context, databaseURL string, logger *slog.Logger) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database config: %w", err)
	}
	cfg.MaxConns = 8
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("open database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}
	return &Store{
		pool:    pool,
		queries: sqlcgen.New(pool),
		logger:  logger,
	}, nil
}

func (s *Store) Close() {
	if s != nil && s.pool != nil {
		s.pool.Close()
	}
}
