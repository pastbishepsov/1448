package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Zone — зона зала со своей ценой часа (миграция 027, В4-этап 5).
// Имя придумывает владелец: «Standard», «VIP», «Симрейсинг» — что угодно.
type Zone struct {
	ID        uuid.UUID `json:"id"        gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID `json:"club_id"   gorm:"type:uuid;not null"`
	Name      string    `json:"name"      gorm:"size:32;not null"`
	RatePLN   float64   `json:"rate_pln"  gorm:"type:decimal(8,2);not null"`
	Sort      int       `json:"sort"      gorm:"not null;default:0"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (z *Zone) BeforeCreate(tx *gorm.DB) error {
	if z.ID == uuid.Nil {
		z.ID = uuid.New()
	}
	return nil
}

func (Zone) TableName() string { return "zones" }
