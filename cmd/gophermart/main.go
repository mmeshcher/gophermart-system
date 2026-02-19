// Package main запускает HTTP-сервер сервиса гофермарт.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/mmeshcher/gophermart-system/internal/accrual"
	"github.com/mmeshcher/gophermart-system/internal/config"
	"github.com/mmeshcher/gophermart-system/internal/handler"
	"github.com/mmeshcher/gophermart-system/internal/middleware"
	"github.com/mmeshcher/gophermart-system/internal/repository"
	"github.com/mmeshcher/gophermart-system/internal/service"
)

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		panic(err)
	}
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

	svc.StartAccrualUpdates(ctx)

	go func() {
		sugar.Infow("starting gophermart server", "addr", cfg.RunAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			sugar.Fatalw("server error", "error", err.Error())
		}
	}()

	<-ctx.Done()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		sugar.Errorw("server shutdown error", "error", err.Error())
	}
}
