package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// З10-и2 (миграция 076; OWNER.md, трек З, №68–№70, №78): партии склада. Поставка
// приходит строками «товар × штук» (партии без цены), владелец её оценивает — у
// строки одна или несколько цен; каждое движение склада знает, из каких партий
// сколько взяло. Остаток позиции = сумма остатков её партий. Модели пока не
// подключены: партии пишет З10-и3 (stockMove), поставки — З10-и4 и З10-и5.

// Статусы поставки.
const (
	SupplyWaiting = "waiting" // принята, ждёт цен
	SupplyPriced  = "priced"  // оценена
)

// Откуда партия.
const (
	LotSupply  = "supply"  // строка поставки
	LotInitial = "initial" // остаток позиции на момент включения партий
	LotFound   = "found"   // «нашлось» при корректировке (Д5)
)

// Supply — поставка. Status — SupplyWaiting или SupplyPriced (без default-тега:
// пишет его вызывающий). При оценке — контрагент, документ, способ оплаты (как у
// расхода), расход «Закупка …» и сумма, которую поставка в него записала:
// ExpenseGrosz не равна сумме расхода — его поправили вручную, не перезаписывать.
type Supply struct {
	ID             uuid.UUID  `json:"id"                        gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID         uuid.UUID  `json:"club_id"                   gorm:"type:uuid;not null"`
	Status         string     `json:"status"                    gorm:"size:8;not null"`
	Note           string     `json:"note"                      gorm:"not null"`
	ReceivedBy     *uuid.UUID `json:"received_by,omitempty"     gorm:"type:uuid"`
	CounterpartyID *uuid.UUID `json:"counterparty_id,omitempty" gorm:"type:uuid"`
	DocNo          string     `json:"doc_no"                    gorm:"size:64;not null"`
	DocDate        *time.Time `json:"doc_date,omitempty"        gorm:"type:date"`
	Method         *string    `json:"method,omitempty"          gorm:"size:8"`
	PricedBy       *uuid.UUID `json:"priced_by,omitempty"       gorm:"type:uuid"`
	PricedAt       *time.Time `json:"priced_at,omitempty"`
	ExpenseID      *uuid.UUID `json:"expense_id,omitempty"      gorm:"type:uuid"`
	ExpenseGrosz   *int64     `json:"-"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (s *Supply) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// AfterFind — дата документа: полночь клуба, а не UTC (как день операции расхода).
func (s *Supply) AfterFind(tx *gorm.DB) error {
	clubDayPtr(s.DocDate)
	return nil
}

func (Supply) TableName() string { return "supplies" }

// StockLot — партия позиции: пришло, осталось, цена за штуку в грошах (nil — не
// оценена). Списание — FIFO по ReceivedAt, затем Part (куски одной строки
// поставки после разбивки на цены).
type StockLot struct {
	ID         uuid.UUID  `json:"id"                    gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID     uuid.UUID  `json:"club_id"               gorm:"type:uuid;not null"`
	GoodID     uuid.UUID  `json:"good_id"               gorm:"type:uuid;not null"`
	Kind       string     `json:"kind"                  gorm:"size:8;not null"`
	SupplyID   *uuid.UUID `json:"supply_id,omitempty"   gorm:"type:uuid"`
	QtyIn      int        `json:"qty_in"                gorm:"not null"`
	QtyLeft    int        `json:"qty_left"              gorm:"not null"`
	PriceGrosz *int64     `json:"price_grosz,omitempty"`
	ReceivedAt time.Time  `json:"received_at"           gorm:"not null"`
	Part       int        `json:"part"                  gorm:"not null"`
	CreatedAt  time.Time  `json:"created_at"`
}

func (l *StockLot) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

func (StockLot) TableName() string { return "stock_lots" }

// LotMove — сколько движение склада взяло из партии или положило в неё (знак —
// как у движения).
type LotMove struct {
	ID          uuid.UUID `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	StockMoveID uuid.UUID `json:"stock_move_id" gorm:"type:uuid;not null"`
	LotID       uuid.UUID `json:"lot_id"        gorm:"type:uuid;not null"`
	Qty         int       `json:"qty"           gorm:"not null"`
}

func (m *LotMove) BeforeCreate(tx *gorm.DB) error {
	if m.ID == uuid.Nil {
		m.ID = uuid.New()
	}
	return nil
}

func (LotMove) TableName() string { return "lot_moves" }
