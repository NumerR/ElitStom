package postgres

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/platform/logger"
)

const testHost = "127.0.0.1"

// freePort returns a port nothing is listening on, so every connection attempt fails fast.
func freePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", net.JoinHostPort(testHost, "0"))
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	return port
}

func unreachableConfig(port int) config.Postgres {
	return config.Postgres{
		Host:            testHost,
		Port:            port,
		User:            "dentacore",
		Password:        "secret",
		Database:        "dentacore",
		SSLMode:         "disable",
		MaxConns:        2,
		MinConns:        0,
		MaxConnLifetime: time.Hour,
		MaxConnIdleTime: 15 * time.Minute,
		ConnectTimeout:  500 * time.Millisecond,
	}
}

func TestNewPoolFailsFastWhenTheDatabaseIsUnreachable(t *testing.T) {
	cfg := unreachableConfig(freePort(t))

	pool, err := NewPool(context.Background(), cfg, logger.NewNop())

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPingFailed)
	assert.Nil(t, pool, "a pool that never connected must not be handed to the caller")
}

func TestNewPoolRejectsAMalformedDSN(t *testing.T) {
	cfg := config.Postgres{
		URL:            "postgres://dentacore:secret@localhost:5432/dentacore?sslmode=bogus",
		MaxConns:       2,
		ConnectTimeout: time.Second,
	}

	pool, err := NewPool(context.Background(), cfg, logger.NewNop())

	require.Error(t, err)
	assert.ErrorContains(t, err, "parse config")
	assert.NotErrorIs(t, err, ErrPingFailed, "a bad DSN is a configuration error, not a connectivity one")
	assert.Nil(t, pool)
}

func TestNewPoolHonoursParentContextCancellation(t *testing.T) {
	cfg := unreachableConfig(freePort(t))
	// A long connect timeout proves the cancellation, not the timeout, ends the attempt.
	cfg.ConnectTimeout = 30 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	pool, err := NewPool(ctx, cfg, logger.NewNop())
	elapsed := time.Since(start)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrPingFailed)
	assert.Nil(t, pool)
	assert.Less(t, elapsed, 5*time.Second, "startup must abort immediately on a cancelled context")
}
