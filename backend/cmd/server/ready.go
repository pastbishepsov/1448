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
	"log"
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

// sessionWaitingReady — сессия ещё не началась: ждёт нажатия [Готов!] или
// (Ж4, Ж-Р6) подключения шелла. Состояние держим полями, а не статусом:
// статус остаётся active, иначе пришлось бы переписывать десятки выборок по
// enum (тот же приём, что у паузы Г2-и1). Оба ожидания для остальных частей
// системы одно и то же «деньги не идут, игры нет»: биллинг, пауза, прогресс
// дня, автозаписи обрыва агента и завершение (→ отмена посадки) смотрят сюда.
func sessionWaitingReady(s *models.Session) bool {
	return s.ReadyAt == nil && (s.ReadyDeadline != nil || s.ShellWaitUntil != nil)
}

// sessionWaitingShell — посадка на ПК без связи: ждём агента (Ж4-и1).
func sessionWaitingShell(s *models.Session) bool {
	return s.ReadyAt == nil && s.ShellWaitUntil != nil
}

// shellArrived — агент подключился, пока посадка ждала его: с ЭТОГО момента
// начинается обычное окно [Готов!] (гость ещё идёт к машине); окно выключено
// настройкой — старт сразу. started_at сдвигается сюда же, как в confirmReady.
// CAS по shell_wait_until IS NOT NULL: биллинг и колбэк хаба могут прийти
// одновременно — переход случится один раз.
func shellArrived(s *models.Session, now time.Time) error {
	dl := readyDeadlineFor(now, settingInt64("ready_wait_min", readyWaitMinDef))
	upd := map[string]any{"shell_wait_until": nil, "ready_deadline": dl, "started_at": now,
		"paused_at": nil, "paused_by": nil, "paused_total_sec": 0}
	if dl == nil {
		upd["ready_at"] = now
	}
	res := db.Model(&models.Session{}).
		Where("id = ? AND status = ? AND shell_wait_until IS NOT NULL", s.ID, models.SessionStatusActive).
		Updates(upd)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return errSessionGone
	}
	s.ShellWaitUntil = nil
	s.ReadyDeadline = dl
	s.StartedAt = now
	if dl == nil {
		t := now
		s.ReadyAt = &t
	}
	// Команда на ПК ушла в пустоту при посадке (шелла не было) — повторяем
	// теперь, когда он на связи: экран покажет окно [Готов!].
	startPayload := gin.H{"session_id": s.ID, "started_at": s.StartedAt}
	if sp := sensPayloadFor(s.UserID); sp != nil {
		startPayload["sens"] = sp
	}
	notifyShell(s.ComputerID.String(), websocket.MsgSessionStart, startPayload)
	hub.AdminBroadcast("session", map[string]any{"kind": "shell_arrived", "session_id": s.ID,
		"computer_id": s.ComputerID.String(), "ready_deadline": dl})
	return nil
}

// shellArrivedForPC — колбэк хаба (агент подключился): если на этом ПК посадка
// ждала шелл — переводим её в окно [Готов!] сразу, не дожидаясь тика биллинга.
func shellArrivedForPC(computerID string) {
	pcID, err := uuid.Parse(computerID)
	if err != nil || db == nil {
		return
	}
	var rows []models.Session
	db.Where("computer_id = ? AND status = ? AND shell_wait_until IS NOT NULL AND ready_at IS NULL",
		pcID, models.SessionStatusActive).Limit(1).Find(&rows)
	if len(rows) == 0 {
		return
	}
	if err := shellArrived(&rows[0], time.Now()); err != nil && !errors.Is(err, errSessionGone) {
		log.Printf("посадка: переход из ожидания шелла не прошёл (%s): %v", rows[0].ID, err)
	}
}

// shellWaitExpired — шелл так и не появился: посадка снимается без списания
// (cancelled, ended_reason shell_offline), админкам — сигнал тостом, гостю —
// уведомление. Общая точка с seat-cancel — cancelWaitingSession.
func shellWaitExpired(s *models.Session, now time.Time) error {
	res, err := cancelWaitingSession(s, now, "shell_offline")
	if err != nil {
		return err
	}
	var pc models.Computer
	db.Select("name").First(&pc, "id = ?", s.ComputerID)
	notifyUser(s.UserID, "seat_cancelled", map[string]any{"reason": "shell_offline", "computer": pc.Name})
	hub.AdminBroadcast("session", map[string]any{"kind": "shell_timeout", "session_id": s.ID,
		"computer_id": s.ComputerID.String(), "computer": pc.Name, "nickname": res.User.Nickname})
	return nil
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
// Ревью 26.08: вместе со сдвигом started_at обнуляем паузный счётчик. Пауза,
// начатая во время ОЖИДАНИЯ (до нажатия [Готов!]), лежит вне оплачиваемого
// интервала, и её секунды вычитались из каждого последующего расчёта —
// гость получал столько же бесплатных минут в каждой сессии.
func confirmReady(s *models.Session, at time.Time) error {
	err := db.Model(&models.Session{}).Where("id = ?", s.ID).Updates(map[string]any{
		"ready_at":         at,
		"started_at":       at,
		"paused_at":        nil,
		"paused_by":        nil,
		"paused_total_sec": 0,
		"shell_wait_until": nil, // Ж4: нажал [Готов!] с экрана — значит, машина живая, ждать шелл незачем
	}).Error
	if err != nil {
		return err
	}
	t := at
	s.ReadyAt = &t
	s.ShellWaitUntil = nil
	s.StartedAt = at
	s.PausedAt, s.PausedBy = nil, nil
	s.PausedTotalSec = 0
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
		respondDBError(c, err)
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
	res, err := cancelWaitingSession(&s, now, "seat_cancel")
	if err != nil {
		if errors.Is(err, errSessionGone) {
			c.JSON(http.StatusConflict, gin.H{"code": "already_playing",
				"message": "Гость успел нажать [Готов!] — отменять уже нечего"})
			return
		}
		respondDBError(c, err)
		return
	}

	notifyUser(s.UserID, "seat_cancelled", map[string]any{})
	target := s.UserID
	logAdminAction(c, "seat_cancel", &target, res.User.Nickname+": посадка отменена до нажатия [Готов!]")

	c.JSON(http.StatusOK, gin.H{"session_id": s.ID, "status": models.SessionStatusCancelled})
}

// cancelWaitingSession — отмена посадки, пока гость не нажал [Готов!]: общая
// точка для seat-cancel у стойки и для любого завершения в окне ожидания
// (finishSession, К0-и3). Это НЕ завершение: минут ноль, XP/монет/кейсов нет,
// сессия дня не засчитывается, прогресс ачивок не пишется. ended_reason —
// кто отменил (seat_cancel | manual | admin), статус — cancelled.
func cancelWaitingSession(s *models.Session, now time.Time, reason string) (*finishResult, error) {
	var user models.User
	db.First(&user, "id = ?", s.UserID)

	err := db.Transaction(func(tx *gorm.DB) error {
		// Закрываем только пока active и до нажатия — гость может нажать
		// [Готов!] ровно сейчас, и тогда это уже настоящее завершение (та же
		// защита от гонки трёх акторов, что в finishSession).
		res := tx.Model(&models.Session{}).
			Where("id = ? AND status = ? AND ready_at IS NULL", s.ID, models.SessionStatusActive).
			Updates(map[string]any{
				"status":       models.SessionStatusCancelled,
				"ended_at":     now,
				"minutes_used": 0,
				"ended_reason": reason,
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
		return nil, err
	}
	s.Status = models.SessionStatusCancelled
	s.EndedAt = &now
	s.MinutesUsed = 0
	s.EndedReason = &reason

	// Экрану гостя — снять окно (если Shell на связи), админкам — живое
	// событие: плитка ПК должна освободиться сразу.
	notifyShell(s.ComputerID.String(), websocket.MsgSessionEnd, gin.H{"session_id": s.ID})
	hub.AdminBroadcast("session_end", map[string]any{
		"computer_id": s.ComputerID.String(),
		"nickname":    user.Nickname,
		"minutes":     0,
	})
	checkWaitlistNotify(s.ClubID) // Б9: ПК освободился — зовём очередь

	rank, _ := accountRankFor(userHoursPlayed(s.UserID.String()))
	return &finishResult{Session: s, Rank: rank, User: user,
		Receipt: receiptFor(s.UserID, user.WalletGrosz, now)}, nil
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
		respondDBError(c, err)
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
