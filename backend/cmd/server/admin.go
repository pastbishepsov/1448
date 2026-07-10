package main

// Admin API (спринт 3б, MVP). Все роуты за authMiddleware + adminMiddleware:
// доступ только для role=admin/owner (роль хранится в users.role, миграция 008,
// и едет в JWT-claim "role"). Повысить аккаунт:
//   UPDATE users SET role='admin' WHERE nickname='<ник>';

import (
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

// GET /admin/overview — сводка для шапки админки.
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

	c.JSON(http.StatusOK, gin.H{
		"users":             users,
		"active_sessions":   activeSessions,
		"computers_total":   computersTotal,
		"computers_free":    computersFree,
		"upcoming_bookings": upcomingBookings,
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
func setUserStatus(c *gin.Context, status models.UserStatus) {
	var user models.User
	if err := db.First(&user, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
		return
	}
	if user.Role != models.UserRolePlayer {
		c.JSON(http.StatusForbidden, gin.H{"code": "cannot_touch_staff", "message": "Нельзя банить персонал"})
		return
	}
	if err := db.Model(&user).Update("status", status).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user_id": user.ID, "nickname": user.Nickname, "status": status})
}

func handleAdminBan(c *gin.Context)   { setUserStatus(c, models.UserStatusBanned) }
func handleAdminUnban(c *gin.Context) { setUserStatus(c, models.UserStatusActive) }

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
			"club": pc.Club.Name, "shell_online": hub.IsConnected(pc.ID.String()),
		}
		if s, ok := byComputer[pc.ID.String()]; ok {
			row["session"] = gin.H{
				"id": s.ID, "started_at": s.StartedAt,
				"nickname": nicknameOf(s.User),
			}
		}
		out = append(out, row)
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "computers": out})
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
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "name": pc.Name, "status": req.Status})
}

// POST /admin/bookings/:id/cancel — отменить бронь (любую живую).
func handleAdminCancelBooking(c *gin.Context) {
	var booking models.Booking
	if err := db.First(&booking, "id = ?", c.Param("id")).Error; err != nil {
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
	c.JSON(http.StatusOK, gin.H{"booking_id": booking.ID, "status": models.BookingStatusCancelled})
}
