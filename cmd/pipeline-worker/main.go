package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/igor/shelfy/internal/bootstrap"
	"github.com/igor/shelfy/internal/ingest"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/worker"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := bootstrap.Load(ctx, true)
	if err != nil {
		panic(err)
	}
	defer runtime.Close()
	cfg := runtime.Config
	tg := telegram.NewClient(cfg.BotToken, runtime.Logger)
	service := ingest.NewService(runtime.Store, tg, runtime.Copy, runtime.Logger, cfg.TmpDir, cfg.OllamaBaseURL, cfg.OllamaModel, cfg.VoskCommand, cfg.VoskModelPath, cfg.VoskGrammarPath)
	if err := worker.Run(ctx, runtime.Logger, runtime.Store, cfg.JobPollInterval, "pipeline-worker", service); err != nil && err != context.Canceled {
		panic(err)
	}
}
