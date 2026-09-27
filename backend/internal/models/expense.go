package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Расходы клуба (миграция 071, З9-и1; З-Р30): категории — справочник плюс
// свои, контрагенты — справочником, расход — дата операции, категория,
// контрагент, сумма брутто и VAT, способ, документ, заметка, фото. Вносит и
// видит только владелец (З-Р29). Удаления нет — сторно: строка остаётся с
// пометкой, кто и почему, и из сумм уходит.

// ExpenseCategory — категория расхода. Стандартные (Code) заводит миграция;
// свои (Code = nil) и скрытие — З9-и4. Purchase — закупка: в «Прибыли» это не
// расход, а себестоимость проданного (Д9).
type ExpenseCategory struct {
	ID        uuid.UUID `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Code      *string   `json:"code,omitempty" gorm:"size:24"`
	Name      string    `json:"name"       gorm:"size:64;not null"`
	Purchase  bool      `json:"purchase"   gorm:"not null;default:false"`
	Hidden    bool      `json:"hidden"     gorm:"not null;default:false"`
	Sort      int       `json:"sort"       gorm:"not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
}

func (ExpenseCategory) TableName() string { return "expense_categories" }

// Counterparty — контрагент: кому платим. NIP — 10 цифр (контрольная сумма — З9-и4).
type Counterparty struct {
	ID        uuid.UUID  `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Name      string     `json:"name"       gorm:"size:128;not null"`
	NIP       *string    `json:"nip,omitempty" gorm:"column:nip;size:10"`
	Contact   string     `json:"contact"    gorm:"size:200;not null;default:''"`
	Note      string     `json:"note"       gorm:"not null;default:''"`
	Hidden    bool       `json:"hidden"     gorm:"not null;default:false"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

func (c *Counterparty) BeforeCreate(tx *gorm.DB) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return nil
}

func (Counterparty) TableName() string { return "counterparties" }

// Способы оплаты расхода (№62): из кассы (ящик), банк, карта клуба, личные
// деньги владельца. Лишние скрываются настройкой — З9-и4.
const (
	ExpenseTill  = "till"
	ExpenseBank  = "bank"
	ExpenseCard  = "card"
	ExpenseOwner = "owner"
)

// Expense — расход. Суммы — в грошах: брутто и VAT (VAT ≤ брутто). OpDate —
// день операции по календарю клуба. SupplyID — поставка (З10), из которой расход.
type Expense struct {
	ID             uuid.UUID  `json:"id"              gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID         uuid.UUID  `json:"club_id"         gorm:"type:uuid;not null"`
	OpDate         time.Time  `json:"op_date"         gorm:"type:date;not null"`
	Paid           bool       `json:"paid"            gorm:"not null"` // без default: GORM молча заменил бы false на значение по умолчанию
	CategoryID     uuid.UUID  `json:"category_id"     gorm:"type:uuid;not null"`
	CounterpartyID *uuid.UUID `json:"counterparty_id,omitempty" gorm:"type:uuid"`
	GrossGrosz     int64      `json:"gross_grosz"     gorm:"not null"`
	VatGrosz       int64      `json:"vat_grosz"       gorm:"not null;default:0"`
	Method         string     `json:"method"          gorm:"size:8;not null"`
	DocNo          string     `json:"doc_no"          gorm:"size:64;not null;default:''"`
	Note           string     `json:"note"            gorm:"not null;default:''"`
	PhotoType      *string    `json:"-"               gorm:"size:16"`
	PhotoAt        *time.Time `json:"-"`
	SupplyID       *uuid.UUID `json:"supply_id,omitempty" gorm:"type:uuid"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	StornoAt       *time.Time `json:"storno_at,omitempty"`
	StornoBy       *uuid.UUID `json:"storno_by,omitempty" gorm:"type:uuid"`
	StornoReason   string     `json:"storno_reason"   gorm:"not null;default:''"`
}

func (e *Expense) BeforeCreate(tx *gorm.DB) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	return nil
}

// AfterFind — день операции: полночь клуба, а не UTC (как у табеля, З2-и2).
func (e *Expense) AfterFind(tx *gorm.DB) error {
	e.OpDate = ClubDay(e.OpDate)
	return nil
}

func (Expense) TableName() string { return "expenses" }
