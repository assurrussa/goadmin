package auth

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// Scan decodes the projection JSON through database/sql as well as native pgx.
func (p *ProfileData) Scan(src any) error {
	if p == nil {
		return errors.New("profile data: nil scan destination")
	}
	var data []byte
	switch value := src.(type) {
	case nil:
		*p = ProfileData{}
		return nil
	case []byte:
		data = value
	case string:
		data = []byte(value)
	default:
		return fmt.Errorf("profile data: unsupported scan type %T", src)
	}
	var decoded ProfileData
	if err := json.Unmarshal(data, &decoded); err != nil {
		return fmt.Errorf("profile data: decode JSON: %w", err)
	}
	*p = decoded
	return nil
}

// Value keeps projection writes compatible with database/sql JSONB parameters.
func (p ProfileData) Value() (driver.Value, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("profile data: encode JSON: %w", err)
	}
	return string(data), nil
}
