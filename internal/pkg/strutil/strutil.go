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
