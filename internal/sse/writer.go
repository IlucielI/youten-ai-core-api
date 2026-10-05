package sse

import (
	"io"
	"net/http"
)

// WriteEvent formats and writes an Event to the provided io.Writer, flushing if possible.
func WriteEvent(w io.Writer, event Event) error {
	encoded, err := Encode(event)
	if err != nil {
		return err
	}

	if _, err := w.Write(encoded); err != nil {
		return err
	}

	if flusher, ok := w.(http.Flusher); ok {
		flusher.Flush()
	}

	return nil
}

// WriteProgress serializes and writes a ProgressEvent as an SSE event named "progress".
func WriteProgress(w io.Writer, event ProgressEvent) error {
	return WriteEvent(w, Event{
		Event: "progress",
		Data:  event,
	})
}

// WritePing writes a heartbeat ping comment line to prevent connection timeouts.
func WritePing(w io.Writer) error {
	return WriteEvent(w, Event{
		Comment: "ping",
	})
}

// WriteChatToken serializes and writes a ChatTokenEvent as an SSE event named "token".
func WriteChatToken(w io.Writer, token string) error {
	return WriteEvent(w, Event{
		Event: "token",
		Data:  ChatTokenEvent{Token: token},
	})
}

// WriteChatDone serializes and writes a ChatDoneEvent as an SSE event named "done".
func WriteChatDone(w io.Writer, event ChatDoneEvent) error {
	return WriteEvent(w, Event{
		Event: "done",
		Data:  event,
	})
}

// WriteChatError serializes and writes a ChatErrorEvent as an SSE event named "error".
func WriteChatError(w io.Writer, errMsg string) error {
	return WriteEvent(w, Event{
		Event: "error",
		Data:  ChatErrorEvent{Error: errMsg},
	})
}

