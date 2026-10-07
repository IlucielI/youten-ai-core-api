package youtube

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"code-base-golang/internal/pkg/ssrf"
	"code-base-golang/internal/services"
)

var (
	// ErrBufferLimitExceeded indicates process output exceeded memory safety threshold.
	ErrBufferLimitExceeded = errors.New("command output exceeded maximum allowed buffer limit")
	// ErrInvalidBinaryPath indicates that the provided custom binary path is untrusted or invalid.
	ErrInvalidBinaryPath = errors.New("invalid or untrusted yt-dlp binary path")
	// ErrInvalidTempDir indicates that the provided temporary directory is invalid or insecure.
	ErrInvalidTempDir = errors.New("invalid or untrusted temporary directory path")
	// ErrYtDlpNotFound indicates that the yt-dlp binary is missing from system PATH or trusted locations.
	ErrYtDlpNotFound = errors.New("yt-dlp binary not found in trusted system locations")
)

const (
	// DefaultMaxAudioSizeBytes defines the default 500MB ceiling on extracted media artifacts.
	DefaultMaxAudioSizeBytes int64 = 500 * 1024 * 1024
	// DefaultMetadataTimeout is the fallback deadline for metadata inspection if context lacks one.
	DefaultMetadataTimeout = 30 * time.Second
	// DefaultDownloadTimeout is the fallback deadline for audio extraction if context lacks one.
	DefaultDownloadTimeout = 10 * time.Minute
	// DefaultMaxBufferBytes enforces a strict 1MB memory ceiling on captured process output.
	DefaultMaxBufferBytes = 1024 * 1024
	// DefaultMaxConcurrency limits simultaneous yt-dlp subprocesses to prevent memory exhaustion.
	DefaultMaxConcurrency = 5
)

// AllowedDomains is the authoritative list of official YouTube hostnames.
var AllowedDomains = []string{
	"youtube.com",
	"www.youtube.com",
	"m.youtube.com",
	"youtu.be",
	"music.youtube.com",
}

var (
	youtubeIDRegex     = regexp.MustCompile(`^[a-zA-Z0-9_-]{11}$`)
	youtubeShortsRegex = regexp.MustCompile(`^/shorts/([a-zA-Z0-9_-]{11})`)
	youtubeEmbedRegex  = regexp.MustCompile(`^/(?:embed|v)/([a-zA-Z0-9_-]{11})`)
)

// Config specifies runtime options for the YouTube extractor adapter.
type Config struct {
	BinaryPath        string
	TempDir           string
	MaxBufferBytes    int64
	MaxAudioSizeBytes int64
	MaxConcurrency    int
	MetadataTimeout   time.Duration
	DownloadTimeout   time.Duration
}

// Extractor implements services.YouTubeExtractor using the yt-dlp system CLI.
type Extractor struct {
	binaryPath        string
	tempDir           string
	maxBufferBytes    int64
	maxAudioSizeBytes int64
	metadataTimeout   time.Duration
	downloadTimeout   time.Duration
	sem               chan struct{}
}

// NewExtractor creates a YouTube extractor resolving yt-dlp from trusted system locations.
func NewExtractor() (*Extractor, error) {
	return NewExtractorWithConfig(Config{})
}

// NewExtractorWithConfig creates an Extractor with explicit configuration options.
func NewExtractorWithConfig(cfg Config) (*Extractor, error) {
	binaryPath := cfg.BinaryPath
	if strings.TrimSpace(binaryPath) == "" {
		resolved, err := resolveYtDlpBinary()
		if err != nil {
			return nil, err
		}
		binaryPath = resolved
	} else {
		validated, err := validateBinaryPath(binaryPath)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidBinaryPath, err)
		}
		binaryPath = validated
	}

	tDir, err := validateTempDir(cfg.TempDir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidTempDir, err)
	}

	maxBuf := cfg.MaxBufferBytes
	if maxBuf <= 0 || maxBuf > 32*1024*1024 {
		maxBuf = DefaultMaxBufferBytes
	}

	maxAudio := cfg.MaxAudioSizeBytes
	if maxAudio <= 0 || maxAudio > 1024*1024*1024 {
		maxAudio = DefaultMaxAudioSizeBytes
	}

	maxConc := cfg.MaxConcurrency
	if maxConc <= 0 || maxConc > 50 {
		maxConc = DefaultMaxConcurrency
	}

	metaTimeout := cfg.MetadataTimeout
	if metaTimeout <= 0 {
		metaTimeout = DefaultMetadataTimeout
	}

	dlTimeout := cfg.DownloadTimeout
	if dlTimeout <= 0 {
		dlTimeout = DefaultDownloadTimeout
	}

	return &Extractor{
		binaryPath:        binaryPath,
		tempDir:           tDir,
		maxBufferBytes:    maxBuf,
		maxAudioSizeBytes: maxAudio,
		metadataTimeout:   metaTimeout,
		downloadTimeout:   dlTimeout,
		sem:               make(chan struct{}, maxConc),
	}, nil
}

// Supports reports whether rawURL is a supported YouTube URL.
func (e *Extractor) Supports(rawURL string) bool {
	return e.IsYouTubeURL(rawURL)
}

// ExtractID extracts the media identifier from rawURL.
func (e *Extractor) ExtractID(rawURL string) (string, error) {
	return e.ExtractVideoID(rawURL)
}

// IsYouTubeURL checks whether the given raw URL belongs to the YouTube platform with a secure https scheme.
func (e *Extractor) IsYouTubeURL(rawURL string) bool {
	return IsYouTubeURL(rawURL)
}

// IsYouTubeURL is a helper function for validating YouTube URLs against official domains.
func IsYouTubeURL(rawURL string) bool {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.ToLower(parsed.Scheme) != "https" {
		return false
	}
	return validateDomainBelongsToYouTube(parsed.Hostname())
}

// validateDomainBelongsToYouTube verifies that host belongs to AllowedDomains via SSRF allowlist.
func validateDomainBelongsToYouTube(host string) bool {
	return ssrf.IsAllowedDomain(host, AllowedDomains)
}

// ExtractVideoID parses an 11-character YouTube video ID from supported URL shapes.
func (e *Extractor) ExtractVideoID(rawURL string) (string, error) {
	return ExtractVideoID(rawURL)
}

// ExtractVideoID is a helper function for extracting YouTube video IDs.
func ExtractVideoID(rawURL string) (string, error) {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return "", services.ErrInvalidYouTubeURL
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", fmt.Errorf("%w: %v", services.ErrInvalidYouTubeURL, err)
	}

	if strings.ToLower(parsed.Scheme) != "https" {
		return "", services.ErrInvalidYouTubeURL
	}

	host := parsed.Hostname()
	if host == "" || !validateDomainBelongsToYouTube(host) {
		return "", services.ErrInvalidYouTubeURL
	}

	path := parsed.Path

	// 1. Short links: youtu.be/<id>
	cleanHost := strings.ToLower(host)
	if cleanHost == "youtu.be" {
		cleaned := strings.TrimPrefix(path, "/")
		cleaned = strings.Split(cleaned, "/")[0]
		if youtubeIDRegex.MatchString(cleaned) {
			return cleaned, nil
		}
		return "", services.ErrVideoIDNotFound
	}

	// 2. Shorts: /shorts/<id>
	if matches := youtubeShortsRegex.FindStringSubmatch(path); len(matches) > 1 {
		return matches[1], nil
	}

	// 3. Embed or v: /embed/<id>, /v/<id>
	if matches := youtubeEmbedRegex.FindStringSubmatch(path); len(matches) > 1 {
		return matches[1], nil
	}

	// 4. Standard watch: /watch?v=<id>
	if strings.HasPrefix(path, "/watch") {
		v := parsed.Query().Get("v")
		if youtubeIDRegex.MatchString(v) {
			return v, nil
		}
	}

	return "", services.ErrVideoIDNotFound
}

// NormalizeURL canonicalizes any supported YouTube URL into https://www.youtube.com/watch?v=<id>.
func (e *Extractor) NormalizeURL(rawURL string) (string, error) {
	return NormalizeURL(rawURL)
}

// NormalizeURL is a helper function for canonicalizing YouTube URLs.
func NormalizeURL(rawURL string) (string, error) {
	id, err := ExtractVideoID(rawURL)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://www.youtube.com/watch?v=%s", id), nil
}

// ytDlpMetadata is the raw json structure produced by yt-dlp.
type ytDlpMetadata struct {
	ID          string  `json:"id"`
	Title       string  `json:"title"`
	Description string  `json:"description"`
	Duration    float64 `json:"duration"`
	Uploader    string  `json:"uploader"`
	Channel     string  `json:"channel"`
	Thumbnail   string  `json:"thumbnail"`
	Ext         string  `json:"ext"`
	FormatID    string  `json:"format_id"`
}

// sanitizeString trims whitespace and bounds the string length to maxLen.
func sanitizeString(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen]
	}
	return s
}

// FetchMetadata invokes yt-dlp in dump-json mode to inspect video information without downloading media streams.
func (e *Extractor) FetchMetadata(ctx context.Context, rawURL string) (*services.MediaMetadata, error) {
	normURL, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	execCtx := ctx
	if dl, ok := ctx.Deadline(); ok {
		if time.Until(dl) > e.metadataTimeout {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, e.metadataTimeout)
			defer cancel()
		}
	} else {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, e.metadataTimeout)
		defer cancel()
	}

	args := []string{
		"-j",
		"--no-playlist",
		"--no-warnings",
		"--no-call-home",
		"--socket-timeout", "15",
		normURL,
	}

	stdout, err := e.runCommand(execCtx, true, args)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to fetch video metadata: %v", services.ErrExtractionFailed, err)
	}

	var raw ytDlpMetadata
	if err := json.Unmarshal(stdout, &raw); err != nil {
		return nil, fmt.Errorf("%w: failed to parse metadata JSON: %v", services.ErrExtractionFailed, err)
	}

	title := raw.Title
	if title == "" {
		title = fmt.Sprintf("YouTube Video (%s)", raw.ID)
	}
	title = sanitizeString(title, 255)

	author := raw.Channel
	if author == "" {
		author = raw.Uploader
	}
	author = sanitizeString(author, 255)

	return &services.MediaMetadata{
		ID:          raw.ID,
		Title:       title,
		Description: sanitizeString(raw.Description, 2000),
		Duration:    raw.Duration,
		Author:      author,
		Thumbnail:   raw.Thumbnail,
	}, nil
}

// ExtractAudio executes yt-dlp to extract native audio directly into an isolated per-request directory.
// It retrieves both audio stream and JSON metadata in a single yt-dlp execution (--write-info-json).
func (e *Extractor) ExtractAudio(ctx context.Context, rawURL string) (*services.ExtractedAudio, error) {
	normURL, err := NormalizeURL(rawURL)
	if err != nil {
		return nil, err
	}

	videoID, err := ExtractVideoID(rawURL)
	if err != nil {
		return nil, err
	}

	execCtx := ctx
	if dl, ok := ctx.Deadline(); ok {
		if time.Until(dl) > e.downloadTimeout {
			var cancel context.CancelFunc
			execCtx, cancel = context.WithTimeout(ctx, e.downloadTimeout)
			defer cancel()
		}
	} else {
		var cancel context.CancelFunc
		execCtx, cancel = context.WithTimeout(ctx, e.downloadTimeout)
		defer cancel()
	}

	// Create an isolated temporary directory with internal prefix
	jobDir, err := os.MkdirTemp(e.tempDir, "yt_job_*")
	if err != nil {
		return nil, fmt.Errorf("%w: failed to allocate workspace: %v", services.ErrExtractionFailed, err)
	}

	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			_ = os.RemoveAll(jobDir)
		}
	}()

	outputTemplate := filepath.Join(jobDir, "audio.%(ext)s")
	args := []string{
		"-f", "ba[ext=m4a]/ba[ext=webm]/ba",
		"--no-playlist",
		"--no-warnings",
		"--no-call-home",
		"--max-filesize", fmt.Sprintf("%d", e.maxAudioSizeBytes),
		"--socket-timeout", "30",
		"--write-info-json",
		"-o", outputTemplate,
		normURL,
	}

	if _, err := e.runCommand(execCtx, false, args); err != nil {
		return nil, fmt.Errorf("%w: audio download failed: %v", services.ErrExtractionFailed, err)
	}

	// Read metadata info file directly
	meta := &services.MediaMetadata{
		ID:    videoID,
		Title: fmt.Sprintf("YouTube Video (%s)", videoID),
	}
	infoPath := filepath.Join(jobDir, "audio.info.json")
	if infoFile, err := os.Open(infoPath); err == nil {
		var raw ytDlpMetadata
		if err := json.NewDecoder(infoFile).Decode(&raw); err == nil {
			meta.Title = raw.Title
			meta.Duration = raw.Duration
			author := raw.Channel
			if author == "" {
				author = raw.Uploader
			}
			meta.Author = author
			meta.Thumbnail = raw.Thumbnail
		}
		_ = infoFile.Close()
	}

	// Deterministic lookup for extracted audio among supported formats in order of preference
	var audioFilePath string
	var contentType string
	supportedExts := []struct {
		ext  string
		mime string
	}{
		{".m4a", "audio/m4a"},
		{".webm", "audio/webm"},
		{".mp3", "audio/mpeg"},
		{".opus", "audio/ogg"},
		{".ogg", "audio/ogg"},
		{".aac", "audio/aac"},
		{".wav", "audio/wav"},
	}

	for _, item := range supportedExts {
		candidate := filepath.Join(jobDir, "audio"+item.ext)
		if fi, err := os.Lstat(candidate); err == nil && fi.Mode().IsRegular() && fi.Mode()&os.ModeSymlink == 0 {
			audioFilePath = candidate
			contentType = item.mime
			break
		}
	}

	if audioFilePath == "" {
		return nil, fmt.Errorf("%w: extracted audio file not found on disk", services.ErrExtractionFailed)
	}

	// Open descriptor first to prevent TOCTOU race conditions
	file, err := os.Open(audioFilePath)
	if err != nil {
		return nil, fmt.Errorf("%w: failed to open extracted file: %v", services.ErrExtractionFailed, err)
	}

	fileInfo, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("%w: failed to inspect opened file descriptor: %v", services.ErrExtractionFailed, err)
	}

	if !fileInfo.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("%w: extracted audio file is not a regular file", services.ErrExtractionFailed)
	}

	if fileInfo.Size() > e.maxAudioSizeBytes {
		_ = file.Close()
		return nil, services.ErrAudioTooLarge
	}

	ext := strings.ToLower(filepath.Ext(audioFilePath))
	filename := fmt.Sprintf("youtube_%s%s", videoID, ext)

	// Transfer cleanup responsibility to autoCleanReader
	cleanupNeeded = false
	cleanReader := newAutoCleanReader(file, jobDir)

	return &services.ExtractedAudio{
		Stream:          cleanReader,
		SizeBytes:       fileInfo.Size(),
		DurationSeconds: meta.Duration,
		ContentType:     contentType,
		Filename:        filename,
		Title:           meta.Title,
	}, nil
}

// runCommand acquires concurrency token and executes yt-dlp with bounded memory buffers.
func (e *Extractor) runCommand(ctx context.Context, captureStdout bool, args []string) ([]byte, error) {
	select {
	case e.sem <- struct{}{}:
		defer func() { <-e.sem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	cmdCtx, cmdCancel := context.WithCancel(ctx)
	defer cmdCancel()

	cmd := exec.CommandContext(cmdCtx, e.binaryPath)
	cmd.Args = make([]string, 1+len(args))
	cmd.Args[0] = e.binaryPath
	copy(cmd.Args[1:], args)
	cmd.Env = []string{
		"PATH=/usr/local/bin:/usr/bin:/bin",
		"LC_ALL=C.UTF-8",
		"LANG=C.UTF-8",
		"HOME=" + e.tempDir,
	}

	var stdoutWriter *limitedWriter
	if captureStdout {
		stdoutWriter = newLimitedWriter(e.maxBufferBytes, cmdCancel)
		cmd.Stdout = stdoutWriter
	} else {
		cmd.Stdout = io.Discard
	}

	stderrWriter := newLimitedWriter(e.maxBufferBytes, cmdCancel)
	cmd.Stderr = stderrWriter

	if err := cmd.Run(); err != nil {
		if (stdoutWriter != nil && stdoutWriter.overflow) || stderrWriter.overflow {
			return nil, ErrBufferLimitExceeded
		}
		errDetail := strings.TrimSpace(stderrWriter.String())
		if errDetail == "" && stdoutWriter != nil {
			errDetail = strings.TrimSpace(stdoutWriter.String())
		}
		if errDetail != "" {
			if len(errDetail) > 200 {
				errDetail = errDetail[:200] + "..."
			}
			return nil, fmt.Errorf("%w: %s", err, errDetail)
		}
		return nil, err
	}

	if stdoutWriter != nil {
		return stdoutWriter.Bytes(), nil
	}
	return nil, nil
}

// limitedWriter wraps bytes.Buffer to enforce a strict memory ceiling on captured process output.
type limitedWriter struct {
	buf        bytes.Buffer
	limit      int64
	overflow   bool
	onOverflow func()
}

func newLimitedWriter(limit int64, onOverflow func()) *limitedWriter {
	if limit <= 0 {
		limit = DefaultMaxBufferBytes
	}
	return &limitedWriter{
		limit:      limit,
		onOverflow: onOverflow,
	}
}

func (w *limitedWriter) Write(p []byte) (int, error) {
	remaining := w.limit - int64(w.buf.Len())
	if remaining <= 0 {
		w.overflow = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
		return 0, ErrBufferLimitExceeded
	}
	if int64(len(p)) > remaining {
		w.overflow = true
		if w.onOverflow != nil {
			w.onOverflow()
		}
		_, _ = w.buf.Write(p[:remaining])
		return int(remaining), ErrBufferLimitExceeded
	}
	return w.buf.Write(p)
}

func (w *limitedWriter) Bytes() []byte {
	return w.buf.Bytes()
}

func (w *limitedWriter) String() string {
	return w.buf.String()
}

// autoCleanReader wraps *os.File to cleanly remove jobDir when Close is called.
type autoCleanReader struct {
	file      *os.File
	jobDir    string
	closeOnce sync.Once
	closeErr  error
}

func newAutoCleanReader(file *os.File, jobDir string) *autoCleanReader {
	r := &autoCleanReader{
		file:   file,
		jobDir: jobDir,
	}

	runtime.SetFinalizer(r, func(cr *autoCleanReader) {
		_ = cr.Close()
	})

	return r
}

func (r *autoCleanReader) Read(p []byte) (int, error) {
	if r.file == nil {
		return 0, io.EOF
	}
	return r.file.Read(p)
}

// Close releases the underlying file descriptor and cleans up temporary directory.
func (r *autoCleanReader) Close() error {
	r.closeOnce.Do(func() {
		runtime.SetFinalizer(r, nil)
		if r.file != nil {
			r.closeErr = r.file.Close()
		}
		if r.jobDir != "" {
			if rmErr := os.RemoveAll(r.jobDir); rmErr != nil && r.closeErr == nil {
				r.closeErr = rmErr
			}
		}
	})
	return r.closeErr
}

// validateTempDir ensures the configured temporary directory exists, is a regular directory, and not a symlink.
func validateTempDir(path string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return os.TempDir(), nil
	}

	fi, err := os.Lstat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("temporary directory does not exist: %w", err)
	}
	if !fi.IsDir() {
		return "", errors.New("temporary path is not a directory")
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("temporary directory cannot be a symbolic link")
	}
	if fi.Mode()&0002 != 0 {
		return "", errors.New("insecure temporary directory: directory is world-writable")
	}

	parentDir := filepath.Dir(cleanPath)
	if parentFi, err := os.Lstat(parentDir); err == nil {
		if parentFi.Mode()&0002 != 0 && parentDir != "/" && parentDir != "/tmp" {
			if parentFi.Mode()&os.ModeSticky == 0 {
				return "", errors.New("insecure temporary directory: parent directory is world-writable")
			}
		}
	}

	return cleanPath, nil
}

// validateBinaryPath strictly validates that the custom binary path points to an executable named yt-dlp.
// Symbolic links and world-writable permissions are rejected using os.Lstat.
func validateBinaryPath(path string) (string, error) {
	cleanPath := filepath.Clean(strings.TrimSpace(path))
	if cleanPath == "" || cleanPath == "." {
		return "", errors.New("binary path is empty")
	}

	base := filepath.Base(cleanPath)
	if base != "yt-dlp" && base != "yt-dlp.exe" {
		return "", fmt.Errorf("binary name must be 'yt-dlp', got %q", base)
	}

	fi, err := os.Lstat(cleanPath)
	if err != nil {
		return "", fmt.Errorf("binary does not exist: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("symbolic links are not allowed for binary path")
	}
	if fi.IsDir() {
		return "", errors.New("binary path cannot be a directory")
	}
	if fi.Mode()&0111 == 0 {
		return "", errors.New("binary file is not executable")
	}
	if fi.Mode()&0002 != 0 {
		return "", errors.New("insecure binary: file is world-writable")
	}

	// Validate parent directory permissions
	parentDir := filepath.Dir(cleanPath)
	if parentFi, err := os.Lstat(parentDir); err == nil {
		if parentFi.Mode()&0002 != 0 {
			return "", errors.New("insecure binary: parent directory is world-writable")
		}
	}

	return cleanPath, nil
}

// resolveYtDlpBinary checks system PATH and trusted standard fallback locations.
func resolveYtDlpBinary() (string, error) {
	if lookPath, err := exec.LookPath("yt-dlp"); err == nil {
		if val, err := validateBinaryPath(lookPath); err == nil {
			return val, nil
		}
	}

	homeDir, _ := os.UserHomeDir()
	trustedPaths := []string{
		filepath.Join(homeDir, ".local", "bin", "yt-dlp"),
		"/usr/local/bin/yt-dlp",
		"/usr/bin/yt-dlp",
		"/bin/yt-dlp",
	}

	for _, p := range trustedPaths {
		if val, err := validateBinaryPath(p); err == nil {
			return val, nil
		}
	}

	return "", ErrYtDlpNotFound
}

// Ensure Extractor satisfies services.YouTubeExtractor and services.MediaLinkExtractor at compile time.
var _ services.YouTubeExtractor = (*Extractor)(nil)
var _ services.MediaLinkExtractor = (*Extractor)(nil)
