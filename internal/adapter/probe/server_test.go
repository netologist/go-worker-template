package probe_test

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/adapter/probe"
)

func setupTestProbeServer(t *testing.T, check probe.CheckFunc) string {
	t.Helper()
	lc := &net.ListenConfig{}
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	tcpAddr, ok := ln.Addr().(*net.TCPAddr)
	require.True(t, ok)

	srv := probe.NewServer(probe.Config{Port: tcpAddr.Port})
	if check != nil {
		srv.RegisterCheck("postgres", check)
	}

	go func() {
		_ = srv.Serve(ln)
	}()

	t.Cleanup(func() {
		_ = ln.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	})

	return "http://" + ln.Addr().String()
}

func TestProbeServer_Liveness(t *testing.T) {
	baseURL := setupTestProbeServer(t, nil)
	client := &http.Client{Timeout: 2 * time.Second}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/livez", nil)
	require.NoError(t, err)

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer func() {
		_ = resp.Body.Close()
	}()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "alive")
}

func TestProbeServer_Readiness(t *testing.T) {
	isHealthy := true
	baseURL := setupTestProbeServer(t, func(ctx context.Context) error {
		if !isHealthy {
			return errors.New("db unreachable")
		}
		return nil
	})
	client := &http.Client{Timeout: 2 * time.Second}

	t.Run("returns 200 when healthy", func(t *testing.T) {
		isHealthy = true
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/readyz", nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()

		assert.Equal(t, http.StatusOK, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "ready")
	})

	t.Run("returns 503 when check fails", func(t *testing.T) {
		isHealthy = false
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, baseURL+"/readyz", nil)
		require.NoError(t, err)

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() {
			_ = resp.Body.Close()
		}()

		assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		assert.Contains(t, string(body), "not ready")
	})
}
