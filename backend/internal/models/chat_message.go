package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ChatMessage — чат гость ↔ админ (миграция 019, спринты Б2–Б3 ADMIN.md).
// Вызов админа — сообщение kind=call. UserID может быть пустым: вызов с ПК
// через агента без активной сессии знает только ComputerID.
type ChatMessage struct {
	ID         uuid.UUID  `json:"id"           gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID     *uuid.UUID `json:"user_id,omitempty"     gorm:"type:uuid"`
	ComputerID *uuid.UUID `json:"computer_id,omitempty" gorm:"type:uuid"`
	AdminID    *uuid.UUID `json:"admin_id,omitempty"    gorm:"type:uuid"`
	Sender     string     `json:"sender"       gorm:"size:8;not null"`  // guest | staff
	Kind       string     `json:"kind"         gorm:"size:8;not null;default:text"` // text | call
	Text       string     `json:"text"         gorm:"size:500;not null;default:''"`
	ReadStaff  bool       `json:"read_staff"   gorm:"not null;default:false"`
	ReadGuest  bool       `json:"read_guest"   gorm:"not null;default:false"`
	CreatedAt  time.Time  `json:"created_at"`
}

const (
	ChatSenderGuest = "guest"
	ChatSenderStaff = "staff"
	ChatKindText    = "text"
	ChatKindCall    = "call"
)

func (m *ChatMessage) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

func (ChatMessage) TableName() string { return "chat_messages" }
