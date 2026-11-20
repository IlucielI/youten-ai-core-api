package rabbitmq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"code-base-golang/internal/config"
	"code-base-golang/internal/pkg/ctxmeta"
)

// Supported exchange types
const (
	ExchangeDirect  = "direct"
	ExchangeFanout  = "fanout"
	ExchangeTopic   = "topic"
	ExchangeHeaders = "headers"
)

// Default exchange and topic queue conventions for event streaming
const (
	HeaderPublishID    = "PublishId"
	DefaultExchange    = "app.events"
	DefaultDLX         = "app.events.dlx"
	DefaultQueuePrefix = "app.queue."
	DefaultDLQPrefix   = "app.dlq."
)

// WorkerHandler is the function signature for handling subscription messages.
// Return true to ACK the message, or false to NACK and route to DLQ.
type WorkerHandler = func(ctx context.Context, msg []byte) (ack bool)

// RabbitMQ wraps amqp.Connection with thread-safe publishing, exchange/queue management, consuming, and DLQ helpers.
type RabbitMQ struct {
	conn               *amqp.Connection
	publishChan        *amqp.Channel
	publishMu          sync.Mutex
	url                string
	dialTimeout        time.Duration
	defaultConcurrency int
	defaultPrefetch    int
	mu                 sync.RWMutex
	isClosed           bool
}

// PublishOption configures AMQP publishing behavior.
type PublishOption func(*amqp.Publishing)

// WithHeaders sets metadata headers on the message.
func WithHeaders(headers amqp.Table) PublishOption {
	return func(p *amqp.Publishing) {
		if p.Headers == nil {
			p.Headers = make(amqp.Table)
		}
		for k, v := range headers {
			p.Headers[k] = v
		}
	}
}

// WithContentType sets the content type of the message.
func WithContentType(contentType string) PublishOption {
	return func(p *amqp.Publishing) {
		p.ContentType = contentType
	}
}

// WithMessageID sets a unique identifier on the message.
func WithMessageID(msgID string) PublishOption {
	return func(p *amqp.Publishing) {
		p.MessageId = msgID
	}
}

// WithExpiration sets per-message TTL (in milliseconds as a string).
func WithExpiration(expiration string) PublishOption {
	return func(p *amqp.Publishing) {
		p.Expiration = expiration
	}
}

// DLQConfig defines configuration for setting up a queue bound to a Dead-Letter Exchange (DLX).
type DLQConfig struct {
	QueueName       string
	ExchangeName    string
	RoutingKey      string
	DLXExchangeName string
	DLQQueueName    string
	DLQRoutingKey   string
	MessageTTL      *int32 // Optional queue-level TTL in milliseconds
}

// MessageHandler is the signature for processing incoming delivery messages.
type MessageHandler func(ctx context.Context, delivery amqp.Delivery) error

// ConsumeConfig defines configuration for running a queue consumer worker.
type ConsumeConfig struct {
	Queue         string
	ConsumerTag   string
	AutoAck       bool
	Exclusive     bool
	NoLocal       bool
	NoWait        bool
	PrefetchCount int
	Concurrency   int // Number of concurrent worker goroutines (default: 1)
	RequeueOnNack bool
	Args          amqp.Table
}

// Worker manages a running subscriber worker pool.
type Worker struct {
	queue       string
	concurrency int
	cancel      context.CancelFunc
	done        chan struct{}
}

// Queue returns the queue name this worker is consuming.
func (w *Worker) Queue() string {
	if w == nil {
		return ""
	}
	return w.queue
}

// Concurrency returns the number of worker goroutines running in this pool.
func (w *Worker) Concurrency() int {
	if w == nil {
		return 0
	}
	return w.concurrency
}

// Stop signals the worker pool to stop and waits for all in-flight jobs to complete.
func (w *Worker) Stop() {
	if w == nil {
		return
	}
	if w.cancel != nil {
		w.cancel()
	}
	if w.done != nil {
		<-w.done
	}
}

// Done returns a channel that is closed when all workers have finished stopping.
func (w *Worker) Done() <-chan struct{} {
	if w == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return w.done
}

// New initializes and connects a RabbitMQ client based on the provided configuration.
func New(cfg config.Config) (*RabbitMQ, error) {
	url := cfg.RabbitMQURL()
	timeout := cfg.RabbitMQDialTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	conn, err := amqp.DialConfig(url, amqp.Config{
		Dial: amqp.DefaultDial(timeout),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq at %s:%s: %w", cfg.RabbitMQHost, cfg.RabbitMQPort, err)
	}

	pubChan, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to open default publisher channel: %w", err)
	}

	concurrency := cfg.WorkerConcurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	prefetch := cfg.WorkerPrefetchCount
	if prefetch <= 0 {
		prefetch = concurrency * 2
	}

	return &RabbitMQ{
		conn:               conn,
		publishChan:        pubChan,
		url:                url,
		dialTimeout:        timeout,
		defaultConcurrency: concurrency,
		defaultPrefetch:    prefetch,
	}, nil
}

// NewWithConnection wraps an existing connection and publisher channel (e.g. for testing).
func NewWithConnection(conn *amqp.Connection, pubChan *amqp.Channel) *RabbitMQ {
	return &RabbitMQ{
		conn:        conn,
		publishChan: pubChan,
	}
}

// Connection returns the underlying *amqp.Connection.
func (r *RabbitMQ) Connection() *amqp.Connection {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.conn
}

// Channel opens and returns a new dedicated *amqp.Channel for customized or isolated operations.
func (r *RabbitMQ) Channel() (*amqp.Channel, error) {
	if r == nil || r.conn == nil {
		return nil, errors.New("rabbitmq connection is nil")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.isClosed || r.conn.IsClosed() {
		return nil, errors.New("rabbitmq connection is closed")
	}
	return r.conn.Channel()
}

// Ping checks if the RabbitMQ connection and publisher channel are alive.
func (r *RabbitMQ) Ping(ctx context.Context) error {
	if r == nil || r.conn == nil {
		return errors.New("rabbitmq connection is nil")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.isClosed || r.conn.IsClosed() {
		return errors.New("rabbitmq connection is closed")
	}

	r.publishMu.Lock()
	defer r.publishMu.Unlock()
	if r.publishChan == nil || r.publishChan.IsClosed() {
		return errors.New("rabbitmq publisher channel is closed or nil")
	}

	return nil
}

// DeclareExchange declares an exchange with the given type, durability, and auto-delete settings.
func (r *RabbitMQ) DeclareExchange(name, kind string, durable, autoDelete bool) error {
	if r == nil {
		return errors.New("rabbitmq client is nil")
	}
	ch, err := r.Channel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()

	return ch.ExchangeDeclare(
		name,
		kind,
		durable,
		autoDelete,
		false, // internal
		false, // noWait
		nil,   // args
	)
}

// DeclareQueue declares a queue with durability, autoDelete, and optional arguments (e.g. DLX, TTL).
func (r *RabbitMQ) DeclareQueue(name string, durable, autoDelete bool, args amqp.Table) (amqp.Queue, error) {
	if r == nil {
		return amqp.Queue{}, errors.New("rabbitmq client is nil")
	}
	ch, err := r.Channel()
	if err != nil {
		return amqp.Queue{}, err
	}
	defer func() { _ = ch.Close() }()

	return ch.QueueDeclare(
		name,
		durable,
		autoDelete,
		false, // exclusive
		false, // noWait
		args,
	)
}

// BindQueue binds a queue to an exchange with a specific routing key.
func (r *RabbitMQ) BindQueue(queueName, routingKey, exchangeName string, args amqp.Table) error {
	if r == nil {
		return errors.New("rabbitmq client is nil")
	}
	ch, err := r.Channel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()

	return ch.QueueBind(
		queueName,
		routingKey,
		exchangeName,
		false, // noWait
		args,
	)
}

// DeclareQueueWithDLQ sets up a complete dead-letter queue pattern:
// 1. Declares the Dead-Letter Exchange (DLX).
// 2. Declares the Dead-Letter Queue (DLQ) and binds it to DLX.
// 3. Declares the main queue configured with `x-dead-letter-exchange` and `x-dead-letter-routing-key`.
// 4. Binds the main queue to the main exchange with routingKey.
func (r *RabbitMQ) DeclareQueueWithDLQ(cfg DLQConfig) error {
	if r == nil {
		return errors.New("rabbitmq client is nil")
	}

	ch, err := r.Channel()
	if err != nil {
		return err
	}
	defer func() { _ = ch.Close() }()

	// 1. Declare DLX Exchange
	if err := ch.ExchangeDeclare(
		cfg.DLXExchangeName,
		ExchangeDirect,
		true,  // durable
		false, // autoDelete
		false, // internal
		false, // noWait
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare dlx exchange: %w", err)
	}

	// 2. Declare DLQ Queue
	if _, err := ch.QueueDeclare(
		cfg.DLQQueueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		nil,
	); err != nil {
		return fmt.Errorf("failed to declare dlq queue: %w", err)
	}

	// Bind DLQ to DLX
	dlqRoutingKey := cfg.DLQRoutingKey
	if dlqRoutingKey == "" {
		dlqRoutingKey = cfg.RoutingKey
	}
	if err := ch.QueueBind(
		cfg.DLQQueueName,
		dlqRoutingKey,
		cfg.DLXExchangeName,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to bind dlq queue to dlx: %w", err)
	}

	// 3. Declare Main Exchange (if specified)
	if cfg.ExchangeName != "" {
		if err := ch.ExchangeDeclare(
			cfg.ExchangeName,
			ExchangeTopic,
			true,  // durable
			false, // autoDelete
			false, // internal
			false, // noWait
			nil,
		); err != nil {
			return fmt.Errorf("failed to declare main exchange: %w", err)
		}
	}

	// 4. Declare Main Queue with DLX arguments
	mainQueueArgs := amqp.Table{
		"x-dead-letter-exchange":    cfg.DLXExchangeName,
		"x-dead-letter-routing-key": dlqRoutingKey,
	}
	if cfg.MessageTTL != nil && *cfg.MessageTTL > 0 {
		mainQueueArgs["x-message-ttl"] = *cfg.MessageTTL
	}

	if _, err := ch.QueueDeclare(
		cfg.QueueName,
		true,  // durable
		false, // autoDelete
		false, // exclusive
		false, // noWait
		mainQueueArgs,
	); err != nil {
		return fmt.Errorf("failed to declare main queue with dlx: %w", err)
	}

	// 5. Bind Main Queue to Main Exchange
	if cfg.ExchangeName != "" {
		if err := ch.QueueBind(
			cfg.QueueName,
			cfg.RoutingKey,
			cfg.ExchangeName,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("failed to bind main queue to main exchange: %w", err)
		}
	}

	return nil
}

// PublishRaw publishes raw byte message to a specific exchange and routing key.
func (r *RabbitMQ) PublishRaw(ctx context.Context, exchange, routingKey string, body []byte, opts ...PublishOption) error {
	if r == nil {
		return errors.New("rabbitmq client is nil")
	}

	r.mu.RLock()
	if r.isClosed || r.conn == nil || r.conn.IsClosed() {
		r.mu.RUnlock()
		return errors.New("rabbitmq connection is closed")
	}
	r.mu.RUnlock()

	r.publishMu.Lock()
	defer r.publishMu.Unlock()

	if r.publishChan == nil || r.publishChan.IsClosed() {
		return errors.New("rabbitmq publisher channel is closed")
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now().UTC(),
		ContentType:  "application/octet-stream",
		Body:         body,
	}

	for _, opt := range opts {
		opt(&msg)
	}

	return r.publishChan.PublishWithContext(
		ctx,
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		msg,
	)
}

// Publish serializes payload to JSON, attaches PublishId/TraceId to header, and publishes to the topic subject.
func (r *RabbitMQ) Publish(ctx context.Context, topic string, payload any) error {
	if r == nil {
		return errors.New("rabbitmq client is nil")
	}

	pubID := ctxmeta.GetRequestID(ctx)
	if pubID == "" {
		pubID = uuid.NewString()
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to serialize payload to json for topic %s: %w", topic, err)
	}

	headers := amqp.Table{
		HeaderPublishID: pubID,
		"x-trace-id":    pubID,
	}

	return r.PublishRaw(
		ctx,
		DefaultExchange,
		topic,
		data,
		WithContentType("application/json"),
		WithMessageID(pubID),
		WithHeaders(headers),
	)
}

// MustPublish publishes data to the topic and panics on failure.
func (r *RabbitMQ) MustPublish(ctx context.Context, topic string, payload any) {
	if err := r.Publish(ctx, topic, payload); err != nil {
		panic(fmt.Errorf("failed to publish to topic '%s': %w", topic, err))
	}
}

// SubscribeOption configures subscription consuming properties such as concurrency and prefetch count.
type SubscribeOption func(*ConsumeConfig)

// WithConcurrency overrides the number of parallel consumer worker goroutines.
func WithConcurrency(concurrency int) SubscribeOption {
	return func(cfg *ConsumeConfig) {
		if concurrency > 0 {
			cfg.Concurrency = concurrency
		}
	}
}

// WithPrefetch overrides the prefetch count for the subscriber channel.
func WithPrefetch(prefetch int) SubscribeOption {
	return func(cfg *ConsumeConfig) {
		if prefetch > 0 {
			cfg.PrefetchCount = prefetch
		}
	}
}

// Subscribe registers a subscription to a topic using WorkerHandler with default worker elasticity.
func (r *RabbitMQ) Subscribe(topic string, handler WorkerHandler) error {
	return r.SubscribeWithOptions(topic, handler)
}

// SubscribeWithOptions registers a subscription with fine-tuned worker concurrency and prefetch options.
// It sets up the queue, DLQ, topic binding on DefaultExchange, and dispatches incoming messages.
func (r *RabbitMQ) SubscribeWithOptions(topic string, handler WorkerHandler, opts ...SubscribeOption) error {
	if r == nil {
		return errors.New("rabbitmq client is nil")
	}

	queueName := DefaultQueuePrefix + topic
	dlqName := DefaultDLQPrefix + topic

	// Ensure Exchange & Queue with DLQ
	if err := r.DeclareQueueWithDLQ(DLQConfig{
		QueueName:       queueName,
		ExchangeName:    DefaultExchange,
		RoutingKey:      topic,
		DLXExchangeName: DefaultDLX,
		DLQQueueName:    dlqName,
		DLQRoutingKey:   topic,
	}); err != nil {
		return fmt.Errorf("failed to declare queue and dlq for topic %s: %w", topic, err)
	}

	concurrency := r.defaultConcurrency
	if concurrency <= 0 {
		concurrency = 4
	}
	prefetch := r.defaultPrefetch
	if prefetch <= 0 {
		prefetch = concurrency * 2
	}

	consumeCfg := ConsumeConfig{
		Queue:         queueName,
		ConsumerTag:   "worker-" + topic,
		PrefetchCount: prefetch,
		Concurrency:   concurrency,
		RequeueOnNack: false, // route to DLQ
	}

	for _, opt := range opts {
		opt(&consumeCfg)
	}

	return r.Consume(context.Background(), consumeCfg, func(ctx context.Context, d amqp.Delivery) error {
		// Set PublishId / RequestID into context
		pubID, _ := d.Headers[HeaderPublishID].(string)
		if pubID == "" {
			pubID = d.MessageId
		}
		if pubID != "" {
			ctx = ctxmeta.WithRequestID(ctx, pubID)
		}

		// Execute worker handler with panic recovery
		var ack bool
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					log.Printf("[RABBITMQ] Panic occurred in Topic: %s. CausedBy=%v\nData: %s",
						topic, rec, string(d.Body))
					ack = false
				}
			}()
			ack = handler(ctx, d.Body)
		}()

		if !ack {
			return errors.New("worker handler returned nack")
		}
		return nil
	})
}

// MustSubscribe registers a subscription to a topic and panics on failure.
func (r *RabbitMQ) MustSubscribe(topic string, handler WorkerHandler) {
	if err := r.Subscribe(topic, handler); err != nil {
		panic(fmt.Errorf("failed to subscribe to topic '%s': %w", topic, err))
	}
}

// MustSubscribeWithOptions registers a subscription with options and panics on failure.
func (r *RabbitMQ) MustSubscribeWithOptions(topic string, handler WorkerHandler, opts ...SubscribeOption) {
	if err := r.SubscribeWithOptions(topic, handler, opts...); err != nil {
		panic(fmt.Errorf("failed to subscribe to topic '%s': %w", topic, err))
	}
}

// PublishJSON serializes payload into JSON and publishes with ContentType application/json.
func (r *RabbitMQ) PublishJSON(ctx context.Context, exchange, routingKey string, payload any, opts ...PublishOption) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal json payload: %w", err)
	}

	jsonOpts := append([]PublishOption{WithContentType("application/json")}, opts...)
	return r.PublishRaw(ctx, exchange, routingKey, data, jsonOpts...)
}

// StartWorker initializes and starts a worker pool for a subscriber in the background, returning a Worker handle.
// It spawns Concurrency worker goroutines, protects against per-message panics, and waits for in-flight tasks on shutdown.
func (r *RabbitMQ) StartWorker(ctx context.Context, cfg ConsumeConfig, handler MessageHandler) (*Worker, error) {
	if r == nil {
		return nil, errors.New("rabbitmq client is nil")
	}

	workerCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = 1
	}

	ch, err := r.Channel()
	if err != nil {
		cancel()
		close(done)
		return nil, fmt.Errorf("failed to open worker consumer channel: %w", err)
	}

	prefetch := cfg.PrefetchCount
	if prefetch <= 0 {
		prefetch = concurrency * 2
	}
	if err := ch.Qos(prefetch, 0, false); err != nil {
		_ = ch.Close()
		cancel()
		close(done)
		return nil, fmt.Errorf("failed to set worker prefetch qos: %w", err)
	}

	deliveries, err := ch.ConsumeWithContext(
		workerCtx,
		cfg.Queue,
		cfg.ConsumerTag,
		cfg.AutoAck,
		cfg.Exclusive,
		cfg.NoLocal,
		cfg.NoWait,
		cfg.Args,
	)
	if err != nil {
		_ = ch.Close()
		cancel()
		close(done)
		return nil, fmt.Errorf("failed to start consuming for worker on queue %s: %w", cfg.Queue, err)
	}

	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for {
				select {
				case <-workerCtx.Done():
					return
				case d, ok := <-deliveries:
					if !ok {
						return
					}

					func() {
						defer func() {
							if rec := recover(); rec != nil {
								if !cfg.AutoAck {
									_ = d.Nack(false, cfg.RequeueOnNack)
								}
							}
						}()

						if cfg.AutoAck {
							_ = handler(workerCtx, d)
							return
						}

						if err := handler(workerCtx, d); err != nil {
							_ = d.Nack(false, cfg.RequeueOnNack)
						} else {
							_ = d.Ack(false)
						}
					}()
				}
			}
		}(i + 1)
	}

	go func() {
		<-workerCtx.Done()
		wg.Wait()
		_ = ch.Close()
		close(done)
	}()

	return &Worker{
		queue:       cfg.Queue,
		concurrency: concurrency,
		cancel:      cancel,
		done:        done,
	}, nil
}

// Consume starts consuming messages from a queue with configurable concurrency and automatic ACK/NACK.
func (r *RabbitMQ) Consume(ctx context.Context, cfg ConsumeConfig, handler MessageHandler) error {
	_, err := r.StartWorker(ctx, cfg, handler)
	return err
}

// Close gracefully closes the publisher channel and connection pool.
func (r *RabbitMQ) Close() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.isClosed {
		return nil
	}
	r.isClosed = true

	var errs []error

	r.publishMu.Lock()
	if r.publishChan != nil && !r.publishChan.IsClosed() {
		if err := r.publishChan.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing publisher channel: %w", err))
		}
	}
	r.publishMu.Unlock()

	if r.conn != nil && !r.conn.IsClosed() {
		if err := r.conn.Close(); err != nil {
			errs = append(errs, fmt.Errorf("error closing connection: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing rabbitmq adapter: %v", errs)
	}

	return nil
}
