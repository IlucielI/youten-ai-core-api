package validations

import (
	"errors"
	"regexp"

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
