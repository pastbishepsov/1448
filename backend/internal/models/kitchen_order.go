package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Статусы заказа кухни (Г7). Поток: new → accepted → preparing → delivering →
// done; отмена — до done (гость сам — только пока new).
const (
	KitchenNew        = "new"
	KitchenAccepted   = "accepted"
	KitchenPreparing  = "preparing"
	KitchenDelivering = "delivering"
	KitchenDone       = "done"
	KitchenCancelled  = "cancelled"
)

// Оплата заказа: кошельком сразу (Р7 — не выручка) или у стойки при выдаче.
const (
	KitchenPaidWallet  = "wallet"
	KitchenPaidCounter = "counter"
)

// KitchenOrder — заказ гостя на кухне (миграция 038): одна позиция × qty,
// как у продаж В2 («без корзины»). Склад резервируется при создании
// (stock_move reason='order'), отмена возвращает; выдача создаёт строку
// sales (wallet-заказ — method='wallet', отчёты его из выручки исключают).
type KitchenOrder struct {
	ID         uuid.UUID  `json:"id"          gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID     uuid.UUID  `json:"club_id"     gorm:"type:uuid;not null"`
	UserID     uuid.UUID  `json:"user_id"     gorm:"type:uuid;not null"`
	GoodID     *uuid.UUID `json:"good_id,omitempty" gorm:"type:uuid"`
	Name       string     `json:"name"        gorm:"size:64;not null"`
	Qty        int        `json:"qty"         gorm:"not null"`
	PricePLN   float64    `json:"price_pln"   gorm:"type:decimal(8,2);not null"`
	TotalPLN   float64    `json:"total_pln"   gorm:"type:decimal(8,2);not null"`
	Paid       string     `json:"paid"        gorm:"size:16;not null"`
	Status     string     `json:"status"      gorm:"size:16;not null;default:new"`
	ComputerID *uuid.UUID `json:"computer_id,omitempty" gorm:"type:uuid"`
	SaleID     *uuid.UUID `json:"sale_id,omitempty"     gorm:"type:uuid"`
	StatusAt   time.Time  `json:"status_at"`
	StatusBy   *uuid.UUID `json:"status_by,omitempty"   gorm:"type:uuid"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (o *KitchenOrder) BeforeCreate(tx *gorm.DB) error {
	if o.ID == uuid.Nil {
		o.ID = uuid.New()
	}
	return nil
}

func (KitchenOrder) TableName() string { return "kitchen_orders" }
