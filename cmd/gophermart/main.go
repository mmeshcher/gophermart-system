// Package main запускает HTTP-сервер сервиса гофермарт.
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/mmeshcher/gophermart-system/internal/accrual"
	"github.com/mmeshcher/gophermart-system/internal/config"
	"github.com/mmeshcher/gophermart-system/internal/handler"
	"github.com/mmeshcher/gophermart-system/internal/middleware"
	"github.com/mmeshcher/gophermart-system/internal/repository"
	"github.com/mmeshcher/gophermart-system/internal/service"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	sugar := logger.Sugar()

	cfg, err := config.Parse()
	if err != nil {
		sugar.Fatalw("configuration error", "error", err.Error())
	}

	repo, err := repository.NewPostgresRepository(cfg.DatabaseURI)
	if err != nil {
		sugar.Fatalw("database initialization error", "error", err.Error())
	}
	defer repo.Close()

	var accrualClient *accrual.Client
	if cfg.AccrualSystemAddress != "" {
		accrualClient = accrual.NewClient(cfg.AccrualSystemAddress)
	}

	svc := service.NewService(repo, accrualClient)
	defer svc.Close()

	authMiddleware := middleware.NewAuthMiddleware("gophermart-secret")
	h := handler.NewHandler(svc, logger, authMiddleware)

	r := h.SetupRouter()

	server := &http.Server{
		Addr:    cfg.RunAddress,
		Handler: r,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	g, ctx := errgroup.WithContext(ctx)

	// Запуск фонового процесса обновления начислений
	g.Go(func() error {
		svc.StartAccrualUpdates(ctx)
		return nil
	})

	// Запуск HTTP-сервера
	g.Go(func() error {
		sugar.Infow("starting gophermart server", "addr", cfg.RunAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("server error: %w", err)
		}
		return nil
	})

	// Graceful shutdown при отмене контекста (сигнал или ошибка в другой горутине)
	g.Go(func() error {
		<-ctx.Done()
		sugar.Info("shutting down server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown error: %w", err)
		}
		sugar.Info("server stopped gracefully")
		return nil
	})

	if err := g.Wait(); err != nil {
		sugar.Fatalw("application terminated with error", "error", err)
	}
}
