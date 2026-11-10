package main

// З18-и5 «Карточка клуба — сервер» (№34, З-Р23; из №40 — публичные /clubs белым списком). Владелец правит карточку
// клуба — имя, адрес, телефон, Telegram, Instagram, часы работы и клубный тариф (цена часа ПК без зоны) — в
// PATCH /admin/clubs/:id; раньше всё это правилось только SQL. Наружу (GET /clubs*, без входа) — только то, что висит
// на двери клуба: служебного (rtp_modifier, схема зала, is_active, MAC и придержание ПК) там больше нет. Настройка
// «ПК без зоны запрещён» (pc_zone_required) — settings.go и hall.go.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxClubName    = 128 // clubs.name — VARCHAR(128)
	maxClubAddress = 300
	maxClubPhone   = 20 // clubs.phone — VARCHAR(20)
)

// publicClub — карточка клуба для всех (GET /clubs, GET /clubs/:id): имя, адрес, координаты, телефон, соцсети, часы
// работы (null — круглосуточно) и клубный тариф.
func publicClub(cl models.Club) gin.H {
	return gin.H{
		"id": cl.ID, "name": cl.Name, "address": cl.Address,
		"latitude": cl.Latitude, "longitude": cl.Longitude,
		"phone": cl.Phone, "telegram": cl.Telegram, "instagram": cl.Instagram,
		"working_hours": clubHoursOut(cl.WorkingHours), "base_rate_pln": cl.BaseRatePLN,
	}
}

// publicComputer — ПК клуба для всех (GET /clubs/:id/computers): имя, зона, статус и место на схеме; MAC, придержание
// под регистрацию и кто придержал — нет.
func publicComputer(pc models.Computer) gin.H {
	return gin.H{"id": pc.ID, "name": pc.Name, "zone": pc.Zone, "status": pc.Status, "pos_x": pc.PosX, "pos_y": pc.PosY}
}

// clubHoursOut — часы работы из базы в ответ: объект или null (круглосуточно).
func clubHoursOut(h *string) any {
	if h == nil || strings.TrimSpace(*h) == "" || strings.TrimSpace(*h) == "null" {
		return nil
	}
	return json.RawMessage(*h)
}

var (
	weekDays   = []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"}
	weekDaysRU = map[string]string{"mon": "пн", "tue": "вт", "wed": "ср", "thu": "чт", "fri": "пт", "sat": "сб", "sun": "вс"}
	hhmmRe     = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)
)

type dayHours struct {
	Open  string `json:"open"`
	Close string `json:"close"`
}

// clubHoursParse — часы работы из формы владельца. null — круглосуточно (→ nil). Иначе объект «день → {open, close}»:
// дни mon…sun, время «ЧЧ:ММ», закрытие — и «24:00»; закрытие раньше открытия — работают после полуночи
// («10:00»–«02:00»); круглые сутки дня — «00:00»–«24:00»; дня в объекте нет (или он null) — выходной; рабочий день
// хотя бы один. → канонический JSON (дни по порядку недели) или фраза об ошибке. Чистая функция (тест).
func clubHoursParse(raw json.RawMessage) (*string, string) {
	if strings.TrimSpace(string(raw)) == "null" {
		return nil, ""
	}
	var in map[string]*dayHours
	if err := json.Unmarshal(raw, &in); err != nil || in == nil {
		return nil, "Часы работы — по дням: {\"mon\": {\"open\": \"10:00\", \"close\": \"23:00\"}, …} или null — круглосуточно"
	}
	for k := range in {
		if weekDaysRU[k] == "" {
			return nil, "Часы работы: неизвестный день «" + k + "» — дни mon, tue, wed, thu, fri, sat, sun"
		}
	}
	parts := []string{}
	for _, d := range weekDays {
		h := in[d]
		if h == nil {
			continue
		}
		o, c := strings.TrimSpace(h.Open), strings.TrimSpace(h.Close)
		if !hhmmRe.MatchString(o) || !(hhmmRe.MatchString(c) || c == "24:00") || o == c {
			return nil, "Часы работы, " + weekDaysRU[d] + ": открытие и закрытие — «ЧЧ:ММ» и разные (круглые сутки — 00:00–24:00, " +
				"после полуночи — например 10:00–02:00)"
		}
		parts = append(parts, fmt.Sprintf(`"%s": {"open": "%s", "close": "%s"}`, d, o, c))
	}
	if len(parts) == 0 {
		return nil, "Часы работы: хотя бы один рабочий день (круглосуточно — null)"
	}
	s := "{" + strings.Join(parts, ", ") + "}"
	return &s, ""
}

// clubHoursCanon — часы из базы в том же виде, что у clubHoursParse (jsonb хранит ключи в своём порядке): сравнить
// «было» со «стало». Нечитаемое — как есть.
func clubHoursCanon(h *string) *string {
	if h == nil || strings.TrimSpace(*h) == "" {
		return nil
	}
	canon, msg := clubHoursParse(json.RawMessage(*h))
	if msg != "" {
		return h
	}
	return canon
}

// clubHoursText — часы словами для журнала: «круглосуточно» или «пн–пт 10:00–23:00, сб 12:00–02:00, выходной: вс» —
// подряд идущие дни с одинаковыми часами одним отрезком. Чистая функция (тест).
func clubHoursText(canon *string) string {
	if canon == nil {
		return "круглосуточно"
	}
	var in map[string]*dayHours
	if json.Unmarshal([]byte(*canon), &in) != nil {
		return *canon
	}
	parts, off := []string{}, []string{}
	for i := 0; i < len(weekDays); {
		h := in[weekDays[i]]
		if h == nil {
			off = append(off, weekDaysRU[weekDays[i]])
			i++
			continue
		}
		j := i
		for j+1 < len(weekDays) && in[weekDays[j+1]] != nil && *in[weekDays[j+1]] == *h {
			j++
		}
		days := weekDaysRU[weekDays[i]]
		if j > i {
			days += "–" + weekDaysRU[weekDays[j]]
		}
		parts = append(parts, days+" "+h.Open+"–"+h.Close)
		i = j + 1
	}
	if len(off) > 0 {
		parts = append(parts, "выходной: "+strings.Join(off, ", "))
	}
	return strings.Join(parts, ", ")
}

var (
	phoneRe     = regexp.MustCompile(`^\+?[0-9 ()-]+$`)
	telegramRe  = regexp.MustCompile(`^[A-Za-z0-9_]{5,32}$`)
	instagramRe = regexp.MustCompile(`^[A-Za-z0-9._]{1,30}$`)
)

// trimPrefixes — снять по порядку каждый из префиксов, если строка с него начинается (без учёта регистра).
func trimPrefixes(v string, ps ...string) string {
	for _, p := range ps {
		if len(v) >= len(p) && strings.EqualFold(v[:len(p)], p) {
			v = v[len(p):]
		}
	}
	return v
}

// clubContact — телефон, Telegram или Instagram из формы: пробелы по краям убраны, пусто — стереть (nil). Телефон —
// цифры (от 6 до 15), пробелы, «-», скобки и «+» в начале, до 20 знаков; Telegram и Instagram — имя без «@» и без
// ссылки (t.me/…, instagram.com/…). Не подходит — false. Чистая функция (тест).
func clubContact(kind, v string) (*string, bool) {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil, true
	}
	switch kind {
	case "telegram":
		v = trimPrefixes(v, "https://", "http://", "www.", "t.me/", "telegram.me/", "@")
	case "instagram":
		v = strings.TrimSuffix(trimPrefixes(v, "https://", "http://", "www.", "instagram.com/", "@"), "/")
	}
	if v == "" { // ссылка или «@» без имени
		return nil, false
	}
	ok := false
	switch kind {
	case "phone":
		digits := 0
		for _, r := range v {
			if r >= '0' && r <= '9' {
				digits++
			}
		}
		ok = len(v) <= maxClubPhone && phoneRe.MatchString(v) && !strings.Contains(v[1:], "+") && digits >= 6 && digits <= 15
	case "telegram":
		ok = telegramRe.MatchString(v)
	case "instagram":
		ok = instagramRe.MatchString(v)
	}
	if !ok {
		return nil, false
	}
	return &v, true
}

type clubPatchRequest struct {
	Name         *string         `json:"name"`
	Address      *string         `json:"address"`
	Phone        *string         `json:"phone"`     // "" — стереть
	Telegram     *string         `json:"telegram"`  // "" — стереть
	Instagram    *string         `json:"instagram"` // "" — стереть
	WorkingHours json.RawMessage `json:"working_hours"`
	BaseRatePLN  *float64        `json:"base_rate_pln"`
}

var clubMessages = map[string]string{
	"bad_name":      fmt.Sprintf("Имя клуба — от 1 до %d символов", maxClubName),
	"bad_address":   fmt.Sprintf("Адрес — от 1 до %d символов", maxClubAddress),
	"bad_phone":     "Телефон — цифры (от 6 до 15), пробелы, «-», скобки и «+» в начале; до 20 знаков",
	"bad_telegram":  "Telegram — имя аккаунта или канала: 5–32 латинские буквы, цифры и «_» (можно с @ или ссылкой t.me/…)",
	"bad_instagram": "Instagram — имя аккаунта: до 30 латинских букв, цифр, «.» и «_» (можно с @ или ссылкой)",
	"bad_rate":      fmt.Sprintf("Клубный тариф — от 0,01 до %.0f zł за час и не больше двух знаков после запятой", maxZoneRate),
}

// orDash — значение для журнала: пусто — «—».
func orDash(p *string) string {
	if p == nil || *p == "" {
		return "—"
	}
	return *p
}

// PATCH /admin/clubs/:id — карточка клуба (owner). Поля нет в запросе — не трогать; телефон, Telegram и Instagram
// пустой строкой — стереть; часы работы null — круглосуточно. Значение как было — не правка: сохранение без правок —
// ни записи, ни строки журнала (как у ПК, З16-и1б). Журнал club_update — что поменялось, «было → стало». Клубный
// тариф — цена часа ПК без зоны; идущие сессии считаются по своему снимку (З16-и1), новый тариф — со следующих.
func handleAdminClubUpdate(c *gin.Context) {
	var club models.Club
	if err := db.First(&club, "id = ?", c.Param("id")).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Клуб не найден"})
		return
	}
	var req clubPatchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondBindError(c, err)
		return
	}
	fail := func(code, msg string) {
		if msg == "" {
			msg = clubMessages[code]
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": msg})
	}
	updates, changes := map[string]any{}, []string{}
	for _, f := range []struct {
		col, label string
		in         *string
		cur        string
		max        int
	}{{"name", "имя", req.Name, club.Name, maxClubName}, {"address", "адрес", req.Address, club.Address, maxClubAddress}} {
		if f.in == nil {
			continue
		}
		v := strings.TrimSpace(*f.in)
		if n := utf8.RuneCountInString(v); n < 1 || n > f.max {
			fail("bad_"+f.col, "")
			return
		}
		if v != f.cur {
			updates[f.col] = v
			changes = append(changes, f.label+" «"+f.cur+"» → «"+v+"»")
		}
	}
	for _, f := range []struct {
		col, label string
		in, cur    *string
	}{{"phone", "телефон", req.Phone, club.Phone}, {"telegram", "Telegram", req.Telegram, club.Telegram},
		{"instagram", "Instagram", req.Instagram, club.Instagram}} {
		if f.in == nil {
			continue
		}
		v, ok := clubContact(f.col, *f.in)
		if !ok {
			fail("bad_"+f.col, "")
			return
		}
		if orDash(v) != orDash(f.cur) {
			if v == nil {
				updates[f.col] = nil
			} else {
				updates[f.col] = *v
			}
			changes = append(changes, f.label+" "+orDash(f.cur)+" → "+orDash(v))
		}
	}
	if len(req.WorkingHours) > 0 {
		canon, msg := clubHoursParse(req.WorkingHours)
		if msg != "" {
			fail("bad_hours", msg)
			return
		}
		was := clubHoursCanon(club.WorkingHours)
		if orDash(canon) != orDash(was) {
			if canon == nil {
				updates["working_hours"] = nil
			} else {
				updates["working_hours"] = *canon
			}
			changes = append(changes, "часы "+clubHoursText(was)+" → "+clubHoursText(canon))
		}
	}
	if req.BaseRatePLN != nil {
		r := *req.BaseRatePLN
		if !validatePLN(r, maxZoneRate) {
			fail("bad_rate", "")
			return
		}
		if r = round2(r); r != club.BaseRatePLN {
			updates["base_rate_pln"] = r
			changes = append(changes, fmt.Sprintf("клубный тариф %.2f → %.2f zł/ч", club.BaseRatePLN, r))
		}
	}
	if len(updates) == 0 {
		c.JSON(http.StatusOK, gin.H{"club": publicClub(club), "unchanged": true})
		return
	}
	if err := db.Model(&club).Updates(updates).Error; err != nil {
		respondDBError(c, err)
		return
	}
	if err := db.First(&club, "id = ?", club.ID).Error; err != nil {
		respondDBError(c, err)
		return
	}
	logAdminAction(c, "club_update", nil, strings.Join(changes, ", "))
	c.JSON(http.StatusOK, gin.H{"club": publicClub(club)})
}
