// Package postgres manages the pgx connection pool.
package postgres

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/NumerR/ElitStom/internal/config"
)

// ErrPingFailed reports a reachable configuration that still refused a health probe.
var ErrPingFailed = errors.New("postgres: ping failed")

// NewPool creates a pool and verifies connectivity before handing it over,
// so a misconfigured DSN fails fast at startup instead of on the first request.
func NewPool(ctx context.Context, cfg config.Postgres, log *slog.Logger) (*pgxpool.Pool, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN())
	if err != nil {
		return nil, fmt.Errorf("postgres: parse config: %w", err)
	}

	poolCfg.MaxConns = cfg.MaxConns
	poolCfg.MinConns = cfg.MinConns
	poolCfg.MaxConnLifetime = cfg.MaxConnLifetime
	poolCfg.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolCfg.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	// statement_timeout is enforced by PostgreSQL on every connection the pool opens, so a
	// query is bounded even when the caller forgot to attach a deadline to the context.
	if cfg.StatementTimeout > 0 {
		poolCfg.ConnConfig.RuntimeParams["statement_timeout"] =
			strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, cfg.ConnectTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("%w: %w", ErrPingFailed, err)
	}

	// Only non-secret coordinates are logged; the DSN embeds the password.
	log.Info("postgres pool initialized",
		slog.String("host", cfg.Host),
		slog.Int("port", cfg.Port),
		slog.String("database", cfg.Database),
		slog.Int("max_conns", int(cfg.MaxConns)),
		slog.Int("min_conns", int(cfg.MinConns)),
		slog.String("statement_timeout", cfg.StatementTimeout.String()),
	)
	return pool, nil
}
