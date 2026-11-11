package main

// Зоны зала со своей ценой часа (спринт В4, этап 5; миграция 027).
// До этого зона была подписью на компьютере, а тариф — один на весь клуб и
// нигде не редактировался: VIP стоил столько же, сколько обычное место.
//
// Источник правды — zone_id у компьютера; текстовая колонка zone остаётся
// кэшем имени (по ней уже строятся карта зала и разрезы отчётов) и пишется
// ТОЛЬКО отсюда, одним путём. Поэтому переименование зоны обновляет кэш у
// всех её машин одной операцией.

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const maxZoneRate = 500.0 // zł/час: потолок от опечатки в три нуля

// validateZone — чистая проверка зоны (тест в zones_test.go).
func validateZone(name string, rate float64) (bool, string) {
	n := strings.TrimSpace(name)
	if n == "" || len([]rune(n)) > 32 {
		return false, "bad_name"
	}
	if !validatePLN(rate, maxZoneRate) { // З6-и3: 0,004 падало на CHECK базы, 1,005 молча становилось 1,01
		return false, "bad_rate"
	}
	return true, ""
}

var zoneErrors = map[string]string{
	"bad_name":       "Название зоны — 1–32 символа",
	"bad_rate":       fmt.Sprintf("Цена часа — от 0.01 до %.0f zł, не больше двух знаков после запятой", maxZoneRate),
	"bad_sort":       sortMessage,
	"already_exists": "Зона с таким названием уже есть",
	"has_computers":  "В зоне есть компьютеры — сначала перенеси их в другую",
	// Е2-и1: пакеты привязаны к зоне (Р10), и у гостей на руках купленные
	// минуты именно этой зоны. Молча снести её нельзя.
	// З18-и1: проданный пакет не удаляется (has_sales), а продажи держат зону навсегда — отдельным кодом ниже;
	// сюда доходят только пакеты каталога, которые ни разу не продавали, — их можно удалить.
	"has_packages": "В каталоге есть пакеты этой зоны — сначала удали их в «Пакетах времени»",
	"bad_vat":      vatBadMessage, // З13-и1
	// З18-и1 (№8, З-Р7; из №40 — «только переименовать»)
	"package_history":   "По зоне продавали пакеты — в истории продаж она нужна: удалить нельзя, можно только переименовать",
	"bad_target":        "Выбери одно: зону, куда перенести минуты, или «сжечь»",
	"to_zone_not_found": "Зона, куда переносим минуты, не найдена",
	"same_zone":         "Минуты уже в этой зоне — выбери другую",
	"no_minutes":        "Живых минут в зоне нет — их уже перенесли, сожгли или отыграли",
}

func zoneFail(c *gin.Context, status int, code string) {
	msg := zoneErrors[code]
	if msg == "" {
		msg = code
	}
	c.JSON(status, gin.H{"code": code, "message": msg})
}

// rateForComputer — цена часа для ПК: тариф его зоны, а если зоны нет —
// клубный тариф. Чистая функция (тест): fallback важен, иначе удаление зоны
// обнулило бы цену сессии.
func rateForComputer(zoneRate *float64, clubRate float64) float64 {
	if zoneRate != nil && *zoneRate > 0 {
		return *zoneRate
	}
	return clubRate
}

// Цена зоны ПК в момент посадки — zoneRateAt (zone_rules.go, З24-и1): по расписанию цен зоны, вне правил — её цена.

// zoneSnapshotOf — зона ПК снимком в сессию (З16-и1): id и имя; ПК без зоны — nil и пусто. Имя — из зоны (кэш имени у
// ПК — запасной): в сессии оно остаётся и после переименования или удаления зоны.
func zoneSnapshotOf(pc *models.Computer) (*uuid.UUID, string) {
	if pc.ZoneID == nil {
		return nil, ""
	}
	id, name := *pc.ZoneID, pc.Zone
	var z models.Zone
	if db.Select("name").First(&z, "id = ?", id).Error == nil {
		name = z.Name
	}
	return &id, name
}

// sameZoneID — одна ли зона (оба без зоны — тоже одна): пересадка и шторка ПК (З16-и1а).
func sameZoneID(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// sessionZoneID — зона сессии для пакетов, зонных минут, прогноза и выдачи минут «сюда» (З16-и2): её снимок (З16-и1),
// а не нынешняя зона машины — ПК перевели в другую зону под гостем, а пакет его зоны тратится дальше; пересадка с
// тарифом новой зоны снимок меняет, с прежним — нет (З16-и1а). Сессия без снимка (без ставок XP: 091 таких не
// оставила, но код их не исключает) — зона её ПК, как до З16-и2.
func sessionZoneID(s *models.Session) *uuid.UUID {
	return zoneOfSession(s, func() *uuid.UUID { return zoneOfComputer(s.ComputerID) })
}

// zoneOfSession — чистая часть sessionZoneID (юнит): зону ПК спрашивает, только если снимка нет.
func zoneOfSession(s *models.Session, pcZone func() *uuid.UUID) *uuid.UUID {
	if s.XPPerMin != nil {
		return s.ZoneID
	}
	return pcZone()
}

// zoneLabel — зона словами для журнала и ответов: пустое имя — «без зоны» (клубный тариф).
func zoneLabel(name string) string {
	if name == "" {
		return "без зоны"
	}
	return name
}

// GET /admin/zones — зоны с ценами и числом машин (staff: цену видно у стойки). З18-и1: и что держит зону — живые
// минуты гостей (и у скольких), пакеты в каталоге, продажи пакетов, `delete_block` — почему её не удалить ("" — можно).
func handleAdminZones(c *gin.Context) {
	var zones []models.Zone
	db.Order("sort, name").Find(&zones)
	holds := zoneHolds()
	var loose int64
	db.Model(&models.Computer{}).Where("zone_id IS NULL").Count(&loose)
	rules := zoneRulesOf(zoneIDs(zones)) // З24-и1: правила цены, цена сейчас и следующая

	out := make([]gin.H, 0, len(zones))
	for i := range zones {
		z := zones[i]
		h := holds[z.ID.String()]
		out = append(out, gin.H{"id": z.ID, "name": z.Name, "rate_pln": z.RatePLN,
			"sort": z.Sort, "computers": h.Computers,
			"coins_per_hour": coinsPerHourFor(z.RatePLN), // В4-2: конвертер считает сам
			"vat_pct":        z.VatPct,                   // З13-и1
			"live_minutes":   h.Minutes, "live_guests": h.Guests, "packages": h.Packages, "package_sales": h.Sales,
			"delete_block": zoneDeleteBlock(h)})
		for k, v := range zoneAdminPrices(rules[z.ID], z.RatePLN) {
			out[len(out)-1][k] = v
		}
	}
	club, _ := defaultClub()
	base := 0.0
	if club != nil {
		base = club.BaseRatePLN
	}
	c.JSON(http.StatusOK, gin.H{"count": len(out), "zones": out,
		"club_rate_pln": base, "without_zone": loose})
}

type zoneRequest struct {
	Name    string   `json:"name"`
	RatePLN *float64 `json:"rate_pln"`
	Sort    *int     `json:"sort"`
	VatPct  *int     `json:"vat_pct"` // З13-и1: ставка VAT времени за ПК; не прислана — 23 у новой, прежняя у старой
}

// POST /admin/zones — новая зона (owner).
func handleAdminZoneCreate(c *gin.Context) {
	var req zoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zoneFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	rate := 0.0
	if req.RatePLN != nil {
		rate = *req.RatePLN
	}
	if ok, code := validateZone(req.Name, rate); !ok {
		zoneFail(c, http.StatusBadRequest, code)
		return
	}
	if !validSort(req.Sort) {
		zoneFail(c, http.StatusBadRequest, "bad_sort")
		return
	}
	rate = round2(rate) // хвост float (0,1 + 0,2) — в ответе то же, что в базе
	vat, vok := vatFromRequest(req.VatPct, models.VatDefault)
	if !vok {
		zoneFail(c, http.StatusBadRequest, "bad_vat")
		return
	}
	club, ok := defaultClub()
	if !ok {
		zoneFail(c, http.StatusConflict, "no_club")
		return
	}
	z := models.Zone{ClubID: club.ID, Name: strings.TrimSpace(req.Name), RatePLN: rate, VatPct: vat}
	if req.Sort != nil {
		z.Sort = *req.Sort
	}
	if err := db.Create(&z).Error; err != nil {
		if isDuplicate(err) {
			zoneFail(c, http.StatusConflict, "already_exists")
			return
		}
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	logAdminAction(c, "zone_create", nil, fmt.Sprintf("%s · %.2f zł/ч · VAT %s", z.Name, z.RatePLN, models.VatLabel(z.VatPct)))
	c.JSON(http.StatusCreated, gin.H{"zone": gin.H{"id": z.ID, "name": z.Name, "rate_pln": z.RatePLN, "sort": z.Sort, "vat_pct": z.VatPct}})
}

// PATCH /admin/zones/:id — имя, цена, порядок (owner).
// Переименование тянет за собой кэш имени у всех машин зоны.
func handleAdminZoneUpdate(c *gin.Context) {
	var z models.Zone
	if err := db.First(&z, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
		return
	}
	var req zoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		zoneFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	name, rate := z.Name, z.RatePLN
	if strings.TrimSpace(req.Name) != "" {
		name = strings.TrimSpace(req.Name)
	}
	if req.RatePLN != nil {
		rate = *req.RatePLN
	}
	if ok, code := validateZone(name, rate); !ok {
		zoneFail(c, http.StatusBadRequest, code)
		return
	}
	if !validSort(req.Sort) {
		zoneFail(c, http.StatusBadRequest, "bad_sort")
		return
	}
	rate = round2(rate)
	vat, vok := vatFromRequest(req.VatPct, z.VatPct)
	if !vok {
		zoneFail(c, http.StatusBadRequest, "bad_vat")
		return
	}
	changes := []string{}
	if name != z.Name {
		changes = append(changes, z.Name+" → "+name)
	}
	if rate != z.RatePLN {
		changes = append(changes, fmt.Sprintf("цена %.2f → %.2f zł/ч", z.RatePLN, rate))
	}
	if vat != z.VatPct {
		changes = append(changes, "VAT "+models.VatLabel(z.VatPct)+" → "+models.VatLabel(vat))
	}
	updates := map[string]any{"name": name, "rate_pln": rate, "vat_pct": vat}
	if req.Sort != nil {
		updates["sort"] = *req.Sort
	}
	if err := db.Model(&models.Zone{}).Where("id = ?", z.ID).Updates(updates).Error; err != nil {
		if isDuplicate(err) {
			zoneFail(c, http.StatusConflict, "already_exists")
			return
		}
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	renamed := ""
	if name != z.Name { // кэш имени у машин зоны
		db.Model(&models.Computer{}).Where("zone_id = ?", z.ID).Update("zone", name)
		// З18-и1 (из №40): и снимок имени у живых остатков — минут за монеты и компенсаций и пакетов, иначе гость видит
		// «60 мин Tmp» у зоны, которой с таким именем уже нет. Отыгранное, отменённое и просроченное — история: как продали.
		g := db.Model(&models.MinuteGrant{}).Where("zone_id = ? AND minutes_left > 0", z.ID).Update("zone_name", name).RowsAffected
		p := db.Model(&models.UserPackage{}).
			Where("zone_id = ? AND minutes_left > 0 AND voided_at IS NULL AND (expires_at IS NULL OR expires_at > now())", z.ID).
			Update("zone_name", name).RowsAffected
		if g+p > 0 {
			renamed = fmt.Sprintf(" · имя у живых остатков: выдач минут — %d, пакетов — %d", g, p)
		}
	}
	if len(changes) > 0 {
		logAdminAction(c, "zone_update", nil, strings.Join(changes, ", ")+renamed)
	}
	c.JSON(http.StatusOK, gin.H{"zone": gin.H{"id": z.ID, "name": name, "rate_pln": rate, "vat_pct": vat}})
}

// DELETE /admin/zones/:id — удалить зону, которую ничто не держит (owner). Е2-и1: пакеты база и так защищает (FK
// RESTRICT), но владелец увидел бы голое «db_error» — говорим сами, что держит зону. З18-и1: и живые минуты гостей
// (409 has_minutes — мастер в шторке зоны), и продажи пакетов (409 package_history — только переименовать).
func handleAdminZoneDelete(c *gin.Context) {
	var z models.Zone
	if err := db.First(&z, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
		return
	}
	if zoneHeld(c, zoneHolds()[z.ID.String()]) {
		return
	}
	// удаление — условием: минуты, выданные в зону между проверкой и удалением, удержат её, а не станут «годны везде»
	res := db.Exec("DELETE FROM zones WHERE id = ? AND NOT EXISTS "+
		"(SELECT 1 FROM minute_grants WHERE zone_id = ? AND minutes_left > 0)", z.ID, z.ID)
	if res.Error != nil {
		zoneFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if res.RowsAffected == 0 {
		if !zoneHeld(c, zoneHolds()[z.ID.String()]) { // держать нечем — значит, зону уже удалили параллельно
			c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
		}
		return
	}
	logAdminAction(c, "zone_delete", nil, z.Name)
	c.JSON(http.StatusOK, gin.H{"deleted": z.ID})
}

// ── З18-и1 (OWNER.md, №8, З-Р7): зона с живыми минутами ──────────────────────────────────────────────────────────────
//
// Минуты за монеты и компенсации (minute_grants, Ж2) годны только в своей зоне, а FK у них — ON DELETE SET NULL:
// удаление зоны молча делало их «годными везде», и 30 минут по 10 zł/ч отыгрывались на VIP (ревью 23.09, №8,
// сценарий L). Теперь удаление при живых минутах — 409 has_minutes с числом минут и гостей, а мастер в шторке зоны
// (POST /admin/zones/:id/move-minutes) переносит их в другую зону — зона и имя у выдачи новые, минута в минуту, оценка
// в злотых — по тарифу выдачи, как была, — или сжигает (остаток — 0); каждому гостю — уведомление, в журнал — строка
// на гостя. Продажи пакетов держат зону навсегда (user_packages, FK RESTRICT): 409 package_history — только
// переименовать; раньше говорили «выключи или удали пакеты», а проданный пакет не удаляется. З18-и1а: сжигание
// пишет у выдачи момент и сколько сожгли (burned_at, burned_minutes, миграция 092) — для «Сгорело за период».

// zoneHold — что держит зону: ПК, продажи пакетов, пакеты в каталоге, живые минуты и у скольких гостей.
type zoneHold struct {
	Computers, Sales, Packages, Minutes, Guests int64
}

// zoneDeleteBlock — чистая (юнит): почему зону сейчас нельзя удалить; "" — можно. Сначала то, что мастер не решает
// (ПК переносят в шторке ПК, продажи держат зону навсегда, каталог правят в «Пакетах»), последними — минуты.
func zoneDeleteBlock(h zoneHold) string {
	switch {
	case h.Computers > 0:
		return "has_computers"
	case h.Sales > 0:
		return "package_history"
	case h.Packages > 0:
		return "has_packages"
	case h.Minutes > 0:
		return "has_minutes"
	}
	return ""
}

// zoneHolds — zoneHold всех зон (ключ — id зоны): четыре сгруппированных запроса на весь список.
func zoneHolds() map[string]zoneHold {
	out := map[string]zoneHold{}
	var rows []struct {
		ZoneID string
		N, G   int64
	}
	add := func(set func(h *zoneHold, n, g int64)) {
		for _, r := range rows {
			h := out[r.ZoneID]
			set(&h, r.N, r.G)
			out[r.ZoneID] = h
		}
		rows = nil
	}
	db.Model(&models.Computer{}).Select("zone_id, COUNT(*) AS n").Where("zone_id IS NOT NULL").Group("zone_id").Scan(&rows)
	add(func(h *zoneHold, n, _ int64) { h.Computers = n })
	db.Model(&models.UserPackage{}).Select("zone_id, COUNT(*) AS n").Group("zone_id").Scan(&rows)
	add(func(h *zoneHold, n, _ int64) { h.Sales = n })
	db.Model(&models.TimePackage{}).Select("zone_id, COUNT(*) AS n").Group("zone_id").Scan(&rows)
	add(func(h *zoneHold, n, _ int64) { h.Packages = n })
	db.Model(&models.MinuteGrant{}).Select("zone_id, COALESCE(SUM(minutes_left),0) AS n, COUNT(DISTINCT user_id) AS g").
		Where("zone_id IS NOT NULL AND minutes_left > 0").Group("zone_id").Scan(&rows)
	add(func(h *zoneHold, n, g int64) { h.Minutes, h.Guests = n, g })
	return out
}

// zoneHeld — 409 тем, что держит зону (true), или ничего (false — удалять можно).
func zoneHeld(c *gin.Context, h zoneHold) bool {
	switch code := zoneDeleteBlock(h); code {
	case "":
		return false
	case "package_history":
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": zoneErrors[code], "sales": h.Sales})
	case "has_minutes":
		c.JSON(http.StatusConflict, gin.H{"code": code, "message": zoneMinutesText(h), "minutes": h.Minutes, "guests": h.Guests})
	default:
		zoneFail(c, http.StatusConflict, code)
	}
	return true
}

// guestsGen — «у 1 гостя», «у 2 гостей», «у 21 гостя»: родительный падеж после «у».
func guestsGen(n int64) string {
	if n%10 == 1 && n%100 != 11 {
		return fmt.Sprintf("%d гостя", n)
	}
	return fmt.Sprintf("%d гостей", n)
}

// zoneMinutesText — текст 409 has_minutes (юнит): сколько минут, у скольких гостей и что сделать.
func zoneMinutesText(h zoneHold) string {
	return fmt.Sprintf("У %s %d мин этой зоны — перенеси их в другую зону или сожги (шторка зоны): без зоны они стали бы "+
		"годны везде", guestsGen(h.Guests), h.Minutes)
}

type zoneMinutesRequest struct {
	ToZoneID string `json:"to_zone_id"`
	Burn     bool   `json:"burn"`
}

// guestMinutes — живые минуты зоны у одного гостя: сколько и во сколько злотых по тарифу выдачи.
type guestMinutes struct {
	UserID  uuid.UUID
	Minutes int
	PLN     float64
}

// minutesByGuest — чистая (юнит): выдачи по гостям в порядке первой выдачи; злотые — минуты × тариф выдачи / 60.
func minutesByGuest(grants []models.MinuteGrant) []guestMinutes {
	var out []guestMinutes
	at := map[uuid.UUID]int{}
	for _, g := range grants {
		i, ok := at[g.UserID]
		if !ok {
			i = len(out)
			at[g.UserID] = i
			out = append(out, guestMinutes{UserID: g.UserID})
		}
		out[i].Minutes += g.MinutesLeft
		out[i].PLN += float64(g.MinutesLeft) * g.RatePLN / 60
	}
	for i := range out {
		out[i].PLN = round2(out[i].PLN)
	}
	return out
}

// POST /admin/zones/:id/move-minutes — живые минуты зоны (owner): {to_zone_id} — в другую зону, {burn: true} — сжечь.
func handleAdminZoneMoveMinutes(c *gin.Context) {
	var z models.Zone
	if err := db.First(&z, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "zone_not_found", "message": "Зона не найдена"})
		return
	}
	var req zoneMinutesRequest
	toID := ""
	if err := c.ShouldBindJSON(&req); err == nil {
		toID = strings.TrimSpace(req.ToZoneID)
	}
	if req.Burn == (toID != "") { // ни того ни другого, оба сразу или битое тело
		zoneFail(c, http.StatusBadRequest, "bad_target")
		return
	}
	var to *models.Zone
	if !req.Burn {
		var t models.Zone
		if err := db.First(&t, "id = ?", toID).Error; err != nil {
			zoneFail(c, http.StatusNotFound, "to_zone_not_found")
			return
		}
		if t.ID == z.ID {
			zoneFail(c, http.StatusBadRequest, "same_zone")
			return
		}
		to = &t
	}
	var grants []models.MinuteGrant
	err := db.Transaction(func(tx *gorm.DB) error {
		// под замком: тик биллинга, снимающий минуты с этих выдач, подождёт и увидит новую зону или нулевой остаток
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("zone_id = ? AND minutes_left > 0", z.ID).Order("created_at").Find(&grants).Error; err != nil {
			return err
		}
		if len(grants) == 0 {
			return nil
		}
		ids := make([]uuid.UUID, len(grants))
		for i := range grants {
			ids[i] = grants[i].ID
		}
		q := tx.Model(&models.MinuteGrant{}).Where("id IN ?", ids)
		if to != nil {
			return q.Updates(map[string]any{"zone_id": to.ID, "zone_name": to.Name}).Error
		}
		// З18-и1а: сколько и когда сожгли — у выдачи (SET берёт прежний minutes_left), «Сгорело за период» — по ним
		return q.Updates(map[string]any{"minutes_left": 0, "burned_at": gorm.Expr("now()"),
			"burned_minutes": gorm.Expr("burned_minutes + minutes_left")}).Error
	})
	if err != nil {
		respondDBError(c, err)
		return
	}
	if len(grants) == 0 {
		zoneFail(c, http.StatusConflict, "no_minutes")
		return
	}
	per := minutesByGuest(grants)
	total, value := 0, 0.0
	for _, g := range per {
		total += g.Minutes
		value += g.PLN
		target := g.UserID
		if to != nil {
			notifyUser(g.UserID, "zone_minutes_moved", map[string]any{"minutes": g.Minutes, "from": z.Name, "to": to.Name})
			logAdminAction(c, "zone_minutes_move", &target, fmt.Sprintf("минуты зоны %s → %s: %d мин", z.Name, to.Name, g.Minutes))
			reviveActiveSession(g.UserID, &to.ID) // гость сейчас в той зоне — минуты оживляют сессию, как выдача
		} else {
			notifyUser(g.UserID, "zone_minutes_burned", map[string]any{"minutes": g.Minutes, "zone": z.Name})
			logAdminAction(c, "zone_minutes_burn", &target, fmt.Sprintf("минуты зоны %s сожжены: %d мин (%.2f zł по тарифу выдачи)",
				z.Name, g.Minutes, g.PLN))
		}
	}
	out := gin.H{"minutes": total, "guests": len(per), "value_pln": round2(value), "burned": req.Burn}
	if to != nil {
		out["to"] = gin.H{"id": to.ID, "name": to.Name}
	}
	c.JSON(http.StatusOK, out)
}

// zoneFieldsFor — поля обновления ПК при смене зоны: единственный путь, где
// пишутся и ссылка, и кэш имени. Пустой id снимает зону (ПК уйдёт на клубный
// тариф). Второе значение — найденная зона, третье — false, если зоны нет.
func zoneFieldsFor(zoneID string) (map[string]any, *models.Zone, bool) {
	if strings.TrimSpace(zoneID) == "" {
		return map[string]any{"zone_id": nil, "zone": ""}, nil, true
	}
	var z models.Zone
	if err := db.First(&z, "id = ?", zoneID).Error; err != nil {
		return nil, nil, false
	}
	return map[string]any{"zone_id": z.ID, "zone": z.Name}, &z, true
}
