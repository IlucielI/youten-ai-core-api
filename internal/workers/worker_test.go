package workers_test

import (
	"context"
	"testing"

	"code-base-golang/internal/config"
	"code-base-golang/internal/constants"
	"code-base-golang/internal/payload"
	"code-base-golang/internal/workers"
)

type mockSubscriber struct {
	subscribedTopics []string
	handlers         map[string]workers.WorkerHandler
}

func newMockSubscriber() *mockSubscriber {
	return &mockSubscriber{
		handlers: make(map[string]workers.WorkerHandler),
	}
}

func (m *mockSubscriber) Subscribe(topic string, handler workers.WorkerHandler) error {
	m.subscribedTopics = append(m.subscribedTopics, topic)
	m.handlers[topic] = handler
	return nil
}

func (m *mockSubscriber) MustSubscribe(topic string, handler workers.WorkerHandler) {
	_ = m.Subscribe(topic, handler)
}

func TestWorkerServer_RegisterWorker(t *testing.T) {
	var s *workers.WorkerServer
	s.RegisterWorker() // should not panic

	server := workers.New(config.Config{}, nil, nil)
	server.RegisterWorker() // should safely skip when subscriber is nil

	mock := newMockSubscriber()
	serverWithMock := workers.New(config.Config{}, nil, mock)
	serverWithMock.RegisterWorker() // should execute register cleanly

	expectedTopics := []string{
		constants.TopicRecordingUploaded,
		constants.TopicRecordingExtract,
		constants.TopicRecordingImport,
		constants.TopicRecordingTranscribe,
		constants.TopicRecordingSummarize,
		constants.TopicRecordingIndex,
		constants.TopicRecordingAnalytics,
		constants.TopicRecordingChapterize,
	}

	for _, topic := range expectedTopics {
		if _, ok := mock.handlers[topic]; !ok {
			t.Errorf("expected handler registered for topic %s", topic)
		}
	}
}

func TestWorkerServer_Handlers_NilServiceOrBadJSON(t *testing.T) {
	server := workers.New(config.Config{}, nil, nil)
	ctx := context.Background()

	badJSON := []byte("invalid json")
	goodJSON := []byte(`{"recording_id":"43af1a3d-7891-4cd2-ad62-f1a9dd823c70"}`)

	// When svc is nil
	if server.HandleRecordingImport(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}
	if server.HandleRecordingExtract(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}
	if server.HandleRecordingTranscribe(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}
	if server.HandleRecordingSummarize(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}
	if server.HandleRecordingIndex(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}
	if server.HandleRecordingAnalytics(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}
	if server.HandleRecordingChapterize(ctx, goodJSON) {
		t.Error("expected false when svc is nil")
	}

	// Bad JSON
	if server.HandleRecordingImport(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
	if server.HandleRecordingExtract(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
	if server.HandleRecordingTranscribe(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
	if server.HandleRecordingSummarize(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
	if server.HandleRecordingIndex(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
	if server.HandleRecordingAnalytics(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
	if server.HandleRecordingChapterize(ctx, badJSON) {
		t.Error("expected false on bad json")
	}
}

func TestPayload_MustParse(t *testing.T) {
	data := []byte(`{"id":"entity-123"}`)
	var p payload.Id
	payload.MustParse(data, &p)
	if p.Id != "entity-123" {
		t.Errorf("expected entity-123, got %s", p.Id)
	}

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected MustParse to panic on invalid json")
		}
	}()
	payload.MustParse([]byte("invalid json"), &p)
}

func TestPayload_Parse(t *testing.T) {
	data := []byte(`{"id":"order-999"}`)
	var p payload.Id
	if err := payload.Parse(data, &p); err != nil {
		t.Errorf("expected nil error parsing payload, got %v", err)
	}
	if p.Id != "order-999" {
		t.Errorf("expected order-999, got %s", p.Id)
	}

	if err := payload.Parse([]byte("bad json"), &p); err == nil {
		t.Error("expected error parsing invalid json, got nil")
	}
}
