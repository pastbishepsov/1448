package models

import (
	"time"

	"github.com/google/uuid"
)

// AimSession — один прогон AI-калибратора сенсы (миграция 059, С4-и1 SENS.md).
// Числа приходят из web/aim.html через шелл (postMessage aim_result → POST
// /me/aim-sessions); cm/360 сервер считает сам из sens×dpi — это инвариант,
// клиенту его не доверяем. Sim — синтетический стрелок стенда: хранится, но
// не участвует в тёплом старте, рейтинге и отчётах.
type AimSession struct {
	ID        uuid.UUID `json:"id"     gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID    uuid.UUID `json:"-"      gorm:"type:uuid;not null;index"`
	CreatedAt time.Time `json:"at"`

	Sens     float64 `json:"sens"     gorm:"type:numeric(8,3);not null"` // рекомендованный sens (CS2) при DPI
	DPI      int     `json:"dpi"      gorm:"not null"`
	Cm360    float64 `json:"cm360"    gorm:"type:numeric(7,2);not null"` // см на оборот
	Cur      float64 `json:"cur"      gorm:"type:numeric(8,3);not null;default:0"`
	CurCm    float64 `json:"cur_cm"   gorm:"type:numeric(7,2);not null;default:0"`
	Lo       float64 `json:"lo"       gorm:"type:numeric(8,3);not null;default:0"`
	Hi       float64 `json:"hi"       gorm:"type:numeric(8,3);not null;default:0"`
	Conf     string  `json:"conf"     gorm:"size:8;not null;default:low"`  // high|mid|low
	Verdict  string  `json:"verdict"  gorm:"size:8;not null;default:none"` // rec|cur|tie|none
	GainPct  float64 `json:"gain_pct" gorm:"type:numeric(6,1);not null;default:0"`
	Acc1     *int    `json:"acc1"` // точность первого выстрела, %
	RtMs     int     `json:"rt_ms"    gorm:"not null;default:0"`
	MtMs     int     `json:"mt_ms"    gorm:"not null;default:0"`
	Tp       float64 `json:"tp"       gorm:"type:numeric(6,2);not null;default:0"` // бит/с
	Mode     string  `json:"mode"     gorm:"size:8;not null;default:custom"`       // quick|precise|custom
	Rounds   int     `json:"rounds"   gorm:"not null;default:0"`
	AbRounds int     `json:"ab_rounds" gorm:"not null;default:0"`
	Partial  int     `json:"partial"  gorm:"not null;default:0"`
	Raw      *bool   `json:"raw"`
	Hz       int     `json:"hz"       gorm:"not null;default:0"`
	Coach    string  `json:"coach"    gorm:"size:8;not null;default:none"` // over|under|none (С4-и2)
	Warm     bool    `json:"warm"     gorm:"not null;default:false"`
	Lang     string  `json:"lang"     gorm:"size:2;not null;default:ru"`
	Sim      bool    `json:"sim"      gorm:"not null;default:false"`
}

func (AimSession) TableName() string { return "aim_sessions" }
