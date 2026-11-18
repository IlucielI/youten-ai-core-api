package ocr

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"code-base-golang/internal/services"
)

var (
	// ErrTesseractNotFound is returned when the tesseract binary is not present in PATH.
	ErrTesseractNotFound = errors.New("tesseract executable not found in PATH")
	// ErrEmptyImageBytes is returned when input image buffer is empty.
	ErrEmptyImageBytes = errors.New("image bytes are empty")
)

// OCRResult holds extracted plain text and confidence evaluation.
type OCRResult struct {
	Text       string
	Confidence float64
	IsUsable   bool
}

// TesseractOCR extracts text locally from images using the system's tesseract binary.
type TesseractOCR struct {
	binaryPath string
}

// NewTesseractOCR creates a new TesseractOCR instance.
func NewTesseractOCR() *TesseractOCR {
	path, _ := exec.LookPath("tesseract")
	return &TesseractOCR{
		binaryPath: path,
	}
}

// IsAvailable returns true if tesseract binary exists in PATH.
func (t *TesseractOCR) IsAvailable() bool {
	return t.binaryPath != ""
}

// ExtractText executes tesseract CLI on image bytes and assesses confidence.
func (t *TesseractOCR) ExtractText(ctx context.Context, imgBytes []byte) (text string, confidence float64, isUsable bool, err error) {
	if !t.IsAvailable() {
		return "", 0.0, false, ErrTesseractNotFound
	}
	if len(imgBytes) == 0 {
		return "", 0.0, false, ErrEmptyImageBytes
	}

	tempDir, err := os.MkdirTemp("", "youten-ocr-*")
	if err != nil {
		return "", 0.0, false, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	imgPath := filepath.Join(tempDir, "input.jpg")
	if err := os.WriteFile(imgPath, imgBytes, 0600); err != nil {
		return "", 0.0, false, fmt.Errorf("failed to write temp image: %w", err)
	}

	cmd := exec.CommandContext(ctx, t.binaryPath, imgPath, "stdout", "-l", "ind+eng", "--psm", "6")
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	if err := cmd.Run(); err != nil {
		// Fallback to default language if ind is missing
		cmdFallback := exec.CommandContext(ctx, t.binaryPath, imgPath, "stdout", "--psm", "6")
		stdoutBuf.Reset()
		cmdFallback.Stdout = &stdoutBuf
		if errFallback := cmdFallback.Run(); errFallback != nil {
			return "", 0.0, false, nil
		}
	}

	rawText := strings.TrimSpace(stdoutBuf.String())
	cleanText, confidence, usable := EvaluateTextQuality(rawText)

	return cleanText, confidence, usable, nil
}

var _ services.LocalOCRExtractor = (*TesseractOCR)(nil)

// EvaluateTextQuality cleans raw OCR text and evaluates confidence and usability.
func EvaluateTextQuality(raw string) (cleaned string, confidence float64, isUsable bool) {
	lines := strings.Split(raw, "\n")
	var validLines []string
	var letterCount, totalRunes int

	for _, line := range lines {
		trim := strings.TrimSpace(line)
		if len(trim) == 0 {
			continue
		}
		for _, r := range trim {
			totalRunes++
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				letterCount++
			}
		}
		validLines = append(validLines, trim)
	}

	cleaned = strings.Join(validLines, "\n")
	if totalRunes == 0 || letterCount < 15 {
		return cleaned, 0.20, false
	}

	ratio := float64(letterCount) / float64(totalRunes)
	if ratio >= 0.75 && len(cleaned) >= 25 {
		return cleaned, 0.85, true
	}
	if ratio >= 0.60 && len(cleaned) >= 15 {
		return cleaned, 0.60, false
	}
	return cleaned, 0.30, false
}
