package main

// Е1 «Готов!» (OPERATOR.md, этап I) — окно подтверждения перед стартом сессии.
//
// Лист основателя: «чтобы не списывать коинсы за то, что он идёт к компу».
// Гостя сажают у стойки, а к машине он ещё идёт через зал; до этого спринта
// деньги текли с момента посадки. Теперь между посадкой и первой оплаченной
// минутой стоит окно: пока гость не нажал [Готов!], сессия числится, ПК занят,
// но кошелёк не трогаем.
//
// Р1 основателя: не нажал за ready_wait_min — списание стартует САМО, посадку
// НЕ снимаем. Гость мог отойти или заговориться; клуб всё это время держит за
// ним машину, и с какого-то момента она должна оплачиваться. Снять посадку —
// решение админа, а не автомата.

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

const readyWaitMinDef = 7 // дефолт настройки ready_wait_min (лист: «не больше 7 минут!»)

// readyDeadlineFor — до какого момента ждём нажатия (чистая, тест).
// Ноль минут — окно выключено: сессия начинается сразу, как было до Е1.
func readyDeadlineFor(now time.Time, waitMin int64) *time.Time {
	if waitMin <= 0 {
		return nil
	}
	d := now.Add(time.Duration(waitMin) * time.Minute)
	return &d
}

// sessionWaitingReady — сессия ждёт нажатия (чистая, тест).
// Состояние держим полями, а не статусом: статус остаётся active, иначе
// пришлось бы переписывать десятки выборок по enum (тот же приём, что у
// паузы Г2-и1).
func sessionWaitingReady(s *models.Session) bool {
	return s.ReadyDeadline != nil && s.ReadyAt == nil
}

// confirmReady — перевести ожидающую сессию в оплачиваемую, начиная с момента
// at. ИМЕННО ЗДЕСЬ сдвигается started_at: все потребители времени сессии
// (биллинг, XP, пауза, AFK, ачивки, прогноз «до HH:MM») считают от него, и
// сдвинуть одну точку честнее, чем учить каждого вычитать ожидание.
//
// Дедлайн чужой брони (Г3-и2) на started_at НЕ смотрит — он считается от
// времени самой брони, поэтому ожидание не съедает чужое время. Это
// закреплено юнитом: ошибка здесь означала бы, что гость, пришедший позже,
// отъедает у следующего.
func confirmReady(s *models.Session, at time.Time) error {
	err := db.Model(&models.Session{}).Where("id = ?", s.ID).Updates(map[string]any{
		"ready_at":   at,
		"started_at": at,
	}).Error
	if err != nil {
		return err
	}
	t := at
	s.ReadyAt = &t
	s.StartedAt = at
	return nil
}

// POST /me/sessions/ready — гость нажал [Готов!] за компьютером (Е1).
//
// Идемпотентно: повторное нажатие (двойной клик, перезагрузка экрана) не
// сдвигает точку отсчёта второй раз и не считается ошибкой.
func handleSessionReady(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	var s models.Session
	if err := db.Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		First(&s).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_session", "message": "Активной сессии нет"})
		return
	}
	if !sessionWaitingReady(&s) {
		c.JSON(http.StatusOK, gin.H{
			"session_id": s.ID, "started_at": s.StartedAt,
			"ready_at": s.ReadyAt, "already": true,
		})
		return
	}
	now := time.Now()
	if err := confirmReady(&s, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	hub.AdminBroadcast("session", map[string]any{"kind": "ready", "session_id": s.ID})
	c.JSON(http.StatusOK, gin.H{
		"session_id": s.ID, "started_at": s.StartedAt, "ready_at": s.ReadyAt, "already": false,
	})
}

// POST /admin/sessions/:id/seat-cancel — отменить посадку, пока гость не
// нажал [Готов!] (Е1).
//
// Это НЕ завершение сессии: денег не брали, минут не было, XP и монет не
// начисляем — иначе «гость передумал у стойки» попал бы в статистику как
// сыгранная сессия. Статус cancelled уже есть в enum, новый заводить не нужно.
func handleAdminSeatCancel(c *gin.Context) {
	var s models.Session
	if err := db.First(&s, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "session_not_found", "message": "Сессия не найдена"})
		return
	}
	if s.Status != models.SessionStatusActive {
		c.JSON(http.StatusConflict, gin.H{"code": "not_active", "message": "Сессия уже закрыта"})
		return
	}
	if !sessionWaitingReady(&s) {
		c.JSON(http.StatusConflict, gin.H{"code": "already_playing",
			"message": "Гость уже за игрой — это завершение сессии, а не отмена посадки"})
		return
	}

	now := time.Now()
	var user models.User
	db.First(&user, "id = ?", s.UserID)

	err := db.Transaction(func(tx *gorm.DB) error {
		// Закрываем только пока active — гость может нажать [Готов!] ровно
		// сейчас, и тогда отменять уже нечего (та же защита от гонки трёх
		// акторов, что в finishSession).
		res := tx.Model(&models.Session{}).
			Where("id = ? AND status = ? AND ready_at IS NULL", s.ID, models.SessionStatusActive).
			Updates(map[string]any{
				"status":       models.SessionStatusCancelled,
				"ended_at":     now,
				"minutes_used": 0,
				"ended_reason": "seat_cancel",
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errSessionGone
		}
		return tx.Model(&models.Computer{}).Where("id = ?", s.ComputerID).
			Update("status", models.ComputerStatusAvailable).Error
	})
	if err != nil {
		if errors.Is(err, errSessionGone) {
			c.JSON(http.StatusConflict, gin.H{"code": "already_playing",
				"message": "Гость успел нажать [Готов!] — отменять уже нечего"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	// Экрану гостя — снять окно (если Shell на связи), гостю — уведомление,
	// админкам — живое событие: плитка ПК должна освободиться сразу.
	notifyShell(s.ComputerID.String(), websocket.MsgSessionEnd, gin.H{"session_id": s.ID})
	notifyUser(s.UserID, "seat_cancelled", map[string]any{})
	target := s.UserID
	logAdminAction(c, "seat_cancel", &target, user.Nickname+": посадка отменена до нажатия [Готов!]")
	hub.AdminBroadcast("session_end", map[string]any{
		"computer_id": s.ComputerID.String(),
		"nickname":    user.Nickname,
		"minutes":     0,
	})
	checkWaitlistNotify(s.ClubID) // Б9: ПК освободился — зовём очередь

	c.JSON(http.StatusOK, gin.H{"session_id": s.ID, "status": models.SessionStatusCancelled})
}

// ── Е1-и5: придержать ПК под регистрацию нового гостя ─────────────────────
//
// Новичка нельзя «посадить»: аккаунта ещё нет, а сажают по нику. Админ
// показывает на машину и говорит «заводи там», но для системы она всё это
// время свободна — и её законно занимает следующий гость или второй админ.
// Метка hold_until решает ровно это: ПК исчезает из АВТОМАТИЧЕСКОГО выбора,
// но явный старт именно на нём проходит и метку снимает. Иначе вышло бы, что
// держим машину для гостя и ему же не даём сесть.

// computerHeld — действует ли придержание (чистая, тест). Истёкшая метка
// ничего не значит: срок сам отпускает ПК, фоновый джоб не нужен.
func computerHeld(pc *models.Computer, now time.Time) bool {
	return pc.HoldUntil != nil && now.Before(*pc.HoldUntil)
}

// releaseHold — снять метку (после старта сессии или вручную).
func releaseHold(pcID uuid.UUID) {
	db.Model(&models.Computer{}).Where("id = ?", pcID).
		Updates(map[string]any{"hold_until": nil, "hold_by": nil})
}

// POST /admin/computers/:id/hold — придержать под регистрацию (Е1-и5).
// Держим на те же ready_wait_min: это тот же по смыслу бюджет «человек
// сейчас этим занят», и второй настройки для него заводить незачем.
func handleAdminComputerHold(c *gin.Context) {
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	if pc.Status != models.ComputerStatusAvailable {
		c.JSON(http.StatusConflict, gin.H{"code": "computer_busy",
			"message": "Придержать можно только свободный ПК"})
		return
	}
	wait := settingInt64("ready_wait_min", readyWaitMinDef)
	if wait <= 0 {
		wait = readyWaitMinDef // окно [Готов!] выключено, но держать всё равно надо
	}
	until := time.Now().Add(time.Duration(wait) * time.Minute)
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	if err := db.Model(&models.Computer{}).Where("id = ?", pc.ID).
		Updates(map[string]any{"hold_until": until, "hold_by": adminID}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	logAdminAction(c, "pc_hold", nil, pc.Name+": придержан под регистрацию")
	hub.AdminBroadcast("computer", map[string]any{"kind": "hold", "computer_id": pc.ID.String()})
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "name": pc.Name, "hold_until": until})
}

// DELETE /admin/computers/:id/hold — снять придержание вручную (Е1-и5).
func handleAdminComputerUnhold(c *gin.Context) {
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	releaseHold(pc.ID)
	logAdminAction(c, "pc_unhold", nil, pc.Name+": придержание снято")
	hub.AdminBroadcast("computer", map[string]any{"kind": "unhold", "computer_id": pc.ID.String()})
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "name": pc.Name})
}
