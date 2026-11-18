package sttalign

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"code-base-golang/internal/dtos"
)

var (
	// Regex matching acoustic reference line: "Speaker X [0.00s - 10.00s]: text"
	acousticLineRegex = regexp.MustCompile(`(?i)^(?:\[)?(Speaker\s*\w+|Pembicara\s*\w+)(?:\])?\s*\[\s*([\d\.]+)\s*s?\s*-\s*([\d\.]+)\s*s?\]\s*:\s*(.*)$`)

	// Regex matching various speaker label formats: SPEAKER_00, speaker_0, Pembicara 1, 0, etc.
	speakerNumRegex = regexp.MustCompile(`(?i)^(?:\[)?(?:speaker|pembicara)?[\s\-_]*(\d+)(?:\])?$`)

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

// NormalizeSpeakerLabel standardizes diverse speaker formats (SPEAKER_00, speaker_0, Pembicara 1, 0)
// into canonical "Speaker N" format.
func NormalizeSpeakerLabel(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "Speaker 0"
	}
	if match := speakerNumRegex.FindStringSubmatch(trimmed); len(match) == 2 {
		num, err := strconv.Atoi(match[1])
		if err == nil {
			return fmt.Sprintf("Speaker %d", num)
		}
	}
	return trimmed
}

// CalculateOverlap returns the duration of temporal overlap between two time intervals.
func CalculateOverlap(s1, e1, s2, e2 float64) float64 {
	overlap := math.Min(e1, e2) - math.Max(s1, s2)
	if overlap < 0 {
		return 0.0
	}
	return overlap
}

// FindBestAcousticSpeaker finds the speaker with the maximum overlap across diarized acoustic segments.
// If direct overlap is zero, it uses a tolerance collar (1.5s) to bridge short VAD silence gaps.
func FindBestAcousticSpeaker(start, end float64, diarizedSegments []dtos.SegmentResult) (bestSpeaker string, maxOverlap float64, totalOverlap float64) {
	if len(diarizedSegments) == 0 || end <= start {
		return "", 0.0, 0.0
	}

	speakerDurations := make(map[string]float64)
	for _, d := range diarizedSegments {
		if strings.TrimSpace(d.SpeakerLabel) == "" {
			continue
		}
		spk := NormalizeSpeakerLabel(d.SpeakerLabel)
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

	if maxOverlap > 0 {
		return bestSpeaker, maxOverlap, totalOverlap
	}

	// Collar gap bridging: If zero direct overlap (e.g. Whisper detected speech during a 0.5s VAD acoustic pause),
	// find the closest acoustic segment within a 1.5s tolerance collar.
	const collar = 1.5
	minDist := collar + 1.0
	closestSpk := ""
	for _, d := range diarizedSegments {
		if strings.TrimSpace(d.SpeakerLabel) == "" {
			continue
		}
		spk := NormalizeSpeakerLabel(d.SpeakerLabel)
		var dist float64
		if d.End < start {
			dist = start - d.End
		} else if d.Start > end {
			dist = d.Start - end
		}
		if dist <= collar && dist < minDist {
			minDist = dist
			closestSpk = spk
		}
	}
	if closestSpk != "" {
		return closestSpk, 0.0, 0.0
	}
	return "", 0.0, 0.0
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

		// Case 0: Compound dialogue turn splitting at sentence punctuation boundaries
		splitCompoundSegs := SplitCompoundDialogue(wSeg, diarizedSegments)
		if len(splitCompoundSegs) > 1 {
			for i := range splitCompoundSegs {
				if splitCompoundSegs[i].SpeakerLabel == "" {
					splitCompoundSegs[i].SpeakerLabel = wSeg.SpeakerLabel
				}
				aligned = append(aligned, splitCompoundSegs[i])
			}
			continue
		}

		// Case 1: Word-level acoustic alignment and speaker turn splitting
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
		if dur > 0 && maxOv > 0 {
			confidence = maxOv / dur
			if confidence > 1.0 {
				confidence = 1.0
			}
		} else if bestSpeaker != "" {
			confidence = 0.75
		}

		// Check if segment should be split on clause boundary (when 2 speakers have significant overlap)
		splitClauseSegs := trySplitByClause(wSeg, diarizedSegments)
		if len(splitClauseSegs) > 1 {
			aligned = append(aligned, splitClauseSegs...)
			continue
		}

		wSegCopy := wSeg
		if bestSpeaker != "" {
			normSpk := NormalizeSpeakerLabel(bestSpeaker)
			wSegCopy.SpeakerLabel = normSpk
			wSegCopy.AcousticSpeaker = normSpk
			wSegCopy.AcousticConfidence = confidence
			if wSegCopy.SpeakerName == "" {
				wSegCopy.SpeakerName = normSpk
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
		if spk == "" && strings.TrimSpace(w.Speaker) != "" {
			spk = NormalizeSpeakerLabel(w.Speaker)
		}
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
			normSpk := NormalizeSpeakerLabel(runs[0][0].speaker)
			resSeg.SpeakerLabel = normSpk
			resSeg.AcousticSpeaker = normSpk

			wordDuration := resSeg.End - resSeg.Start
			if wordDuration > 0 {
				overlapStart := math.Max(resSeg.Start, runs[0][0].word.Start)
				overlapEnd := math.Min(resSeg.End, runs[0][len(runs[0])-1].word.End)
				overlap := math.Max(0.0, overlapEnd-overlapStart)
				resSeg.AcousticConfidence = math.Min(1.0, overlap/wordDuration)
			} else {
				resSeg.AcousticConfidence = 0.0
			}

			if resSeg.SpeakerName == "" {
				resSeg.SpeakerName = normSpk
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
		spk = NormalizeSpeakerLabel(spk)

		start := r[0].word.Start
		end := r[len(r)-1].word.End
		if end <= start {
			end = start + 0.1
		}

		confidence := 1.0
		dur := end - start
		if dur > 0 {
			_, maxOv, _ := FindBestAcousticSpeaker(start, end, diarizedSegments)
			if maxOv > 0 {
				confidence = math.Min(1.0, maxOv/dur)
			} else {
				confidence = 0.85
			}
		}

		result = append(result, dtos.SegmentResult{
			Start:              start,
			End:                end,
			Text:               text,
			SpeakerLabel:       spk,
			SpeakerName:        spk,
			AcousticSpeaker:    spk,
			AcousticConfidence: confidence,
			Words:              wordsList,
		})
	}
	return result
}

// trySplitByClause splits a Whisper segment without word timestamps at punctuation boundaries,
// discourse markers, or word boundaries if two distinct speakers each have significant overlap.
func trySplitByClause(wSeg dtos.SegmentResult, diarizedSegments []dtos.SegmentResult) []dtos.SegmentResult {
	dur := wSeg.End - wSeg.Start
	if dur < 1.2 {
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

	// Check if top 2 speakers both have >= 20% of duration
	if list[0].overlap < 0.20*dur || list[1].overlap < 0.20*dur {
		return nil
	}

	// Find early speaker vs late speaker based on acoustic segments
	earlySpk, earlyOv, _ := FindBestAcousticSpeaker(wSeg.Start, wSeg.Start+dur*0.4, diarizedSegments)
	lateSpk, lateOv, _ := FindBestAcousticSpeaker(wSeg.End-dur*0.4, wSeg.End, diarizedSegments)
	if earlySpk == "" || lateSpk == "" || earlyOv <= 0 || lateOv <= 0 {
		return nil
	}
	earlySpk = NormalizeSpeakerLabel(earlySpk)
	lateSpk = NormalizeSpeakerLabel(lateSpk)
	if earlySpk == lateSpk {
		return nil
	}

	// Estimate transition time between earlySpk and lateSpk
	transitionTime := wSeg.Start + (dur * 0.5)
	for i, d := range diarizedSegments {
		norm := NormalizeSpeakerLabel(d.SpeakerLabel)
		if norm != earlySpk || d.End <= wSeg.Start || d.End >= wSeg.End {
			continue
		}

		for j := i + 1; j < len(diarizedSegments); j++ {
			next := diarizedSegments[j]
			nextSpk := NormalizeSpeakerLabel(next.SpeakerLabel)
			if nextSpk == earlySpk {
				continue
			}
			if nextSpk == lateSpk && next.Start <= wSeg.End && next.Start >= wSeg.Start {
				transitionTime = d.End
			}
			break
		}
	}

	targetRatio := (transitionTime - wSeg.Start) / dur
	if targetRatio < 0.15 {
		targetRatio = 0.15
	} else if targetRatio > 0.85 {
		targetRatio = 0.85
	}

	text := strings.TrimSpace(wSeg.Text)
	splitIdx := findOptimalTextSplitPoint(text, targetRatio)
	if splitIdx <= 0 {
		return nil
	}

	part1 := strings.TrimSpace(text[:splitIdx])
	part2 := strings.TrimSpace(text[splitIdx:])
	if part1 == "" || part2 == "" {
		return nil
	}

	splitRatio := float64(len([]rune(part1))) / float64(len([]rune(text)))
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

// findOptimalTextSplitPoint identifies the best character index to split text
// based on punctuation, discourse markers, or word boundaries closest to targetRatio.
func findOptimalTextSplitPoint(text string, targetRatio float64) int {
	textLen := len(text)
	if textLen < 5 {
		return -1
	}

	targetIdx := int(targetRatio * float64(textLen))

	// Priority 1: Sentence boundaries (split after punctuation)
	if idx := findClosestPatternMatch(text, []string{"? ", "! ", ". ", "?\n", "!\n", ".\n", "？", "。"}, targetIdx, 0.15, 0.85, false); idx > 0 {
		return idx
	}

	// Priority 2: Clause boundaries (split after comma, semicolon, colon)
	if idx := findClosestPatternMatch(text, []string{", ", "; ", ": "}, targetIdx, 0.20, 0.80, false); idx > 0 {
		return idx
	}

	// Priority 3: Word boundary space closest to targetIdx
	bestSpace := -1
	minDist := textLen
	minBound := int(0.15 * float64(textLen))
	maxBound := int(0.85 * float64(textLen))

	for i := minBound; i <= maxBound; i++ {
		if text[i] == ' ' {
			dist := int(math.Abs(float64(i - targetIdx)))
			if dist < minDist {
				minDist = dist
				bestSpace = i + 1
			}
		}
	}
	return bestSpace
}

// findClosestPatternMatch finds the occurrence of any pattern closest to targetIdx within bounds.
// If splitAtPrefix is true, it splits right before the pattern (used for turn-initial discourse markers).
// If splitAtPrefix is false, it splits right after the pattern (used for sentence terminators).
func findClosestPatternMatch(text string, patterns []string, targetIdx int, minRatio, maxRatio float64, splitAtPrefix bool) int {
	textLen := len(text)
	minIdx := int(minRatio * float64(textLen))
	maxIdx := int(maxRatio * float64(textLen))

	bestIdx := -1
	minDist := float64(textLen)

	for _, p := range patterns {
		pLen := len(p)
		if pLen == 0 || pLen > textLen {
			continue
		}
		for i := 0; i+pLen <= textLen; {
			if strings.EqualFold(text[i:i+pLen], p) {
				matchIdx := i + pLen
				if splitAtPrefix {
					matchIdx = i
				}
				if matchIdx >= minIdx && matchIdx <= maxIdx {
					dist := math.Abs(float64(matchIdx - targetIdx))
					if dist < minDist {
						minDist = dist
						bestIdx = matchIdx
					}
				}
			}
			_, size := utf8.DecodeRuneInString(text[i:])
			if size <= 0 {
				break
			}
			i += size
		}
	}
	return bestIdx
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

	for pIdx, p := range proposed {
		propSpk := strings.TrimSpace(p.SpeakerLabel)
		if propSpk == "" {
			continue
		}

		var matchedOrig *dtos.SegmentResult
		maxOv := 0.0
		for idx := range original {
			if original[idx].ID == p.ID && p.ID > 0 {
				matchedOrig = &original[idx]
				break
			}
			ov := CalculateOverlap(p.Start, p.End, original[idx].Start, original[idx].End)
			if ov > maxOv {
				maxOv = ov
				matchedOrig = &original[idx]
			}
		}

		// Fallback to sequential index if no temporal overlap or unassigned timestamps/IDs
		if matchedOrig == nil && pIdx < len(original) {
			matchedOrig = &original[pIdx]
		}

		if matchedOrig == nil {
			continue
		}

		acSpk := strings.TrimSpace(matchedOrig.AcousticSpeaker)
		if acSpk == "" {
			acSpk = strings.TrimSpace(matchedOrig.SpeakerLabel)
		}
		if acSpk == "" {
			continue
		}

		acSpk = NormalizeSpeakerLabel(acSpk)
		propSpk = NormalizeSpeakerLabel(propSpk)

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
	clean := strings.ToLower(strings.TrimSpace(text))
	if clean == "" {
		return true
	}
	bare := strings.Trim(clean, "[]() \t\n\r")
	if bare == "" || nonSpeechTokens[bare] {
		return true
	}
	// Check bracketed markers e.g. [musik piano], [applause], (tertawa), [tepuk tangan penonton]
	if (strings.HasPrefix(clean, "[") && strings.HasSuffix(clean, "]")) ||
		(strings.HasPrefix(clean, "(") && strings.HasSuffix(clean, ")")) {
		inner := strings.Trim(clean[1:len(clean)-1], "[]() \t\n\r")
		for token := range nonSpeechTokens {
			if strings.Contains(inner, token) {
				return true
			}
		}
	}
	return false
}

// IsAcousticallyAnchored checks if a segment has high biometric acoustic grounding
// and determines whether an LLM speaker change should be rejected to prevent hallucinated speaker swaps.
func IsAcousticallyAnchored(seg dtos.SegmentResult, proposedSpeaker string) bool {
	acSpk := strings.TrimSpace(seg.AcousticSpeaker)
	if acSpk == "" {
		acSpk = strings.TrimSpace(seg.SpeakerLabel)
	}
	if acSpk == "" || strings.TrimSpace(proposedSpeaker) == "" {
		return false
	}
	acSpk = NormalizeSpeakerLabel(acSpk)
	propSpk := NormalizeSpeakerLabel(proposedSpeaker)

	if strings.EqualFold(acSpk, propSpk) {
		return false
	}

	// High confidence acoustic threshold (>= 70% duration overlap)
	return seg.AcousticConfidence >= 0.70
}

// ResolveSpeakerLabelAndName maps raw speaker inputs (e.g. "Putri", "Speaker 0", "speaker_1", "0")
// into canonical speaker labels ("Speaker N") and human-readable names using the discovered speaker roster.
func ResolveSpeakerLabelAndName(rawSpeaker, rawName string, rosterMap map[string]dtos.SpeakerRosterItem) (string, string) {
	trimmedSpk := strings.TrimSpace(rawSpeaker)
	trimmedName := strings.TrimSpace(rawName)

	// 1. Check if rawSpeaker matches an established person's name in roster
	for lbl, item := range rosterMap {
		if strings.EqualFold(trimmedSpk, item.Name) && !isGenericSpeaker(item.Name) {
			normLbl := NormalizeSpeakerLabel(lbl)
			return normLbl, item.Name
		}
	}

	// 2. Normalize rawSpeaker label format
	normLbl := NormalizeSpeakerLabel(trimmedSpk)

	// 3. Determine speaker name
	outName := trimmedName
	if outName == "" || isGenericSpeaker(outName) {
		if item, ok := rosterMap[normLbl]; ok && !isGenericSpeaker(item.Name) {
			outName = item.Name
		} else {
			outName = normLbl
		}
	}

	return normLbl, outName
}

func isGenericSpeaker(name string) bool {
	trimmed := strings.ToLower(strings.TrimSpace(name))
	if trimmed == "" {
		return true
	}
	return strings.HasPrefix(trimmed, "speaker") || strings.HasPrefix(trimmed, "pembicara") || trimmed == "unknown" || trimmed == "peserta"
}

// BuildSegmentsFromWords groups word-level results into cohesive transcript segments
// demarcated by speaker changes, natural speech pauses, and sentence boundaries.
func BuildSegmentsFromWords(words []dtos.WordResult, maxPauseSeconds float64, maxDurationSeconds float64) []dtos.SegmentResult {
	if len(words) == 0 {
		return nil
	}
	if maxPauseSeconds <= 0 {
		maxPauseSeconds = 1.2
	}
	if maxDurationSeconds <= 0 {
		maxDurationSeconds = 15.0
	}

	var segments []dtos.SegmentResult
	var currentWords []dtos.WordResult
	currentSpeaker := NormalizeSpeakerLabel(words[0].Speaker)

	flushCurrent := func() {
		if len(currentWords) == 0 {
			return
		}
		var textParts []string
		for _, w := range currentWords {
			textParts = append(textParts, w.Word)
		}
		text := strings.TrimSpace(strings.Join(textParts, " "))
		if text == "" {
			currentWords = nil
			return
		}

		start := currentWords[0].Start
		end := currentWords[len(currentWords)-1].End
		if end <= start {
			end = start + 0.1
		}

		spk := currentSpeaker
		if spk == "" {
			spk = "Speaker 0"
		}

		wordsCopy := make([]dtos.WordResult, len(currentWords))
		copy(wordsCopy, currentWords)

		segments = append(segments, dtos.SegmentResult{
			ID:                 len(segments),
			Start:              start,
			End:                end,
			Text:               text,
			SpeakerLabel:       spk,
			SpeakerName:        spk,
			AcousticSpeaker:    spk,
			AcousticConfidence: 1.0,
			Words:              wordsCopy,
		})
		currentWords = nil
	}

	for _, w := range words {
		var wSpk string
		if strings.TrimSpace(w.Speaker) != "" {
			wSpk = NormalizeSpeakerLabel(w.Speaker)
		}

		if len(currentWords) == 0 {
			currentWords = append(currentWords, w)
			if wSpk != "" {
				currentSpeaker = wSpk
			}
			continue
		}

		prev := currentWords[len(currentWords)-1]
		speakerChanged := wSpk != "" && currentSpeaker != "" && wSpk != currentSpeaker
		pauseExceeded := (w.Start - prev.End) > maxPauseSeconds
		durationExceeded := (w.End - currentWords[0].Start) > maxDurationSeconds

		isSentenceEnd := strings.HasSuffix(prev.Word, ".") || strings.HasSuffix(prev.Word, "?") || strings.HasSuffix(prev.Word, "!")
		shouldSplitSentence := isSentenceEnd && (w.End-currentWords[0].Start) > 4.0

		if speakerChanged || pauseExceeded || durationExceeded || shouldSplitSentence {
			flushCurrent()
			currentWords = append(currentWords, w)
			if wSpk != "" {
				currentSpeaker = wSpk
			}
		} else {
			currentWords = append(currentWords, w)
		}
	}
	flushCurrent()

	return segments
}

// IsAnswerPrefix is deprecated and retained only for backward compatibility.
func IsAnswerPrefix(text string) bool {
	return false
}

// IsCandidateIdentityMarker is deprecated and retained only for backward compatibility.
// Conversational context and role resolution is handled dynamically by LLM prompts.
func IsCandidateIdentityMarker(text string) bool {
	return false
}

// IsInterviewerPromptMarker is deprecated and retained only for backward compatibility.
// Conversational context and role resolution is handled dynamically by LLM prompts.
func IsInterviewerPromptMarker(text string) bool {
	return false
}

// DetectCompoundDialogueSplitPoints returns character indices where sentence punctuation boundaries occur.
// It is strictly language-agnostic and relies on standard punctuation marks followed by whitespace.
func DetectCompoundDialogueSplitPoints(text string) []int {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 10 {
		return nil
	}

	var rawPoints []int
	for i := 0; i < len(text); i++ {
		isTerminator := false
		termLen := 1
		if text[i] == '?' || text[i] == '!' || text[i] == '.' {
			isTerminator = true
		} else if strings.HasPrefix(text[i:], "？") {
			isTerminator = true
			termLen = len("？")
		} else if strings.HasPrefix(text[i:], "。") {
			isTerminator = true
			termLen = len("。")
		}

		if isTerminator {
			nextIdx := i + termLen
			for nextIdx < len(text) && (text[nextIdx] == ' ' || text[nextIdx] == '\t' || text[nextIdx] == '\n') {
				nextIdx++
			}
			if nextIdx < len(text) && nextIdx >= 4 && len(text)-nextIdx >= 4 {
				rawPoints = append(rawPoints, nextIdx)
			}
		}
	}

	if len(rawPoints) == 0 {
		return nil
	}

	sort.Ints(rawPoints)
	var points []int
	for _, p := range rawPoints {
		if len(points) == 0 {
			points = append(points, p)
		} else if p-points[len(points)-1] >= 4 {
			points = append(points, p)
		}
	}
	return points
}

func sliceTextByIndices(text string, indices []int) []string {
	if len(indices) == 0 {
		return []string{strings.TrimSpace(text)}
	}
	var parts []string
	prev := 0
	for _, idx := range indices {
		if idx > prev && idx <= len(text) {
			part := strings.TrimSpace(text[prev:idx])
			if part != "" {
				parts = append(parts, part)
			}
			prev = idx
		}
	}
	if prev < len(text) {
		part := strings.TrimSpace(text[prev:])
		if part != "" {
			parts = append(parts, part)
		}
	}
	return parts
}

func splitWordsByTextParts(words []dtos.WordResult, parts []string) [][]dtos.WordResult {
	if len(words) == 0 || len(parts) == 0 {
		return nil
	}
	result := make([][]dtos.WordResult, len(parts))
	wIdx := 0
	for pIdx, part := range parts {
		partWords := strings.Fields(part)
		for range partWords {
			if wIdx < len(words) {
				result[pIdx] = append(result[pIdx], words[wIdx])
				wIdx++
			}
		}
	}
	if len(parts) > 0 && wIdx < len(words) {
		result[len(parts)-1] = append(result[len(parts)-1], words[wIdx:]...)
	}
	return result
}

func assignDialogueTurnSpeakers(parts []string, originalSpeaker string, diarizedSegments []dtos.SegmentResult, startTimes, endTimes []float64) []string {
	normOrig := NormalizeSpeakerLabel(originalSpeaker)
	if normOrig == "" {
		normOrig = "Speaker 0"
	}

	speakers := make([]string, len(parts))
	for i := range parts {
		if len(startTimes) > i && len(endTimes) > i {
			acSpk, _, _ := FindBestAcousticSpeaker(startTimes[i], endTimes[i], diarizedSegments)
			if acSpk != "" {
				speakers[i] = NormalizeSpeakerLabel(acSpk)
				continue
			}
		}
		if i > 0 && speakers[i-1] != "" {
			speakers[i] = speakers[i-1]
		} else {
			speakers[i] = normOrig
		}
	}
	return speakers
}

// SplitCompoundDialogue splits a compound Whisper segment containing conversational turns
// into multiple sub-segments with interpolated timestamps and attributed speakers.
func SplitCompoundDialogue(wSeg dtos.SegmentResult, diarizedSegments []dtos.SegmentResult) []dtos.SegmentResult {
	dur := wSeg.End - wSeg.Start
	if dur < 1.0 || len(wSeg.Text) < 10 {
		return nil
	}

	// 1. If word-level timestamps are present, word alignment handles turn splitting directly
	if len(wSeg.Words) > 0 {
		wordSplits := alignAndSplitByWords(wSeg, diarizedSegments)
		if len(wordSplits) > 1 {
			return wordSplits
		}
	}

	// 2. If diarized segments indicate speaker transition, try clause splitting
	if len(diarizedSegments) > 0 {
		clauseSplits := trySplitByClause(wSeg, diarizedSegments)
		if len(clauseSplits) > 1 {
			return clauseSplits
		}
	}

	// 3. Punctuation boundary splitting when multiple acoustic speakers overlap
	splitPoints := DetectCompoundDialogueSplitPoints(wSeg.Text)
	if len(splitPoints) == 0 {
		return nil
	}

	parts := sliceTextByIndices(wSeg.Text, splitPoints)
	if len(parts) <= 1 {
		return nil
	}

	startTimes := make([]float64, len(parts))
	endTimes := make([]float64, len(parts))

	var partWords [][]dtos.WordResult
	if len(wSeg.Words) > 0 {
		partWords = splitWordsByTextParts(wSeg.Words, parts)
	}

	var totalRunes int
	for _, p := range parts {
		totalRunes += len([]rune(p))
	}
	if totalRunes <= 0 {
		return nil
	}

	currStart := wSeg.Start
	for i, p := range parts {
		if len(partWords) > i && len(partWords[i]) > 0 {
			wList := partWords[i]
			startTimes[i] = wList[0].Start
			endTimes[i] = wList[len(wList)-1].End
			if endTimes[i] <= startTimes[i] {
				endTimes[i] = startTimes[i] + 0.1
			}
			currStart = endTimes[i]
		} else {
			pRunes := len([]rune(p))
			pDur := (float64(pRunes) / float64(totalRunes)) * dur
			startTimes[i] = currStart
			endTimes[i] = currStart + pDur
			currStart = endTimes[i]
		}
	}
	if len(endTimes) > 0 && len(wSeg.Words) == 0 {
		endTimes[len(endTimes)-1] = wSeg.End
	}

	relevantDiarizedSegments := make([]dtos.SegmentResult, 0, len(diarizedSegments))
	for _, seg := range diarizedSegments {
		if seg.End > wSeg.Start && seg.Start < wSeg.End {
			relevantDiarizedSegments = append(relevantDiarizedSegments, seg)
		}
	}
	assignedSpeakers := assignDialogueTurnSpeakers(parts, wSeg.SpeakerLabel, relevantDiarizedSegments, startTimes, endTimes)

	results := make([]dtos.SegmentResult, len(parts))
	for i, p := range parts {
		spk := assignedSpeakers[i]
		var wordsList []dtos.WordResult
		if len(partWords) > i {
			wordsList = partWords[i]
		}

		bestAcSpk, maxOv, _ := FindBestAcousticSpeaker(startTimes[i], endTimes[i], diarizedSegments)
		realAcSpk := ""
		acConf := 0.50
		if bestAcSpk != "" {
			realAcSpk = NormalizeSpeakerLabel(bestAcSpk)
			dur := endTimes[i] - startTimes[i]
			if dur > 0 && maxOv > 0 {
				acConf = maxOv / dur
				if acConf > 1.0 {
					acConf = 1.0
				}
			} else {
				acConf = 0.70
			}
		}

		results[i] = dtos.SegmentResult{
			Start:              startTimes[i],
			End:                endTimes[i],
			Text:               p,
			SpeakerLabel:       spk,
			SpeakerName:        spk,
			AcousticSpeaker:    realAcSpk,
			AcousticConfidence: acConf,
			Words:              wordsList,
		}
	}

	return results
}

// EnforceDialogueRoleConsistency ensures that acoustic speaker evidence is strictly respected
// and empty speaker labels are populated from acoustic diarization without hardcoded text guessing.
func EnforceDialogueRoleConsistency(segments []dtos.SegmentResult) []dtos.SegmentResult {
	if len(segments) == 0 {
		return segments
	}

	corrected := make([]dtos.SegmentResult, len(segments))
	copy(corrected, segments)

	for i := range corrected {
		if corrected[i].AcousticConfidence >= 0.70 && corrected[i].AcousticSpeaker != "" {
			normAc := NormalizeSpeakerLabel(corrected[i].AcousticSpeaker)
			corrected[i].SpeakerLabel = normAc
			if corrected[i].SpeakerName == "" || isGenericSpeaker(corrected[i].SpeakerName) {
				corrected[i].SpeakerName = normAc
			}
		} else if corrected[i].SpeakerLabel == "" && corrected[i].AcousticSpeaker != "" {
			normAc := NormalizeSpeakerLabel(corrected[i].AcousticSpeaker)
			corrected[i].SpeakerLabel = normAc
			if corrected[i].SpeakerName == "" || isGenericSpeaker(corrected[i].SpeakerName) {
				corrected[i].SpeakerName = normAc
			}
		}
	}

	return corrected
}
