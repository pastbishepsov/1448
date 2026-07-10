package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// AdminAction — запись журнала-аудита действий персонала (миграция 016).
// Начисления и депозиты живут в своих таблицах (admin_grants, deposits);
// здесь — баны, форс-энды, ремонт ПК, брони за гостя, правки каталога.
type AdminAction struct {
	ID           uuid.UUID  `json:"id"             gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	AdminID      uuid.UUID  `json:"admin_id"       gorm:"type:uuid;not null"`
	Action       string     `json:"action"         gorm:"size:32;not null"`
	TargetUserID *uuid.UUID `json:"target_user_id,omitempty" gorm:"type:uuid"`
	Details      string     `json:"details"        gorm:"not null;default:''"`
	CreatedAt    time.Time  `json:"created_at"`
}

func (a *AdminAction) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (AdminAction) TableName() string { return "admin_actions" }
