package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Виды трат монет на клубные плюшки (миграция 042).
const CoinSpendStreakFreeze = "streak_freeze"

// CoinSpend — списание монет на плюшку клуба (Г6-и4). Второе место после
// coin_redemptions, где баланс монет уменьшается по воле гостя, и третье
// вместе с coin_burns, где уменьшается обязательство клуба. Каждая трата —
// строка: иначе отчёт эмиссии (В4-2) не сойдётся с балансами на руках.
type CoinSpend struct {
	ID           uuid.UUID `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID `json:"user_id"       gorm:"type:uuid;not null"`
	Kind         string    `json:"kind"          gorm:"size:24;not null"`
	Coins        int64     `json:"coins"         gorm:"not null"`
	BalanceAfter int64     `json:"balance_after" gorm:"not null"`
	Note         string    `json:"note"          gorm:"not null;default:''"`
	CreatedAt    time.Time `json:"created_at"`
}

func (s *CoinSpend) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (CoinSpend) TableName() string { return "coin_spends" }
