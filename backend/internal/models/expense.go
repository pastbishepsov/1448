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
	// З9-и3а (миграция 072): расход из кассы в модели ящика — когда ушёл из ящика
	// (момент записи или миг перед объясняемой сдачей) и какую сдачу объясняет.
	DrawerAt         *time.Time `json:"drawer_at,omitempty"`
	ExplainsCloseout *uuid.UUID `json:"explains_closeout,omitempty" gorm:"column:explains_closeout;type:uuid"`
	// З9-и5 (миграция 074): внесён по шаблону регулярного расхода — за этот месяц (первое число).
	TemplateID    *uuid.UUID `json:"template_id,omitempty" gorm:"type:uuid"`
	TemplateMonth *time.Time `json:"template_month,omitempty" gorm:"type:date"`
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
	clubDayPtr(e.TemplateMonth)
	return nil
}

func (Expense) TableName() string { return "expenses" }

// OwnerRepayment — возврат владельцу его личных денег, потраченных на клуб
// (миграция 073, З9-и3б): из кассы (ушло из ящика) или с банка (ящик не
// трогает). «Клуб должен владельцу» = оплаченные расходы личными − возвраты.
// Отменённый (VoidedAt) не считается ни в долге, ни в ящике.
type OwnerRepayment struct {
	ID          uuid.UUID  `json:"id"           gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID      uuid.UUID  `json:"club_id"      gorm:"type:uuid;not null"`
	AmountGrosz int64      `json:"amount_grosz" gorm:"not null"`
	Method      string     `json:"method"       gorm:"size:8;not null"`
	Note        string     `json:"note"         gorm:"not null;default:''"`
	CreatedBy   *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt   time.Time  `json:"created_at"`
	VoidedAt    *time.Time `json:"voided_at,omitempty"`
	VoidedBy    *uuid.UUID `json:"voided_by,omitempty"  gorm:"type:uuid"`
}

// Откуда вернули владельцу.
const (
	RepayTill = "till" // из кассы — «должно быть в ящике» меньше
	RepayBank = "bank" // с банка — ящик не трогает
)

func (r *OwnerRepayment) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

func (OwnerRepayment) TableName() string { return "owner_repayments" }

// ExpenseTemplate — регулярный расход (миграция 074, З9-и5; №67, Д10): раз в
// месяц в день Day (нет такого числа в месяце — в последний день) расход ждёт
// внесения: в «Расходах» — «Ждут внесения», на «Бухгалтерии» — бейдж, в день
// срока — одно сообщение «только владельцу» в канал. FromMonth — первый месяц,
// за который шаблон ждёт; RemindedFor — месяц, о котором уже напомнили.
// Выключенный (Active = false) не ждёт и не напоминает; удаления нет.
type ExpenseTemplate struct {
	ID             uuid.UUID  `json:"id"              gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID         uuid.UUID  `json:"club_id"         gorm:"type:uuid;not null"`
	Name           string     `json:"name"            gorm:"size:64;not null"`
	CategoryID     uuid.UUID  `json:"category_id"     gorm:"type:uuid;not null"`
	CounterpartyID *uuid.UUID `json:"counterparty_id,omitempty" gorm:"type:uuid"`
	GrossGrosz     int64      `json:"-"               gorm:"not null"`
	VatGrosz       int64      `json:"-"               gorm:"not null"`
	Method         string     `json:"method"          gorm:"size:8;not null"`
	Day            int        `json:"day"             gorm:"not null"`
	Note           string     `json:"note"            gorm:"not null"`
	Active         bool       `json:"active"          gorm:"not null"` // без default-тега: иначе GORM не запишет false
	FromMonth      time.Time  `json:"from_month"      gorm:"type:date;not null"`
	RemindedFor    *time.Time `json:"reminded_for,omitempty" gorm:"type:date"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

func (t *ExpenseTemplate) BeforeCreate(tx *gorm.DB) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	return nil
}

// AfterFind — месяцы шаблона: полночь клуба, а не UTC (как день операции расхода).
func (t *ExpenseTemplate) AfterFind(tx *gorm.DB) error {
	t.FromMonth = ClubDay(t.FromMonth)
	clubDayPtr(t.RemindedFor)
	return nil
}

func (ExpenseTemplate) TableName() string { return "expense_templates" }
