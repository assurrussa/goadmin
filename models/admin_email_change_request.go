package models

import "time"

// AdminEmailChangeRequest stores pending email change confirmation for an admin.
type AdminEmailChangeRequest struct {
	ID        int64     `json:"id" db:"id"`
	AdminID   int64     `json:"adminId" db:"admin_id"`
	OldEmail  string    `json:"oldEmail" db:"old_email"`
	NewEmail  string    `json:"newEmail" db:"new_email"`
	Code      string    `json:"-" db:"code"`
	Attempts  int       `json:"attempts" db:"attempts"`
	ExpiresAt time.Time `json:"expiresAt" db:"expires_at"`
	CreatedAt time.Time `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time `json:"updatedAt" db:"updated_at"`
}
