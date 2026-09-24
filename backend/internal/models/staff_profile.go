package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StaffProfile — кадровая карточка сотрудника (миграция 025, В3-этап 2).
// Отдельно от User: users — профиль гостя, он виден в клубе; личные данные
// сотрудника видит и правит только владелец.
type StaffProfile struct {
	UserID      uuid.UUID  `json:"user_id"      gorm:"type:uuid;primaryKey"`
	FullName    string     `json:"full_name"    gorm:"size:128;not null;default:''"`
	Phone       string     `json:"phone"        gorm:"size:32;not null;default:''"`
	Position    string     `json:"position"     gorm:"size:64;not null;default:''"`
	HiredAt     *time.Time `json:"hired_at"     gorm:"type:date"`
	DismissedAt *time.Time `json:"dismissed_at" gorm:"type:date"`
	RateType    string     `json:"rate_type"    gorm:"size:16;not null;default:none"`
	RateAmount  float64    `json:"rate_amount"  gorm:"type:decimal(10,2);not null;default:0"`
	Note        string     `json:"note"         gorm:"not null;default:''"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// AfterFind — даты найма и увольнения: полночь клуба, а не UTC (З2-и2, ClubDay):
// иначе «уволен сегодня» выходил раньше «нанят сегодня».
func (p *StaffProfile) AfterFind(tx *gorm.DB) error {
	clubDayPtr(p.HiredAt)
	clubDayPtr(p.DismissedAt)
	return nil
}

func (StaffProfile) TableName() string { return "staff_profiles" }
