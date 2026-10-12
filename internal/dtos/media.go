package dtos

import "io"

// MediaMetadata represents provider-neutral descriptive attributes of an external media stream.
type MediaMetadata struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description,omitempty"`
	Duration    float64 `json:"duration"` // in seconds
	Author      string  `json:"author,omitempty"`
	Thumbnail   string  `json:"thumbnail,omitempty"`
}

// VideoMetadata aliases MediaMetadata for backwards compatibility.
type VideoMetadata = MediaMetadata

// ExtractedAudio wraps the audio stream and metadata extracted from a remote media source.
// Callers assume resource ownership and MUST call Close() when consumption finishes
// to release underlying OS file handles and trigger temporary workspace directory cleanup.
type ExtractedAudio struct {
	Stream          io.ReadCloser `json:"-"`
	SizeBytes       int64         `json:"size_bytes"`
	DurationSeconds float64       `json:"duration_seconds"`
	ContentType     string        `json:"content_type"`
	Filename        string        `json:"filename"`
	Title           string        `json:"title"`
}

// Read implements io.Reader by delegating to the managed media stream.
func (e *ExtractedAudio) Read(p []byte) (int, error) {
	if e == nil || e.Stream == nil {
		return 0, io.EOF
	}
	return e.Stream.Read(p)
}

// Close implements io.Closer by releasing underlying resources and removing temporary files.
func (e *ExtractedAudio) Close() error {
	if e == nil || e.Stream == nil {
		return nil
	}
	return e.Stream.Close()
}
