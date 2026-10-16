package rabbitmq

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/netologist/go-worker-template/internal/adapter/consumer"
	"github.com/netologist/go-worker-template/internal/domain"
)

// Listener implements consumer.MessageListener for RabbitMQ.
type Listener struct {
	url           string
	queueName     string
	prefetchCount int
	concurrency   int
	logger        *slog.Logger

	mu        sync.Mutex
	conn      *amqp.Connection
	ch        *amqp.Channel
	cancel    context.CancelFunc
	drainDone chan struct{}
}

// Config holds RabbitMQ consumer settings.
type Config struct {
	URL           string
	QueueName     string
	PrefetchCount int
	Concurrency   int
	Logger        *slog.Logger
}

// NewListener initializes a new RabbitMQ consumer listener.
func NewListener(cfg Config) *Listener {
	if cfg.PrefetchCount <= 0 {
		cfg.PrefetchCount = 20
	}
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	return &Listener{
		url:           cfg.URL,
		queueName:     cfg.QueueName,
		prefetchCount: cfg.PrefetchCount,
		concurrency:   cfg.Concurrency,
		logger:        cfg.Logger,
		drainDone:     make(chan struct{}),
	}
}

// Start connects to RabbitMQ and starts consuming deliveries.
func (l *Listener) Start(ctx context.Context, processor domain.TaskProcessor) error {
	runCtx, cancel := context.WithCancel(ctx)
	l.mu.Lock()
	l.cancel = cancel
	l.mu.Unlock()

	defer close(l.drainDone)

	deliveries, err := l.connectAndConsume()
	if err != nil {
		return err
	}
	defer func() {
		if l.ch != nil {
			_ = l.ch.Close()
		}
		if l.conn != nil {
			_ = l.conn.Close()
		}
	}()

	l.logger.InfoContext(runCtx, "started rabbitmq consumer loop",
		"queue", l.queueName,
		"prefetch", l.prefetchCount,
		"concurrency", l.concurrency,
	)

	return l.consumeLoop(runCtx, processor, deliveries)
}

func (l *Listener) connectAndConsume() (<-chan amqp.Delivery, error) {
	conn, err := amqp.Dial(l.url)
	if err != nil {
		return nil, fmt.Errorf("failed to dial rabbitmq: %w", err)
	}
	l.conn = conn

	ch, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to open rabbitmq channel: %w", err)
	}
	l.ch = ch

	if err := ch.Qos(l.prefetchCount, 0, false); err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("failed to set rabbitmq Qos prefetch: %w", err)
	}

	deliveries, err := ch.Consume(
		l.queueName,
		"",    // consumer tag
		false, // autoAck = false
		false, // exclusive
		false, // noLocal
		false, // noWait
		nil,   // args
	)
	if err != nil {
		_ = ch.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("failed to register rabbitmq consumer: %w", err)
	}

	return deliveries, nil
}

func (l *Listener) consumeLoop(
	ctx context.Context,
	processor domain.TaskProcessor,
	deliveries <-chan amqp.Delivery,
) error {
	sem := make(chan struct{}, l.concurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			l.logger.Info("rabbitmq consumer stopping; waiting for in-flight messages to drain")
			wg.Wait()
			return nil

		case d, ok := <-deliveries:
			if !ok {
				l.logger.Warn("rabbitmq deliveries channel closed")
				wg.Wait()
				return nil
			}

			sem <- struct{}{}
			wg.Add(1)

			delivery := d
			go func() {
				defer func() {
					<-sem
					wg.Done()
				}()
				l.handleDelivery(ctx, processor, delivery)
			}()
		}
	}
}

func (l *Listener) handleDelivery(ctx context.Context, processor domain.TaskProcessor, delivery amqp.Delivery) {
	headers := make(map[string]string)
	for k, v := range delivery.Headers {
		if strVal, ok := v.(string); ok {
			headers[k] = strVal
		}
	}

	msgID := delivery.MessageId
	if msgID == "" {
		msgID = delivery.CorrelationId
	}

	eventMsg := domain.EventMessage{
		Metadata: domain.MessageMetadata{
			ID:            msgID,
			CorrelationID: delivery.CorrelationId,
			Type:          delivery.Type,
			Timestamp:     delivery.Timestamp,
			Headers:       headers,
		},
		Payload: delivery.Body,
	}

	procErr := consumer.SafeProcess(ctx, processor, eventMsg, l.logger)
	if procErr == nil {
		_ = delivery.Ack(false)
		return
	}

	if errors.Is(procErr, domain.ErrTransient) {
		l.logger.WarnContext(ctx, "transient error; requeuing message for retry", "message_id", msgID)
		_ = delivery.Nack(false, true)
	} else {
		l.logger.ErrorContext(ctx, "fatal error / poison pill; rejecting message to DLX",
			"message_id", msgID,
			"error", procErr,
		)
		_ = delivery.Reject(false)
	}
}

// Stop gracefully shuts down the listener.
func (l *Listener) Stop(ctx context.Context) error {
	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	select {
	case <-l.drainDone:
		l.logger.Info("rabbitmq listener successfully drained and stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for rabbitmq listener to drain: %w", ctx.Err())
	}
}
