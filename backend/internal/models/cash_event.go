package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// CashEvent — движение ящика вне продаж (миграция 070, З8-и1): инкассация
// (collect — вынули в банк или сейф, AmountGrosz > 0) и коррекция владельца
// (correct — «в ящике сейчас N zł»: StatedGrosz = N, AmountGrosz — поправка к
// ожидаемому, ±). Отменённое событие (VoidedAt) ящик не трогает.
type CashEvent struct {
	ID          uuid.UUID  `json:"id"           gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID      uuid.UUID  `json:"club_id"      gorm:"type:uuid;not null"`
	Kind        string     `json:"kind"         gorm:"size:8;not null"`
	AmountGrosz int64      `json:"amount_grosz" gorm:"not null"`
	StatedGrosz *int64     `json:"stated_grosz,omitempty"`
	Dest        *string    `json:"dest,omitempty" gorm:"size:8"`
	Note        string     `json:"note"         gorm:"not null;default:''"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt   time.Time  `json:"created_at"`
	VoidedAt    *time.Time `json:"voided_at,omitempty"`
	VoidedBy    *uuid.UUID `json:"voided_by,omitempty"  gorm:"type:uuid"`
}

const (
	CashEventCollect = "collect" // инкассация
	CashEventCorrect = "correct" // коррекция владельца
)

func (e *CashEvent) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

func (CashEvent) TableName() string { return "cash_events" }
