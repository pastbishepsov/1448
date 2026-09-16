package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Источники зонных минут (minute_grants.source).
const (
	MinuteGrantRedeem       = "redeem"       // выдача за монеты (ref_id → coin_redemptions.id)
	MinuteGrantCompensation = "compensation" // компенсация по проблеме с ПК, К8 (ref_id → incidents.id)
)

// MinuteGrant — выданные гостю минуты, привязанные к зоне (миграция 063,
// Ж2 / Ж-Р3). Строка на каждую выдачу — как UserPackage: зона и её имя —
// снимок на момент выдачи, остаток уменьшает биллинг. Тратятся только в
// своей зоне; ZoneID nil — «без зоны», годны везде. Старый пул
// users.coin_minutes остаётся и дотрачивается без зоны.
type MinuteGrant struct {
	ID           uuid.UUID  `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID  `json:"user_id"       gorm:"type:uuid;not null"`
	ZoneID       *uuid.UUID `json:"zone_id,omitempty" gorm:"type:uuid"`
	ZoneName     string     `json:"zone_name"     gorm:"size:32;not null;default:''"`
	MinutesTotal int        `json:"minutes_total" gorm:"not null"`
	MinutesLeft  int        `json:"minutes_left"  gorm:"not null"`
	Source       string     `json:"source"        gorm:"size:16;not null"`
	RefID        *uuid.UUID `json:"ref_id,omitempty" gorm:"type:uuid"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (g *MinuteGrant) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

func (MinuteGrant) TableName() string { return "minute_grants" }
