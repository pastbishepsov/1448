package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// TimePackage — позиция каталога пакетов времени (миграция 047, Е2-и1).
//
// Пакет — не деньги, а купленный запас минут в конкретной зоне (Р2, Р10):
// «3 часа STANDARD за 45 zł». Кошелёк такую скидку дать не может, не смешав
// предоплаченные злотые со скидкой на час, поэтому пакет живёт отдельной
// сущностью, как товар ценника В2.
//
// Себестоимости нет — по тому же решению основателя, что и у товаров.
type TimePackage struct {
	ID       uuid.UUID `json:"id"       gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID   uuid.UUID `json:"club_id"  gorm:"type:uuid;not null"`
	ZoneID   uuid.UUID `json:"zone_id"  gorm:"type:uuid;not null"` // обязательна (Р10)
	Name     string    `json:"name"     gorm:"size:64;not null"`
	Minutes  int       `json:"minutes"  gorm:"not null"`
	PricePLN float64   `json:"price_pln" gorm:"type:decimal(8,2);not null"`
	// Срок годности ВЫДАННЫХ минут в днях; 0 — бессрочно (Р11). Считается от
	// момента выдачи, а не от создания пакета: гость покупает свой срок.
	DaysValid int `json:"days_valid" gorm:"not null;default:0"`
	Sort      int `json:"sort" gorm:"not null;default:0"`
	// Тега default:true тут НЕТ намеренно: с ним GORM считает false «пустым
	// значением» и не шлёт колонку при вставке — база подставляет TRUE, и
	// пакет, созданный выключенным, оказывается включённым и продаётся.
	// Найдено живым e2e Е2-и2. Дефолт остаётся на стороне БД, для чистого SQL.
	Active    bool      `json:"active"     gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (p *TimePackage) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (TimePackage) TableName() string { return "time_packages" }

// UserPackage — выданный или купленный пакет: строка на каждую покупку
// (миграция 047, Е2). Счётчиком на госте это не сделать — у каждой покупки
// свои срок (Р11), зона (Р10) и цена, а «сколько минут осталось» без этого не
// разложить ни в отчёте, ни в разговоре с гостем.
//
// Имя, зона и цена — снимок на момент покупки (образец CoinRedemption): цену
// поднимут, зону переименуют, а проданное останется тем, чем его продали.
type UserPackage struct {
	ID           uuid.UUID  `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID       uuid.UUID  `json:"club_id"    gorm:"type:uuid;not null"`
	UserID       uuid.UUID  `json:"user_id"    gorm:"type:uuid;not null"`
	PackageID    *uuid.UUID `json:"package_id,omitempty" gorm:"type:uuid"`
	ZoneID       uuid.UUID  `json:"zone_id"    gorm:"type:uuid;not null"`
	Name         string     `json:"name"       gorm:"size:64;not null"`
	ZoneName     string     `json:"zone_name"  gorm:"size:32;not null;default:''"`
	MinutesTotal int        `json:"minutes_total" gorm:"not null"`
	MinutesLeft  int        `json:"minutes_left"  gorm:"not null"`
	PricePLN     float64    `json:"price_pln"     gorm:"type:decimal(8,2);not null"`
	// cash|card|blik — выручка; wallet — погашение обязательства (Г-Р7).
	Method    string     `json:"method"     gorm:"size:16;not null;default:cash"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"` // NULL — бессрочно (Р11)
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	// Е2-и5: когда гостю сказали, что пакет скоро сгорит. Флаг на строке, а не
	// раскопки по журналу уведомлений: у пакета один срок и одно предупреждение.
	WarnedAt  *time.Time `json:"warned_at,omitempty"`
	VoidedAt  *time.Time `json:"voided_at,omitempty"`
	VoidedBy  *uuid.UUID `json:"voided_by,omitempty"  gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

func (p *UserPackage) BeforeCreate(tx *gorm.DB) error {
	if p.ID == uuid.Nil {
		p.ID = uuid.New()
	}
	return nil
}

func (UserPackage) TableName() string { return "user_packages" }
