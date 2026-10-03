package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// PackSpend — одно списание минут пакета биллингом (З12-и2, миграция 082): с какого пакета, в какой сессии, сколько
// минут, когда и сколько это стоит — доля цены пакета за эти минуты (подаренный пакет — 0). По этим строкам
// считается «Отработанная выручка» пакетов за период.
type PackSpend struct {
	ID            uuid.UUID  `json:"id"              gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserPackageID uuid.UUID  `json:"user_package_id" gorm:"type:uuid;not null"`
	SessionID     *uuid.UUID `json:"session_id,omitempty" gorm:"type:uuid"`
	Minutes       int        `json:"minutes"         gorm:"not null"`
	Grosz         int64      `json:"grosz"           gorm:"not null"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (p *PackSpend) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (PackSpend) TableName() string { return "pack_spends" }
