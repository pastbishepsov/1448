package models

import (
	"crypto/rand"
	"math/big"
	"time"

	"github.com/google/uuid"
)

type CaseTier   string
type CaseSource string
type DropType   string

const (
	CaseTierLight  CaseTier = "light"
	CaseTierMedium CaseTier = "medium"
	CaseTierHeavy  CaseTier = "heavy"
	CaseTierTitan  CaseTier = "titan"
	CaseTierGods   CaseTier = "gods"

	CaseSourceAchievement CaseSource = "achievement"
	CaseSourceDailyVisit  CaseSource = "daily_visit"
	CaseSourceDeposit     CaseSource = "deposit"
	CaseSourceLevelUp     CaseSource = "level_up"
	CaseSourceAdminGrant  CaseSource = "admin_grant"

	DropTypeCoins  DropType = "coins"
	DropTypeBuster DropType = "buster"
)

// Case — кейс игрока
type Case struct {
	ID       uuid.UUID  `json:"id"        gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID   uuid.UUID  `json:"user_id"   gorm:"type:uuid;not null;index"`
	ClubID   *uuid.UUID `json:"club_id"   gorm:"type:uuid"`
	Tier     CaseTier   `json:"tier"      gorm:"type:case_tier;not null"`
	Source   CaseSource `json:"source"    gorm:"type:case_source;not null"`

	OpenedAt  *time.Time `json:"opened_at"  gorm:"index"`
	ExpiresAt time.Time  `json:"expires_at" gorm:"not null"`

	DropType   *DropType `json:"drop_type"   gorm:"type:drop_type"`
	DropAmount *int64    `json:"drop_amount"`

	CreatedAt time.Time `json:"created_at"`
}

// CaseDropConfig — таблица дропа по градациям
type CaseDropConfig struct {
	CoinsMin      int64
	CoinsMax      int64
	JackpotChance int64 // из 100000 (10000 = 10%)
	JackpotAmount int64
	BusterChance  int64 // из 100000
	BusterAmount  int64 // в сотых процента (100 = 1%)
}

var dropConfigs = map[CaseTier]CaseDropConfig{
	CaseTierLight:  {CoinsMin: 50,   CoinsMax: 200,   JackpotChance: 100,  JackpotAmount: 50000, BusterChance: 5000,  BusterAmount: 100},
	CaseTierMedium: {CoinsMin: 200,  CoinsMax: 600,   JackpotChance: 500,  JackpotAmount: 50000, BusterChance: 10000, BusterAmount: 150},
	CaseTierHeavy:  {CoinsMin: 500,  CoinsMax: 2000,  JackpotChance: 1000, JackpotAmount: 50000, BusterChance: 20000, BusterAmount: 200},
	CaseTierTitan:  {CoinsMin: 1000, CoinsMax: 5000,  JackpotChance: 3000, JackpotAmount: 50000, BusterChance: 40000, BusterAmount: 300},
	CaseTierGods:   {CoinsMin: 5000, CoinsMax: 20000, JackpotChance: 10000,JackpotAmount: 50000, BusterChance: 50000, BusterAmount: 500},
}

// Roll — генерирует дроп кейса.
// ВАЖНО: вызывается ТОЛЬКО на сервере, никогда на клиенте.
// Использует crypto/rand для криптографически стойкой случайности.
func (c *Case) Roll(rtpModifier float64) (DropType, int64, error) {
	config := dropConfigs[c.Tier]

	roll, err := cryptoRandInt(100000)
	if err != nil {
		return DropTypeCoins, 0, err
	}

	jackpotThreshold := int64(float64(config.JackpotChance) * rtpModifier)
	busterThreshold := jackpotThreshold + int64(float64(config.BusterChance)*rtpModifier)

	if roll < jackpotThreshold {
		return DropTypeCoins, config.JackpotAmount, nil
	}
	if roll < busterThreshold {
		return DropTypeBuster, config.BusterAmount, nil
	}

	rangeCoins := config.CoinsMax - config.CoinsMin
	extra, err := cryptoRandInt(rangeCoins)
	if err != nil {
		return DropTypeCoins, config.CoinsMin, err
	}
	return DropTypeCoins, config.CoinsMin + extra, nil
}

// cryptoRandInt — безопасное случайное число в диапазоне [0, max)
func cryptoRandInt(max int64) (int64, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(max))
	if err != nil {
		return 0, err
	}
	return n.Int64(), nil
}
