package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ZonePriceRule — правило цены часа зоны (З24-и1, миграция 100; OWNER.md, трек З, №53 «в»): дни недели (DaysMask —
// пн = бит 0 … вс = бит 6, как у шаблонов смен), окно по стенным часам клуба [StartMin, EndMin) — EndMin 1440 — до
// полуночи, EndMin <= StartMin — через полночь (принадлежит дню начала), цена часа, срок действия по датам дня начала
// окна (включительно; nil — без границы), заметка. Вне правил зона стоит Zone.RatePLN. Правила зоны не накладываются.
type ZonePriceRule struct {
	ID        uuid.UUID  `json:"id"                   gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ZoneID    uuid.UUID  `json:"zone_id"              gorm:"type:uuid;not null"`
	DaysMask  int        `json:"days_mask"            gorm:"not null"`
	StartMin  int        `json:"start_min"            gorm:"not null"`
	EndMin    int        `json:"end_min"              gorm:"not null"`
	RatePLN   float64    `json:"rate_pln"             gorm:"type:decimal(8,2);not null"`
	ValidFrom *time.Time `json:"valid_from,omitempty" gorm:"type:date"`
	ValidTo   *time.Time `json:"valid_to,omitempty"   gorm:"type:date"`
	Note      string     `json:"note"                 gorm:"size:64;not null"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

func (r *ZonePriceRule) BeforeCreate(tx *gorm.DB) error {
	if r.ID == uuid.Nil {
		r.ID = uuid.New()
	}
	return nil
}

// AfterFind — даты срока действия: полночь клуба, а не UTC (как дата документа поставки).
func (r *ZonePriceRule) AfterFind(tx *gorm.DB) error {
	clubDayPtr(r.ValidFrom)
	clubDayPtr(r.ValidTo)
	return nil
}

func (ZonePriceRule) TableName() string { return "zone_price_rules" }
