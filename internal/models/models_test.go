package models

import (
	"testing"
)

func TestTableNames(t *testing.T) {
	testCases := []struct {
		model    interface{ TableName() string }
		expected string
	}{
		{model: User{}, expected: "users"},
		{model: AuthToken{}, expected: "auth_tokens"},
		{model: AdminRole{}, expected: "admin_roles"},
		{model: AdminUser{}, expected: "admin_users"},
		{model: AdminAuditLog{}, expected: "admin_audit_logs"},
		{model: AuditLog{}, expected: "audit_logs"},
		{model: Template{}, expected: "templates"},
		{model: Recording{}, expected: "recordings"},
		{model: TranscriptSegment{}, expected: "transcript_segments"},
		{model: TranscriptChunk{}, expected: "transcript_chunks"},
		{model: Summary{}, expected: "summaries"},
		{model: Chapter{}, expected: "chapters"},
		{model: Highlight{}, expected: "highlights"},
		{model: InlineComment{}, expected: "inline_comments"},
		{model: ChatMessage{}, expected: "chat_messages"},
		{model: Notification{}, expected: "notifications"},
		{model: Report{}, expected: "reports"},
	}

	for _, tc := range testCases {
		if tc.model.TableName() != tc.expected {
			t.Errorf("expected table name %q, got %q", tc.expected, tc.model.TableName())
		}
	}
}

func TestJSONMap_ValueAndScan(t *testing.T) {
	m := JSONMap{"key": "value", "count": float64(42)}

	// Test Value
	val, err := m.Value()
	if err != nil {
		t.Fatalf("unexpected error on Value(): %v", err)
	}

	// Test Scan with []byte
	var scanned JSONMap
	if err := scanned.Scan(val); err != nil {
		t.Fatalf("unexpected error on Scan([]byte): %v", err)
	}
	if scanned["key"] != "value" || scanned["count"] != float64(42) {
		t.Fatalf("scanned data mismatch: %+v", scanned)
	}

	// Test Scan with string
	var scannedStr JSONMap
	if err := scannedStr.Scan(`{"foo":"bar"}`); err != nil {
		t.Fatalf("unexpected error on Scan(string): %v", err)
	}
	if scannedStr["foo"] != "bar" {
		t.Fatalf("scanned data mismatch for string: %+v", scannedStr)
	}

	// Test Scan with nil
	var scannedNil JSONMap
	if err := scannedNil.Scan(nil); err != nil {
		t.Fatalf("unexpected error on Scan(nil): %v", err)
	}
	if len(scannedNil) != 0 {
		t.Fatalf("expected empty map for Scan(nil), got %+v", scannedNil)
	}

	// Test Scan with empty bytes
	var scannedEmpty JSONMap
	if err := scannedEmpty.Scan([]byte{}); err != nil {
		t.Fatalf("unexpected error on Scan([]byte{}): %v", err)
	}
	if len(scannedEmpty) != 0 {
		t.Fatalf("expected empty map for Scan([]byte{}), got %+v", scannedEmpty)
	}

	// Test Value for nil map
	var nilMap JSONMap
	nilVal, err := nilMap.Value()
	if err != nil {
		t.Fatalf("unexpected error on nil Value(): %v", err)
	}
	if nilVal != "{}" {
		t.Fatalf("expected '{}' for nil map Value(), got %v", nilVal)
	}

	// Test Scan with invalid type
	var invalidMap JSONMap
	if err := invalidMap.Scan(12345); err == nil {
		t.Fatal("expected error on Scan with unsupported type, got nil")
	}
}

func TestVector_ValueAndScan(t *testing.T) {
	v := Vector{0.123, -0.456, 7.89}

	// Test Value
	val, err := v.Value()
	if err != nil {
		t.Fatalf("unexpected error on Vector.Value(): %v", err)
	}

	// Test Scan with string
	var scanned Vector
	if err := scanned.Scan(val); err != nil {
		t.Fatalf("unexpected error on Vector.Scan(string): %v", err)
	}
	if len(scanned) != len(v) {
		t.Fatalf("expected len %d, got %d", len(v), len(scanned))
	}
	for i := range v {
		if scanned[i] != v[i] {
			t.Errorf("at index %d: expected %f, got %f", i, v[i], scanned[i])
		}
	}

	// Test Scan with []byte
	var scannedBytes Vector
	if err := scannedBytes.Scan([]byte("[1.5, -2.5]")); err != nil {
		t.Fatalf("unexpected error on Vector.Scan([]byte): %v", err)
	}
	if len(scannedBytes) != 2 || scannedBytes[0] != 1.5 || scannedBytes[1] != -2.5 {
		t.Fatalf("scannedBytes mismatch: %+v", scannedBytes)
	}

	// Test Scan with nil
	var scannedNil Vector
	if err := scannedNil.Scan(nil); err != nil {
		t.Fatalf("unexpected error on Vector.Scan(nil): %v", err)
	}
	if scannedNil != nil {
		t.Fatalf("expected nil for Scan(nil), got %+v", scannedNil)
	}

	// Test Scan with empty
	var scannedEmpty Vector
	if err := scannedEmpty.Scan("[]"); err != nil {
		t.Fatalf("unexpected error on Vector.Scan('[]'): %v", err)
	}
	if len(scannedEmpty) != 0 {
		t.Fatalf("expected empty for Scan('[]'), got %+v", scannedEmpty)
	}

	// Test Value for nil Vector
	var nilVec Vector
	nilVal, err := nilVec.Value()
	if err != nil {
		t.Fatalf("unexpected error on nil Vector.Value(): %v", err)
	}
	if nilVal != nil {
		t.Fatalf("expected nil for nil Vector Value(), got %v", nilVal)
	}

	// Test Scan with invalid string
	var invalidVec Vector
	if err := invalidVec.Scan("[abc, def]"); err == nil {
		t.Fatal("expected error on Vector.Scan with invalid floats, got nil")
	}

	// Test Scan with unsupported type
	if err := invalidVec.Scan(12345); err == nil {
		t.Fatal("expected error on Vector.Scan with unsupported type, got nil")
	}
}
