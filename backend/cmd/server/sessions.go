package main

import (
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

// Дефолты начисления. Рабочие значения владелец задаёт в админке —
// таблица settings (миграция 017), чтение через settingInt64 (спринт А5).
const (
	xpPerMinute    int64 = 10
	coinsPerMinute int64 = 2
)

type startSessionRequest struct {
	ComputerID *string `json:"computer_id"` // К0-и6: обязателен — гость стартует только со своей машины
	PlannedMin *int    `json:"planned_min"` // Г3: сколько планирует сидеть (для ПК с бронью); пусто = 30
}

type endSessionRequest struct {
	Minutes *int `json:"minutes"` // dev-оверрайд; в production время берётся из факта
}

// applyXP — начисляет опыт и обрабатывает повышение уровня.
// XP для перехода на следующий уровень: XP(n) = 1000 * n^1.4, округлено до 100
// (см. models.XPForNextLevel; правило цифр — DESIGN.md).
func applyXP(u *models.User, gained int64) int {
	if gained <= 0 {
		return 0
	}
	startLevel := u.Level
	u.XPTotal += gained
	u.XPCurrent += gained
	for u.XPCurrent >= models.XPForNextLevel(u.Level) {
		u.XPCurrent -= models.XPForNextLevel(u.Level)
		u.Level++
		u.SkillpointsAvailable++ // одно очко навыка за уровень
	}
	return u.Level - startLevel
}

// boostedXP — базовый опыт с учётом таланта (effect — доля прибавки, напр. 0.30 = +30%).
func boostedXP(baseXP int64, effect float64) int64 {
	if effect <= 0 {
		return baseXP
	}
	return int64(math.Round(float64(baseXP) * (1 + effect)))
}

// walletLowMessage — отказ в старте словами гостя (К3-и3, №18): сколько
// нужно, сколько есть; минуты монетами и пакетом упоминаются, только если
// они есть — «0 мин монетами и 0 мин пакетом» ничего не объясняло. Читается
// и с экрана посадки в админке: админ видит те же цифры.
func walletLowMessage(minStart int64, needPLN, walletPLN float64, coinMin, packMin int64) string {
	msg := fmt.Sprintf("Не хватает на старт: нужно ~%s zł на %d мин, на кошельке %s zł", plnRU(needPLN), minStart, plnRU(walletPLN))
	if coinMin > 0 {
		msg += fmt.Sprintf(" + %d мин монетами", coinMin)
	}
	if packMin > 0 {
		msg += fmt.Sprintf(" + %d мин пакетом", packMin)
	}
	return msg + ". Пополни у стойки"
}

// economyRates — ставки начисления из настроек клуба (settings, А5) с
// дефолтами кода. Одна точка чтения для finishSession и GET /me/economy
// (К3-и1): раньше калькулятор отдавал константы и расходился с начислением,
// стоило владельцу поменять ставку в админке.
type economyRateSet struct {
	XPPerMin, CoinsPerMin, CoinsPerPLN int64
}

func economyRates() economyRateSet {
	return economyRateSet{
		XPPerMin:    settingInt64("xp_per_min", xpPerMinute),
		CoinsPerMin: settingInt64("coins_per_min", coinsPerMinute),
		CoinsPerPLN: settingInt64("coins_per_pln", coinsPerPLN),
	}
}

// sessionRewards — XP и монеты за minutes активной игры (чистая функция, тест).
// XP: база × (1 + xp_boost) × множитель ранга; монеты: база × множитель ранга.
// Итог кратен 5 (правило цифр). Калькулятор на клиенте считает по той же
// формуле из /me/economy — при любых ставках цифры обязаны совпадать.
func sessionRewards(minutes int, r economyRateSet, xpBoost float64, rank AccountRank) (xp, coins int64) {
	xp = models.RoundToStep(int64(math.Round(float64(boostedXP(int64(minutes)*r.XPPerMin, xpBoost))*rank.XPMult)), 5)
	coins = models.RoundToStep(int64(math.Round(float64(int64(minutes)*r.CoinsPerMin)*rank.CoinMult)), 5)
	return xp, coins
}

// POST /me/sessions/start — начать игровую сессию за компьютером.
func handleStartSession(c *gin.Context) {
	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "invalid_user", "message": "Некорректный пользователь"})
		return
	}
	// Е0-и2: с временным паролем гость сам не садится — его знает и админ,
	// который пароль выдал. Проверка стоит ЗДЕСЬ, а не в startSessionFor:
	// посадку админом (Б8) она блокировать не должна, у стойки человека
	// опознали лично, и это надёжнее пароля.
	var u models.User
	if db.Select("must_change_password").First(&u, "id = ?", userID).Error == nil && u.MustChangePassword {
		c.JSON(http.StatusConflict, gin.H{"code": "password_change_required",
			"message": "Сначала задай свой пароль вместо временного"})
		return
	}
	if rejectBanned(c, userID) { // К0-и4: бан действует сразу, а не через 15 мин токена
		return
	}
	var req startSessionRequest
	_ = c.ShouldBindJSON(&req) // тело необязательно
	// К0-и6 (KIOSK.md): без computer_id сервер брал первый свободный ПК по
	// алфавиту — гость за ПК-2 получал сессию на ПК-1 (баг 26.08 №5 закрывался
	// только через /ping Go-агента; в C#-киоске без агента номера нет). PWA
	// сессий не начинает (в app.html только /ready), так что гостевой старт —
	// всегда со своей машины, и «угадывать» больше нечего.
	if req.ComputerID == nil || *req.ComputerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "computer_required",
			"message": "ПК не определён — позови админа, он посадит тебя у стойки"})
		return
	}
	// Р10 (KIOSK.md): гость сам нажал «Начать сессию» у ПК — он уже за ним,
	// окно [Готов!] ему не нужно; окно остаётся для посадки от стойки.
	code, resp := startSessionFor(userID, req.ComputerID, req.PlannedMin, true)
	c.JSON(code, resp)
}

// startSessionFor — общий старт сессии (Б8): гость сам (/me/sessions/start)
// или админ сажает его у стойки (/admin/computers/:id/session). Скидки и
// таланты всегда гостя. Возвращает HTTP-код и готовое тело ответа.
// readyNow — точка отсчёта денег ставится сразу (гость сам стартовал с ПК,
// Р10 KIOSK.md); false — окно [Готов!] по ready_wait_min (посадка от стойки, Е1).
func startSessionFor(userID uuid.UUID, computerID *string, plannedMin *int, readyNow bool) (int, gin.H) {
	// Уже есть активная сессия?
	var activeCount int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		Count(&activeCount)
	if activeCount > 0 {
		return http.StatusConflict, gin.H{"code": "session_active", "message": "У гостя уже есть активная сессия"}
	}

	// Выбрать компьютер: по id или первый свободный. Г3 (Р3 GUEST.md):
	// «свободный» — значит ещё и без конфликтной брони. Планируемое время
	// (planned_min, без него — минимальный сеанс 30 мин) должно влезать в
	// окно до чужой брони МИНУС буфер; за booking_lock_min до брони ПК
	// придержан под её хозяина. Хозяин садится всегда — его бронь гасится
	// клеймом (seated) после старта.
	now := time.Now()
	bkBuffer := settingInt64("booking_buffer_min", bookingBufferMinDef)
	bkLock := settingInt64("booking_lock_min", bookingLockMinDef)
	planned := defaultPlannedMin
	if plannedMin != nil && *plannedMin > 0 {
		planned = *plannedMin
	}

	explicit := computerID != nil && *computerID != ""
	var candidates []models.Computer
	if explicit {
		var one models.Computer
		if err := db.First(&one, "id = ?", *computerID).Error; err != nil {
			return http.StatusNotFound, gin.H{"code": "no_computer", "message": "Нет доступного компьютера"}
		}
		candidates = []models.Computer{one}
	} else {
		if err := db.Where("status = ?", models.ComputerStatusAvailable).
			Order("name").Find(&candidates).Error; err != nil || len(candidates) == 0 {
			return http.StatusNotFound, gin.H{"code": "no_computer", "message": "Нет доступного компьютера"}
		}
	}

	var computer models.Computer
	picked := false
	for i := range candidates {
		pc := &candidates[i]
		if pc.Status != models.ComputerStatusAvailable {
			if explicit {
				return http.StatusConflict, gin.H{"code": "computer_busy", "message": "Компьютер занят"}
			}
			continue
		}
		// Е1-и5: ПК придержан под регистрацию — из АВТОМАТИЧЕСКОГО выбора он
		// уходит, но явный старт именно на нём проходит: держим-то мы его как
		// раз для того, кто сейчас за ним заводит аккаунт.
		if !explicit && computerHeld(pc, now) {
			continue
		}
		if nb := nextRelevantBooking(pc.ID, now); nb != nil && nb.UserID != userID {
			if isBookingLocked(nb.StartTime, now, bkLock) {
				if explicit {
					return http.StatusConflict, gin.H{"code": "computer_reserved",
						"message": "ПК придержан под бронь к " + nb.StartTime.Local().Format("15:04")}
				}
				continue
			}
			if w := seatWindowMin(nb.StartTime, now, bkBuffer); planned > w {
				if explicit {
					return http.StatusConflict, gin.H{"code": "booking_soon", "message": fmt.Sprintf(
						"До брони (%s) свободно %d мин с учётом буфера — планируй меньше или выбери другой ПК",
						nb.StartTime.Local().Format("15:04"), w)}
				}
				continue
			}
		}
		computer = *pc
		picked = true
		break
	}
	if !picked {
		return http.StatusConflict, gin.H{"code": "no_computer_window", "message": fmt.Sprintf(
			"Свободного ПК под %d минут нет — всё занято или скоро забронировано", planned)}
	}

	var club models.Club
	if err := db.First(&club, "id = ?", computer.ClubID).Error; err != nil {
		return http.StatusInternalServerError, gin.H{"code": "club_missing", "message": "Клуб не найден"}
	}

	// Скидка на тариф: кэшбек игрока (бустеры кейсов) + талант cashback_master;
	// ночью (22:00–07:59) дополнительно действует талант night_mode.
	var user models.User
	_ = db.First(&user, "id = ?", userID).Error
	discountPct := user.PaymentIncreasePct + talentEffect(userID.String(), "cashback_master")*100
	if isNightHour(time.Now().In(clubLocation).Hour()) {
		discountPct += talentEffect(userID.String(), "night_mode") * 100
	}
	if discountPct > maxDiscountPct {
		discountPct = maxDiscountPct
	}
	// В4-5: час стоит столько, сколько стоит ЗОНА этого ПК; клубный тариф
	// остаётся запасным вариантом для машин без зоны.
	baseRate := rateForComputer(zoneRateOf(&computer), club.BaseRatePLN)
	rate := effectiveRate(baseRate, discountPct)

	// Г1-и3 (Р8 GUEST.md): порог старта — кошелёк + минутный запас монет
	// должны покрывать min_start_minutes по ставке этого ПК. Пустой гость
	// получает понятный отказ и у киоска, и при посадке админом (админ
	// сначала проводит депозит, потом сажает).
	if minStart := settingInt64("min_start_minutes", minStartMinutesDef); minStart > 0 {
		rateGrosz := models.GroszFromPLN(rate)
		// Е2-и3: минуты пакета этой зоны — такой же запас, как монеты. Без них
		// гость с пятичасовым пакетом и пустым кошельком не смог бы сесть за
		// машину, время на которой он уже оплатил.
		packMin := livePackMinutes(user.ID, computer.ZoneID, now)
		// Ж2 (Ж-Р3): зонные минуты — только ЭТОЙ зоны: минуты «в Standard» на
		// VIP-машину не пускают, как и пакет чужой зоны.
		reserveMin := user.CoinMinutes + liveGrantMinutes(user.ID, computer.ZoneID)
		covered := int64(minutesLeft(reserveMin, packMin, user.WalletGrosz, rateGrosz, 0))
		if covered < minStart {
			needPLN := models.PLNFromGrosz(costForMinutes(rateGrosz, int(minStart)))
			return http.StatusConflict, gin.H{"code": "wallet_low",
				"message": walletLowMessage(minStart, needPLN, models.PLNFromGrosz(user.WalletGrosz), reserveMin, packMin)}
		}
	}

	// Е1: окно [Готов!]. До нажатия деньги не текут — гость ещё идёт к машине.
	// started_at ставим сейчас и СДВИНЕМ при подтверждении (confirmReady):
	// так все потребители времени сессии продолжают читать одно поле.
	readyDeadline := readyDeadlineFor(now, settingInt64("ready_wait_min", readyWaitMinDef))
	var readyAt *time.Time
	if readyNow { // Р10: сам за ПК — время идёт с этой секунды, окна нет
		readyDeadline = nil
		t := now
		readyAt = &t
	}
	// Ж4-и1 (Ж-Р6): посадка от стойки на ПК без связи. Окно [Готов!] не
	// открываем — нажать на тёмном экране некому, а через ready_wait_min оно
	// стартовало бы само и списывало деньги за пустую машину (ревью 14.09,
	// №9–10). Вместо этого ждём шелл те же ready_wait_min: подключился —
	// окно начинается с того момента (shellArrived), нет — посадка снимается
	// без списания (shellWaitExpired). Ж4-и2: заодно сами будим машину по
	// сети, если есть MAC и живой сосед-агент.
	var shellWait *time.Time
	wol := ""
	if !readyNow && readyDeadline != nil && !hub.IsConnected(computer.ID.String()) {
		shellWait, readyDeadline = readyDeadline, nil
		wol = autoWakeOnSeat(&computer)
	}

	session := models.Session{
		UserID:           userID,
		ComputerID:       computer.ID,
		ClubID:           club.ID,
		Status:           models.SessionStatusActive,
		StartedAt:        now,
		ReadyDeadline:    readyDeadline,
		ReadyAt:          readyAt,
		ShellWaitUntil:   shellWait,
		BaseRatePLN:      baseRate,
		EffectiveRatePLN: rate,
	}

	// Сессия началась — придержание своё дело сделало (Е1-и5).
	if computerHeld(&computer, now) {
		releaseHold(computer.ID)
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		// Статус ПК занимаем ПЕРВЫМ и с условием: проверка выше делалась вне
		// транзакции, и два одновременных запроса (гость с киоска + посадка
		// админом) успевали создать две активные сессии на одной машине
		// (ревью 26.08). Проигравший получает 409 computer_busy.
		res := tx.Model(&models.Computer{}).
			Where("id = ? AND status = ?", computer.ID, models.ComputerStatusAvailable).
			Update("status", models.ComputerStatusInSession)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errComputerBusy
		}
		return tx.Omit("User", "Computer").Create(&session).Error
	})
	if errors.Is(err, errComputerBusy) {
		return http.StatusConflict, gin.H{"code": "computer_busy", "message": "Компьютер уже занят"}
	}
	if isDuplicate(err) {
		// К0-и6: двойной тап/ретрай проскочил проверку activeCount выше и
		// упёрся в уникальный индекс активной сессии (миграция 053) — это не
		// «ошибка базы», а «сессия уже идёт».
		return http.StatusConflict, gin.H{"code": "session_active", "message": "Сессия уже идёт"}
	}
	if err != nil {
		log.Printf("db_error старт сессии: %v", err) // К3-и3: гостю — фраза, причина — в лог
		return http.StatusInternalServerError, gin.H{"code": "db_error", "message": dbErrorMessage}
	}

	// Команда на ПК в реальном времени (если Shell подключён). К7: с профилем
	// сенсы гостя — агент подставит её в конфиги игр до первого запуска.
	startPayload := gin.H{
		"session_id": session.ID,
		"started_at": session.StartedAt,
	}
	if sp := sensPayloadFor(userID); sp != nil { // С2-и1: пересчитано под мышь клуба (mouse_dpi)
		startPayload["sens"] = sp
	}
	notifyShell(computer.ID.String(), websocket.MsgSessionStart, startPayload)

	// Live-событие в админку (спринт А6).
	hub.AdminBroadcast("session_start", map[string]any{
		"computer_id": computer.ID.String(),
		"computer":    computer.Name,
		"nickname":    user.Nickname,
	})

	// Вейтлист (Б9): гость сел — его место в очереди закрывается само,
	// каким бы путём ни началась сессия (сам с киоска, посадка у стойки).
	fromWaitlist := resolveWaitlistOnSeat(userID, user.Nickname)

	// Г3-и3: посадка хозяина гасит его ближайшую бронь на этом ПК (seated).
	claimed := claimBookingOnSeat(computer.ID, userID, now)

	checkBirthdayGift(userID)     // Г8-и4: сел играть в день рождения — подарок
	applyStreakFreezes(userID, now) // Г6-и4: вернулся — заморозки латают пропуск

	resp := gin.H{
		"session_id":         session.ID,
		"started_at":         session.StartedAt,
		"computer":           computer.Name,
		"club":               club.Name,
		"rate_pln":           baseRate,
		"effective_rate_pln": rate,
		"discount_pct":       discountPct,
		"from_waitlist":      fromWaitlist,
		"booking_claimed":    claimed != nil,
		"ready_deadline":     session.ReadyDeadline, // Е1: nil — окно выключено или старт с ПК (Р10)
		"ready_at":           session.ReadyAt,       // К0-и6: сам стартовал — время уже идёт
	}
	if shellWait != nil { // Ж4: ждём шелл — стойке честно, что машина не на связи и что с WoL
		resp["shell_wait_until"] = shellWait
		resp["wol"] = wol
		resp["shell_note"] = shellWaitNote(wol, settingInt64("ready_wait_min", readyWaitMinDef))
	}
	return http.StatusCreated, resp
}

// autoWakeOnSeat — Ж4-и2: при посадке на спящий ПК шлём WoL сами, той же
// логикой, что кнопка «Включить» (handleAdminPCPower on): нужен MAC машины и
// хоть один живой агент в клубе, который отправит магический пакет по сети.
// Возвращает исход словом: sent | no_mac | no_agents | send_failed.
func autoWakeOnSeat(pc *models.Computer) string {
	if pc.MAC == nil || *pc.MAC == "" {
		return "no_mac"
	}
	proxyID, ok := hub.AnyClientInClub(pc.ClubID.String())
	if !ok {
		return "no_agents"
	}
	if err := hub.Send(proxyID, websocket.MsgWOL, gin.H{"mac": *pc.MAC}); err != nil {
		return "send_failed"
	}
	return "sent"
}

// shellWaitNote — что сказать стойке о посадке на ПК без связи (чистая, тест).
func shellWaitNote(wol string, waitMin int64) string {
	base := fmt.Sprintf("ПК не на связи — ждём его %d мин, деньги не идут", waitMin)
	switch wol {
	case "sent":
		return base + "; включаю по сети (WoL)"
	case "no_mac":
		return base + "; включить по сети не могу — у ПК нет MAC, включи руками"
	case "no_agents":
		return base + "; включить по сети не могу — в клубе нет живых агентов, включи руками"
	case "send_failed":
		return base + "; WoL не отправился, включи руками"
	}
	return base
}

// POST /me/sessions/end — завершить активную сессию и начислить XP/coins.
func handleEndSession(c *gin.Context) {
	userID := c.GetString("user_id")

	var req endSessionRequest
	_ = c.ShouldBindJSON(&req)

	var session models.Session
	if err := db.Where("user_id = ? AND status = ?", userID, models.SessionStatusActive).
		First(&session).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_active_session", "message": "Активная сессия не найдена"})
		return
	}

	res, err := finishSession(&session, req.Minutes, "manual")
	if err != nil {
		if errors.Is(err, errSessionGone) {
			c.JSON(http.StatusConflict, gin.H{"code": "session_closed", "message": "Сессия уже завершена"})
			return
		}
		respondDBError(c, err)
		return
	}
	c.JSON(http.StatusOK, finishResponse(res))
}

// finishResult — итог завершения сессии (общий для игрока и админа).
type finishResult struct {
	Session        *models.Session
	Minutes        int
	XPGained       int64
	CoinsGained    int64
	DailyBonusXP   int64
	LevelsGained   int
	BonusCase      bool
	BonusCaseTier  models.CaseTier
	BonusCaseTier2 models.CaseTier
	Rank           AccountRank
	User           models.User
	Receipt        receipt // К3-и6 (Р5): чек на экран «Сессия завершена»
}

// receipt — что гость должен знать, вставая из-за ПК (К3-и6, Р5): остаток
// кошелька, долг кухни (постоплата, Р10 трека Г) и стрик визитов. Те же три
// числа отдаёт GET /me/sessions после внешнего завершения — киоск показывает
// один и тот же экран независимо от того, кто закрыл сессию.
type receipt struct {
	WalletGrosz      int64   `json:"wallet_grosz"`
	KitchenUnpaidPLN float64 `json:"kitchen_unpaid_pln"`
	StreakDays       int     `json:"streak_days"`
}

// receiptFor — чек по текущему состоянию гостя.
func receiptFor(userID uuid.UUID, walletGrosz int64, now time.Time) receipt {
	_, kSum := unpaidKitchen(userID)
	return receipt{
		WalletGrosz:      walletGrosz,
		KitchenUnpaidPLN: kSum,
		StreakDays:       gatherPeriodicStats(userID, now).VisitStreak,
	}
}

func finishResponse(r *finishResult) gin.H {
	return gin.H{
		"session_id":        r.Session.ID,
		"receipt":           r.Receipt,
		"minutes":           r.Minutes,
		"xp_earned":         r.XPGained,
		"coins_earned":      r.CoinsGained,
		"daily_bonus_xp":    r.DailyBonusXP,
		"levels_gained":     r.LevelsGained,
		"bonus_case":        r.BonusCase,
		"bonus_case_tier":   r.BonusCaseTier,
		"bonus_case_tier_2": r.BonusCaseTier2,
		"rank":              gin.H{"level": r.Rank.Level, "name": r.Rank.Name, "xp_mult": r.Rank.XPMult, "coin_mult": r.Rank.CoinMult},
		"user": gin.H{
			"level":                 r.User.Level,
			"xp_current":            r.User.XPCurrent,
			"xp_total":              r.User.XPTotal,
			"xp_for_next_level":     models.XPForNextLevel(r.User.Level),
			"coins_balance":         r.User.CoinsBalance,
			"skillpoints_available": r.User.SkillpointsAvailable,
		},
	}
}

// finishSession — единая логика завершения: финальный расчёт кошелька,
// начисление XP/coins, уровни, кейсы, достижения, освобождение ПК, команда
// Shell. Используется игроком (/me/sessions/end), админом
// (/admin/sessions/:id/end, бан) и биллингом Г1 (ноль кошелька).
// reason — причина завершения (ended_reason): manual | admin | balance
// (Г3 добавит booking, Г2 — afk).
func finishSession(session *models.Session, minutesOverride *int, reason string) (*finishResult, error) {
	userID := session.UserID.String()

	now := time.Now()

	// К0-и3 (KIOSK.md): гость ещё не нажал [Готов!] — денег не брали, минут
	// не было. Раньше завершение в окне ожидания (админ через «Завершить»,
	// гость с телефона, бан) считало минуты от started_at и списывало
	// «дорогу к компьютеру», а заодно начисляло XP/монеты и «сессию дня».
	// Теперь это отмена посадки, как у seat-cancel: статус cancelled, ноль
	// минут, ноль наград. Защита стоит только в тике биллинга и паузе —
	// здесь её не было.
	if sessionWaitingReady(session) {
		return cancelWaitingSession(session, now, reason)
	}
	// Г2: паузы не тарифицируются и XP не приносят — минуты БЕЗ пауз, вверх.
	minutes := effectiveMinutesCeil(session, now)

	// Г1: финальный расчёт кошелька — доначислить хвост по РЕАЛЬНОМУ времени
	// (вверх до целой минуты, как и XP). Дев-оверрайд минут ниже на деньги
	// сознательно не влияет: он существует для проверки начислений XP/монет.
	// Сессия уже закрыта параллельно (errSessionGone) — наружу без наград,
	// второй раз ничего не начисляем.
	var payer models.User
	if _, serr := settleSessionMinutes(session, &payer, minutes, now); serr != nil {
		if errors.Is(serr, errSessionGone) {
			return nil, serr
		}
		log.Printf("биллинг: финальный расчёт сессии %s не прошёл: %v", session.ID, serr)
	}

	// dev-оверрайд: позволяет тестировать начисление без ожидания реального времени
	if minutesOverride != nil && os.Getenv("SERVER_ENV") != "production" {
		minutes = *minutesOverride
		if minutes < 0 {
			minutes = 0
		}
	}

	// Ранг аккаунта (по наигранным часам) множит XP/coins и бустит кейсы.
	rank, _ := accountRankFor(userHoursPlayed(userID))

	// Ставки начисления — из настроек клуба (settings, спринт А5); формула —
	// одна на начисление и калькулятор (К3-и1), см. sessionRewards.
	xpGained, coinsGained := sessionRewards(minutes, economyRates(), talentEffect(userID, "xp_boost"), rank)

	// Первый визит за день: +50 XP (фиксировано, без модификаторов — ТЗ 4.1).
	dailyBonusXP := int64(0)
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	var todayCount int64
	db.Model(&models.Session{}).
		Where("user_id = ? AND status = ? AND ended_at >= ? AND id <> ?",
			userID, models.SessionStatusCompleted, startOfDay, session.ID).
		Count(&todayCount)
	if todayCount == 0 {
		dailyBonusXP = 50
		xpGained += dailyBonusXP
	}

	var user models.User
	if err := db.First(&user, "id = ?", userID).Error; err != nil {
		return nil, err
	}

	levelsGained := applyXP(&user, xpGained)
	user.CoinsBalance += coinsGained
	user.LastActiveAt = now

	err := db.Transaction(func(tx *gorm.DB) error {
		// Закрываем только пока active: гость, админ и биллинг могут захотеть
		// завершить одну сессию одновременно — награды получает первый.
		res := tx.Model(&models.Session{}).
			Where("id = ? AND status = ?", session.ID, models.SessionStatusActive).
			Updates(map[string]any{
				"status":       models.SessionStatusCompleted,
				"ended_at":     now,
				"minutes_used": minutes,
				"xp_earned":    xpGained,
				"coins_earned": coinsGained,
				"ended_reason": reason,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errSessionGone
		}
		session.Status = models.SessionStatusCompleted
		session.EndedAt = &now
		session.MinutesUsed = minutes
		session.XPEarned = xpGained
		session.CoinsEarned = coinsGained
		session.EndedReason = &reason
		// Освобождаем ПК по АКТУАЛЬНОМУ computer_id из строки сессии: гостя
		// могли пересадить между чтением сессии и этой транзакцией, и тогда
		// освобождался не тот ПК, а машина-приёмник залипала в in_session
		// без выхода (ревью 26.08).
		if err := tx.Model(&models.Computer{}).
			Where("id = (SELECT computer_id FROM sessions WHERE id = ?)", session.ID).
			Update("status", models.ComputerStatusAvailable).Error; err != nil {
			return err
		}
		// Обновляем только поля наград (Г1): полный Save затирал бы
		// wallet_grosz/coin_minutes, если биллинг успел списать между чтением
		// гостя и этой транзакцией. Монеты и очки — ИНКРЕМЕНТОМ: абсолютное
		// значение затирало бы депозит, проведённый у стойки в эти же
		// миллисекунды (ревью 26.08).
		return tx.Model(&models.User{}).Where("id = ?", user.ID).
			Updates(map[string]any{
				"level":                 user.Level,
				"xp_current":            user.XPCurrent,
				"xp_total":              user.XPTotal,
				"coins_balance":         gorm.Expr("coins_balance + ?", coinsGained),
				"skillpoints_available": gorm.Expr("skillpoints_available + ?", levelsGained),
				"last_active_at":        user.LastActiveAt,
			}).Error
	})
	if err != nil {
		return nil, err
	}

	// Кейс за каждый новый уровень.
	if levelsGained > 0 {
		for i := 0; i < levelsGained; i++ {
			_ = grantCase(db, user.ID, &session.ClubID, tierForLevel(user.Level), models.CaseSourceLevelUp)
		}
	}

	// Бонусный кейс за сессию. Шанс: case_hunter + ранг; тир: luck_grade + ранг.
	bonusCase := false
	var bonusCaseTier models.CaseTier
	dropChance := sessionCaseChance(talentEffect(userID, "case_hunter"), rank.CaseChanceBonus)
	tierBoost := talentEffect(userID, "luck_grade") + rank.TierBoost
	if chance(dropChance) {
		bonusCaseTier = rollCaseTier(tierBoost)
		if grantCase(db, user.ID, &session.ClubID, bonusCaseTier, models.CaseSourceDailyVisit) == nil {
			bonusCase = true
		}
	}

	// Талант double_drop (Strength): шанс второго бонусного кейса (тир роллится заново).
	var bonusCaseTier2 models.CaseTier
	if bonusCase && chance(talentEffect(userID, "double_drop")) {
		bonusCaseTier2 = rollCaseTier(tierBoost)
		if grantCase(db, user.ID, &session.ClubID, bonusCaseTier2, models.CaseSourceDailyVisit) != nil {
			bonusCaseTier2 = ""
		}
	}

	// Г5: суточный прогресс (топливо периодических ачивок, резет 14:48) +
	// проверка ВСЕХ достижений — lifetime и периодических. Дев-оверрайд минут
	// честно тестирует и ачивки (пишется как активные минуты). К5: сессия через
	// 14:48 ложится в двое суток; выданное по ходу сессии (тик биллинга) второй
	// раз не выдаётся — сессия уже completed, живого досчёта нет.
	devOverride := minutesOverride != nil && os.Getenv("SERVER_ENV") != "production"
	recordSessionProgress(session.UserID, minutes, session.ActiveMinutes, session.StartedAt, now, devOverride)
	stats := fullPlayerStats(session.UserID, now)
	checkAchievements(session.UserID, stats)

	// К8-и2: открытая автозапись обрыва агента по этой сессии — закрыть длительностью
	closeSessionIncidents(session.ID, now)

	// Г7/Р10: доиграл с неоплаченной кухней → гостю напоминание «рассчитайся
	// у стойки», админам сигнал в живую ленту
	kCnt, kSum := unpaidKitchen(session.UserID)
	if kCnt > 0 {
		notifyUser(session.UserID, "kitchen_unpaid", map[string]any{"count": kCnt, "total_pln": kSum})
		hub.AdminBroadcast("kitchen", map[string]any{
			"kind": "unpaid", "nickname": user.Nickname, "total_pln": kSum, "count": kCnt})
	}

	// Команда на ПК: завершить и заблокировать (если Shell подключён).
	notifyShell(session.ComputerID.String(), websocket.MsgSessionEnd, gin.H{
		"session_id":   session.ID,
		"xp_earned":    xpGained,
		"coins_earned": coinsGained,
	})

	// Live-событие в админку (спринт А6).
	hub.AdminBroadcast("session_end", map[string]any{
		"computer_id": session.ComputerID.String(),
		"nickname":    user.Nickname,
		"minutes":     minutes,
		"xp":          xpGained,
		"coins":       coinsGained,
	})

	// Вейтлист (Б9): ПК освободился — если очередь не пуста, разово зовём
	// голову («ПК свободен — подойди к стойке», шина Б4).
	checkWaitlistNotify(session.ClubID)

	return &finishResult{
		Session: session, Minutes: minutes,
		XPGained: xpGained, CoinsGained: coinsGained,
		DailyBonusXP: dailyBonusXP,
		LevelsGained: levelsGained,
		BonusCase:    bonusCase, BonusCaseTier: bonusCaseTier, BonusCaseTier2: bonusCaseTier2,
		Rank: rank,
		User: user,
		// К3-и6: кошелёк — после финального расчёта (user прочитан после settle),
		// стрик — с учётом только что закрытой сессии (stats собраны после
		// recordSessionProgress).
		Receipt: receipt{WalletGrosz: user.WalletGrosz, KitchenUnpaidPLN: kSum, StreakDays: stats.VisitStreak},
	}, nil
}

// GET /me/sessions — последние сессии текущего игрока.
func handleGetMySessions(c *gin.Context) {
	userID := c.GetString("user_id")

	var sessions []models.Session
	db.Where("user_id = ?", userID).
		Order("started_at DESC").
		Limit(50).
		Find(&sessions)

	// Г3: активной сессии отдаём дедлайн чужой брони — шелл режет прогноз
	// «хватит до» и честно показывает, что ПК уйдёт под бронь.
	var bookingDeadline *time.Time
	var activeComputer string // Е1: экран [Готов!] называет машину — «ПК-07»
	var packMinutes int64     // Е2-и5: минуты пакета, годные ЗДЕСЬ — прогноз без них врёт
	var zoneMinutes int64     // Ж2: зонные минуты (за монеты, компенсации), годные ЗДЕСЬ
	var activeZone string     // Ж2: имя зоны машины — клиент говорит «в этой зоне» словами
	hasActive := false
	for i := range sessions {
		if sessions[i].Status != models.SessionStatusActive {
			continue
		}
		hasActive = true
		if nb := nextForeignBooking(sessions[i].ComputerID, sessions[i].UserID, time.Now()); nb != nil {
			d := nb.StartTime.Add(-time.Duration(settingInt64("booking_lock_min", bookingLockMinDef)) * time.Minute)
			bookingDeadline = &d
		}
		var pc models.Computer
		if db.Select("name, zone_id").First(&pc, "id = ?", sessions[i].ComputerID).Error == nil {
			activeComputer = pc.Name
			// Пакет привязан к зоне (Р10): считать «сколько осталось» по всем
			// пакетам сразу значило бы обещать гостю время, которое на ЭТОЙ
			// машине не потратить.
			packMinutes = livePackMinutes(sessions[i].UserID, pc.ZoneID, time.Now())
			zoneMinutes = liveGrantMinutes(sessions[i].UserID, pc.ZoneID)
			if z := zoneByID(pc.ZoneID); z != nil {
				activeZone = z.Name
			}
		}
		break
	}

	resp := gin.H{"count": len(sessions), "sessions": sessions,
		"booking_deadline": bookingDeadline, "computer": activeComputer, "zone": activeZone,
		"pack_minutes": packMinutes, "zone_minutes": zoneMinutes}
	// К3-и6: сессию закрыли не с этого экрана (админ, ноль кошелька, бронь,
	// AFK, телефон) — киоск узнаёт об этом из поллинга и показывает тот же
	// экран «Сессия завершена» с чеком. Чек считаем только когда он к месту:
	// активной сессии нет, а последняя закрылась только что.
	if !hasActive && len(sessions) > 0 && sessions[0].EndedAt != nil &&
		time.Since(*sessions[0].EndedAt) < receiptWindow {
		var me models.User
		if db.Select("wallet_grosz").First(&me, "id = ?", userID).Error == nil {
			resp["receipt"] = receiptFor(sessions[0].UserID, me.WalletGrosz, time.Now())
		}
	}
	c.JSON(http.StatusOK, resp)
}

// receiptWindow — сколько после завершения GET /me/sessions ещё отдаёт чек.
const receiptWindow = 15 * time.Minute
