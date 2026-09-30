package main

// З8-и1 «Модель ящика» (OWNER.md, трек З; З-Р1; ревью 23.09 №1 и №16,
// миграция 070).
//
// Касса считала наличные ЭТОГО админа за его смену, а ящик у клуба один:
// пополнение от владельца в смену админа, возврат наличными по чужой продаже,
// размен и вторая сдача за сутки давали ложные излишки и недостачи. Теперь
// «должно быть в ящике» — ящик клуба:
//
//	остаток прошлой сдачи (пересчитанный, если пересчитывали; без пересчёта —
//	ожидавшийся)
//	+ пришло наличными с неё: пополнения, товары и кухня (по моменту оплаты),
//	  пакеты — кто бы их ни провёл
//	− возвраты наличными по отменам, сделанным с неё: депозита (что забрали с
//	  кошелька), продажи, пакета (неотыгранная доля; до З11 возврат — тем же
//	  способом, что и оплата)
//	− инкассации ± коррекции владельца (cash_events; ручки — З8-и2)
//	− расходы из кассы (З9-и3а): по моменту, когда ушли из ящика (drawer_at)
//	− возвраты владельцу из кассы (З9-и3б): по минуте записи, отменённые — вне
//
// Каждая операция ложится в период по своему моменту: продажа и её отмена в
// одном периоде гасят друг друга, отмена вчерашней продажи — минус сегодня.
// Отменённое событие ящика не считается вовсе: его не было.
//
// Точка отсчёта — последняя сдача с остатком на начало (opening_grosz). Сдачи
// до модели ящика точкой не служат; пока ящика нет, сдача с пересчётом —
// нулевая точка (решение основателя 26.09): пересчитанное ложится остатком на
// начало без расхождения, владельцу — строка «Ящик: начальный остаток».
// «Из них провёл ты» — справка тем же счётом по операциям одного человека.

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// drawerLock — ключ pg_advisory_xact_lock: сдачи идут по одной, иначе две
// одновременные взяли бы одну точку отсчёта и посчитали период дважды.
const drawerLock int64 = 14480801

// drawerFlow — движение наличных ящика за период, гроши.
type drawerFlow struct {
	Deposits, Goods, Packs int64 // пришло: пополнения, товары и кухня, пакеты
	Refunds                int64 // ушло: возвраты наличными по отменам
	Collections            int64 // ушло: инкассации
	Expenses               int64 // ушло: расходы из кассы (З9-и3а)
	Owner                  int64 // ушло: возвраты владельцу из кассы (З9-и3б)
	Adjust                 int64 // ± коррекции владельца
}

func (f drawerFlow) In() int64  { return f.Deposits + f.Goods + f.Packs }
func (f drawerFlow) Out() int64 { return f.Refunds + f.Collections + f.Expenses + f.Owner }

func (f drawerFlow) out() gin.H {
	pln := models.PLNFromGrosz
	return gin.H{
		"in_pln": pln(f.In()), "out_pln": pln(f.Out()), "adjust_pln": pln(f.Adjust),
		"deposits_pln": pln(f.Deposits), "goods_pln": pln(f.Goods), "packs_pln": pln(f.Packs),
		"refunds_pln": pln(f.Refunds), "collections_pln": pln(f.Collections), "expenses_pln": pln(f.Expenses),
		"owner_pln": pln(f.Owner),
	}
}

// drawerExpected — «должно быть в ящике» (чистая, тест).
func drawerExpected(opening int64, f drawerFlow) int64 {
	return opening + f.In() - f.Out() + f.Adjust
}

// drawerClosing — с чем ящик остался после сдачи (чистая, тест): пересчитанное,
// если пересчитывали, иначе — ожидавшееся.
func drawerClosing(expected int64, counted *int64) int64 {
	if counted != nil {
		return *counted
	}
	return expected
}

// drawerPoint — точка отсчёта ящика: последняя сдача модели ящика.
type drawerPoint struct {
	At       time.Time
	Closing  int64 // остаток после неё — остаток на начало следующей
	By       uuid.UUID
	Counted  bool // пересчитали (иначе остаток — по расчёту)
	Baseline bool // нулевая точка
}

// drawerState — ящик на момент: точка отсчёта (nil — ящик ещё не заведён),
// движение с неё и «должно быть».
type drawerState struct {
	Point    *drawerPoint
	Flow     drawerFlow
	Expected int64
}

func lastDrawerPoint(q *gorm.DB) *drawerPoint {
	var co models.ShiftCloseout
	if q.Where("opening_grosz IS NOT NULL").Order("created_at DESC").First(&co).Error != nil {
		return nil
	}
	return &drawerPoint{At: co.CreatedAt, Closing: drawerClosing(co.ExpectedGrosz, co.CountedGrosz),
		By: co.UserID, Counted: co.CountedGrosz != nil, Baseline: co.Baseline}
}

// drawerAt — ящик на момент now.
func drawerAt(q *gorm.DB, now time.Time) drawerState {
	st := drawerState{Point: lastDrawerPoint(q)}
	if st.Point != nil {
		st.Flow = drawerFlowFor(q, nil, st.Point.At, now)
		st.Expected = drawerExpected(st.Point.Closing, st.Flow)
	}
	return st
}

// drawerFlowFor — наличные клуба за [from, to); actor — только его операции
// («из них провёл ты»). Суммы — в SQL и сразу в грошах: decimal zł через float
// не гоняем.
func drawerFlowFor(q *gorm.DB, actor *uuid.UUID, from, to time.Time) drawerFlow {
	sum := func(query, actorCol string) int64 {
		args := []any{from, to}
		if actor != nil {
			query += " AND " + actorCol + " = ?"
			args = append(args, *actor)
		}
		var n int64
		q.Raw(query, args...).Scan(&n)
		return n
	}
	grosz := func(col string) string { return "SELECT COALESCE(ROUND(SUM(" + col + ") * 100), 0)::bigint " }
	const refund = "SELECT COALESCE(SUM(refund_grosz), 0)::bigint "
	const event = "SELECT COALESCE(SUM(amount_grosz), 0)::bigint FROM cash_events WHERE voided_at IS NULL "
	return drawerFlow{
		Deposits: sum(grosz("amount_pln")+"FROM deposits WHERE method = 'cash' AND created_at >= ? AND created_at < ?", "created_by"),
		Goods: sum(grosz("total_pln")+"FROM sales WHERE method = 'cash' AND paid_at >= ? AND paid_at < ?",
			"COALESCE(paid_by, created_by)"),
		Packs: sum(grosz("price_pln")+"FROM user_packages WHERE method = 'cash' AND created_at >= ? AND created_at < ?", "created_by"),
		// З11-и1: возврат из ящика — только наличный (как вернули; до З11 — как платили)
		Refunds: sum(refund+"FROM deposits WHERE COALESCE(refund_method, method) = 'cash' AND voided_at >= ? AND voided_at < ?", "voided_by") +
			sum(grosz("total_pln")+"FROM sales WHERE COALESCE(refund_method, method) = 'cash' AND paid_at IS NOT NULL AND voided_at >= ? AND voided_at < ?",
				"voided_by") +
			sum(refund+"FROM user_packages WHERE COALESCE(refund_method, method) = 'cash' AND voided_at >= ? AND voided_at < ?", "voided_by"),
		Collections: sum(event+"AND kind = 'collect' AND created_at >= ? AND created_at < ?", "created_by"),
		// З9-и3а: расход из кассы — в момент, когда ушёл из ящика (drawer_at); сторно — вне ящика
		Expenses: sum("SELECT COALESCE(SUM(gross_grosz), 0)::bigint FROM expenses WHERE storno_at IS NULL AND drawer_at >= ? AND drawer_at < ?", "created_by"),
		// З9-и3б: возврат владельцу из кассы — в минуту записи; отменённый — вне ящика
		Owner:  sum("SELECT COALESCE(SUM(amount_grosz), 0)::bigint FROM owner_repayments WHERE method = 'till' AND voided_at IS NULL AND created_at >= ? AND created_at < ?", "created_by"),
		Adjust: sum(event+"AND kind = 'correct' AND created_at >= ? AND created_at < ?", "created_by"),
	}
}

// drawerCloseout — чем сдача ложится в ящик (чистая, тест). Ящик есть:
// остаток на начало — с прошлой сдачи, ожидаемое — по движению, расхождение —
// пересчитанное минус ожидаемое. Ящика нет: с пересчётом — нулевая точка
// (пересчитанное = остаток на начало = ожидаемое, расхождения нет), без
// пересчёта — сдача вне ящика, точкой отсчёта она не служит.
func drawerCloseout(co *models.ShiftCloseout, st drawerState, counted *int64) {
	switch {
	case st.Point == nil && counted == nil:
	case st.Point == nil:
		opening, zero := *counted, int64(0)
		co.OpeningGrosz, co.ExpectedGrosz, co.Baseline = &opening, opening, true
		co.CountedGrosz, co.DiffGrosz = counted, &zero
	default:
		opening, f := st.Point.Closing, st.Flow
		co.OpeningGrosz, co.ExpectedGrosz = &opening, st.Expected
		co.InGrosz, co.OutGrosz, co.AdjustGrosz = f.In(), f.Out(), f.Adjust
		co.DepositsGrosz, co.GoodsGrosz, co.PacksGrosz, co.RefundsGrosz = f.Deposits, f.Goods, f.Packs, f.Refunds
		co.ExpensesGrosz, co.OwnerGrosz = f.Expenses, f.Owner
		if counted != nil {
			diff := closeoutDiff(st.Expected, *counted)
			co.CountedGrosz, co.DiffGrosz = counted, &diff
		}
	}
}

// drawerStartMessage — строка владельцу о нулевой точке (чистая, тест).
func drawerStartMessage(nick string, counted int64) string {
	if nick == "" {
		nick = "сотрудник"
	}
	return fmt.Sprintf("🧮 Ящик: начальный остаток %.2f zł, пересчитал %s — дальше касса считает ящик от этой суммы",
		models.PLNFromGrosz(counted), nick)
}

// ── З8-и2: инкассация и коррекция ─────────────────────────────────────────
//
// Инкассация (№64 «б») — деньги вынули из ящика в банк или сейф: вносит админ
// (обязательно с заметкой «по чьему указанию», Д11) или владелец; за админа
// владельцу — служебная строка в канал. Коррекция «в ящике сейчас N zł» —
// только владелец и с причиной: ожидаемое становится N, в событии — названная
// сумма и поправка к ожидаемому. Отменяет событие только владелец; строка
// остаётся с пометкой. Инкассацию в журнале видят и админы — деньги ушли из их
// ящика; коррекцию и отмену — только владелец (audit.go).

const (
	maxCashPLN  = 1_000_000 // как у пересчёта при сдаче
	maxCashNote = 200
)

var errNoDrawer = errors.New("no_drawer")

var cashEventErrors = map[string]string{
	"invalid_request": "Нужны вид события, сумма и заметка",
	"bad_kind":        "Вид события: collect — инкассация, correct — коррекция",
	"bad_amount":      "Сумма — от 0,01 до 1 000 000 zł, не больше двух знаков после запятой",
	"bad_stated":      "Сколько в ящике — от 0 до 1 000 000 zł, не больше двух знаков после запятой",
	"bad_dest":        "Куда: bank — в банк, safe — в сейф",
	"need_note":       "Напиши, по чьему указанию — без этого инкассация не запишется",
	"need_reason":     "У коррекции нужна причина",
	"bad_note":        "Заметка — до 200 символов",
	"owner_only":      "Коррекцию остатка делает владелец",
	"no_drawer":       "Ящика ещё нет — начальный остаток задаст первая сдача с пересчётом",
	"no_club":         "Клуб не настроен",
	"event_not_found": "Событие ящика не найдено",
	"already_void":    "Событие уже отменено",
	"db_error":        "Не удалось записать — попробуй ещё раз",
}

func cashFail(c *gin.Context, status int, code string) {
	c.JSON(status, gin.H{"code": code, "message": cashEventErrors[code]})
}

type cashEventRequest struct {
	Kind      string   `json:"kind"`
	AmountPLN *float64 `json:"amount_pln"` // инкассация: сколько вынули
	StatedPLN *float64 `json:"stated_pln"` // коррекция: сколько в ящике сейчас
	Dest      string   `json:"dest"`       // инкассация: bank | safe
	Note      string   `json:"note"`
}

// checkCashEvent — что не так с событием ящика (чистая, тест); "" — можно писать.
func checkCashEvent(req cashEventRequest, owner bool) string {
	note := strings.TrimSpace(req.Note)
	switch req.Kind {
	case models.CashEventCollect:
		if req.AmountPLN == nil || !validatePLN(*req.AmountPLN, maxCashPLN) {
			return "bad_amount"
		}
		if req.Dest != "bank" && req.Dest != "safe" {
			return "bad_dest"
		}
		if !owner && note == "" {
			return "need_note"
		}
	case models.CashEventCorrect:
		if !owner {
			return "owner_only"
		}
		if req.StatedPLN == nil || !(*req.StatedPLN == 0 || validatePLN(*req.StatedPLN, maxCashPLN)) {
			return "bad_stated"
		}
		if note == "" {
			return "need_reason"
		}
	default:
		return "bad_kind"
	}
	if len([]rune(note)) > maxCashNote {
		return "bad_note"
	}
	return ""
}

var cashDestRU = map[string]string{"bank": "в банк", "safe": "в сейф"}

// cashEventWhat — суть события без вида (чистая, тест): «300.00 zł в сейф» или
// «в ящике 250.00 zł (поправка -150.00)». Вид уже назван меткой строки журнала.
func cashEventWhat(e *models.CashEvent) string {
	if e.Kind == models.CashEventCorrect && e.StatedGrosz != nil {
		return fmt.Sprintf("в ящике %.2f zł (поправка %+.2f)", models.PLNFromGrosz(*e.StatedGrosz), models.PLNFromGrosz(e.AmountGrosz))
	}
	what := fmt.Sprintf("%.2f zł", models.PLNFromGrosz(e.AmountGrosz))
	if e.Dest != nil {
		what += " " + cashDestRU[*e.Dest]
	}
	return what
}

// cashEventText — событие словами с видом и заметкой (чистая, тест): строка
// журнала об отмене — там вид события метка не называет.
func cashEventText(e *models.CashEvent) string {
	kind := "инкассация "
	if e.Kind == models.CashEventCorrect {
		kind = "коррекция: "
	}
	return kind + cashEventWhat(e) + noteSuffix(e.Note)
}

// cashCollectLine — служебная строка владельцу об инкассации админа (чистая, тест).
func cashCollectLine(nick string, e *models.CashEvent) string {
	if nick == "" {
		nick = "Сотрудник"
	}
	line := "🧮 " + nick + ": инкассация " + cashEventWhat(e)
	if e.Note != "" {
		line += " — «" + e.Note + "»"
	}
	return line
}

func cashEventOut(e *models.CashEvent) gin.H {
	row := gin.H{"id": e.ID, "kind": e.Kind, "amount_pln": models.PLNFromGrosz(e.AmountGrosz), "note": e.Note,
		"created_at": e.CreatedAt, "created_by": e.CreatedBy}
	if e.Dest != nil {
		row["dest"] = *e.Dest
	}
	if e.StatedGrosz != nil {
		row["stated_pln"] = models.PLNFromGrosz(*e.StatedGrosz)
	}
	if e.VoidedAt != nil {
		row["voided_at"], row["voided_by"] = e.VoidedAt, e.VoidedBy
	}
	return row
}

// drawerNowOut — «должно быть» сейчас: ответ ручек ящика, чтобы экран не
// переспрашивал.
func drawerNowOut(h gin.H) gin.H {
	st := drawerAt(db, time.Now())
	h["expected_pln"], h["baseline"] = models.PLNFromGrosz(st.Expected), st.Point == nil
	return h
}

// POST /admin/cash/events — инкассация (staff) или коррекция остатка (owner).
// Коррекция считает поправку к ожидаемому под тем же замком, что и сдачи.
func handleCashEventCreate(c *gin.Context) {
	var req cashEventRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		cashFail(c, http.StatusBadRequest, "invalid_request")
		return
	}
	owner := c.GetString("user_role") == string(models.UserRoleOwner)
	if code := checkCashEvent(req, owner); code != "" {
		status := http.StatusBadRequest
		if code == "owner_only" {
			status = http.StatusForbidden
		}
		cashFail(c, status, code)
		return
	}
	club, ok := defaultClub()
	if !ok {
		cashFail(c, http.StatusConflict, "no_club")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	ev := models.CashEvent{ClubID: club.ID, Kind: req.Kind, Note: strings.TrimSpace(req.Note), CreatedBy: &adminID}
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", drawerLock).Error; err != nil {
			return err
		}
		ev.CreatedAt = time.Now()
		if req.Kind == models.CashEventCorrect {
			st := drawerAt(tx, ev.CreatedAt)
			if st.Point == nil {
				return errNoDrawer
			}
			stated := models.GroszFromPLN(*req.StatedPLN)
			ev.StatedGrosz, ev.AmountGrosz = &stated, stated-st.Expected
		} else {
			dest := req.Dest
			ev.Dest, ev.AmountGrosz = &dest, models.GroszFromPLN(*req.AmountPLN)
		}
		return tx.Create(&ev).Error
	})
	if errors.Is(err, errNoDrawer) {
		cashFail(c, http.StatusConflict, "no_drawer")
		return
	}
	if err != nil {
		cashFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	action := "cash_collect"
	if ev.Kind == models.CashEventCorrect {
		action = "cash_correct"
	}
	logAdminAction(c, action, nil, cashEventWhat(&ev)+noteSuffix(ev.Note))
	if ev.Kind == models.CashEventCollect && !owner {
		nick := nicknamesByID([]string{adminID.String()})[adminID.String()]
		postSystem(cashCollectLine(nick, &ev), true, map[string]any{"cash_event": true})
	}
	c.JSON(http.StatusCreated, drawerNowOut(gin.H{"event": cashEventOut(&ev)}))
}

// POST /admin/cash/events/:id/void — отменить ошибочное событие ящика (owner).
// Строка остаётся с пометкой: кассовая книга покажет и её, и кто отменил.
func handleCashEventVoid(c *gin.Context) {
	var ev models.CashEvent
	if err := db.First(&ev, "id = ?", c.Param("id")).Error; err != nil {
		cashFail(c, http.StatusNotFound, "event_not_found")
		return
	}
	adminID, _ := uuid.Parse(c.GetString("user_id"))
	res := db.Model(&models.CashEvent{}).Where("id = ? AND voided_at IS NULL", ev.ID).
		Updates(map[string]any{"voided_at": time.Now(), "voided_by": adminID})
	if res.Error != nil {
		cashFail(c, http.StatusInternalServerError, "db_error")
		return
	}
	if res.RowsAffected == 0 {
		cashFail(c, http.StatusConflict, "already_void")
		return
	}
	logAdminAction(c, "cash_event_void", nil, "отмена: "+cashEventText(&ev))
	c.JSON(http.StatusOK, drawerNowOut(gin.H{"voided": ev.ID}))
}
