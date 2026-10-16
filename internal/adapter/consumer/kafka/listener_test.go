package kafka_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/twmb/franz-go/pkg/kgo"

	"github.com/netologist/go-worker-template/internal/adapter/consumer/kafka"
	"github.com/netologist/go-worker-template/internal/domain"
)

const (
	testTopic = "tasks"
	testGroup = "worker-grp"
)

type mockKafkaClient struct {
	mu            sync.Mutex
	pollResponses []kgo.Fetches
	committed     []*kgo.Record
	closed        bool
	pingErr       error
}

func (m *mockKafkaClient) PollFetches(ctx context.Context) kgo.Fetches {
	m.mu.Lock()
	if len(m.pollResponses) > 0 {
		resp := m.pollResponses[0]
		m.pollResponses = m.pollResponses[1:]
		m.mu.Unlock()
		return resp
	}
	m.mu.Unlock()

	<-ctx.Done()
	return kgo.NewErrFetch(ctx.Err())
}

func (m *mockKafkaClient) CommitRecords(_ context.Context, rs ...*kgo.Record) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.committed = append(m.committed, rs...)
	return nil
}

func (m *mockKafkaClient) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
}

func (m *mockKafkaClient) Ping(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.pingErr
}

type mockProcessor struct {
	mu        sync.Mutex
	processed []domain.EventMessage
	processFn func(ctx context.Context, msg domain.EventMessage) error
}

func (p *mockProcessor) Process(ctx context.Context, msg domain.EventMessage) error {
	p.mu.Lock()
	p.processed = append(p.processed, msg)
	fn := p.processFn
	p.mu.Unlock()

	if fn != nil {
		return fn(ctx, msg)
	}
	return nil
}

func buildFetches(topic string, partition int32, records ...*kgo.Record) kgo.Fetches {
	return kgo.Fetches{
		{
			Topics: []kgo.FetchTopic{
				{
					Topic: topic,
					Partitions: []kgo.FetchPartition{
						{
							Partition: partition,
							Records:   records,
						},
					},
				},
			},
		},
	}
}

func TestNewListener_Validation(t *testing.T) {
	t.Run("fails when brokers slice is empty", func(t *testing.T) {
		_, err := kafka.NewListener(kafka.Config{
			Brokers: []string{},
			Topics:  []string{testTopic},
			Group:   "test-group",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "broker address")
	})

	t.Run("fails when topics slice is empty", func(t *testing.T) {
		_, err := kafka.NewListener(kafka.Config{
			Brokers: []string{"localhost:9092"},
			Topics:  []string{},
			Group:   "test-group",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "topic")
	})

	t.Run("fails when consumer group is empty", func(t *testing.T) {
		_, err := kafka.NewListener(kafka.Config{
			Brokers: []string{"localhost:9092"},
			Topics:  []string{testTopic},
			Group:   "",
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "consumer group")
	})

	t.Run("succeeds with custom client", func(t *testing.T) {
		client := &mockKafkaClient{}
		listener, err := kafka.NewListener(kafka.Config{
			Client: client,
		})
		require.NoError(t, err)
		assert.NotNil(t, listener)
	})
}

func TestListener_Start_NilProcessor(t *testing.T) {
	client := &mockKafkaClient{}
	listener, err := kafka.NewListener(kafka.Config{
		Client: client,
	})
	require.NoError(t, err)

	err = listener.Start(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processor must not be nil")
}

func TestListener_ProcessSuccess(t *testing.T) {
	client := &mockKafkaClient{}
	processor := &mockProcessor{}

	record := &kgo.Record{
		Key:       []byte("msg-100"),
		Value:     []byte(`{"task_id":"123","action":"process"}`),
		Topic:     testTopic,
		Partition: 0,
		Offset:    42,
		Timestamp: time.Now().UTC(),
		Headers: []kgo.RecordHeader{
			{Key: "type", Value: []byte("task.created")},
			{Key: "correlation_id", Value: []byte("corr-abc")},
		},
	}
	client.pollResponses = []kgo.Fetches{buildFetches(testTopic, 0, record)}

	listener, err := kafka.NewListener(kafka.Config{
		Client:      client,
		Topics:      []string{testTopic},
		Group:       testGroup,
		Concurrency: 2,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = listener.Start(ctx, processor) }()

	require.Eventually(t, func() bool {
		client.mu.Lock()
		defer mUnlock(&client.mu)
		return len(client.committed) == 1
	}, 2*time.Second, 20*time.Millisecond)

	processor.mu.Lock()
	require.Len(t, processor.processed, 1)
	msg := processor.processed[0]
	processor.mu.Unlock()

	assert.Equal(t, "msg-100", msg.Metadata.ID)
	assert.Equal(t, "corr-abc", msg.Metadata.CorrelationID)
	assert.Equal(t, "task.created", msg.Metadata.Type)
	assert.Equal(t, testTopic, msg.Metadata.Source)
	assert.JSONEq(t, `{"task_id":"123","action":"process"}`, string(msg.Payload))

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, listener.Stop(stopCtx))
}

func TestListener_TransientError_SkipsCommit(t *testing.T) {
	client := &mockKafkaClient{}
	processor := &mockProcessor{
		processFn: func(_ context.Context, _ domain.EventMessage) error {
			return domain.ErrTransient
		},
	}

	record := &kgo.Record{
		Key:       []byte("msg-retry"),
		Value:     []byte(`{"task_id":"transient"}`),
		Topic:     testTopic,
		Partition: 1,
		Offset:    10,
	}
	client.pollResponses = []kgo.Fetches{buildFetches(testTopic, 1, record)}

	listener, err := kafka.NewListener(kafka.Config{
		Client:      client,
		Topics:      []string{testTopic},
		Group:       testGroup,
		Concurrency: 1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = listener.Start(ctx, processor) }()

	require.Eventually(t, func() bool {
		processor.mu.Lock()
		defer mUnlock(&processor.mu)
		return len(processor.processed) == 1
	}, 2*time.Second, 20*time.Millisecond)

	client.mu.Lock()
	assert.Empty(t, client.committed)
	client.mu.Unlock()

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, listener.Stop(stopCtx))
}

func TestListener_FatalError_CommitsOffsetAfterDLQ(t *testing.T) {
	client := &mockKafkaClient{}
	processor := &mockProcessor{
		processFn: func(_ context.Context, _ domain.EventMessage) error {
			return errors.New("poison pill json syntax error")
		},
	}

	record := &kgo.Record{
		Key:       []byte("msg-poison"),
		Value:     []byte(`invalid-json`),
		Topic:     testTopic,
		Partition: 0,
		Offset:    99,
	}
	client.pollResponses = []kgo.Fetches{buildFetches(testTopic, 0, record)}

	listener, err := kafka.NewListener(kafka.Config{
		Client:      client,
		Topics:      []string{testTopic},
		Group:       testGroup,
		Concurrency: 1,
	})
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = listener.Start(ctx, processor) }()

	require.Eventually(t, func() bool {
		client.mu.Lock()
		defer mUnlock(&client.mu)
		return len(client.committed) == 1
	}, 2*time.Second, 20*time.Millisecond)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	require.NoError(t, listener.Stop(stopCtx))
}

func mUnlock(mu *sync.Mutex) {
	mu.Unlock()
}
