package models

import (
	"time"
	"github.com/google/uuid"
)

type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusCancelled SessionStatus = "cancelled"
)

// Session — игровая сессия
type Session struct {
	ID         uuid.UUID     `json:"id" gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID     uuid.UUID     `json:"user_id" gorm:"type:uuid;not null;index"`
	ComputerID uuid.UUID     `json:"computer_id" gorm:"type:uuid;not null"`
	ClubID     uuid.UUID     `json:"club_id" gorm:"type:uuid;not null"`
	Status     SessionStatus `json:"status" gorm:"type:session_status;default:active"`

	StartedAt   time.Time  `json:"started_at" gorm:"not null"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	MinutesTotal int       `json:"minutes_total" gorm:"not null"`
	MinutesUsed  int       `json:"minutes_used" gorm:"default:0"`

	BaseRatePLN      float64 `json:"base_rate_pln" gorm:"type:decimal(8,2);not null"`
	EffectiveRatePLN float64 `json:"effective_rate_pln" gorm:"type:decimal(8,2);not null"`

	XPEarned    int64 `json:"xp_earned" gorm:"default:0"`
	CoinsEarned int64 `json:"coins_earned" gorm:"default:0"`

	CreatedAt time.Time `json:"created_at"`

	// Связи
	User     User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Computer Computer `json:"computer,omitempty" gorm:"foreignKey:ComputerID"`
}
