package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"code-base-golang/internal/constants"
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

	// Acoustic speech enhancement:
	// 1. highpass=f=80: eliminates low-frequency HVAC rumble, air conditioning hum, and desk thumps.
	// 2. lowpass=f=8000: attenuates high-frequency noise outside human vocal spectrum.
	// 3. dynaudnorm=p=0.95:m=10:r=0.9:s=12: Dynamic Audio Normalizer continuously amplifies quiet/distant
	//    speakers (up to +10-12dB) while preventing louder participants from clipping, ensuring balanced vocal levels.
	// 4. -b:a 128k: preserves speech harmonics and sibilants critical for ASR phonetic accuracy.
	enhancedArgs := []string{
		"-y",
		"-i", inputPath,
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-af", "highpass=f=80,lowpass=f=8000,dynaudnorm=p=0.95:m=10:r=0.9:s=12",
		"-b:a", "128k",
		"-f", "mp3",
		outputPath,
	}

	cmd := exec.CommandContext(ctx, f.binaryPath, enhancedArgs...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		// Fallback to standard mono extraction if filter chain is unsupported on input stream
		basicArgs := []string{
			"-y",
			"-i", inputPath,
			"-vn",
			"-ac", "1",
			"-ar", "16000",
			"-b:a", "64k",
			"-f", "mp3",
			outputPath,
		}
		stderr.Reset()
		fallbackCmd := exec.CommandContext(ctx, f.binaryPath, basicArgs...)
		fallbackCmd.Stderr = &stderr
		if fallbackErr := fallbackCmd.Run(); fallbackErr != nil {
			return nil, fmt.Errorf("ffmpeg execution failed (%w): %s", fallbackErr, strings.TrimSpace(stderr.String()))
		}
	}

	outputData, err := os.ReadFile(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read converted audio file: %w", err)
	}

	durationSeconds := ParseDuration(stderr.String())

	return &dtos.AudioExtractionResult{
		Reader:          io.NopCloser(bytes.NewReader(outputData)),
		Format:          "mp3",
		DurationSeconds: durationSeconds,
		SizeBytes:       int64(len(outputData)),
	}, nil
}

var durationRegex = regexp.MustCompile(`Duration:\s*(\d{2,}):(\d{2}):(\d{2}(?:\.\d+)?)`)

// ParseDuration extracts the media duration in seconds from ffmpeg log output.
func ParseDuration(output string) float64 {
	matches := durationRegex.FindStringSubmatch(output)
	if len(matches) != 4 {
		return 0
	}
	hours, _ := strconv.ParseFloat(matches[1], 64)
	minutes, _ := strconv.ParseFloat(matches[2], 64)
	seconds, _ := strconv.ParseFloat(matches[3], 64)
	return hours*3600 + minutes*60 + seconds
}

// Ensure FFmpegExtractor satisfies services.AudioExtractor and services.AudioSlicer at compile time.
var (
	_ services.AudioExtractor = (*FFmpegExtractor)(nil)
	_ services.AudioSlicer    = (*FFmpegExtractor)(nil)
)

var silenceEndRegex = regexp.MustCompile(`silence_end:\s*(\d+(?:\.\d+)?)`)

// ParseSilencePoints extracts audio silence end timestamps from ffmpeg silencedetect log output.
func ParseSilencePoints(output string) []float64 {
	matches := silenceEndRegex.FindAllStringSubmatch(output, -1)
	if len(matches) == 0 {
		return nil
	}
	var points []float64
	for _, m := range matches {
		if len(m) > 1 {
			val, err := strconv.ParseFloat(m[1], 64)
			if err == nil && val > 0 {
				points = append(points, val)
			}
		}
	}
	return points
}

// ComputeChunkBoundaries computes [start, end] intervals for chunking using detected silence points.
func ComputeChunkBoundaries(totalDuration float64, silencePoints []float64, minSec, targetSec, maxSec float64) [][2]float64 {
	if totalDuration <= 0 {
		return nil
	}
	if minSec <= 0 {
		minSec = constants.DefaultAudioChunkMinSeconds
	}
	if targetSec <= 0 {
		targetSec = constants.DefaultAudioChunkTargetSeconds
	}
	if maxSec <= 0 {
		maxSec = constants.DefaultAudioChunkMaxSeconds
	}

	// For recordings under threshold duration, keep single chunk
	if totalDuration < constants.MinAudioDurationForChunkingSeconds {
		return [][2]float64{{0, totalDuration}}
	}

	var boundaries [][2]float64
	currStart := 0.0

	for currStart+maxSec < totalDuration {
		windowMin := currStart + minSec
		windowMax := currStart + maxSec
		idealTarget := currStart + targetSec

		// Find best silence point in [windowMin, windowMax] closest to idealTarget
		bestPoint := -1.0
		bestDist := 1e9

		for _, p := range silencePoints {
			if p >= windowMin && p <= windowMax {
				dist := math.Abs(p - idealTarget)
				if dist < bestDist {
					bestDist = dist
					bestPoint = p
				}
			}
		}

		cutPoint := math.Max(windowMin, math.Min(idealTarget, windowMax))
		if bestPoint > 0 {
			cutPoint = bestPoint
		}

		boundaries = append(boundaries, [2]float64{currStart, cutPoint})
		currStart = cutPoint
	}

	// Final remaining chunk to end
	if currStart < totalDuration {
		boundaries = append(boundaries, [2]float64{currStart, totalDuration})
	}

	return boundaries
}

// SliceAudioOnSilence splits audio longer than threshold duration into silence-aligned chunks.
func (f *FFmpegExtractor) SliceAudioOnSilence(ctx context.Context, input io.Reader, filename string, totalDurationSec float64) ([]dtos.AudioChunk, error) {
	if !f.IsAvailable() {
		return nil, ErrFFmpegNotFound
	}
	if input == nil {
		return nil, ErrEmptyInput
	}

	tempDir, err := os.MkdirTemp("", "youten-slice-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	ext := filepath.Ext(filename)
	format := strings.TrimPrefix(strings.ToLower(ext), ".")
	if format == "" {
		format = "mp3"
		ext = ".mp3"
	}
	inputPath := filepath.Join(tempDir, "input"+ext)

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

	if totalDurationSec <= 0 {
		probeCmd := exec.CommandContext(ctx, f.binaryPath, "-i", inputPath)
		var probeErr bytes.Buffer
		probeCmd.Stderr = &probeErr
		_ = probeCmd.Run()
		totalDurationSec = ParseDuration(probeErr.String())
	}

	if totalDurationSec < constants.MinAudioDurationForChunkingSeconds {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read input file: %w", err)
		}
		return []dtos.AudioChunk{{
			Index:           0,
			StartOffsetSec:  0,
			DurationSeconds: totalDurationSec,
			Data:            data,
			Format:          format,
		}}, nil
	}

	detectCmd := exec.CommandContext(ctx, f.binaryPath, "-i", inputPath, "-af", "silencedetect=noise=-30dB:d=0.5", "-f", "null", "-")
	var detectErr bytes.Buffer
	detectCmd.Stderr = &detectErr
	_ = detectCmd.Run()

	silencePoints := ParseSilencePoints(detectErr.String())
	boundaries := ComputeChunkBoundaries(
		totalDurationSec,
		silencePoints,
		constants.DefaultAudioChunkMinSeconds,
		constants.DefaultAudioChunkTargetSeconds,
		constants.DefaultAudioChunkMaxSeconds,
	)

	if len(boundaries) <= 1 {
		data, err := os.ReadFile(inputPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read input file: %w", err)
		}
		return []dtos.AudioChunk{{
			Index:           0,
			StartOffsetSec:  0,
			DurationSeconds: totalDurationSec,
			Data:            data,
			Format:          format,
		}}, nil
	}

	var chunks []dtos.AudioChunk
	for i, b := range boundaries {
		start := b[0]
		end := b[1]
		chunkDuration := end - start
		if chunkDuration <= 0 {
			continue
		}

		outChunkPath := filepath.Join(tempDir, fmt.Sprintf("chunk_%d.mp3", i))
		sliceArgs := []string{
			"-y",
			"-ss", fmt.Sprintf("%.3f", start),
			"-to", fmt.Sprintf("%.3f", end),
			"-i", inputPath,
			"-vn",
			"-ac", "1",
			"-ar", "16000",
			"-b:a", "64k",
			"-f", "mp3",
			outChunkPath,
		}

		cmd := exec.CommandContext(ctx, f.binaryPath, sliceArgs...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			return nil, fmt.Errorf("ffmpeg slicing chunk %d failed (%w): %s", i, err, strings.TrimSpace(stderr.String()))
		}

		chunkData, err := os.ReadFile(outChunkPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read chunk %d: %w", i, err)
		}

		chunks = append(chunks, dtos.AudioChunk{
			Index:           i,
			StartOffsetSec:  start,
			DurationSeconds: chunkDuration,
			Data:            chunkData,
			Format:          "mp3",
		})
	}

	return chunks, nil
}
