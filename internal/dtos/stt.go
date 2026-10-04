package dtos

// STTOptions contains configuration options for a transcription request.
type STTOptions struct {
	Language    string  `json:"language,omitempty"`
	Prompt      string  `json:"prompt,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

// WordResult represents word-level karaoke timing.
type WordResult struct {
	Word        string  `json:"word"`
	Start       float64 `json:"start"`
	End         float64 `json:"end"`
	Probability float64 `json:"probability,omitempty"`
}

// SegmentResult represents an individual transcribed segment with timing and optional speaker.
type SegmentResult struct {
	ID               int          `json:"id"`
	Seek             int          `json:"seek"`
	Start            float64      `json:"start"`
	End              float64      `json:"end"`
	Text             string       `json:"text"`
	SpeakerLabel     string       `json:"speaker_label,omitempty"`
	Tokens           []int        `json:"tokens,omitempty"`
	Temperature      float64      `json:"temperature,omitempty"`
	AvgLogprob       float64      `json:"avg_logprob,omitempty"`
	CompressionRatio float64      `json:"compression_ratio,omitempty"`
	NoSpeechProb     float64      `json:"no_speech_prob,omitempty"`
	Words            []WordResult `json:"words,omitempty"`
}

// TranscriptionResult represents the overall transcription output.
type TranscriptionResult struct {
	Text     string          `json:"text"`
	Language string          `json:"language"`
	Duration float64         `json:"duration"`
	Segments []SegmentResult `json:"segments"`
}
