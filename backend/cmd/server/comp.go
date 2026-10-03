package main

// З11-и5 «Подарено» (OWNER.md, трек З; №18; З-Р12, Д6; решение основателя
// 30.09).
//
// Продажа у стойки, выдача пакета времени и заказ кухни от стойки могут быть
// подарком: способ comp. Подарок — не выручка и не ящик (moneySources, ящик
// берёт только наличные); товар уходит со склада как обычно, пакет даёт минуты
// как обычно, за заказом кухни стоит его продажа с тем же способом. Причина
// обязательна (решение основателя 30.09): дарит обычно админ, и владелец
// должен видеть, за что. Админ дарит в пределах дневного потолка компенсаций —
// потолок один на компенсации (К8) и подарки (Д6), по цене подарка; владелец —
// без лимита.
//
// В «Деньгах» — «Подарено» по цене и по себестоимости (товары — по партиям, как
// маржа; у пакетов себестоимости нет) и список подарков периода с причинами.
// В марже, «Что покупают» и «Возвратах» подарков нет: это не продажа.

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxCompReason = 200 // символов в причине подарка

var compErrors = map[string]string{
	"comp_reason":      "У подарка нужна причина — за что дарим (её увидит владелец)",
	"comp_reason_long": fmt.Sprintf("Причина подарка — до %d символов", maxCompReason),
}

// compReason — причина подарка из запроса (чистая, тест): обрезанная, не пустая,
// не длиннее maxCompReason; второе значение — код отказа.
func compReason(s string) (string, string) {
	r := strings.TrimSpace(s)
	switch {
	case r == "":
		return "", "comp_reason"
	case len([]rune(r)) > maxCompReason:
		return "", "comp_reason_long"
	}
	return r, ""
}

// compFail — 400 по причине подарка.
func compFail(c *gin.Context, code string) {
	c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": compErrors[code]})
}

// compAsked — способ comp и его причина: не comp — "", true; comp без причины
// или с длинной — отвечает 400 сам и false.
func compAsked(c *gin.Context, method, reason string) (string, bool) {
	if method != models.SaleMethodComp {
		return "", true
	}
	r, code := compReason(reason)
	if code != "" {
		compFail(c, code)
		return "", false
	}
	return r, true
}

// giftsGrosz — сколько сотрудник подарил с момента since, по цене подарка:
// товары и заказы кухни (их продажи) и пакеты; отменённые подарки не считаются.
func giftsGrosz(adminID uuid.UUID, since time.Time) int64 {
	var g int64
	db.Raw(`SELECT COALESCE((SELECT SUM(ROUND(total_pln * 100)) FROM sales
	                          WHERE method = 'comp' AND voided_at IS NULL AND created_by = ? AND created_at >= ?), 0)::bigint
	             + COALESCE((SELECT SUM(ROUND(price_pln * 100)) FROM user_packages
	                          WHERE method = 'comp' AND voided_at IS NULL AND created_by = ? AND created_at >= ?), 0)::bigint`,
		adminID, since, adminID, since).Scan(&g)
	return g
}

// giftCapOK — потолок подарков админа (Д6): тот же дневной потолок, что у
// компенсаций, — компенсации деньгами и подарки вместе, клубные сутки; владелец
// не ограничен. Сверх — 403 day_cap, пишет сам.
func giftCapOK(c *gin.Context, adminID uuid.UUID, addGrosz int64) bool {
	if c.GetString("user_role") != string(models.UserRoleAdmin) {
		return true
	}
	capPLN := settingInt64("admin_day_comp_cap_pln", 0)
	if capPLN <= 0 {
		return true
	}
	since := clubDayStart(time.Now())
	var inc int64
	db.Model(&models.Incident{}).Select("COALESCE(SUM(comp_grosz),0)").
		Where("resolved_by = ? AND compensated_at >= ?", adminID, since).Scan(&inc)
	used := models.PLNFromGrosz(inc + giftsGrosz(adminID, since))
	if adminDayCapExceeded(used, models.PLNFromGrosz(addGrosz), float64(capPLN)) {
		c.JSON(http.StatusForbidden, gin.H{"code": "day_cap", "message": fmt.Sprintf(
			"Дневной лимит подарков и компенсаций админа: %d zł (уже выдано %.0f zł) — больше может только владелец", capPLN, used)})
		return false
	}
	return true
}

// compText — «подарено: за что» для журнала.
func compText(reason *string) string {
	if reason == nil {
		return "подарено"
	}
	return "подарено: " + *reason
}

// giftRow — один подарок периода.
type giftRow struct {
	Kind      string // sale | package
	ID        uuid.UUID
	Name      string
	Qty       int
	Grosz     int64
	Reason    string
	UserID    *uuid.UUID
	CreatedBy *uuid.UUID
	CreatedAt time.Time
}

const maxGiftItems = 100

// giftsSQL — подарки [from, to) по моменту подарка, свежие сверху; отменённые
// не считаются.
const giftsSQL = `
SELECT 'sale' AS kind, id, name, qty, ROUND(total_pln * 100)::bigint AS grosz, COALESCE(comp_reason, '') AS reason,
       user_id, created_by, created_at
  FROM sales WHERE method = 'comp' AND voided_at IS NULL AND created_at >= ? AND created_at < ?
UNION ALL
SELECT 'package', id, name, 1, ROUND(price_pln * 100)::bigint, COALESCE(comp_reason, ''), user_id, created_by, created_at
  FROM user_packages WHERE method = 'comp' AND voided_at IS NULL AND created_at >= ? AND created_at < ?
ORDER BY created_at DESC, kind, id`

// giftsIn — «Подарено» за период: по цене (товары и пакеты), по себестоимости
// (товары — по партиям, как маржа), список с причинами.
func giftsIn(from, to time.Time) gin.H {
	var rows []giftRow
	db.Raw(giftsSQL, from, to, from, to).Scan(&rows)
	var goods, packs int64
	var packCount int
	guests := map[uuid.UUID]bool{}
	for _, r := range rows {
		if r.Kind == "package" {
			packs += r.Grosz
			packCount++
		} else {
			goods += r.Grosz
		}
		if r.UserID != nil {
			guests[*r.UserID] = true
		}
	}
	ge := giftsEcon(from, to) // себестоимость подаренных товаров — по партиям
	out := gin.H{
		"pln": models.PLNFromGrosz(goods + packs), "count": len(rows), "guests": len(guests),
		"goods_pln": models.PLNFromGrosz(goods), "goods_units": ge.Items, "packs_pln": models.PLNFromGrosz(packs), "packs": packCount,
		"cost_pln": nil, "costed_units": ge.Costed, "no_cost_units": ge.Items - ge.Costed,
	}
	if ge.Costed > 0 {
		out["cost_pln"] = models.PLNFromGrosz(ge.Cost)
	}
	shown := rows[:min(len(rows), maxGiftItems)]
	ids := make([]string, 0, len(shown)*2)
	for _, r := range shown {
		for _, u := range []*uuid.UUID{r.UserID, r.CreatedBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	nick := nicknamesByID(ids)
	who := func(u *uuid.UUID) string {
		if u == nil {
			return ""
		}
		return nick[u.String()]
	}
	items := make([]gin.H, 0, len(shown))
	for _, r := range shown {
		items = append(items, gin.H{"kind": r.Kind, "id": r.ID, "name": r.Name, "qty": r.Qty,
			"price_pln": models.PLNFromGrosz(r.Grosz), "reason": r.Reason, "user_id": r.UserID, "guest": who(r.UserID),
			"by": who(r.CreatedBy), "created_at": r.CreatedAt})
	}
	out["items"], out["items_total"] = items, len(rows)
	return out
}
