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
	"fmt"
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
	Adjust                 int64 // ± коррекции владельца
}

func (f drawerFlow) In() int64  { return f.Deposits + f.Goods + f.Packs }
func (f drawerFlow) Out() int64 { return f.Refunds + f.Collections }

func (f drawerFlow) out() gin.H {
	pln := models.PLNFromGrosz
	return gin.H{
		"in_pln": pln(f.In()), "out_pln": pln(f.Out()), "adjust_pln": pln(f.Adjust),
		"deposits_pln": pln(f.Deposits), "goods_pln": pln(f.Goods), "packs_pln": pln(f.Packs),
		"refunds_pln": pln(f.Refunds), "collections_pln": pln(f.Collections),
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
		Refunds: sum(refund+"FROM deposits WHERE method = 'cash' AND voided_at >= ? AND voided_at < ?", "voided_by") +
			sum(grosz("total_pln")+"FROM sales WHERE method = 'cash' AND paid_at IS NOT NULL AND voided_at >= ? AND voided_at < ?",
				"voided_by") +
			sum(refund+"FROM user_packages WHERE method = 'cash' AND voided_at >= ? AND voided_at < ?", "voided_by"),
		Collections: sum(event+"AND kind = 'collect' AND created_at >= ? AND created_at < ?", "created_by"),
		Adjust:      sum(event+"AND kind = 'correct' AND created_at >= ? AND created_at < ?", "created_by"),
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
