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
	_ "time/tzdata" // fuso embutido: TZ funciona mesmo em imagem sem zoneinfo (alpine)

	"github.com/jmoiron/sqlx"
	"github.com/kgazineu/finApp-back/internal/account"
	"github.com/kgazineu/finApp-back/internal/api"
	"github.com/kgazineu/finApp-back/internal/auth"
	"github.com/kgazineu/finApp-back/internal/billing"
	"github.com/kgazineu/finApp-back/internal/config"
	"github.com/kgazineu/finApp-back/internal/dataexport"
	"github.com/kgazineu/finApp-back/internal/goal"
	"github.com/kgazineu/finApp-back/internal/overview"
	"github.com/kgazineu/finApp-back/internal/password"
	"github.com/kgazineu/finApp-back/internal/passwordreset"
	"github.com/kgazineu/finApp-back/internal/postgres"
	"github.com/kgazineu/finApp-back/internal/receivable"
	"github.com/kgazineu/finApp-back/internal/recurring"
	"github.com/kgazineu/finApp-back/internal/transaction"
	"github.com/kgazineu/finApp-back/internal/user"
	"github.com/kgazineu/finApp-back/migrations"

	"github.com/gin-gonic/gin"
)

func newRouter(server api.ServerInterface, corsOrigins ...string) *gin.Engine {
	router := gin.Default()
	// There is no trusted proxy configured for this deployment.
	_ = router.SetTrustedProxies(nil)
	// antes das rotas: o Gin só aplica middleware às rotas registradas depois dele
	router.Use(api.CORS(corsOrigins))

	api.RegisterHandlersWithOptions(router, server, api.GinServerOptions{
		ErrorHandler: func(c *gin.Context, _ error, status int) {
			c.JSON(status, api.ErrorResponse{Message: "Parâmetros da requisição inválidos"})
		},
	})
	api.RegisterDocumentation(router)

	return router
}

// registerBalanceModules mounts the balance-tracking modules (accounts, billings,
// recurring transactions and receivables) behind the Bearer session middleware.
// They are plain Gin routes, outside the OpenAPI-generated interface.
func registerBalanceModules(router gin.IRouter, server *api.Server, db *sqlx.DB) {
	protected := router.Group("", server.RequireSession)

	accounts := account.NewModule(db)
	accounts.RegisterRoutes(protected.Group("/accounts"))

	transactions := recurring.NewModule(db)
	transactions.RegisterRoutes(protected.Group("/recurring-transactions"))

	receivables := receivable.NewModule(db)
	receivables.RegisterRoutes(protected.Group("/receivables"))

	billings := billing.NewModule(db, accounts.Service(), transactions.Service(), receivables.Service())
	billings.RegisterRoutes(protected.Group("/billings"))

	// GET /export e POST /import: backup de todos os dados do usuário
	dataexport.NewModule(db).RegisterRoutes(protected)
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
	sessions := auth.NewService(postgres.NewAuthRepository(db), password.Hasher{})
	transactions := transaction.NewService(postgres.NewTransactionRepository(db))
	goals := goal.NewService(postgres.NewGoalRepository(db))
	dashboard := overview.NewService(postgres.NewOverviewRepository(db))
	apiServer := api.NewServer(users, api.WithFinancialServices(sessions, transactions), api.WithGoalService(goals), api.WithOverviewService(dashboard))
	router := newRouter(apiServer, cfg.CORSOrigins...)
	// same pool as GORM; "pgx" is the database/sql driver registered by gorm.io/driver/postgres
	sqlDB := sqlx.NewDb(pool, "pgx")
	registerBalanceModules(router, apiServer, sqlDB)
	passwordreset.NewModule(sqlDB, passwordreset.NewMailer(cfg.SMTP), password.Hasher{}).
		RegisterRoutes(router.Group("/password-resets"))
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
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
