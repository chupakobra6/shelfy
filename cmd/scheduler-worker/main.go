package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/igor/shelfy/internal/bootstrap"
	"github.com/igor/shelfy/internal/scheduler"
	"github.com/igor/shelfy/internal/telegram"
	"github.com/igor/shelfy/internal/worker"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	runtime, err := bootstrap.Load(ctx, false)
	if err != nil {
		return err
	}
	defer runtime.Close()
	cfg := runtime.Config
	tg := telegram.NewClient(cfg.BotToken, runtime.Logger)
	service := scheduler.NewService(runtime.Store, tg, runtime.Copy, runtime.Logger, scheduler.Options{
		DefaultTimezone: runtime.Config.DefaultTimezone,
		DigestLocalTime: runtime.Config.DigestLocalTime,
		E2ETestUserID:   runtime.Config.E2ETestUserID,
		EnableE2EReset:  cfg.EnableDevControl && !cfg.IsProduction() && cfg.E2ETestUserID != 0,
	})

	if cfg.EnableDevControl && !cfg.IsProduction() {
		server := &http.Server{
			Addr:              cfg.DevControlAddr,
			Handler:           service.Handler(),
			ReadHeaderTimeout: 5 * time.Second,
		}
		go func() {
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				runtime.Logger.ErrorContext(ctx, "dev_control_server_failed", "error", err)
			}
		}()
		go func() {
			<-ctx.Done()
			_ = server.Shutdown(context.Background())
		}()
	}

	go func() {
		ticker := time.NewTicker(cfg.SchedulerInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := service.RunMaintenanceTick(ctx); err != nil {
					runtime.Logger.ErrorContext(ctx, "scheduler_maintenance_failed", "error", err)
				}
			}
		}
	}()

	if err := worker.Run(ctx, runtime.Logger, runtime.Store, cfg.JobPollInterval, "scheduler-worker", service); err != nil && err != context.Canceled {
		return err
	}
	return nil
}
