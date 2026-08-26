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

	// Профиль гостя (Г8, миграция 040; имя/фамилия — 043, страница
	// регистрации Р9): всё опционально и стираемо (GDPR), значения в аудит
	// не пишутся. Награды за заполнение — lifetime-ачивки.
	FirstName        *string    `json:"first_name,omitempty" gorm:"size:64"`
	LastName         *string    `json:"last_name,omitempty"  gorm:"size:64"`
	BirthDate        *time.Time `json:"birth_date,omitempty" gorm:"type:date"`
	Discord          *string    `json:"discord,omitempty"    gorm:"size:64"`
	Telegram         *string    `json:"telegram,omitempty"   gorm:"size:64"`
	Source           *string    `json:"source,omitempty"     gorm:"size:32"`
	FavoriteGames    []string   `json:"favorite_games"       gorm:"serializer:json;type:jsonb;not null;default:'[]'"`
	BirthdayGiftYear int        `json:"-"                    gorm:"default:0"` // год последней выдачи ДР-подарка
	Language         string     `json:"language"             gorm:"size:2;not null;default:ru"` // Г9: ru|en|pl, едет за гостем
	StreakFreezes    int        `json:"streak_freezes"       gorm:"not null;default:0"` // Г6-и4: запас заморозок стрика

	// Сброс пароля админом (Е0-и2, миграция 044). Флаг едет клиенту — по нему
	// шелл и PWA поднимают экран обязательной смены; момент отсечки токенов
	// наружу не отдаём, он служебный.
	MustChangePassword bool       `json:"must_change_password" gorm:"not null;default:false"`
	TokensValidFrom    *time.Time `json:"-"`

	// Онбординг персонала (Е5, миграция 051). Флаг на сервере, а не в
	// localStorage: админка открывается с разных машин клуба, и локальный флаг
	// означал бы тур на каждой новой.
	OnboardedAt      *time.Time `json:"onboarded_at,omitempty"`
	OnboardedVersion int        `json:"onboarded_version" gorm:"not null;default:0"`
	// Е6: докуда человек дочитал служебный канал. Отметка «до», а не флаги на
	// каждом сообщении: участников много, таблица флагов росла бы произведением.
	StaffChatReadAt *time.Time `json:"staff_chat_read_at,omitempty"`

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
