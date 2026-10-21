// Command seed creates demo accounts for local development. It is idempotent and refuses to
// run against a production environment.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/platform/auth"
	"github.com/NumerR/ElitStom/internal/platform/logger"
	"github.com/NumerR/ElitStom/internal/platform/postgres"
	repopostgres "github.com/NumerR/ElitStom/internal/repository/postgres"
)

const defaultPassword = "dentacore-demo-2026"

type account struct {
	Email    string
	FullName string
	Phone    string
	Role     domain.Role
}

var demoAccounts = []account{
	{Email: "admin@dentacore.local", FullName: "Ирина Администратор", Phone: "+7 900 000-00-01", Role: domain.RoleAdmin},
	{Email: "ivanov@dentacore.local", FullName: "Алексей Иванов", Phone: "+7 900 000-00-11", Role: domain.RoleDoctor},
	{Email: "petrova@dentacore.local", FullName: "Мария Петрова", Phone: "+7 900 000-00-12", Role: domain.RoleDoctor},
	{Email: "sidorov@dentacore.local", FullName: "Олег Сидоров", Phone: "+7 900 000-00-13", Role: domain.RoleDoctor},
	{Email: "patient@dentacore.local", FullName: "Анна Смирнова", Phone: "+7 900 000-00-21", Role: domain.RolePatient},
	{Email: "patient2@dentacore.local", FullName: "Дмитрий Кузнецов", Phone: "+7 900 000-00-22", Role: domain.RolePatient},
}

type userCreator interface {
	Create(ctx context.Context, user domain.User) (domain.User, error)
}

type passwordHasher interface {
	Hash(password string) (string, error)
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "dentacore seed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("seed", flag.ContinueOnError)
	password := flags.String("password", defaultPassword, "password for every demo account")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if err := godotenv.Load(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load .env: %w", err)
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Env.IsProduction() {
		return errors.New("refusing to seed demo accounts in production")
	}

	log := logger.New(cfg.Log, os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.Postgres, log)
	if err != nil {
		return err
	}
	defer pool.Close()

	hasher, err := auth.NewBcryptHasher(cfg.Auth.BcryptCost)
	if err != nil {
		return err
	}

	users := repopostgres.NewUsers(repopostgres.NewExecutor(pool))
	created, err := seed(ctx, users, hasher, demoAccounts, *password, log)
	if err != nil {
		return err
	}

	if _, err := fmt.Fprintf(out, "created %d of %d demo accounts; password: %s\n", created, len(demoAccounts), *password); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

// seed creates every missing account and reports how many were new. An existing email is
// skipped rather than overwritten, so re-running never resets a password someone changed.
func seed(ctx context.Context, users userCreator, hasher passwordHasher, accounts []account, password string, log *slog.Logger) (int, error) {
	if err := domain.ValidatePasswordStrength(password); err != nil {
		return 0, err
	}
	hash, err := hasher.Hash(password)
	if err != nil {
		return 0, fmt.Errorf("hash password: %w", err)
	}

	created := 0
	for _, a := range accounts {
		user, err := domain.NewUser(domain.UserParams{
			Email: a.Email, FullName: a.FullName, Phone: a.Phone, Role: a.Role, PasswordHash: hash,
		})
		if err != nil {
			return created, fmt.Errorf("account %s: %w", a.Email, err)
		}

		stored, err := users.Create(ctx, *user)
		switch {
		case errors.Is(err, domain.ErrAlreadyExists):
			log.Info("account exists, skipped", slog.String("email", user.Email))
			continue
		case err != nil:
			return created, fmt.Errorf("create %s: %w", a.Email, err)
		}

		created++
		log.Info("account created", slog.String("email", stored.Email), slog.String("role", string(stored.Role)))
	}
	return created, nil
}
