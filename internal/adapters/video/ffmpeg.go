package video

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"

	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/phash"
	"code-base-golang/internal/services"
)

var (
	// ErrFFmpegNotFound is returned when ffmpeg binary is missing from system PATH.
	ErrFFmpegNotFound = errors.New("ffmpeg executable not found in PATH")
	// ErrEmptyInput is returned when input stream is nil.
	ErrEmptyInput = errors.New("video extraction input reader is empty")
	// ErrNoVideoFrames is returned when no candidate keyframes could be extracted.
	ErrNoVideoFrames = errors.New("no candidate keyframes extracted from video")
)

var (
	ptsTimeRegex = regexp.MustCompile(`pts_time:\s*([0-9.]+)`)
)

// FFmpegVisualExtractor extracts visual keyframes using local FFmpeg and perceptual hash deduplication.
type FFmpegVisualExtractor struct {
	binaryPath string
}

// NewFFmpegVisualExtractor creates a new FFmpegVisualExtractor instance.
func NewFFmpegVisualExtractor() *FFmpegVisualExtractor {
	path, _ := exec.LookPath("ffmpeg")
	return &FFmpegVisualExtractor{
		binaryPath: path,
	}
}

// IsAvailable returns true if ffmpeg executable exists.
func (f *FFmpegVisualExtractor) IsAvailable() bool {
	return f.binaryPath != ""
}

// ExtractKeyframes extracts candidate visual keyframes from video streams.
func (f *FFmpegVisualExtractor) ExtractKeyframes(ctx context.Context, input io.Reader, filename string, opts dtos.KeyframeExtractionOptions) ([]dtos.ExtractedKeyframe, error) {
	if !f.IsAvailable() {
		return nil, ErrFFmpegNotFound
	}
	if input == nil {
		return nil, ErrEmptyInput
	}

	tempDir, err := os.MkdirTemp("", "youten-keyframes-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tempDir)

	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".mp4"
	}
	inputPath := filepath.Join(tempDir, "input"+ext)
	outFile, err := os.Create(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create input temp file: %w", err)
	}

	if _, err := io.Copy(outFile, input); err != nil {
		outFile.Close()
		return nil, fmt.Errorf("failed to write video stream: %w", err)
	}
	if err := outFile.Close(); err != nil {
		return nil, fmt.Errorf("failed to close video input file: %w", err)
	}

	sceneThreshold := opts.SceneThreshold
	if sceneThreshold <= 0 {
		sceneThreshold = 0.25
	}
	minInterval := opts.MinIntervalSeconds
	if minInterval <= 0 {
		minInterval = 5.0
	}
	targetWidth := opts.TargetWidth
	if targetWidth <= 0 {
		targetWidth = 1280
	}

	// Step 1: Run FFmpeg with scene-change filter + showinfo to capture timestamps
	framePattern := filepath.Join(tempDir, "scene_%05d.jpg")
	filterStr := fmt.Sprintf("select='gt(scene,%0.2f)',showinfo,scale='min(%d,iw)':-2", sceneThreshold, targetWidth)

	cmd := exec.CommandContext(ctx, f.binaryPath,
		"-y",
		"-i", inputPath,
		"-vf", filterStr,
		"-vsync", "vfr",
		"-q:v", "3",
		framePattern,
	)

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	_ = cmd.Run() // FFmpeg may exit with non-zero if stream ends or EOF, so we inspect generated frames

	// Parse timestamps from showinfo stderr
	timestamps := parseShowinfoTimestamps(stderrBuf.String())

	// Read extracted scene frame files
	files, _ := filepath.Glob(filepath.Join(tempDir, "scene_*.jpg"))
	sort.Strings(files)

	var candidates []dtos.ExtractedKeyframe
	for idx, filePath := range files {
		data, err := os.ReadFile(filePath)
		if err != nil || len(data) == 0 {
			continue
		}

		ts := 0.0
		if idx < len(timestamps) {
			ts = timestamps[idx]
		}

		dh, err := phash.DHashFromBytes(data)
		if err != nil {
			continue
		}

		candidates = append(candidates, dtos.ExtractedKeyframe{
			TimestampSeconds: ts,
			ImageBytes:       data,
			MimeType:         "image/jpeg",
			DHash:            dh,
			SequenceOrder:    idx + 1,
		})
	}

	// Step 2: Sample audio anchor timestamps if any anchor is not covered by scene frames
	for _, anchorTs := range opts.AudioAnchors {
		if anchorTs < 0 {
			continue
		}
		if isTimestampCovered(candidates, anchorTs, minInterval) {
			continue
		}

		anchorFile := filepath.Join(tempDir, fmt.Sprintf("anchor_%08.2f.jpg", anchorTs))
		anchorCmd := exec.CommandContext(ctx, f.binaryPath,
			"-y",
			"-ss", fmt.Sprintf("%.2f", anchorTs),
			"-i", inputPath,
			"-frames:v", "1",
			"-vf", fmt.Sprintf("scale='min(%d,iw)':-2", targetWidth),
			"-q:v", "3",
			anchorFile,
		)
		if err := anchorCmd.Run(); err == nil {
			if data, err := os.ReadFile(anchorFile); err == nil && len(data) > 0 {
				if dh, err := phash.DHashFromBytes(data); err == nil {
					candidates = append(candidates, dtos.ExtractedKeyframe{
						TimestampSeconds: anchorTs,
						ImageBytes:       data,
						MimeType:         "image/jpeg",
						DHash:            dh,
					})
				}
			}
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoVideoFrames
	}

	// Step 3: Sort all candidates chronologically by timestamp
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].TimestampSeconds < candidates[j].TimestampSeconds
	})

	// Step 4: Apply perceptual hash deduplication and min-interval filtering
	var filtered []dtos.ExtractedKeyframe
	var prevKeyframe *dtos.ExtractedKeyframe

	for _, cand := range candidates {
		if prevKeyframe != nil {
			// Enforce minimum time interval between frames
			if (cand.TimestampSeconds - prevKeyframe.TimestampSeconds) < minInterval {
				continue
			}
			// Enforce perceptual deduplication
			if phash.IsDuplicate(prevKeyframe.DHash, cand.DHash) {
				continue
			}
		}

		current := cand
		current.SequenceOrder = len(filtered) + 1
		filtered = append(filtered, current)
		prevCopy := current
		prevKeyframe = &prevCopy
	}

	// Step 5: Enforce MaxFramesPerHour cap if configured
	if opts.MaxFramesPerHour > 0 && len(filtered) > 0 {
		maxFrames := opts.MaxFramesPerHour
		lastTs := filtered[len(filtered)-1].TimestampSeconds
		if lastTs > 3600 {
			hours := int(lastTs/3600.0) + 1
			maxFrames = opts.MaxFramesPerHour * hours
		}
		if len(filtered) > maxFrames {
			filtered = downsampleKeyframes(filtered, maxFrames)
		}
	}

	for i := range filtered {
		filtered[i].SequenceOrder = i + 1
	}

	return filtered, nil
}

// parseShowinfoTimestamps extracts pts_time values from FFmpeg showinfo filter output.
func parseShowinfoTimestamps(output string) []float64 {
	matches := ptsTimeRegex.FindAllStringSubmatch(output, -1)
	var timestamps []float64
	for _, m := range matches {
		if len(m) > 1 {
			if val, err := strconv.ParseFloat(m[1], 64); err == nil {
				timestamps = append(timestamps, val)
			}
		}
	}
	return timestamps
}

// isTimestampCovered checks whether an anchor timestamp is already represented within window seconds.
func isTimestampCovered(keyframes []dtos.ExtractedKeyframe, targetTs float64, window float64) bool {
	for _, kf := range keyframes {
		diff := kf.TimestampSeconds - targetTs
		if diff < 0 {
			diff = -diff
		}
		if diff <= window {
			return true
		}
	}
	return false
}

// downsampleKeyframes uniformly samples targetCount frames when total exceeds limit.
func downsampleKeyframes(frames []dtos.ExtractedKeyframe, targetCount int) []dtos.ExtractedKeyframe {
	if len(frames) <= targetCount || targetCount <= 0 {
		return frames
	}
	step := float64(len(frames)-1) / float64(targetCount-1)
	result := make([]dtos.ExtractedKeyframe, 0, targetCount)
	for i := 0; i < targetCount; i++ {
		idx := int(float64(i)*step + 0.5)
		if idx >= len(frames) {
			idx = len(frames) - 1
		}
		result = append(result, frames[idx])
	}
	return result
}

// Ensure interface satisfaction
var _ services.VisualExtractor = (*FFmpegVisualExtractor)(nil)
