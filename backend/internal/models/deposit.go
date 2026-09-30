package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Deposit — пополнение баланса (миграция 009).
//
// Ж1 (миграция 062): отменённый депозит не удаляется — VoidedAt выводит его
// из выручки, RefundGrosz говорит, сколько реально удалось снять с кошелька
// (остаток, но не больше суммы), CoinsTaken — сколько кэшбек-монет забрано.
type Deposit struct {
	ID           uuid.UUID  `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID  `json:"user_id"       gorm:"type:uuid;not null;index"`
	AmountPLN    float64    `json:"amount_pln"    gorm:"type:decimal(8,2);not null"`
	CoinsGranted int64      `json:"coins_granted" gorm:"not null"`
	BonusCoins   int64      `json:"bonus_coins"   gorm:"not null;default:0"`
	Method       string     `json:"method"        gorm:"size:16;default:cash"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt    time.Time  `json:"created_at"`
	VoidedAt     *time.Time `json:"voided_at,omitempty"`
	VoidedBy     *uuid.UUID `json:"voided_by,omitempty" gorm:"type:uuid"`
	RefundGrosz  int64      `json:"refund_grosz"  gorm:"not null;default:0"`
	CoinsTaken   int64      `json:"coins_taken"   gorm:"not null;default:0"`
	// З11-и1 (миграция 079): как вернули деньги отменённого депозита — cash |
	// card | blik | none; ящик уменьшает только cash.
	RefundMethod *string `json:"refund_method,omitempty" gorm:"size:8"`
}

func (d *Deposit) BeforeCreate(tx *gorm.DB) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	return nil
}
