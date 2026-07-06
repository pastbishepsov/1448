package models

import (
	"time"

	"github.com/google/uuid"
)

// SensitivityProfile — профиль сенсы игрока (миграция 013).
// Games — JSONB-строка вида {"cs2":2.0,"valorant":0.4}; парсинг в handler.
type SensitivityProfile struct {
	UserID    uuid.UUID `json:"user_id"    gorm:"type:uuid;primaryKey"`
	DPI       int       `json:"dpi"        gorm:"not null;default:800"`
	Games     string    `json:"-"          gorm:"type:jsonb;not null;default:'{}'"`
	UpdatedAt time.Time `json:"updated_at" gorm:"autoUpdateTime"`
}

func (SensitivityProfile) TableName() string { return "sensitivity_profiles" }
