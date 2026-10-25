package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StaffPeriod — период работы сотрудника (миграция 089, З15-и5): с какого дня, по какой и причина увольнения.
// Открытый период (DismissedOn == nil) у человека один; карточка (StaffProfile) показывает последний. Раньше
// повторный наём стирал прежнее увольнение из карточки — теперь каждый период остаётся своей строкой.
type StaffPeriod struct {
	ID          uuid.UUID  `json:"id"           gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID      uuid.UUID  `json:"user_id"      gorm:"type:uuid;not null"`
	HiredOn     *time.Time `json:"hired_on"     gorm:"type:date"`
	DismissedOn *time.Time `json:"dismissed_on" gorm:"type:date"`
	Reason      string     `json:"reason"       gorm:"not null;default:''"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (s *StaffPeriod) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

// AfterFind — даты периода: полночь клуба, а не UTC (как у карточки, З2-и2).
func (s *StaffPeriod) AfterFind(tx *gorm.DB) error {
	clubDayPtr(s.HiredOn)
	clubDayPtr(s.DismissedOn)
	return nil
}

func (StaffPeriod) TableName() string { return "staff_periods" }
