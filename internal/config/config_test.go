package config_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netologist/go-worker-template/internal/config"
)

func TestLoad_Defaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/testdb")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "development", cfg.Environment)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, config.BrokerKindPostgresOutbox, cfg.BrokerKind)
	assert.Equal(t, "default-worker-group", cfg.ConsumerGroup)
	assert.Equal(t, 10, cfg.WorkerConcurrency)
	assert.Equal(t, int32(10), cfg.BatchSize)
	assert.Equal(t, 1*time.Second, cfg.PollInterval)
	assert.Equal(t, 30*time.Second, cfg.ProcessingTimeout)
	assert.Equal(t, 8081, cfg.ProbePort)
	assert.Equal(t, ":8081", cfg.ProbeAddr())
	assert.Equal(t, 30*time.Second, cfg.ShutdownTimeout)
	assert.Equal(t, 168*time.Hour, cfg.IdempotencyTTL)
	assert.Equal(t, []string{"localhost:9092"}, cfg.KafkaBrokers)
	assert.Equal(t, []string{"tasks"}, cfg.KafkaTopics)
	assert.False(t, cfg.IsProduction())
}

func TestLoad_CustomEnv(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/proddb")
	t.Setenv("APP_ENV", "production")
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("BROKER_KIND", "rabbitmq")
	t.Setenv("CONSUMER_GROUP", "orders-group")
	t.Setenv("WORKER_CONCURRENCY", "25")
	t.Setenv("PROBE_PORT", "9090")

	cfg, err := config.Load()
	require.NoError(t, err)

	assert.Equal(t, "production", cfg.Environment)
	assert.True(t, cfg.IsProduction())
	assert.Equal(t, "warn", cfg.LogLevel)
	assert.Equal(t, "rabbitmq", cfg.BrokerKind)
	assert.Equal(t, "orders-group", cfg.ConsumerGroup)
	assert.Equal(t, 25, cfg.WorkerConcurrency)
	assert.Equal(t, 9090, cfg.ProbePort)
	assert.Equal(t, ":9090", cfg.ProbeAddr())
}

func TestLoad_KafkaAndRedpanda(t *testing.T) {
	t.Run("broker kind kafka with custom brokers and topics", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
		t.Setenv("BROKER_KIND", "kafka")
		t.Setenv("KAFKA_BROKERS", "broker1:9092,broker2:9092")
		t.Setenv("KAFKA_TOPICS", "events,notifications")

		cfg, err := config.Load()
		require.NoError(t, err)

		assert.Equal(t, config.BrokerKindKafka, cfg.BrokerKind)
		assert.Equal(t, []string{"broker1:9092", "broker2:9092"}, cfg.KafkaBrokers)
		assert.Equal(t, []string{"events", "notifications"}, cfg.KafkaTopics)
	})

	t.Run("broker kind redpanda", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://user:pass@localhost:5432/db")
		t.Setenv("BROKER_KIND", "redpanda")

		cfg, err := config.Load()
		require.NoError(t, err)

		assert.Equal(t, config.BrokerKindRedpanda, cfg.BrokerKind)
	})
}

func TestLoad_ValidationErrors(t *testing.T) {
	t.Run("missing database url", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "")
		_, err := config.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "DatabaseURL")
	})

	t.Run("invalid broker kind", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/db")
		t.Setenv("BROKER_KIND", "unsupported_broker")
		_, err := config.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "BrokerKind")
	})

	t.Run("invalid probe port", func(t *testing.T) {
		t.Setenv("DATABASE_URL", "postgres://localhost/db")
		t.Setenv("PROBE_PORT", "99999")
		_, err := config.Load()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "ProbePort")
	})
}
