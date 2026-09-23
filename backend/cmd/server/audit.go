package main

// Журнал-аудит (спринт А4, ADMIN.md): единая лента действий персонала.
// Источники: admin_grants (ручные начисления), deposits (пополнения),
// admin_actions (остальное: баны, форс-энды, ремонт ПК, брони, каталог).

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// logAdminAction — запись действия в admin_actions. Сбой записи не роняет
// основное действие — только строка в лог сервера.
func logAdminAction(c *gin.Context, action string, targetUserID *uuid.UUID, details string) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		return
	}
	entry := models.AdminAction{AdminID: adminID, Action: action, TargetUserID: targetUserID, Details: details}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("audit: действие %s не записалось: %v", action, err)
	}
}

// logAdminActionAs — то же, но без HTTP-контекста: действие совершил сервер
// от имени человека (Е3-и1: смену открыл вход в систему, а не нажатие).
func logAdminActionAs(adminID uuid.UUID, action string, targetUserID *uuid.UUID, details string) {
	entry := models.AdminAction{AdminID: adminID, Action: action, TargetUserID: targetUserID, Details: details}
	if err := db.Create(&entry).Error; err != nil {
		log.Printf("audit: действие %s не записалось: %v", action, err)
	}
}

// logLoginFailed — серия неверных паролей к аккаунту персонала (З0-и3, №28):
// одна строка журнала на серию — кто, сколько, с какого IP. Видна только
// владельцу (hiddenFromAdmin) и в «действия» сотрудника не считается.
func logLoginFailed(u *models.User, n int, ip string) {
	target := u.ID
	logAdminActionAs(u.ID, "login_failed", &target, fmt.Sprintf(
		"%d неверных паролей подряд · IP %s · дальше вход не чаще раза в %d с", n, ip, int(loginStaffPace.Seconds())))
}

// GET /admin/audit — лента событий: источники (начисления, депозиты и их
// отмены, продажи и их отмены, admin_actions) сливаются по времени.
// Роли (Б1-и4, решение №3): owner видит всё; admin — текущие клубные сутки и
// без owner-событий (settings_update, staff_*).
//
// Ж3-и3 (№14, №22): параметры и слияние.
//
//	?day=YYYY-MM-DD — клубные сутки (только owner; admin всегда получает
//	                  текущие), без day у owner — всё время;
//	?before=RFC3339 — курсор: события строго раньше момента («ещё 100»);
//	?limit=1..200   — размер страницы (по умолчанию 100);
//	?kind=a,b,c     — виды событий (ключи как в журнале: deposit,
//	                  deposit_void, sale, sale_void, grant, ban, …).
//
// Раньше бралось «по 100 свежих с каждого источника» и резалось до 100:
// в бойкий день депозит со 150-й позиции пропадал, хотя формально был
// «сегодня». Теперь каждый источник режется в ОДНОМ окне [from, before) тем
// же лимитом, что и итог: любой элемент из настоящего топ-limit объединения
// входит и в топ-limit своего источника, так что страница точная, а
// следующая начинается с next_before. Отмены (deposit_void, sale_void)
// живут в окне по моменту отмены, а не создания — иначе вчерашний депозит,
// отменённый сегодня, не попадал бы в сегодняшний журнал.
func handleAdminAudit(c *gin.Context) {
	ownerView := c.GetString("user_role") == string(models.UserRoleOwner)
	now := time.Now()
	reportHour := int(settingInt64("report_hour", 8))

	limit := 100
	if v, err := strconv.Atoi(c.Query("limit")); err == nil {
		limit = max(1, min(v, 200))
	}
	var from, to time.Time
	windowed := true
	dayKey := ""
	if ownerView {
		if d := c.Query("day"); d != "" {
			f, t, key, ok := shiftWindow(d, reportHour, now)
			if !ok {
				c.JSON(http.StatusBadRequest, gin.H{"code": "bad_day", "message": "day — в формате YYYY-MM-DD"})
				return
			}
			from, to, dayKey = f, t, key
		} else {
			windowed = false
		}
	} else {
		from, to, dayKey, _ = shiftWindow("", reportHour, now)
	}
	var before time.Time
	if b := c.Query("before"); b != "" {
		t, err := time.Parse(time.RFC3339Nano, b)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "bad_before", "message": "before — момент в формате RFC3339"})
			return
		}
		before = t
	}
	kinds := map[string]bool{}
	for _, k := range strings.Split(c.Query("kind"), ",") {
		if k = strings.TrimSpace(k); k != "" {
			kinds[k] = true
		}
	}
	want := func(k string) bool { return len(kinds) == 0 || kinds[k] }
	// scope — одно окно для всех источников по нужной колонке времени
	scope := func(q *gorm.DB, col string) *gorm.DB {
		if windowed {
			q = q.Where(col+" >= ? AND "+col+" < ?", from, to)
		}
		if !before.IsZero() {
			q = q.Where(col+" < ?", before)
		}
		return q.Order(col + " DESC").Limit(limit)
	}
	hiddenFromAdmin := func(action string) bool {
		return !ownerView &&
			(action == "settings_update" || action == "login_failed" || strings.HasPrefix(action, "staff_"))
	}

	type item struct {
		ID        string
		CreatedAt time.Time
		Kind      string
		AdminID   uuid.UUID
		UserID    *uuid.UUID
		Text      string
	}
	items := []item{}

	if want("grant") {
		var grants []models.AdminGrant
		scope(db.Model(&models.AdminGrant{}), "created_at").Find(&grants)
		for _, g := range grants {
			text := g.Reason
			if g.GrantType == "xp" && g.Amount != nil {
				text = fmt.Sprintf("+%d XP — %s", *g.Amount, g.Reason)
			} else if g.CaseTier != nil {
				text = fmt.Sprintf("кейс %s — %s", *g.CaseTier, g.Reason)
			}
			uid := g.UserID
			items = append(items, item{"grant:" + g.ID.String(), g.CreatedAt, "grant", g.AdminID, &uid, text})
		}
	}

	depositText := func(d *models.Deposit) string {
		return fmt.Sprintf("%.0f zł → +%d монет (%s)", d.AmountPLN, d.CoinsGranted+d.BonusCoins, d.Method)
	}
	depositAdmin := func(d *models.Deposit) uuid.UUID {
		if d.CreatedBy != nil {
			return *d.CreatedBy
		}
		return uuid.Nil
	}
	if want("deposit") {
		var deposits []models.Deposit
		scope(db.Model(&models.Deposit{}), "created_at").Find(&deposits)
		for i := range deposits {
			d := &deposits[i]
			uid := d.UserID
			items = append(items, item{"deposit:" + d.ID.String(), d.CreatedAt, "deposit", depositAdmin(d), &uid, depositText(d)})
		}
	}
	if want("deposit_void") {
		// Ж1: отменённый депозит — второй строкой в момент отмены, как у продаж:
		// сколько удалось вернуть, а сколько гость уже отыграл.
		var voided []models.Deposit
		scope(db.Model(&models.Deposit{}).Where("voided_at IS NOT NULL"), "voided_at").Find(&voided)
		for i := range voided {
			d := &voided[i]
			by := depositAdmin(d)
			if d.VoidedBy != nil {
				by = *d.VoidedBy
			}
			vt := fmt.Sprintf("отмена: %.0f zł (%s) · снято с кошелька %.2f zł", d.AmountPLN, d.Method, models.PLNFromGrosz(d.RefundGrosz))
			if lost := models.GroszFromPLN(d.AmountPLN) - d.RefundGrosz; lost > 0 {
				vt += fmt.Sprintf(", уже отыграно %.2f zł", models.PLNFromGrosz(lost))
			}
			if d.CoinsTaken > 0 {
				vt += fmt.Sprintf(" · забрано %d монет", d.CoinsTaken)
			}
			uid := d.UserID
			items = append(items, item{"deposit_void:" + d.ID.String(), *d.VoidedAt, "deposit_void", by, &uid, vt})
		}
	}

	// Продажи товаров (В2) — свой источник, как у депозитов; отменённая
	// продажа даёт вторую строку в момент отмены. Ж6-и2: позиции одного чека
	// (общий receipt_id, одна секунда) — одной строкой «чек: A ×2, B — 23.00 zł»;
	// Ж6-и1: продажа в долг помечена «(в долг)», её расчёт — отдельной строкой
	// sale_paid в момент денег.
	saleMethod := func(s *models.Sale) string {
		if s.PaidAt == nil && s.VoidedAt == nil {
			return "в долг"
		}
		if s.Postpaid && s.PaidAt != nil {
			return s.Method + ", был в долг"
		}
		return s.Method
	}
	saleText := func(s *models.Sale) string {
		return fmt.Sprintf("%s ×%d — %.2f zł (%s)", s.Name, s.Qty, s.TotalPLN, saleMethod(s))
	}
	receiptText := func(group []*models.Sale) string {
		names := make([]string, 0, len(group))
		total := 0.0
		for _, s := range group {
			names = append(names, receiptItemName(s))
			total += s.TotalPLN
		}
		return fmt.Sprintf("чек: %s — %.2f zł (%s)", strings.Join(names, ", "), roundPLN(total), saleMethod(group[0]))
	}
	if want("sale") {
		var sales []models.Sale
		scope(db.Model(&models.Sale{}), "created_at").Order("created_at").Find(&sales)
		byReceipt := map[string][]*models.Sale{}
		order := []string{}
		for i := range sales {
			s := &sales[i]
			if s.ReceiptID == nil {
				items = append(items, item{"sale:" + s.ID.String(), s.CreatedAt, "sale", s.CreatedBy, s.UserID, saleText(s)})
				continue
			}
			k := s.ReceiptID.String()
			if _, ok := byReceipt[k]; !ok {
				order = append(order, k)
			}
			byReceipt[k] = append(byReceipt[k], s)
		}
		for _, k := range order {
			group := byReceipt[k]
			first := group[0]
			if len(group) == 1 {
				items = append(items, item{"sale:" + first.ID.String(), first.CreatedAt, "sale", first.CreatedBy, first.UserID, saleText(first)})
				continue
			}
			items = append(items, item{"receipt:" + k, first.CreatedAt, "sale", first.CreatedBy, first.UserID, receiptText(group)})
		}
	}
	if want("sale_paid") {
		// расчёт долга (Ж6): деньги пришли позже продажи — строка в момент paid_at
		var paid []models.Sale
		scope(db.Model(&models.Sale{}).Where("postpaid = TRUE AND paid_at IS NOT NULL AND paid_at <> created_at"), "paid_at").Find(&paid)
		for i := range paid {
			s := &paid[i]
			by := s.CreatedBy
			if s.PaidBy != nil {
				by = *s.PaidBy
			}
			items = append(items, item{"sale_paid:" + s.ID.String(), *s.PaidAt, "sale_paid", by, s.UserID,
				fmt.Sprintf("расчёт: %s ×%d — %.2f zł (%s)", s.Name, s.Qty, s.TotalPLN, s.Method)})
		}
	}
	if want("sale_void") {
		var voided []models.Sale
		scope(db.Model(&models.Sale{}).Where("voided_at IS NOT NULL"), "voided_at").Find(&voided)
		for i := range voided {
			s := &voided[i]
			by := s.CreatedBy
			if s.VoidedBy != nil {
				by = *s.VoidedBy
			}
			vt := "отмена: " + fmt.Sprintf("%s ×%d — %.2f zł (%s)", s.Name, s.Qty, s.TotalPLN, s.Method)
			if s.PaidAt == nil {
				vt += " · долг списан, денег не брали"
			}
			items = append(items, item{"sale_void:" + s.ID.String(), *s.VoidedAt, "sale_void", by, s.UserID, vt})
		}
	}

	// admin_actions — всё остальное; при фильтре по видам — только они
	// (виды, которых в admin_actions нет, просто не дадут строк).
	actQ := db.Model(&models.AdminAction{})
	if len(kinds) > 0 {
		list := make([]string, 0, len(kinds))
		for k := range kinds {
			list = append(list, k)
		}
		actQ = actQ.Where("action IN ?", list)
	}
	var actions []models.AdminAction
	scope(actQ, "created_at").Find(&actions)
	for _, a := range actions {
		if hiddenFromAdmin(a.Action) {
			continue
		}
		by := a.AdminID
		if a.Action == "login_failed" {
			by = uuid.Nil // З0-и3: кто подбирал пароль — неизвестно; чей аккаунт — в колонке гостя
		}
		items = append(items, item{"action:" + a.ID.String(), a.CreatedAt, a.Action, by, a.TargetUserID, a.Details})
	}

	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.After(items[j].CreatedAt) })
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	idSet := map[string]bool{}
	for _, it := range items {
		if it.AdminID != uuid.Nil {
			idSet[it.AdminID.String()] = true
		}
		if it.UserID != nil {
			idSet[it.UserID.String()] = true
		}
	}
	nick := map[string]string{}
	if len(idSet) > 0 {
		ids := make([]string, 0, len(idSet))
		for id := range idSet {
			ids = append(ids, id)
		}
		var users []models.User
		db.Where("id IN ?", ids).Find(&users)
		for _, u := range users {
			nick[u.ID.String()] = u.Nickname
		}
	}

	out := make([]gin.H, 0, len(items))
	for _, it := range items {
		row := gin.H{"id": it.ID, "created_at": it.CreatedAt, "kind": it.Kind, "text": it.Text}
		if it.AdminID != uuid.Nil {
			row["admin"] = nick[it.AdminID.String()]
		}
		if it.UserID != nil {
			row["nickname"] = nick[it.UserID.String()]
		}
		out = append(out, row)
	}
	resp := gin.H{"count": len(out), "items": out, "has_more": hasMore, "limit": limit}
	if hasMore && len(items) > 0 {
		resp["next_before"] = items[len(items)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	if windowed {
		resp["day"], resp["from"], resp["to"] = dayKey, from, to
	}
	c.JSON(http.StatusOK, resp)
}
