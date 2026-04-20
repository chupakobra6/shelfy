package main

import (
	"context"
	"database/sql"
	"os"
	"os/signal"
	"syscall"

	"github.com/igor/shelfy/internal/config"
	"github.com/igor/shelfy/internal/logging"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	logger := logging.New(cfg.LogLevel)

	db, err := sql.Open("pgx", cfg.DatabaseURL)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	if err := goose.SetDialect("postgres"); err != nil {
		panic(err)
	}
	goose.SetLogger(goose.NopLogger())
	if err := goose.UpContext(ctx, db, "migrations"); err != nil {
		panic(err)
	}
	logger.InfoContext(ctx, "migrations_applied")
}
