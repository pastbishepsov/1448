package main

// З24-и4 «Массовые правки цен» (OWNER.md, трек З; №53 «в»). Владелец меняет цены разом: зоны (цена часа), товары (цена
// позиции; фильтр — категория) или пакеты (цена пакета; фильтр — зона): «+10 %», «−5 %» или «+2 zł», с округлением до
// 0,50 или до целых злотых. Сначала — просмотр «было → станет» (ничего не пишет), потом — запись одной транзакцией и
// одной строкой журнала. Запись сверяет цены с просмотром (expect): что-то правили между просмотром и записью (другая
// вкладка, карточка позиции) — 409 prices_moved со списком, и не пишется ничего.
//
// Цена, которая вышла бы за границы своей сущности (от 0,01 до потолка зоны, позиции или пакета), — 400 out_of_range
// со списком: правку целиком не пускаем, а не режем молча. Расписание цен зон (З24-и1) правка не трогает: правило —
// отдельная цена, которую владелец поставил сам. Начатые сессии и проданные пакеты — по своим снимкам, как и при
// правке по одной.

import (
	"fmt"
	"math"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	minBulkPct   = -90.0  // −90 %: ниже — почти наверняка опечатка (обнулить цену так нельзя)
	maxBulkPct   = 300.0  // +300 % — вчетверо
	maxBulkAdd   = 1000.0 // ±1000 zł
	bulkLogItems = 10     // в строке журнала — первые 10 «было → стало», дальше «и ещё N»
)

var bulkErrors = map[string]string{
	"bad_kind":         "Что правим — зоны, товары или пакеты",
	"bad_op":           "Изменение — проценты или злотые (одно из двух) или хотя бы округление",
	"bad_pct":          fmt.Sprintf("Проценты — от %.0f до +%.0f, не больше двух знаков после запятой", minBulkPct, maxBulkPct),
	"bad_add":          fmt.Sprintf("Злотые — от −%.0f до +%.0f, не больше двух знаков после запятой", maxBulkAdd, maxBulkAdd),
	"bad_round":        "Округление — без него, до 0,50 или до целых",
	"bad_ids":          "Список позиций — идентификаторы",
	"nothing_selected": "Под выбор не попало ничего",
	"out_of_range":     "Цена вышла бы за границы",
	"need_preview":     "Сначала посмотри, что станет: запись сверяет цены с просмотром",
	"prices_moved":     "Цены изменились, пока ты смотрел, — посмотри ещё раз",
	"zone_not_found":   "Зона не найдена",
}

func bulkFail(c *gin.Context, status int, code string, extra gin.H) {
	out := gin.H{"code": code, "message": bulkErrors[code]}
	for k, v := range extra {
		out[k] = v
	}
	c.JSON(status, out)
}

type bulkRequest struct {
	Kind     string             `json:"kind"`     // zones | goods | packages
	IDs      []string           `json:"ids"`      // пусто — всё, что под фильтром
	Category *string            `json:"category"` // товары: категория ("" — без категории); не прислано — все
	ZoneID   string             `json:"zone_id"`  // пакеты: зона; пусто — все
	All      bool               `json:"all"`      // товары и пакеты: и выключенные (по умолчанию — только включённые)
	Pct      float64            `json:"pct"`      // ±% …
	AddPLN   float64            `json:"add_pln"`  // … или ±zł — одно из двух; ни того ни другого — только округление
	Round    float64            `json:"round"`    // 0 — до гроша, 0.5 — до 0,50, 1 — до целых
	Apply    bool               `json:"apply"`    // false — просмотр, true — запись
	Expect   map[string]float64 `json:"expect"`   // запись: цены из просмотра, id → было
}

// bulkRow — строка правки: что было и что станет.
type bulkRow struct {
	ID    uuid.UUID
	Name  string
	Group string // категория товара, зона пакета
	Was   float64
	Will  float64
	Max   float64
}

// validateBulk — проверка запроса (чистая, тест): код ошибки или "".
func validateBulk(q bulkRequest) string {
	switch {
	case q.Kind != "zones" && q.Kind != "goods" && q.Kind != "packages":
		return "bad_kind"
	case q.Pct != 0 && q.AddPLN != 0, q.Pct == 0 && q.AddPLN == 0 && q.Round == 0:
		return "bad_op"
	case q.Pct != 0 && (q.Pct < minBulkPct || q.Pct > maxBulkPct || !twoDecimals(q.Pct)):
		return "bad_pct"
	case q.AddPLN != 0 && (math.Abs(q.AddPLN) > maxBulkAdd || !twoDecimals(q.AddPLN)):
		return "bad_add"
	case q.Round != 0 && q.Round != 0.5 && q.Round != 1:
		return "bad_round"
	}
	return ""
}

func twoDecimals(v float64) bool {
	c := v * 100
	return math.Abs(c-math.Round(c)) < 1e-6
}

// bulkNewPrice — новая цена (чистая, тест): в грошах, «+pct %» или «+add zł», затем округление до шага (до гроша,
// до 0,50, до целых; половина — вверх).
func bulkNewPrice(was, pct, add, round float64) float64 {
	g := math.Round(was * 100)
	x := g + math.Round(add*100)
	if pct != 0 {
		x = g * (100 + pct) / 100
	}
	step := 1.0
	if round > 0 {
		step = round * 100
	}
	return math.Floor(x/step+0.5+1e-9) * step / 100
}

// bulkOpText — изменение словами: «+10 %», «−7,5 %», «−2,50 zł», «+5 %, до 0,50», «+3 %, до целых», «округление до 0,50».
func bulkOpText(pct, add, round float64) string {
	sign := func(v float64) string {
		if v < 0 {
			return "−"
		}
		return "+"
	}
	parts := []string{}
	if pct != 0 {
		parts = append(parts, sign(pct)+strings.Replace(strconv.FormatFloat(math.Abs(pct), 'f', -1, 64), ".", ",", 1)+" %")
	} else if add != 0 {
		parts = append(parts, sign(add)+plnRU(math.Abs(add))+" zł")
	}
	switch round {
	case 0.5:
		parts = append(parts, "до 0,50")
	case 1:
		parts = append(parts, "до целых")
	}
	if pct == 0 && add == 0 {
		return "округление " + strings.Join(parts, ", ")
	}
	return strings.Join(parts, ", ")
}

// bulkLogText — строка журнала: «товары «Напитки»: +10 %, до 0,50 — 3 из 4: Cola 5,00 → 5,50 zł; …; и ещё N».
func bulkLogText(scope, op string, rows []bulkRow) string {
	changed := []string{}
	for _, r := range rows {
		if r.Will != r.Was {
			changed = append(changed, fmt.Sprintf("%s %s → %s zł", r.Name, plnRU(r.Was), plnRU(r.Will)))
		}
	}
	t := fmt.Sprintf("%s: %s — %d из %d", scope, op, len(changed), len(rows))
	if len(changed) > bulkLogItems {
		t += ": " + strings.Join(changed[:bulkLogItems], "; ") + fmt.Sprintf("; и ещё %d", len(changed)-bulkLogItems)
	} else if len(changed) > 0 {
		t += ": " + strings.Join(changed, "; ")
	}
	return t
}

// bulkRows — строки под выбор (в транзакции записи — под замком FOR UPDATE) и слова «что правим» для журнала.
func bulkRows(tx *gorm.DB, q bulkRequest, ids []uuid.UUID, lock bool) ([]bulkRow, string, string) {
	if lock {
		tx = tx.Clauses(clause.Locking{Strength: "UPDATE"})
	}
	out := []bulkRow{}
	switch q.Kind {
	case "zones":
		var zs []models.Zone
		t := tx.Order("sort, name")
		if len(ids) > 0 {
			t = t.Where("id IN ?", ids)
		}
		t.Find(&zs)
		for _, z := range zs {
			out = append(out, bulkRow{ID: z.ID, Name: z.Name, Was: z.RatePLN, Max: maxZoneRate})
		}
		return out, "зоны", ""
	case "goods":
		var gs []models.Good
		t := tx.Order("category, sort, name")
		if len(ids) > 0 {
			t = t.Where("id IN ?", ids)
		}
		scope := "товары"
		if q.Category != nil {
			t = t.Where("category = ?", strings.TrimSpace(*q.Category))
			scope = fmt.Sprintf("товары «%s»", strings.TrimSpace(*q.Category))
			if strings.TrimSpace(*q.Category) == "" {
				scope = "товары без категории"
			}
		}
		if !q.All {
			t = t.Where("active")
		}
		t.Find(&gs)
		for _, g := range gs {
			out = append(out, bulkRow{ID: g.ID, Name: g.Name, Group: g.Category, Was: g.PricePLN, Max: maxGoodPrice})
		}
		return out, scope, ""
	}
	// пакеты
	var zone *models.Zone
	if q.ZoneID != "" {
		zid, err := uuid.Parse(q.ZoneID)
		if err != nil {
			return nil, "", "zone_not_found"
		}
		if zone = zoneByID(&zid); zone == nil {
			return nil, "", "zone_not_found"
		}
	}
	var ps []models.TimePackage
	t := tx.Order("sort, name")
	if len(ids) > 0 {
		t = t.Where("id IN ?", ids)
	}
	scope := "пакеты"
	if zone != nil {
		t = t.Where("zone_id = ?", zone.ID)
		scope = fmt.Sprintf("пакеты зоны «%s»", zone.Name)
	}
	if !q.All {
		t = t.Where("active")
	}
	t.Find(&ps)
	zoneNames := map[uuid.UUID]string{}
	for _, p := range ps {
		if _, ok := zoneNames[p.ZoneID]; !ok {
			zoneNames[p.ZoneID] = ""
			if z := zoneByID(&p.ZoneID); z != nil {
				zoneNames[p.ZoneID] = z.Name
			}
		}
		out = append(out, bulkRow{ID: p.ID, Name: p.Name, Group: zoneNames[p.ZoneID], Was: p.PricePLN, Max: float64(maxPackagePrice)})
	}
	return out, scope, ""
}

func bulkRowsOut(rows []bulkRow) ([]gin.H, int) {
	out := make([]gin.H, 0, len(rows))
	changed := 0
	for _, r := range rows {
		if r.Will != r.Was {
			changed++
		}
		out = append(out, gin.H{"id": r.ID, "name": r.Name, "group": r.Group, "was": r.Was, "will": r.Will, "changed": r.Will != r.Was})
	}
	return out, changed
}

// bulkOutOfRange — строки, у которых новая цена вышла бы за границы своей сущности.
func bulkOutOfRange(rows []bulkRow) []gin.H {
	bad := []gin.H{}
	for _, r := range rows {
		if r.Will != r.Was && !validatePLN(r.Will, r.Max) {
			bad = append(bad, gin.H{"id": r.ID, "name": r.Name, "was": r.Was, "will": r.Will, "max": r.Max})
		}
	}
	return bad
}

// POST /admin/prices/bulk (владелец): просмотр (apply: false) или запись (apply: true, expect — цены из просмотра).
func handlePricesBulk(c *gin.Context) {
	var q bulkRequest
	if err := c.ShouldBindJSON(&q); err != nil {
		bulkFail(c, http.StatusBadRequest, "bad_kind", nil)
		return
	}
	if code := validateBulk(q); code != "" {
		bulkFail(c, http.StatusBadRequest, code, nil)
		return
	}
	ids := make([]uuid.UUID, 0, len(q.IDs))
	for _, s := range q.IDs {
		id, err := uuid.Parse(s)
		if err != nil {
			bulkFail(c, http.StatusBadRequest, "bad_ids", nil)
			return
		}
		ids = append(ids, id)
	}
	if q.Apply && len(q.Expect) == 0 {
		bulkFail(c, http.StatusBadRequest, "need_preview", nil)
		return
	}
	op := bulkOpText(q.Pct, q.AddPLN, q.Round)
	compute := func(rows []bulkRow) {
		for i := range rows {
			rows[i].Will = bulkNewPrice(rows[i].Was, q.Pct, q.AddPLN, q.Round)
		}
	}

	if !q.Apply {
		rows, scope, code := bulkRows(db, q, ids, false)
		if code != "" {
			bulkFail(c, http.StatusNotFound, code, nil)
			return
		}
		if len(rows) == 0 {
			bulkFail(c, http.StatusBadRequest, "nothing_selected", nil)
			return
		}
		compute(rows)
		out, changed := bulkRowsOut(rows)
		c.JSON(http.StatusOK, gin.H{"applied": false, "kind": q.Kind, "scope": scope, "op": op, "rows": out,
			"changed": changed, "out_of_range": bulkOutOfRange(rows)})
		return
	}

	var rows []bulkRow
	var scope, text string
	var fail func()
	err := db.Transaction(func(tx *gorm.DB) error {
		var code string
		rows, scope, code = bulkRows(tx, q, ids, true)
		if code != "" {
			fail = func() { bulkFail(c, http.StatusNotFound, code, nil) }
			return errBulkRejected
		}
		if len(rows) == 0 {
			fail = func() { bulkFail(c, http.StatusBadRequest, "nothing_selected", nil) }
			return errBulkRejected
		}
		moved := []gin.H{}
		for _, r := range rows {
			was, ok := q.Expect[r.ID.String()]
			if !ok || math.Abs(was-r.Was) > 0.004 {
				m := gin.H{"id": r.ID, "name": r.Name, "now": r.Was}
				if ok {
					m["was"] = was
				}
				moved = append(moved, m)
			}
		}
		if len(moved) > 0 {
			fail = func() { bulkFail(c, http.StatusConflict, "prices_moved", gin.H{"rows": moved}) }
			return errBulkRejected
		}
		compute(rows)
		if bad := bulkOutOfRange(rows); len(bad) > 0 {
			fail = func() { bulkFail(c, http.StatusBadRequest, "out_of_range", gin.H{"rows": bad}) }
			return errBulkRejected
		}
		table := map[string]string{"zones": "zones", "goods": "goods", "packages": "time_packages"}[q.Kind]
		col := map[string]string{"zones": "rate_pln", "goods": "price_pln", "packages": "price_pln"}[q.Kind]
		for _, r := range rows {
			if r.Will == r.Was {
				continue
			}
			set := map[string]any{col: r.Will}
			if q.Kind != "goods" { // у позиции ценника нет updated_at
				set["updated_at"] = gorm.Expr("now()")
			}
			if err := tx.Table(table).Where("id = ?", r.ID).Updates(set).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if fail != nil {
		fail()
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "db_error", "message": "Не удалось записать цены — попробуй ещё раз"})
		return
	}
	out, changed := bulkRowsOut(rows)
	if changed > 0 {
		text = bulkLogText(scope, op, rows)
		logAdminAction(c, "prices_bulk", nil, text)
	}
	c.JSON(http.StatusOK, gin.H{"applied": true, "kind": q.Kind, "scope": scope, "op": op, "rows": out, "changed": changed, "text": text})
}

var errBulkRejected = fmt.Errorf("bulk rejected")
