package ocr

import (
	"context"
	"errors"
	"testing"
)

func TestEvaluateTextQuality(t *testing.T) {
	// High quality slide text
	goodSlide := `
Arsitektur Sistem Backend
1. PostgreSQL untuk data utama
2. Redis untuk caching
3. RabbitMQ untuk streaming antrean
Target SLA: 99.9% uptime
`
	clean, conf, usable := EvaluateTextQuality(goodSlide)
	if !usable || conf < 0.70 {
		t.Errorf("expected good slide to be usable with high confidence, got usable=%v, conf=%.2f", usable, conf)
	}
	if len(clean) == 0 {
		t.Errorf("expected non-empty cleaned text")
	}

	// Garbage noise
	garbage := `%%$$ @@!! ~~~ *** ///`
	_, confGarbage, usableGarbage := EvaluateTextQuality(garbage)
	if usableGarbage || confGarbage >= 0.70 {
		t.Errorf("expected garbage to be non-usable, got usable=%v, conf=%.2f", usableGarbage, confGarbage)
	}

	// Empty text
	_, confEmpty, usableEmpty := EvaluateTextQuality("")
	if usableEmpty || confEmpty > 0.30 {
		t.Errorf("expected empty text to be non-usable, got usable=%v", usableEmpty)
	}
}

func TestTesseractOCR_EmptyBytes(t *testing.T) {
	ocr := &TesseractOCR{binaryPath: "/bin/sh"}
	_, _, _, err := ocr.ExtractText(context.Background(), nil)
	if !errors.Is(err, ErrEmptyImageBytes) {
		t.Errorf("expected ErrEmptyImageBytes, got %v", err)
	}
}

func TestTesseractOCR_MissingBinary(t *testing.T) {
	ocr := &TesseractOCR{binaryPath: ""}
	_, _, _, err := ocr.ExtractText(context.Background(), []byte("fake-bytes"))
	if !errors.Is(err, ErrTesseractNotFound) {
		t.Errorf("expected ErrTesseractNotFound, got %v", err)
	}
}

func TestTesseractOCR_ExecutionFailure(t *testing.T) {
	ocr := &TesseractOCR{binaryPath: "/bin/false"}
	_, _, _, err := ocr.ExtractText(context.Background(), []byte("fake-bytes"))
	if err == nil {
		t.Fatalf("expected error on tesseract execution failure, got nil")
	}
}
