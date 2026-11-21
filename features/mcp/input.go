package mcp

import (
	"bytes"
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/assurrussa/goadmin/toolkit/datagrid"
)

const (
	defaultPageSize = 25
	maxPageSize     = 100
	maxRequestSize  = 4096
)

func (s Search) filters(in map[string]json.RawMessage) (datagrid.Filters, bool) {
	page, ok := paginationValue(in["page"], 1)
	if !ok {
		return datagrid.Filters{}, false
	}
	size, ok := paginationValue(in["page_size"], defaultPageSize)
	if !ok || size > maxPageSize || page > math.MaxInt/size {
		return datagrid.Filters{}, false
	}
	filters := datagrid.Filters{Page: page, Limit: size, Fields: map[string]any{}}
	count := 0
	for _, key := range []string{"page", "page_size"} {
		if _, exists := in[key]; exists {
			count++
		}
	}
	if s.Query != nil {
		raw, exists := in[s.Query.Name]
		value, ok := s.Query.parse(raw)
		if !ok {
			return datagrid.Filters{}, false
		}
		if exists {
			count++
			filters.Search, _ = value.(string)
		}
	}
	for _, field := range s.Filters {
		raw, exists := in[field.Name]
		value, ok := field.parse(raw)
		if !ok {
			return datagrid.Filters{}, false
		}
		if exists {
			count++
			if value != "" || field.Required {
				filters.Fields[field.FilterKey] = value
			}
		}
	}
	return filters, count == len(in)
}

func paginationValue(raw json.RawMessage, fallback int) (int, bool) {
	if len(raw) == 0 {
		return fallback, true
	}
	var value int
	if json.Unmarshal(raw, &value) != nil || value < 1 {
		return 0, false
	}
	return value, true
}

func (d Detail) recordID(in map[string]json.RawMessage) (string, bool) {
	value, ok := d.ID.parse(in[d.ID.Name])
	if !ok || len(in) != 1 {
		return "", false
	}
	id, ok := value.(string)
	return id, ok && validRecordID(id)
}

func (p Parameter) parse(raw json.RawMessage) (any, bool) {
	if len(raw) == 0 {
		return nil, !p.Required
	}
	value, ok := p.Kind.parse(raw)
	if !ok || len(p.Enum) == 0 {
		return value, ok
	}
	option, isText := value.(string)
	return value, isText && slices.Contains(p.Enum, option)
}

func (k Kind) parse(raw json.RawMessage) (any, bool) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, false
	}
	switch k {
	case Text, PositiveID:
		var value string
		if json.Unmarshal(raw, &value) != nil {
			return nil, false
		}
		return value, k != PositiveID || validPositiveID(value)
	case Boolean:
		var value bool
		err := json.Unmarshal(raw, &value)
		return value, err == nil
	case Integer:
		var value int64
		err := json.Unmarshal(raw, &value)
		return value, err == nil
	case Number:
		var value float64
		err := json.Unmarshal(raw, &value)
		return value, err == nil
	default:
		return nil, false
	}
}

func validRecordID(id string) bool {
	return id != "" && id != "." && id != ".." && len(id) <= 255 &&
		strings.TrimSpace(id) == id && !strings.ContainsAny(id, ",\r\n\t\x00")
}

func validPositiveID(value string) bool {
	id, err := strconv.ParseInt(value, 10, 64)
	return err == nil && id > 0 && strconv.FormatInt(id, 10) == value
}
