package models

import (
	"database/sql"
	"time"

	authcore "github.com/assurrussa/goadmin/internal/auth"
	identity "github.com/assurrussa/goadmin/internal/identity"
)

type SessionAdmin struct {
	ExternalValidUntil time.Time           `json:"-"`
	ID                 int64               `json:"id"`
	SubjectID          authcore.SubjectID  `json:"subjectId"`
	UUID               identity.UserID     `json:"uuid"`
	Username           string              `json:"username,omitempty"`
	Name               string              `json:"name"`
	LastName           string              `json:"lastName,omitempty"`
	Email              string              `json:"email,omitempty"`
	Roles              []string            `json:"roles,omitempty"`
	RoleSlugs          []string            `json:"roleSlugs,omitempty"`
	Permissions        map[string][]string `json:"permissions,omitempty"`
	PreviewID          int64               `json:"previewId,omitempty"`
	SessionID          string              `json:"-"`
	CreatedAt          time.Time           `json:"createdAt,omitempty"`
	UpdatedAt          time.Time           `json:"updatedAt,omitempty"`
	DeletedAt          sql.NullTime        `json:"deletedAt,omitempty"`
}

func (a *SessionAdmin) GetUUID() identity.UserID {
	if a == nil {
		return identity.UserIDNil
	}
	return a.UUID
}

func (a *SessionAdmin) AuthSubjectID() authcore.SubjectID {
	if a == nil {
		return authcore.SubjectID{}
	}
	if !a.SubjectID.IsZero() {
		return a.SubjectID
	}

	return authcore.SubjectID{}
}

func (a *SessionAdmin) HasRole(roles ...string) bool {
	if a == nil {
		return false
	}

	for _, r := range a.RoleSlugs {
		for _, x := range roles {
			if r == x {
				return true
			}
		}
	}

	return false
}
