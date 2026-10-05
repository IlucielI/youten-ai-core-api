package sse

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"code-base-golang/internal/models"
)

type mockFlusher struct {
	bytes.Buffer
	flushed bool
}

func (m *mockFlusher) Flush() {
	m.flushed = true
}

func TestSSE_Encode(t *testing.T) {
	t.Run("basic event with string data", func(t *testing.T) {
		encoded, err := Encode(Event{
			Event: "message",
			Data:  "hello world",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "event: message\ndata: hello world\n\n"
		if string(encoded) != expected {
			t.Errorf("expected %q, got %q", expected, string(encoded))
		}
	})

	t.Run("event with id, retry, and byte data", func(t *testing.T) {
		encoded, err := Encode(Event{
			ID:    "123",
			Event: "update",
			Retry: 5 * time.Second,
			Data:  []byte("byte data"),
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "id: 123\nevent: update\nretry: 5000\ndata: byte data\n\n"
		if string(encoded) != expected {
			t.Errorf("expected %q, got %q", expected, string(encoded))
		}
	})

	t.Run("event with struct json data", func(t *testing.T) {
		payload := ProgressEvent{
			RecordingID: "rec-1",
			Status:      "TRANSCRIBING",
			Progress:    55,
		}
		encoded, err := Encode(Event{
			Event: "progress",
			Data:  payload,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasPrefix(string(encoded), "event: progress\ndata: {") {
			t.Errorf("expected json payload prefix, got: %s", string(encoded))
		}
	})

	t.Run("comment event (ping)", func(t *testing.T) {
		encoded, err := Encode(Event{
			Comment: "ping",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := ": ping\n\n"
		if string(encoded) != expected {
			t.Errorf("expected %q, got %q", expected, string(encoded))
		}
	})

	t.Run("multiline string data", func(t *testing.T) {
		encoded, err := Encode(Event{
			Data: "line1\nline2",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "data: line1\ndata: line2\n\n"
		if string(encoded) != expected {
			t.Errorf("expected %q, got %q", expected, string(encoded))
		}
	})
}

func TestSSE_MapStatusToProgress(t *testing.T) {
	tests := []struct {
		status       string
		wantStage    string
		wantProgress int
	}{
		{models.RecordingStatusPending, "PENDING", 5},
		{models.RecordingStatusQueued, "QUEUED", 5},
		{models.RecordingStatusValidating, "VALIDATING", 15},
		{models.RecordingStatusExtracting, "EXTRACTING", 30},
		{models.RecordingStatusTranscribing, "TRANSCRIBING", 55},
		{models.RecordingStatusSummarizing, "SUMMARIZING", 75},
		{models.RecordingStatusIndexing, "INDEXING", 90},
		{models.RecordingStatusCompleted, "COMPLETED", 100},
		{models.RecordingStatusFailed, "FAILED", 100},
		{"CUSTOM", "CUSTOM", 0},
	}

	for _, tc := range tests {
		stage, prog := MapStatusToProgress(tc.status)
		if stage != tc.wantStage || prog != tc.wantProgress {
			t.Errorf("status %s: got (%s, %d), want (%s, %d)", tc.status, stage, prog, tc.wantStage, tc.wantProgress)
		}
	}
}

func TestSSE_IsTerminalStatus(t *testing.T) {
	if !IsTerminalStatus(models.RecordingStatusCompleted) {
		t.Error("expected COMPLETED to be terminal")
	}
	if !IsTerminalStatus(models.RecordingStatusFailed) {
		t.Error("expected FAILED to be terminal")
	}
	if IsTerminalStatus(models.RecordingStatusTranscribing) {
		t.Error("expected TRANSCRIBING to not be terminal")
	}
	if IsTerminalStatus(models.RecordingStatusExtracting) {
		t.Error("expected EXTRACTING to not be terminal")
	}
}

func TestSSE_Hub_SubscribePublish(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	recID := uuid.New()
	ch, unsub := hub.Subscribe(recID)
	defer unsub()

	if hub.SubscriberCount(recID) != 1 {
		t.Fatalf("expected subscriber count 1, got %d", hub.SubscriberCount(recID))
	}

	event := ProgressEvent{
		RecordingID: recID.String(),
		Status:      models.RecordingStatusExtracting,
		Stage:       "EXTRACTING",
		Progress:    30,
		UpdatedAt:   time.Now(),
	}

	hub.Publish(recID, event)

	select {
	case received := <-ch:
		if received.RecordingID != recID.String() || received.Status != models.RecordingStatusExtracting {
			t.Errorf("received unexpected event: %+v", received)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("timed out waiting for event on channel")
	}
}

func TestSSE_Hub_Unsubscribe(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	recID := uuid.New()
	ch, unsub := hub.Subscribe(recID)

	if hub.SubscriberCount(recID) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", hub.SubscriberCount(recID))
	}

	unsub()

	if hub.SubscriberCount(recID) != 0 {
		t.Fatalf("expected 0 subscribers after unsub, got %d", hub.SubscriberCount(recID))
	}

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after unsubscribe")
	}

	// Calling unsub multiple times should be safe (sync.Once)
	unsub()
}

func TestSSE_Hub_BufferFull(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	recID := uuid.New()
	ch, unsub := hub.Subscribe(recID)
	defer unsub()

	// Fill channel buffer (buffer size = 16)
	for i := 0; i < 20; i++ {
		hub.Publish(recID, ProgressEvent{
			RecordingID: recID.String(),
			Progress:    i,
		})
	}

	// Verify first item received
	select {
	case received := <-ch:
		if received.Progress != 0 {
			t.Errorf("expected progress 0, got %d", received.Progress)
		}
	default:
		t.Fatal("expected item in channel")
	}
}

func TestSSE_Hub_MultipleSubscribers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	recID := uuid.New()
	ch1, unsub1 := hub.Subscribe(recID)
	defer unsub1()
	ch2, unsub2 := hub.Subscribe(recID)
	defer unsub2()

	if hub.SubscriberCount(recID) != 2 {
		t.Fatalf("expected 2 subscribers, got %d", hub.SubscriberCount(recID))
	}

	event := ProgressEvent{RecordingID: recID.String(), Status: "COMPLETED"}
	hub.Publish(recID, event)

	select {
	case r1 := <-ch1:
		if r1.Status != "COMPLETED" {
			t.Errorf("ch1 expected COMPLETED, got %s", r1.Status)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ch1 timeout")
	}

	select {
	case r2 := <-ch2:
		if r2.Status != "COMPLETED" {
			t.Errorf("ch2 expected COMPLETED, got %s", r2.Status)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("ch2 timeout")
	}
}

func TestSSE_Hub_Close(t *testing.T) {
	hub := NewHub()

	recID := uuid.New()
	ch, _ := hub.Subscribe(recID)

	hub.Close()

	// Double close should be idempotent
	hub.Close()

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("expected channel to be closed after hub close")
	}

	// Subscribing on closed hub returns closed channel
	ch2, unsub2 := hub.Subscribe(recID)
	unsub2()
	_, ok2 := <-ch2
	if ok2 {
		t.Error("expected closed channel when subscribing to closed hub")
	}

	// Publishing on closed hub should be a no-op
	hub.Publish(recID, ProgressEvent{})
}

func TestSSE_Hub_Concurrency(t *testing.T) {
	hub := NewHub()
	defer hub.Close()

	recID := uuid.New()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, unsub := hub.Subscribe(recID)
			defer unsub()

			for j := 0; j < 5; j++ {
				hub.Publish(recID, ProgressEvent{
					RecordingID: recID.String(),
					Progress:    j,
				})
			}

			// Drain one item if available
			select {
			case <-ch:
			default:
			}
		}()
	}

	wg.Wait()
}

func TestSSE_Writer(t *testing.T) {
	t.Run("WriteProgress with flusher", func(t *testing.T) {
		flusher := &mockFlusher{}
		event := ProgressEvent{
			RecordingID: "rec-123",
			Status:      models.RecordingStatusTranscribing,
			Stage:       "TRANSCRIBING",
			Progress:    55,
		}

		err := WriteProgress(flusher, event)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !flusher.flushed {
			t.Error("expected flusher.Flush() to be called")
		}

		out := flusher.String()
		if !strings.Contains(out, "event: progress\n") {
			t.Errorf("missing event line, got: %s", out)
		}
		if !strings.Contains(out, `"status":"TRANSCRIBING"`) {
			t.Errorf("missing status in json, got: %s", out)
		}

		var decoded ProgressEvent
		dataLine := strings.Split(out, "data: ")[1]
		dataLine = strings.TrimSpace(dataLine)
		if err := json.Unmarshal([]byte(dataLine), &decoded); err != nil {
			t.Fatalf("failed to unmarshal written event: %v", err)
		}
		if decoded.Progress != 55 {
			t.Errorf("expected progress 55, got %d", decoded.Progress)
		}
	})

	t.Run("WritePing with flusher", func(t *testing.T) {
		flusher := &mockFlusher{}
		err := WritePing(flusher)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !flusher.flushed {
			t.Error("expected flusher.Flush() to be called")
		}
		if flusher.String() != ": ping\n\n" {
			t.Errorf("expected ': ping\\n\\n', got %q", flusher.String())
		}
	})
}
