package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminGrant — запись журнала ручного начисления (миграция 012).
type AdminGrant struct {
	ID        uuid.UUID `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID    uuid.UUID `json:"user_id"    gorm:"type:uuid;not null;index"`
	AdminID   uuid.UUID `json:"admin_id"   gorm:"type:uuid;not null"`
	GrantType string    `json:"grant_type" gorm:"size:8;not null"` // xp | case
	Amount    *int64    `json:"amount,omitempty"`
	CaseTier  *CaseTier `json:"case_tier,omitempty" gorm:"type:case_tier"`
	Reason    string    `json:"reason"     gorm:"not null"`
	CreatedAt time.Time `json:"created_at"`
}

func (g *AdminGrant) BeforeCreate(tx *gorm.DB) error {
	if g.ID == uuid.Nil {
		g.ID = uuid.New()
	}
	return nil
}

func (AdminGrant) TableName() string { return "admin_grants" }
