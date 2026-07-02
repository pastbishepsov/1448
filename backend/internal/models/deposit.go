package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Deposit — пополнение баланса (миграция 009).
type Deposit struct {
	ID           uuid.UUID  `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID  `json:"user_id"       gorm:"type:uuid;not null;index"`
	AmountPLN    float64    `json:"amount_pln"    gorm:"type:decimal(8,2);not null"`
	CoinsGranted int64      `json:"coins_granted" gorm:"not null"`
	BonusCoins   int64      `json:"bonus_coins"   gorm:"not null;default:0"`
	Method       string     `json:"method"        gorm:"size:16;default:cash"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (d *Deposit) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}
