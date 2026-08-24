package models

import (
	"time"

	"github.com/google/uuid"
)

// UserProgress — счётчики гостя за одни ачивочные сутки (14:48→14:48 клуба,
// миграция 036, трек Г/Г5). Топливо периодических достижений и стриков.
type UserProgress struct {
	UserID         uuid.UUID  `json:"user_id"  gorm:"type:uuid;primaryKey"`
	DayKey         string     `json:"day_key"  gorm:"size:10;primaryKey"`
	Minutes        int        `json:"minutes"          gorm:"default:0"`
	ActiveMinutes  int        `json:"active_minutes"   gorm:"default:0"`
	Sessions       int        `json:"sessions"         gorm:"default:0"`
	KitchenOrders  int        `json:"kitchen_orders"   gorm:"default:0"`
	NightSessions  int        `json:"night_sessions"   gorm:"default:0"` // Г6: сессии, начатые ночью (22:00–07:59 клуба)
	FirstSessionAt *time.Time `json:"first_session_at,omitempty"`
}

func (UserProgress) TableName() string { return "user_progress" }
