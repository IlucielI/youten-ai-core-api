package queue

import (
	"encoding/json"
	"fmt"
)

// Id represents a common payload containing an entity identifier.
type Id struct {
	Id string `json:"id"`
}

// TriggerOption represents a common payload option for triggering background jobs.
type TriggerOption struct {
	Force bool  `json:"force"`
	Since int64 `json:"since"`
}

// MustParse unmarshals JSON byte payload into dest or panics on error.
func MustParse(b []byte, dest any) {
	err := json.Unmarshal(b, dest)
	if err != nil {
		panic(fmt.Errorf("failed to parse payload: %w", err))
	}
}

// Parse unmarshals JSON byte payload into dest returning any error encountered.
func Parse(b []byte, dest any) error {
	return json.Unmarshal(b, dest)
}
