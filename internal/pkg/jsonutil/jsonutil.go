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

// RepairJSON cleans markdown fences and attempts to repair common LLM JSON syntax errors
// such as mismatched closing brackets (e.g., `]` closing `{`) and trailing commas.
func RepairJSON(raw string) string {
	cleaned := CleanMarkdownJSON(raw)
	if cleaned == "" || json.Valid([]byte(cleaned)) {
		return cleaned
	}

	repaired := repairJSONBracketsAndCommas(cleaned)
	if json.Valid([]byte(repaired)) {
		return repaired
	}
	return cleaned
}

// UnmarshalMapLenient cleans and repairs LLM JSON output before unmarshaling into a map.
// It returns the parsed map, the cleaned/repaired JSON string, and any unmarshal error.
func UnmarshalMapLenient(raw string) (map[string]interface{}, string, error) {
	cleaned := CleanMarkdownJSON(raw)
	var out map[string]interface{}
	if err := json.Unmarshal([]byte(cleaned), &out); err == nil {
		return out, cleaned, nil
	}

	repaired := RepairJSON(raw)
	if err := json.Unmarshal([]byte(repaired), &out); err != nil {
		return nil, cleaned, err
	}
	return out, repaired, nil
}

func stripTrailingCommaBeforeClose(buf []byte) []byte {
	idx := len(buf) - 1
	for idx >= 0 && isJSONWhitespace(buf[idx]) {
		idx--
	}
	if idx >= 0 && buf[idx] == ',' {
		return buf[:idx]
	}
	return buf
}

func isJSONWhitespace(b byte) bool {
	return b == ' ' || b == '\t' || b == '\n' || b == '\r'
}

func repairJSONBracketsAndCommas(input string) string {
	out := make([]byte, 0, len(input)+8)
	stack := make([]byte, 0, 16)
	inString := false
	escaped := false

	for i := 0; i < len(input); i++ {
		ch := input[i]
		if inString {
			out = append(out, ch)
			inString, escaped = advanceStringState(ch, escaped)
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}

		out, stack = handleStructuralByte(ch, out, stack)
	}

	if inString {
		if escaped {
			out = append(out, '\\')
		}
		out = append(out, '"')
	}
	for len(stack) > 0 {
		out = stripTrailingCommaBeforeClose(out)
		expected := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		out = append(out, expected)
	}
	return string(out)
}

func advanceStringState(ch byte, escaped bool) (inString bool, nextEscaped bool) {
	if escaped {
		return true, false
	}
	if ch == '\\' {
		return true, true
	}
	if ch == '"' {
		return false, false
	}
	return true, false
}

func handleStructuralByte(ch byte, out, stack []byte) ([]byte, []byte) {
	switch ch {
	case '{':
		return append(out, ch), append(stack, '}')
	case '[':
		return append(out, ch), append(stack, ']')
	case '}', ']':
		if len(stack) == 0 {
			return append(out, ch), stack
		}
		out = stripTrailingCommaBeforeClose(out)
		expected := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return append(out, expected), stack
	default:
		return append(out, ch), stack
	}
}

