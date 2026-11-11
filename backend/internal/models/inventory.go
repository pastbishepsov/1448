package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// З23-и3 (миграция 098; OWNER.md, трек З, №54 «в»): инвентаризация — акт пересчёта склада. Разница по строке —
// корректировка с номером акта (StockMove.InventoryID), сошедшаяся строка — без движения. Пишет акт только
// handleInventoryCreate (inventory.go): акт не правится и не отменяется.

// Inventory — акт: номер по клубу (1, 2, 3 …), кто провёл и когда, заметка.
type Inventory struct {
	ID        uuid.UUID  `json:"id"                   gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID  `json:"club_id"              gorm:"type:uuid;not null"`
	No        int        `json:"no"                   gorm:"not null"`
	Note      string     `json:"note"                 gorm:"not null"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`
}

func (i *Inventory) BeforeCreate(tx *gorm.DB) error {
	if i.ID == uuid.Nil {
		i.ID = uuid.New()
	}
	return nil
}

func (Inventory) TableName() string { return "inventories" }

// InventoryLine — строка акта: позиция (удалят — nil, имя и категория остаются снимком), сколько было в системе, когда
// начали считать, и сколько насчитали; цена единицы — в момент акта; движение разницы (сошлось — nil); порядок
// пересчёта — Sort.
type InventoryLine struct {
	ID          uuid.UUID  `gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	InventoryID uuid.UUID  `gorm:"type:uuid;not null"`
	GoodID      *uuid.UUID `gorm:"type:uuid"`
	Name        string     `gorm:"size:64;not null"`
	Category    string     `gorm:"size:32;not null"`
	Expected    int        `gorm:"not null"`
	Counted     int        `gorm:"not null"`
	PricePLN    float64    `gorm:"type:decimal(8,2);not null"`
	StockMoveID *uuid.UUID `gorm:"type:uuid"`
	Sort        int        `gorm:"not null"`
}

func (l *InventoryLine) BeforeCreate(tx *gorm.DB) error {
	if l.ID == uuid.Nil {
		l.ID = uuid.New()
	}
	return nil
}

func (InventoryLine) TableName() string { return "inventory_lines" }
