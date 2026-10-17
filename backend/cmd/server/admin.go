package main

// Admin API (спринт 3б, MVP). Все роуты за authMiddleware + adminMiddleware:
// доступ только для role=admin/owner (роль хранится в users.role, миграция 008,
// и едет в JWT-claim "role"). Повысить аккаунт:
//   UPDATE users SET role='admin' WHERE nickname='<ник>';

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
	"github.com/pastbishepsov/1448/backend/internal/websocket"
)

// roleIsStaff — чистая проверка роли (тестируется отдельно).
func roleIsStaff(role string) bool {
	return role == string(models.UserRoleAdmin) || role == string(models.UserRoleOwner)
}

// staffRoleNow — роль и статус человека из базы, а не из токена (З0-и1, №3):
// увольнение и снятие роли действуют в ту же секунду. Персонала мало, чтение
// по первичному ключу. active=false — аккаунт заблокирован.
func staffRoleNow(userID string) (role string, active bool, err error) {
	var u models.User
	if err := db.Select("role", "status").First(&u, "id = ?", userID).Error; err != nil {
		return "", false, err
	}
	return string(u.Role), u.Status != models.UserStatusBanned, nil
}

// adminMiddleware — пускает только персонал. Ставится ПОСЛЕ authMiddleware.
// З0-и1 (№3): роль и статус — из базы, claim токена больше ничего не решает;
// в контекст кладётся роль из базы — её видят ownerMiddleware и все ручки ниже.
func adminMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		role, active, err := staffRoleNow(c.GetString("user_id"))
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"code": "user_not_found", "message": "Пользователь не найден"})
			return
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"code": "db_unavailable", "message": "База недоступна — повтори через минуту"})
			return
		}
		if !active || !roleIsStaff(role) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "forbidden", "message": "Нужны права администратора"})
			return
		}
		// З14-и2: отложенное увольнение — последний рабочий день прошёл, а фоновый прогон ещё не снял роль
		if role == string(models.UserRoleAdmin) && staffLeft(c.GetString("user_id"), time.Now()) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"code": "staff_left", "message": "Последний рабочий день прошёл — доступа в админку больше нет"})
			return
		}
		c.Set("user_role", role)
		c.Next()
	}
}

// dropAdminWS — закрыть live-каналы админки человека, у которого забрали роль
// (З0-и1): открытая вкладка уволенного не должна дальше получать ленту зала.
func dropAdminWS(userID uuid.UUID) {
	if hub == nil {
		return
	}
	hub.DropAdmin(userID.String())
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

// clubDayStart — начало клубных суток, в которые попадает now: report_hour
// сегодня, а до report_hour — вчера. Ж3-и1 (ревью 14.09, №12): у админки
// было ТРИ «сегодня» — полночь у дневных лимитов, журнала и карточки
// (startOfToday), report_hour у отчётов и сводки смены, календарный день у
// KPI. Депозит в 23:50 пропадал из журнала админа в 00:00, а дневной потолок
// обнулялся посреди ночной смены. Теперь «сегодня» одно — клубные сутки.
func clubDayStart(now time.Time) time.Time {
	return clubDayStartAt(int(settingInt64("report_hour", 8)), now)
}

// clubDayStartAt — чистая часть (тест на границу 07:59/08:00).
func clubDayStartAt(reportHour int, now time.Time) time.Time {
	from, _, _, _ := shiftWindow("", reportHour, now)
	return from
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
// (операции дня видны и роли admin — решение №3). Ж3-и2 (№13): «сегодня» —
// клубные сутки, а выручка считается ТОЙ ЖЕ функцией, что разрез «Деньги»
// отчёта (aggMoney: пополнения + товары + пакеты, без отменённых и без
// оплат кошельком) — раньше KPI показывал только депозиты с полуночи и не
// сходился с отчётом владельца ни по составу, ни по границе дня.
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

	reportHour := int(settingInt64("report_hour", 8))
	today := clubDayOf(time.Now(), reportHour)
	day := makePeriod(today, today, reportHour, "today")
	var bookingsToday int64
	db.Model(&models.Booking{}).
		Where("status IN ? AND start_time >= ? AND start_time < ?",
			[]models.BookingStatus{models.BookingStatusPending, models.BookingStatusConfirmed},
			day.From, day.To).
		Count(&bookingsToday)
	money := aggMoney(day)

	c.JSON(http.StatusOK, gin.H{
		"users":             users,
		"active_sessions":   activeSessions,
		"computers_total":   computersTotal,
		"computers_free":    computersFree,
		"upcoming_bookings": upcomingBookings,
		"bookings_today":    bookingsToday,
		"revenue_today_pln": money.Revenue,
		// Ж3-и2: из чего сложилась выручка и с какого часа считаются сутки
		"revenue_today": gin.H{"deposits_pln": money.DepositsPLN, "goods_pln": money.GoodsPLN,
			"packs_pln": money.PacksPLN, "deposits": money.Deposits, "sales": money.GoodsSales},
		"day_from": day.From, "day_to": day.To, "report_hour": reportHour,
		"ready_wait_min":   settingInt64("ready_wait_min", readyWaitMinDef), // Ж4: форма посадки говорит, сколько ждём ПК без связи
		"expenses_due":     overviewExpensesDue(c),                          // З9-и5: бейдж «Бухгалтерии» — только владельцу
		"supplies_waiting": overviewSuppliesWaiting(c),                      // З10-и4: поставки ждут цен — в тот же бейдж
	})
}

// overviewExpensesDue — сколько регулярных расходов ждёт внесения: владельцу
// число, админу — nil (бухгалтерию он не видит, З-Р29).
func overviewExpensesDue(c *gin.Context) any {
	if c.GetString("user_role") != string(models.UserRoleOwner) {
		return nil
	}
	return len(dueList(expenseToday()))
}

// GET /admin/users?q=… — гости (поиск, свежие сверху).
//
// Е0-и3: ищем по нику, ТЕЛЕФОНУ и имени с фамилией, а не только по нику.
// Телефон сравниваем цифра к цифре (`regexp_replace`), потому что у стойки
// его называют как придётся, а в базе он лежит в E.164. Полное имя склеиваем
// через COALESCE — иначе строка с одним пустым полем схлопывается в NULL и
// «Ковальский» не находится, пока не заполнено имя.
//
// Про производительность честно: ILIKE '%…%' индексами не пользуется, это
// seq scan. На масштабе клуба (тысячи гостей) — доли миллисекунды; когда
// станет много, лечится триграммным индексом (pg_trgm), а не переписыванием
// логики. Гнаться за этим сейчас — оптимизировать то, чего нет.
//
// Ж5-и3 (№23, №43): список — про ГОСТЕЙ. По умолчанию role=player: персонал
// живёт в «Персонале», и «admin1» среди гостей у стойки только путал (role=all
// оставлен для служебных нужд). Фильтры и сортировка — на сервере, а не в
// браузере: таблица режется до 100 строк, и клиентский фильтр по ним показывал
// бы «с деньгами» только среди недавно активных.
//
//	filter=at_pc  — сейчас за ПК (есть активная сессия)
//	filter=money  — с деньгами на кошельке
//	filter=new    — новые: первое появление не раньше seg_new_days назад (З5-и1)
//	sort=last_active|nickname|level|coins|wallet|created|first_seen, dir=asc|desc
//
// «Сейчас» (ПК и прогноз «хватит до») отдаётся картой now_at по id гостя —
// одним запросом по активным сессиям найденных, тем же forecast.go, что в
// шторке ПК и карточке.
func handleAdminUsers(c *gin.Context) {
	now := time.Now()
	role := strings.TrimSpace(c.DefaultQuery("role", string(models.UserRolePlayer)))
	q := db.Model(&models.User{}).Limit(100)
	switch role {
	case "all":
	case string(models.UserRolePlayer), string(models.UserRoleAdmin), string(models.UserRoleOwner):
		q = q.Where("role = ?", role)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_role", "message": "role: player, admin, owner или all"})
		return
	}
	s := strings.TrimSpace(c.Query("q"))
	if s != "" {
		// Условие общее с резолвером посадки и брони (Е0-и4): «нашёлся
		// в поиске» и «сажается по этому же запросу» разъезжаться не должны.
		q = q.Where(guestSearchCondition(s))
	}
	filter := strings.TrimSpace(c.Query("filter"))
	switch filter {
	case "", "all":
		filter = ""
	case "at_pc":
		q = q.Where("id IN (?)", db.Model(&models.Session{}).Select("user_id").
			Where("status = ?", models.SessionStatusActive))
	case "money":
		q = q.Where("wallet_grosz > 0")
	case "new":
		// З5-и1 (З-Р10): новые на «сейчас» — первое появление (первая засчитанная
		// сессия, без сессий — регистрация, Д2) не раньше seg_new_days суток
		// назад, как в отчётах; «аккаунт моложе недели» ушёл
		q = q.Where(firstSeenSQL+" >= ?", newSince(now))
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_filter", "message": "filter: at_pc, money или new"})
		return
	}
	col, sortKey, dir, ok := usersOrder(c.Query("sort"), c.Query("dir"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_sort",
			"message": "sort: last_active, nickname, level, coins, wallet, created или first_seen; dir: asc или desc"})
		return
	}
	q = q.Order(col + " " + strings.ToUpper(dir))
	if sortKey != "last_active" {
		q = q.Order("last_active_at DESC") // равные значения — свежие сверху, как и раньше
	}
	var users []models.User
	if err := q.Find(&users).Error; err != nil {
		respondDBError(c, err)
		return
	}
	resp := gin.H{"count": len(users), "users": users, "role": role, "filter": filter, "sort": sortKey, "dir": dir,
		"new_days": segNewDays()} // З5-и1: подпись чипа «Новые за N дней»
	if filter == "new" && len(users) > 0 {
		resp["first_seen"] = firstSeenOf(users) // колонка «Впервые»
	}
	if nowAt := usersNowAt(users, now); len(nowAt) > 0 {
		resp["now_at"] = nowAt
	}
	if s != "" {
		// Чем гость совпал — половина смысла ответа у стойки: три Ковальских
		// в списке бесполезны, а «нашёлся по телефону» — уже ответ. Отдаём
		// отдельной картой, чтобы не ломать форму `users` для админки.
		matched := make(map[string]string, len(users))
		for i := range users {
			if r := guestMatchReason(&users[i], s); r != "" {
				matched[users[i].ID.String()] = r
			}
		}
		resp["matched"] = matched
	}
	c.JSON(http.StatusOK, resp)
}

// usersSortCols — белый список сортировок списка гостей: ключ запроса →
// выражение ORDER BY. Имя столбца из запроса в SQL не попадает никогда.
var usersSortCols = map[string]string{
	"last_active": "last_active_at",
	"nickname":    "LOWER(nickname)",
	"level":       "level",
	"coins":       "coins_balance",
	"wallet":      "wallet_grosz",
	"created":     "created_at",
	"first_seen":  firstSeenSQL, // З5-и1: первое появление (Д2)
}

// firstSeenOf — первое появление гостей картой id → момент (Д2, З5-и1).
func firstSeenOf(users []models.User) map[string]time.Time {
	ids := make([]string, 0, len(users))
	for i := range users {
		ids = append(ids, users[i].ID.String())
	}
	var rows []struct {
		ID        string
		FirstSeen time.Time
	}
	db.Model(&models.User{}).Select("id, "+firstSeenSQL+" AS first_seen").Where("id IN ?", ids).Scan(&rows)
	out := make(map[string]time.Time, len(rows))
	for _, r := range rows {
		out[r.ID] = r.FirstSeen
	}
	return out
}

// usersOrder — разобрать sort/dir (чистая, тест). Пусто — свежие сверху, как
// было; для ника естественный порядок — по алфавиту, для чисел и дат — с
// большего. ok=false — параметр вне списка.
func usersOrder(sortKey, dir string) (col, key, direction string, ok bool) {
	if sortKey == "" {
		sortKey = "last_active"
	}
	col, ok = usersSortCols[sortKey]
	if !ok {
		return "", "", "", false
	}
	switch dir {
	case "":
		dir = "desc"
		if sortKey == "nickname" {
			dir = "asc"
		}
	case "asc", "desc":
	default:
		return "", "", "", false
	}
	return col, sortKey, dir, true
}

// usersNowAt — «Сейчас» для списка гостей: по активным сессиям найденных —
// ПК, ожидание [Готов!]/шелла, пауза и прогноз «хватит до». Пусто, если никто
// из списка не играет.
func usersNowAt(users []models.User, now time.Time) map[string]gin.H {
	if len(users) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(users))
	byID := make(map[uuid.UUID]*models.User, len(users))
	for i := range users {
		ids = append(ids, users[i].ID)
		byID[users[i].ID] = &users[i]
	}
	var act []models.Session
	db.Preload("Computer").Where("status = ? AND user_id IN ?", models.SessionStatusActive, ids).Find(&act)
	if len(act) == 0 {
		return nil
	}
	out := make(map[string]gin.H, len(act))
	for i := range act {
		s := &act[i]
		row := gin.H{
			"session_id": s.ID, "started_at": s.StartedAt,
			"waiting": sessionWaitingReady(s), "paused": s.PausedAt != nil,
		}
		if u := byID[s.UserID]; u != nil && s.Computer != nil {
			row["computer"] = s.Computer.Name
			f := forecastSession(s, u, s.Computer.ZoneID, now)
			row["minutes_left"] = f.MinutesLeft
			if f.Until != nil {
				row["until"] = f.Until
			}
		}
		out[s.UserID.String()] = row
	}
	return out
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
		respondDBError(c, err)
		return
	}
	action, details := "unban", ""
	if status == models.UserStatusBanned {
		action = "ban"
		var s models.Session
		if db.Where("user_id = ? AND status = ?", user.ID, models.SessionStatusActive).
			First(&s).Error == nil {
			if res, err := finishSession(&s, nil, "admin"); err == nil {
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
		depQ = depQ.Where("created_at >= ?", clubDayStart(time.Now())) // Ж3: клубные сутки
	}
	depQ.Order("created_at DESC").Limit(20).Find(&deposits)

	var grants []models.AdminGrant
	db.Where("user_id = ?", user.ID).Order("created_at DESC").Limit(20).Find(&grants)

	// сводка за всё время (деньги — только owner)
	var agg struct {
		Cnt     int64
		Minutes int64
	}
	// З4-и1: снятая посадка в истории есть, в счётчике — нет
	countedSessions().
		Select("COUNT(*) AS cnt, COALESCE(SUM(minutes_used),0) AS minutes").
		Where("user_id = ?", user.ID).Scan(&agg)
	var depSum float64
	if ownerView {
		db.Model(&models.Deposit{}).Select("COALESCE(SUM(amount_pln),0)").
			Where("user_id = ? AND voided_at IS NULL", user.ID).Scan(&depSum) // Ж1
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

	// Г4-и3: счётчик no-show — копим факт для решений владельца о санкциях.
	var noShows int64
	db.Model(&models.Booking{}).
		Where("user_id = ? AND status = ?", user.ID, models.BookingStatusNoShow).
		Count(&noShows)

	stats := gin.H{
		"sessions_count": agg.Cnt,
		"hours_played":   agg.Minutes / 60,
		"no_show_count":  noShows,
	}
	// Г7/Р10: выданная и не рассчитанная кухня — видно прямо в карточке.
	// Ж6 (Ж-Р8): плюс бар в долг — одной строкой «должен N zł (кухня + бар)».
	if debt := guestDebt(user.ID); debt.TotalPLN > 0 {
		stats["kitchen_unpaid_count"] = debt.KitchenCount
		stats["kitchen_unpaid_pln"] = debt.KitchenPLN
		stats["bar_unpaid_count"] = debt.BarCount
		stats["bar_unpaid_pln"] = debt.BarPLN
		stats["unpaid_pln"] = debt.TotalPLN
	}
	// З11-и4: списанный и не возвращённый долг — «Долг списан … · Вернуть долг» (кнопка — у владельца)
	if wos := activeWriteOffs(user.ID); len(wos) > 0 {
		stats["debt_written_off"] = wos
	}
	if ownerView {
		stats["deposited_pln"] = depSum
	}
	// Ж2: запас минут по зонам («60 мин Standard · 18 мин без зоны») и зона ПК
	// активной сессии — форма «Время за монеты» подставляет её по умолчанию.
	// Ж5-и2 (№19): та же сессия целиком — «Сейчас за PC-02 · 42 мин · хватит
	// до 19:42» в шапке карточки и id для «Завершить сессию» прямо отсюда;
	// прогноз — общий с шторкой ПК и экраном гостя (forecast.go).
	var activeSess gin.H
	var act models.Session
	if db.Where("user_id = ? AND status = ?", user.ID, models.SessionStatusActive).First(&act).Error == nil {
		var pc models.Computer
		if db.Select("id, name, zone_id").First(&pc, "id = ?", act.ComputerID).Error == nil {
			now := time.Now()
			activeSess = gin.H{
				"id": act.ID, "computer": pc.Name, "computer_id": pc.ID, "zone_id": pc.ZoneID,
				"started_at": act.StartedAt, "paused_at": act.PausedAt, "paused_by": act.PausedBy,
				"ready_at": act.ReadyAt, "ready_deadline": act.ReadyDeadline, "shell_wait_until": act.ShellWaitUntil,
				"waiting":  sessionWaitingReady(&act), // окно [Готов!] или ожидание шелла — деньги не идут
				"rate_pln": act.EffectiveRatePLN, "billed_minutes": act.BilledMinutes, "zero_since": act.ZeroSince,
			}
			if z := zoneByID(pc.ZoneID); z != nil {
				activeSess["zone"] = z.Name
			}
			for k, v := range forecastFields(&user, forecastSession(&act, &user, pc.ZoneID, now)) {
				activeSess[k] = v
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"user":           user,
		"stats":          stats,
		"sessions":       sesOut,
		"deposits":       deposits,
		"grants":         grOut,
		"reserve":        grantMinutesByZone(user.ID, user.CoinMinutes),
		"active_session": activeSess,
		"money_scope":    map[bool]string{true: "all", false: "today"}[ownerView],
	})
}

// GET /admin/computers — все ПК + кто сейчас играет.
func handleAdminComputers(c *gin.Context) {
	var computers []models.Computer
	if err := db.Preload("Club").Order("name").Find(&computers).Error; err != nil {
		respondDBError(c, err)
		return
	}

	var active []models.Session
	db.Preload("User").Where("status = ?", models.SessionStatusActive).Find(&active)
	byComputer := map[string]*models.Session{}
	for i := range active {
		byComputer[active[i].ComputerID.String()] = &active[i]
	}

	now := time.Now()
	out := make([]gin.H, 0, len(computers))
	for _, pc := range computers {
		row := gin.H{
			"id": pc.ID, "name": pc.Name, "zone": pc.Zone, "status": pc.Status,
			// zone_id обязателен: без него селект зоны в шторке правки ПК не
			// подсвечивал текущую зону, и «Сохранить» после переименования
			// молча снимал зону — ПК уезжал на базовый тариф клуба
			// (ревью 26.08).
			"zone_id": pc.ZoneID,
			"club":    pc.Club.Name, "club_id": pc.ClubID,
			"pos_x": pc.PosX, "pos_y": pc.PosY, // схема зала (спринт А8)
			"mac":  pc.MAC,                     // WoL (Б8, редактор зала)
			"shell_online": hub.IsConnected(pc.ID.String()),
		}
		// Е1-и5: придержан под регистрацию — в зале это отдельная пометка,
		// а не «занят»: машина свободна, просто её не надо перехватывать.
		if computerHeld(&pc, now) {
			row["hold_until"] = pc.HoldUntil
		}
		if s, ok := byComputer[pc.ID.String()]; ok {
			sess := gin.H{
				"id": s.ID, "started_at": s.StartedAt,
				"nickname": nicknameOf(s.User),
				// Г2-и4: пауза видна в зале и в шторке ПК
				"paused_at": s.PausedAt, "paused_by": s.PausedBy,
				// Е1-и4: ждёт нажатия [Готов!] — в зале это «⏳ ждёт 6:12»,
				// и по этому же признаку в шторке появляется отмена посадки.
				"ready_at": s.ReadyAt, "ready_deadline": s.ReadyDeadline,
				// Ж4-и1: посадка ждёт шелл — «⏻ просыпается · 6:12», отмена посадки та же
				"shell_wait_until": s.ShellWaitUntil,
			}
			// Ж5-и1 (№18): деньги гостя прямо в шторке ПК — «тариф · кошелёк ·
			// запас · хватит до HH:MM», тем же прогнозом, что видит гость на
			// своём экране (forecast.go). user_id — ссылка на карточку и
			// «Пополнить» без второго поиска по нику.
			if s.User != nil {
				sess["user_id"] = s.UserID
				sess["rate_pln"] = s.EffectiveRatePLN
				sess["billed_minutes"] = s.BilledMinutes
				sess["charged_grosz"] = s.ChargedGrosz
				sess["zero_since"] = s.ZeroSince
				for k, v := range forecastFields(s.User, forecastSession(s, s.User, pc.ZoneID, now)) {
					sess[k] = v
				}
			}
			row["session"] = sess
		} else if pc.Status == models.ComputerStatusInSession || pc.Status == models.ComputerStatusReserved {
			// Ж4-и3 (Ж-Р9): статус «занят», а сессии нет — залипший ПК. Не лечим
			// молча: плитке пометка «⚠ занят без сессии», в шторке кнопка
			// «Освободить ПК» (POST /admin/computers/:id/release).
			row["stuck"] = true
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
	res, err := finishSession(&session, nil, "admin")
	if err != nil {
		if errors.Is(err, errSessionGone) {
			c.JSON(http.StatusConflict, gin.H{"code": "session_closed", "message": "Сессия уже завершена"})
			return
		}
		respondDBError(c, err)
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
		respondDBError(c, err)
		return
	}
	action := "pc_available"
	if req.Status == models.ComputerStatusMaintenance {
		action = "pc_maintenance"
	} else {
		checkWaitlistNotify(pc.ClubID) // Б9: ПК вернулся из ремонта — зовём очередь
	}
	logAdminAction(c, action, nil, pc.Name)
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "name": pc.Name, "status": req.Status})
}

// POST /admin/computers/:id/power {action: on|restart|shutdown} — Б8.
// on — WoL через живого агента-соседа (бэкенд в докере до LAN-broadcast
// не достаёт); restart/shutdown — адресно агенту ПК. При активной сессии —
// 409: сначала заверши сессию, время гостя не сгорает.
func handleAdminPCPower(c *gin.Context) {
	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil ||
		(req.Action != "on" && req.Action != "restart" && req.Action != "shutdown") {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_action", "message": "action: on | restart | shutdown"})
		return
	}
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}

	if req.Action == "on" {
		if pc.MAC == nil || *pc.MAC == "" {
			c.JSON(http.StatusBadRequest, gin.H{"code": "no_mac",
				"message": "У «" + pc.Name + "» нет MAC — владелец задаёт его в редакторе зала"})
			return
		}
		proxyID, ok := hub.AnyClientInClub(pc.ClubID.String())
		if !ok {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "no_agents",
				"message": "В клубе нет живых агентов — WoL послать некому"})
			return
		}
		if err := hub.Send(proxyID, websocket.MsgWOL, gin.H{"mac": *pc.MAC}); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{"code": "send_failed", "message": err.Error()})
			return
		}
		logAdminAction(c, "pc_power", nil, pc.Name+" — включение (WoL)")
		c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "status": "wol_sent"})
		return
	}

	var active int64
	db.Model(&models.Session{}).
		Where("computer_id = ? AND status = ?", pc.ID, models.SessionStatusActive).Count(&active)
	if active > 0 {
		c.JSON(http.StatusConflict, gin.H{"code": "has_session",
			"message": "На ПК идёт сессия — сначала заверши её"})
		return
	}
	if !hub.IsConnected(pc.ID.String()) {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "shell_offline",
			"message": "Shell оффлайн — команду доставить некому"})
		return
	}
	if err := hub.Send(pc.ID.String(), websocket.MsgPCPower, gin.H{"action": req.Action}); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"code": "send_failed", "message": err.Error()})
		return
	}
	label := map[string]string{"restart": "перезагрузка", "shutdown": "выключение"}[req.Action]
	logAdminAction(c, "pc_power", nil, pc.Name+" — "+label)
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "status": req.Action})
}

// POST /admin/computers/:id/release — «Освободить ПК» (Ж4-и3, Ж-Р9): статус
// in_session или reserved без активной сессии — залипший ПК (обрыв на
// завершении, ручная правка базы, старые данные). Не самолечение: админ видит
// пометку «⚠ занят без сессии» и жмёт кнопку сам. В транзакции: активной
// сессии нет → available; есть — 409, потому что тогда ПК занят по-честному.
func handleAdminPCRelease(c *gin.Context) {
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	if pc.Status != models.ComputerStatusInSession && pc.Status != models.ComputerStatusReserved {
		c.JSON(http.StatusConflict, gin.H{"code": "not_stuck",
			"message": "ПК не залип: статус «" + string(pc.Status) + "» — освобождать нечего"})
		return
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		var active int64
		if err := tx.Model(&models.Session{}).
			Where("computer_id = ? AND status = ?", pc.ID, models.SessionStatusActive).Count(&active).Error; err != nil {
			return err
		}
		if active > 0 {
			return errHasSession
		}
		res := tx.Model(&models.Computer{}).
			Where("id = ? AND status IN ?", pc.ID, []models.ComputerStatus{models.ComputerStatusInSession, models.ComputerStatusReserved}).
			Update("status", models.ComputerStatusAvailable)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errNotStuck
		}
		return nil
	})
	if errors.Is(err, errHasSession) {
		c.JSON(http.StatusConflict, gin.H{"code": "has_session", "message": "На ПК идёт сессия — это не залипание, заверши её"})
		return
	}
	if errors.Is(err, errNotStuck) {
		c.JSON(http.StatusConflict, gin.H{"code": "not_stuck", "message": "ПК уже освободили"})
		return
	}
	if err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "pc_release", nil, pc.Name+" — был «"+string(pc.Status)+"» без сессии, освобождён")
	hub.AdminBroadcast("pc_release", map[string]any{"computer_id": pc.ID.String(), "computer": pc.Name})
	checkWaitlistNotify(pc.ClubID) // Б9: свободный ПК — очередь узнаёт
	c.JSON(http.StatusOK, gin.H{"computer_id": pc.ID, "name": pc.Name, "status": models.ComputerStatusAvailable, "was": pc.Status})
}

var (
	errHasSession = errors.New("has_session")
	errNotStuck   = errors.New("not_stuck")
)

// POST /admin/computers/:id/session {nickname} — посадить гостя за этот ПК
// (Б8): без пароля гостя, тариф и таланты — его собственные (общий
// startSessionFor). Инвариант цели тот же, что у депозита/гранта.
func handleAdminSeatGuest(c *gin.Context) {
	var pc models.Computer
	if err := db.First(&pc, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "computer_not_found", "message": "ПК не найден"})
		return
	}
	var req struct {
		Nickname   string `json:"nickname"`    // строгий ник (как понимала админка до Е0)
		Guest      string `json:"guest"`       // Е0-и4: ник, телефон ИЛИ имя
		GuestID    string `json:"guest_id"`    // Е0-и5б: выбран из списка кандидатов
		PlannedMin *int   `json:"planned_min"` // Г3: сколько гость планирует (для ПК с бронью)
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_request", "message": "Нужен ник, телефон или имя гостя"})
		return
	}
	user := lookupGuestForAction(c, req.GuestID, req.Guest, req.Nickname) // 404 и 409 пишет сам
	if user == nil {
		return
	}
	if ok, code := canTargetUser(user.Role, user.ID.String(), c.GetString("user_id")); !ok {
		msg := map[string]string{
			"cannot_touch_self":  "Нельзя применить действие к самому себе",
			"cannot_touch_staff": "Действия персонала применимы только к гостям",
		}[code]
		c.JSON(http.StatusForbidden, gin.H{"code": code, "message": msg})
		return
	}
	if user.Status == models.UserStatusBanned {
		c.JSON(http.StatusConflict, gin.H{"code": "banned", "message": "Аккаунт заблокирован — сначала разбань"})
		return
	}
	cid := pc.ID.String()
	code, resp := startSessionFor(user.ID, &cid, req.PlannedMin, false) // посадка от стойки: окно [Готов!] (Е1)
	if code < 300 {
		// Ник в ответе обязателен: без него админка показывала в подтверждении
		// поисковый ЗАПРОС, а не того, кого реально посадили — при выборе из
		// списка однофамильцев это «списать деньги с чужого кошелька»
		// (ревью 26.08).
		resp["nickname"] = user.Nickname
		target := user.ID
		details := user.Nickname + " за " + pc.Name
		if _, waiting := resp["shell_wait_until"]; waiting { // Ж4: в журнале видно, что машина была не на связи
			details += " · ПК не на связи, WoL: " + fmt.Sprint(resp["wol"])
		}
		logAdminAction(c, "session_start", &target, details)
		if from, _ := resp["from_waitlist"].(bool); from { // Б9: посадка из очереди
			logAdminAction(c, "waitlist_seat", &target, user.Nickname+" за "+pc.Name)
		}
	}
	c.JSON(code, resp)
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
		respondDBError(c, err)
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
		Nickname    string  `json:"nickname"`  // строгий ник (как понимала админка до Е0)
		Guest       string  `json:"guest"`     // Е0-и4: ник, телефон ИЛИ имя
		GuestID     string  `json:"guest_id"`  // Е0-и5б: выбран из списка кандидатов
		ComputerID  *string `json:"computer_id"`
		StartTime   string  `json:"start_time" binding:"required"` // RFC3339
		DurationMin int     `json:"duration_min"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
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

	user := lookupGuestForAction(c, req.GuestID, req.Guest, req.Nickname) // 404 и 409 пишет сам
	if user == nil {
		return
	}
	// Ж0-и4 (Ж-Р5): бан, лимит живых броней и «одна бронь на одно время» —
	// walk-in от стойки больше не обходит правила гостевой брони.
	if code, msg := bookingGuestChecks(user, start, req.DurationMin, time.Now()); code != "" {
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
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
		if isBookingOverlapErr(err) { // Ж0-и3: ограничение bookings_no_overlap (миграция 061)
			c.JSON(http.StatusConflict, gin.H{"code": "slot_busy", "message": slotBusyRaceMessage})
			return
		}
		respondDBError(c, err)
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
	// Ж0-и4: пока бронь лежала отменённой, гостя могли забанить или он мог
	// забронировать другое время — возвращаем по тем же правилам, что создаём.
	var guest models.User
	if err := db.First(&guest, "id = ?", booking.UserID).Error; err == nil {
		if code, msg := bookingGuestChecks(&guest, booking.StartTime, booking.DurationMin, time.Now()); code != "" {
			c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
			return
		}
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
		if isBookingOverlapErr(err) { // Ж0-и3
			c.JSON(http.StatusConflict, gin.H{"code": "slot_busy", "message": "Время уже занято другой бронью"})
			return
		}
		respondDBError(c, err)
		return
	}
	target := booking.UserID
	logAdminAction(c, "booking_restore", &target, "на "+booking.StartTime.Format("02.01 15:04"))
	notifyUser(booking.UserID, "booking_restore", map[string]any{"start_time": booking.StartTime}) // Б4
	hub.AdminBroadcast("booking", map[string]any{"kind": "restore", "start_time": booking.StartTime})
	c.JSON(http.StatusOK, gin.H{"booking_id": booking.ID, "status": models.BookingStatusConfirmed})
}
