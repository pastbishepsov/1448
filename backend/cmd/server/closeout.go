package main

// Ж6-и4 «Закрытие смены с кассой» (ADMIN.md, трек Ж; ревью 14.09 №32,
// миграция 066). «Отметить уход» закрывал табель и молчал про деньги —
// расхождение в ящике всплывало через день, когда уже не восстановить, кто и
// что. Теперь уход идёт через пересчёт: сервер считает, сколько наличных
// должно быть по операциям ЭТОГО админа за его смену, админ вбивает
// пересчитанное, разница ложится в shift_closeouts и уходит владельцу — в
// канал персонала, в сводку смены и в табель. Уход без пересчёта остаётся
// возможным и честно помечается.
//
// З1-и1 (OWNER.md, №1 «а»): окно — смена, а не клубные сутки. Ночная смена,
// сданная в 08:05, получала «должно быть 0» и ложный излишек, вторая сдача за
// сутки включала деньги первой (ложная недостача), сдача датировалась днём
// нажатия и пропадала из сводки своей смены, а «сошлось» владельцу не писалось
// вовсе. Теперь окно — от начала записи табеля (или от прошлой сдачи этого
// админа, если она позже) до «сейчас», дата сдачи — дата записи табеля.
//
// З1-и2 (№59, Д4): деньги сдачи — должно, пересчитано, расхождение — уходят в
// канал служебной строкой «только владельцу»; заметка «передал смену» — от
// сдавшего, её видят все. Строка журнала shift_close с суммами — тоже только
// владельцу (audit.go).
//
// З8-и1 (З-Р1, миграция 070): «должно быть в ящике» — ящик клуба, а не
// наличные одного админа: остаток прошлой сдачи + всё, что пришло наличными с
// неё, кто бы ни провёл, − наличные возвраты − инкассации ± коррекции (счёт —
// drawer.go). Первая сдача после обновления — нулевая точка. «Из них провёл
// ты» — справка; дата сдачи и сообщения — как в З1.

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// closeoutFrom — начало окна «должно быть в ящике» (З1-и1, чистая, тест): от
// начала смены (записи табеля) или от прошлой сдачи этого админа, если она
// позже; без открытой смены — от начала клубных суток, но не раньше прошлой сдачи.
func closeoutFrom(entryStart, lastCloseout *time.Time, dayStart time.Time) time.Time {
	from := dayStart
	if entryStart != nil {
		from = *entryStart
	}
	if lastCloseout != nil && lastCloseout.After(from) {
		from = *lastCloseout
	}
	return from
}

// closeoutWindow — окно сдачи для админа сейчас и дата, которой она ляжет:
// дата записи табеля (ночная смена — вчерашняя), без смены — клубные сутки.
// З8-и1: «должно быть» считает ящик (drawer.go) — отсюда берётся только дата.
func closeoutWindow(adminID uuid.UUID, entry *models.WorkEntry, now time.Time) (from, day time.Time) {
	reportHour := int(settingInt64("report_hour", 8))
	dayStart, _, _, _ := shiftWindow("", reportHour, now)
	day = clubDayOf(now, reportHour)
	var start, lastAt *time.Time
	if entry != nil {
		start, day = &entry.StartedAt, entry.Date
	}
	var last models.ShiftCloseout
	if db.Where("user_id = ? AND created_at <= ?", adminID, now).Order("created_at DESC").First(&last).Error == nil {
		lastAt = &last.CreatedAt
	}
	return closeoutFrom(start, lastAt, dayStart), day
}

// closeoutDiff — расхождение (чистая, тест): пересчитано минус ожидалось.
func closeoutDiff(expected, counted int64) int64 { return counted - expected }

// closeoutMessage — денежная строка сдачи для владельца (чистая, тест). З1-и1:
// «сошлось» тоже строка с суммой — иначе оно неотличимо от «вбил ожидаемую
// цифру». З1-и2: пишет система, поэтому в тексте — кто сдавал; заметки здесь
// нет, она уходит всем отдельно (closeoutNote).
func closeoutMessage(nick string, expected int64, counted *int64) string {
	if nick == "" {
		nick = "Сотрудник"
	}
	var b strings.Builder
	switch {
	case counted == nil:
		fmt.Fprintf(&b, "🧮 %s сдал смену без пересчёта кассы (в ящике должно быть %.2f zł)", nick, models.PLNFromGrosz(expected))
	case closeoutDiff(expected, *counted) == 0:
		fmt.Fprintf(&b, "🧮 %s сдал смену: касса сошлась — в ящике %.2f zł", nick, models.PLNFromGrosz(*counted))
	default:
		d := closeoutDiff(expected, *counted)
		sign := "−"
		if d > 0 {
			sign = "+"
		}
		if d < 0 {
			d = -d
		}
		fmt.Fprintf(&b, "🧮 %s сдал смену: в ящике %.2f zł, должно быть %.2f zł — расхождение %s%.2f zł",
			nick, models.PLNFromGrosz(*counted), models.PLNFromGrosz(expected), sign, models.PLNFromGrosz(d))
	}
	return b.String()
}

// closeoutNote — заметка «передал смену» для всех (чистая, тест); пустая — "".
func closeoutNote(note string) string {
	if n := strings.TrimSpace(note); n != "" {
		return "Передал смену: " + n
	}
	return ""
}

func closeoutOut(co *models.ShiftCloseout) gin.H {
	row := gin.H{
		"id": co.ID, "user_id": co.UserID, "date": co.Date.Format("2006-01-02"), "created_at": co.CreatedAt,
		"expected_pln": models.PLNFromGrosz(co.ExpectedGrosz), "deposits_pln": models.PLNFromGrosz(co.DepositsGrosz),
		"goods_pln": models.PLNFromGrosz(co.GoodsGrosz), "packs_pln": models.PLNFromGrosz(co.PacksGrosz),
		"refunds_pln": models.PLNFromGrosz(co.RefundsGrosz), "skipped": co.Skipped, "note": co.Note,
	}
	if co.WorkEntryID != nil {
		row["work_entry_id"] = co.WorkEntryID
	}
	if co.CountedGrosz != nil {
		row["counted_pln"] = models.PLNFromGrosz(*co.CountedGrosz)
	}
	if co.DiffGrosz != nil {
		row["diff_pln"] = models.PLNFromGrosz(*co.DiffGrosz)
	}
	row["baseline"] = co.Baseline
	if co.OpeningGrosz != nil { // З8-и1: сдача модели ящика
		row["opening_pln"], row["in_pln"] = models.PLNFromGrosz(*co.OpeningGrosz), models.PLNFromGrosz(co.InGrosz)
		row["out_pln"], row["adjust_pln"] = models.PLNFromGrosz(co.OutGrosz), models.PLNFromGrosz(co.AdjustGrosz)
		row["collections_pln"] = models.PLNFromGrosz(co.OutGrosz - co.RefundsGrosz)
	}
	return row
}

// GET /admin/work/closeout — «сколько должно быть в ящике» перед уходом
// (staff): ящик клуба (З8-и1) — остаток на начало, кто и когда его оставил,
// что пришло и ушло с тех пор; «из них провёл ты» — справкой. baseline —
// ящика ещё нет: эта сдача — нулевая точка, ожидаемого нет.
func handleWorkCloseoutPreview(c *gin.Context) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	now := time.Now()
	open := openEntryFor(adminID)
	_, day := closeoutWindow(adminID, open, now)
	st := drawerAt(db, now)
	resp := st.Flow.out()
	resp["date"], resp["on_shift"], resp["baseline"] = day.Format("2006-01-02"), open != nil, st.Point == nil
	resp["expected_pln"] = models.PLNFromGrosz(st.Expected)
	if p := st.Point; p != nil {
		resp["from"], resp["opening_pln"] = p.At, models.PLNFromGrosz(p.Closing)
		resp["opening_by"] = nicknamesByID([]string{p.By.String()})[p.By.String()]
		resp["opening_counted"], resp["opening_baseline"] = p.Counted, p.Baseline
		resp["mine"] = drawerFlowFor(db, &adminID, p.At, now).out()
	}
	c.JSON(http.StatusOK, resp)
}

// recordCloseout — записать сдачу кассы при уходе и известить владельца.
// countedPLN nil = ушёл без пересчёта. З8-и1: сдачи — под замком по одной,
// точка отсчёта и движение берутся в той же транзакции.
func recordCloseout(c *gin.Context, adminID uuid.UUID, entry *models.WorkEntry, countedPLN *float64, note string, now time.Time) *models.ShiftCloseout {
	_, day := closeoutWindow(adminID, entry, now) // З1-и1: дата — день записи табеля
	club, ok := defaultClub()
	if !ok {
		return nil
	}
	co := models.ShiftCloseout{ClubID: club.ID, UserID: adminID, Date: day, Note: strings.TrimSpace(note), Skipped: countedPLN == nil}
	if entry != nil {
		id := entry.ID
		co.WorkEntryID = &id
	}
	var counted *int64
	if countedPLN != nil {
		g := models.GroszFromPLN(*countedPLN)
		counted = &g
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", drawerLock).Error; err != nil {
			return err
		}
		at := time.Now() // под замком: следующая сдача начнёт ровно отсюда
		if at.Before(now) {
			at = now
		}
		drawerCloseout(&co, drawerAt(tx, at), counted)
		co.CreatedAt = at
		return tx.Create(&co).Error
	})
	if err != nil {
		log.Printf("сдача кассы не записалась: %v", err)
		return nil
	}
	logAdminAction(c, "shift_close", nil, closeoutDetails(&co)) // З1-и2: строку видит только владелец (hiddenFromAdmin)

	// З1-и2 (Д4): заметка — от сдавшего, всем; деньги — служебной строкой только
	// владельцу. Заметка первой: у владельца внизу ленты остаётся строка с деньгами.
	if note := closeoutNote(co.Note); note != "" {
		m := models.StaffMessage{ClubID: club.ID, AuthorID: &adminID, Role: c.GetString("user_role"), Text: note}
		if err := postStaffMessage(&m, map[string]any{"closeout": true}); err != nil {
			log.Printf("сдача кассы: заметка в канал не записалась: %v", err)
		}
	}
	nick := nicknamesByID([]string{adminID.String()})[adminID.String()]
	postSystem(closeoutOwnerLine(nick, &co), true, map[string]any{"closeout": true})
	return &co
}

// closeoutOwnerLine — денежная строка сдачи владельцу (чистая, тест): нулевая
// точка, сдача без ящика или обычная «сошлось / расхождение / без пересчёта».
func closeoutOwnerLine(nick string, co *models.ShiftCloseout) string {
	switch {
	case co.Baseline && co.CountedGrosz != nil:
		return drawerStartMessage(nick, *co.CountedGrosz)
	case co.OpeningGrosz == nil:
		if nick == "" {
			nick = "Сотрудник"
		}
		return "🧮 " + nick + " сдал смену без пересчёта кассы — начального остатка ящика ещё нет, его задаст первая сдача с пересчётом"
	}
	return closeoutMessage(nick, co.ExpectedGrosz, co.CountedGrosz)
}

// closeoutDetails — строка журнала shift_close (чистая, тест): из чего ящик.
func closeoutDetails(co *models.ShiftCloseout) string {
	pln := models.PLNFromGrosz
	var b strings.Builder
	switch {
	case co.Baseline && co.CountedGrosz != nil:
		fmt.Fprintf(&b, "касса: начальный остаток ящика %.2f zł", pln(*co.CountedGrosz))
	case co.OpeningGrosz == nil:
		b.WriteString("касса: ящик ещё не заведён")
	default:
		fmt.Fprintf(&b, "касса: должно быть %.2f zł (на начало %.2f + пришло %.2f − ушло %.2f",
			pln(co.ExpectedGrosz), pln(*co.OpeningGrosz), pln(co.InGrosz), pln(co.OutGrosz))
		if co.AdjustGrosz != 0 {
			fmt.Fprintf(&b, ", коррекции %+.2f", pln(co.AdjustGrosz))
		}
		b.WriteString(")")
		if co.CountedGrosz != nil {
			fmt.Fprintf(&b, ", пересчитано %.2f zł, расхождение %+.2f zł", pln(*co.CountedGrosz), pln(*co.DiffGrosz))
		}
	}
	if co.CountedGrosz == nil {
		b.WriteString(" · без пересчёта")
	}
	if co.Note != "" {
		b.WriteString(" · " + co.Note)
	}
	return b.String()
}

// closeoutsForDay — сдачи касс за клубные сутки (сводка смены владельца).
func closeoutsForDay(day string) []gin.H {
	var list []models.ShiftCloseout
	db.Where("date = ?", day).Order("created_at").Find(&list)
	if len(list) == 0 {
		return []gin.H{}
	}
	ids := make([]string, 0, len(list))
	for _, co := range list {
		ids = append(ids, co.UserID.String())
	}
	nick := nicknamesByID(ids)
	out := make([]gin.H, 0, len(list))
	for i := range list {
		row := closeoutOut(&list[i])
		row["nickname"] = nick[list[i].UserID.String()]
		out = append(out, row)
	}
	return out
}

// closeoutsByEntry — сдачи касс по записям табеля (табель владельца).
func closeoutsByEntry(entries []models.WorkEntry) map[string]gin.H {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, 0, len(entries))
	for _, w := range entries {
		ids = append(ids, w.ID)
	}
	var list []models.ShiftCloseout
	db.Where("work_entry_id IN ?", ids).Find(&list)
	out := make(map[string]gin.H, len(list))
	for i := range list {
		if list[i].WorkEntryID != nil {
			out[list[i].WorkEntryID.String()] = closeoutOut(&list[i])
		}
	}
	return out
}
