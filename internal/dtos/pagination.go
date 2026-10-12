package dtos

// DefaultPaginationLimit defines the standard default page size.
const DefaultPaginationLimit = 20

// MaxPaginationLimit defines the maximum allowed page size.
const MaxPaginationLimit = 100

// PaginationQuery provides reusable pagination query parameters and offset calculation.
type PaginationQuery struct {
	Page  int `form:"page" json:"page"`
	Limit int `form:"limit" json:"limit"`
}

// SetDefaults ensures Page >= 1 and Limit is bounded between 1 and 100, falling back to defaultLimit or 20.
func (p *PaginationQuery) SetDefaults(defaultLimit ...int) {
	if p.Page < 1 {
		p.Page = 1
	}
	fallback := DefaultPaginationLimit
	if len(defaultLimit) > 0 && defaultLimit[0] > 0 {
		fallback = defaultLimit[0]
	}
	if p.Limit < 1 {
		p.Limit = fallback
	} else if p.Limit > MaxPaginationLimit {
		p.Limit = MaxPaginationLimit
	}
}

// Offset computes the database query offset for pagination.
func (p PaginationQuery) Offset() int {
	if p.Page <= 1 {
		return 0
	}
	return (p.Page - 1) * p.Limit
}

// PaginationMeta represents pagination details in list responses.
type PaginationMeta struct {
	CurrentPage int   `json:"current_page"`
	PageSize    int   `json:"page_size"`
	TotalItems  int64 `json:"total_items"`
	TotalPages  int   `json:"total_pages"`
}

// CalculateTotalPages computes the total number of pages using integer ceiling division.
func CalculateTotalPages(total int64, limit int) int {
	if limit <= 0 || total <= 0 {
		return 0
	}
	return int((total + int64(limit) - 1) / int64(limit))
}

// NewPaginationMeta constructs a PaginationMeta struct with automatically computed TotalPages.
func NewPaginationMeta(page, limit int, total int64) PaginationMeta {
	return PaginationMeta{
		CurrentPage: page,
		PageSize:    limit,
		TotalItems:  total,
		TotalPages:  CalculateTotalPages(total, limit),
	}
}
