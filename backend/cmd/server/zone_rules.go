package main

// З24-и1 «Расписание цен — сервер» (OWNER.md, трек З; №53 «в», З-Р28; миграция 100). У зоны — правила цены часа: дни
// недели, окно по стенным часам клуба (через полночь — принадлежит дню начала, как смена; конец 24:00 — до полуночи),
// цена, срок действия по датам дня начала окна. Вне правил зона стоит своей ценой. Правила одной зоны не накладываются
// (409 rule_overlap): в один момент у зоны одна цена.
//
// Цена берётся на посадке (sessions.go — zoneRateAt в момент посадки) и при пересадке в другую зону с её тарифом
// (pause.go) — и фиксируется снимком в сессии, как и было: граница правила посреди игры её не меняет. Пакеты, монеты,
// минуты зоны и подсказка «на сколько хватит» без сессии — по цене зоны, правила их не трогают.
//
// Правила заводит владелец: POST /admin/zones/:id/rules, PATCH и DELETE /admin/zone-rules/:id (в журнале — правило
// словами). В GET /admin/zones у зоны — правила, цена сейчас и следующая смена цены; наружу (GET /clubs, /clubs/:id) и
// на табло (/board/state) — цена сейчас (rate_pln) и следующая (next_at, next_rate_pln — если смена в ближайшие 7 дней).

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxZoneRules       = 20 // правил у зоны
	maxZoneRuleNote    = 64 // символов в заметке правила
	priceLookAheadDays = 7  // «следующая цена» — в ближайшие дни
)

func init() {
	zoneErrors["bad_days"] = "Дни недели — хотя бы один"
	zoneErrors["bad_time"] = "Время — от 00:00 до 24:00, начало — до 23:59"
	zoneErrors["zero_length"] = "Начало и конец совпадают — окно пустое"
	zoneErrors["bad_dates"] = "Срок действия — даты ГГГГ-ММ-ДД, «с» не позже «по»"
	zoneErrors["bad_note"] = fmt.Sprintf("Заметка — до %d символов", maxZoneRuleNote)
	zoneErrors["rule_overlap"] = "Правило накладывается на другое правило этой зоны"
	zoneErrors["too_many_rules"] = fmt.Sprintf("У зоны — до %d правил", maxZoneRules)
	zoneErrors["rule_not_found"] = "Правило не найдено"
	zoneErrors["zone_not_found"] = "Зона не найдена"
}

var weekdayRu = [7]string{"Пн", "Вт", "Ср", "Чт", "Пт", "Сб", "Вс"}

// daysMaskText — дни правила словами (чистая, тест): «каждый день», «Пн–Пт», «Сб, Вс», «Пн–Ср, Пт».
func daysMaskText(mask int) string {
	if mask&127 == 127 {
		return "каждый день"
	}
	parts := []string{}
	for d := 0; d < 7; {
		if mask&(1<<d) == 0 {
			d++
			continue
		}
		e := d
		for e+1 < 7 && mask&(1<<(e+1)) != 0 {
			e++
		}
		switch {
		case e-d >= 2:
			parts = append(parts, weekdayRu[d]+"–"+weekdayRu[e])
		case e == d+1:
			parts = append(parts, weekdayRu[d], weekdayRu[e])
		default:
			parts = append(parts, weekdayRu[d])
		}
		d = e + 1
	}
	return strings.Join(parts, ", ")
}

// ruleClock — минуты суток часами: 1080 → «18:00», 1440 → «24:00» (чистая).
func ruleClock(min int) string { return fmt.Sprintf("%02d:%02d", min/60, min%60) }

// zoneRuleText — правило словами (чистая, тест): «Пн–Пт 18:00–22:00 — 20,00 zł/ч, с 01.12.2026 по 31.12.2026».
func zoneRuleText(r models.ZonePriceRule) string {
	t := fmt.Sprintf("%s %s–%s — %s zł/ч", daysMaskText(r.DaysMask), ruleClock(r.StartMin), ruleClock(r.EndMin), plnRU(r.RatePLN))
	switch {
	case r.ValidFrom != nil && r.ValidTo != nil:
		t += ", с " + r.ValidFrom.Format("02.01.2006") + " по " + r.ValidTo.Format("02.01.2006")
	case r.ValidFrom != nil:
		t += ", с " + r.ValidFrom.Format("02.01.2006")
	case r.ValidTo != nil:
		t += ", по " + r.ValidTo.Format("02.01.2006")
	}
	return t
}

// validateZoneRule — чистая проверка правила (тест): дни 1…127, начало 0…1439, конец 1…1440 и не равен началу, цена —
// как у зоны, срок «с» не позже «по», заметка до maxZoneRuleNote.
func validateZoneRule(r models.ZonePriceRule) string {
	switch {
	case r.DaysMask < 1 || r.DaysMask > 127:
		return "bad_days"
	case r.StartMin < 0 || r.StartMin > 1439 || r.EndMin < 1 || r.EndMin > 1440:
		return "bad_time"
	case r.StartMin == r.EndMin:
		return "zero_length"
	case !validatePLN(r.RatePLN, maxZoneRate):
		return "bad_rate"
	case r.ValidFrom != nil && r.ValidTo != nil && r.ValidTo.Before(*r.ValidFrom):
		return "bad_dates"
	case len([]rune(r.Note)) > maxZoneRuleNote:
		return "bad_note"
	}
	return ""
}

// ruleValidOn — правило действует в окне, начатом в день day (полночь клуба): день недели и срок действия (чистая).
func ruleValidOn(r models.ZonePriceRule, day time.Time) bool {
	if r.DaysMask&(1<<dowMonday(day)) == 0 {
		return false
	}
	return (r.ValidFrom == nil || !day.Before(*r.ValidFrom)) && (r.ValidTo == nil || !day.After(*r.ValidTo))
}

// ruleAt — правило, по которому зона стоит в момент t (чистая, тест): окно по стенным часам клуба содержит t, правило
// действует в день начала окна. Правила не накладываются — подходит одно; если всё же два (данные правили мимо
// сервера) — первое по порядку.
func ruleAt(rules []models.ZonePriceRule, t time.Time) *models.ZonePriceRule {
	t = t.In(clubLocation)
	for i := range rules {
		r := &rules[i]
		if ok, day := shiftActiveAt(r.StartMin, r.EndMin, 127, t); ok && ruleValidOn(*r, day) {
			return r
		}
	}
	return nil
}

// priceAt — цена часа зоны в момент t: правило или цена зоны (чистая, тест).
func priceAt(rules []models.ZonePriceRule, base float64, t time.Time) float64 {
	if r := ruleAt(rules, t); r != nil {
		return r.RatePLN
	}
	return base
}

// nextPriceChange — когда в ближайшие priceLookAheadDays дней после t цена зоны сменится и на какую (чистая, тест):
// кандидаты — начала и концы окон правил по стенным часам, первый, где цена другая, чем в t.
func nextPriceChange(rules []models.ZonePriceRule, base float64, t time.Time) (time.Time, float64, bool) {
	t = t.In(clubLocation)
	now := priceAt(rules, base, t)
	day0 := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, clubLocation)
	cands := []time.Time{}
	for k := -1; k <= priceLookAheadDays+1; k++ {
		d := day0.AddDate(0, 0, k)
		for _, r := range rules {
			if ruleValidOn(r, d) {
				cands = append(cands, wallAt(d, r.StartMin), shiftEndAt(d, r.StartMin, r.EndMin))
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].Before(cands[j]) })
	limit := t.AddDate(0, 0, priceLookAheadDays)
	for _, c := range cands {
		if !c.After(t) {
			continue
		}
		if c.After(limit) {
			break
		}
		if p := priceAt(rules, base, c); p != now {
			return c, p, true
		}
	}
	return time.Time{}, 0, false
}

// rulesOverlap — накладываются ли два правила (чистая, тест): есть ли момент, когда действуют оба. Без сроков — по неделе
// (shiftsOverlap: ночь воскресенья встречает утро понедельника); со сроком — по дням общего срока (окно через полночь
// заходит в следующий день, поэтому срок расширяется на день в обе стороны).
func rulesOverlap(a, b models.ZonePriceRule) bool {
	if a.ValidFrom == nil && a.ValidTo == nil && b.ValidFrom == nil && b.ValidTo == nil {
		return shiftsOverlap(a.StartMin, a.EndMin, a.DaysMask, b.StartMin, b.EndMin, b.DaysMask)
	}
	later := func(x, y *time.Time) *time.Time {
		if x == nil || (y != nil && y.After(*x)) {
			return y
		}
		return x
	}
	earlier := func(x, y *time.Time) *time.Time {
		if x == nil || (y != nil && y.Before(*x)) {
			return y
		}
		return x
	}
	lo, hi := later(a.ValidFrom, b.ValidFrom), earlier(a.ValidTo, b.ValidTo)
	var from, to time.Time
	switch {
	case lo != nil && hi != nil:
		if lo.After(hi.AddDate(0, 0, 1)) { // между сроками — целые сутки: окно больше суток не бывает
			return false
		}
		from, to = lo.AddDate(0, 0, -1), hi.AddDate(0, 0, 1)
	case lo != nil:
		from = lo.AddDate(0, 0, -1)
		to = from.AddDate(0, 0, 16)
	default:
		to = hi.AddDate(0, 0, 1)
		from = to.AddDate(0, 0, -16)
	}
	if to.Sub(from) > 16*24*time.Hour { // длинный общий срок: каждый день недели встретится дважды — хватит 16 дней
		to = from.AddDate(0, 0, 16)
	}
	spans := func(r models.ZonePriceRule) [][2]int {
		out := [][2]int{}
		k := 0
		for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
			if ruleValidOn(r, d) {
				length := r.EndMin - r.StartMin
				if r.EndMin <= r.StartMin {
					length += 1440
				}
				s := k*1440 + r.StartMin
				out = append(out, [2]int{s, s + length})
			}
			k++
		}
		return out
	}
	for _, x := range spans(a) {
		for _, y := range spans(b) {
			if x[0] < y[1] && y[0] < x[1] {
				return true
			}
		}
	}
	return false
}

// zoneRulesOf — правила зон ids по порядку заведения.
func zoneRulesOf(ids []uuid.UUID) map[uuid.UUID][]models.ZonePriceRule {
	out := map[uuid.UUID][]models.ZonePriceRule{}
	if len(ids) == 0 {
		return out
	}
	var rules []models.ZonePriceRule
	db.Where("zone_id IN ?", ids).Order("created_at, id").Find(&rules)
	for _, r := range rules {
		out[r.ZoneID] = append(out[r.ZoneID], r)
	}
	return out
}

// zoneRateAt — цена часа зоны ПК в момент t: правило расписания или цена зоны; ПК без зоны или зона не найдена — nil
// (тогда — клубный тариф, rateForComputer).
func zoneRateAt(pc *models.Computer, t time.Time) *float64 {
	if pc.ZoneID == nil {
		return nil
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", *pc.ZoneID).Error; err != nil {
		return nil
	}
	p := priceAt(zoneRulesOf([]uuid.UUID{z.ID})[z.ID], z.RatePLN, t)
	return &p
}

// zonePriceFields — цена сейчас и следующая для ответа (чистая): rate_pln — сейчас; next_at и next_rate_pln — если цена
// сменится в ближайшие priceLookAheadDays дней.
func zonePriceFields(rules []models.ZonePriceRule, base float64, t time.Time, out gin.H) gin.H {
	out["rate_pln"] = priceAt(rules, base, t)
	if at, p, ok := nextPriceChange(rules, base, t); ok {
		out["next_at"], out["next_rate_pln"] = at, p
	}
	return out
}

// zoneRuleOut — правило в ответе.
func zoneRuleOut(r models.ZonePriceRule) gin.H {
	day := func(t *time.Time) any {
		if t == nil {
			return nil
		}
		return t.Format("2006-01-02")
	}
	return gin.H{"id": r.ID, "zone_id": r.ZoneID, "days_mask": r.DaysMask, "start_min": r.StartMin, "end_min": r.EndMin,
		"start": ruleClock(r.StartMin), "end": ruleClock(r.EndMin), "rate_pln": r.RatePLN, "valid_from": day(r.ValidFrom),
		"valid_to": day(r.ValidTo), "note": r.Note, "text": zoneRuleText(r)}
}

type zoneRuleRequest struct {
	DaysMask  *int     `json:"days_mask"`
	StartMin  *int     `json:"start_min"`
	EndMin    *int     `json:"end_min"`
	RatePLN   *float64 `json:"rate_pln"`
	ValidFrom *string  `json:"valid_from"` // ГГГГ-ММ-ДД; "" — без начала
	ValidTo   *string  `json:"valid_to"`   // ГГГГ-ММ-ДД; "" — бессрочно
	Note      *string  `json:"note"`
}

// ruleDate — дата срока действия из запроса (чистая): "" — без границы (nil, true); ГГГГ-ММ-ДД — полночь клуба.
func ruleDate(s string) (*time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, true
	}
	t, err := time.ParseInLocation("2006-01-02", s, clubLocation)
	if err != nil {
		return nil, false
	}
	return &t, true
}

// apply — поля запроса поверх правила r (не прислано — как было); false — кривая дата.
func (q zoneRuleRequest) apply(r *models.ZonePriceRule) bool {
	if q.DaysMask != nil {
		r.DaysMask = *q.DaysMask
	}
	if q.StartMin != nil {
		r.StartMin = *q.StartMin
	}
	if q.EndMin != nil {
		r.EndMin = *q.EndMin
	}
	if q.RatePLN != nil {
		r.RatePLN = round2(*q.RatePLN)
	}
	if q.Note != nil {
		r.Note = strings.TrimSpace(*q.Note)
	}
	for _, f := range []struct {
		in  *string
		out **time.Time
	}{{q.ValidFrom, &r.ValidFrom}, {q.ValidTo, &r.ValidTo}} {
		if f.in != nil {
			d, ok := ruleDate(*f.in)
			if !ok {
				return false
			}
			*f.out = d
		}
	}
	return true
}

// saveZoneRule — проверить правило и записать (новое или правку) под замком зоны: две правки разом не проскочат мимо
// проверки наложения. Ответ об ошибке пишет сам; nil — записано.
func saveZoneRule(c *gin.Context, r *models.ZonePriceRule, isNew bool) error {
	if code := validateZoneRule(*r); code != "" {
		zoneFail(c, http.StatusBadRequest, code)
		return fmt.Errorf("%s", code)
	}
	var clash *models.ZonePriceRule
	tooMany := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var z models.Zone
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&z, "id = ?", r.ZoneID).Error; err != nil {
			return err
		}
		var others []models.ZonePriceRule
		if err := tx.Where("zone_id = ? AND id <> ?", r.ZoneID, r.ID).Order("created_at, id").Find(&others).Error; err != nil {
			return err
		}
		if isNew && len(others) >= maxZoneRules {
			tooMany = true
			return errRuleRejected
		}
		for i := range others {
			if rulesOverlap(*r, others[i]) {
				clash = &others[i]
				return errRuleRejected
			}
		}
		if isNew {
			return tx.Create(r).Error
		}
		return tx.Save(r).Error
	})
	switch {
	case tooMany:
		zoneFail(c, http.StatusConflict, "too_many_rules")
	case clash != nil:
		c.JSON(http.StatusConflict, gin.H{"code": "rule_overlap", "with": zoneRuleOut(*clash),
			"message": "Накладывается на правило «" + zoneRuleText(*clash) + "» — поправь дни, часы или срок"})
	case err != nil:
		zoneFail(c, http.StatusInternalServerError, "db_error")
	}
	return err
}

var errRuleRejected = fmt.Errorf("rule_rejected")

// zoneByParam — зона по :id в адресе; нет — 404 и false.
func zoneByParam(c *gin.Context) (models.Zone, bool) {
	var z models.Zone
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || db.First(&z, "id = ?", id).Error != nil {
		zoneFail(c, http.StatusNotFound, "zone_not_found")
		return z, false
	}
	return z, true
}

// zoneRuleReply — ответ после записи: правило и цены зоны сейчас.
func zoneRuleReply(c *gin.Context, status int, r models.ZonePriceRule, z models.Zone) {
	rules := zoneRulesOf([]uuid.UUID{z.ID})[z.ID]
	c.JSON(status, gin.H{"rule": zoneRuleOut(r), "zone": zonePriceFields(rules, z.RatePLN, time.Now(),
		gin.H{"id": z.ID, "name": z.Name, "base_rate_pln": z.RatePLN})})
}

// POST /admin/zones/:id/rules — новое правило цены зоны (owner).
func handleZoneRuleCreate(c *gin.Context) {
	z, ok := zoneByParam(c)
	if !ok {
		return
	}
	var req zoneRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.DaysMask == nil || req.StartMin == nil || req.EndMin == nil || req.RatePLN == nil {
		zoneFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	r := models.ZonePriceRule{ZoneID: z.ID}
	if !req.apply(&r) {
		zoneFail(c, http.StatusBadRequest, "bad_dates")
		return
	}
	r.ID = uuid.New()
	if saveZoneRule(c, &r, true) != nil {
		return
	}
	logAdminAction(c, "zone_rule_create", nil, z.Name+": правило "+zoneRuleText(r)+noteSuffix(r.Note))
	zoneRuleReply(c, http.StatusCreated, r, z)
}

// ruleByParam — правило по :id и его зона; нет — 404 и false.
func ruleByParam(c *gin.Context) (models.ZonePriceRule, models.Zone, bool) {
	var r models.ZonePriceRule
	var z models.Zone
	id, err := uuid.Parse(c.Param("id"))
	if err != nil || db.First(&r, "id = ?", id).Error != nil || db.First(&z, "id = ?", r.ZoneID).Error != nil {
		zoneFail(c, http.StatusNotFound, "rule_not_found")
		return r, z, false
	}
	return r, z, true
}

// PATCH /admin/zone-rules/:id — правка правила (owner): не прислано — как было.
func handleZoneRuleUpdate(c *gin.Context) {
	r, z, ok := ruleByParam(c)
	if !ok {
		return
	}
	var req zoneRuleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zoneFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	was := zoneRuleText(r) + noteSuffix(r.Note)
	if !req.apply(&r) {
		zoneFail(c, http.StatusBadRequest, "bad_dates")
		return
	}
	now := zoneRuleText(r) + noteSuffix(r.Note)
	if now == was {
		zoneRuleReply(c, http.StatusOK, r, z)
		return
	}
	if saveZoneRule(c, &r, false) != nil {
		return
	}
	logAdminAction(c, "zone_rule_update", nil, z.Name+": правило "+was+" → "+now)
	zoneRuleReply(c, http.StatusOK, r, z)
}

// DELETE /admin/zone-rules/:id — убрать правило (owner): зона снова стоит своей ценой в этом окне.
func handleZoneRuleDelete(c *gin.Context) {
	r, z, ok := ruleByParam(c)
	if !ok {
		return
	}
	if err := db.Delete(&models.ZonePriceRule{}, "id = ?", r.ID).Error; err != nil {
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "zone_rule_delete", nil, z.Name+": правило убрано — "+zoneRuleText(r)+noteSuffix(r.Note))
	c.JSON(http.StatusOK, gin.H{"deleted": r.ID})
}

// zoneIDs — id зон списком.
func zoneIDs(zones []models.Zone) []uuid.UUID {
	ids := make([]uuid.UUID, len(zones))
	for i := range zones {
		ids[i] = zones[i].ID
	}
	return ids
}

// publicZonePrices — зоны наружу (GET /clubs, /clubs/:id; К2-и5): имя и цена часа сейчас, следующая — если сменится в
// ближайшие priceLookAheadDays дней.
func publicZonePrices(zones []models.Zone) map[uuid.UUID]gin.H {
	rules, now := zoneRulesOf(zoneIDs(zones)), time.Now()
	out := make(map[uuid.UUID]gin.H, len(zones))
	for _, z := range zones {
		out[z.ID] = zonePriceFields(rules[z.ID], z.RatePLN, now, gin.H{"name": z.Name})
	}
	return out
}

// zoneAdminPrices — к зоне в GET /admin/zones: правила, цена сейчас (rate_now_pln; rate_pln зоны — её цена вне правил)
// и следующая смена цены.
func zoneAdminPrices(rules []models.ZonePriceRule, base float64) gin.H {
	now := time.Now()
	out := gin.H{"rate_now_pln": priceAt(rules, base, now)}
	if at, p, ok := nextPriceChange(rules, base, now); ok {
		out["next_at"], out["next_rate_pln"] = at, p
	}
	list := make([]gin.H, 0, len(rules))
	for _, r := range rules {
		list = append(list, zoneRuleOut(r))
	}
	out["rules"] = list
	return out
}
