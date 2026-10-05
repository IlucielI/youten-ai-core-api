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
