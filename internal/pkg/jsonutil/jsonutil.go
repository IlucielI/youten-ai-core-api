package jsonutil

import (
	"encoding/json"
	"strings"
)

// RawStringSlice encodes a slice of strings to json.RawMessage.
// If the slice is empty or nil, it returns a static "[]" without allocation or reflection.
func RawStringSlice(items []string) json.RawMessage {
	if len(items) == 0 {
		return json.RawMessage("[]")
	}
	b, err := json.Marshal(items)
	if err != nil {
		return json.RawMessage("[]")
	}
	return json.RawMessage(b)
}

// CleanMarkdownJSON strips markdown code fences (```json or ```) and trims whitespace.
// If the content is wrapped inside a JSON object or array, it isolates the outermost brackets.
func CleanMarkdownJSON(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```json") {
		trimmed = strings.TrimPrefix(trimmed, "```json")
	} else if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimPrefix(trimmed, "```")
	}
	trimmed = strings.TrimSuffix(trimmed, "```")
	trimmed = strings.TrimSpace(trimmed)

	startObj := strings.Index(trimmed, "{")
	endObj := strings.LastIndex(trimmed, "}")
	if startObj != -1 && endObj != -1 && endObj > startObj {
		return trimmed[startObj : endObj+1]
	}

	startArr := strings.Index(trimmed, "[")
	endArr := strings.LastIndex(trimmed, "]")
	if startArr != -1 && endArr != -1 && endArr > startArr {
		return trimmed[startArr : endArr+1]
	}

	return trimmed
}

