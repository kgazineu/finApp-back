package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/config"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/user"
	"github.com/kgazineu/finApp-back/migrations"

	"github.com/gin-gonic/gin"
)

func newRouter(server api.ServerInterface) *gin.Engine {
	router := gin.Default()
	// There is no trusted proxy configured for this deployment.
	_ = router.SetTrustedProxies(nil)

	api.RegisterHandlersWithOptions(router, server, api.GinServerOptions{
		ErrorHandler: func(c *gin.Context, _ error, status int) {
			c.JSON(status, api.ErrorResponse{Message: "Parâmetros da requisição inválidos"})
		},
	})
	api.RegisterDocumentation(router)

	return router
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("API encerrada", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := migrations.Up(cfg.DatabaseURL); err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 5*time.Second)
	db, err := postgres.Open(startup, cfg.DatabaseURL)
	cancel()
	if err != nil {
		return err
	}
	pool, err := db.DB()
	if err != nil {
		return err
	}
	defer pool.Close()

	repo := postgres.NewUserRepository(db)
	users := user.NewService(repo, password.Hasher{})
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           newRouter(api.NewServer(users)),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	result := make(chan error, 1)
	go func() { result <- server.ListenAndServe() }()
	slog.Info("API iniciada", "address", cfg.HTTPAddr)
	select {
	case err := <-result:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			server.Close()
			return err
		}
		return nil
	}
}
