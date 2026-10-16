package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/config"
)

func TestParseFlags(t *testing.T) {
	t.Run("default flags leave config intact", func(t *testing.T) {
		cfg := &config.Config{
			BrokerKind:        config.BrokerKindPostgresOutbox,
			WorkerConcurrency: 10,
		}

		err := parseFlags([]string{}, cfg)
		require.NoError(t, err)
		assert.Equal(t, config.BrokerKindPostgresOutbox, cfg.BrokerKind)
		assert.Equal(t, 10, cfg.WorkerConcurrency)
	})

	t.Run("overriding broker kind flag", func(t *testing.T) {
		cfg := &config.Config{
			BrokerKind:        config.BrokerKindPostgresOutbox,
			WorkerConcurrency: 10,
		}

		err := parseFlags([]string{"--broker-kind=rabbitmq", "--concurrency=20"}, cfg)
		require.NoError(t, err)
		assert.Equal(t, config.BrokerKindRabbitMQ, cfg.BrokerKind)
		assert.Equal(t, 20, cfg.WorkerConcurrency)
	})

	t.Run("invalid broker kind flag", func(t *testing.T) {
		cfg := &config.Config{}
		err := parseFlags([]string{"--broker-kind=unknown"}, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid broker kind")
	})
}

func TestRun_ConfigError(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	err := runWithArgs([]string{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "load config")
}
