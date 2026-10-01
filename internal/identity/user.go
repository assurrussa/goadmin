// Package identity owns goadmin projection identifiers. Canonical auth subject IDs
// and upload/event transport IDs remain separate types.
package identity

import (
	"database/sql/driver"
	"errors"

	"github.com/google/uuid"
)

var (
	ErrUserIDZero = errors.New("UserID uuid is zero")
	UserIDNil     = UserID(uuid.Nil)
)

type UserID uuid.UUID

func NewUserID() UserID                            { return UserID(uuid.New()) }
func (id UserID) String() string                   { return uuid.UUID(id).String() }
func (id UserID) Value() (driver.Value, error)     { return id.String(), nil }
func (id *UserID) Scan(src any) error              { return (*uuid.UUID)(id).Scan(src) }
func (id UserID) MarshalText() ([]byte, error)     { return uuid.UUID(id).MarshalText() }
func (id *UserID) UnmarshalText(data []byte) error { return (*uuid.UUID)(id).UnmarshalText(data) }
func (id UserID) IsZero() bool                     { return id == UserIDNil }
func (id UserID) Validate() error {
	if id.IsZero() {
		return ErrUserIDZero
	}
	return nil
}

func (id UserID) AsPointer() *UserID {
	if id.IsZero() {
		return nil
	}
	return &id
}
func (id UserID) Matches(value any) bool { other, ok := value.(UserID); return ok && id == other }
