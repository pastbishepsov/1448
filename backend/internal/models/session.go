package models

import (
	"time"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type SessionStatus string

const (
	SessionStatusActive    SessionStatus = "active"
	SessionStatusCompleted SessionStatus = "completed"
	SessionStatusCancelled SessionStatus = "cancelled"
)

// Session — игровая сессия
type Session struct {
	ID         uuid.UUID     `json:"id" gorm:"type:uuid;primaryKey;default:uuid_generate_v4()"`
	UserID     uuid.UUID     `json:"user_id" gorm:"type:uuid;not null;index"`
	ComputerID uuid.UUID     `json:"computer_id" gorm:"type:uuid;not null"`
	ClubID     uuid.UUID     `json:"club_id" gorm:"type:uuid;not null"`
	Status     SessionStatus `json:"status" gorm:"type:session_status;default:active"`

	StartedAt   time.Time  `json:"started_at" gorm:"not null"`
	EndedAt     *time.Time `json:"ended_at,omitempty"`
	MinutesTotal int       `json:"minutes_total" gorm:"not null"`
	MinutesUsed  int       `json:"minutes_used" gorm:"default:0"`

	BaseRatePLN      float64 `json:"base_rate_pln" gorm:"type:decimal(8,2);not null"`
	EffectiveRatePLN float64 `json:"effective_rate_pln" gorm:"type:decimal(8,2);not null"`

	XPEarned    int64 `json:"xp_earned" gorm:"default:0"`
	CoinsEarned int64 `json:"coins_earned" gorm:"default:0"`

	// Биллинг кошелька (трек Г, Г1; миграция 032). Поля меняет только
	// биллинг-движок (billing.go) — транзакционно, вместе со списанием.
	BilledMinutes   int        `json:"billed_minutes" gorm:"default:0"`    // минут учтено всего
	CoinMinutesUsed int        `json:"coin_minutes_used" gorm:"default:0"` // покрыто минутным запасом монет
	MoneyMinutes    int        `json:"money_minutes" gorm:"default:0"`     // оплачено кошельком
	// Е2-и3: покрыто минутами купленного пакета. Отдельно от монет и кошелька:
	// иначе чек «3 часа из пакета, 40 минут с кошелька» не сойдётся, а
	// обязательство клуба по непроигранным пакетам нечем будет проверить.
	PackMinutesUsed int        `json:"pack_minutes_used" gorm:"default:0"`
	ChargedGrosz    int64      `json:"charged_grosz" gorm:"default:0"`     // списано денег за сессию
	Warn15At        *time.Time `json:"warn15_at,omitempty"`                // «осталось ~15 мин» отправлено
	Warn5At         *time.Time `json:"warn5_at,omitempty"`                 // «осталось ~5 мин» отправлено
	ZeroSince       *time.Time `json:"zero_since,omitempty"`               // кошелёк на нуле с … (грейс)
	EndedReason     *string    `json:"ended_reason,omitempty" gorm:"size:16"` // manual|admin|balance|afk (Г3: booking)

	// Пауза и AFK (трек Г, Г2; миграция 033). Паузные поля меняют только
	// startPause/resumePause (pause.go) и биллинг.
	PausedAt       *time.Time `json:"paused_at,omitempty"`                  // пауза идёт с …
	PausedTotalSec int        `json:"paused_total_sec" gorm:"default:0"`    // суммарная пауза, сек
	PausedBy       *string    `json:"paused_by,omitempty" gorm:"size:8"`    // guest | afk
	ActiveMinutes  int        `json:"active_minutes" gorm:"default:0"`      // минуты без простоя (анти-фарм Г5)
	AfkWarnedAt    *time.Time `json:"afk_warned_at,omitempty"`              // afk_warn отправлен

	// Дедлайн чужой брони (Г3, миграция 034): предупреждения за ~15/~5 минут
	// до принудительного освобождения ПК (билл-тик, ended_reason=booking).
	BkWarn15At *time.Time `json:"bkwarn15_at,omitempty" gorm:"column:bkwarn15_at"`
	BkWarn5At  *time.Time `json:"bkwarn5_at,omitempty"  gorm:"column:bkwarn5_at"`

	// Окно [Готов!] (Е1, миграция 045): гость идёт от стойки к машине, и это
	// время не оплачивается. Ожидание = ReadyDeadline задан, ReadyAt пуст.
	ReadyAt       *time.Time `json:"ready_at,omitempty"`       // нажал [Готов!] (или авто-старт по дедлайну)
	ReadyDeadline *time.Time `json:"ready_deadline,omitempty"` // до какого момента ждём нажатия
	// Ж4 (Ж-Р6, миграция 064): посадка на ПК без связи ждёт шелл до этого
	// момента; агент подключился — открывается окно [Готов!], не подключился —
	// посадка снимается без списания. Ожидание шелла = ShellWaitUntil задан,
	// ReadyAt пуст.
	ShellWaitUntil *time.Time `json:"shell_wait_until,omitempty"`

	CreatedAt time.Time `json:"created_at"`

	// З13-и1 (миграция 083): снимок ставки VAT зоны ПК на начало сессии — по ней время с кошелька в режиме «VAT при
	// трате»; NULL — сессия до ставок.
	VatPct *int `json:"vat_pct,omitempty" gorm:"column:vat_pct"`

	// З16-и1 (миграция 091, З-Р25): снимок условий на начало сессии — зона ПК (id и имя: имя остаётся, если зону потом
	// переименуют или удалят; ПК без зоны — nil и пусто) и ставки XP и монет за минуту из настроек; по ставкам начисляет
	// finishSession (sessionRates). Пишет startSessionFor, пересадка не меняет — как и тариф. Ставки nil — сессия,
	// начатая прежней версией сервера: начисление берёт нынешние.
	ZoneID      *uuid.UUID `json:"zone_id,omitempty" gorm:"type:uuid"`
	ZoneName    string     `json:"zone_name" gorm:"size:32;not null"`
	XPPerMin    *int64     `json:"xp_per_min,omitempty" gorm:"column:xp_per_min"`
	CoinsPerMin *int64     `json:"coins_per_min,omitempty" gorm:"column:coins_per_min"`

	// Связи
	User     *User     `json:"user,omitempty" gorm:"foreignKey:UserID"`
	Computer *Computer `json:"computer,omitempty" gorm:"foreignKey:ComputerID"`
}

// BeforeCreate — З13-и1: снимок ставки VAT зоны ПК; ПК без зоны — VatDefault.
func (s *Session) BeforeCreate(tx *gorm.DB) error {
	if s.VatPct == nil {
		s.VatPct = vatSnapshot(tx, "SELECT z.vat_pct FROM computers c JOIN zones z ON z.id = c.zone_id WHERE c.id = ?", s.ComputerID)
	}
	return nil
}
