package services

import (
	"errors"
	"io"
	"strings"
	"testing"

	"code-base-golang/internal/constants"
)

func TestMaxBytesReader(t *testing.T) {
	t.Run("reading under limit succeeds", func(t *testing.T) {
		input := "hello world"
		reader := &maxBytesReader{
			r:     strings.NewReader(input),
			limit: 20,
		}

		buf, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(buf) != input {
			t.Errorf("expected %q, got %q", input, string(buf))
		}
		if reader.read != int64(len(input)) {
			t.Errorf("expected read count %d, got %d", len(input), reader.read)
		}
	})

	t.Run("reading exact limit succeeds", func(t *testing.T) {
		input := "12345"
		reader := &maxBytesReader{
			r:     strings.NewReader(input),
			limit: 5,
		}

		buf, err := io.ReadAll(reader)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(buf) != input {
			t.Errorf("expected %q, got %q", input, string(buf))
		}
	})

	t.Run("reading exceeding limit returns ErrPayloadTooLarge", func(t *testing.T) {
		input := "1234567890"
		reader := &maxBytesReader{
			r:     strings.NewReader(input),
			limit: 5,
		}

		_, err := io.ReadAll(reader)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		expectedErr := constants.ErrPayloadTooLarge.WithMessage("media size from link exceeds maximum limit of 1GB")
		if !errors.Is(err, expectedErr) {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})
}
