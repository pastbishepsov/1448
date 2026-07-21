package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Статусы места в очереди (миграция 022, спринт Б9).
const (
	WaitlistStatusWaiting = "waiting"
	WaitlistStatusSeated  = "seated"
	WaitlistStatusRemoved = "removed"
)

// WaitlistEntry — место в очереди, когда все ПК заняты (миграция 022, Б9).
// Только зарегистрированные гости (решение Б9); AddedBy — админ, поставивший
// у стойки, NULL — гость встал сам (задел под PWA). NotifiedAt защищает от
// спама уведомлением «ПК свободен» (шлётся голове очереди один раз).
type WaitlistEntry struct {
	ID         uuid.UUID  `json:"id"                 gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID     uuid.UUID  `json:"club_id"            gorm:"type:uuid;not null"`
	UserID     uuid.UUID  `json:"user_id"            gorm:"type:uuid;not null"`
	Status     string     `json:"status"             gorm:"size:16;not null;default:waiting"`
	AddedBy    *uuid.UUID `json:"added_by,omitempty" gorm:"type:uuid"`
	NotifiedAt *time.Time `json:"notified_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`

	User *User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

func (w *WaitlistEntry) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}

func (WaitlistEntry) TableName() string { return "waitlist" }
