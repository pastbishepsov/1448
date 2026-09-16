package models

import (
	"time"

	"github.com/google/uuid"
)

// Виды проблем (К8, миграция 057).
const (
	IncidentPCHang    = "pc_hang"    // ПК завис / перезагрузился
	IncidentAgentLost = "agent_lost" // агент пропал во время сессии (автозапись хаба)
	IncidentPower     = "power"      // свет / питание
	IncidentOther     = "other"
)

// Incident — запись журнала проблем для стойки (К8, решение Р4 KIOSK.md):
// факт, длительность, решение админа — компенсация минутами (coin_minutes)
// или деньгами на кошелёк (wallet adjust, ref=incident) либо закрытие без
// компенсации. resolved_at IS NULL — открытая.
type Incident struct {
	ID         uuid.UUID  `json:"id"          gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID     uuid.UUID  `json:"club_id"     gorm:"type:uuid;not null"`
	ComputerID *uuid.UUID `json:"computer_id" gorm:"type:uuid"`
	UserID     *uuid.UUID `json:"user_id"     gorm:"type:uuid"`
	SessionID  *uuid.UUID `json:"session_id"  gorm:"type:uuid"`
	Kind       string     `json:"kind"        gorm:"size:16;not null"`
	Auto       bool       `json:"auto"        gorm:"not null;default:false"`
	StartedAt  time.Time  `json:"started_at"  gorm:"not null"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
	Minutes    int        `json:"minutes"     gorm:"not null;default:0"`
	Note       string     `json:"note"        gorm:"not null;default:''"`

	CompMinutes   int        `json:"comp_minutes"  gorm:"not null;default:0"` // → minute_grants зоны ПК (Ж2), без ПК — users.coin_minutes
	CompGrosz     int64      `json:"comp_grosz"    gorm:"not null;default:0"` // → кошелёк (adjust)
	CompensatedAt *time.Time `json:"compensated_at,omitempty"`
	ResolvedAt    *time.Time `json:"resolved_at,omitempty"`
	ResolvedBy    *uuid.UUID `json:"resolved_by,omitempty" gorm:"type:uuid"`
	CreatedBy     *uuid.UUID `json:"created_by,omitempty"  gorm:"type:uuid"` // nil — автозапись
	CreatedAt     time.Time  `json:"created_at"`
}

func (Incident) TableName() string { return "incidents" }
