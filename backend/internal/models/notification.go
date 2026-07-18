package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Notification — уведомление гостю о действии админа (миграция 020, Б4).
// Payload — JSON-строка (jsonb в БД); гостевой экран получает её объектом.
type Notification struct {
	ID        uuid.UUID  `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID    uuid.UUID  `json:"user_id"    gorm:"type:uuid;not null"`
	Type      string     `json:"type"       gorm:"size:32;not null"`
	Payload   string     `json:"-"          gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time  `json:"created_at"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
}

func (n *Notification) BeforeCreate(tx *gorm.DB) error {
	if n.ID == uuid.Nil {
		n.ID = uuid.New()
	}
	return nil
}

func (Notification) TableName() string { return "notifications" }
