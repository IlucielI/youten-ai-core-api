package jsonutil

import (
	"encoding/json"
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
