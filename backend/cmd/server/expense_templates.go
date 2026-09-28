package main

// З9-и5 (OWNER.md, трек З; ревью 23.09 №67, Д10; решение основателя 28.09 —
// только ежемесячные): регулярные расходы. Шаблон «аренда 1-го числа 4 500 zł,
// банк» — владельцу: в «Справочниках» завести, поправить, выключить (удаления
// нет). Когда срок наступил (число месяца; нет такого — последний день), месяц
// ждёт внесения: «Ждут внесения» в «Расходах» с кнопкой «Внести» (шторка
// расхода, заполненная по шаблону — черновик одним кликом), бейдж на
// «Бухгалтерии», в день срока — одно сообщение «только владельцу» в канал.
// Расход, внесённый по шаблону, помнит шаблон и месяц: за месяц — один (409),
// сторно освобождает месяц. Шаблон ждёт с ближайшего срока после заведения
// (или включения): прошедшие месяцы на него не вешаются. З9-и5б (решение
// основателя 28.09): заплатили раньше срока — «Внести заранее» у шаблона в
// «Справочниках»: ближайший месяц, чей срок впереди (этот или следующий), в
// срок тогда не ждёт и не напоминает.

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

const (
	maxTemplateName = 64
	dueLookback     = 12 // «ждут» — не дальше года назад: забытый шаблон не вывалит десятки месяцев
)

var errTemplate = errors.New("template")

// monthOf — первое число месяца даты d.
func monthOf(d time.Time) time.Time {
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, d.Location())
}

// monthIn — первое число месяца даты d в поясе loc: даты из базы (полночь клуба
// или UTC) сравниваются с «сегодня» по календарю, а не по мгновению.
func monthIn(d time.Time, loc *time.Location) time.Time {
	return time.Date(d.Year(), d.Month(), 1, 0, 0, 0, 0, loc)
}

// dueDay — срок шаблона в месяце m: число day, а нет такого в месяце — последний
// день (31-е в сентябре — 30-е, в феврале — 28-е или 29-е). Чистая, тест.
func dueDay(m time.Time, day int) time.Time {
	if last := time.Date(m.Year(), m.Month()+1, 0, 0, 0, 0, 0, m.Location()).Day(); day > last {
		day = last
	}
	return time.Date(m.Year(), m.Month(), day, 0, 0, 0, 0, m.Location())
}

// firstDueMonth — с какого месяца ждёт шаблон, заведённый (или включённый) в
// день today: срок в этом месяце сегодня или впереди — с этого, прошёл — со
// следующего. Чистая, тест.
func firstDueMonth(today time.Time, day int) time.Time {
	m := monthOf(today)
	if dueDay(m, day).Before(today) {
		return m.AddDate(0, 1, 0)
	}
	return m
}

// monthKey — месяц ключом «ГГГГ-ММ».
func monthKey(m time.Time) string { return m.Format("2006-01") }

// dueMonths — месяцы шаблона, чей срок наступил (≤ today) и которые не внесены
// (filled — ключи monthKey): с from, но не дальше dueLookback месяцев назад, по
// порядку. Чистая, тест.
func dueMonths(from, today time.Time, day int, filled map[string]bool) []time.Time {
	cur := monthOf(today)
	start := cur.AddDate(0, -(dueLookback - 1), 0)
	if f := monthIn(from, today.Location()); f.After(start) {
		start = f
	}
	out := []time.Time{}
	for m := start; !m.After(cur); m = m.AddDate(0, 1, 0) {
		if dueDay(m, day).After(today) || filled[monthKey(m)] {
			continue
		}
		out = append(out, m)
	}
	return out
}

// nextDue — ближайший срок шаблона, которого ещё не внесли: ждущий (наступил)
// или будущий. Чистая, тест.
func nextDue(from, today time.Time, day int, filled map[string]bool) time.Time {
	if due := dueMonths(from, today, day, filled); len(due) > 0 {
		return dueDay(due[0], day)
	}
	m := monthOf(today)
	if f := monthIn(from, today.Location()); f.After(m) {
		m = f
	}
	for filled[monthKey(m)] || dueDay(m, day).Before(today) {
		m = m.AddDate(0, 1, 0)
	}
	return dueDay(m, day)
}

// earlyMonth — месяц шаблона, который можно внести заранее (З9-и5б): ничего не
// ждёт, а ближайший срок впереди — в этом месяце или в следующем. Чистая, тест.
func earlyMonth(from, today time.Time, day int, filled map[string]bool) (time.Time, bool) {
	if len(dueMonths(from, today, day, filled)) > 0 {
		return time.Time{}, false // сначала — то, что уже ждёт («Ждут внесения»)
	}
	m := monthOf(nextDue(from, today, day, filled))
	if m.After(monthOf(today).AddDate(0, 1, 0)) {
		return time.Time{}, false // следующий месяц уже внесён — дальше рано
	}
	return m, true
}

type templateRequest struct {
	Name           *string  `json:"name"`
	CategoryID     *string  `json:"category_id"`
	CounterpartyID *string  `json:"counterparty_id"` // "" — без контрагента
	AmountPLN      *float64 `json:"amount_pln"`
	VatPLN         *float64 `json:"vat_pln"`
	Method         *string  `json:"method"`
	Day            *int     `json:"day"`
	Note           *string  `json:"note"`
	Active         *bool    `json:"active"`
}

// checkTemplate — поля шаблона поверх текущих (чистая, тест): cur — шаблон до
// правки (новый — пустой). Категория, контрагент и способ проверяются базой.
func checkTemplate(req templateRequest, cur models.ExpenseTemplate) (string, models.ExpenseTemplate) {
	t := cur
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if n := len([]rune(name)); n == 0 || n > maxTemplateName {
			return "bad_template_name", t
		}
		t.Name = name
	}
	if req.CategoryID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*req.CategoryID))
		if err != nil {
			return "bad_category", t
		}
		t.CategoryID = id
	}
	if req.CounterpartyID != nil {
		if s := strings.TrimSpace(*req.CounterpartyID); s == "" {
			t.CounterpartyID = nil
		} else {
			id, err := uuid.Parse(s)
			if err != nil {
				return "bad_counterparty", t
			}
			t.CounterpartyID = &id
		}
	}
	if req.AmountPLN != nil {
		if !validatePLN(*req.AmountPLN, maxExpensePLN) {
			return "bad_amount", t
		}
		t.GrossGrosz = models.GroszFromPLN(*req.AmountPLN)
	}
	if req.VatPLN != nil {
		v := *req.VatPLN
		if v < 0 || (v > 0 && !validatePLN(v, maxExpensePLN)) {
			return "bad_vat", t
		}
		t.VatGrosz = models.GroszFromPLN(v)
	}
	if t.VatGrosz > t.GrossGrosz {
		return "bad_vat", t
	}
	if req.Method != nil {
		if _, ok := expenseMethods[*req.Method]; !ok {
			return "bad_method", t
		}
		t.Method = *req.Method
	}
	if req.Day != nil {
		if *req.Day < 1 || *req.Day > 31 {
			return "bad_day", t
		}
		t.Day = *req.Day
	}
	if req.Note != nil {
		note := strings.TrimSpace(*req.Note)
		if len([]rune(note)) > maxExpenseNote {
			return "bad_note", t
		}
		t.Note = note
	}
	if t.Name == "" || t.CategoryID == uuid.Nil || t.GrossGrosz <= 0 || t.Method == "" || t.Day == 0 {
		return "invalid_request", t
	}
	return "", t
}

// templateRefs — категория и контрагент есть; новая категория не скрыта,
// новый способ включён (как у расхода, З9-и4). cur — до правки или nil.
func templateRefs(t, cur *models.ExpenseTemplate) string {
	var n int64
	if db.Model(&models.ExpenseCategory{}).Where("id = ?", t.CategoryID).Count(&n); n == 0 {
		return "bad_category"
	}
	if t.CounterpartyID != nil {
		if db.Model(&models.Counterparty{}).Where("id = ?", *t.CounterpartyID).Count(&n); n == 0 {
			return "bad_counterparty"
		}
	}
	e, was := models.Expense{Method: t.Method, CategoryID: t.CategoryID}, (*models.Expense)(nil)
	if cur != nil {
		was = &models.Expense{Method: cur.Method, CategoryID: cur.CategoryID}
	}
	return expenseAllowed(&e, was)
}

// templateLine — шаблон строкой для журнала (чистая, тест).
func templateLine(t *models.ExpenseTemplate, category string) string {
	s := fmt.Sprintf("«%s» · %s · %s zł", t.Name, category, plnRU(models.PLNFromGrosz(t.GrossGrosz)))
	if t.VatGrosz > 0 {
		s += " (VAT " + plnRU(models.PLNFromGrosz(t.VatGrosz)) + ")"
	}
	return s + " · " + expenseMethods[t.Method] + fmt.Sprintf(" · %d-го числа", t.Day)
}

// templateChanges — «было → стало» для журнала правки шаблона (чистая, тест).
func templateChanges(a, b models.ExpenseTemplate, names map[string]string) []string {
	out := []string{}
	id := func(u *uuid.UUID) string {
		if u == nil {
			return "—"
		}
		return names[u.String()]
	}
	pln := func(g int64) string { return plnRU(models.PLNFromGrosz(g)) }
	if a.Name != b.Name {
		out = append(out, "«"+a.Name+"» → «"+b.Name+"»")
	}
	if a.CategoryID != b.CategoryID {
		out = append(out, "категория "+names[a.CategoryID.String()]+" → "+names[b.CategoryID.String()])
	}
	if id(a.CounterpartyID) != id(b.CounterpartyID) {
		out = append(out, "контрагент "+id(a.CounterpartyID)+" → "+id(b.CounterpartyID))
	}
	if a.GrossGrosz != b.GrossGrosz {
		out = append(out, "сумма "+pln(a.GrossGrosz)+" → "+pln(b.GrossGrosz)+" zł")
	}
	if a.VatGrosz != b.VatGrosz {
		out = append(out, "VAT "+pln(a.VatGrosz)+" → "+pln(b.VatGrosz))
	}
	if a.Method != b.Method {
		out = append(out, "способ "+expenseMethods[a.Method]+" → "+expenseMethods[b.Method])
	}
	if a.Day != b.Day {
		out = append(out, fmt.Sprintf("число %d → %d", a.Day, b.Day))
	}
	if a.Note != b.Note {
		out = append(out, "заметка изменена")
	}
	if a.Active != b.Active {
		out = append(out, map[bool]string{true: "включён", false: "выключен"}[b.Active])
	}
	return out
}

// filledMonths — какие месяцы шаблонов уже внесены (без сторно), с since.
func filledMonths(since time.Time) map[uuid.UUID]map[string]bool {
	var rows []struct {
		TemplateID    uuid.UUID
		TemplateMonth time.Time
	}
	db.Raw("SELECT template_id, template_month FROM expenses WHERE template_id IS NOT NULL AND storno_at IS NULL AND template_month >= ?",
		since.Format("2006-01-02")).Scan(&rows)
	out := map[uuid.UUID]map[string]bool{}
	for _, r := range rows {
		if out[r.TemplateID] == nil {
			out[r.TemplateID] = map[string]bool{}
		}
		out[r.TemplateID][r.TemplateMonth.Format("2006-01")] = true
	}
	return out
}

func lookbackStart(today time.Time) time.Time {
	return monthOf(today).AddDate(0, -(dueLookback - 1), 0)
}

func templateOut(t *models.ExpenseTemplate, names, cps map[string]string, today time.Time, filled map[string]bool) gin.H {
	row := gin.H{"id": t.ID, "name": t.Name, "category_id": t.CategoryID, "category": names[t.CategoryID.String()],
		"amount_pln": models.PLNFromGrosz(t.GrossGrosz), "vat_pln": models.PLNFromGrosz(t.VatGrosz), "method": t.Method,
		"day": t.Day, "note": t.Note, "active": t.Active, "from_month": t.FromMonth.Format("2006-01-02")}
	if t.CounterpartyID != nil {
		row["counterparty_id"], row["counterparty"] = *t.CounterpartyID, cps[t.CounterpartyID.String()]
	}
	if t.Active {
		row["next_due"] = nextDue(t.FromMonth, today, t.Day, filled).Format("2006-01-02")
		row["waiting"] = len(dueMonths(t.FromMonth, today, t.Day, filled))
		if m, ok := earlyMonth(t.FromMonth, today, t.Day, filled); ok { // З9-и5б: «Внести заранее»
			row["early"] = m.Format("2006-01-02")
		}
	}
	return row
}

// dueItem — месяц шаблона, который ждёт внесения.
type dueItem struct {
	T     *models.ExpenseTemplate
	Month time.Time
}

// dueList — что ждёт внесения сейчас: включённые шаблоны, месяцы со сроком ≤
// сегодня без расхода; старые сроки — первыми.
func dueList(today time.Time) []dueItem {
	var ts []models.ExpenseTemplate
	db.Where("active").Order("day, lower(name)").Find(&ts)
	if len(ts) == 0 {
		return nil
	}
	filled := filledMonths(lookbackStart(today))
	out := []dueItem{}
	for i := range ts {
		for _, m := range dueMonths(ts[i].FromMonth, today, ts[i].Day, filled[ts[i].ID]) {
			out = append(out, dueItem{&ts[i], m})
		}
	}
	sort.SliceStable(out, func(a, b int) bool {
		return dueDay(out[a].Month, out[a].T.Day).Before(dueDay(out[b].Month, out[b].T.Day))
	})
	return out
}

func dueOut(items []dueItem, today time.Time) []gin.H {
	names, cps := categoryNames(), counterpartyNames()
	out := make([]gin.H, 0, len(items))
	for _, d := range items {
		t, due := d.T, dueDay(d.Month, d.T.Day)
		row := gin.H{"template_id": t.ID, "name": t.Name, "category_id": t.CategoryID, "category": names[t.CategoryID.String()],
			"amount_pln": models.PLNFromGrosz(t.GrossGrosz), "vat_pln": models.PLNFromGrosz(t.VatGrosz), "method": t.Method,
			"note": t.Note, "month": d.Month.Format("2006-01-02"), "due": due.Format("2006-01-02"),
			"overdue_days": int(today.Sub(due).Hours()/24 + 0.5)}
		if t.CounterpartyID != nil {
			row["counterparty_id"], row["counterparty"] = *t.CounterpartyID, cps[t.CounterpartyID.String()]
		}
		out = append(out, row)
	}
	return out
}

// GET /admin/expense-templates — регулярные расходы: все, включённые первыми (owner).
func handleExpenseTemplates(c *gin.Context) {
	var ts []models.ExpenseTemplate
	db.Order("NOT active, day, lower(name)").Find(&ts)
	today := expenseToday()
	filled := filledMonths(lookbackStart(today))
	names, cps := categoryNames(), counterpartyNames()
	out := make([]gin.H, 0, len(ts))
	for i := range ts {
		out = append(out, templateOut(&ts[i], names, cps, today, filled[ts[i].ID]))
	}
	c.JSON(http.StatusOK, gin.H{"templates": out})
}

// GET /admin/expenses/due — что ждёт внесения (owner).
func handleExpensesDue(c *gin.Context) {
	today := expenseToday()
	items := dueOut(dueList(today), today)
	c.JSON(http.StatusOK, gin.H{"due": items, "count": len(items)})
}

// POST /admin/expense-templates — новый шаблон (owner): ждёт с ближайшего срока.
func handleExpenseTemplateCreate(c *gin.Context) {
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	code, t := checkTemplate(req, models.ExpenseTemplate{Active: true})
	if code == "" {
		code = templateRefs(&t, nil)
	}
	if code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	var same models.ExpenseTemplate
	if db.Where("lower(name) = lower(?)", t.Name).First(&same).Error == nil {
		c.JSON(http.StatusConflict, gin.H{"code": "template_exists", "message": expenseErrors["template_exists"], "id": same.ID})
		return
	}
	club, ok := defaultClub()
	if !ok {
		expenseFail(c, http.StatusConflict, "no_club")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	today := expenseToday()
	t.ClubID, t.CreatedBy, t.Active, t.FromMonth = club.ID, &adminID, true, firstDueMonth(today, t.Day)
	if err := db.Create(&t).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	names := categoryNames()
	logAdminAction(c, "expense_template_create", nil, templateLine(&t, names[t.CategoryID.String()]))
	c.JSON(http.StatusCreated, gin.H{"template": templateOut(&t, names, counterpartyNames(), today, nil)})
}

// PATCH /admin/expense-templates/:id — поправить, выключить или включить
// (owner). Включённый снова ждёт с ближайшего срока — пропущенные месяцы на
// него не вешаются.
func handleExpenseTemplateUpdate(c *gin.Context) {
	var cur models.ExpenseTemplate
	if err := db.First(&cur, "id = ?", c.Param("id")).Error; err != nil {
		expenseFail(c, http.StatusNotFound, "template_not_found")
		return
	}
	var req templateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		expenseFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	code, t := checkTemplate(req, cur)
	if code == "" {
		code = templateRefs(&t, &cur)
	}
	if code != "" {
		expenseFail(c, http.StatusBadRequest, code)
		return
	}
	if req.Active != nil {
		t.Active = *req.Active
	}
	if t.Name != cur.Name {
		var same models.ExpenseTemplate
		if db.Where("lower(name) = lower(?) AND id <> ?", t.Name, cur.ID).First(&same).Error == nil {
			c.JSON(http.StatusConflict, gin.H{"code": "template_exists", "message": expenseErrors["template_exists"], "id": same.ID})
			return
		}
	}
	names := categoryNames()
	for k, v := range counterpartyNames() {
		names[k] = v
	}
	changes := templateChanges(cur, t, names)
	today := expenseToday()
	if len(changes) == 0 {
		c.JSON(http.StatusOK, gin.H{"template": templateOut(&cur, categoryNames(), counterpartyNames(), today, filledMonths(lookbackStart(today))[cur.ID]), "changed": 0})
		return
	}
	upd := map[string]any{"name": t.Name, "category_id": t.CategoryID, "counterparty_id": t.CounterpartyID,
		"gross_grosz": t.GrossGrosz, "vat_grosz": t.VatGrosz, "method": t.Method, "day": t.Day, "note": t.Note,
		"active": t.Active, "updated_at": time.Now()}
	if t.Active && !cur.Active { // включили — с ближайшего срока
		upd["from_month"] = firstDueMonth(today, t.Day).Format("2006-01-02")
	}
	if err := db.Model(&models.ExpenseTemplate{}).Where("id = ?", cur.ID).Updates(upd).Error; err != nil {
		expenseFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	var fresh models.ExpenseTemplate
	db.First(&fresh, "id = ?", cur.ID)
	logAdminAction(c, "expense_template_update", nil, "«"+fresh.Name+"»: "+strings.Join(changes, "; "))
	c.JSON(http.StatusOK, gin.H{"template": templateOut(&fresh, categoryNames(), counterpartyNames(), today,
		filledMonths(lookbackStart(today))[fresh.ID]), "changed": len(changes)})
}

// expenseTemplateLink — расход по шаблону за месяц (для записи): шаблон есть,
// месяц — с первого месяца шаблона по следующий за текущим (заранее, З9-и5б),
// ещё не внесён. Возвращает код ошибки, шаблон и месяц.
func expenseTemplateLink(tx *gorm.DB, tid, month string, today time.Time) (string, *models.ExpenseTemplate, time.Time) {
	var t models.ExpenseTemplate
	id, err := uuid.Parse(strings.TrimSpace(tid))
	if err != nil || tx.First(&t, "id = ?", id).Error != nil {
		return "bad_template", nil, time.Time{}
	}
	m, err := time.ParseInLocation("2006-01-02", strings.TrimSpace(month), today.Location())
	if err != nil {
		if m, err = time.ParseInLocation("2006-01", strings.TrimSpace(month), today.Location()); err != nil {
			return "bad_template_month", nil, time.Time{}
		}
	}
	m = monthOf(m)
	if m.Before(monthIn(t.FromMonth, today.Location())) || m.After(monthOf(today).AddDate(0, 1, 0)) {
		return "bad_template_month", nil, time.Time{}
	}
	var n int64
	tx.Model(&models.Expense{}).Where("template_id = ? AND template_month = ? AND storno_at IS NULL", t.ID, m.Format("2006-01-02")).Count(&n)
	if n > 0 {
		return "template_entered", nil, time.Time{}
	}
	return "", &t, m
}

// dueMessage — напоминание в канал «только владельцу» (чистая, тест).
func dueMessage(t *models.ExpenseTemplate, due, today time.Time) string {
	when := "срок сегодня, " + due.Format("02.01")
	if due.Before(today) {
		when = "срок был " + due.Format("02.01")
	}
	return fmt.Sprintf("🧾 Пора внести регулярный расход «%s» — %s zł, %s; %s. «Бухгалтерия» → «Расходы» → «Ждут внесения» → «Внести»",
		t.Name, plnRU(models.PLNFromGrosz(t.GrossGrosz)), expenseMethods[t.Method], when)
}

// remindExpensesDue — одно напоминание на срок: включённые шаблоны, у которых
// срок этого месяца наступил, а расхода нет. Кто первым отметил reminded_for,
// тот и пишет — два прогона разом дважды не напомнят. Внесли раньше срока —
// месяц отмечается без сообщения.
func remindExpensesDue(now time.Time) int {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	m := monthOf(today)
	var ts []models.ExpenseTemplate
	db.Where("active AND from_month <= ? AND (reminded_for IS NULL OR reminded_for < ?)",
		m.Format("2006-01-02"), m.Format("2006-01-02")).Find(&ts)
	filled := filledMonths(m)
	n := 0
	for i := range ts {
		t := &ts[i]
		due := dueDay(m, t.Day)
		if due.After(today) && !filled[t.ID][monthKey(m)] {
			continue // срок впереди
		}
		res := db.Model(&models.ExpenseTemplate{}).Where("id = ? AND (reminded_for IS NULL OR reminded_for < ?)", t.ID, m.Format("2006-01-02")).
			Update("reminded_for", m.Format("2006-01-02"))
		if res.Error != nil || res.RowsAffected == 0 || filled[t.ID][monthKey(m)] {
			continue // напомнил другой прогон или уже внесли
		}
		postSystem(dueMessage(t, due, today), true, map[string]any{"expense_due": true, "template_id": t.ID})
		n++
	}
	return n
}

// POST /admin/expenses/due/remind — напомнить сейчас, не дожидаясь фона (owner):
// та же логика, что у джоба.
func handleExpensesDueRemind(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"reminded": remindExpensesDue(time.Now())})
}

// startExpenseDueJob — фоновый прогон раз в 15 минут, первый через минуту после
// старта: срок — число месяца, напоминание — в тот же день.
func startExpenseDueJob() {
	go func() {
		time.Sleep(time.Minute)
		for {
			safely("expenseDue", func() {
				if n := remindExpensesDue(time.Now()); n > 0 {
					log.Printf("регулярные расходы: напомнил владельцу — %d", n)
				}
			})
			time.Sleep(15 * time.Minute)
		}
	}()
}
