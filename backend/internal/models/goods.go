package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Good — позиция ценника клуба (миграция 024, В2): цена и остаток.
// Себестоимости нет осознанно — решение основателя 2026-08-18.
// Г7 (038): описание и фото — карточка позиции для гостевой кухни.
type Good struct {
	ID          uuid.UUID  `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID      uuid.UUID  `json:"club_id"    gorm:"type:uuid;not null"`
	Name        string     `json:"name"       gorm:"size:64;not null"`
	Category    string     `json:"category"   gorm:"size:32;not null;default:''"`
	Description string     `json:"description"          gorm:"not null;default:''"`
	PricePLN    float64    `json:"price_pln"  gorm:"type:decimal(8,2);not null"`
	Stock       int        `json:"stock"      gorm:"not null;default:0"`
	LowStock    int        `json:"low_stock"  gorm:"not null;default:0"`
	Sort        int        `json:"sort"       gorm:"not null;default:0"`
	Active      bool       `json:"active"     gorm:"not null;default:true"`
	PhotoType   *string    `json:"-"          gorm:"size:32"` // MIME фото; байты НЕ в модели —
	PhotoAt     *time.Time `json:"-"`                         // читаются/пишутся только raw-SQL (kitchen.go)
	CreatedAt   time.Time  `json:"created_at"`
}

func (g *Good) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

func (Good) TableName() string { return "goods" }

// Sale — продажа позиции за злотые. Отменённая продажа не удаляется:
// voided_at выводит её из выручки, но след в журнале остаётся.
type Sale struct {
	ID        uuid.UUID  `json:"id"        gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID  `json:"club_id"   gorm:"type:uuid;not null"`
	GoodID    *uuid.UUID `json:"good_id,omitempty" gorm:"type:uuid"`
	Name      string     `json:"name"      gorm:"size:64;not null"`
	Qty       int        `json:"qty"       gorm:"not null"`
	PricePLN  float64    `json:"price_pln" gorm:"type:decimal(8,2);not null"`
	TotalPLN  float64    `json:"total_pln" gorm:"type:decimal(8,2);not null"`
	Method    string     `json:"method"    gorm:"size:16;not null;default:cash"`
	UserID    *uuid.UUID `json:"user_id,omitempty"   gorm:"type:uuid"`
	CreatedBy uuid.UUID  `json:"created_by"          gorm:"type:uuid;not null"`
	VoidedAt  *time.Time `json:"voided_at,omitempty"`
	VoidedBy  *uuid.UUID `json:"voided_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

func (s *Sale) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (Sale) TableName() string { return "sales" }

// StockMove — движение склада. reason: supply (приход), sale (продажа),
// void (возврат при отмене), adjust (ручная корректировка с причиной).
type StockMove struct {
	ID        uuid.UUID  `json:"id"      gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID  `json:"club_id" gorm:"type:uuid;not null"`
	GoodID    uuid.UUID  `json:"good_id" gorm:"type:uuid;not null"`
	Delta     int        `json:"delta"   gorm:"not null"`
	Reason    string     `json:"reason"  gorm:"size:16;not null"`
	Note      string     `json:"note"    gorm:"not null;default:''"`
	SaleID    *uuid.UUID `json:"sale_id,omitempty"    gorm:"type:uuid"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

func (m *StockMove) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

func (StockMove) TableName() string { return "stock_moves" }
