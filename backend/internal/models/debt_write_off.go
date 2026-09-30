package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// DebtWriteOff — списание долга гостя (З11-и4, миграция 080; решения основателя 30.09): весь открытый долг —
// бар «в долг» и выданные заказы кухни «при выходе» — одной строкой, без причины; у списанных продаж и
// заказов — ссылка WriteOffID. Склад не трогается, выручки нет, сумма — «Потери → долги» по дню списания.
// CancelledAt — владелец списание отменил: ссылки сняты, долг снова открыт.
type DebtWriteOff struct {
	ID           uuid.UUID  `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID  `json:"user_id"       gorm:"type:uuid;not null"`
	KitchenCount int        `json:"kitchen_count" gorm:"not null;default:0"`
	KitchenGrosz int64      `json:"kitchen_grosz" gorm:"not null;default:0"`
	BarCount     int        `json:"bar_count"     gorm:"not null;default:0"`
	BarGrosz     int64      `json:"bar_grosz"     gorm:"not null;default:0"`
	TotalGrosz   int64      `json:"total_grosz"   gorm:"not null;default:0"`
	CreatedBy    *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt    time.Time  `json:"created_at"`
	CancelledAt  *time.Time `json:"cancelled_at,omitempty"`
	CancelledBy  *uuid.UUID `json:"cancelled_by,omitempty" gorm:"type:uuid"`
}

func (w *DebtWriteOff) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}

func (DebtWriteOff) TableName() string { return "debt_write_offs" }
