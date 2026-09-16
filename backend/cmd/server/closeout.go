package main

// Ж6-и4 «Закрытие смены с кассой» (ADMIN.md, трек Ж; ревью 14.09 №32,
// миграция 066). «Отметить уход» закрывал табель и молчал про деньги —
// расхождение в ящике всплывало через день, когда уже не восстановить, кто и
// что. Теперь уход идёт через пересчёт: сервер считает, сколько наличных
// должно быть по операциям ЭТОГО админа за клубные сутки, админ вбивает
// пересчитанное, разница ложится в shift_closeouts и уходит владельцу — в
// канал персонала, в сводку смены и в табель. Уход без пересчёта остаётся
// возможным и честно помечается.
//
// Ожидаемая наличность (только method=cash, живые деньги у стойки):
//   + депозиты, созданные админом в клубные сутки и не отменённые
//   + продажи (бар и расчёты кухни) с paid_at в сутках, деньги принял админ,
//     не отменённые — Ж6: долг, закрытый сегодня, лежит в ящике сегодня
//   + пакеты времени, проданные админом за нал в сутки, не отменённые
//   − наличные, выданные сегодня по отменённым операциям ПРОШЛЫХ суток
//     (отмена сегодняшней операции сама выводит её из плюса)
// Кошельком, картой и BLIK ящик не трогают; пакеты возвращаются на кошелёк,
// а не наличными (Е2), поэтому в минусе их нет.

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// cashExpectation — ожидаемая наличность и её составляющие, гроши.
type cashExpectation struct {
	Deposits int64
	Goods    int64
	Packs    int64
	Refunds  int64
}

func (e cashExpectation) Total() int64 { return e.Deposits + e.Goods + e.Packs - e.Refunds }

func (e cashExpectation) out() gin.H {
	return gin.H{
		"expected_pln": models.PLNFromGrosz(e.Total()),
		"deposits_pln": models.PLNFromGrosz(e.Deposits),
		"goods_pln":    models.PLNFromGrosz(e.Goods),
		"packs_pln":    models.PLNFromGrosz(e.Packs),
		"refunds_pln":  models.PLNFromGrosz(e.Refunds),
	}
}

// expectedCash — наличность за окно [from, to): adminID пустой — по всем
// (справочная цифра «всего за сутки»).
func expectedCash(adminID *uuid.UUID, from, to time.Time) cashExpectation {
	var e cashExpectation
	var pln float64

	dq := db.Model(&models.Deposit{}).Select("COALESCE(SUM(amount_pln),0)").
		Where("method = 'cash' AND created_at >= ? AND created_at < ? AND voided_at IS NULL", from, to)
	if adminID != nil {
		dq = dq.Where("created_by = ?", *adminID)
	}
	dq.Scan(&pln)
	e.Deposits = models.GroszFromPLN(pln)

	pln = 0
	sq := db.Model(&models.Sale{}).Select("COALESCE(SUM(total_pln),0)").
		Where("method = 'cash' AND paid_at >= ? AND paid_at < ? AND voided_at IS NULL", from, to)
	if adminID != nil {
		sq = sq.Where("COALESCE(paid_by, created_by) = ?", *adminID)
	}
	sq.Scan(&pln)
	e.Goods = models.GroszFromPLN(pln)

	pln = 0
	pq := db.Model(&models.UserPackage{}).Select("COALESCE(SUM(price_pln),0)").
		Where("method = 'cash' AND created_at >= ? AND created_at < ? AND voided_at IS NULL", from, to)
	if adminID != nil {
		pq = pq.Where("created_by = ?", *adminID)
	}
	pq.Scan(&pln)
	e.Packs = models.GroszFromPLN(pln)

	// возвраты наличными по операциям прошлых суток, отменённым в этом окне
	var refund int64
	rq := db.Model(&models.Deposit{}).Select("COALESCE(SUM(refund_grosz),0)").
		Where("method = 'cash' AND voided_at >= ? AND voided_at < ? AND created_at < ?", from, to, from)
	if adminID != nil {
		rq = rq.Where("voided_by = ?", *adminID)
	}
	rq.Scan(&refund)
	e.Refunds = refund
	pln = 0
	rs := db.Model(&models.Sale{}).Select("COALESCE(SUM(total_pln),0)").
		Where("method = 'cash' AND voided_at >= ? AND voided_at < ? AND paid_at IS NOT NULL AND paid_at < ?", from, to, from)
	if adminID != nil {
		rs = rs.Where("voided_by = ?", *adminID)
	}
	rs.Scan(&pln)
	e.Refunds += models.GroszFromPLN(pln)
	return e
}

// closeoutDiff — расхождение (чистая, тест): пересчитано минус ожидалось.
func closeoutDiff(expected, counted int64) int64 { return counted - expected }

// closeoutMessage — текст владельцу в канал персонала (чистая, тест). Пусто —
// сообщать нечего: касса сошлась.
func closeoutMessage(expected int64, counted *int64, note string) string {
	var b strings.Builder
	switch {
	case counted == nil:
		fmt.Fprintf(&b, "🧮 Сдал смену без пересчёта кассы (по операциям должно быть %.2f zł)", models.PLNFromGrosz(expected))
	case closeoutDiff(expected, *counted) == 0:
		return ""
	default:
		d := closeoutDiff(expected, *counted)
		sign := "−"
		if d > 0 {
			sign = "+"
		}
		if d < 0 {
			d = -d
		}
		fmt.Fprintf(&b, "🧮 Сдал смену: в ящике %.2f zł, должно быть %.2f zł — расхождение %s%.2f zł",
			models.PLNFromGrosz(*counted), models.PLNFromGrosz(expected), sign, models.PLNFromGrosz(d))
	}
	if n := strings.TrimSpace(note); n != "" {
		b.WriteString(". Передал смену: " + n)
	}
	return b.String()
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
	return row
}

// GET /admin/work/closeout — «сколько должно быть в ящике» перед уходом
// (staff, за себя): ожидаемая наличность этого админа за текущие клубные
// сутки с разбивкой, и справочно — всего за сутки по всем.
func handleWorkCloseoutPreview(c *gin.Context) {
	adminID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": "bad_token", "message": "Не разобрал токен"})
		return
	}
	now := time.Now()
	reportHour := int(settingInt64("report_hour", 8))
	from, _, key, _ := shiftWindow("", reportHour, now)
	mine := expectedCash(&adminID, from, now)
	all := expectedCash(nil, from, now)
	resp := mine.out()
	resp["date"], resp["from"], resp["club_expected_pln"] = key, from, models.PLNFromGrosz(all.Total())
	resp["on_shift"] = openEntryFor(adminID) != nil
	c.JSON(http.StatusOK, resp)
}

// recordCloseout — записать сдачу кассы при уходе и известить владельца.
// countedPLN nil = ушёл без пересчёта. Возвращает запись и текст сообщения.
func recordCloseout(c *gin.Context, adminID uuid.UUID, entry *models.WorkEntry, countedPLN *float64, note string, now time.Time) *models.ShiftCloseout {
	reportHour := int(settingInt64("report_hour", 8))
	from, _, _, _ := shiftWindow("", reportHour, now)
	e := expectedCash(&adminID, from, now)
	club, ok := defaultClub()
	if !ok {
		return nil
	}
	co := models.ShiftCloseout{
		ClubID: club.ID, UserID: adminID, Date: clubDayOf(now, reportHour),
		ExpectedGrosz: e.Total(), DepositsGrosz: e.Deposits, GoodsGrosz: e.Goods,
		PacksGrosz: e.Packs, RefundsGrosz: e.Refunds, Note: strings.TrimSpace(note), Skipped: countedPLN == nil,
	}
	if entry != nil {
		id := entry.ID
		co.WorkEntryID = &id
	}
	if countedPLN != nil {
		counted := models.GroszFromPLN(*countedPLN)
		diff := closeoutDiff(co.ExpectedGrosz, counted)
		co.CountedGrosz, co.DiffGrosz = &counted, &diff
	}
	if err := db.Create(&co).Error; err != nil {
		return nil
	}
	details := fmt.Sprintf("касса: должно быть %.2f zł", models.PLNFromGrosz(co.ExpectedGrosz))
	if co.CountedGrosz != nil {
		details += fmt.Sprintf(", пересчитано %.2f zł, расхождение %+.2f zł",
			models.PLNFromGrosz(*co.CountedGrosz), models.PLNFromGrosz(*co.DiffGrosz))
	} else {
		details += " · без пересчёта"
	}
	if co.Note != "" {
		details += " · " + co.Note
	}
	logAdminAction(c, "shift_close", nil, details)

	// владельцу — в канал персонала, от имени сдавшего (это его отчёт о смене)
	if msg := closeoutMessage(co.ExpectedGrosz, co.CountedGrosz, co.Note); msg != "" {
		m := models.StaffMessage{ClubID: club.ID, AuthorID: adminID, Role: c.GetString("user_role"), Text: msg}
		if err := db.Create(&m).Error; err == nil {
			db.Model(&models.User{}).Where("id = ?", adminID).Update("staff_chat_read_at", m.CreatedAt)
			hub.AdminBroadcast("staff_chat", map[string]any{"kind": "message", "author_id": adminID.String(), "role": m.Role, "closeout": true})
		}
	}
	return &co
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
