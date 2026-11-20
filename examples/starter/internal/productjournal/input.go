// Package productjournal defines the starter's journal tool inputs.
package productjournal

import (
	"math"
	"strconv"
)

type SearchInput struct {
	ProductID string `json:"product_id" jsonschema:"Exact positive product number as a decimal string."`
	Action    string `json:"action,omitempty" jsonschema:"Optional action such as unpublish."`
	Page      *int   `json:"page,omitempty" jsonschema:"Page number, starting at 1 (default 1)."`
	PageSize  *int   `json:"page_size,omitempty" jsonschema:"Rows per page, 1 through 100 (default 25)."`
}

func (in SearchInput) Pagination() (page, size int) {
	page, size = 1, 25
	if in.Page != nil {
		page = *in.Page
	}
	if in.PageSize != nil {
		size = *in.PageSize
	}
	return page, size
}

func (in SearchInput) Valid() bool {
	page, size := in.Pagination()
	return validID(in.ProductID) && size >= 1 && size <= 100 && page >= 1 && page <= math.MaxInt/size
}

type GetInput struct {
	EventID string `json:"event_id" jsonschema:"The event id returned by search_product_events."`
}

func (in GetInput) Valid() bool { return validID(in.EventID) }

func validID(id string) bool {
	n, err := strconv.ParseInt(id, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == id
}
