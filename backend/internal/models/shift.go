package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Shift — шаблон смены (миграция 023, Б11): имя, время (EndMin < StartMin —
// смена через полночь), дни недели битовой маской (бит 0 = понедельник).
type Shift struct {
	ID        uuid.UUID `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID `json:"club_id"    gorm:"type:uuid;not null"`
	Name      string    `json:"name"       gorm:"size:32;not null"`
	StartMin  int       `json:"start_min"  gorm:"not null"`
	EndMin    int       `json:"end_min"    gorm:"not null"`
	DaysMask  int       `json:"days_mask"  gorm:"not null;default:127"`
	Sort      int       `json:"sort"       gorm:"not null;default:0"`
	Active    bool      `json:"active"     gorm:"not null;default:true"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Shift) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (Shift) TableName() string { return "shifts" }

// ShiftAssignment — сотрудник на смене в конкретную дату (день начала смены).
type ShiftAssignment struct {
	ID        uuid.UUID  `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID  `json:"club_id"    gorm:"type:uuid;not null"`
	Date      time.Time  `json:"date"       gorm:"type:date;not null"`
	ShiftID   uuid.UUID  `json:"shift_id"   gorm:"type:uuid;not null"`
	UserID    uuid.UUID  `json:"user_id"    gorm:"type:uuid;not null"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty" gorm:"type:uuid"`
	CreatedAt time.Time  `json:"created_at"`

	User  *User  `json:"user,omitempty"  gorm:"foreignKey:UserID"`
	Shift *Shift `json:"shift,omitempty" gorm:"foreignKey:ShiftID"`
}

func (a *ShiftAssignment) BeforeCreate(tx *gorm.DB) error {
	if a.ID == uuid.Nil {
		a.ID = uuid.New()
	}
	return nil
}

func (ShiftAssignment) TableName() string { return "shift_assignments" }
