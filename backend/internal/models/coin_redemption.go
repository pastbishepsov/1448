package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CoinRedemption — монеты, погашенные временем за ПК (миграция 028, В4-этап 2).
// Единственное место, где баланс монет уменьшается: до этого спринта монеты
// только копились. Цена часа и имя зоны снимаются на момент погашения, чтобы
// история не поехала при смене тарифа или переименовании зоны.
type CoinRedemption struct {
	ID        uuid.UUID  `json:"id"        gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID  `json:"club_id"   gorm:"type:uuid;not null"`
	UserID    uuid.UUID  `json:"user_id"   gorm:"type:uuid;not null"`
	ZoneID    *uuid.UUID `json:"zone_id,omitempty" gorm:"type:uuid"`
	ZoneName  string     `json:"zone_name" gorm:"size:32;not null;default:''"`
	Minutes   int        `json:"minutes"   gorm:"not null"`
	Coins     int64      `json:"coins"     gorm:"not null"`
	RatePLN   float64    `json:"rate_pln"  gorm:"type:decimal(8,2);not null"`
	ValuePLN  float64    `json:"value_pln" gorm:"type:decimal(8,2);not null"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

func (r *CoinRedemption) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

func (CoinRedemption) TableName() string { return "coin_redemptions" }
