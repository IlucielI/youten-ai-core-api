package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/services"
)

var (
	// ErrFFmpegNotFound is returned when the ffmpeg executable is not installed on the host.
	ErrFFmpegNotFound = errors.New("ffmpeg executable not found in PATH")
	// ErrEmptyInput is returned when input stream is empty.
	ErrEmptyInput = errors.New("audio extraction input reader is empty")
)

// FFmpegExtractor extracts mono audio using the system's ffmpeg CLI binary.
type FFmpegExtractor struct {
	binaryPath string
}

// NewFFmpeg creates a new FFmpegExtractor.
func NewFFmpeg() *FFmpegExtractor {
	path, _ := exec.LookPath("ffmpeg")
	return &FFmpegExtractor{
		binaryPath: path,
	}
}

// IsAvailable returns true if ffmpeg binary was detected in PATH.
func (f *FFmpegExtractor) IsAvailable() bool {
	return f.binaryPath != ""
}

// ExtractMonoAudio converts video/audio stream into 16kHz 64kbps mono MP3 format.
func (f *FFmpegExtractor) ExtractMonoAudio(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error) {
	if !f.IsAvailable() {
		return nil, ErrFFmpegNotFound
	}
	if input == nil {
		return nil, ErrEmptyInput
	}

	tempDir, err := os.MkdirTemp("", "youten-audio-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".tmp"
	}
	inputPath := filepath.Join(tempDir, "input"+ext)
	outputPath := filepath.Join(tempDir, "output.mp3")

	inputFile, err := os.Create(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create input temp file: %w", err)
	}

	written, err := io.Copy(inputFile, input)
	_ = inputFile.Close()
	if err != nil {
		return nil, fmt.Errorf("failed to write temp input audio: %w", err)
	}
	if written == 0 {
		return nil, ErrEmptyInput
	}

	// ffmpeg command: -i <input> -vn (no video) -ac 1 (mono) -ar 16000 (16kHz) -b:a 64k (64kbps) -f mp3 <output>
	cmd := exec.CommandContext(ctx, f.binaryPath,
		"-y",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-b:a", "64k",
		"-f", "mp3",
		outputPath,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg execution failed (%w): %s", err, strings.TrimSpace(stderr.String()))
	}

	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted audio file: %w", err)
	}

	return &dtos.AudioExtractionResult{
		Reader:          io.NopCloser(bytes.NewReader(outputData)),
		Format:          "mp3",
		DurationSeconds: 0, // Duration will be extracted via STT or metadata
		SizeBytes:       int64(len(outputData)),
	}, nil
}

// Ensure FFmpegExtractor satisfies services.AudioExtractor at compile time.
var _ services.AudioExtractor = (*FFmpegExtractor)(nil)
