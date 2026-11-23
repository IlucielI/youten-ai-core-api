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

//go:embed schemas/*.json
var schemaFS embed.FS

var parsedTemplates = template.Must(template.ParseFS(promptFS, "prompts/*.tmpl"))

// RAGChunkView wraps dtos.TranscriptChunkData with preformatted timestamp strings for template rendering.
type RAGChunkView struct {
	ChunkIndex     int
	Content        string
	StartFormatted string
	EndFormatted   string
}

// RAGMetadataView wraps high-level meeting metadata for grounded prompt rendering.
type RAGMetadataView struct {
	Title             string
	DurationFormatted string
	TemplateCategory  string
	DetectedLanguage  string
	Speakers          []string
	SummaryOverview   string
	KeyTopics         []string
	ActionItems       []string
}

// RAGUserData holds input parameters for rag_user.tmpl.
type RAGUserData struct {
	Query    string
	Chunks   []RAGChunkView
	Metadata *RAGMetadataView
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

// WorkspaceMeetingCatalogItem holds summarized metadata of an individual meeting in the workspace catalog.
type WorkspaceMeetingCatalogItem struct {
	RecordingID        string
	Title              string
	FolderName         string
	DurationFormatted  string
	CreatedAtFormatted string
	Overview           string
}

// WorkspaceRAGUserData holds input parameters for workspace_rag_user.tmpl.
type WorkspaceRAGUserData struct {
	Query   string
	Matches []WorkspaceChunkView
	Catalog []WorkspaceMeetingCatalogItem
}

// SummaryUserData holds input parameters for summary_user.tmpl.
type SummaryUserData struct {
	Language       string
	TranscriptBody string
	CustomAngle    string
	VisualCues     []VisualCueView
}

// VisualCueView represents a presentation slide or screen cue formatted for prompt ingestion.
type VisualCueView struct {
	TimestampFormatted string
	CueType            string
	Title              string
	ExtractedText      string
	VisualSummary      string
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

// RenderRAGUserPrompt formats the query, optional meeting metadata, and retrieved context chunks into the grounded RAG user prompt.
func RenderRAGUserPrompt(query string, chunks []dtos.TranscriptChunkData, meta ...*dtos.RecordingContextMetadata) (string, error) {
	var chunkViews []RAGChunkView
	for _, c := range chunks {
		chunkViews = append(chunkViews, RAGChunkView{
			ChunkIndex:     c.ChunkIndex,
			Content:        strings.TrimSpace(c.Content),
			StartFormatted: FormatTimestamp(c.StartTime),
			EndFormatted:   FormatTimestamp(c.EndTime),
		})
	}

	var metaView *RAGMetadataView
	if len(meta) > 0 && meta[0] != nil {
		m := meta[0]
		metaView = &RAGMetadataView{
			Title:             strings.TrimSpace(m.Title),
			DurationFormatted: strings.TrimSpace(m.DurationFormatted),
			TemplateCategory:  strings.TrimSpace(m.SelectedTemplate),
			DetectedLanguage:  strings.TrimSpace(m.DetectedLanguage),
			Speakers:          m.Speakers,
			SummaryOverview:   strings.TrimSpace(m.SummaryOverview),
			KeyTopics:         m.KeyTopics,
			ActionItems:       m.ActionItems,
		}
	}

	data := RAGUserData{
		Query:    strings.TrimSpace(query),
		Chunks:   chunkViews,
		Metadata: metaView,
	}

	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "rag_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute rag_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderRAGPrompts returns both system and user prompts ready for LLM consumption.
func RenderRAGPrompts(query string, chunks []dtos.TranscriptChunkData, meta ...*dtos.RecordingContextMetadata) (systemPrompt, userPrompt string, err error) {
	systemPrompt, err = DefaultRAGSystemPrompt()
	if err != nil {
		return "", "", err
	}
	userPrompt, err = RenderRAGUserPrompt(query, chunks, meta...)
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
	return RenderSummaryUserPromptWithCues(transcriptBody, targetLanguage, angle, nil)
}

// RenderSummaryUserPromptWithCues renders the user prompt containing transcript body and timestamped visual cues.
func RenderSummaryUserPromptWithCues(transcriptBody string, targetLanguage string, customAngle string, cues []VisualCueView) (string, error) {
	data := SummaryUserData{
		Language:       strings.TrimSpace(targetLanguage),
		TranscriptBody: strings.TrimSpace(transcriptBody),
		CustomAngle:    strings.TrimSpace(customAngle),
		VisualCues:     cues,
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

// DefaultSpeakerRosterSystemPrompt returns the system prompt for discovering the global speaker roster.
func DefaultSpeakerRosterSystemPrompt() (string, error) {
	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "speaker_roster_system.tmpl", nil); err != nil {
		return "", fmt.Errorf("execute speaker_roster_system.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderWorkspaceRAGUserPrompt formats query, retrieved chunks, and optional meeting catalog into user prompt.
func RenderWorkspaceRAGUserPrompt(query string, matches []WorkspaceChunkView, catalog ...WorkspaceMeetingCatalogItem) (string, error) {
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
		Catalog: catalog,
	}

	var buf bytes.Buffer
	if err := parsedTemplates.ExecuteTemplate(&buf, "workspace_rag_user.tmpl", data); err != nil {
		return "", fmt.Errorf("execute workspace_rag_user.tmpl: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

// RenderWorkspaceRAGPrompts returns both system and user prompts ready for workspace LLM chat.
func RenderWorkspaceRAGPrompts(query string, matches []WorkspaceChunkView, catalog ...WorkspaceMeetingCatalogItem) (systemPrompt, userPrompt string, err error) {
	systemPrompt, err = DefaultWorkspaceRAGSystemPrompt()
	if err != nil {
		return "", "", err
	}
	userPrompt, err = RenderWorkspaceRAGUserPrompt(query, matches, catalog...)
	if err != nil {
		return "", "", err
	}
	return systemPrompt, userPrompt, nil
}

// DefaultPromptForTemplate returns the fallback system prompt for a given template category key.
func DefaultPromptForTemplate(categoryKey string) string {
	switch strings.ToUpper(categoryKey) {
	case constants.TemplateKeyGeneral:
		return "Sintesis percakapan umum/diskusi eksekutif ke dalam format Executive Memo terstruktur mencakup ringkasan eksekutif (executive_overview), sub pembahasan dan poin-poin kunci per topik (discussion_topics), isu-isu terbuka yang belum terselesaikan (open_points), tindakan lanjutan ber-PIC (action_items), serta kutipan dan poin kunci strategis."
	case constants.TemplateKeyMOM:
		return "Analisis transkrip percakapan berikut ke format Minutes of Meeting (MOM) terstruktur dengan agenda, daftar hadir, sub pembahasan dan poin kunci per topik (discussion_topics), konsensus dan keputusan final (key_decisions), isu-isu terbuka (open_points/open_issues), tabel action items dengan properti task, pic, due_date, priority, definition_of_done, dan cost_of_inaction, dinamika rapat (meeting_dynamics) yang mencakup speaker_talk_time, consensus_score, dan sentiment_trajectory, serta perspektif pemangku kepentingan (stakeholder_perspectives) yang mencakup executive_brief, engineering_focus, dan product_delivery. Pastikan seluruh properti wajib tersebut dihasilkan sesuai struktur schema MOM."
	case constants.TemplateKeyTechReview:
		return "Sintesis percakapan teknis arsitektur ke dalam format RFC/ADR mencakup konteks arsitektural (context), sub pembahasan teknis dan poin kunci per topik (discussion_topics), keputusan yang diadopsi vs ditolak, penilaian kebutuhan non-fungsional (nfr_assessment) mencakup keamanan, performa, dan reliabilitas, isu terbuka atau risiko yang masih menggantung (open_points), serta rencana aksi mitigasi teknis (action_items). Pastikan seluruh properti wajib tersebut dihasilkan sesuai struktur schema."
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

var cachedSchemas = make(map[string]map[string]interface{})

func init() {
	schemaFiles := map[string]string{
		constants.TemplateKeyGeneral:     "schemas/general.json",
		constants.TemplateKeyMOM:         "schemas/mom.json",
		constants.TemplateKeyTechReview:  "schemas/tech_review.json",
		constants.TemplateKeyPodcast:     "schemas/podcast.json",
		constants.TemplateKeyLecture:     "schemas/lecture.json",
		constants.TemplateKeyMusicLyrics: "schemas/music_lyrics.json",
	}
	for key, filename := range schemaFiles {
		raw, err := schemaFS.ReadFile(filename)
		if err != nil {
			panic(fmt.Sprintf("failed to read embedded schema file %s for %s: %v", filename, key, err))
		}
		var schema map[string]interface{}
		if err := json.Unmarshal(raw, &schema); err != nil {
			panic(fmt.Sprintf("failed to parse static template schema for %s: %v", key, err))
		}
		cachedSchemas[key] = schema
	}
}

// DefaultSchemaForTemplate returns the cached fallback JSON schema map for a given template key.
func DefaultSchemaForTemplate(categoryKey string) map[string]interface{} {
	return cachedSchemas[strings.ToUpper(categoryKey)]
}

