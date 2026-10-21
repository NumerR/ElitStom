package httpserver

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/config"
	"github.com/NumerR/ElitStom/internal/platform/logger"
)

const testHost = "127.0.0.1"

// freePort reserves and immediately releases an ephemeral port. The tiny window in
// which another process could grab it is the standard trade-off for testing a real listener.
func freePort(t *testing.T) int {
	t.Helper()

	l, err := net.Listen("tcp", net.JoinHostPort(testHost, "0"))
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	require.NoError(t, l.Close())

	return port
}

func testConfig(port int, shutdownTimeout time.Duration) config.HTTP {
	return config.HTTP{
		Host:              testHost,
		Port:              port,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       30 * time.Second,
		ShutdownTimeout:   shutdownTimeout,
	}
}

func addrOf(port int) string { return net.JoinHostPort(testHost, strconv.Itoa(port)) }

func waitForListener(t *testing.T, addr string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			require.NoError(t, conn.Close())
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("server never started listening on %s", addr)
}

func TestShutdownDrainsInFlightRequests(t *testing.T) {
	port := freePort(t)

	inFlight := make(chan struct{})
	releaseCh := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseCh) }) }
	defer release()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight)
		<-releaseCh
		w.WriteHeader(http.StatusOK)
	})

	srv := New(testConfig(port, 10*time.Second), handler, logger.NewNop())

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start() }()
	waitForListener(t, addrOf(port))

	type response struct {
		status int
		err    error
	}
	respCh := make(chan response, 1)
	go func() {
		resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addrOf(port)))
		if err != nil {
			respCh <- response{err: err}
			return
		}
		defer func() {
			if closeErr := resp.Body.Close(); closeErr != nil {
				t.Errorf("close response body: %v", closeErr)
			}
		}()
		respCh <- response{status: resp.StatusCode}
	}()

	select {
	case <-inFlight:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the handler")
	}

	shutdownErr := make(chan error, 1)
	go func() { shutdownErr <- srv.Shutdown(context.Background()) }()

	// The request is mid-flight: Shutdown must wait for it rather than drop it.
	release()

	select {
	case err := <-shutdownErr:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		t.Fatal("Shutdown did not return")
	}

	require.NoError(t, <-startErr, "Start must report a graceful shutdown as success")

	select {
	case res := <-respCh:
		require.NoError(t, res.err)
		assert.Equal(t, http.StatusOK, res.status, "the drained request must still receive its response")
	case <-time.After(5 * time.Second):
		t.Fatal("the in-flight request never completed")
	}
}

func TestShutdownForcesCloseOnceTheTimeoutExpires(t *testing.T) {
	port := freePort(t)

	inFlight := make(chan struct{})
	release := make(chan struct{})
	defer close(release)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(inFlight)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	srv := New(testConfig(port, 150*time.Millisecond), handler, logger.NewNop())

	startErr := make(chan error, 1)
	go func() { startErr <- srv.Start() }()
	waitForListener(t, addrOf(port))

	go func() {
		resp, err := http.Get(fmt.Sprintf("http://%s/slow", addrOf(port)))
		if err != nil {
			// Expected: the forced close aborts this request.
			return
		}
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Logf("close response body: %v", closeErr)
		}
	}()

	select {
	case <-inFlight:
	case <-time.After(5 * time.Second):
		t.Fatal("the request never reached the handler")
	}

	err := srv.Shutdown(context.Background())

	require.Error(t, err, "a stuck request must surface as a shutdown failure")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	require.NoError(t, <-startErr, "Start must still exit after a forced close")
}

func TestStartFailsWhenThePortIsAlreadyTaken(t *testing.T) {
	port := freePort(t)

	blocker, err := net.Listen("tcp", addrOf(port))
	require.NoError(t, err)
	defer func() { require.NoError(t, blocker.Close()) }()

	srv := New(testConfig(port, time.Second), http.NotFoundHandler(), logger.NewNop())

	err = srv.Start()

	require.Error(t, err)
	assert.ErrorContains(t, err, "address already in use")
}
