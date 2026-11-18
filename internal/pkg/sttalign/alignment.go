package sttalign

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
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

	// Conversational answer prefixes after a question in dialogue
	dialogueAnswerPrefixes = []string{
		"alhamdulillah", "terima kasih", "terimakasih", "ya", "iya", "tentu",
		"betul", "bisa", "siap", "tidak", "nggak", "enggak", "bukan",
		"bebas", "boleh", "oke", "baik", "nah", "saya", "aku", "kami", "kita",
		"rumah saya", "pengalaman saya", "kelebihan saya", "kekurangan saya",
		"nama saya", "akun sendiri", "di situ", "perusahaan ini", "itu bernama",
	}

	// Interviewer question and prompt turn starters
	dialogueQuestionStarters = []string{
		"lalu,", "lalu ", "kemudian,", "kemudian ", "selanjutnya,", "selanjutnya ",
		"sebelumnya,", "sebelumnya ", "gimana ", "bagaimana ", "kalau boleh tahu",
		"apakah anda ", "apakah kamu ", "bisakah anda ", "bisakah kamu ",
		"mengapa anda ", "mengapa kamu ", "kenapa anda ", "kenapa kamu ",
		"apa yang anda ", "apa yang kamu ", "menurut anda ", "menurut kamu ",
		"sudah makan belum", "gimana tadi perjalanannya", "paling gini sih",
	}

	// Conversational greeting turn starters
	greetingStarters = []string{
		"selamat siang", "selamat pagi", "selamat sore", "selamat malam",
		"halo", "hai", "assalamu'alaikum", "assalamualaikum",
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

		// Case 0: Compound dialogue turn splitting (Q&A pairs, greeting exchanges, conversation turn transitions)
		splitCompoundSegs := SplitCompoundDialogue(wSeg, diarizedSegments)
		if len(splitCompoundSegs) > 1 {
			// Preserve the original Whisper speaker label for sub-segments
			// that could not obtain a speaker from acoustic alignment.
			for i := range splitCompoundSegs {
				if splitCompoundSegs[i].SpeakerLabel == "" {
					splitCompoundSegs[i].SpeakerLabel = wSeg.SpeakerLabel
				}
				// If word timestamps are available and acoustic diarization exists, refine sub-segment with word alignment
				if len(splitCompoundSegs[i].Words) > 0 && len(diarizedSegments) > 0 {
					wordRefined := alignAndSplitByWords(splitCompoundSegs[i], diarizedSegments)
					if len(wordRefined) > 0 {
						aligned = append(aligned, wordRefined...)
						continue
					}
				}
				aligned = append(aligned, splitCompoundSegs[i])
			}
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
	if idx := findClosestPatternMatch(text, []string{"? ", "! ", ". ", "?\n", "!\n", ".\n"}, targetIdx, 0.15, 0.85, false); idx > 0 {
		return idx
	}

	// Priority 2: Conversational discourse turn markers (split before turn marker so it starts the new turn)
	turnMarkers := []string{
		" terima kasih ", " terimakasih ", " silakan ", " silahkan ",
		" ya ", " iya ", " oke ", " baik ", " halo ", " selamat ",
		" tapi ", " nah ", " betul ", " tentu ", " yes ", " thank you ",
	}
	if idx := findClosestPatternMatch(text, turnMarkers, targetIdx, 0.18, 0.82, true); idx > 0 {
		return idx
	}

	// Priority 3: Clause commas (split after comma)
	if idx := findClosestPatternMatch(text, []string{", ", "; ", ": "}, targetIdx, 0.20, 0.80, false); idx > 0 {
		return idx
	}

	// Priority 4: Word boundary space closest to targetIdx
	bestSpace := -1
	minDist := textLen
	minBound := int(0.20 * float64(textLen))
	maxBound := int(0.80 * float64(textLen))

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
	if seg.AcousticConfidence < 0.70 {
		return false
	}

	// Exception: explicit self-introduction or identity declaration
	lowerText := strings.ToLower(seg.Text)
	if strings.Contains(lowerText, "nama saya ") || strings.Contains(lowerText, "saya adalah ") || strings.Contains(lowerText, "perkenalkan nama saya") {
		return false
	}

	return true
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

func isAnswerPrefix(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	for _, p := range dialogueAnswerPrefixes {
		if lower == p || strings.HasPrefix(lower, p+" ") || strings.HasPrefix(lower, p+",") || strings.HasPrefix(lower, p+".") || strings.HasPrefix(lower, p+"!") {
			return true
		}
	}
	return false
}

func isQuestionStarter(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	for _, p := range dialogueQuestionStarters {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	return false
}

func isGreeting(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	for _, g := range greetingStarters {
		if strings.HasPrefix(lower, g) {
			if len(lower) == len(g) {
				return true
			}
			nextChar, _ := utf8.DecodeRuneInString(lower[len(g):])
			if unicode.IsSpace(nextChar) || nextChar == ',' || nextChar == '.' || nextChar == '!' || nextChar == '?' {
				return true
			}
		}
	}
	return false
}

func isSelfIntroOrRoleDeclaration(lower string) bool {
	if strings.Contains(lower, "?") {
		return false
	}
	trimmed := strings.TrimSpace(lower)
	markers := []string{
		"nama saya ", "saya adalah ", "perkenalkan saya ", "perkenalkan, saya ",
		"perkenalkan nama saya ", "perkenalkan, nama saya ",
		"nama ku ", "namaku ", "panggil saja saya ", "panggil saya ",
		"saya bernama ", "saya melamar ",
	}
	for _, m := range markers {
		if strings.HasPrefix(trimmed, m) || strings.Contains(trimmed, ". "+m) || strings.Contains(trimmed, ", "+m) {
			return true
		}
	}
	return false
}

// DetectCompoundDialogueSplitPoints returns character indices where dialogue turn transitions occur.
func DetectCompoundDialogueSplitPoints(text string) []int {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) < 15 {
		return nil
	}

	var rawPoints []int

	// 1. Scan for question marks followed by answer / response
	for i := 0; i < len(text); i++ {
		if text[i] == '?' {
			nextIdx := i + 1
			for nextIdx < len(text) && (text[nextIdx] == ' ' || text[nextIdx] == '\t' || text[nextIdx] == '\n') {
				nextIdx++
			}
			if nextIdx < len(text) {
				after := text[nextIdx:]
				if isAnswerPrefix(after) || isGreeting(after) || isQuestionStarter(after) {
					if i >= 6 && len(text)-nextIdx >= 6 {
						rawPoints = append(rawPoints, nextIdx)
					}
				}
			}
		}
	}

	// 2. Scan for mid-sentence question starters or greetings after sentence terminator (". ", "! ")
	for i := 0; i < len(text)-2; i++ {
		if (text[i] == '.' || text[i] == '!') && (text[i+1] == ' ' || text[i+1] == '\n') {
			nextIdx := i + 2
			for nextIdx < len(text) && (text[nextIdx] == ' ' || text[nextIdx] == '\t' || text[nextIdx] == '\n') {
				nextIdx++
			}
			if nextIdx < len(text) {
				after := text[nextIdx:]
				if isQuestionStarter(after) {
					if qOffset := strings.Index(after, "?"); qOffset > 0 {
						if nextIdx >= 10 && len(text)-nextIdx >= 10 {
							rawPoints = append(rawPoints, nextIdx)
						}
					}
				} else if isGreeting(after) {
					if nextIdx >= 10 && len(text)-nextIdx >= 10 {
						rawPoints = append(rawPoints, nextIdx)
					}
				} else if strings.Index(after, "?") > 0 && len(rawPoints) > 0 {
					// If a previous split point marked an answer turn, and this sentence asks a new question
					lastPoint := rawPoints[len(rawPoints)-1]
					if nextIdx > lastPoint {
						middleClause := strings.TrimSpace(text[lastPoint:i])
						if isAnswerPrefix(middleClause) && nextIdx >= 10 && len(text)-nextIdx >= 10 {
							rawPoints = append(rawPoints, nextIdx)
						}
					}
				}
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
			if p >= 8 && p <= len(text)-8 {
				points = append(points, p)
			}
		} else {
			last := points[len(points)-1]
			if p-last >= 8 && p <= len(text)-8 {
				points = append(points, p)
			}
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
	for wIdx < len(words) {
		result[len(parts)-1] = append(result[len(parts)-1], words[wIdx])
		wIdx++
	}
	return result
}

func assignDialogueTurnSpeakers(parts []string, originalSpeaker string, diarizedSegments []dtos.SegmentResult, startTimes, endTimes []float64) []string {
	normOrig := NormalizeSpeakerLabel(originalSpeaker)
	if normOrig == "" {
		normOrig = "Speaker 0"
	}

	alternate := "Speaker 1"
	if normOrig == "Speaker 1" {
		alternate = "Speaker 0"
	}

	speakers := make([]string, len(parts))
	allSameAcoustic := true
	var firstAcoustic string
	hasAcoustic := false

	for i := range parts {
		acSpk, maxOv, _ := FindBestAcousticSpeaker(startTimes[i], endTimes[i], diarizedSegments)
		if maxOv > 0 && acSpk != "" {
			normAc := NormalizeSpeakerLabel(acSpk)
			speakers[i] = normAc
			hasAcoustic = true
			if firstAcoustic == "" {
				firstAcoustic = normAc
			} else if normAc != firstAcoustic {
				allSameAcoustic = false
			}
		}
	}

	if hasAcoustic && !allSameAcoustic && firstAcoustic != "" {
		for i := range speakers {
			if speakers[i] == "" {
				speakers[i] = normOrig
			}
		}
		return speakers
	}

	if !hasAcoustic {
		allSameAcoustic = false
	}

	currentSpeaker := normOrig
	for i, p := range parts {
		isQ := isQuestionStarter(p) || (strings.HasSuffix(strings.TrimSpace(p), "?") && !isAnswerPrefix(p))
		isAns := isAnswerPrefix(p)

		if i == 0 {
			if isAns {
				currentSpeaker = alternate
			} else {
				currentSpeaker = normOrig
			}
		} else {
			prev := parts[i-1]
			prevIsQ := strings.HasSuffix(strings.TrimSpace(prev), "?")
			if prevIsQ && isAns {
				if speakers[i-1] == normOrig {
					currentSpeaker = alternate
				} else {
					currentSpeaker = normOrig
				}
			} else if isQ {
				if speakers[i-1] == normOrig {
					currentSpeaker = alternate
				} else {
					currentSpeaker = normOrig
				}
			}
		}
		if !allSameAcoustic || speakers[i] == "" {
			speakers[i] = currentSpeaker
		}
	}

	for i := range speakers {
		if speakers[i] == "" {
			speakers[i] = normOrig
		}
	}

	return speakers
}

// SplitCompoundDialogue splits a compound Whisper segment containing conversational turns
// into multiple sub-segments with interpolated timestamps and attributed speakers.
func SplitCompoundDialogue(wSeg dtos.SegmentResult, diarizedSegments []dtos.SegmentResult) []dtos.SegmentResult {
	dur := wSeg.End - wSeg.Start
	if dur < 1.0 || len(wSeg.Text) < 15 {
		return nil
	}

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

	assignedSpeakers := assignDialogueTurnSpeakers(parts, wSeg.SpeakerLabel, diarizedSegments, startTimes, endTimes)

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
		if maxOv > 0 && bestAcSpk != "" {
			realAcSpk = NormalizeSpeakerLabel(bestAcSpk)
			dur := endTimes[i] - startTimes[i]
			if dur > 0 {
				acConf = maxOv / dur
				if acConf > 1.0 {
					acConf = 1.0
				}
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

func isCandidateIdentityMarker(lower string) bool {
	if strings.Contains(lower, "?") {
		return false
	}
	trimmed := strings.TrimSpace(lower)
	markers := []string{
		"perkenalkan nama saya ", "perkenalkan, nama saya ", "nama saya ",
		"perkenalkan saya ", "saya adalah ", "saya berasal dari ",
		"rumah saya di ", "saya melamar ",
	}
	for _, m := range markers {
		if strings.HasPrefix(trimmed, m) {
			return true
		}
	}
	return false
}

func isCandidateResponseContent(lower string) bool {
	if strings.Contains(lower, "?") {
		return false
	}
	prefixes := []string{
		"alhamdulillah lancar", "alhamdulillah sudah", "alhamdulillah, sudah",
		"saya berdedikasi tinggi", "saya mampu mengerjakan", "saya siap bekerja",
		"ya, saya bersedia", "ya saya bersedia", "saya harus profesional",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(lower, p) {
			return true
		}
		if strings.Contains(lower, ". "+p) || strings.Contains(lower, ", "+p) {
			return true
		}
	}
	return false
}

func isInterviewerPromptMarker(lower string) bool {
	trimmed := strings.TrimSpace(lower)
	prompts := []string{
		"perkenalkan diri kamu", "perkenalkan diri anda",
		"gimana tadi perjalanannya", "sudah makan belum",
		"rumahnya di mana", "rumah kamu di mana", "rumah anda di mana",
		"kelebihan dan kekurangan kamu", "kelebihan dan kekurangan anda",
		"apa yang anda ketahui tentang perusahaan", "apa yang kamu ketahui tentang perusahaan",
		"mengapa kamu tertarik", "mengapa anda tertarik",
		"kontribusi apa yang akan kamu", "kontribusi apa yang akan anda",
		"lingkungan kerja seperti apa yang anda sukai",
		"lingkungan kerja seperti apa yang kamu sukai",
		"bisakah anda bekerja", "bisakah kamu bekerja",
		"apakah anda bersedia", "apakah kamu bersedia",
		"rencana anda dalam", "rencana kamu dalam",
		"apakah anda memiliki keterampilan lain", "apakah kamu memiliki keterampilan lain",
	}
	for _, p := range prompts {
		if strings.HasPrefix(trimmed, p) || strings.Contains(trimmed, ", "+p) || strings.Contains(trimmed, ". "+p) || strings.Contains(trimmed, "? "+p) {
			return true
		}
	}
	return false
}

// EnforceDialogueRoleConsistency ensures that conversational roles (Interviewer vs Candidate / Q&A)
// maintain consistent identity attribution across the entire recording, preventing identity flips.
func EnforceDialogueRoleConsistency(segments []dtos.SegmentResult) []dtos.SegmentResult {
	if len(segments) == 0 {
		return segments
	}

	var candidateSpeaker string
	var candidateName string
	var interviewerSpeaker string
	var interviewerName string

	for _, seg := range segments {
		clean := strings.ToLower(strings.TrimSpace(seg.Text))
		spk := NormalizeSpeakerLabel(seg.SpeakerLabel)

		if isCandidateIdentityMarker(clean) && candidateSpeaker == "" {
			candidateSpeaker = spk
			if !isGenericSpeaker(seg.SpeakerName) {
				candidateName = seg.SpeakerName
			}
		}

		if isInterviewerPromptMarker(clean) && interviewerSpeaker == "" {
			interviewerSpeaker = spk
			if !isGenericSpeaker(seg.SpeakerName) {
				interviewerName = seg.SpeakerName
			}
		}
	}

	if candidateSpeaker == "" || interviewerSpeaker == "" || candidateSpeaker == interviewerSpeaker {
		return segments
	}

	corrected := make([]dtos.SegmentResult, len(segments))
	copy(corrected, segments)

	for i := range corrected {
		clean := strings.ToLower(strings.TrimSpace(corrected[i].Text))

		// Guard against overriding high-confidence acoustic diarization evidence
		hasStrongAcousticAnchor := corrected[i].AcousticConfidence >= 0.70 && corrected[i].AcousticSpeaker != ""
		if hasStrongAcousticAnchor && !isCandidateIdentityMarker(clean) {
			continue
		}

		if isCandidateIdentityMarker(clean) {
			if NormalizeSpeakerLabel(corrected[i].SpeakerLabel) != candidateSpeaker {
				corrected[i].SpeakerLabel = candidateSpeaker
				if candidateName != "" {
					corrected[i].SpeakerName = candidateName
				} else {
					corrected[i].SpeakerName = candidateSpeaker
				}
			}
		} else if isInterviewerPromptMarker(clean) {
			if NormalizeSpeakerLabel(corrected[i].SpeakerLabel) != interviewerSpeaker {
				corrected[i].SpeakerLabel = interviewerSpeaker
				if interviewerName != "" {
					corrected[i].SpeakerName = interviewerName
				} else {
					corrected[i].SpeakerName = interviewerSpeaker
				}
			}
		} else if isCandidateResponseContent(clean) {
			if NormalizeSpeakerLabel(corrected[i].SpeakerLabel) != candidateSpeaker {
				corrected[i].SpeakerLabel = candidateSpeaker
				if candidateName != "" {
					corrected[i].SpeakerName = candidateName
				} else {
					corrected[i].SpeakerName = candidateSpeaker
				}
			}
		}
	}

	return corrected
}
