package main

// Admin API (спринт 3б, MVP). Все роуты за authMiddleware + adminMiddleware:
// доступ только для role=admin/owner (роль хранится в users.role, миграция 008,
// и едет в JWT-claim "role"). Повысить аккаунт:
//   UPDATE users SET role='admin' WHERE nickname='<ник>';

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// roleIsStaff — чистая проверка роли (тестируется отдельно).
func roleIsStaff(role string) bool {
	return role == string(models.UserRoleAdmin) || role == string(models.UserRoleOwner)
}

// adminMiddleware — пускает только персонал. Ставится ПОСЛЕ authMiddleware.
func adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !roleIsStaff(c.GetString("user_role")) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "forbidden", "message": "Нужны права администратора"})
			return
		}
		c.Next()
	}
}

// canTargetUser — чистый инвариант действий персонала над аккаунтом
// (спринт Б0-и1, решение №4 в ADMIN.md; тест в admin_test.go):
// цель — только гость (role=player) и не сам исполнитель.
// Применяется к бану/разбану, депозиту и ручным начислениям.
func canTargetUser(targetRole models.UserRole, targetID, actorID string) (ok bool, code string) {
	if targetID != "" && targetID == actorID {
		return false, "cannot_touch_self"
	}
	if targetRole != models.UserRolePlayer {
		return false, "cannot_touch_staff"
	}
	return true, ""
}

// targetPlayer — находит цель действия по :id и применяет canTargetUser.
// Ответ об ошибке пишет сам; вернул nil — обработчику выходить.
func targetPlayer(c *gin.Context) *models.User {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return nil
	}
	if ok, code := canTargetUser(user.Role, user.ID.String(), c.GetString("user_id")); !ok {
		msg := map[string]string{
			"cannot_touch_self":  "Нельзя применить действие к самому себе",
			"cannot_touch_staff": "Действия персонала применимы только к гостям",
		}[code]
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": msg})
		return nil
	}
	return &user
}

// startOfToday — полночь текущего дня (для дневных лимитов, Б0-и4).
func startOfToday() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
}

// adminDayCapExceeded — превышен ли дневной потолок админа
// (cap <= 0 — лимита нет; тест в admin_test.go).
func adminDayCapExceeded(used, add, cap float64) bool {
	if cap <= 0 {
		return false
	}
	return used+add > cap
}

// GET /admin/overview — сводка для шапки админки.
// Б1: + сегодняшний срез — брони сегодня (бэйдж и KPI) и выручка за день
// (депозиты с полуночи; операции дня видны и роли admin — решение №3).
func handleAdminOverview(c *gin.Context) {
	var users, activeSessions, computersTotal, computersFree, upcomingBookings int64
	db.Model(&models.User{}).Count(&users)
	db.Model(&models.Session{}).Where("status = ?", models.SessionStatusActive).Count(&activeSessions)
	db.Model(&models.Computer{}).Count(&computersTotal)
	db.Model(&models.Computer{}).Where("status = ?", models.ComputerStatusAvailable).Count(&computersFree)
	db.Model(&models.Booking{}).
		Where("status IN ? AND start_time > ?",
			[]models.BookingStatus{models.BookingStatusPending, models.BookingStatusConfirmed}, time.Now()).
		Count(&upcomingBookings)

	today := startOfToday()
	var bookingsToday int64
	db.Model(&models.Booking{}).
		Where("status IN ? AND start_time >= ? AND start_time < ?",
			[]models.BookingStatus{models.BookingStatusPending, models.BookingStatusConfirmed},
			today, today.Add(24*time.Hour)).
		Count(&bookingsToday)
	var revenueToday float64
	db.Model(&models.Deposit{}).Select("COALESCE(SUM(amount_pln),0)").
		Where("created_at >= ?", today).Scan(&revenueToday)

	c.JSON(http.StatusOK, gin.H{
		"users":             users,
		"active_sessions":   activeSessions,
		"computers_total":   computersTotal,
		"computers_free":    computersFree,
		"upcoming_bookings": upcomingBookings,
		"bookings_today":    bookingsToday,
		"revenue_today_pln": revenueToday,
	})
}

// GET /admin/users?q=ник — гости (поиск, свежие сверху).
func handleAdminUsers(c *gin.Context) {
	q := db.Model(&models.User{}).Order("last_active_at DESC").Limit(100)
	if s := c.Query("q"); s != "" {
		q = q.Where("nickname ILIKE ?", "%"+s+"%")
	}
	var users []models.User
	if err := q.Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(users), "users": users})
}

// setUserStatus — общий код бана/разбана.
// Бан гасит активную сессию честно (Б0-и2, решение №5 в ADMIN.md):
// finishSession начисляет за фактическое время, освобождает ПК и шлёт
// session_end на Shell (агент лочит экран); в админ-ленту уходит «ban».
func setUserStatus(c *gin.Context, status models.UserStatus) {
	user := targetPlayer(c)
	if user == nil {
		return
	}
	if err := db.Model(user).Update("status", status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	action, details := "unban", ""
	if status == models.UserStatusBanned {
		action = "ban"
		var s models.Session
		if db.Where("user_id = ? AND status = ?", user.ID, models.SessionStatusActive).
			First(&s).Error == nil {
			if res, err := finishSession(&s, nil); err == nil {
				details = fmt.Sprintf("активная сессия завершена: %d мин, +%d XP, +%d монет",
					res.Minutes, res.XPGained, res.CoinsGained)
			} else {
				details = "не удалось завершить активную сессию: " + err.Error()
			}
		}
		hub.AdminBroadcast("ban", map[string]any{"nickname": user.Nickname})
	}
	target := user.ID
	logAdminAction(c, action, &target, details)
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "status": status})
}

func handleAdminBan(c *gin.Context)   { setUserStatus(c, models.UserStatusBanned) }
func handleAdminUnban(c *gin.Context) { setUserStatus(c, models.UserStatusActive) }

// GET /admin/users/:id — карточка гостя одним заходом (спринт А2).
// Деньги по ролям (Б1-и4, решение №3): owner видит всю историю депозитов и
// «внесено всего»; admin — только депозиты текущего дня, без агрегата.
func handleAdminUserCard(c *gin.Context) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	ownerView := c.GetString("user_role") == string(models.UserRoleOwner)

	var sessions []models.Session
	db.Preload("Computer").Where("user_id = ?", user.ID).
		Order("started_at DESC").Limit(20).Find(&sessions)

	var deposits []models.Deposit
	depQ := db.Where("user_id = ?", user.ID)
	if !ownerView {
		depQ = depQ.Where("created_at >= ?", startOfToday())
	}
	depQ.Order("created_at DESC").Limit(20).Find(&deposits)

	var grants []models.AdminGrant
	db.Where("user_id = ?", user.ID).Order("created_at DESC").Limit(20).Find(&grants)

	// сводка за всё время (деньги — только owner)
	var agg struct {
		Cnt     int64
		Minutes int64
	}
	db.Model(&models.Session{}).
		Select("COUNT(*) AS cnt, COALESCE(SUM(minutes_used),0) AS minutes").
		Where("user_id = ?", user.ID).Scan(&agg)
	var depSum float64
	if ownerView {
		db.Model(&models.Deposit{}).Select("COALESCE(SUM(amount_pln),0)").
			Where("user_id = ?", user.ID).Scan(&depSum)
	}

	// ники админов для журнала начислений
	adminIDs := map[string]bool{}
	for _, g := range grants {
		adminIDs[g.AdminID.String()] = true
	}
	nick := map[string]string{}
	if len(adminIDs) > 0 {
		ids := make([]string, 0, len(adminIDs))
		for id := range adminIDs {
			ids = append(ids, id)
		}
		var admins []models.User
		db.Where("id IN ?", ids).Find(&admins)
		for _, a := range admins {
			nick[a.ID.String()] = a.Nickname
		}
	}

	sesOut := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		row := gin.H{
			"started_at": s.StartedAt, "status": s.Status, "minutes": s.MinutesUsed,
			"xp_earned": s.XPEarned, "coins_earned": s.CoinsEarned,
		}
		if s.Computer != nil {
			row["computer"] = s.Computer.Name
		}
		sesOut = append(sesOut, row)
	}
	grOut := make([]gin.H, 0, len(grants))
	for _, g := range grants {
		grOut = append(grOut, gin.H{
			"created_at": g.CreatedAt, "type": g.GrantType, "amount": g.Amount,
			"case_tier": g.CaseTier, "reason": g.Reason, "admin": nick[g.AdminID.String()],
		})
	}

	stats := gin.H{
		"sessions_count": agg.Cnt,
		"hours_played":   agg.Minutes / 60,
	}
	if ownerView {
		stats["deposited_pln"] = depSum
	}
	c.JSON(http.StatusOK, gin.H{
		"user":        user,
		"stats":       stats,
		"sessions":    sesOut,
		"deposits":    deposits,
		"grants":      grOut,
		"money_scope": map[bool]string{true: "all", false: "today"}[ownerView],
	})
}

// GET /admin/computers — все ПК + кто сейчас играет.
func handleAdminComputers(c *gin.Context) {
	var computers []models.Computer
	if err := db.Preload("Club").Order("name").Find(&computers).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}

	var active []models.Session
	db.Preload("User").Where("status = ?", models.SessionStatusActive).Find(&active)
	byComputer := map[string]*models.Session{}
	for i := range active {
		byComputer[active[i].ComputerID.String()] = &active[i]
	}

	out := make([]gin.H, 0, len(computers))
	for _, pc := range computers {
		row := gin.H{
			"id": pc.ID, "name": pc.Name, "zone": pc.Zone, "status": pc.Status,
			"club": pc.Club.Name, "club_id": pc.ClubID,
			"pos_x": pc.PosX, "pos_y": pc.PosY, // схема зала (спринт А8)
			"shell_online": hub.IsConnected(pc.ID.String()),
		}
		if s, ok := byComputer[pc.ID.String()]; ok {
			row["session"] = gin.H{
				"id": s.ID, "started_at": s.StartedAt,
				"nickname": nicknameOf(s.User),
			}
		}
		out = append(out, row)
	}

	// Клубы с размерами схем — для позиционной карты (спринт А8).
	var clubs []models.Club
	db.Where("is_active = ?", true).Order("name").Find(&clubs)
	clubsOut := make([]gin.H, 0, len(clubs))
	for _, cl := range clubs {
		clubsOut = append(clubsOut, gin.H{
			"id": cl.ID, "name": cl.Name, "layout_w": cl.LayoutW, "layout_h": cl.LayoutH,
		})
	}

	c.JSON(http.StatusOK, gin.H{"count": len(out), "computers": out, "clubs": clubsOut})
}

func nicknameOf(u *models.User) string {
	if u == nil {
		return "?"
	}
	return u.Nickname
}

// GET /admin/sessions/active — активные сессии.
func handleAdminActiveSessions(c *gin.Context) {
	var sessions []models.Session
	db.Preload("User").Preload("Computer").
		Where("status = ?", models.SessionStatusActive).
		Order("started_at").Find(&sessions)

	out := make([]gin.H, 0, len(sessions))
	for _, s := range sessions {
		row := gin.H{"id": s.ID, "started_at": s.StartedAt, "nickname": nicknameOf(s.User)}
		if s.Computer != nil {
			row["computer"] = s.Computer.Name
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "sessions": out})
}

// POST /admin/sessions/:id/end — принудительно завершить сессию.
// Начисление честное — та же finishSession, что и у игрока; ПК освобождается,
// Shell получает session_end, гостевой экран блокируется.
func handleAdminEndSession(c *gin.Context) {
	var session models.Session
	if err := db.First(&session, "id = ? AND status = ?", c.Param("id"), models.SessionStatusActive).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "session_not_found", "message": "Активная сессия не найдена"})
		return
	}
	res, err := finishSession(&session, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := session.UserID
	logAdminAction(c, "session_end", &target,
		fmt.Sprintf("%d мин, +%d XP, +%d монет", res.Minutes, res.XPGained, res.CoinsGained))
	c.JSON(http.StatusOK, finishResponse(res))
}

// GET /admin/bookings — ближайшие брони (от «2 часа назад» и дальше).
func handleAdminBookings(c *gin.Context) {
	var bookings []models.Booking
	db.Preload("Computer").Preload("Club").
		Where("start_time > ?", time.Now().Add(-2*time.Hour)).
		Order("start_time").Limit(100).Find(&bookings)

	// ники одним запросом
	ids := make([]string, 0, len(bookings))
	for _, b := range bookings {
		ids = append(ids, b.UserID.String())
	}
	nick := map[string]string{}
	if len(ids) > 0 {
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		for _, u := range users {
			nick[u.ID.String()] = u.Nickname
		}
	}

	out := make([]gin.H, 0, len(bookings))
	for _, b := range bookings {
		row := gin.H{
			"id": b.ID, "status": b.Status, "start_time": b.StartTime,
			"duration_min": b.DurationMin, "nickname": nick[b.UserID.String()],
			"prepaid": b.Prepaid, "computer_id": b.ComputerID, // id — для метки «скоро бронь» на карте зала
		}
		if b.Computer != nil {
			row["computer"] = b.Computer.Name
		}
		if b.Club != nil {
			row["club"] = b.Club.Name
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "bookings": out})
}

// canSetComputerStatus — чистое правило смены статуса ПК из админки (тест в
// admin_test.go). Разрешено только available ⇄ maintenance: занятые и
// зарезервированные ПК статусом руками не трогаем.
func canSetComputerStatus(cur, next models.ComputerStatus) bool {
	if next != models.ComputerStatusAvailable && next != models.ComputerStatusMaintenance {
		return false
	}
	if cur == next {
		return true // идемпотентно
	}
	return (cur == models.ComputerStatusAvailable && next == models.ComputerStatusMaintenance) ||
		(cur == models.ComputerStatusMaintenance && next == models.ComputerStatusAvailable)
}

// PATCH /admin/computers/:id/status — перевод ПК в ремонт и обратно (спринт А1).
func handleAdminSetComputerStatus(c *gin.Context) {
	var req struct {
		Status models.ComputerStatus `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil ||
		(req.Status != models.ComputerStatusAvailable && req.Status != models.ComputerStatusMaintenance) {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_status", "message": "status: available или maintenance"})
		return
	}
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	if !canSetComputerStatus(pc.Status, req.Status) {
		c.JSON(http.StatusConflict, gin.H{"code": "status_conflict", "message": "Сначала завершите сессию на этом ПК"})
		return
	}
	if err := db.Model(&pc).Update("status", req.Status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	action := "pc_available"
	if req.Status == models.ComputerStatusMaintenance {
		action = "pc_maintenance"
	}
	logAdminAction(c, action, nil, pc.Name)
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "name": pc.Name, "status": req.Status})
}

// POST /admin/bookings/:id/cancel — отменить бронь (любую живую).
// Гость узнаёт тостом через /me/notifications (Б4).
func handleAdminCancelBooking(c *gin.Context) {
	var booking models.Booking
	if err := db.Preload("Computer").First(&booking, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "booking_not_found", "message": "Бронь не найдена"})
		return
	}
	if booking.Status != models.BookingStatusPending && booking.Status != models.BookingStatusConfirmed {
		c.JSON(http.StatusConflict, gin.H{"code": "not_cancellable", "message": "Эту бронь уже нельзя отменить"})
		return
	}
	if err := db.Model(&booking).Update("status", models.BookingStatusCancelled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := booking.UserID
	logAdminAction(c, "booking_cancel", &target, "на "+booking.StartTime.Format("02.01 15:04"))
	pcName := ""
	if booking.Computer != nil {
		pcName = booking.Computer.Name
	}
	notifyUser(booking.UserID, "booking_cancel",
		map[string]any{"start_time": booking.StartTime, "computer": pcName})
	hub.AdminBroadcast("booking", map[string]any{"kind": "cancel", "start_time": booking.StartTime})
	c.JSON(http.StatusOK, gin.H{"booking_id": booking.ID, "status": models.BookingStatusCancelled})
}

// POST /admin/bookings — walk-in бронь за гостя (спринт А3): по нику, на
// конкретный ПК или первый свободный. Сразу confirmed, prepaid=false —
// оплата на месте. Время/длительность проверяет validateBookingTime,
// пересечения — bookingOverlaps (те же правила, что у брони игрока).
func handleAdminCreateBooking(c *gin.Context) {
	var req struct {
		Nickname    string  `json:"nickname" binding:"required"`
		ComputerID  *string `json:"computer_id"`
		StartTime   string  `json:"start_time" binding:"required"` // RFC3339
		DurationMin int     `json:"duration_min"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": err.Error()})
		return
	}
	if req.DurationMin == 0 {
		req.DurationMin = 60
	}
	start, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_time", "message": "start_time — в формате RFC3339"})
		return
	}
	if ok, code := validateBookingTime(start, req.DurationMin, time.Now()); !ok {
		msg := map[string]string{
			"bad_duration": "Длительность брони: от 30 минут до 8 часов",
			"in_past":      "Бронь в прошлом невозможна",
			"too_far":      "Бронь возможна максимум за 30 дней",
		}[code]
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
		return
	}

	var user models.User
	if err := db.First(&user, "nickname = ? AND role = ?", req.Nickname, models.UserRolePlayer).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Гость с таким ником не найден"})
		return
	}

	dur := time.Duration(req.DurationMin) * time.Minute
	var candidates []models.Computer
	q := db.Where("status <> ?", models.ComputerStatusMaintenance).Order("name")
	if req.ComputerID != nil && *req.ComputerID != "" {
		q = q.Where("id = ?", *req.ComputerID)
	}
	if err := q.Find(&candidates).Error; err != nil || len(candidates) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"code": "no_computer", "message": "Подходящий ПК не найден"})
		return
	}

	var existing []models.Booking
	db.Where("status IN ? AND start_time BETWEEN ? AND ?",
		[]models.BookingStatus{models.BookingStatusPending, models.BookingStatusConfirmed},
		start.Add(-24*time.Hour), start.Add(24*time.Hour)).Find(&existing)
	busy := map[string]bool{}
	for _, b := range existing {
		if bookingOverlaps(start, dur, b.StartTime, time.Duration(b.DurationMin)*time.Minute) {
			busy[b.ComputerID.String()] = true
		}
	}
	var pick *models.Computer
	for i := range candidates {
		if !busy[candidates[i].ID.String()] {
			pick = &candidates[i]
			break
		}
	}
	if pick == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "slot_busy", "message": "На это время всё занято — выбери другое время"})
		return
	}

	booking := models.Booking{
		UserID: user.ID, ComputerID: pick.ID, ClubID: pick.ClubID,
		Status: models.BookingStatusConfirmed, StartTime: start,
		DurationMin: req.DurationMin, Prepaid: false,
	}
	if err := db.Create(&booking).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := user.ID
	logAdminAction(c, "booking_create", &target,
		fmt.Sprintf("%s · %s · %d мин", pick.Name, start.Format("02.01 15:04"), req.DurationMin))
	hub.AdminBroadcast("booking", map[string]any{
		"kind": "create", "nickname": user.Nickname, "computer": pick.Name, "start_time": booking.StartTime,
	})
	c.JSON(http.StatusCreated, gin.H{
		"booking_id": booking.ID, "nickname": user.Nickname, "computer": pick.Name,
		"start_time": booking.StartTime, "duration_min": booking.DurationMin,
	})
}

// POST /admin/bookings/:id/restore — вернуть отменённую бронь (undo из тоста).
// Перед возвратом заново проверяем пересечения по этому ПК.
func handleAdminRestoreBooking(c *gin.Context) {
	var booking models.Booking
	if err := db.First(&booking, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "booking_not_found", "message": "Бронь не найдена"})
		return
	}
	if booking.Status != models.BookingStatusCancelled {
		c.JSON(http.StatusConflict, gin.H{"code": "not_restorable", "message": "Вернуть можно только отменённую бронь"})
		return
	}
	dur := time.Duration(booking.DurationMin) * time.Minute
	var existing []models.Booking
	db.Where("computer_id = ? AND status IN ? AND start_time BETWEEN ? AND ?",
		booking.ComputerID,
		[]models.BookingStatus{models.BookingStatusPending, models.BookingStatusConfirmed},
		booking.StartTime.Add(-24*time.Hour), booking.StartTime.Add(24*time.Hour)).Find(&existing)
	for _, b := range existing {
		if bookingOverlaps(booking.StartTime, dur, b.StartTime, time.Duration(b.DurationMin)*time.Minute) {
			c.JSON(http.StatusConflict, gin.H{"code": "slot_busy", "message": "Время уже занято другой бронью"})
			return
		}
	}
	if err := db.Model(&booking).Update("status", models.BookingStatusConfirmed).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	target := booking.UserID
	logAdminAction(c, "booking_restore", &target, "на "+booking.StartTime.Format("02.01 15:04"))
	notifyUser(booking.UserID, "booking_restore", map[string]any{"start_time": booking.StartTime}) // Б4
	hub.AdminBroadcast("booking", map[string]any{"kind": "restore", "start_time": booking.StartTime})
	c.JSON(http.StatusOK, gin.H{"booking_id": booking.ID, "status": models.BookingStatusConfirmed})
}
