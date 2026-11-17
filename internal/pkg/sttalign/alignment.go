package sttalign

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"code-base-golang/internal/dtos"
)

var (
	// Regex matching acoustic reference line: "Speaker X [0.00s - 10.00s]: text"
	acousticLineRegex = regexp.MustCompile(`(?i)^(?:\[)?(Speaker\s*\w+|Pembicara\s*\w+)(?:\])?\s*\[\s*([\d\.]+)\s*s?\s*-\s*([\d\.]+)\s*s?\]\s*:\s*(.*)$`)

	// Non-speech and music noise tokens frequently hallucinated by ASR
	nonSpeechTokens = map[string]bool{
		"musik":            true,
		"music":            true,
		"instrumental":     true,
		"background music": true,
		"intro":            true,
		"outro":            true,
		"applause":         true,
		"tepuk tangan":     true,
		"laughter":         true,
		"tertawa":          true,
		"tawa":             true,
		"hening":           true,
		"silence":          true,
		"...":              true,
		"....":             true,
		"batuk":            true,
		"cough":            true,
	}
)

// CalculateOverlap returns the duration of temporal overlap between two time intervals.
func CalculateOverlap(s1, e1, s2, e2 float64) float64 {
	overlap := math.Min(e1, e2) - math.Max(s1, s2)
	if overlap < 0 {
		return 0.0
	}
	return overlap
}

// FindBestAcousticSpeaker finds the speaker with the maximum overlap across diarized acoustic segments.
func FindBestAcousticSpeaker(start, end float64, diarizedSegments []dtos.SegmentResult) (bestSpeaker string, maxOverlap float64, totalOverlap float64) {
	if len(diarizedSegments) == 0 || end <= start {
		return "", 0.0, 0.0
	}

	speakerDurations := make(map[string]float64)
	for _, d := range diarizedSegments {
		spk := strings.TrimSpace(d.SpeakerLabel)
		if spk == "" {
			continue
		}
		ov := CalculateOverlap(start, end, d.Start, d.End)
		if ov > 0 {
			speakerDurations[spk] += ov
			totalOverlap += ov
		}
	}

	for spk, dur := range speakerDurations {
		if dur > maxOverlap {
			maxOverlap = dur
			bestSpeaker = spk
		}
	}
	return bestSpeaker, maxOverlap, totalOverlap
}

// AlignSegments aligns Whisper segments with acoustic diarization segments.
// It performs word-level speaker turn splitting if word timestamps exist,
// or segment-level overlap alignment with sentence clause splitting fallback.
func AlignSegments(whisperSegments []dtos.SegmentResult, diarizedSegments []dtos.SegmentResult) []dtos.SegmentResult {
	if len(whisperSegments) == 0 {
		return diarizedSegments
	}
	if len(diarizedSegments) == 0 {
		return whisperSegments
	}

	var aligned []dtos.SegmentResult

	for _, wSeg := range whisperSegments {
		if IsNonSpeechToken(wSeg.Text) {
			continue
		}

		// Case 1: Word-level alignment and speaker turn splitting
		if len(wSeg.Words) > 0 {
			splitWordSegs := alignAndSplitByWords(wSeg, diarizedSegments)
			if len(splitWordSegs) > 0 {
				aligned = append(aligned, splitWordSegs...)
				continue
			}
		}

		// Case 2: Segment-level overlap alignment
		dur := wSeg.End - wSeg.Start
		bestSpeaker, maxOv, _ := FindBestAcousticSpeaker(wSeg.Start, wSeg.End, diarizedSegments)

		confidence := 0.0
		if dur > 0 {
			confidence = maxOv / dur
			if confidence > 1.0 {
				confidence = 1.0
			}
		}

		// Check if segment should be split on clause boundary (when 2 speakers have significant overlap)
		splitClauseSegs := trySplitByClause(wSeg, diarizedSegments)
		if len(splitClauseSegs) > 1 {
			aligned = append(aligned, splitClauseSegs...)
			continue
		}

		wSegCopy := wSeg
		if bestSpeaker != "" {
			wSegCopy.SpeakerLabel = bestSpeaker
			wSegCopy.AcousticSpeaker = bestSpeaker
			wSegCopy.AcousticConfidence = confidence
			if wSegCopy.SpeakerName == "" {
				wSegCopy.SpeakerName = bestSpeaker
			}
		}
		aligned = append(aligned, wSegCopy)
	}

	// Renumber IDs sequentially
	for i := range aligned {
		aligned[i].ID = i
	}
	return aligned
}

// alignAndSplitByWords aligns each word with acoustic speakers and splits the segment
// into distinct segments whenever a speaker turn transition occurs.
func alignAndSplitByWords(wSeg dtos.SegmentResult, diarizedSegments []dtos.SegmentResult) []dtos.SegmentResult {
	type taggedWord struct {
		word    dtos.WordResult
		speaker string
	}

	tagged := make([]taggedWord, len(wSeg.Words))
	for i, w := range wSeg.Words {
		spk, _, _ := FindBestAcousticSpeaker(w.Start, w.End, diarizedSegments)
		tagged[i] = taggedWord{word: w, speaker: spk}
	}

	// Smooth out single-word isolated acoustic glitches within a phrase
	for i := 1; i < len(tagged)-1; i++ {
		prevSpk := tagged[i-1].speaker
		nextSpk := tagged[i+1].speaker
		currSpk := tagged[i].speaker
		if prevSpk != "" && prevSpk == nextSpk && currSpk != prevSpk {
			tagged[i].speaker = prevSpk
		}
	}

	// Fill any empty speaker from neighboring words or segment-level best speaker
	overallBest, _, _ := FindBestAcousticSpeaker(wSeg.Start, wSeg.End, diarizedSegments)
	lastSeen := overallBest
	for i := range tagged {
		if tagged[i].speaker != "" {
			lastSeen = tagged[i].speaker
		} else {
			tagged[i].speaker = lastSeen
		}
	}

	// Group contiguous words by speaker
	var runs [][]taggedWord
	var currentRun []taggedWord
	for _, tw := range tagged {
		if len(currentRun) == 0 || currentRun[len(currentRun)-1].speaker == tw.speaker {
			currentRun = append(currentRun, tw)
		} else {
			runs = append(runs, currentRun)
			currentRun = []taggedWord{tw}
		}
	}
	if len(currentRun) > 0 {
		runs = append(runs, currentRun)
	}

	if len(runs) <= 1 {
		// Single speaker across all words: assign speaker and return
		resSeg := wSeg
		if len(runs) == 1 && runs[0][0].speaker != "" {
			resSeg.SpeakerLabel = runs[0][0].speaker
			resSeg.AcousticSpeaker = runs[0][0].speaker
			resSeg.AcousticConfidence = 1.0
			if resSeg.SpeakerName == "" {
				resSeg.SpeakerName = runs[0][0].speaker
			}
		}
		return []dtos.SegmentResult{resSeg}
	}

	// Multiple speaker runs: create split segments
	var result []dtos.SegmentResult
	for _, r := range runs {
		if len(r) == 0 {
			continue
		}
		var wordsList []dtos.WordResult
		var textParts []string
		for _, tw := range r {
			wordsList = append(wordsList, tw.word)
			textParts = append(textParts, tw.word.Word)
		}
		text := strings.TrimSpace(strings.Join(textParts, " "))
		if text == "" {
			continue
		}

		spk := r[0].speaker
		if spk == "" {
			spk = overallBest
		}

		start := r[0].word.Start
		end := r[len(r)-1].word.End
		if end <= start {
			end = start + 0.1
		}

		result = append(result, dtos.SegmentResult{
			Start:              start,
			End:                end,
			Text:               text,
			SpeakerLabel:       spk,
			SpeakerName:        spk,
			AcousticSpeaker:    spk,
			AcousticConfidence: 1.0,
			Words:              wordsList,
		})
	}
	return result
}

// trySplitByClause splits a Whisper segment without word timestamps at punctuation boundaries
// if two distinct speakers each have significant overlap (> 25% duration).
func trySplitByClause(wSeg dtos.SegmentResult, diarizedSegments []dtos.SegmentResult) []dtos.SegmentResult {
	dur := wSeg.End - wSeg.Start
	if dur < 2.0 {
		return nil
	}

	// Find overlap per speaker
	speakerDurations := make(map[string]float64)
	for _, d := range diarizedSegments {
		spk := strings.TrimSpace(d.SpeakerLabel)
		if spk == "" {
			continue
		}
		ov := CalculateOverlap(wSeg.Start, wSeg.End, d.Start, d.End)
		if ov > 0 {
			speakerDurations[spk] += ov
		}
	}

	if len(speakerDurations) < 2 {
		return nil
	}

	type spkOv struct {
		speaker string
		overlap float64
	}
	var list []spkOv
	for s, ov := range speakerDurations {
		list = append(list, spkOv{speaker: s, overlap: ov})
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].overlap > list[j].overlap
	})

	// Check if top 2 speakers both have >= 25% of duration
	if list[0].overlap < 0.25*dur || list[1].overlap < 0.25*dur {
		return nil
	}

	// Find early speaker vs late speaker based on acoustic segments
	earlySpk, _, _ := FindBestAcousticSpeaker(wSeg.Start, wSeg.Start+dur*0.4, diarizedSegments)
	lateSpk, _, _ := FindBestAcousticSpeaker(wSeg.End-dur*0.4, wSeg.End, diarizedSegments)
	if earlySpk == "" || lateSpk == "" || earlySpk == lateSpk {
		return nil
	}

	// Find split point in text (prefer sentence terminators '?', '.', '!')
	text := strings.TrimSpace(wSeg.Text)
	splitIdx := -1
	for _, p := range []string{"? ", "! ", ". ", "?\n", "!\n", ".\n"} {
		if idx := strings.Index(text, p); idx != -1 {
			// Ensure split is not at the very edge (between 20% and 80% of text)
			pct := float64(idx) / float64(len(text))
			if pct >= 0.2 && pct <= 0.8 {
				splitIdx = idx + len(p)
				break
			}
		}
	}

	if splitIdx <= 0 {
		return nil
	}

	part1 := strings.TrimSpace(text[:splitIdx])
	part2 := strings.TrimSpace(text[splitIdx:])
	if part1 == "" || part2 == "" {
		return nil
	}

	splitRatio := float64(len(part1)) / float64(len(text))
	splitTime := wSeg.Start + splitRatio*dur

	return []dtos.SegmentResult{
		{
			Start:              wSeg.Start,
			End:                splitTime,
			Text:               part1,
			SpeakerLabel:       earlySpk,
			SpeakerName:        earlySpk,
			AcousticSpeaker:    earlySpk,
			AcousticConfidence: 0.85,
		},
		{
			Start:              splitTime,
			End:                wSeg.End,
			Text:               part2,
			SpeakerLabel:       lateSpk,
			SpeakerName:        lateSpk,
			AcousticSpeaker:    lateSpk,
			AcousticConfidence: 0.85,
		},
	}
}

// SliceAcousticReferenceByTime extracts only the acoustic reference lines that fall
// within [startTime - bufferSeconds, endTime + bufferSeconds], eliminating temporal mismatch.
func SliceAcousticReferenceByTime(segments []dtos.SegmentResult, rawRef string, startTime, endTime, totalDuration float64, bufferSeconds float64) string {
	if bufferSeconds <= 0 {
		bufferSeconds = 12.0
	}

	winStart := startTime - bufferSeconds
	if winStart < 0 {
		winStart = 0
	}
	winEnd := endTime + bufferSeconds

	// 1. If structured diarized segments are available, format them directly
	if len(segments) > 0 {
		var sb strings.Builder
		for _, seg := range segments {
			if seg.End >= winStart && seg.Start <= winEnd {
				spk := seg.SpeakerLabel
				if spk == "" {
					spk = "Speaker 0"
				}
				sb.WriteString(fmt.Sprintf("%s [%.2fs - %.2fs]: %s\n", spk, seg.Start, seg.End, seg.Text))
			}
		}
		if res := strings.TrimSpace(sb.String()); res != "" {
			return res
		}
	}

	// 2. If rawRef string is provided, parse line timestamps
	cleanRef := strings.TrimSpace(rawRef)
	if cleanRef == "" {
		return ""
	}

	lines := strings.Split(cleanRef, "\n")
	var matchedLines []string
	hasTimestamps := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if matches := acousticLineRegex.FindStringSubmatch(trimmed); len(matches) == 5 {
			hasTimestamps = true
			sStart, err1 := strconv.ParseFloat(matches[2], 64)
			sEnd, err2 := strconv.ParseFloat(matches[3], 64)
			if err1 == nil && err2 == nil {
				if sEnd >= winStart && sStart <= winEnd {
					matchedLines = append(matchedLines, trimmed)
				}
			}
		}
	}

	if hasTimestamps {
		if len(matchedLines) > 0 {
			return strings.Join(matchedLines, "\n")
		}
		// If window found no segments, fallback to first/last proximity
		return ""
	}

	// 3. Fallback: If raw text lacks timestamps, proportional fallback with character limits
	if len(cleanRef) <= 6000 {
		return cleanRef
	}
	if totalDuration <= 0 {
		return cleanRef[:6000]
	}

	fracStart := (startTime / totalDuration) - 0.10
	if fracStart < 0 {
		fracStart = 0
	}
	fracEnd := (endTime / totalDuration) + 0.10
	if fracEnd > 1.0 {
		fracEnd = 1.0
	}

	refLen := len(cleanRef)
	bStart := int(fracStart * float64(refLen))
	bEnd := int(fracEnd * float64(refLen))
	if bStart > 0 && bStart < refLen {
		if idx := strings.Index(cleanRef[bStart:], "\n"); idx != -1 {
			bStart += idx + 1
		}
	}
	if bEnd < refLen && bEnd > bStart {
		if idx := strings.LastIndex(cleanRef[:bEnd], "\n"); idx != -1 && idx > bStart {
			bEnd = idx
		}
	}
	if bEnd <= bStart {
		bStart = 0
		bEnd = 6000
		if bEnd > refLen {
			bEnd = refLen
		}
	}
	return strings.TrimSpace(cleanRef[bStart:bEnd])
}

// DetectAndCorrectSpeakerInversion checks if the LLM inverted speaker labels across a chunk
// and restores acoustic ground truth to prevent user A/B identity flipping.
func DetectAndCorrectSpeakerInversion(original []dtos.SegmentResult, proposed []dtos.SegmentResult) []dtos.SegmentResult {
	if len(original) == 0 || len(proposed) == 0 {
		return proposed
	}

	// Mapping count: acousticSpeaker -> proposedSpeaker -> count
	mapping := make(map[string]map[string]int)
	acousticCounts := make(map[string]int)
	totalEvaluated := 0

	limit := len(original)
	if len(proposed) < limit {
		limit = len(proposed)
	}

	for i := 0; i < limit; i++ {
		acSpk := strings.TrimSpace(original[i].AcousticSpeaker)
		if acSpk == "" {
			acSpk = strings.TrimSpace(original[i].SpeakerLabel)
		}
		propSpk := strings.TrimSpace(proposed[i].SpeakerLabel)

		if acSpk == "" || propSpk == "" {
			continue
		}

		if _, exists := mapping[acSpk]; !exists {
			mapping[acSpk] = make(map[string]int)
		}
		mapping[acSpk][propSpk]++
		acousticCounts[acSpk]++
		totalEvaluated++
	}

	// Inversion detection between any pair of distinct speakers (e.g. Speaker 0 and Speaker 1)
	swaps := make(map[string]string)
	for spkA, totalA := range acousticCounts {
		for spkB, totalB := range acousticCounts {
			if spkA >= spkB || totalA < 3 || totalB < 3 {
				continue
			}
			// Check if A is mostly mapped to B, and B is mostly mapped to A
			mappedAtoB := mapping[spkA][spkB]
			mappedBtoA := mapping[spkB][spkA]

			ratioA := float64(mappedAtoB) / float64(totalA)
			ratioB := float64(mappedBtoA) / float64(totalB)

			if ratioA >= 0.75 && ratioB >= 0.75 {
				// Inversion detected!
				swaps[spkA] = spkB
				swaps[spkB] = spkA
			}
		}
	}

	if len(swaps) == 0 {
		return proposed
	}

	// Apply inversion correction
	corrected := make([]dtos.SegmentResult, len(proposed))
	copy(corrected, proposed)

	for i := range corrected {
		curr := strings.TrimSpace(corrected[i].SpeakerLabel)
		if target, ok := swaps[curr]; ok {
			corrected[i].SpeakerLabel = target
			// If name matches the inverted label, swap name as well
			if strings.EqualFold(corrected[i].SpeakerName, curr) {
				corrected[i].SpeakerName = target
			}
		}
	}
	return corrected
}

// FilterHallucinationsAndNoise strips segments that are pure music or non-speech markers,
// and removes consecutive duplicate hallucination loops.
func FilterHallucinationsAndNoise(segments []dtos.SegmentResult) []dtos.SegmentResult {
	if len(segments) == 0 {
		return segments
	}

	var filtered []dtos.SegmentResult
	for _, seg := range segments {
		if IsNonSpeechToken(seg.Text) {
			continue
		}

		cleanText := strings.TrimSpace(seg.Text)
		// Check consecutive stutter/hallucination loop (identical text within 0.5s)
		if len(filtered) > 0 {
			prev := filtered[len(filtered)-1]
			if strings.EqualFold(prev.Text, cleanText) && (seg.Start-prev.End < 0.5 || seg.Start <= prev.End) {
				continue
			}
		}

		filtered = append(filtered, seg)
	}

	for i := range filtered {
		filtered[i].ID = i
	}
	return filtered
}

// IsNonSpeechToken checks if the text represents music, applause, or non-speech noise.
func IsNonSpeechToken(text string) bool {
	clean := strings.ToLower(strings.Trim(text, "[]() \t\n\r"))
	if clean == "" {
		return true
	}
	if nonSpeechTokens[clean] {
		return true
	}
	// Check bracketed markers e.g. [musik piano], [applause], (tertawa)
	if (strings.HasPrefix(clean, "[") && strings.HasSuffix(clean, "]")) ||
		(strings.HasPrefix(clean, "(") && strings.HasSuffix(clean, ")")) {
		inner := strings.Trim(clean, "[]()")
		words := strings.Fields(inner)
		for _, w := range words {
			if nonSpeechTokens[w] {
				return true
			}
		}
	}
	return false
}
