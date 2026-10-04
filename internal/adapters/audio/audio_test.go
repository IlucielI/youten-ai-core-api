package audio_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"code-base-golang/internal/adapters/audio"
	"code-base-golang/internal/services"
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
	mock.ExtractFn = func(ctx context.Context, input io.Reader, filename string) (*services.AudioExtractionResult, error) {
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
