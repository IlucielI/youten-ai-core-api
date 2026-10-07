package strutil

import (
	"strings"
	"unicode"
)

// SanitizeFilename normalizes an input title into a filesystem-safe filename slug.
// Spaces are replaced with underscores, non-alphanumeric/hyphen/underscore characters are removed,
// leading/trailing separators are trimmed, and the result is capped at maxLength (defaults to 60).
func SanitizeFilename(title string, fallback string, maxLength ...int) string {
	limit := 60
	if len(maxLength) > 0 && maxLength[0] > 0 {
		limit = maxLength[0]
	}

	t := strings.TrimSpace(title)
	if t == "" {
		return fallback
	}
	var sb strings.Builder
	var lastWritten rune
	for _, r := range t {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			if (r == '_' || r == '-') && (lastWritten == '_' || lastWritten == '-') {
				continue
			}
			sb.WriteRune(r)
			lastWritten = r
		} else if unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r) {
			if lastWritten != '_' && lastWritten != '-' && sb.Len() > 0 {
				sb.WriteRune('_')
				lastWritten = '_'
			}
		}
	}
	res := strings.Trim(sb.String(), "_-")
	if res == "" {
		return fallback
	}
	runes := []rune(res)
	if len(runes) > limit {
		res = strings.Trim(string(runes[:limit]), "_-")
	}
	if res == "" {
		return fallback
	}
	return res
}

// InferMediaExtension maps a MIME content-type string to its standard file extension with dot.
// Returns an empty string if the MIME type is unrecognized or unsupported.
func InferMediaExtension(contentType string) string {
	cleanMIME := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch cleanMIME {
	case "audio/mpeg", "audio/mp3":
		return ".mp3"
	case "audio/wav", "audio/x-wav", "audio/wave":
		return ".wav"
	case "audio/mp4", "video/mp4":
		return ".mp4"
	case "audio/m4a", "audio/x-m4a":
		return ".m4a"
	case "audio/webm", "video/webm":
		return ".webm"
	case "audio/ogg":
		return ".ogg"
	case "video/quicktime":
		return ".mov"
	default:
		return ""
	}
}

// NormalizeLanguageCode converts language name or tag into normalized 2-letter ISO code ("id", "en", etc.).
// If empty, returns fallback (typically "id").
func NormalizeLanguageCode(lang string, fallback string) string {
	l := strings.ToLower(strings.TrimSpace(lang))
	switch l {
	case "en", "english", "eng":
		return "en"
	case "id", "indonesian", "indonesia", "ind":
		return "id"
	case "":
		return fallback
	default:
		return l
	}
}

// DetectLanguage inspects text using high-frequency marker word heuristics to detect language ("en" or "id").
// Returns fallback if confidence is low or text has insufficient word signal.
func DetectLanguage(text string, fallback string) string {
	clean := strings.ToLower(text)
	words := strings.Fields(clean)
	if len(words) == 0 {
		return fallback
	}

	idKeywords := map[string]struct{}{
		"yang": {}, "dan": {}, "di": {}, "ini": {}, "itu": {},
		"untuk": {}, "dengan": {}, "saya": {}, "kamu": {}, "bisa": {},
		"adalah": {}, "tidak": {}, "dari": {}, "ke": {}, "pada": {},
		"sudah": {}, "akan": {}, "kita": {}, "mereka": {}, "ada": {},
		"karena": {}, "tapi": {}, "juga": {}, "oleh": {}, "seperti": {},
	}

	enKeywords := map[string]struct{}{
		"the": {}, "and": {}, "you": {}, "that": {}, "is": {},
		"was": {}, "for": {}, "with": {}, "are": {}, "this": {},
		"have": {}, "from": {}, "they": {}, "here": {}, "how": {},
		"what": {}, "did": {}, "not": {}, "there": {}, "about": {},
		"well": {}, "like": {}, "just": {}, "were": {}, "when": {},
	}

	idCount := 0
	enCount := 0
	for _, w := range words {
		w = strings.Trim(w, ",.?!:;\"'()[]{}")
		if _, ok := idKeywords[w]; ok {
			idCount++
		}
		if _, ok := enKeywords[w]; ok {
			enCount++
		}
	}

	if enCount > idCount && enCount >= 2 {
		return "en"
	}
	if idCount > enCount && idCount >= 2 {
		return "id"
	}
	if fallback != "" {
		return fallback
	}
	return "id"
}

