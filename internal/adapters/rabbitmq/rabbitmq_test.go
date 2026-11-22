package rabbitmq_test

import (
	"context"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"code-base-golang/internal/adapters/rabbitmq"
	"code-base-golang/internal/config"
)

func TestRabbitMQ_New_Unreachable(t *testing.T) {
	cfg := config.Config{
		RabbitMQHost:        "127.0.0.1",
		RabbitMQPort:        "59998", // non-existent port
		RabbitMQUser:        "guest",
		RabbitMQPassword:    "guest",
		RabbitMQVHost:       "/",
		RabbitMQDialTimeout: 50 * time.Millisecond,
	}

	client, err := rabbitmq.New(cfg)
	if err == nil {
		t.Fatal("expected error when connecting to unreachable rabbitmq, got nil")
	}
	if client != nil {
		t.Fatal("expected client to be nil on failed connection")
	}
}

func TestRabbitMQ_NilReceiver(t *testing.T) {
	var r *rabbitmq.RabbitMQ
	ctx := context.Background()

	if err := r.Ping(ctx); err == nil {
		t.Error("expected error calling Ping on nil RabbitMQ, got nil")
	}

	if r.Connection() != nil {
		t.Error("expected nil *amqp.Connection from nil RabbitMQ")
	}

	if _, err := r.Channel(); err == nil {
		t.Error("expected error calling Channel on nil RabbitMQ, got nil")
	}

	if err := r.DeclareExchange("test_exchange", rabbitmq.ExchangeDirect, true, false); err == nil {
		t.Error("expected error calling DeclareExchange on nil RabbitMQ, got nil")
	}

	if _, err := r.DeclareQueue("test_queue", true, false, nil); err == nil {
		t.Error("expected error calling DeclareQueue on nil RabbitMQ, got nil")
	}

	if err := r.BindQueue("test_queue", "test_key", "test_exchange", nil); err == nil {
		t.Error("expected error calling BindQueue on nil RabbitMQ, got nil")
	}

	if err := r.DeclareQueueWithDLQ(rabbitmq.DLQConfig{
		QueueName:       "main_q",
		ExchangeName:    "main_ex",
		RoutingKey:      "task.process",
		DLXExchangeName: "dlx_ex",
		DLQQueueName:    "dlq_q",
	}); err == nil {
		t.Error("expected error calling DeclareQueueWithDLQ on nil RabbitMQ, got nil")
	}

	if err := r.PublishRaw(ctx, "test_ex", "test_key", []byte("hello")); err == nil {
		t.Error("expected error calling PublishRaw on nil RabbitMQ, got nil")
	}

	if err := r.Publish(ctx, "test_topic", map[string]string{"msg": "hi"}); err == nil {
		t.Error("expected error calling Publish on nil RabbitMQ, got nil")
	}

	if err := r.Subscribe("test_topic", func(ctx context.Context, msg []byte) bool { return true }); err == nil {
		t.Error("expected error calling Subscribe on nil RabbitMQ, got nil")
	}

	if err := r.PublishJSON(ctx, "test_ex", "test_key", map[string]string{"msg": "hi"}); err == nil {
		t.Error("expected error calling PublishJSON on nil RabbitMQ, got nil")
	}

	if err := r.Consume(ctx, rabbitmq.ConsumeConfig{Queue: "test_q"}, func(ctx context.Context, d amqp.Delivery) error {
		return nil
	}); err == nil {
		t.Error("expected error calling Consume on nil RabbitMQ, got nil")
	}

	if _, err := r.StartWorker(ctx, rabbitmq.ConsumeConfig{Queue: "test_q"}, func(ctx context.Context, d amqp.Delivery) error {
		return nil
	}); err == nil {
		t.Error("expected error calling StartWorker on nil RabbitMQ, got nil")
	}

	if err := r.ShutdownWorkers(ctx); err != nil {
		t.Errorf("expected nil error calling ShutdownWorkers on nil RabbitMQ, got %v", err)
	}

	if err := r.Close(); err != nil {
		t.Errorf("expected nil error calling Close on nil RabbitMQ, got %v", err)
	}
}

func TestRabbitMQ_Worker_Nil(t *testing.T) {
	var w *rabbitmq.Worker
	if w.Queue() != "" {
		t.Errorf("expected empty queue name on nil Worker, got %s", w.Queue())
	}
	if w.Concurrency() != 0 {
		t.Errorf("expected 0 concurrency on nil Worker, got %d", w.Concurrency())
	}
	if w.ActiveWorkers() != 0 {
		t.Errorf("expected 0 active workers on nil Worker, got %d", w.ActiveWorkers())
	}
	w.Stop() // should not panic
	select {
	case <-w.Done():
	case <-time.After(100 * time.Millisecond):
		t.Error("expected Done channel to be closed immediately on nil Worker")
	}
}

func TestRabbitMQ_PublishOptions(t *testing.T) {
	pub := amqp.Publishing{}

	opt1 := rabbitmq.WithContentType("application/json")
	opt2 := rabbitmq.WithMessageID("msg-12345")
	opt3 := rabbitmq.WithExpiration("5000")
	opt4 := rabbitmq.WithHeaders(amqp.Table{"source": "test-suite"})

	opt1(&pub)
	opt2(&pub)
	opt3(&pub)
	opt4(&pub)

	if pub.ContentType != "application/json" {
		t.Errorf("expected ContentType application/json, got %s", pub.ContentType)
	}
	if pub.MessageId != "msg-12345" {
		t.Errorf("expected MessageId msg-12345, got %s", pub.MessageId)
	}
	if pub.Expiration != "5000" {
		t.Errorf("expected Expiration 5000, got %s", pub.Expiration)
	}
	if pub.Headers["source"] != "test-suite" {
		t.Errorf("expected Headers['source'] == 'test-suite', got %v", pub.Headers["source"])
	}
}

func TestRabbitMQ_SubscribeOptions(t *testing.T) {
	cfg := rabbitmq.ConsumeConfig{
		Concurrency:   1,
		PrefetchCount: 1,
	}

	optConc := rabbitmq.WithConcurrency(8)
	optPref := rabbitmq.WithPrefetch(16)
	optMin := rabbitmq.WithMinWorkers(2)
	optIdle := rabbitmq.WithIdleTimeout(15 * time.Second)

	optConc(&cfg)
	optPref(&cfg)
	optMin(&cfg)
	optIdle(&cfg)

	if cfg.Concurrency != 8 {
		t.Errorf("expected Concurrency 8, got %d", cfg.Concurrency)
	}
	if cfg.PrefetchCount != 16 {
		t.Errorf("expected PrefetchCount 16, got %d", cfg.PrefetchCount)
	}
	if cfg.MinWorkers != 2 {
		t.Errorf("expected MinWorkers 2, got %d", cfg.MinWorkers)
	}
	if cfg.IdleTimeout != 15*time.Second {
		t.Errorf("expected IdleTimeout 15s, got %v", cfg.IdleTimeout)
	}

	var r *rabbitmq.RabbitMQ
	if err := r.SubscribeWithOptions("test.topic", func(ctx context.Context, msg []byte) bool { return true }, optConc, optPref, optMin, optIdle); err == nil {
		t.Error("expected error calling SubscribeWithOptions on nil RabbitMQ, got nil")
	}

	optW1 := rabbitmq.WithConcurrency(5)
	optW2 := rabbitmq.WithMinWorkers(1)
	optW3 := rabbitmq.WithPrefetch(10)
	optW4 := rabbitmq.WithIdleTimeout(10 * time.Second)
	if err := r.SubscribeWithOptions("test.topic", func(ctx context.Context, msg []byte) bool { return true }, optW1, optW2, optW3, optW4); err == nil {
		t.Error("expected error calling SubscribeWithOptions on nil RabbitMQ, got nil")
	}
}
