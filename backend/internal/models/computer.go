package models

import (
	"time"

	"github.com/google/uuid"
)

type ComputerStatus string

const (
	ComputerStatusAvailable   ComputerStatus = "available"
	ComputerStatusInSession   ComputerStatus = "in_session"
	ComputerStatusMaintenance ComputerStatus = "maintenance"
	ComputerStatusReserved    ComputerStatus = "reserved"
)

type Computer struct {
	ID        uuid.UUID      `json:"id"         gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	ClubID    uuid.UUID      `json:"club_id"    gorm:"type:uuid;not null;index"`
	Name      string         `json:"name"       gorm:"size:32;not null"`
	Zone      string         `json:"zone"       gorm:"size:32"`
	Status    ComputerStatus `json:"status"     gorm:"type:computer_status;default:available"`
	LastSeen  time.Time      `json:"last_seen"`
	CreatedAt time.Time      `json:"created_at"`

	Club Club `json:"club,omitempty" gorm:"foreignKey:ClubID"`
}

type Club struct {
	ID          uuid.UUID `json:"id"           gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Name        string    `json:"name"         gorm:"size:128;not null"`
	Address     string    `json:"address"      gorm:"not null"`
	Latitude    *float64  `json:"latitude,omitempty"`
	Longitude   *float64  `json:"longitude,omitempty"`
	Phone       *string   `json:"phone,omitempty"     gorm:"size:20"`
	Telegram    *string   `json:"telegram,omitempty"  gorm:"size:64"`
	Instagram   *string   `json:"instagram,omitempty" gorm:"size:64"`

	// Тарифы — рынок Польши, PLN
	// Диапазон: 10–50 zł/час, средний ~23 zł/час
	BaseRatePLN float64 `json:"base_rate_pln" gorm:"type:decimal(8,2);default:23.00"`
	RTPModifier float64 `json:"rtp_modifier"  gorm:"type:decimal(4,2);default:1.00"`

	IsActive  bool      `json:"is_active"  gorm:"default:true"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
