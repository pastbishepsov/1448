package models

import (
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusBanned    UserStatus = "banned"
	UserStatusSuspended UserStatus = "suspended"
)

// User — аккаунт игрока
type User struct {
	ID           uuid.UUID  `json:"id"             gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Nickname     string     `json:"nickname"       gorm:"uniqueIndex;size:32;not null"`
	Phone        *string    `json:"phone,omitempty" gorm:"uniqueIndex;size:20"`
	Email        *string    `json:"email,omitempty" gorm:"uniqueIndex;size:255"`
	PasswordHash string     `json:"-"              gorm:"size:255;not null"`
	Status       UserStatus `json:"status"         gorm:"type:user_status;default:active"`

	Level                int     `json:"level"                  gorm:"default:1"`
	XPCurrent            int64   `json:"xp_current"             gorm:"default:0"`
	XPTotal              int64   `json:"xp_total"               gorm:"default:0"`
	CoinsBalance         int64   `json:"coins_balance"          gorm:"default:0"`
	SkillpointsAvailable int     `json:"skillpoints_available"  gorm:"default:0"`
	PaymentIncreasePct   float64 `json:"payment_increase_pct"   gorm:"type:decimal(5,2);default:0"`

	AvatarID int `json:"avatar_id" gorm:"default:1"`

	RegisteredAt time.Time `json:"registered_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// XPForNextLevel — сколько XP нужно для перехода на следующий уровень.
// Формула: XP(n) = 1000 * n^1.4
func XPForNextLevel(level int) int64 {
	return int64(1000 * math.Pow(float64(level), 1.4))
}

// BeforeCreate — устанавливает UUID перед созданием
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
