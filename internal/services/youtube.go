package services

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrInvalidMediaURL indicates that the supplied URL does not match any known supported media pattern.
	ErrInvalidMediaURL = errors.New("invalid or unsupported media URL")
	// ErrMediaIDNotFound indicates that a unique media identifier could not be extracted from the URL.
	ErrMediaIDNotFound = errors.New("unable to extract media ID from URL")
	// ErrExtractionFailed indicates a failure during media extraction execution or output parsing.
	ErrExtractionFailed = errors.New("media extraction failed")
	// ErrAudioTooLarge indicates that the extracted audio exceeds the hard upper limit.
	ErrAudioTooLarge = errors.New("extracted audio exceeds maximum allowed size (500MB)")

	// ErrInvalidYouTubeURL aliases ErrInvalidMediaURL for YouTube link validation.
	ErrInvalidYouTubeURL = ErrInvalidMediaURL
	// ErrVideoIDNotFound aliases ErrMediaIDNotFound for YouTube video ID resolution.
	ErrVideoIDNotFound = ErrMediaIDNotFound

	// MaxImportAudioSizeBytes sets the hard ceiling on imported audio size (500MB).
	MaxImportAudioSizeBytes int64 = 500 * 1024 * 1024
)

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
	Stream          io.ReadCloser
	SizeBytes       int64
	DurationSeconds float64
	ContentType     string
	Filename        string
	Title           string
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

// MediaLinkExtractor defines the provider-neutral application port contract
// for inspecting and extracting media streams from external links.
type MediaLinkExtractor interface {
	Supports(rawURL string) bool
	ExtractID(rawURL string) (string, error)
	NormalizeURL(rawURL string) (string, error)
	FetchMetadata(ctx context.Context, rawURL string) (*MediaMetadata, error)
	ExtractAudio(ctx context.Context, rawURL string) (*ExtractedAudio, error)
}

// YouTubeExtractor aliases MediaLinkExtractor for backwards compatibility.
type YouTubeExtractor = MediaLinkExtractor
