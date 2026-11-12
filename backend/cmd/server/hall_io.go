package main

// З25-и1 «Экспорт и импорт зала — сервер» (OWNER.md, трек З; №56). Зал клуба — файлом: размер схемы, зоны с ценой часа,
// ставкой VAT, порядком и расписанием цен (З24-и1), ПК с клеткой схемы и MAC. Экспорт (GET /admin/hall/export) —
// шаблон второго клуба или копия на чёрный день; импорт (POST /admin/hall/import) — сначала просмотр: что заведём,
// что поправим, чего не тронем и чего в файле нет, — потом запись одной транзакцией и одной строкой журнала.
//
// Импорт ничего не удаляет: зоны и ПК клуба, которых нет в файле, остаются как есть (в просмотре — «в файле нет»).
// ПК с историей (сессии, брони) не трогает: отличия из файла для них — только в просмотре, «есть история — не
// трогаем». Совпадение — по имени без учёта регистра, как у конструктора зала (З19-и2). Клетка из файла занята машиной,
// которую импорт не двигает, — ПК заводится вне схемы, с пометкой. Схему не сжимаем, если за новыми границами
// останутся ПК. Правило расписания из файла добавляется к зоне, если такого нет и оно ни с чем не накладывается.

import (
	"fmt"
	"net"
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
	hallFormat     = "1448-hall"
	hallVersion    = 1
	maxHallZones   = 100
	maxHallPCs     = 1000
	hallLogPCNames = 8 // в строке журнала — первые 8 имён заведённых ПК
)

type hallRule struct {
	DaysMask  int     `json:"days_mask"`
	StartMin  int     `json:"start_min"`
	EndMin    int     `json:"end_min"`
	RatePLN   float64 `json:"rate_pln"`
	ValidFrom string  `json:"valid_from,omitempty"`
	ValidTo   string  `json:"valid_to,omitempty"`
	Note      string  `json:"note,omitempty"`
}

type hallZone struct {
	Name    string     `json:"name"`
	RatePLN float64    `json:"rate_pln"`
	VatPct  int        `json:"vat_pct"`
	Sort    int        `json:"sort"`
	Rules   []hallRule `json:"rules,omitempty"`
}

type hallPC struct {
	Name string `json:"name"`
	Zone string `json:"zone,omitempty"`
	PosX *int   `json:"pos_x,omitempty"`
	PosY *int   `json:"pos_y,omitempty"`
	MAC  string `json:"mac,omitempty"`
}

type hallLayout struct {
	W int `json:"w"`
	H int `json:"h"`
}

// hallFile — файл зала (format «1448-hall», version 1).
type hallFile struct {
	Format     string     `json:"format"`
	Version    int        `json:"version"`
	ExportedAt time.Time  `json:"exported_at"`
	Club       string     `json:"club"`
	Layout     hallLayout `json:"layout"`
	Zones      []hallZone `json:"zones"`
	Computers  []hallPC   `json:"computers"`
}

func hallRuleOf(r models.ZonePriceRule) hallRule {
	h := hallRule{DaysMask: r.DaysMask, StartMin: r.StartMin, EndMin: r.EndMin, RatePLN: r.RatePLN, Note: r.Note}
	if r.ValidFrom != nil {
		h.ValidFrom = r.ValidFrom.Format("2006-01-02")
	}
	if r.ValidTo != nil {
		h.ValidTo = r.ValidTo.Format("2006-01-02")
	}
	return h
}

// ruleOfHall — правило из файла (даты — уже проверены validateHall).
func ruleOfHall(h hallRule, zoneID uuid.UUID) models.ZonePriceRule {
	r := models.ZonePriceRule{ZoneID: zoneID, DaysMask: h.DaysMask, StartMin: h.StartMin, EndMin: h.EndMin, RatePLN: h.RatePLN,
		Note: strings.TrimSpace(h.Note)}
	r.ValidFrom, _ = ruleDate(h.ValidFrom)
	r.ValidTo, _ = ruleDate(h.ValidTo)
	return r
}

func samePos(ax, ay, bx, by *int) bool {
	if ax == nil || ay == nil || bx == nil || by == nil {
		return (ax == nil || ay == nil) == (bx == nil || by == nil)
	}
	return *ax == *bx && *ay == *by
}

func cellText(x, y *int) string {
	if x == nil || y == nil {
		return "вне схемы"
	}
	return fmt.Sprintf("клетка %d×%d", *x+1, *y+1)
}

// hallExport — зал клуба файлом.
func hallExport(club models.Club) hallFile {
	f := hallFile{Format: hallFormat, Version: hallVersion, ExportedAt: time.Now().In(clubLocation), Club: club.Name,
		Layout: hallLayout{W: club.LayoutW, H: club.LayoutH}, Zones: []hallZone{}, Computers: []hallPC{}}
	var zones []models.Zone
	db.Where("club_id = ?", club.ID).Order("sort, name").Find(&zones)
	rules := zoneRulesOf(zoneIDs(zones))
	names := map[uuid.UUID]string{}
	for _, z := range zones {
		names[z.ID] = z.Name
		hz := hallZone{Name: z.Name, RatePLN: z.RatePLN, VatPct: z.VatPct, Sort: z.Sort}
		for _, r := range rules[z.ID] {
			hz.Rules = append(hz.Rules, hallRuleOf(r))
		}
		f.Zones = append(f.Zones, hz)
	}
	var pcs []models.Computer
	db.Where("club_id = ?", club.ID).Order("name").Find(&pcs)
	for _, pc := range pcs {
		h := hallPC{Name: pc.Name, PosX: pc.PosX, PosY: pc.PosY}
		if pc.ZoneID != nil {
			h.Zone = names[*pc.ZoneID]
		}
		if pc.MAC != nil {
			h.MAC = *pc.MAC
		}
		f.Computers = append(f.Computers, h)
	}
	return f
}

// GET /admin/hall/export — зал файлом (владелец); ?download=1 — с именем файла для сохранения.
func handleHallExport(c *gin.Context) {
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Активный клуб не найден"})
		return
	}
	f := hallExport(*club)
	if c.Query("download") == "1" {
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="hall-%s.json"`, time.Now().In(clubLocation).Format("2006-01-02")))
	}
	logAdminAction(c, "hall_export", nil, fmt.Sprintf("схема %d×%d, зон %d, ПК %d", f.Layout.W, f.Layout.H, len(f.Zones), len(f.Computers)))
	c.JSON(http.StatusOK, f)
}

// validateHall — файл зала целиком (чистая, тест): "" или причина словами.
func validateHall(f hallFile) string {
	switch {
	case f.Format != hallFormat:
		return "Это не файл зала 14:48 (format — «1448-hall»)"
	case f.Version != hallVersion:
		return fmt.Sprintf("Файл версии %d — эта админка читает версию %d", f.Version, hallVersion)
	case f.Layout.W < 4 || f.Layout.W > 30 || f.Layout.H < 3 || f.Layout.H > 20:
		return "Схема в файле — ширина 4–30, высота 3–20"
	case len(f.Zones) > maxHallZones:
		return fmt.Sprintf("Зон в файле больше %d", maxHallZones)
	case len(f.Computers) > maxHallPCs:
		return fmt.Sprintf("ПК в файле больше %d", maxHallPCs)
	}
	zones := map[string]bool{}
	for _, z := range f.Zones {
		key := strings.ToLower(strings.TrimSpace(z.Name))
		if ok, code := validateZone(z.Name, z.RatePLN); !ok {
			return fmt.Sprintf("Зона «%s»: %s", z.Name, zoneErrors[code])
		}
		if zones[key] {
			return fmt.Sprintf("Зона «%s» в файле дважды", z.Name)
		}
		zones[key] = true
		if !models.ValidVat(z.VatPct) {
			return fmt.Sprintf("Зона «%s»: ставка VAT %d — не из списка", z.Name, z.VatPct)
		}
		if len(z.Rules) > maxZoneRules {
			return fmt.Sprintf("Зона «%s»: правил больше %d", z.Name, maxZoneRules)
		}
		rules := []models.ZonePriceRule{}
		for _, h := range z.Rules {
			if _, ok := ruleDate(h.ValidFrom); !ok {
				return fmt.Sprintf("Зона «%s»: дата правила «%s» — не ГГГГ-ММ-ДД", z.Name, h.ValidFrom)
			}
			if _, ok := ruleDate(h.ValidTo); !ok {
				return fmt.Sprintf("Зона «%s»: дата правила «%s» — не ГГГГ-ММ-ДД", z.Name, h.ValidTo)
			}
			r := ruleOfHall(h, uuid.Nil)
			if code := validateZoneRule(r); code != "" {
				return fmt.Sprintf("Зона «%s», правило %s: %s", z.Name, zoneRuleText(r), zoneErrors[code])
			}
			for _, o := range rules {
				if rulesOverlap(o, r) {
					return fmt.Sprintf("Зона «%s»: правила «%s» и «%s» накладываются", z.Name, zoneRuleText(o), zoneRuleText(r))
				}
			}
			rules = append(rules, r)
		}
	}
	names, cells := map[string]bool{}, map[string]string{}
	for _, p := range f.Computers {
		n := strings.TrimSpace(p.Name)
		if n == "" || len([]rune(n)) > 32 {
			return fmt.Sprintf("ПК «%s»: имя — 1–32 символа", p.Name)
		}
		if names[strings.ToLower(n)] {
			return fmt.Sprintf("ПК «%s» в файле дважды", n)
		}
		names[strings.ToLower(n)] = true
		if (p.PosX == nil) != (p.PosY == nil) {
			return fmt.Sprintf("ПК «%s»: клетка — оба поля, pos_x и pos_y", n)
		}
		if p.PosX != nil {
			if !posInBounds(*p.PosX, *p.PosY, f.Layout.W, f.Layout.H) {
				return fmt.Sprintf("ПК «%s»: клетка %d×%d за схемой %d×%d", n, *p.PosX+1, *p.PosY+1, f.Layout.W, f.Layout.H)
			}
			key := fmt.Sprintf("%d:%d", *p.PosX, *p.PosY)
			if other, busy := cells[key]; busy {
				return fmt.Sprintf("ПК «%s» и «%s» — в одной клетке %d×%d", other, n, *p.PosX+1, *p.PosY+1)
			}
			cells[key] = n
		}
		if m := strings.TrimSpace(p.MAC); m != "" {
			if _, err := net.ParseMAC(m); err != nil {
				return fmt.Sprintf("ПК «%s»: MAC «%s» — не вида AA:BB:CC:DD:EE:FF", n, m)
			}
		}
	}
	return ""
}

// ── план импорта ─────────────────────────────────────────────────────────────

type hallZoneStep struct {
	Act      string // create | update | same — замораживается в конце плана: запись меняет Zone, а итог — по плану
	File     hallZone
	Zone     *models.Zone // нет — заводим
	Changes  []string
	AddRules []models.ZonePriceRule
	Notes    []string
}

type hallPCStep struct {
	Act     string // create | update | same | history — заморожено в конце плана
	File    hallPC
	PC      *models.Computer // нет — заводим
	History bool
	ZoneKey string // зона из файла, ключ — имя в нижнем регистре; "" — без зоны
	PosX    *int   // итоговая клетка (может отличаться от файла: занята — вне схемы)
	PosY    *int
	MAC     *string
	Changes []string
	Note    string
}

type hallPlan struct {
	Club          models.Club
	LayoutTo      hallLayout
	LayoutChange  bool
	LayoutNote    string
	Zones         []*hallZoneStep
	PCs           []*hallPCStep
	MissingZones  []string
	MissingPCs    []string
	zoneByKey     map[string]*models.Zone
	zoneStepByKey map[string]*hallZoneStep
}

func (s *hallZoneStep) action() string {
	if s.Act != "" {
		return s.Act
	}
	switch {
	case s.Zone == nil:
		return "create"
	case len(s.Changes) > 0 || len(s.AddRules) > 0:
		return "update"
	}
	return "same"
}

func (s *hallPCStep) action() string {
	if s.Act != "" {
		return s.Act
	}
	switch {
	case s.PC == nil:
		return "create"
	case s.History && len(s.Changes) > 0:
		return "history"
	case len(s.Changes) > 0:
		return "update"
	}
	return "same"
}

// buildHallPlan — что сделает импорт файла f (tx — в записи под замком клуба; в просмотре — db).
func buildHallPlan(tx *gorm.DB, club models.Club, f hallFile) *hallPlan {
	p := &hallPlan{Club: club, LayoutTo: hallLayout{W: club.LayoutW, H: club.LayoutH}, zoneByKey: map[string]*models.Zone{},
		zoneStepByKey: map[string]*hallZoneStep{}}

	// зоны
	var zones []models.Zone
	tx.Where("club_id = ?", club.ID).Order("sort, name").Find(&zones)
	for i := range zones {
		p.zoneByKey[strings.ToLower(zones[i].Name)] = &zones[i]
	}
	rules := zoneRulesOf(zoneIDs(zones))
	inFileZone := map[string]bool{}
	for _, fz := range f.Zones {
		key := strings.ToLower(strings.TrimSpace(fz.Name))
		inFileZone[key] = true
		st := &hallZoneStep{File: fz, Zone: p.zoneByKey[key]}
		p.zoneStepByKey[key] = st
		p.Zones = append(p.Zones, st)
		have := []models.ZonePriceRule{}
		if st.Zone != nil {
			z := st.Zone
			if z.RatePLN != fz.RatePLN {
				st.Changes = append(st.Changes, fmt.Sprintf("цена %s → %s zł/ч", plnRU(z.RatePLN), plnRU(fz.RatePLN)))
			}
			if z.VatPct != fz.VatPct {
				st.Changes = append(st.Changes, "VAT "+models.VatLabel(z.VatPct)+" → "+models.VatLabel(fz.VatPct))
			}
			if z.Sort != fz.Sort {
				st.Changes = append(st.Changes, fmt.Sprintf("порядок %d → %d", z.Sort, fz.Sort))
			}
			have = append(have, rules[z.ID]...)
		}
		for _, h := range fz.Rules {
			r := ruleOfHall(h, uuid.Nil)
			dup, clash := false, ""
			for _, o := range have {
				if o.DaysMask == r.DaysMask && o.StartMin == r.StartMin && o.EndMin == r.EndMin && o.RatePLN == r.RatePLN &&
					dayEq(o.ValidFrom, r.ValidFrom) && dayEq(o.ValidTo, r.ValidTo) {
					dup = true
					break
				}
				if clash == "" && rulesOverlap(o, r) {
					clash = zoneRuleText(o)
				}
			}
			switch {
			case dup:
			case clash != "":
				st.Notes = append(st.Notes, fmt.Sprintf("правило «%s» накладывается на «%s» — не добавляем", zoneRuleText(r), clash))
			case len(have) >= maxZoneRules:
				st.Notes = append(st.Notes, fmt.Sprintf("правило «%s» — у зоны уже %d правил, не добавляем", zoneRuleText(r), maxZoneRules))
			default:
				st.AddRules = append(st.AddRules, r)
				have = append(have, r)
			}
		}
		if len(st.AddRules) > 0 && st.Zone != nil {
			st.Changes = append(st.Changes, fmt.Sprintf("правил +%d", len(st.AddRules)))
		}
	}
	for _, z := range zones {
		if !inFileZone[strings.ToLower(z.Name)] {
			p.MissingZones = append(p.MissingZones, z.Name)
		}
	}

	// ПК
	var pcs []models.Computer
	tx.Where("club_id = ?", club.ID).Order("name").Find(&pcs)
	byKey := map[string]*models.Computer{}
	for i := range pcs {
		byKey[strings.ToLower(pcs[i].Name)] = &pcs[i]
	}
	hist := map[uuid.UUID]bool{}
	if len(pcs) > 0 {
		var ids []uuid.UUID
		tx.Raw(`SELECT DISTINCT computer_id FROM sessions WHERE club_id = ? UNION SELECT DISTINCT computer_id FROM bookings
		        WHERE computer_id IS NOT NULL`, club.ID).Scan(&ids)
		for _, id := range ids {
			hist[id] = true
		}
	}
	zoneName := func(id *uuid.UUID) string {
		if id == nil {
			return ""
		}
		for _, z := range zones {
			if z.ID == *id {
				return z.Name
			}
		}
		return ""
	}
	inFilePC := map[string]bool{}
	for _, fp := range f.Computers {
		key := strings.ToLower(strings.TrimSpace(fp.Name))
		inFilePC[key] = true
		st := &hallPCStep{File: fp, PC: byKey[key], ZoneKey: strings.ToLower(strings.TrimSpace(fp.Zone)), PosX: fp.PosX, PosY: fp.PosY}
		if m := strings.ToUpper(strings.TrimSpace(fp.MAC)); m != "" {
			st.MAC = &m
		}
		if st.PC != nil {
			pc := st.PC
			st.History = hist[pc.ID]
			if zn := zoneName(pc.ZoneID); strings.ToLower(zn) != st.ZoneKey {
				st.Changes = append(st.Changes, "зона: "+orText(zn, "без зоны")+" → "+orText(strings.TrimSpace(fp.Zone), "без зоны"))
			}
			if !samePos(pc.PosX, pc.PosY, fp.PosX, fp.PosY) {
				st.Changes = append(st.Changes, cellText(pc.PosX, pc.PosY)+" → "+cellText(fp.PosX, fp.PosY))
			}
			cur := ""
			if pc.MAC != nil {
				cur = strings.ToUpper(*pc.MAC)
			}
			if want := deref(st.MAC); cur != want {
				st.Changes = append(st.Changes, "MAC "+orText(cur, "нет")+" → "+orText(want, "нет"))
			}
		}
		p.PCs = append(p.PCs, st)
	}
	for _, pc := range pcs {
		if !inFilePC[strings.ToLower(pc.Name)] {
			p.MissingPCs = append(p.MissingPCs, pc.Name)
		}
	}
	// зона ПК из файла — в файле или в клубе
	for _, st := range p.PCs {
		if st.ZoneKey == "" {
			continue
		}
		if _, ok := p.zoneStepByKey[st.ZoneKey]; !ok && p.zoneByKey[st.ZoneKey] == nil {
			st.Note = fmt.Sprintf("зоны «%s» нет ни в файле, ни в клубе — ПК без зоны", strings.TrimSpace(st.File.Zone))
			st.ZoneKey = ""
		}
	}

	// клетки: машины, которые импорт не двигает, держат свои клетки
	held := map[string]string{}
	stays := func(pc *models.Computer) {
		if pc.PosX != nil && pc.PosY != nil {
			held[fmt.Sprintf("%d:%d", *pc.PosX, *pc.PosY)] = pc.Name
		}
	}
	for i := range pcs {
		if !inFilePC[strings.ToLower(pcs[i].Name)] {
			stays(&pcs[i])
		}
	}
	for _, st := range p.PCs {
		if st.PC != nil && (st.History || samePos(st.PC.PosX, st.PC.PosY, st.File.PosX, st.File.PosY)) {
			stays(st.PC)
		}
	}
	for _, st := range p.PCs {
		if st.PC != nil && (st.History || samePos(st.PC.PosX, st.PC.PosY, st.File.PosX, st.File.PosY)) {
			continue
		}
		if st.PosX == nil {
			continue
		}
		key := fmt.Sprintf("%d:%d", *st.PosX, *st.PosY)
		if who, busy := held[key]; busy {
			st.Note = fmt.Sprintf("%s занята «%s» — %s", cellText(st.PosX, st.PosY), who,
				map[bool]string{true: "заводим вне схемы", false: "клетку не меняем"}[st.PC == nil])
			if st.PC == nil {
				st.PosX, st.PosY = nil, nil
			} else {
				st.PosX, st.PosY = st.PC.PosX, st.PC.PosY
				st.Changes = dropCellChange(st.Changes)
				if st.PosX != nil {
					held[fmt.Sprintf("%d:%d", *st.PosX, *st.PosY)] = st.PC.Name
				}
			}
			continue
		}
		held[key] = strings.TrimSpace(st.File.Name)
	}

	// схема: растёт всегда, сжимается — если за новыми границами никого
	if f.Layout.W != club.LayoutW || f.Layout.H != club.LayoutH {
		out := []string{}
		for key, who := range held {
			var x, y int
			fmt.Sscanf(key, "%d:%d", &x, &y)
			if !posInBounds(x, y, f.Layout.W, f.Layout.H) {
				out = append(out, who)
			}
		}
		sort.Strings(out)
		if len(out) == 0 {
			p.LayoutTo, p.LayoutChange = f.Layout, true
		} else {
			p.LayoutNote = fmt.Sprintf("схему %d×%d не сжимаем: за границами останутся %s", f.Layout.W, f.Layout.H, strings.Join(out, ", "))
		}
	}
	for _, s := range p.Zones {
		s.Act = s.action()
	}
	for _, s := range p.PCs {
		s.Act = s.action()
	}
	return p
}

func dayEq(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

func orText(s, empty string) string {
	if s == "" {
		return empty
	}
	return s
}

func dropCellChange(ch []string) []string {
	out := ch[:0]
	for _, c := range ch {
		if !strings.HasPrefix(c, "клетка ") && !strings.HasPrefix(c, "вне схемы") {
			out = append(out, c)
		}
	}
	return out
}

// summary — итог словами для просмотра и журнала.
func (p *hallPlan) summary() string {
	count := func(acts []string, a string) int {
		n := 0
		for _, x := range acts {
			if x == a {
				n++
			}
		}
		return n
	}
	za, pa := []string{}, []string{}
	for _, s := range p.Zones {
		za = append(za, s.action())
	}
	for _, s := range p.PCs {
		pa = append(pa, s.action())
	}
	parts := []string{}
	if p.LayoutChange {
		parts = append(parts, fmt.Sprintf("схема %d×%d → %d×%d", p.Club.LayoutW, p.Club.LayoutH, p.LayoutTo.W, p.LayoutTo.H))
	}
	parts = append(parts, fmt.Sprintf("зоны: новых %d, правок %d, без изменений %d", count(za, "create"), count(za, "update"), count(za, "same")))
	parts = append(parts, fmt.Sprintf("ПК: новых %d, правок %d, без изменений %d, с историей не тронуто %d", count(pa, "create"),
		count(pa, "update"), count(pa, "same"), count(pa, "history")))
	if len(p.MissingZones)+len(p.MissingPCs) > 0 {
		parts = append(parts, fmt.Sprintf("в файле нет: зон %d, ПК %d — остаются", len(p.MissingZones), len(p.MissingPCs)))
	}
	return strings.Join(parts, "; ")
}

func (p *hallPlan) out(applied bool) gin.H {
	zs := make([]gin.H, 0, len(p.Zones))
	for _, s := range p.Zones {
		zs = append(zs, gin.H{"name": s.File.Name, "action": s.action(), "changes": nz(s.Changes), "notes": nz(s.Notes),
			"rules_add": len(s.AddRules)})
	}
	ps := make([]gin.H, 0, len(p.PCs))
	for _, s := range p.PCs {
		ps = append(ps, gin.H{"name": strings.TrimSpace(s.File.Name), "action": s.action(), "changes": nz(s.Changes), "note": s.Note,
			"zone": strings.TrimSpace(s.File.Zone), "cell": cellText(s.PosX, s.PosY)})
	}
	return gin.H{"applied": applied, "summary": p.summary(),
		"layout": gin.H{"from": fmt.Sprintf("%d×%d", p.Club.LayoutW, p.Club.LayoutH), "to": fmt.Sprintf("%d×%d", p.LayoutTo.W, p.LayoutTo.H),
			"change": p.LayoutChange, "note": p.LayoutNote},
		"zones": zs, "computers": ps, "missing": gin.H{"zones": nz(p.MissingZones), "computers": nz(p.MissingPCs)}}
}

func nz(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// applyHallPlan — запись плана (в транзакции под замком клуба).
func applyHallPlan(tx *gorm.DB, p *hallPlan) error {
	if p.LayoutChange {
		if err := tx.Model(&models.Club{}).Where("id = ?", p.Club.ID).
			Updates(map[string]any{"layout_w": p.LayoutTo.W, "layout_h": p.LayoutTo.H}).Error; err != nil {
			return err
		}
	}
	for _, s := range p.Zones {
		if s.Zone == nil {
			z := models.Zone{ClubID: p.Club.ID, Name: strings.TrimSpace(s.File.Name), RatePLN: s.File.RatePLN, VatPct: s.File.VatPct, Sort: s.File.Sort}
			if err := tx.Create(&z).Error; err != nil {
				return err
			}
			s.Zone = &z
			p.zoneByKey[strings.ToLower(z.Name)] = &z
		} else if len(s.Changes) > 0 {
			if err := tx.Model(&models.Zone{}).Where("id = ?", s.Zone.ID).Updates(map[string]any{"rate_pln": s.File.RatePLN,
				"vat_pct": s.File.VatPct, "sort": s.File.Sort, "updated_at": gorm.Expr("now()")}).Error; err != nil {
				return err
			}
		}
		for i := range s.AddRules {
			s.AddRules[i].ZoneID = s.Zone.ID
			if err := tx.Create(&s.AddRules[i]).Error; err != nil {
				return err
			}
		}
	}
	zoneOf := func(key string) (*uuid.UUID, string) {
		if key == "" {
			return nil, ""
		}
		if z := p.zoneByKey[key]; z != nil {
			return &z.ID, z.Name
		}
		return nil, ""
	}
	// двигаемые машины сначала снимаем со схемы — обмен клетками не упрётся сам в себя
	for _, s := range p.PCs {
		if s.PC != nil && s.action() == "update" && !samePos(s.PC.PosX, s.PC.PosY, s.PosX, s.PosY) {
			if err := tx.Model(&models.Computer{}).Where("id = ?", s.PC.ID).Updates(map[string]any{"pos_x": nil, "pos_y": nil}).Error; err != nil {
				return err
			}
		}
	}
	for _, s := range p.PCs {
		zid, zname := zoneOf(s.ZoneKey)
		switch s.action() {
		case "create":
			pc := models.Computer{ClubID: p.Club.ID, Name: strings.TrimSpace(s.File.Name), Status: models.ComputerStatusAvailable,
				PosX: s.PosX, PosY: s.PosY, MAC: s.MAC, ZoneID: zid, Zone: zname}
			if err := tx.Create(&pc).Error; err != nil {
				return err
			}
		case "update":
			if err := tx.Model(&models.Computer{}).Where("id = ?", s.PC.ID).Updates(map[string]any{"zone_id": zid, "zone": zname,
				"pos_x": s.PosX, "pos_y": s.PosY, "mac": s.MAC}).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

type hallImportRequest struct {
	Hall  hallFile `json:"hall"`
	Apply bool     `json:"apply"`
}

// POST /admin/hall/import — просмотр (apply: false) или запись (apply: true) файла зала (владелец).
func handleHallImport(c *gin.Context) {
	var q hallImportRequest
	if err := c.ShouldBindJSON(&q); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_file", "message": "Файл не читается как файл зала (JSON)"})
		return
	}
	if msg := validateHall(q.Hall); msg != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_file", "message": msg})
		return
	}
	club, ok := defaultClub()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "club_not_found", "message": "Активный клуб не найден"})
		return
	}
	if !q.Apply {
		c.JSON(http.StatusOK, buildHallPlan(db, *club, q.Hall).out(false))
		return
	}
	var plan *hallPlan
	err := db.Transaction(func(tx *gorm.DB) error {
		var locked models.Club
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", club.ID).Error; err != nil {
			return err
		}
		plan = buildHallPlan(tx, locked, q.Hall)
		return applyHallPlan(tx, plan)
	})
	if err != nil {
		if isDuplicate(err) {
			c.JSON(http.StatusConflict, gin.H{"code": "name_taken", "message": "Имя зоны или ПК уже занято — посмотри ещё раз"})
			return
		}
		respondDBError(c, err)
		return
	}
	created := []string{}
	for _, s := range plan.PCs {
		if s.action() == "create" {
			created = append(created, strings.TrimSpace(s.File.Name))
		}
	}
	text := "из файла: " + plan.summary()
	if len(created) > 0 {
		if len(created) > hallLogPCNames {
			text += fmt.Sprintf("; заведены: %s и ещё %d", strings.Join(created[:hallLogPCNames], ", "), len(created)-hallLogPCNames)
		} else {
			text += "; заведены: " + strings.Join(created, ", ")
		}
	}
	logAdminAction(c, "hall_import", nil, text)
	hub.AdminBroadcast("hall", map[string]any{"kind": "import"})
	c.JSON(http.StatusOK, plan.out(true))
}
