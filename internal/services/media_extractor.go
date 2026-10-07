package services

import (
	"context"
)

// CompositeMediaLinkExtractor chains multiple MediaLinkExtractor implementations
// and routes method calls to the first extractor matching the URL pattern.
type CompositeMediaLinkExtractor struct {
	extractors []MediaLinkExtractor
}

// NewCompositeMediaLinkExtractor initializes a composite extractor from ordered delegates.
func NewCompositeMediaLinkExtractor(extractors []MediaLinkExtractor) *CompositeMediaLinkExtractor {
	var valid []MediaLinkExtractor
	for _, ext := range extractors {
		if ext != nil {
			valid = append(valid, ext)
		}
	}
	return &CompositeMediaLinkExtractor{extractors: valid}
}

func (c *CompositeMediaLinkExtractor) findExtractor(rawURL string) MediaLinkExtractor {
	for _, ext := range c.extractors {
		if ext.Supports(rawURL) {
			return ext
		}
	}
	return nil
}

// Supports returns true if any registered delegate supports rawURL.
func (c *CompositeMediaLinkExtractor) Supports(rawURL string) bool {
	return c.findExtractor(rawURL) != nil
}

// ExtractID extracts the resource identifier from rawURL using the matching delegate.
func (c *CompositeMediaLinkExtractor) ExtractID(rawURL string) (string, error) {
	if ext := c.findExtractor(rawURL); ext != nil {
		return ext.ExtractID(rawURL)
	}
	return "", ErrInvalidMediaURL
}

// NormalizeURL converts rawURL to its canonical representation using the matching delegate.
func (c *CompositeMediaLinkExtractor) NormalizeURL(rawURL string) (string, error) {
	if ext := c.findExtractor(rawURL); ext != nil {
		return ext.NormalizeURL(rawURL)
	}
	return "", ErrInvalidMediaURL
}

// FetchMetadata retrieves metadata from rawURL using the matching delegate.
func (c *CompositeMediaLinkExtractor) FetchMetadata(ctx context.Context, rawURL string) (*MediaMetadata, error) {
	if ext := c.findExtractor(rawURL); ext != nil {
		return ext.FetchMetadata(ctx, rawURL)
	}
	return nil, ErrInvalidMediaURL
}

// ExtractAudio extracts an audio stream from rawURL using the matching delegate.
func (c *CompositeMediaLinkExtractor) ExtractAudio(ctx context.Context, rawURL string) (*ExtractedAudio, error) {
	if ext := c.findExtractor(rawURL); ext != nil {
		return ext.ExtractAudio(ctx, rawURL)
	}
	return nil, ErrInvalidMediaURL
}
