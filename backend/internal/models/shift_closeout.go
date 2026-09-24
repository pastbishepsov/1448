package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ShiftCloseout — сдача смены с пересчётом наличных (миграция 066, Ж6-и4).
//
// Табель (WorkEntry) отвечает «сколько часов», эта запись — «сошлась ли
// касса»: сколько наличных должно быть в ящике по операциям этого админа за
// клубные сутки, сколько он насчитал и какая разница. Составляющие ожидаемой
// суммы хранятся отдельно: через месяц одну цифру «должно было быть 480 zł»
// проверить нечем. Skipped — ушёл без пересчёта: владелец должен это видеть
// так же ясно, как расхождение.
type ShiftCloseout struct {
	ID            uuid.UUID  `json:"id"            gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID        uuid.UUID  `json:"club_id"       gorm:"type:uuid;not null"`
	UserID        uuid.UUID  `json:"user_id"       gorm:"type:uuid;not null"`
	WorkEntryID   *uuid.UUID `json:"work_entry_id,omitempty" gorm:"type:uuid"`
	Date          time.Time  `json:"date"          gorm:"type:date;not null"`
	ExpectedGrosz int64      `json:"expected_grosz" gorm:"not null;default:0"`
	DepositsGrosz int64      `json:"deposits_grosz" gorm:"not null;default:0"`
	GoodsGrosz    int64      `json:"goods_grosz"    gorm:"not null;default:0"`
	PacksGrosz    int64      `json:"packs_grosz"    gorm:"not null;default:0"`
	RefundsGrosz  int64      `json:"refunds_grosz"  gorm:"not null;default:0"`
	CountedGrosz  *int64     `json:"counted_grosz,omitempty"`
	DiffGrosz     *int64     `json:"diff_grosz,omitempty"`
	Skipped       bool       `json:"skipped"       gorm:"not null;default:false"`
	Note          string     `json:"note"          gorm:"not null;default:''"`
	CreatedAt     time.Time  `json:"created_at"`
}

// AfterFind — день сдачи: полночь клуба, а не UTC (З2-и2, ClubDay).
func (s *ShiftCloseout) AfterFind(tx *gorm.DB) error {
	s.Date = ClubDay(s.Date)
	return nil
}

func (s *ShiftCloseout) BeforeCreate(tx *gorm.DB) error {
	if s.ID == uuid.Nil {
		s.ID = uuid.New()
	}
	return nil
}

func (ShiftCloseout) TableName() string { return "shift_closeouts" }
