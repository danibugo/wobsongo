// Package dto provides data transfer objects for API requests and responses.
package dto

const (
	maxItemsPerPage     int32 = 100
	defaultItemsPerPage int32 = 20
)

// PaginationDTO represents pagination parameters for API requests.
type PaginationDTO struct {
	// Page number, starting from 1.
	Page int32 `query:"page" validate:"omitempty,min=1" example:"1"`

	// PerPage defines the number of items per page, with a maximum limit of 100.
	PerPage int32 `query:"per_page" validate:"omitempty,min=1,max=100" example:"20"`
}

// getPage returns the current page number, defaulting to 1 if not set.
func (d *PaginationDTO) getPage() int32 {
	return max(d.Page, 1)
}

// GetPage returns the current page number, defaulting to 1 if not set.
func (d *PaginationDTO) GetPage() int32 {
	return d.getPage()
}

// Limit returns the number of items per page, defaulting to 20 if not set.
func (d *PaginationDTO) Limit() int32 {
	if d.PerPage <= 0 {
		return defaultItemsPerPage
	}
	return min(d.PerPage, maxItemsPerPage)
}

// Offset calculates the offset for database queries
// based on the current page and limit.
func (d *PaginationDTO) Offset() int32 {
	return (d.getPage() - 1) * d.Limit()
}

// PaginationResults represents the pagination information in API responses.
type PaginationResults[T any] struct {
	// Page is the current page number
	Page int `json:"page" validate:"required"`

	// PerPage is the number of items per page
	PerPage int `json:"per_page" validate:"required,max=100"`

	// TotalItems is the total number of items available
	TotalItems int `json:"total_items" validate:"required"`

	// Items contains the paginated data items
	Items []T `json:"items"`
}
