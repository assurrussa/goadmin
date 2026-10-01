package auth

import (
	"database/sql"
	"time"

	"github.com/assurrussa/goauth"

	identity "github.com/assurrussa/goadmin/internal/identity"
)

// The types in this file are goadmin-owned projection/session DTOs retained
// while the host handlers move to Runtime. They are deliberately not aliases
// of goauth.Subject: canonical v0.2 subjects do not contain profile, password,
// host IDs, roles, or permissions.
type SubjectID = goauth.SubjectID

var (
	SubjectIDNil        = goauth.NilSubjectID
	ErrInvalidSubjectID = goauth.ErrInvalidSubjectID
)

func NewSubjectID() SubjectID                              { return goauth.NewSubjectID() }
func ParseSubjectIDString(value string) (SubjectID, error) { return goauth.ParseSubjectID(value) }
func MustParseSubjectIDString(value string) SubjectID      { return goauth.MustParseSubjectID(value) }

type ProfileData struct {
	Role        string              `json:"role,omitempty"`
	Permissions map[string][]string `json:"permissions,omitempty"`
}

type Profile struct {
	ID               int64           `json:"id" db:"id"`
	SubjectID        SubjectID       `json:"-" db:"subject_id"`
	PublicID         identity.UserID `json:"uuid" db:"uuid"`
	Email            string          `json:"email" db:"email"`
	Username         *string         `json:"username" db:"username"`
	Name             string          `json:"name" db:"name"`
	LastName         *string         `json:"lastName" db:"last_name"`
	FatherName       *string         `json:"fatherName" db:"father_name"`
	Bio              *string         `json:"bio" db:"bio"`
	Gender           *int            `json:"gender" db:"gender"`
	Birthday         sql.NullTime    `json:"birthday" db:"birthday"`
	Phone            *int64          `json:"phone" db:"phone"`
	Version          int64           `json:"version" db:"version"`
	TelegramChatID   sql.NullInt64   `json:"telegramChatId,omitempty" db:"telegram_chat_id"`
	TelegramUsername sql.NullString  `json:"telegramUsername,omitempty" db:"telegram_username"`
	Data             *ProfileData    `json:"data,omitempty" db:"data,omitempty"`
	ConfirmedEmailAt sql.NullTime    `json:"confirmedEmailAt" db:"confirmed_email_at"`
	ConfirmedPhoneAt sql.NullTime    `json:"confirmedPhoneAt" db:"confirmed_phone_at"`
	CreatedAt        time.Time       `json:"createdAt" db:"created_at"`
	UpdatedAt        time.Time       `json:"updatedAt" db:"updated_at"`
	DeletedAt        sql.NullTime    `json:"deletedAt,omitempty" db:"deleted_at"`
}

func (p *Profile) GetRole() string {
	if p == nil || p.Data == nil {
		return ""
	}

	return p.Data.Role
}

func (p *Profile) GetPermissions() map[string][]string {
	if p == nil || p.Data == nil {
		return nil
	}

	return p.Data.Permissions
}

func (p Profile) IsEmailConfirmed() bool   { return p.ConfirmedEmailAt.Valid }
func (p Profile) IsPhoneConfirmed() bool   { return p.ConfirmedPhoneAt.Valid }
func (p Profile) AuthSubjectID() SubjectID { return p.SubjectID }
