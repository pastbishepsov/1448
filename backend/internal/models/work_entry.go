package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WorkEntry — запись табеля: фактический приход и уход сотрудника
// (миграция 026, В3-этап 4). График — это план, а это факт.
// Date — клубные сутки (день начала смены), как и везде в отчётах.
type WorkEntry struct {
	ID         uuid.UUID  `json:"id"          gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID     uuid.UUID  `json:"club_id"     gorm:"type:uuid;not null"`
	UserID     uuid.UUID  `json:"user_id"     gorm:"type:uuid;not null"`
	ShiftID    *uuid.UUID `json:"shift_id,omitempty" gorm:"type:uuid"`
	Date       time.Time  `json:"date"        gorm:"type:date;not null"`
	StartedAt  time.Time  `json:"started_at"  gorm:"not null"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Minutes    int        `json:"minutes"     gorm:"not null;default:0"`
	AutoClosed bool       `json:"auto_closed" gorm:"not null;default:false"`
	Note       string     `json:"note"        gorm:"not null;default:''"`
	CreatedBy  *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	EditedBy   *uuid.UUID `json:"edited_by,omitempty"  gorm:"type:uuid"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

func (w *WorkEntry) BeforeCreate(tx *gorm.DB) error {
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	return nil
}

func (WorkEntry) TableName() string { return "work_entries" }
