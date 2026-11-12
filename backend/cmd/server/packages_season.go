package main

// З24-и5 «Сезонные пакеты — сервер» (OWNER.md, трек З; №55; миграция 101). У пакета каталога — когда он продаётся
// (sell_from, sell_to — даты по календарю клуба включительно), сколько раз один гость может его купить (per_guest; 0 —
// без ограничения; отменённые выдачи не в счёт), «только новым» (new_only: первое появление не раньше seg_new_days суток
// назад — как «Новые» в отчётах, З5-и1) и когда тратятся его минуты (spend_days_mask, spend_start_min, spend_end_min —
// дни недели и окно по стенным часам клуба, как у расписания цен; по умолчанию — всегда).
//
// Условия продажи проверяет issuePackage под замком строки гостя — у стойки, подарком и самим гостем в приложении
// одинаково, и две покупки разом лимит не обойдут; отказ — 409 с причиной словами. Окно списания снимается в выдачу
// (user_packages): правка каталога не меняет условий проданного. Биллинг (livePackagesFor) и прогноз (livePackMinutes)
// берут только пакеты, чьё окно идёт сейчас; остальные ждут своего часа, а сессия тем временем платит другим запасом и
// кошельком. Каталог гостя (GET /me/packages/catalog) показывает только то, что этот гость может купить сейчас.

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxPackPerGuest = 100

// отказы продажи — 409 с причиной словами (seasonFail)
var seasonCodes = map[string]bool{"not_on_sale": true, "new_only": true, "guest_limit": true}

func init() {
	packageErrors["bad_sell_dates"] = "Продажа — даты ГГГГ-ММ-ДД, «с» не позже «по»"
	packageErrors["bad_per_guest"] = fmt.Sprintf("На гостя — от 0 (без ограничения) до %d", maxPackPerGuest)
	packageErrors["bad_spend_days"] = "Дни списания — хотя бы один"
	packageErrors["bad_spend_time"] = "Часы списания — от 00:00 до 24:00, начало — до 23:59, начало и конец разные"
}

// seasonRequest — поля сезонности в запросе пакета (встроены в packageRequest; не прислано — как было).
type seasonRequest struct {
	SellFrom   *string `json:"sell_from"` // ГГГГ-ММ-ДД; "" — без границы
	SellTo     *string `json:"sell_to"`
	PerGuest   *int    `json:"per_guest"`
	NewOnly    *bool   `json:"new_only"`
	SpendDays  *int    `json:"spend_days_mask"`
	SpendStart *int    `json:"spend_start_min"`
	SpendEnd   *int    `json:"spend_end_min"`
}

// apply — поля запроса поверх пакета и проверка; код ошибки или "".
func (q seasonRequest) apply(p *models.TimePackage) string {
	for _, f := range []struct {
		in  *string
		out **time.Time
	}{{q.SellFrom, &p.SellFrom}, {q.SellTo, &p.SellTo}} {
		if f.in != nil {
			d, ok := ruleDate(*f.in)
			if !ok {
				return "bad_sell_dates"
			}
			*f.out = d
		}
	}
	if q.PerGuest != nil {
		p.PerGuest = *q.PerGuest
	}
	if q.NewOnly != nil {
		p.NewOnly = *q.NewOnly
	}
	if q.SpendDays != nil {
		p.SpendDaysMask = *q.SpendDays
	}
	if q.SpendStart != nil {
		p.SpendStartMin = *q.SpendStart
	}
	if q.SpendEnd != nil {
		p.SpendEndMin = *q.SpendEnd
	}
	return validateSeason(*p)
}

// validateSeason — границы полей сезонности (чистая, тест).
func validateSeason(p models.TimePackage) string {
	switch {
	case p.SellFrom != nil && p.SellTo != nil && p.SellTo.Before(*p.SellFrom):
		return "bad_sell_dates"
	case p.PerGuest < 0 || p.PerGuest > maxPackPerGuest:
		return "bad_per_guest"
	case p.SpendDaysMask < 1 || p.SpendDaysMask > 127:
		return "bad_spend_days"
	case p.SpendStartMin < 0 || p.SpendStartMin > 1439 || p.SpendEndMin < 1 || p.SpendEndMin > 1440 || p.SpendStartMin == p.SpendEndMin:
		return "bad_spend_time"
	}
	return ""
}

func dayRU(t *time.Time) string { return t.Format("02.01.2006") }

func spendAlways(mask, start, end int) bool { return mask == 127 && start == 0 && end == 1440 }

// spendText — окно списания словами: «Пн–Пт 00:00–17:00», «Сб, Вс», «каждый день 22:00–06:00»; всегда — "".
func spendText(mask, start, end int) string {
	if spendAlways(mask, start, end) {
		return ""
	}
	if start == 0 && end == 1440 {
		return daysMaskText(mask)
	}
	return daysMaskText(mask) + " " + ruleClock(start) + "–" + ruleClock(end)
}

// sellText — окно продажи словами: «01.12.2026–31.12.2026», «с 01.12.2026», «по 31.12.2026»; без границ — "".
func sellText(p models.TimePackage) string {
	switch {
	case p.SellFrom != nil && p.SellTo != nil:
		return dayRU(p.SellFrom) + "–" + dayRU(p.SellTo)
	case p.SellFrom != nil:
		return "с " + dayRU(p.SellFrom)
	case p.SellTo != nil:
		return "по " + dayRU(p.SellTo)
	}
	return ""
}

// seasonText — условия пакета словами (чистая, тест): «продаётся 01.12.2026–31.12.2026 · 1 на гостя · только новым ·
// тратится Пн–Пт 00:00–17:00»; обычный пакет — "".
func seasonText(p models.TimePackage) string {
	parts := []string{}
	if t := sellText(p); t != "" {
		parts = append(parts, "продаётся "+t)
	}
	if p.PerGuest > 0 {
		parts = append(parts, fmt.Sprintf("%d на гостя", p.PerGuest))
	}
	if p.NewOnly {
		parts = append(parts, "только новым")
	}
	if t := spendText(p.SpendDaysMask, p.SpendStartMin, p.SpendEndMin); t != "" {
		parts = append(parts, "тратится "+t)
	}
	return strings.Join(parts, " · ")
}

// spendOpen — идёт ли окно списания в момент t (чистая, тест): дни — по дню начала окна, через полночь — как смена.
func spendOpen(mask, start, end int, t time.Time) bool {
	if spendAlways(mask, start, end) {
		return true
	}
	ok, _ := shiftActiveAt(start, end, mask, t.In(clubLocation))
	return ok
}

// saleState — продаётся ли пакет в день now по календарю клуба (чистая, тест): "" — да, "not_yet" — продажа ещё не
// началась, "sale_over" — закончилась.
func saleState(p models.TimePackage, now time.Time) string {
	t := now.In(clubLocation)
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, clubLocation)
	if p.SellFrom != nil && d.Before(*p.SellFrom) {
		return "not_yet"
	}
	if p.SellTo != nil && d.After(*p.SellTo) {
		return "sale_over"
	}
	return ""
}

// guestIsNew — новый ли гость на момент now (первое появление не раньше seg_new_days суток назад, З5-и1).
func guestIsNew(tx *gorm.DB, userID uuid.UUID, now time.Time) bool {
	var n int64
	tx.Model(&models.User{}).Where("id = ? AND "+firstSeenSQL+" >= ?", userID, newSince(now)).Count(&n)
	return n > 0
}

// packBought — сколько раз гость купил этот пакет (отменённые не в счёт).
func packBought(tx *gorm.DB, userID, packID uuid.UUID) int64 {
	var n int64
	tx.Model(&models.UserPackage{}).Where("user_id = ? AND package_id = ? AND voided_at IS NULL", userID, packID).Count(&n)
	return n
}

// seasonBlock — можно ли продать пакет этому гостю сейчас: код отказа или "". В issuePackage — под замком строки гостя.
func seasonBlock(tx *gorm.DB, p *models.TimePackage, userID uuid.UUID, now time.Time) string {
	if saleState(*p, now) != "" {
		return "not_on_sale"
	}
	if p.NewOnly && !guestIsNew(tx, userID, now) {
		return "new_only"
	}
	if p.PerGuest > 0 && packBought(tx, userID, p.ID) >= int64(p.PerGuest) {
		return "guest_limit"
	}
	return ""
}

// seasonFail — отказ продажи словами (409); self — покупает сам гость (приложение).
func seasonFail(c *gin.Context, code string, p *models.TimePackage, userID uuid.UUID, self bool) {
	msg := ""
	switch code {
	case "not_on_sale":
		if saleState(*p, time.Now()) == "not_yet" {
			msg = fmt.Sprintf("Пакет «%s» продаётся с %s", p.Name, dayRU(p.SellFrom))
		} else {
			msg = fmt.Sprintf("Продажа пакета «%s» закончилась %s", p.Name, dayRU(p.SellTo))
		}
	case "new_only":
		msg = fmt.Sprintf("Пакет «%s» — только новым гостям: первый визит — не раньше %d дн. назад", p.Name, segNewDays())
	case "guest_limit":
		who := "у гостя уже"
		if self {
			who = "у тебя уже"
		}
		msg = fmt.Sprintf("Пакет «%s» — не больше %d на гостя, %s %d", p.Name, p.PerGuest, who, packBought(db, userID, p.ID))
	}
	c.JSON(http.StatusConflict, gin.H{"code": code, "message": msg})
}

// seasonChanges — правка сезонности словами для журнала: «продажа: всегда → 01.12.2026–31.12.2026», …
func seasonChanges(was, now models.TimePackage) []string {
	or := func(t, empty string) string {
		if t == "" {
			return empty
		}
		return t
	}
	out := []string{}
	if a, b := sellText(was), sellText(now); a != b {
		out = append(out, "продажа: "+or(a, "всегда")+" → "+or(b, "всегда"))
	}
	if was.PerGuest != now.PerGuest {
		lim := func(n int) string {
			if n == 0 {
				return "без ограничения"
			}
			return fmt.Sprint(n)
		}
		out = append(out, "на гостя: "+lim(was.PerGuest)+" → "+lim(now.PerGuest))
	}
	if was.NewOnly != now.NewOnly {
		out = append(out, map[bool]string{true: "только новым", false: "всем гостям"}[now.NewOnly])
	}
	a := spendText(was.SpendDaysMask, was.SpendStartMin, was.SpendEndMin)
	b := spendText(now.SpendDaysMask, now.SpendStartMin, now.SpendEndMin)
	if a != b {
		out = append(out, "списание: "+or(a, "всегда")+" → "+or(b, "всегда"))
	}
	return out
}

// seasonOut — поля сезонности в ответе каталога.
func seasonOut(p *models.TimePackage, out gin.H) {
	day := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.Format("2006-01-02")
	}
	out["sell_from"], out["sell_to"] = day(p.SellFrom), day(p.SellTo)
	out["per_guest"], out["new_only"] = p.PerGuest, p.NewOnly
	out["spend_days_mask"], out["spend_start_min"], out["spend_end_min"] = p.SpendDaysMask, p.SpendStartMin, p.SpendEndMin
	out["season_text"] = seasonText(*p)
	out["on_sale"] = saleState(*p, time.Now()) == ""
}

// catalogFor — пакеты, которые этот гость может купить сейчас: в окне продажи, «только новым» — если он новый,
// с лимитом — если лимит не выбран.
func catalogFor(userID uuid.UUID, packs []models.TimePackage, now time.Time) []models.TimePackage {
	isNew, newKnown := false, false
	bought := map[uuid.UUID]int64{}
	limited := []uuid.UUID{}
	for i := range packs {
		if packs[i].PerGuest > 0 {
			limited = append(limited, packs[i].ID)
		}
	}
	if len(limited) > 0 {
		var rows []struct {
			PackageID uuid.UUID
			N         int64
		}
		db.Model(&models.UserPackage{}).Select("package_id, COUNT(*) AS n").
			Where("user_id = ? AND package_id IN ? AND voided_at IS NULL", userID, limited).Group("package_id").Scan(&rows)
		for _, r := range rows {
			bought[r.PackageID] = r.N
		}
	}
	out := make([]models.TimePackage, 0, len(packs))
	for _, p := range packs {
		if saleState(p, now) != "" {
			continue
		}
		if p.NewOnly {
			if !newKnown {
				isNew, newKnown = guestIsNew(db, userID, now), true
			}
			if !isNew {
				continue
			}
		}
		if p.PerGuest > 0 && bought[p.ID] >= int64(p.PerGuest) {
			continue
		}
		out = append(out, p)
	}
	return out
}
