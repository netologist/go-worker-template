package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
	"github.com/go-playground/validator/v10"
)

const (
	environmentProduction = "production"

	BrokerKindPostgresOutbox = "postgres_outbox"
	BrokerKindRabbitMQ       = "rabbitmq"
)

type Config struct {
	Environment string `env:"APP_ENV" envDefault:"development" validate:"required,oneof=development staging production test"` //nolint:lll
	LogLevel    string `env:"LOG_LEVEL" envDefault:"info" validate:"required,oneof=debug info warn error"`

	// Broker selection & consumer group
	BrokerKind    string `env:"BROKER_KIND" envDefault:"postgres_outbox" validate:"required,oneof=postgres_outbox rabbitmq"` //nolint:lll
	ConsumerGroup string `env:"CONSUMER_GROUP" envDefault:"default-worker-group" validate:"required"`

	// RabbitMQ settings
	RabbitMQURL           string `env:"RABBITMQ_URL" envDefault:"amqp://guest:guest@localhost:5672/"`
	RabbitMQQueue         string `env:"RABBITMQ_QUEUE" envDefault:"tasks_queue"`
	RabbitMQPrefetchCount int    `env:"RABBITMQ_PREFETCH_COUNT" envDefault:"20" validate:"min=1"`

	// Worker concurrency & execution
	WorkerConcurrency int           `env:"WORKER_CONCURRENCY" envDefault:"10" validate:"required,min=1"`
	BatchSize         int32         `env:"BATCH_SIZE" envDefault:"10" validate:"required,min=1"`
	PollInterval      time.Duration `env:"POLL_INTERVAL" envDefault:"1s" validate:"required"`
	ProcessingTimeout time.Duration `env:"PROCESSING_TIMEOUT" envDefault:"30s" validate:"required"`

	// Retry & DLQ
	MaxRetries int    `env:"MAX_RETRIES" envDefault:"3" validate:"min=0"`
	DLQQueue   string `env:"DLQ_QUEUE_NAME" envDefault:"tasks_dlq"`

	// Idempotency
	IdempotencyTTL time.Duration `env:"IDEMPOTENCY_TTL" envDefault:"168h" validate:"required"`

	// Health/Metrics Probe Server
	ProbePort int `env:"PROBE_PORT" envDefault:"8081" validate:"required,min=1,max=65535"`

	// Lifecycle & Shutdown
	ShutdownTimeout time.Duration `env:"SHUTDOWN_TIMEOUT" envDefault:"30s" validate:"required"`

	// PostgreSQL settings
	DatabaseURL         string        `env:"DATABASE_URL" validate:"required"`
	DBMaxConns          int32         `env:"DB_MAX_CONNS" envDefault:"25" validate:"required,min=1"`
	DBMinConns          int32         `env:"DB_MIN_CONNS" envDefault:"2" validate:"min=0"`
	DBMaxConnLifetime   time.Duration `env:"DB_MAX_CONN_LIFETIME" envDefault:"30m" validate:"required"`
	DBMaxConnIdleTime   time.Duration `env:"DB_MAX_CONN_IDLE_TIME" envDefault:"5m" validate:"required"`
	DBHealthCheckPeriod time.Duration `env:"DB_HEALTH_CHECK_PERIOD" envDefault:"1m" validate:"required"`
	DBPingTimeout       time.Duration `env:"DB_PING_TIMEOUT" envDefault:"5s" validate:"required"`
}

// ProbeAddr returns the probe listening address formatted as ":<port>".
func (c *Config) ProbeAddr() string {
	return fmt.Sprintf(":%d", c.ProbePort)
}

// IsProduction reports whether the environment is production.
func (c *Config) IsProduction() bool {
	return c.Environment == environmentProduction
}

// Load reads configuration from environment variables and validates all fields.
func Load() (*Config, error) {
	cfg := &Config{}
	if err := env.Parse(cfg); err != nil {
		return nil, fmt.Errorf("failed to parse environment variables: %w", err)
	}

	validate := validator.New()
	if err := validate.Struct(cfg); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return cfg, nil
}
