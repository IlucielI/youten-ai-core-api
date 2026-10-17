package services

import (
	"context"
	"errors"

	"code-base-golang/internal/dtos"
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

// MediaLinkExtractor defines the provider-neutral application port contract
// for inspecting and extracting media streams from external links.
type MediaLinkExtractor interface {
	Supports(rawURL string) bool
	ExtractID(rawURL string) (string, error)
	NormalizeURL(rawURL string) (string, error)
	FetchMetadata(ctx context.Context, rawURL string) (*dtos.MediaMetadata, error)
	ExtractAudio(ctx context.Context, rawURL string) (*dtos.ExtractedAudio, error)
}

// YouTubeExtractor aliases MediaLinkExtractor for backwards compatibility.
type YouTubeExtractor = MediaLinkExtractor

type mediaContextKey string

const maxAudioSizeCtxKey mediaContextKey = "services:max_audio_size_bytes"

// WithMaxAudioSizeBytes injects a custom max audio size ceiling into the context.
func WithMaxAudioSizeBytes(ctx context.Context, maxBytes int64) context.Context {
	if maxBytes <= 0 {
		return ctx
	}
	return context.WithValue(ctx, maxAudioSizeCtxKey, maxBytes)
}

// GetMaxAudioSizeBytes retrieves custom max audio size ceiling from context or returns fallback.
func GetMaxAudioSizeBytes(ctx context.Context, fallback int64) int64 {
	if ctx == nil {
		return fallback
	}
	if v, ok := ctx.Value(maxAudioSizeCtxKey).(int64); ok && v > 0 {
		return v
	}
	return fallback
}
