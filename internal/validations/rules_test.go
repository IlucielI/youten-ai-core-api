package validations

import (
	"testing"

	validation "github.com/go-ozzo/ozzo-validation/v4"
	"github.com/google/uuid"
)

func TestNotEmptyUUID(t *testing.T) {
	t.Run("valid uuid passes", func(t *testing.T) {
		id := uuid.New()
		if err := validation.Validate(id, NotEmptyUUID); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("nil uuid fails", func(t *testing.T) {
		if err := validation.Validate(uuid.Nil, NotEmptyUUID); err == nil {
			t.Fatal("expected error for uuid.Nil, got nil")
		}
	})

	t.Run("non uuid type fails", func(t *testing.T) {
		if err := validation.Validate("not-a-uuid", NotEmptyUUID); err == nil {
			t.Fatal("expected error for non-uuid type, got nil")
		}
	})
}

func TestDigitRegex(t *testing.T) {
	t.Run("contains digit passes", func(t *testing.T) {
		if !DigitRegex.MatchString("pass1word") {
			t.Fatal("expected match for string with digit")
		}
	})

	t.Run("no digit fails", func(t *testing.T) {
		if DigitRegex.MatchString("password") {
			t.Fatal("expected no match for string without digit")
		}
	})
}

func TestSixDigitCodeRegex(t *testing.T) {
	t.Run("valid 6 digit code passes", func(t *testing.T) {
		if !SixDigitCodeRegex.MatchString("123456") {
			t.Fatal("expected match for 6 digit code")
		}
	})

	t.Run("5 digit code fails", func(t *testing.T) {
		if SixDigitCodeRegex.MatchString("12345") {
			t.Fatal("expected no match for 5 digit code")
		}
	})

	t.Run("7 digit code fails", func(t *testing.T) {
		if SixDigitCodeRegex.MatchString("1234567") {
			t.Fatal("expected no match for 7 digit code")
		}
	})

	t.Run("alpha chars fail", func(t *testing.T) {
		if SixDigitCodeRegex.MatchString("12345a") {
			t.Fatal("expected no match for alphanumeric code")
		}
	})
}

func TestTrimSpacePtr(t *testing.T) {
	t.Run("nil pointer remains nil", func(t *testing.T) {
		res := TrimSpacePtr(nil)
		if res != nil {
			t.Fatalf("expected nil, got %v", res)
		}
	})

	t.Run("trims string whitespace", func(t *testing.T) {
		str := "  hello world  "
		res := TrimSpacePtr(&str)
		if *res != "hello world" || str != "hello world" {
			t.Fatalf("expected 'hello world', got '%s'", *res)
		}
	})
}

func TestTrimSlice(t *testing.T) {
	t.Run("trims slice items", func(t *testing.T) {
		items := []string{"  foo ", "bar  ", " baz "}
		TrimSlice(items)
		expected := []string{"foo", "bar", "baz"}
		for i, v := range items {
			if v != expected[i] {
				t.Fatalf("at index %d: expected %s, got %s", i, expected[i], v)
			}
		}
	})
}

func TestTrimSlicePtr(t *testing.T) {
	t.Run("nil pointer remains nil", func(t *testing.T) {
		res := TrimSlicePtr(nil)
		if res != nil {
			t.Fatalf("expected nil, got %v", res)
		}
	})

	t.Run("trims slice pointer items", func(t *testing.T) {
		items := []string{" a ", " b "}
		TrimSlicePtr(&items)
		if items[0] != "a" || items[1] != "b" {
			t.Fatalf("expected ['a', 'b'], got %v", items)
		}
	})
}

