// Command api runs the DentaCore HTTP backend.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	// The image is built with CGO_ENABLED=0 on a minimal base that has no zoneinfo, and the
	// clinic calendar cannot work without its timezone.
	_ "time/tzdata"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/delivery/rest"
	"github.com/NumerR/ElitStom/internal/platform/auth"
	"github.com/NumerR/ElitStom/internal/platform/httpserver"
	"github.com/NumerR/ElitStom/internal/platform/logger"
	"github.com/NumerR/ElitStom/internal/platform/postgres"
	repopostgres "github.com/NumerR/ElitStom/internal/repository/postgres"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// version is stamped at build time with -ldflags "-X main.version=<ref>".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dentacore: fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// .env is a developer convenience; deployments inject the real environment.
	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	log := logger.New(cfg.Log, os.Stderr)
	log.Info("starting dentacore api",
		slog.String("version", version),
		slog.String("env", string(cfg.Env)),
		slog.String("addr", cfg.HTTP.Addr()),
	)

	ctx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	pool, err := postgres.NewPool(ctx, cfg.Postgres, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	deps, err := routerDeps(cfg, pool, log)
	if err != nil {
		return err
	}

	srv := httpserver.New(cfg.HTTP, rest.NewRouter(deps), log)

	serveErr := make(chan error, 1)
	go func() { serveErr <- srv.Start() }()

	select {
	case err := <-serveErr:
		return err
	case <-ctx.Done():
		log.Info("shutdown signal received")
	}

	// Stop intercepting signals so that a second one kills the process outright.
	stopSignals()

	// ctx is already cancelled by the signal; detach it while keeping its values.
	if err := srv.Shutdown(context.WithoutCancel(ctx)); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-serveErr; err != nil {
		return err
	}

	log.Info("dentacore api stopped")
	return nil
}

// routerDeps is the whole composition root: adapters are built first, then handed to the use
// cases as the ports they declare. Nothing below main imports a concrete driver, which is what
// keeps the dependency rule intact. An error here means the process cannot serve authenticated
// traffic at all, so run() must refuse to start rather than mount half-wired handlers.
func routerDeps(cfg config.Config, pool *pgxpool.Pool, log *slog.Logger) (rest.Deps, error) {
	executor := repopostgres.NewExecutor(pool)
	clock := usecase.SystemClock{}

	// config.Load already rejected a weak or missing secret and an out-of-range cost, so these
	// two only fire on a library change.
	issuer, err := auth.NewTokenIssuer(cfg.Auth, clock.Now)
	if err != nil {
		return rest.Deps{}, err
	}

	hasher, err := auth.NewBcryptHasher(cfg.Auth.BcryptCost)
	if err != nil {
		return rest.Deps{}, err
	}

	users := repopostgres.NewUsers(executor)
	tokens := repopostgres.NewRefreshTokens(executor)

	authService, err := usecase.NewAuth(users, tokens, issuer, hasher, auth.NewSecretGenerator(), clock, usecase.AuthConfig{
		AccessTokenTTL:  cfg.Auth.AccessTokenTTL,
		RefreshTokenTTL: cfg.Auth.RefreshTokenTTL,
	})
	if err != nil {
		return rest.Deps{}, err
	}

	userService, err := usecase.NewUsers(users, tokens, hasher, clock)
	if err != nil {
		return rest.Deps{}, err
	}

	// config.Load validated the calendar already; rebuilding it here only fails on a code change.
	schedule, err := cfg.Clinic.Schedule()
	if err != nil {
		return rest.Deps{}, err
	}

	apptService, err := usecase.NewAppointments(repopostgres.NewAppointments(executor), users, schedule, clock)
	if err != nil {
		return rest.Deps{}, err
	}

	return rest.Deps{
		DB:           pool,
		Log:          log,
		Tokens:       issuer,
		Auth:         authService,
		Users:        userService,
		Appointments: apptService,
	}, nil
}
