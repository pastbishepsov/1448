package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CoinBurn — сгоревшие монеты неактивного гостя (миграция 029, В4-этап 3).
// Пишем каждое сгорание строкой, чтобы обязательства в отчёте не уменьшались
// «сами по себе»: владелец должен видеть, куда делись монеты.
type CoinBurn struct {
	ID           uuid.UUID `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID `json:"user_id"       gorm:"type:uuid;not null"`
	Coins        int64     `json:"coins"         gorm:"not null"`
	BalanceAfter int64     `json:"balance_after" gorm:"not null"`
	IdleDays     int       `json:"idle_days"     gorm:"not null"`
	Pct          int       `json:"pct"           gorm:"not null"`
	Manual       bool      `json:"manual"        gorm:"not null;default:false"`
	CreatedAt    time.Time `json:"created_at"`
}

func (b *CoinBurn) BeforeCreate(tx *gorm.DB) error {
	if b.ID == uuid.Nil {
		b.ID = uuid.New()
	}
	return nil
}

func (CoinBurn) TableName() string { return "coin_burns" }
