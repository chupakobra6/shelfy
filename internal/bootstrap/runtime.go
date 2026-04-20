package bootstrap

import (
	"context"
	"log/slog"
	"os"

	"github.com/igor/shelfy/internal/config"
	copycat "github.com/igor/shelfy/internal/copy"
	"github.com/igor/shelfy/internal/logging"
	"github.com/igor/shelfy/internal/storage/postgres"
)

type Runtime struct {
	Config config.Config
	Logger *slog.Logger
	Copy   *copycat.Loader
	Store  *postgres.Store
}

func Load(ctx context.Context, ensureTmpDir bool) (Runtime, error) {
	cfg, err := config.Load()
	if err != nil {
		return Runtime{}, err
	}
	logger := logging.New(cfg.LogLevel)
	if ensureTmpDir {
		if err := os.MkdirAll(cfg.TmpDir, 0o755); err != nil {
			return Runtime{}, err
		}
	}
	copyLoader, err := copycat.Load(cfg.CopyRuntimePath)
	if err != nil {
		return Runtime{}, err
	}
	store, err := postgres.Open(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return Runtime{}, err
	}
	return Runtime{
		Config: cfg,
		Logger: logger,
		Copy:   copyLoader,
		Store:  store,
	}, nil
}

func (r Runtime) Close() {
	if r.Store != nil {
		r.Store.Close()
	}
}
