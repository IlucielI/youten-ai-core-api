package audio_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"code-base-golang/internal/adapters/audio"
	"code-base-golang/internal/dtos"
)

func TestMockAudioExtractor_Success(t *testing.T) {
	mock := audio.NewMock()

	sampleData := []byte("fake audio mp3 byte data")
	result, err := mock.ExtractMonoAudio(context.Background(), bytes.NewReader(sampleData), "test.mp4")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Format != "mp3" {
		t.Errorf("expected format mp3, got %s", result.Format)
	}
	if result.SizeBytes != int64(len(sampleData)) {
		t.Errorf("expected size %d, got %d", len(sampleData), result.SizeBytes)
	}

	outBytes, err := io.ReadAll(result.Reader)
	if err != nil {
		t.Fatalf("failed reading output reader: %v", err)
	}
	if !bytes.Equal(outBytes, sampleData) {
		t.Errorf("expected output bytes to match sample data")
	}
}

func TestMockAudioExtractor_EmptyInput(t *testing.T) {
	mock := audio.NewMock()

	_, err := mock.ExtractMonoAudio(context.Background(), strings.NewReader(""), "empty.wav")
	if err == nil {
		t.Fatalf("expected error on empty reader, got nil")
	}

	_, err = mock.ExtractMonoAudio(context.Background(), nil, "nil.wav")
	if err == nil {
		t.Fatalf("expected error on nil reader, got nil")
	}
}

func TestMockAudioExtractor_CustomError(t *testing.T) {
	mock := audio.NewMock()
	mock.ExtractFn = func(ctx context.Context, input io.Reader, filename string) (*dtos.AudioExtractionResult, error) {
		return nil, errors.New("custom audio decode error")
	}

	_, err := mock.ExtractMonoAudio(context.Background(), strings.NewReader("sample"), "test.mp4")
	if err == nil || !strings.Contains(err.Error(), "custom audio decode error") {
		t.Errorf("expected custom error, got %v", err)
	}
}

func TestFFmpegExtractor_LookPath(t *testing.T) {
	ffmpeg := audio.NewFFmpeg()
	if !ffmpeg.IsAvailable() {
		_, err := ffmpeg.ExtractMonoAudio(context.Background(), strings.NewReader("dummy"), "test.mp4")
		if !errors.Is(err, audio.ErrFFmpegNotFound) {
			t.Errorf("expected ErrFFmpegNotFound when ffmpeg is missing, got %v", err)
		}
	}
}

func TestParseDuration(t *testing.T) {
	logOutput := `Input #0, matroska,webm, from 'input.webm':
  Metadata:
    encoder         : google/video-file
  Duration: 00:06:10.78, start: 0.000000, bitrate: 128 kb/s
  Stream #0:0: Audio: opus, 48000 Hz, stereo, fltp (default)`

	d := audio.ParseDuration(logOutput)
	expected := 6*60 + 10.78
	if d != expected {
		t.Errorf("expected %f, got %f", expected, d)
	}

	invalid := audio.ParseDuration("no duration info")
	if invalid != 0 {
		t.Errorf("expected 0 for invalid, got %f", invalid)
	}
}

func TestParseSilencePoints(t *testing.T) {
	logOutput := `[silencedetect @ 0x55d01] silence_start: 120.45
[silencedetect @ 0x55d01] silence_end: 122.10 | silence_duration: 1.65
[silencedetect @ 0x55d01] silence_start: 350.12
[silencedetect @ 0x55d01] silence_end: 352.00 | silence_duration: 1.88
[silencedetect @ 0x55d01] silence_start: 710.00
[silencedetect @ 0x55d01] silence_end: 712.50 | silence_duration: 2.50`

	points := audio.ParseSilencePoints(logOutput)
	if len(points) != 3 {
		t.Fatalf("expected 3 silence points, got %d", len(points))
	}
	if points[0] != 122.10 || points[1] != 352.00 || points[2] != 712.50 {
		t.Errorf("unexpected silence points: %v", points)
	}

	empty := audio.ParseSilencePoints("no silence detected here")
	if len(empty) != 0 {
		t.Errorf("expected empty points on invalid log, got %v", empty)
	}
}

func TestComputeChunkBoundaries(t *testing.T) {
	// 1. Audio under threshold (< 600s) -> single chunk
	shortBoundaries := audio.ComputeChunkBoundaries(500.0, []float64{100.0, 200.0}, 240.0, 360.0, 480.0)
	if len(shortBoundaries) != 1 {
		t.Fatalf("expected 1 boundary for short audio, got %d", len(shortBoundaries))
	}
	if shortBoundaries[0][0] != 0 || shortBoundaries[0][1] != 500.0 {
		t.Errorf("expected [0, 500], got %v", shortBoundaries[0])
	}

	// 2. Audio of 1200s with silence point at 350.0s and 720.0s
	silencePoints := []float64{350.0, 720.0, 1050.0}
	longBoundaries := audio.ComputeChunkBoundaries(1200.0, silencePoints, 240.0, 360.0, 480.0)
	if len(longBoundaries) < 2 {
		t.Fatalf("expected at least 2 boundaries for 1200s audio, got %d", len(longBoundaries))
	}
	// First chunk should split at the detected silence point (350.0)
	if longBoundaries[0][0] != 0 || longBoundaries[0][1] != 350.0 {
		t.Errorf("expected chunk 0 [0, 350.0], got %v", longBoundaries[0])
	}

	// 3. Fallback when no silence points found in window
	fallbackBoundaries := audio.ComputeChunkBoundaries(1000.0, nil, 240.0, 360.0, 480.0)
	if len(fallbackBoundaries) < 2 {
		t.Fatalf("expected fallback boundaries for 1000s audio without silence, got %d", len(fallbackBoundaries))
	}
	if fallbackBoundaries[0][0] != 0 || fallbackBoundaries[0][1] != 360.0 {
		t.Errorf("expected fallback chunk 0 [0, 360.0], got %v", fallbackBoundaries[0])
	}
}

func TestMockAudioExtractor_SliceAudioOnSilence(t *testing.T) {
	mock := audio.NewMock()
	ctx := context.Background()

	// Default behavior
	chunks, err := mock.SliceAudioOnSilence(ctx, strings.NewReader("dummy audio bytes"), "sample.wav", 120.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(chunks))
	}
	if chunks[0].DurationSeconds != 120.0 {
		t.Errorf("expected 120.0s, got %f", chunks[0].DurationSeconds)
	}
	if chunks[0].Format != "wav" {
		t.Errorf("expected format wav, got %s", chunks[0].Format)
	}

	// Empty input checks
	_, err = mock.SliceAudioOnSilence(ctx, nil, "sample.mp3", 120.0)
	if err != audio.ErrEmptyInput {
		t.Errorf("expected ErrEmptyInput on nil reader, got %v", err)
	}
	_, err = mock.SliceAudioOnSilence(ctx, strings.NewReader(""), "sample.mp3", 120.0)
	if err != audio.ErrEmptyInput {
		t.Errorf("expected ErrEmptyInput on empty reader, got %v", err)
	}

	// Custom slice behavior
	mock.SliceFn = func(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error) {
		return []dtos.AudioChunk{
			{Index: 0, StartOffsetSec: 0, DurationSeconds: 300, Data: []byte("c1"), Format: "mp3"},
			{Index: 1, StartOffsetSec: 300, DurationSeconds: 300, Data: []byte("c2"), Format: "mp3"},
		}, nil
	}

	customChunks, err := mock.SliceAudioOnSilence(ctx, strings.NewReader("audio"), "test.mp3", 600.0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(customChunks) != 2 {
		t.Errorf("expected 2 custom chunks, got %d", len(customChunks))
	}
}
