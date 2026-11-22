package workers

import (
	"context"
	"time"
)

// WorkerHandler is the function signature for handling domain event messages.
// Return true to ACK the message, or false to NACK and route to DLQ/retry.
type WorkerHandler = func(ctx context.Context, msg []byte) (ack bool)

// SubscribeConfig carries tuning options for subscription consumers.
type SubscribeConfig struct {
	Concurrency int
	MinWorkers  int
	Prefetch    int
	IdleTimeout time.Duration
}

// SubscribeOption is a functional option for configuring a topic subscription.
type SubscribeOption func(*SubscribeConfig)

// WithConcurrency overrides the maximum parallel worker goroutines.
func WithConcurrency(concurrency int) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		if concurrency > 0 {
			cfg.Concurrency = concurrency
		}
	}
}

// WithMinWorkers overrides the minimum standby worker goroutines.
func WithMinWorkers(min int) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		if min >= 0 {
			cfg.MinWorkers = min
		}
	}
}

// WithPrefetch overrides the channel prefetch count.
func WithPrefetch(prefetch int) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		if prefetch > 0 {
			cfg.Prefetch = prefetch
		}
	}
}

// WithIdleTimeout overrides the idle auto-reap duration.
func WithIdleTimeout(timeout time.Duration) SubscribeOption {
	return func(cfg *SubscribeConfig) {
		if timeout > 0 {
			cfg.IdleTimeout = timeout
		}
	}
}

// EventSubscriber defines the abstract contract for message broker subscriptions.
// Any message broker adapter (RabbitMQ, NATS, Kafka) that provides these methods
// can be used to drive background workers without changing worker logic.
type EventSubscriber interface {
	// Subscribe registers a worker handler for the given topic/subject.
	Subscribe(topic string, handler WorkerHandler) error

	// MustSubscribe registers a handler and panics on configuration or binding failure.
	MustSubscribe(topic string, handler WorkerHandler)
}

// ConfigurableEventSubscriber is an optional extension interface implemented by message brokers
// that support fine-tuned worker concurrency, prefetch, and elastic idle auto-reaping.
type ConfigurableEventSubscriber interface {
	EventSubscriber

	// SubscribeWithWorkerOptions registers a subscription with fine-tuned concurrency, prefetch, and idle reap options.
	SubscribeWithWorkerOptions(topic string, handler WorkerHandler, opts ...SubscribeOption) error

	// MustSubscribeWithWorkerOptions registers a subscription with options and panics on failure.
	MustSubscribeWithWorkerOptions(topic string, handler WorkerHandler, opts ...SubscribeOption)
}

// DrainableEventSubscriber is an optional interface implemented by message brokers
// that support draining background worker pools gracefully on shutdown.
type DrainableEventSubscriber interface {
	ShutdownWorkers(ctx context.Context) error
}
