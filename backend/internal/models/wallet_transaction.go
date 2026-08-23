package models

import (
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WalletTxKind — тип операции кошелька (enum wallet_tx_kind, миграция 031).
type WalletTxKind string

const (
	WalletTxDeposit      WalletTxKind = "deposit"       // пополнение (депозит у стойки, позже Stripe/BLIK)
	WalletTxSessionSpend WalletTxKind = "session_spend" // поминутное списание за время (Г1)
	WalletTxKitchenSpend WalletTxKind = "kitchen_spend" // заказ кухни с кошелька (Г7)
	WalletTxRefund       WalletTxKind = "refund"        // возврат (отмена заказа и т.п.)
	WalletTxAdjust       WalletTxKind = "adjust"        // ручная корректировка владельцем
)

// WalletTransaction — строка журнала денежного кошелька (трек Г, Г0-и1).
// Кошелёк — предоплаченные деньги гостя в грошах (Р1/Р2 GUEST.md): не тает,
// с монетами не смешивается. Меняется ТОЛЬКО через walletApply — каждая
// операция пишет сюда строку с балансом после неё. Инвариант: сумма
// amount_grosz по гостю всегда равна users.wallet_grosz.
type WalletTransaction struct {
	ID           uuid.UUID    `json:"id"             gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID       uuid.UUID    `json:"user_id"        gorm:"type:uuid;not null;index"`
	Kind         WalletTxKind `json:"kind"           gorm:"type:wallet_tx_kind;not null"`
	AmountGrosz  int64        `json:"amount_grosz"   gorm:"not null"` // со знаком: + пополнение, − списание
	BalanceAfter int64        `json:"balance_after"  gorm:"not null"`
	RefType      *string      `json:"ref_type,omitempty" gorm:"size:16"` // deposit | session | sale | manual
	RefID        *uuid.UUID   `json:"ref_id,omitempty"   gorm:"type:uuid"`
	Note         *string      `json:"note,omitempty"`
	CreatedBy    *uuid.UUID   `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt    time.Time    `json:"created_at"`
}

func (t *WalletTransaction) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// GroszFromPLN — злотые (ввод админа/Stripe) → гроши. Округление к ближайшему
// грошу: деньги считаем в целых, плавающая запятая живёт только на границе API.
func GroszFromPLN(pln float64) int64 {
	return int64(math.Round(pln * 100))
}

// PLNFromGrosz — гроши → злотые для показа и отчётов (ровно 2 знака).
func PLNFromGrosz(grosz int64) float64 {
	return float64(grosz) / 100
}
