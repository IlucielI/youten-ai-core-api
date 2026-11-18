package video

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"code-base-golang/internal/dtos"
)

func TestFFmpegVisualExtractor_LookPath(t *testing.T) {
	extractor := NewFFmpegVisualExtractor()
	if extractor == nil {
		t.Fatal("expected non-nil extractor")
	}
}

func TestFFmpegVisualExtractor_EmptyInput(t *testing.T) {
	extractor := &FFmpegVisualExtractor{binaryPath: "/bin/sh"}
	_, err := extractor.ExtractKeyframes(context.Background(), nil, "test.mp4", dtos.KeyframeExtractionOptions{})
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
}

func TestFFmpegVisualExtractor_MissingBinary(t *testing.T) {
	extractor := &FFmpegVisualExtractor{binaryPath: ""}
	_, err := extractor.ExtractKeyframes(context.Background(), strings.NewReader("dummy"), "test.mp4", dtos.KeyframeExtractionOptions{})
	if !errors.Is(err, ErrFFmpegNotFound) {
		t.Errorf("expected ErrFFmpegNotFound, got %v", err)
	}
}

func TestParseShowinfoTimestamps(t *testing.T) {
	sampleOutput := `
[Parsed_showinfo_1 @ 0x55d7f0] n: 0 pts: -23 pts_time: -0.023000 pos: 512 fmt: yuv420p
[Parsed_showinfo_1 @ 0x55d7f0] n: 1 pts: 12000 pts_time: 12.500000 pos: 1024 fmt: yuv420p
[Parsed_showinfo_1 @ 0x55d7f0] n: 2 pts: 48000 pts_time: 50.000000 pos: 2048 fmt: yuv420p
[Parsed_showinfo_1 @ 0x55d7f0] n: 3 pts: 96000 pts_time: 100.25 pos: 4096 fmt: yuv420p
`
	ts := parseShowinfoTimestamps(sampleOutput)
	if len(ts) != 4 {
		t.Fatalf("expected 4 timestamps, got %d", len(ts))
	}
	if ts[0] != -0.023 || ts[1] != 12.5 || ts[2] != 50.0 || ts[3] != 100.25 {
		t.Errorf("unexpected parsed timestamps: %v", ts)
	}
}

func TestFFmpegVisualExtractor_ContextCanceled(t *testing.T) {
	extractor := &FFmpegVisualExtractor{binaryPath: "/bin/sleep"}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := extractor.ExtractKeyframes(ctx, strings.NewReader("dummy video content"), "test.mp4", dtos.KeyframeExtractionOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestIsTimestampCovered(t *testing.T) {
	frames := []dtos.ExtractedKeyframe{
		{TimestampSeconds: 10.0},
		{TimestampSeconds: 30.0},
	}

	if !isTimestampCovered(frames, 12.0, 5.0) {
		t.Errorf("expected 12.0s to be covered by 10.0s frame with 5.0s window")
	}
	if isTimestampCovered(frames, 20.0, 5.0) {
		t.Errorf("expected 20.0s not to be covered with 5.0s window")
	}
}

func TestDownsampleKeyframes(t *testing.T) {
	var frames []dtos.ExtractedKeyframe
	for i := 0; i < 100; i++ {
		frames = append(frames, dtos.ExtractedKeyframe{
			TimestampSeconds: float64(i * 10),
			SequenceOrder:    i + 1,
		})
	}

	downsampled := downsampleKeyframes(frames, 10)
	if len(downsampled) != 10 {
		t.Fatalf("expected 10 frames, got %d", len(downsampled))
	}
	if downsampled[0].TimestampSeconds != 0 {
		t.Errorf("expected first frame at 0s, got %.1fs", downsampled[0].TimestampSeconds)
	}
	if downsampled[len(downsampled)-1].TimestampSeconds != 990 {
		t.Errorf("expected last frame at 990s, got %.1fs", downsampled[len(downsampled)-1].TimestampSeconds)
	}

	// Test downsampling to 1 frame (division by zero guardrail)
	single := downsampleKeyframes(frames, 1)
	if len(single) != 1 {
		t.Fatalf("expected 1 frame, got %d", len(single))
	}
	if single[0].TimestampSeconds != 0 {
		t.Errorf("expected frame at 0s, got %.1fs", single[0].TimestampSeconds)
	}
}

func TestMockVisualExtractor(t *testing.T) {
	mock := NewMockVisualExtractor()
	frames, err := mock.ExtractKeyframes(context.Background(), bytes.NewReader([]byte("test")), "video.mp4", dtos.KeyframeExtractionOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 keyframes, got %d", len(frames))
	}

	// Test nil reader
	_, err = mock.ExtractKeyframes(context.Background(), nil, "video.mp4", dtos.KeyframeExtractionOptions{})
	if !errors.Is(err, ErrEmptyInput) {
		t.Errorf("expected ErrEmptyInput, got %v", err)
	}
}

