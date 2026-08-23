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

type UserRole string

const (
	UserRolePlayer UserRole = "player"
	UserRoleAdmin  UserRole = "admin"
	UserRoleOwner  UserRole = "owner"
)

// User — аккаунт игрока
type User struct {
	ID           uuid.UUID  `json:"id"             gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	Nickname     string     `json:"nickname"       gorm:"uniqueIndex;size:32;not null"`
	Phone        *string    `json:"phone,omitempty" gorm:"uniqueIndex;size:20"`
	Email        *string    `json:"email,omitempty" gorm:"uniqueIndex;size:255"`
	PasswordHash string     `json:"-"              gorm:"size:255;not null"`
	Status       UserStatus `json:"status"         gorm:"type:user_status;default:active"`
	Role         UserRole   `json:"role"           gorm:"type:user_role;default:player"`

	Level                int     `json:"level"                  gorm:"default:1"`
	XPCurrent            int64   `json:"xp_current"             gorm:"default:0"`
	XPTotal              int64   `json:"xp_total"               gorm:"default:0"`
	CoinsBalance         int64   `json:"coins_balance"          gorm:"default:0"`
	WalletGrosz          int64   `json:"wallet_grosz"           gorm:"default:0"` // денежный кошелёк в грошах (трек Г, Г0-и1): меняется только через walletApply
	CoinMinutes          int64   `json:"coin_minutes"           gorm:"default:0"` // минутный запас из обмена монет (В4 redeem): биллинг Г1 тратит его раньше кошелька
	SkillpointsAvailable int     `json:"skillpoints_available"  gorm:"default:0"`
	PaymentIncreasePct   float64 `json:"payment_increase_pct"   gorm:"type:decimal(5,2);default:0"`

	AvatarID int `json:"avatar_id" gorm:"default:1"`

	RegisteredAt time.Time `json:"registered_at"`
	LastActiveAt time.Time `json:"last_active_at"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// RoundToStep — округляет неотрицательное число к ближайшему кратному step.
// Правило цифр (DESIGN.md): всё, что видит игрок, кратно 5, пороги уровней — 100.
// Округляем в момент начисления, а не при показе — иначе разойдётся с балансом.
func RoundToStep(v, step int64) int64 {
	if step <= 0 || v < 0 {
		return v
	}
	return (v + step/2) / step * step
}

// XPForNextLevel — сколько XP нужно для перехода на следующий уровень.
// Формула: XP(n) = 1000 * n^1.4, округлённая до 100: пороги выглядят аккуратно
// (1000, 2600, 4700, 7000...), кривая роста сохраняется.
func XPForNextLevel(level int) int64 {
	raw := 1000 * math.Pow(float64(level), 1.4)
	return int64(math.Round(raw/100)) * 100
}

// BeforeCreate — устанавливает UUID перед созданием
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.ID == uuid.Nil {
		u.ID = uuid.New()
	}
	return nil
}
