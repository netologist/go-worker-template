package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/netologist/go-worker-template/internal/adapter/consumer"
	"github.com/netologist/go-worker-template/internal/adapter/consumer/outbox"
	"github.com/netologist/go-worker-template/internal/adapter/consumer/rabbitmq"
	"github.com/netologist/go-worker-template/internal/adapter/dlq"
	"github.com/netologist/go-worker-template/internal/adapter/postgres"
	"github.com/netologist/go-worker-template/internal/adapter/probe"
	"github.com/netologist/go-worker-template/internal/config"
	"github.com/netologist/go-worker-template/internal/domain"
	"github.com/netologist/go-worker-template/internal/usecase"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker process terminated with error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	return runWithArgs(os.Args[1:])
}

func runWithArgs(args []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	if err := parseFlags(args, cfg); err != nil {
		return fmt.Errorf("parse flags: %w", err)
	}

	setupLogger(cfg.LogLevel)
	slog.Info("starting background worker process",
		"environment", cfg.Environment,
		"broker_kind", cfg.BrokerKind,
		"consumer_group", cfg.ConsumerGroup,
		"concurrency", cfg.WorkerConcurrency,
		"probe_port", cfg.ProbePort,
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := setupPostgresPool(ctx, cfg)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	slog.Info("connected to PostgreSQL database successfully")

	return runWorkerLoop(ctx, pool, cfg)
}

func parseFlags(args []string, cfg *config.Config) error {
	fs := flag.NewFlagSet("worker", flag.ContinueOnError)
	var brokerKind string
	var concurrency int
	fs.StringVar(&brokerKind, "broker-kind", "", "Broker kind: postgres_outbox or rabbitmq")
	fs.IntVar(&concurrency, "concurrency", 0, "Worker concurrency goroutine count")

	if err := fs.Parse(args); err != nil {
		return err
	}

	if brokerKind != "" {
		if brokerKind != config.BrokerKindPostgresOutbox && brokerKind != config.BrokerKindRabbitMQ {
			return fmt.Errorf("invalid broker kind %q: must be postgres_outbox or rabbitmq", brokerKind)
		}
		cfg.BrokerKind = brokerKind
	}

	if concurrency > 0 {
		cfg.WorkerConcurrency = concurrency
	}

	return nil
}

func setupLogger(levelStr string) {
	var level slog.Level
	switch levelStr {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})
	slog.SetDefault(slog.New(handler))
}

func setupPostgresPool(ctx context.Context, cfg *config.Config) (*pgxpool.Pool, error) {
	poolCfg := postgres.PoolConfig{
		DatabaseURL:       cfg.DatabaseURL,
		MaxConns:          cfg.DBMaxConns,
		MinConns:          cfg.DBMinConns,
		MaxConnLifetime:   cfg.DBMaxConnLifetime,
		MaxConnIdleTime:   cfg.DBMaxConnIdleTime,
		HealthCheckPeriod: cfg.DBHealthCheckPeriod,
		PingTimeout:       cfg.DBPingTimeout,
	}

	return postgres.NewPool(ctx, poolCfg)
}

func runWorkerLoop(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) error {
	processor := buildProcessor(pool, cfg)

	listener, err := buildListener(pool, cfg)
	if err != nil {
		return fmt.Errorf("build listener: %w", err)
	}

	probeSrv := buildProbeServer(pool, cfg)

	g, gCtx := errgroup.WithContext(ctx)

	// 1. Health & metrics probe server
	g.Go(func() error {
		if err := probeSrv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("probe server error: %w", err)
		}
		return nil
	})

	// 2. Inbound consumer execution loop
	g.Go(func() error {
		if err := listener.Start(gCtx, processor); err != nil {
			return fmt.Errorf("consumer listener error: %w", err)
		}
		return nil
	})

	// 3. Graceful shutdown handler
	g.Go(func() error {
		return handleGracefulShutdown(gCtx, listener, probeSrv, cfg)
	})

	return g.Wait()
}

func buildProcessor(pool *pgxpool.Pool, cfg *config.Config) domain.TaskProcessor {
	taskRepo := postgres.NewTaskRepository(pool)
	idempStore := postgres.NewIdempotencyStore(pool, cfg.ConsumerGroup)
	dlqPublisher := dlq.NewLogPublisher(slog.Default())

	return usecase.NewProcessTaskUseCase(
		taskRepo,
		idempStore,
		dlqPublisher,
		cfg.IdempotencyTTL,
		slog.Default(),
	)
}

func buildProbeServer(pool *pgxpool.Pool, cfg *config.Config) *probe.Server {
	probeSrv := probe.NewServer(probe.Config{
		Port:   cfg.ProbePort,
		Logger: slog.Default(),
	})
	probeSrv.RegisterCheck("postgres", func(c context.Context) error {
		return pool.Ping(c)
	})
	return probeSrv
}

func handleGracefulShutdown(
	gCtx context.Context,
	listener consumer.MessageListener,
	probeSrv *probe.Server,
	cfg *config.Config,
) error {
	<-gCtx.Done()
	slog.Info("shutdown signal received; commencing worker drain sequence")

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(gCtx), cfg.ShutdownTimeout)
	defer cancel()

	var shutdownErr error
	if err := listener.Stop(shutdownCtx); err != nil {
		slog.Error("error draining consumer listener", "error", err)
		shutdownErr = err
	}

	if err := probeSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("error shutting down probe server", "error", err)
		shutdownErr = err
	}

	return shutdownErr
}

func buildListener(pool *pgxpool.Pool, cfg *config.Config) (consumer.MessageListener, error) {
	switch cfg.BrokerKind {
	case config.BrokerKindRabbitMQ:
		return rabbitmq.NewListener(rabbitmq.Config{
			URL:           cfg.RabbitMQURL,
			QueueName:     cfg.RabbitMQQueue,
			PrefetchCount: cfg.RabbitMQPrefetchCount,
			Concurrency:   cfg.WorkerConcurrency,
			Logger:        slog.Default(),
		}), nil

	case config.BrokerKindPostgresOutbox:
		return outbox.NewListener(pool, outbox.Config{
			Concurrency:  cfg.WorkerConcurrency,
			PollInterval: cfg.PollInterval,
			BatchSize:    cfg.BatchSize,
			Logger:       slog.Default(),
		}), nil

	default:
		return nil, fmt.Errorf("unsupported broker kind: %s", cfg.BrokerKind)
	}
}

// Compile-time assertion for domain.TaskProcessor interface
var _ domain.TaskProcessor = (*usecase.ProcessTaskUseCase)(nil)
