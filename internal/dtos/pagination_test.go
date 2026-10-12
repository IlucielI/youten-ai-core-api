package dtos

import (
	"testing"
)

func TestPaginationQuery_SetDefaults(t *testing.T) {
	t.Run("defaults invalid values to standard limit", func(t *testing.T) {
		p := PaginationQuery{Page: 0, Limit: -10}
		p.SetDefaults()

		if p.Page != 1 {
			t.Errorf("expected page 1, got %d", p.Page)
		}
		if p.Limit != DefaultPaginationLimit {
			t.Errorf("expected limit %d, got %d", DefaultPaginationLimit, p.Limit)
		}
	})

	t.Run("defaults invalid values with custom limit", func(t *testing.T) {
		p := PaginationQuery{Page: -5, Limit: 0}
		p.SetDefaults(10)

		if p.Page != 1 {
			t.Errorf("expected page 1, got %d", p.Page)
		}
		if p.Limit != 10 {
			t.Errorf("expected limit 10, got %d", p.Limit)
		}
	})

	t.Run("caps limit at MaxPaginationLimit", func(t *testing.T) {
		p := PaginationQuery{Page: 2, Limit: 500}
		p.SetDefaults()

		if p.Page != 2 {
			t.Errorf("expected page 2, got %d", p.Page)
		}
		if p.Limit != MaxPaginationLimit {
			t.Errorf("expected limit %d, got %d", MaxPaginationLimit, p.Limit)
		}
	})
}

func TestPaginationQuery_Offset(t *testing.T) {
	t.Run("page 1 returns 0", func(t *testing.T) {
		p := PaginationQuery{Page: 1, Limit: 20}
		if got := p.Offset(); got != 0 {
			t.Errorf("expected 0, got %d", got)
		}
	})

	t.Run("page 0 or negative returns 0", func(t *testing.T) {
		p := PaginationQuery{Page: 0, Limit: 20}
		if got := p.Offset(); got != 0 {
			t.Errorf("expected 0, got %d", got)
		}
	})

	t.Run("page 3 with limit 15 returns 30", func(t *testing.T) {
		p := PaginationQuery{Page: 3, Limit: 15}
		if got := p.Offset(); got != 30 {
			t.Errorf("expected 30, got %d", got)
		}
	})
}

func TestCalculateTotalPages(t *testing.T) {
	tests := []struct {
		name     string
		total    int64
		limit    int
		expected int
	}{
		{"zero total", 0, 20, 0},
		{"negative total", -5, 20, 0},
		{"zero limit", 100, 0, 0},
		{"exact match", 40, 20, 2},
		{"partial page rounded up", 41, 20, 3},
		{"single item", 1, 20, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalculateTotalPages(tt.total, tt.limit); got != tt.expected {
				t.Errorf("CalculateTotalPages(%d, %d) = %d, expected %d", tt.total, tt.limit, got, tt.expected)
			}
		})
	}
}

func TestNewPaginationMeta(t *testing.T) {
	meta := NewPaginationMeta(2, 20, 45)
	if meta.CurrentPage != 2 {
		t.Errorf("expected current page 2, got %d", meta.CurrentPage)
	}
	if meta.PageSize != 20 {
		t.Errorf("expected page size 20, got %d", meta.PageSize)
	}
	if meta.TotalItems != 45 {
		t.Errorf("expected total items 45, got %d", meta.TotalItems)
	}
	if meta.TotalPages != 3 {
		t.Errorf("expected total pages 3, got %d", meta.TotalPages)
	}
}
