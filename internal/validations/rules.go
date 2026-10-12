package validations

import (
	"errors"
	"regexp"
	"strings"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"
)

var (
	// DigitRegex matches strings containing at least one digit character.
	DigitRegex = regexp.MustCompile(`[0-9]`)
	digitRegex = DigitRegex

	// SixDigitCodeRegex matches exactly 6 numeric digits (e.g. OTP verification codes).
	SixDigitCodeRegex = regexp.MustCompile(`^[0-9]{6}$`)
	sixDigitCodeRegex = SixDigitCodeRegex

	// NotEmptyUUID validates that a uuid.UUID field is not the zero-value uuid.Nil.
	NotEmptyUUID = validation.By(func(value interface{}) error {
		id, ok := value.(uuid.UUID)
		if !ok || id == uuid.Nil {
			return errors.New("cannot be blank")
		}
		return nil
	})
	notEmptyUUID = NotEmptyUUID
)

// TrimSpacePtr trims whitespace from the referenced string if the pointer is non-nil.
func TrimSpacePtr(s *string) *string {
	if s != nil {
		*s = strings.TrimSpace(*s)
	}
	return s
}

// TrimSlice trims whitespace from each string item in the provided slice in-place.
func TrimSlice(items []string) []string {
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}
	return items
}

// TrimSlicePtr trims whitespace from each string item if the slice pointer is non-nil.
func TrimSlicePtr(items *[]string) *[]string {
	if items != nil {
		TrimSlice(*items)
	}
	return items
}
