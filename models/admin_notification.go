package models

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// NotificationPayload хранит произвольные данные уведомления (CTA, ссылки, системные id).
type NotificationPayload map[string]any

func (p NotificationPayload) clone() NotificationPayload {
	if p == nil {
		return NotificationPayload{}
	}

	cp := make(NotificationPayload, len(p))
	for k, v := range p {
		cp[k] = v
	}

	return cp
}

// Value реализует driver.Valuer для сохранения JSON payload.
func (p NotificationPayload) Value() (driver.Value, error) {
	if p == nil {
		return []byte("{}"), nil
	}

	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// Scan реализует sql.Scanner для чтения JSON payload.
func (p *NotificationPayload) Scan(value any) error {
	if value == nil {
		*p = NotificationPayload{}
		return nil
	}

	var b []byte
	switch v := value.(type) {
	case string:
		b = []byte(v)
	case []byte:
		b = v
	default:
		return errors.New("type assertion to []byte or string failed")
	}

	if len(b) == 0 {
		*p = NotificationPayload{}
		return nil
	}

	var data NotificationPayload
	if err := json.Unmarshal(b, &data); err != nil {
		return err
	}

	*p = data
	return nil
}

// AdminNotification описывает уведомление для администратора.
type AdminNotification struct {
	ID        int64               `json:"id" db:"id"`
	AdminID   int64               `json:"adminId" db:"admin_id"`
	Title     string              `json:"title" db:"title"`
	Message   string              `json:"message" db:"message"`
	Level     string              `json:"level" db:"level"`
	Payload   NotificationPayload `json:"payload" db:"payload"`
	IsRead    bool                `json:"isRead" db:"is_read"`
	ReadAt    sql.NullTime        `json:"readAt" db:"read_at"`
	CreatedAt time.Time           `json:"createdAt" db:"created_at"`
	UpdatedAt time.Time           `json:"updatedAt" db:"updated_at"`
}

// GetPayload возвращает payload (никогда не nil) для удобной сериализации.
func (n *AdminNotification) GetPayload() NotificationPayload {
	if n.Payload == nil {
		return NotificationPayload{}
	}

	return n.Payload.clone()
}
