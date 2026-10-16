package kafka

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/netologist/go-worker-template/internal/adapter/consumer"
	"github.com/netologist/go-worker-template/internal/domain"
)

// KafkaClient abstracts the franz-go kgo.Client operations for testing and decoupling.
type KafkaClient interface {
	PollFetches(ctx context.Context) kgo.Fetches
	CommitRecords(ctx context.Context, rs ...*kgo.Record) error
	Close()
	Ping(ctx context.Context) error
}

// Listener implements consumer.MessageListener for Redpanda / Apache Kafka brokers.
type Listener struct {
	client      KafkaClient
	topics      []string
	group       string
	concurrency int
	logger      *slog.Logger

	mu        sync.Mutex
	cancel    context.CancelFunc
	drainDone chan struct{}
}

// Config holds configuration parameters for the Kafka / Redpanda listener.
type Config struct {
	Brokers     []string
	Topics      []string
	Group       string
	Concurrency int
	Logger      *slog.Logger
	// Client is optional. If provided, it overrides automatic kgo.Client creation (useful in tests).
	Client KafkaClient
}

// NewListener creates and initializes a new Kafka/Redpanda consumer listener.
func NewListener(cfg Config) (*Listener, error) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = 10
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	client := cfg.Client
	if client == nil {
		if len(cfg.Brokers) == 0 {
			return nil, errors.New("kafka listener requires at least one broker address")
		}
		if len(cfg.Topics) == 0 {
			return nil, errors.New("kafka listener requires at least one topic to consume")
		}
		if cfg.Group == "" {
			return nil, errors.New("kafka listener requires a non-empty consumer group")
		}

		opts := []kgo.Opt{
			kgo.SeedBrokers(cfg.Brokers...),
			kgo.ConsumerGroup(cfg.Group),
			kgo.ConsumeTopics(cfg.Topics...),
			kgo.DisableAutoCommit(),
		}

		kgoClient, err := kgo.NewClient(opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create kafka client: %w", err)
		}
		client = kgoClient
	}

	return &Listener{
		client:      client,
		topics:      cfg.Topics,
		group:       cfg.Group,
		concurrency: cfg.Concurrency,
		logger:      cfg.Logger,
		drainDone:   make(chan struct{}),
	}, nil
}

// Start begins polling and consuming messages from Kafka/Redpanda, dispatching to processor.
func (l *Listener) Start(ctx context.Context, processor domain.TaskProcessor) error {
	if processor == nil {
		return errors.New("task processor must not be nil")
	}

	l.mu.Lock()
	loopCtx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	l.mu.Unlock()

	defer func() {
		l.client.Close()
		close(l.drainDone)
	}()

	l.logger.Info("starting kafka/redpanda consumer loop",
		"topics", l.topics,
		"group", l.group,
		"concurrency", l.concurrency,
	)

	return l.consumeLoop(loopCtx, processor)
}

func (l *Listener) consumeLoop(ctx context.Context, processor domain.TaskProcessor) error {
	sem := make(chan struct{}, l.concurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			l.logger.Info("kafka consumer stopping; waiting for in-flight messages to drain")
			wg.Wait()
			return nil
		default:
		}

		fetches := l.client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			l.logger.Info("kafka client closed; stopping consume loop")
			wg.Wait()
			return nil
		}

		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				l.logger.Info("kafka poll context canceled; draining in-flight workers")
				wg.Wait()
				return nil
			}
			l.logger.WarnContext(ctx, "kafka poll returned error", "error", err)
			continue
		}

		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()

			select {
			case <-ctx.Done():
				wg.Wait()
				return nil
			case sem <- struct{}{}:
			}

			wg.Add(1)
			rec := record
			go func() {
				defer func() {
					<-sem
					wg.Done()
				}()
				l.handleRecord(ctx, processor, rec)
			}()
		}
	}
}

func (l *Listener) handleRecord(ctx context.Context, processor domain.TaskProcessor, record *kgo.Record) {
	eventMsg := extractEventMessage(record)
	msgID := eventMsg.Metadata.ID

	procErr := consumer.SafeProcess(ctx, processor, eventMsg, l.logger)
	if procErr == nil {
		l.commitRecord(ctx, record, msgID)
		return
	}

	if errors.Is(procErr, domain.ErrTransient) {
		l.logger.WarnContext(ctx, "transient processing error; skipping commit to allow retry",
			"message_id", msgID,
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"error", procErr,
		)
		return
	}

	// Fatal error or poison pill: usecase has already routed to DLQ; commit offset so partition is not blocked.
	l.logger.ErrorContext(ctx, "fatal error / poison pill; committing offset after DLQ routing",
		"message_id", msgID,
		"topic", record.Topic,
		"partition", record.Partition,
		"offset", record.Offset,
		"error", procErr,
	)
	l.commitRecord(ctx, record, msgID)
}

func (l *Listener) commitRecord(ctx context.Context, record *kgo.Record, msgID string) {
	if err := l.client.CommitRecords(ctx, record); err != nil {
		l.logger.ErrorContext(ctx, "failed to commit kafka record offset",
			"error", err,
			"topic", record.Topic,
			"partition", record.Partition,
			"offset", record.Offset,
			"message_id", msgID,
		)
	}
}

func extractEventMessage(record *kgo.Record) domain.EventMessage {
	headers := make(map[string]string, len(record.Headers))
	for _, h := range record.Headers {
		headers[h.Key] = string(h.Value)
	}

	msgID := string(record.Key)
	if msgID == "" {
		if id, ok := headers["id"]; ok && id != "" {
			msgID = id
		} else if corrID, ok := headers["correlation_id"]; ok && corrID != "" {
			msgID = corrID
		} else {
			msgID = fmt.Sprintf("%s-%d-%d", record.Topic, record.Partition, record.Offset)
		}
	}

	correlationID := headers["correlation_id"]
	if correlationID == "" {
		correlationID = msgID
	}

	return domain.EventMessage{
		Metadata: domain.MessageMetadata{
			ID:            msgID,
			CorrelationID: correlationID,
			Source:        record.Topic,
			Type:          headers["type"],
			Timestamp:     record.Timestamp,
			Headers:       headers,
		},
		Payload: record.Value,
	}
}

// Stop gracefully signals the consumer loop to exit and waits for in-flight tasks to complete.
func (l *Listener) Stop(ctx context.Context) error {
	l.mu.Lock()
	cancel := l.cancel
	l.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	select {
	case <-l.drainDone:
		l.logger.Info("kafka listener successfully drained and stopped")
		return nil
	case <-ctx.Done():
		return fmt.Errorf("timeout waiting for kafka listener to drain: %w", ctx.Err())
	}
}

// Compile-time check ensuring Listener implements consumer.MessageListener.
var _ consumer.MessageListener = (*Listener)(nil)
