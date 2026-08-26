package main

// Пауза сессии, AFK и пересадка (трек Г, спринт Г2; GUEST.md, этап I).
//
// Пауза: время, деньги (биллинг Г1) и XP стоят, ПК остаётся за гостем.
// Лимит — НА СЕССИЮ (настройка pause_limit_min, деф. 15 мин, 0 = пауза
// выключена); по исчерпании биллинг снимает паузу сам и время снова тикает.
// (В плане лимит звался «дневным» — дневной учёт придёт с user_progress Г5;
// на сессию — строже к абьюзу «держать ПК бесплатно» и без новой таблицы.)
//
// AFK: агент кладёт в session_tick секунды простоя ввода (GetLastInputInfo);
// порог afk_stop_min (деф. 10 мин, 0 = выкл). Реакция на тике биллинга:
// afk_warn (один раз) → автопауза от системы (paused_by=afk), если паузный
// бюджет есть; гость шевельнул мышью — пауза снимается сама; бюджета нет —
// завершение ended_reason=afk. Нет датчика (нет агента) — AFK не судим.
//
// Пересадка (Г2-и3): админ переносит живую сессию на другой свободный ПК
// клуба без разрыва биллинга — тариф сессии зафиксирован при старте и
// сознательно НЕ пересчитывается (пересадка чинит железо, а не цену).

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

const (
	pauseLimitMinDef int64 = 15  // настройка pause_limit_min: лимит паузы на сессию
	afkStopMinDef    int64 = 10  // настройка afk_stop_min: порог простоя
	afkBackIdleSec         = 15  // простой меньше — гость вернулся, afk-пауза снимается
	activeIdleSec          = 150 // минута активна, если простой меньше (анти-фарм Г5)
	idleFreshSec           = 150 // сигнал агента старше — считаем, что датчика нет
)

// pausedDuration — сколько секунд сессия провела на паузе к моменту now.
func pausedDuration(s *models.Session, now time.Time) int {
	total := s.PausedTotalSec
	if s.PausedAt != nil {
		total += int(now.Sub(*s.PausedAt).Seconds())
	}
	if total < 0 {
		total = 0
	}
	return total
}

// effectiveElapsedMinutes — целые минуты сессии БЕЗ пауз (вниз; тики Г1).
func effectiveElapsedMinutes(s *models.Session, now time.Time) int {
	sec := int(now.Sub(s.StartedAt).Seconds()) - pausedDuration(s, now)
	if sec < 0 {
		sec = 0
	}
	return sec / 60
}

// effectiveMinutesCeil — минуты БЕЗ пауз, вверх (финальный расчёт и XP).
func effectiveMinutesCeil(s *models.Session, now time.Time) int {
	sec := int(now.Sub(s.StartedAt).Seconds()) - pausedDuration(s, now)
	if sec <= 0 {
		return 0
	}
	return (sec + 59) / 60
}

// pauseBudgetLeftSec — остаток паузного бюджета сессии, сек.
func pauseBudgetLeftSec(s *models.Session, limitMin int64, now time.Time) int {
	left := int(limitMin)*60 - pausedDuration(s, now)
	if left < 0 {
		left = 0
	}
	return left
}

// shellIdleSec — свежий AFK-датчик ПК; known=false — датчика нет.
func shellIdleSec(computerID string) (idle int, known bool) {
	if hub == nil {
		return 0, false
	}
	idleSec, age, ok := hub.ShellIdle(computerID)
	if !ok || age > idleFreshSec*time.Second {
		return 0, false
	}
	return idleSec, true
}

// startPause / resumePause — единственные точки изменения паузных полей.
func startPause(s *models.Session, now time.Time, by string) error {
	res := db.Model(&models.Session{}).
		Where("id = ? AND status = ? AND paused_at IS NULL", s.ID, models.SessionStatusActive).
		Updates(map[string]any{"paused_at": now, "paused_by": by})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errSessionGone
	}
	t := now
	s.PausedAt, s.PausedBy = &t, &by
	hub.AdminBroadcast("session_pause", map[string]any{"computer_id": s.ComputerID, "by": by})
	// Блокируем экран: на паузе биллинг не тикает, и без этой команды гость
	// ставил паузу с телефона и продолжал играть бесплатно ровно
	// pause_limit_min минут каждую сессию (ревью 26.08).
	notifyShell(s.ComputerID.String(), websocket.MsgSessionEnd, gin.H{
		"session_id": s.ID, "reason": "paused",
	})
	return nil
}

func resumePause(s *models.Session, now time.Time) error {
	if s.PausedAt == nil {
		return nil
	}
	total := s.PausedTotalSec + int(now.Sub(*s.PausedAt).Seconds())
	res := db.Model(&models.Session{}).
		Where("id = ? AND paused_at IS NOT NULL", s.ID).
		Updates(map[string]any{"paused_at": nil, "paused_by": nil, "paused_total_sec": total})
	if res.Error != nil {
		return res.Error
	}
	s.PausedAt, s.PausedBy = nil, nil
	s.PausedTotalSec = total
	hub.AdminBroadcast("session_resume", map[string]any{"computer_id": s.ComputerID})
	// Снимаем блокировку экрана, поставленную startPause.
	notifyShell(s.ComputerID.String(), websocket.MsgSessionStart, gin.H{
		"session_id": s.ID, "resumed": true,
	})
	return nil
}

// POST /me/sessions/pause — гость ставит сессию на паузу (Г2-и1).
func handlePauseSession(c *gin.Context) {
	limit := settingInt64("pause_limit_min", pauseLimitMinDef)
	if limit <= 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "pause_disabled", "message": "Пауза отключена владельцем"})
		return
	}
	var s models.Session
	if err := db.Where("user_id = ? AND status = ?", c.GetString("user_id"), models.SessionStatusActive).
		First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_active_session", "message": "Активная сессия не найдена"})
		return
	}
	if s.PausedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "already_paused", "message": "Сессия уже на паузе"})
		return
	}
	// Пока гость не нажал [Готов!], время и так не тарифицируется — пауза тут
	// не нужна и только путала расчёты (ревью 26.08).
	if sessionWaitingReady(&s) {
		c.JSON(http.StatusConflict, gin.H{"code": "not_started",
			"message": "Сессия ещё не началась — нажми «Готов!»"})
		return
	}
	now := time.Now()
	left := pauseBudgetLeftSec(&s, limit, now)
	if left <= 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "pause_exhausted",
			"message": fmt.Sprintf("Лимит паузы на сессию исчерпан (%d мин)", limit)})
		return
	}
	if err := startPause(&s, now, "guest"); err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "session_closed", "message": "Сессия уже завершена"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"paused_at": now, "budget_left_sec": left, "limit_min": limit})
}

// POST /me/sessions/resume — гость снимает паузу.
func handleResumeSession(c *gin.Context) {
	var s models.Session
	if err := db.Where("user_id = ? AND status = ?", c.GetString("user_id"), models.SessionStatusActive).
		First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_active_session", "message": "Активная сессия не найдена"})
		return
	}
	if s.PausedAt == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "not_paused", "message": "Сессия не на паузе"})
		return
	}
	if err := resumePause(&s, time.Now()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"paused_total_sec": s.PausedTotalSec})
}

// POST /admin/sessions/:id/move {computer_id} — пересадить гостя на другой
// свободный ПК клуба без разрыва биллинга (Г2-и3): железо сломалось или
// гость попросился. Тариф сессии не меняется (см. шапку файла).
func handleAdminMoveSession(c *gin.Context) {
	var session models.Session
	if err := db.First(&session, "id = ? AND status = ?", c.Param("id"), models.SessionStatusActive).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "session_not_found", "message": "Активная сессия не найдена"})
		return
	}
	var req struct {
		ComputerID string `json:"computer_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	var target models.Computer
	if err := db.First(&target, "id = ?", req.ComputerID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	if target.ID == session.ComputerID {
		c.JSON(http.StatusConflict, gin.H{"code": "same_computer", "message": "Гость уже за этим ПК"})
		return
	}
	if target.ClubID != session.ClubID {
		c.JSON(http.StatusConflict, gin.H{"code": "other_club", "message": "ПК из другого клуба"})
		return
	}
	if target.Status != models.ComputerStatusAvailable {
		c.JSON(http.StatusConflict, gin.H{"code": "computer_busy", "message": "ПК занят или в ремонте"})
		return
	}

	oldID := session.ComputerID
	var oldPC models.Computer
	db.First(&oldPC, "id = ?", oldID)

	err := db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&models.Session{}).
			Where("id = ? AND status = ?", session.ID, models.SessionStatusActive).
			Update("computer_id", target.ID)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errSessionGone
		}
		if err := tx.Model(&models.Computer{}).Where("id = ?", target.ID).
			Update("status", models.ComputerStatusInSession).Error; err != nil {
			return err
		}
		return tx.Model(&models.Computer{}).Where("id = ?", oldID).
			Update("status", models.ComputerStatusAvailable).Error
	})
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"code": "move_failed", "message": err.Error()})
		return
	}

	// Старый ПК — в лок, новый — поднимается; гостю — тост.
	var guest models.User
	db.First(&guest, "id = ?", session.UserID)
	notifyShell(oldID.String(), websocket.MsgSessionEnd, gin.H{"session_id": session.ID, "reason": "moved"})
	notifyShell(target.ID.String(), websocket.MsgSessionStart, gin.H{
		"session_id": session.ID, "user_id": session.UserID, "nickname": guest.Nickname,
	})
	notifyUser(session.UserID, "session_moved", map[string]any{"computer": target.Name})
	targetUID := session.UserID
	logAdminAction(c, "session_move", &targetUID, fmt.Sprintf("%s → %s", oldPC.Name, target.Name))
	hub.AdminBroadcast("session_move", map[string]any{
		"nickname": guest.Nickname, "from": oldPC.Name, "to": target.Name,
	})
	c.JSON(http.StatusOK, gin.H{"session_id": session.ID, "computer": target.Name})
}
