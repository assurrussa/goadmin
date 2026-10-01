package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"

	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
	"github.com/assurrussa/goadmin/internal/pointer"
)

type Admin struct {
	ID               int64              `json:"id" db:"id"`
	SubjectID        authcore.SubjectID `json:"-" db:"subject_id"`
	UUID             identity.UserID    `json:"uuid" db:"uuid"`
	Name             string             `json:"name" db:"name"`
	LastName         string             `json:"lastName" db:"last_name"`
	Username         string             `json:"username" db:"username"`
	Email            string             `json:"email" db:"email"`
	ConfirmedEmailAt *time.Time         `json:"confirmedEmailAt,omitempty" db:"confirmed_email_at"`
	Phone            *int64             `json:"phone" db:"phone"`
	Version          int                `json:"version" db:"version"`
	Data             *AdminData         `json:"data,omitempty" db:"data,omitempty"`
	CreatedAt        time.Time          `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time          `json:"updatedAt" db:"updated_at"`
	DeletedAt        sql.NullTime       `json:"deletedAt,omitempty" db:"deleted_at"`
}

func (a Admin) AuthSubjectID() authcore.SubjectID {
	if !a.SubjectID.IsZero() {
		return a.SubjectID
	}

	return authcore.SubjectID{}
}

type AdminData struct {
	Roles         []string            `json:"roles,omitempty"`
	Permissions   map[string][]string `json:"permissions,omitempty"`
	PreviewFileID *int64              `json:"previewFileId,omitempty"`
	LastLoginAt   *time.Time          `json:"lastLoginAt,omitempty"`
}

// Value Make the Attrs struct implement the driver.Valuer interface. This method
// simply returns the JSON-encoded representation of the struct.
func (a *AdminData) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// Scan Make the Attrs struct implement the sql.Scanner interface. This method
// simply decodes a JSON-encoded value into the struct fields.
func (a *AdminData) Scan(value any) error {
	var b []byte
	switch v := value.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return errors.New("type assertion to []byte or string failed")
	}

	return json.Unmarshal(b, &a)
}

func (a *Admin) GetRoles() []string {
	if a.Data != nil && len(a.Data.Roles) > 0 {
		return a.Data.Roles
	}

	return nil
}

func (a *Admin) GetPermissions() map[string][]string {
	if a.Data != nil {
		return a.Data.Permissions
	}

	return nil
}

func (a *Admin) GetPreviewFileID() int64 {
	if a.Data != nil {
		return pointer.Indirect(a.Data.PreviewFileID)
	}

	return 0
}

func (a *Admin) GetLastLoginAt() *time.Time {
	if a == nil || a.Data == nil || a.Data.LastLoginAt == nil {
		return nil
	}

	loggedAt := a.Data.LastLoginAt.UTC()
	return &loggedAt
}

func (a *Admin) GetEmailConfirmedAt() *time.Time {
	if a == nil {
		return nil
	}
	if a.ConfirmedEmailAt != nil {
		confirmedAt := a.ConfirmedEmailAt.UTC()
		return &confirmedAt
	}

	return nil
}

func (a *Admin) GetData() *AdminData {
	if a.Data != nil {
		return a.Data
	}

	return &AdminData{}
}
