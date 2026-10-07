package templates

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"code-base-golang/internal/constants"
	"code-base-golang/internal/dtos"
	"code-base-golang/internal/pkg/timeutil"
)

//go:embed prompts/*.tmpl
var promptFS embed.FS

var parsedTemplates = template.Must(template.ParseFS(promptFS, "prompts/*.tmpl"))

// RAGChunkView wraps dtos.TranscriptChunkData with preformatted timestamp strings for template rendering.
type RAGChunkView struct {
	ChunkIndex     int
	Content        string
	StartFormatted string
	EndFormatted   string
}

// RAGUserData holds input parameters for rag_user.tmpl.
type RAGUserData struct {
	Query  string
	Chunks []RAGChunkView
}

// WorkspaceChunkView wraps chunk metadata for cross-meeting workspace Q&A.
type WorkspaceChunkView struct {
	ChunkIndex     int
	Content        string
	RecordingTitle string
	StartTime      float64
	EndTime        float64
	StartFormatted string
	EndFormatted   string
}

// WorkspaceRAGUserData holds input parameters for workspace_rag_user.tmpl.
type WorkspaceRAGUserData struct {
	Query   string
	Matches []WorkspaceChunkView
}

// SummaryUserData holds input parameters for summary_user.tmpl.
type SummaryUserData struct {
	Language       string
	TranscriptBody string
	CustomAngle    string
}

// FormatTimestamp converts seconds into a human-readable [MM:SS] or [HH:MM:SS] string.
func FormatTimestamp(seconds float64) string {
	return timeutil.FormatTimestamp(seconds)
}

// DefaultRAGSystemPrompt returns the static grounded RAG system prompt string.
func DefaultRAGSystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "rag_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute rag_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderRAGUserPrompt formats the query and retrieved context chunks into the grounded RAG user prompt.
func RenderRAGUserPrompt(query string, chunks []dtos.TranscriptChunkData) (string, error) {
	var chunkViews []RAGChunkView
	for _, c := range chunks {
		chunkViews = append(chunkViews, RAGChunkView{
			ChunkIndex:     c.ChunkIndex,
			Content:        strings.TrimSpace(c.Content),
			StartFormatted: FormatTimestamp(c.StartTime),
			EndFormatted:   FormatTimestamp(c.EndTime),
		})
	}

	data := RAGUserData{
		Query:  strings.TrimSpace(query),
		Chunks: chunkViews,
	}

	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "rag_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute rag_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderRAGPrompts returns both system and user prompts ready for LLM consumption.
func RenderRAGPrompts(query string, chunks []dtos.TranscriptChunkData) (systemPrompt, userPrompt string, err error) {
	systemPrompt, err = DefaultRAGSystemPrompt()
	if err != nil {
		return "", "", err
	}
	userPrompt, err = RenderRAGUserPrompt(query, chunks)
	if err != nil {
		return "", "", err
	}
	return systemPrompt, userPrompt, nil
}

// DefaultSummarySystemPrompt returns the default executive summarizer system prompt.
func DefaultSummarySystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "summary_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute summary_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderSummaryUserPrompt renders the user prompt containing the transcript body to be summarized.
// targetLanguage specifies the requested output language (e.g. "Indonesian", "English", "id", "en"), or empty for speaker's primary language.
// customAngle optionally specifies a particular analytical lens or focus area (e.g. "Focus on technical architecture and blockers").
func RenderSummaryUserPrompt(transcriptBody string, targetLanguage string, customAngle ...string) (string, error) {
	var angle string
	if len(customAngle) > 0 {
		angle = strings.TrimSpace(customAngle[0])
	}

	data := SummaryUserData{
		Language:       strings.TrimSpace(targetLanguage),
		TranscriptBody: strings.TrimSpace(transcriptBody),
		CustomAngle:    angle,
	}
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "summary_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute summary_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// DefaultWorkspaceRAGSystemPrompt returns the static cross-meeting workspace RAG system prompt.
func DefaultWorkspaceRAGSystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "workspace_rag_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute workspace_rag_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// DefaultDiarizeSystemPrompt returns the system prompt for transcription post-processing, speaker diarization, and phonetic correction.
func DefaultDiarizeSystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "diarize_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute diarize_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderWorkspaceRAGUserPrompt formats query and cross-meeting retrieved chunks into user prompt.
func RenderWorkspaceRAGUserPrompt(query string, matches []WorkspaceChunkView) (string, error) {
	var views []WorkspaceChunkView
	for _, m := range matches {
		title := strings.TrimSpace(m.RecordingTitle)
		if title == "" {
			title = "Untitled Meeting"
		}
		cleanContent := strings.ReplaceAll(m.Content, "</meeting_transcript>", "&lt;/meeting_transcript&gt;")

		startFmt := m.StartFormatted
		if startFmt == "" {
			startFmt = FormatTimestamp(m.StartTime)
		}
		endFmt := m.EndFormatted
		if endFmt == "" {
			endFmt = FormatTimestamp(m.EndTime)
		}

		views = append(views, WorkspaceChunkView{
			ChunkIndex:     m.ChunkIndex,
			Content:        cleanContent,
			RecordingTitle: title,
			StartFormatted: startFmt,
			EndFormatted:   endFmt,
		})
	}

	data := WorkspaceRAGUserData{
		Query:   strings.TrimSpace(query),
		Matches: views,
	}

	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "workspace_rag_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute workspace_rag_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderWorkspaceRAGPrompts returns both system and user prompts ready for workspace LLM chat.
func RenderWorkspaceRAGPrompts(query string, matches []WorkspaceChunkView) (systemPrompt, userPrompt string, err error) {
	systemPrompt, err = DefaultWorkspaceRAGSystemPrompt()
	if err != nil {
		return "", "", err
	}
	userPrompt, err = RenderWorkspaceRAGUserPrompt(query, matches)
	if err != nil {
		return "", "", err
	}
	return systemPrompt, userPrompt, nil
}

// DefaultPromptForTemplate returns the fallback system prompt for a given template category key.
func DefaultPromptForTemplate(categoryKey string) string {
	switch strings.ToUpper(categoryKey) {
	case constants.TemplateKeyPodcast:
		return "Analisis transkrip percakapan podcast atau talkshow berikut ke dalam format show notes terstruktur. Ekstraksi judul episode, profil tamu, ringkasan episode, pembahasan berdasarkan topik/bab (chapters) dengan perkiraan rentang timestamp, poin penting (key takeaways), kutipan berkesan (golden quotes), serta rekomendasi buku, artikel, atau tautan yang disebutkan."
	case constants.TemplateKeyLecture:
		return "Analisis transkrip perkuliahan, kelas daring, atau webinar akademis/profesional berikut ke dalam ringkasan materi pembelajaran yang komprehensif. Ekstraksi tujuan pembelajaran, ringkasan materi kuliah, konsep dan teori fundamental, glosarium istilah dan definisi teknis, catatan inti per topik bahasan, serta pertanyaan tinjauan ujian atau bahan diskusi mendalam."
	case constants.TemplateKeyMusicLyrics:
		return "Transkripsi dan analisis komposisi lirik musik dari rekaman audio berikut. Susun lirik secara terstruktur berdasarkan bagian lagu (seperti Intro, Verse, Pre-Chorus, Chorus, Bridge, dan Outro). Lakukan analisis nada emosional dan suasana hati (mood), pesan filosofis atau makna inti lagu, serta gaya musik dan instrumen yang teridentifikasi."
	default:
		return ""
	}
}

const (
	podcastSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["episode_title", "show_notes", "guest_overview", "topic_chapters", "key_takeaways", "golden_quotes"],
  "properties": {
    "episode_title": {"type": "string"},
    "show_notes": {"type": "string"},
    "guest_overview": {
      "type": "object",
      "properties": {
        "guest_name": {"type": "string"},
        "guest_title": {"type": "string"},
        "bio_or_background": {"type": "string"}
      }
    },
    "topic_chapters": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["timestamp", "topic", "summary"],
        "properties": {
          "timestamp": {"type": "string"},
          "topic": {"type": "string"},
          "summary": {"type": "string"}
        }
      }
    },
    "key_takeaways": {
      "type": "array",
      "items": {"type": "string"}
    },
    "golden_quotes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["quote", "speaker"],
        "properties": {
          "quote": {"type": "string"},
          "speaker": {"type": "string"},
          "context": {"type": "string"}
        }
      }
    }
  }
}`

	lectureSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["lecture_title", "course_summary", "core_concepts", "technical_glossary", "topic_notes", "exam_review_questions"],
  "properties": {
    "lecture_title": {"type": "string"},
    "course_summary": {"type": "string"},
    "learning_objectives": {
      "type": "array",
      "items": {"type": "string"}
    },
    "core_concepts": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["concept", "explanation"],
        "properties": {
          "concept": {"type": "string"},
          "explanation": {"type": "string"},
          "examples": {"type": "string"}
        }
      }
    },
    "technical_glossary": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["term", "definition"],
        "properties": {
          "term": {"type": "string"},
          "definition": {"type": "string"}
        }
      }
    },
    "topic_notes": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["topic", "key_points"],
        "properties": {
          "topic": {"type": "string"},
          "key_points": {
            "type": "array",
            "items": {"type": "string"}
          }
        }
      }
    },
    "exam_review_questions": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["question", "hint_or_key_answer"],
        "properties": {
          "question": {"type": "string"},
          "hint_or_key_answer": {"type": "string"}
        }
      }
    }
  }
}`

	musicLyricsSchemaJSON = `{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "required": ["song_title", "structured_lyrics", "emotional_tone", "core_message"],
  "properties": {
    "song_title": {"type": "string"},
    "artist_or_performers": {"type": "string"},
    "musical_style": {"type": "string"},
    "structured_lyrics": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["section", "lyrics"],
        "properties": {
          "section": {"type": "string"},
          "lyrics": {"type": "string"}
        }
      }
    },
    "emotional_tone": {
      "type": "object",
      "required": ["primary_mood", "energy_level"],
      "properties": {
        "primary_mood": {"type": "string"},
        "energy_level": {"type": "string"},
        "sentiment_analysis": {"type": "string"}
      }
    },
    "core_message": {"type": "string"}
  }
}`
)

var cachedSchemas = make(map[string]map[string]interface{})

func init() {
	rawMap := map[string]string{
		constants.TemplateKeyPodcast:     podcastSchemaJSON,
		constants.TemplateKeyLecture:     lectureSchemaJSON,
		constants.TemplateKeyMusicLyrics: musicLyricsSchemaJSON,
	}
	for key, rawJSON := range rawMap {
		var schema map[string]interface{}
		if err := json.Unmarshal([]byte(rawJSON), &schema); err != nil {
			panic(fmt.Sprintf("failed to parse static template schema for %s: %v", key, err))
		}
		cachedSchemas[key] = schema
	}
}

// DefaultSchemaForTemplate returns the cached fallback JSON schema map for a given template key.
func DefaultSchemaForTemplate(categoryKey string) map[string]interface{} {
	return cachedSchemas[strings.ToUpper(categoryKey)]
}

