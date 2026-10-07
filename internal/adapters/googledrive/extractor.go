package googledrive

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"code-base-golang/internal/pkg/ssrf"
	"code-base-golang/internal/services"
)

const (
	// DefaultGoogleDriveBaseURL is the official production Google Drive endpoint.
	DefaultGoogleDriveBaseURL = "https://drive.google.com"
	// DefaultMetadataTimeout is the deadline for header and metadata inspections.
	DefaultMetadataTimeout = 30 * time.Second
	// DefaultDownloadTimeout is the deadline for streaming media downloads.
	DefaultDownloadTimeout = 10 * time.Minute
	// DefaultMaxAudioSizeBytes sets the hard 500MB ceiling for media downloads.
	DefaultMaxAudioSizeBytes = 500 * 1024 * 1024
	// DefaultMaxConcurrency limits simultaneous download goroutines.
	DefaultMaxConcurrency = 5
	// MaxFilenameLength limits sanitized filename string lengths.
	MaxFilenameLength = 255
)

// AllowedDomains defines the official Google Drive URL hostnames.
var AllowedDomains = []string{
	"drive.google.com",
	"docs.google.com",
}

var (
	// gdriveIDRegex validates alphanumeric Google Drive file IDs.
	gdriveIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{10,}$`)
	// gdrivePathRegex matches /file/d/{id} or /d/{id} paths.
	gdrivePathRegex = regexp.MustCompile(`^/(?:file/)?d/([a-zA-Z0-9_-]{10,})`)
	// confirmTokenRegex extracts Google Drive virus-scan bypass confirmation tokens.
	confirmTokenRegex = regexp.MustCompile(`(?:confirm=([a-zA-Z0-9_-]+)|download_warning_([a-zA-Z0-9_-]+))`)
	// filenameHeaderRegex parses filename parameter from Content-Disposition headers.
	filenameHeaderRegex = regexp.MustCompile(`filename[*]?=(?:UTF-8'')?["']?([^"';]+)["']?`)
)

// Config specifies runtime parameters for Google Drive extraction.
type Config struct {
	Transport         http.RoundTripper
	MaxAudioSizeBytes int64
	MaxConcurrency    int
	MetadataTimeout   time.Duration
	DownloadTimeout   time.Duration
	AudioExtractor    services.AudioExtractor
}

// Extractor implements services.MediaLinkExtractor for Google Drive files.
type Extractor struct {
	cfg            Config
	baseURL        string
	isTest         bool
	semaphore      chan struct{}
	audioExtractor services.AudioExtractor
	client         *http.Client
}

// NewExtractor initializes a Google Drive media link extractor with defaults.
func NewExtractor() *Extractor {
	return NewExtractorWithConfig(Config{})
}

// NewExtractorWithConfig initializes a Google Drive media link extractor with explicit config.
func NewExtractorWithConfig(cfg Config) *Extractor {
	return newExtractorInternal(DefaultGoogleDriveBaseURL, false, cfg)
}

// NewExtractorForTest initializes an Extractor wired to a local mock server for unit testing.
func NewExtractorForTest(testEndpoint string, cfg Config) *Extractor {
	return newExtractorInternal(testEndpoint, true, cfg)
}

func newExtractorInternal(baseURL string, isTest bool, c Config) *Extractor {
	if !isTest {
		baseURL = DefaultGoogleDriveBaseURL
	}

	if c.MaxAudioSizeBytes <= 0 {
		c.MaxAudioSizeBytes = DefaultMaxAudioSizeBytes
	}
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = DefaultMaxConcurrency
	}
	if c.MetadataTimeout <= 0 {
		c.MetadataTimeout = DefaultMetadataTimeout
	}
	if c.DownloadTimeout <= 0 {
		c.DownloadTimeout = DefaultDownloadTimeout
	}

	jar, _ := cookiejar.New(nil)
	var transport http.RoundTripper = c.Transport
	if transport == nil {
		transport = ssrf.NewSafeTransport(c.DownloadTimeout)
	}

	client := &http.Client{
		Transport: transport,
		Jar:       jar,
		Timeout:   c.DownloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("stopped after 5 redirects")
			}
			if !isTest {
				if _, err := ssrf.ValidateURL(req.Context(), req.URL.String()); err != nil {
					return fmt.Errorf("redirect blocked by ssrf policy: %w", err)
				}
			}
			return nil
		},
	}

	return &Extractor{
		cfg:            c,
		baseURL:        baseURL,
		isTest:         isTest,
		semaphore:      make(chan struct{}, c.MaxConcurrency),
		audioExtractor: c.AudioExtractor,
		client:         client,
	}
}

// Supports checks whether rawURL is an authentic, HTTPS Google Drive file link.
func (e *Extractor) Supports(rawURL string) bool {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return false
	}

	if strings.ToLower(u.Scheme) != "https" {
		return false
	}

	hostname := strings.ToLower(u.Hostname())
	if !ssrf.IsAllowedDomain(hostname, AllowedDomains) {
		return false
	}

	_, extractErr := e.ExtractID(rawURL)
	return extractErr == nil
}

// ExtractID extracts the Google Drive file identifier from a URL.
func (e *Extractor) ExtractID(rawURL string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", fmt.Errorf("invalid google drive url: %w", err)
	}

	hostname := strings.ToLower(u.Hostname())
	if !ssrf.IsAllowedDomain(hostname, AllowedDomains) {
		return "", errors.New("unsupported or untrusted domain for google drive extraction")
	}

	// Pattern 1: /file/d/{id}/... or /d/{id}/...
	if matches := gdrivePathRegex.FindStringSubmatch(u.Path); len(matches) > 1 {
		id := matches[1]
		if gdriveIDRegex.MatchString(id) {
			return id, nil
		}
	}

	// Pattern 2: ?id={id} query parameter
	q := u.Query()
	if id := q.Get("id"); id != "" && gdriveIDRegex.MatchString(id) {
		return id, nil
	}

	return "", errors.New("unable to extract valid file ID from google drive URL")
}

// NormalizeURL converts various Google Drive link formats into the canonical view link.
func (e *Extractor) NormalizeURL(rawURL string) (string, error) {
	id, err := e.ExtractID(rawURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://drive.google.com/file/d/%s/view", id), nil
}

// FetchMetadata retrieves file attributes from Google Drive headers.
func (e *Extractor) FetchMetadata(ctx context.Context, rawURL string) (*services.MediaMetadata, error) {
	id, err := e.ExtractID(rawURL)
	if err != nil {
		return nil, err
	}

	timeout := e.cfg.MetadataTimeout
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < timeout {
			timeout = remaining
		}
	}
	metaCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	downloadURL := fmt.Sprintf("%s/uc?export=download&id=%s", e.baseURL, id)
	if !e.isTest {
		if _, err := ssrf.ValidateURL(metaCtx, downloadURL); err != nil {
			return nil, fmt.Errorf("ssrf validation failed: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(metaCtx, http.MethodHead, downloadURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to construct metadata request: %w", err)
	}
	req.Header.Set("User-Agent", "Youten-AI-MediaFetcher/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metadata: %w", err)
	}
	defer resp.Body.Close()

	title := parseFilenameFromHeader(resp.Header.Get("Content-Disposition"))
	if title == "" {
		title = fmt.Sprintf("Google Drive (%s)", id)
	}

	return &services.MediaMetadata{
		ID:        id,
		Title:     title,
		Thumbnail: fmt.Sprintf("https://drive.google.com/thumbnail?id=%s", id),
	}, nil
}

// ExtractAudio securely downloads the Google Drive media file, handles virus scan
// confirmation redirects, converts video to mono audio when required, and returns
// a resource-managed ExtractedAudio stream.
func (e *Extractor) ExtractAudio(ctx context.Context, rawURL string) (*services.ExtractedAudio, error) {
	id, err := e.ExtractID(rawURL)
	if err != nil {
		return nil, err
	}

	// Bounded concurrency acquisition
	select {
	case e.semaphore <- struct{}{}:
		defer func() { <-e.semaphore }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	timeout := e.cfg.DownloadTimeout
	if dl, ok := ctx.Deadline(); ok {
		remaining := time.Until(dl)
		if remaining < timeout {
			timeout = remaining
		}
	}
	dlCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	downloadURL := fmt.Sprintf("%s/uc?export=download&id=%s", e.baseURL, id)
	resp, err := e.executeDownloadRequest(dlCtx, downloadURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Check if Google Drive returned a virus scan confirmation page for large files
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "text/html") {
		confirmedURL, confirmErr := e.handleConfirmToken(resp, id)
		if confirmErr != nil {
			return nil, fmt.Errorf("failed to resolve virus scan warning: %w", confirmErr)
		}

		resp.Body.Close()
		resp, err = e.executeDownloadRequest(dlCtx, confirmedURL)
		if err != nil {
			return nil, err
		}
		contentType = strings.ToLower(resp.Header.Get("Content-Type"))
	}

	filename := parseFilenameFromHeader(resp.Header.Get("Content-Disposition"))
	if filename == "" {
		filename = fmt.Sprintf("%s.mp3", id)
	}

	// Stream safely to bounded temporary file
	tempFile, err := os.CreateTemp("", "gdrive-download-*")
	if err != nil {
		return nil, fmt.Errorf("failed to allocate temporary download file: %w", err)
	}
	tempFilePath := tempFile.Name()

	cleanup := func() {
		_ = tempFile.Close()
		_ = os.Remove(tempFilePath)
	}

	limitedReader := io.LimitReader(resp.Body, e.cfg.MaxAudioSizeBytes+1)
	written, copyErr := io.Copy(tempFile, limitedReader)
	if copyErr != nil {
		cleanup()
		return nil, fmt.Errorf("failed during streaming download: %w", copyErr)
	}

	if written > e.cfg.MaxAudioSizeBytes {
		cleanup()
		return nil, fmt.Errorf("downloaded file exceeds maximum allowed size of %d bytes", e.cfg.MaxAudioSizeBytes)
	}

	if written == 0 {
		cleanup()
		return nil, errors.New("downloaded file is empty")
	}

	// Rewind temporary file for inspection and processing
	if _, err := tempFile.Seek(0, io.SeekStart); err != nil {
		cleanup()
		return nil, fmt.Errorf("failed to seek temp file: %w", err)
	}

	// If the file is video, extract audio via audioExtractor
	ext := strings.ToLower(filepath.Ext(filename))
	isVideo := strings.HasPrefix(contentType, "video/") ||
		ext == ".mp4" || ext == ".mov" || ext == ".mkv" || ext == ".webm" || ext == ".avi"

	if isVideo {
		if e.audioExtractor == nil {
			cleanup()
			return nil, errors.New("video file detected but audio extractor is not configured")
		}
		convResult, convErr := e.audioExtractor.ExtractMonoAudio(dlCtx, tempFile, filename)
		cleanup() // clean original download temp file
		if convErr != nil {
			return nil, fmt.Errorf("audio conversion failed: %w", convErr)
		}

		return &services.ExtractedAudio{
			Stream:          convResult.Reader,
			SizeBytes:       convResult.SizeBytes,
			DurationSeconds: convResult.DurationSeconds,
			ContentType:     "audio/mpeg",
			Filename:        strings.TrimSuffix(filename, filepath.Ext(filename)) + ".mp3",
			Title:           strings.TrimSuffix(filename, filepath.Ext(filename)),
		}, nil
	}

	// Already audio or audio extractor bypassed
	audioStream := &tempFileReadCloser{
		File: tempFile,
		path: tempFilePath,
	}

	return &services.ExtractedAudio{
		Stream:          audioStream,
		SizeBytes:       written,
		DurationSeconds: 0,
		ContentType:     contentType,
		Filename:        filename,
		Title:           strings.TrimSuffix(filename, filepath.Ext(filename)),
	}, nil
}

func (e *Extractor) executeDownloadRequest(ctx context.Context, targetURL string) (*http.Response, error) {
	if !e.isTest {
		if _, err := ssrf.ValidateURL(ctx, targetURL); err != nil {
			return nil, fmt.Errorf("ssrf validation failed for download request: %w", err)
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build request: %w", err)
	}
	req.Header.Set("User-Agent", "Youten-AI-MediaFetcher/1.0")

	resp, err := e.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to download from google drive: %w", err)
	}
	return resp, nil
}

func (e *Extractor) handleConfirmToken(resp *http.Response, fileID string) (string, error) {
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read confirmation page: %w", err)
	}

	bodyStr := string(bodyBytes)
	matches := confirmTokenRegex.FindStringSubmatch(bodyStr)
	var token string
	if len(matches) > 1 && matches[1] != "" {
		token = matches[1]
	} else if len(matches) > 2 && matches[2] != "" {
		token = matches[2]
	}

	if token == "" {
		token = "t"
	}

	u, err := url.Parse(fmt.Sprintf("%s/uc", e.baseURL))
	if err != nil {
		return "", fmt.Errorf("failed to parse base URL: %w", err)
	}
	q := u.Query()
	q.Set("export", "download")
	q.Set("confirm", token)
	q.Set("id", fileID)
	u.RawQuery = q.Encode()

	return u.String(), nil
}

func parseFilenameFromHeader(contentDisposition string) string {
	if contentDisposition == "" {
		return ""
	}
	var filename string
	_, params, err := mime.ParseMediaType(contentDisposition)
	if err == nil && params["filename"] != "" {
		filename = filepath.Base(params["filename"])
	} else {
		matches := filenameHeaderRegex.FindStringSubmatch(contentDisposition)
		if len(matches) > 1 {
			filename = filepath.Base(matches[1])
		}
	}

	// Sanitize control characters and filesystem delimiters
	filename = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 || strings.ContainsRune(`<>:"/\|?*`, r) {
			return -1
		}
		return r
	}, filename)
	filename = strings.TrimSpace(filename)

	if len(filename) > MaxFilenameLength {
		ext := filepath.Ext(filename)
		base := filename[:MaxFilenameLength-len(ext)]
		filename = base + ext
	}
	return filename
}

type tempFileReadCloser struct {
	*os.File
	path string
}

func (t *tempFileReadCloser) Close() error {
	closeErr := t.File.Close()
	removeErr := os.Remove(t.path)
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}
