package main

// Отчёты владельца за произвольный период (спринт В1, ADMIN.md).
// Решение основателя 2026-08-18: владельческая часть была «подбитой» —
// выручка и загрузка считались только за жёстко зашитые 14 дней, сегменты
// гостей вообще игнорировали период. Здесь один общий слой периода
// (пресеты + произвольный диапазон «с…по…»), автоматическое сравнение с
// предыдущим периодом и четыре разреза: Деньги · Гости · Загрузка · Персонал.
//
// Все окна считаются КЛУБНЫМИ СУТКАМИ от report_hour (настройка владельца).
// По умолчанию — с полуночи, как календарь (З18-и4б, З-Р38: клубы работают
// 24/7); свой час границы — чтобы ночная смена целиком принадлежала дню своего
// начала, а не размазывалась на два календарных дня.
//
// Часы работы клуба для процента занятости берутся из активных шаблонов
// смен (миграция 023), а не из отдельной настройки: владелец и так ведёт
// смены, второй источник правды разошёлся бы с первым.

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ── Засчитанные сессии (З4-и1, OWNER.md №6, №12) ──────────────────────
//
// Снятая посадка (status cancelled: ПК не вышел на связь, посадку сняли,
// ничего не списано) — не сессия: её не было в «Сессиях», «Новых», «Средней
// сессии» и таянии монет, а она туда попадала. Гостевые разрезы считают только
// гостей (role player): админ, севший за ПК, — не гость; загрузка зала
// считает и персонал — машина была занята (Д1).
const sessionCounted = "sessions.status <> 'cancelled'"

func countedSessions() *gorm.DB {
	return db.Model(&models.Session{}).Where(sessionCounted)
}

func guestSessions() *gorm.DB {
	return countedSessions().Joins("JOIN users ON users.id = sessions.user_id").
		Where("users.role = ?", models.UserRolePlayer)
}

// sessionMinutes — «наиграно» сессией (З4-и3, №16): у завершённой — итог при
// завершении (minutes_used, без пауз), у идущей — «на сейчас», сколько уже
// учёл биллинг (billed_minutes). Сессия относится к суткам своего начала — и в
// сводке смены, и в отчётах. Раньше идущая давала 0 везде (minutes_used
// пишется при завершении), а сводка смены брала завершённые в эти сутки:
// ночная сессия через report_hour попадала в сводке в один день, в отчётах —
// в другой.
const sessionMinutes = "CASE WHEN sessions.status = 'active' THEN sessions.billed_minutes ELSE sessions.minutes_used END"

// ── «Новый гость» (З5-и1, OWNER.md №11, З-Р10, Д2) ─────────────────────
//
// Первое появление гостя — его первая засчитанная сессия, а если сессий нет —
// регистрация (Д2: новичок из PWA виден в фильтре «новые» у стойки, но не в
// отчётах — в них только те, кто играл). Новый на момент T — появился не
// раньше seg_new_days суток до T: T — конец периода в отчёте «Гости», конец
// смены в сводке, «сейчас» в фильтре у стойки. Раньше определений было три
// (первая сессия в окне отчёта, регистрация за смену, «аккаунт моложе
// недели»), а seg_new_days ни на что не влиял.

// firstSeenSQL — первое появление гостя строки users (SQL-выражение).
const firstSeenSQL = "COALESCE((SELECT MIN(fs.started_at) FROM sessions fs WHERE fs.user_id = users.id AND fs.status <> 'cancelled'), users.registered_at)"

// segNewDays — окно «новых», суток (настройка владельца в «Экономике»).
func segNewDays() int { return int(settingInt64("seg_new_days", 14)) }

// newSince — с какого момента первое появление ещё делает гостя новым на момент t.
func newSince(t time.Time) time.Time { return t.AddDate(0, 0, -segNewDays()) }

// newGuestsIn — сколько гостей, игравших в окне [from, to), новые на момент to:
// их первая засчитанная сессия не раньше seg_new_days до to.
func newGuestsIn(from, to time.Time) int64 {
	var ids []string
	guestSessions().Group("sessions.user_id").
		Having("MIN(sessions.started_at) >= ? AND bool_or(sessions.started_at >= ? AND sessions.started_at < ?)",
			newSince(to), from, to).
		Pluck("sessions.user_id", &ids)
	return int64(len(ids))
}

// ── Период ────────────────────────────────────────────────────────────

const maxPeriodDays = 730 // два года: защита от отчёта «за всё время» на проде

// period — окно отчёта в клубных сутках.
type period struct {
	FromDay time.Time // первый день периода (полночь календарной даты)
	ToDay   time.Time // последний день периода, включительно
	Days    int
	From    time.Time // начало окна: FromDay + report_hour
	To      time.Time // конец окна, исключительно
	Preset  string
	Until   time.Time // З19-и1: прошлый период обрезан «до того же часа» (у «сегодня»); нулевое — не обрезан
}

func (p period) out() gin.H {
	out := gin.H{
		"from":   p.FromDay.Format("2006-01-02"),
		"to":     p.ToDay.Format("2006-01-02"),
		"days":   p.Days,
		"preset": p.Preset,
	}
	if !p.Until.IsZero() { // З19-и1: «вчера до 17:20» — у «сегодня»
		out["until"] = p.Until.Format("15:04")
	}
	return out
}

// clubDayOf — клубные сутки, которым принадлежит момент t при границе reportHour.
func clubDayOf(t time.Time, reportHour int) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	if t.Hour() < reportHour {
		d = d.AddDate(0, 0, -1)
	}
	return d
}

// reportHourDef — граница клубных суток по умолчанию (report_hour). З18-и4б (решение основателя 07.11, З-Р38):
// большинство клубов работают 24/7 — своих «клубных часов» у них нет, поэтому сутки по умолчанию — с полуночи,
// 00:00–24:00, как календарь (с Б10-и1 было 08:00). Свой час границы — опция «Экономики»: тогда ночная смена целиком
// в сутках своего начала. Сохранённую 8 стенда и пилотного клуба миграция 093 перевела на 0.
const reportHourDef = 0

// clubDayHour — час границы клубных суток: 0 — с полуночи (по умолчанию), иначе свой час клуба. Одно место чтения
// report_hour — раньше каждая ручка читала его сама, со своей восьмёркой по умолчанию (З18-и4б, тест
// TestClubDayHourOnePlace).
func clubDayHour() int { return int(settingInt64("report_hour", reportHourDef)) }

// daysBetween — сколько календарных дней в [a, b] включительно (через AddDate,
// а не деление на 24 часа: перевод часов сломал бы арифметику).
func daysBetween(a, b time.Time) int {
	n := 1
	for d := a; d.Before(b); d = d.AddDate(0, 0, 1) {
		n++
	}
	return n
}

// makePeriod — период по двум датам включительно. Чистая функция (тест).
func makePeriod(fromDay, toDay time.Time, reportHour int, preset string) period {
	loc := fromDay.Location()
	return period{
		FromDay: fromDay,
		ToDay:   toDay,
		Days:    daysBetween(fromDay, toDay),
		Preset:  preset,
		From:    time.Date(fromDay.Year(), fromDay.Month(), fromDay.Day(), reportHour, 0, 0, 0, loc),
		To:      time.Date(toDay.Year(), toDay.Month(), toDay.Day()+1, reportHour, 0, 0, 0, loc),
	}
}

// resolvePeriod — разбор запроса в период. Чистая функция (тест в reports_test.go).
// Приоритет у произвольного диапазона: если пришёл from — пресет игнорируется.
func resolvePeriod(preset, fromStr, toStr string, reportHour int, now time.Time) (period, string) {
	loc := now.Location()
	cur := clubDayOf(now, reportHour)
	parse := func(s string) (time.Time, bool) {
		d, err := time.ParseInLocation("2006-01-02", s, loc)
		return d, err == nil
	}

	if fromStr != "" || toStr != "" {
		from, ok := parse(fromStr)
		if !ok {
			return period{}, "bad_from"
		}
		to := cur
		if toStr != "" {
			if to, ok = parse(toStr); !ok {
				return period{}, "bad_to"
			}
		}
		if to.Before(from) {
			return period{}, "bad_range"
		}
		p := makePeriod(from, to, reportHour, "custom")
		if p.Days > maxPeriodDays {
			return period{}, "too_long"
		}
		return p, ""
	}

	switch preset {
	case "", "d7":
		return makePeriod(cur.AddDate(0, 0, -6), cur, reportHour, "d7"), ""
	case "today":
		return makePeriod(cur, cur, reportHour, "today"), ""
	case "yesterday":
		d := cur.AddDate(0, 0, -1)
		return makePeriod(d, d, reportHour, "yesterday"), ""
	case "d30":
		return makePeriod(cur.AddDate(0, 0, -29), cur, reportHour, "d30"), ""
	case "d90":
		return makePeriod(cur.AddDate(0, 0, -89), cur, reportHour, "d90"), ""
	case "month":
		return makePeriod(time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc), cur, reportHour, "month"), ""
	case "prev_month":
		first := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc).AddDate(0, -1, 0)
		return makePeriod(first, first.AddDate(0, 1, -1), reportHour, "prev_month"), ""
	case "year":
		return makePeriod(time.Date(cur.Year(), 1, 1, 0, 0, 0, 0, loc), cur, reportHour, "year"), ""
	}
	return period{}, "bad_preset"
}

// prevPeriod — с чем сравниваем. Для календарных пресетов это тот же отрезок
// прошлого месяца/года (владелец сравнивает «1–18 августа» с «1–18 июля»,
// а не с концом июля), для остальных — примыкающее окно той же длины.
func prevPeriod(p period, reportHour int) period {
	switch p.Preset {
	case "month":
		first := p.FromDay.AddDate(0, -1, 0)
		last := first.AddDate(0, 0, p.Days-1)
		if endOfMonth := first.AddDate(0, 1, -1); last.After(endOfMonth) {
			last = endOfMonth
		}
		return makePeriod(first, last, reportHour, "prev")
	case "prev_month", "cal_month": // З12-и4а: прошлый календарный месяц — целиком
		first := p.FromDay.AddDate(0, -1, 0)
		return makePeriod(first, first.AddDate(0, 1, -1), reportHour, "prev")
	case "year":
		return makePeriod(p.FromDay.AddDate(-1, 0, 0), p.ToDay.AddDate(-1, 0, 0), reportHour, "prev")
	}
	to := p.FromDay.AddDate(0, 0, -1)
	return makePeriod(to.AddDate(0, 0, -(p.Days-1)), to, reportHour, "prev")
}

// cutPrevAt — «сегодня» сравнивается со «вчера до того же часа» (З19-и1, №40): прошлый период кончается моментом at
// (сейчас минус сутки по стенным часам), если он внутри окна. Раньше «сегодня» до вечера сравнивалось с полными
// вчерашними сутками, и рост почти всегда был минусом. Чистая функция (тест).
func cutPrevAt(prev period, at time.Time) period {
	if at.After(prev.From) && at.Before(prev.To) {
		prev.To, prev.Until = at, at
	}
	return prev
}

// deltaScale — периоды разной длины сравниваются на среднедневных (З19-и1, №40): во сколько раз текущий длиннее
// прошлого — сумма прошлого периода умножается на это, прежде чем считать рост. Октябрь (31 день) против сентября (30),
// «этот месяц» 31 марта против обрезанного февраля; одинаковой длины — 1. Средние и доли (средний чек, занятость, средняя
// сессия) не масштабируются — они уже на единицу. Чистая функция (тест).
func deltaScale(p, prev period) float64 {
	if prev.Days <= 0 || p.Days == prev.Days {
		return 1
	}
	return float64(p.Days) / float64(prev.Days)
}

var periodErrors = map[string]string{
	"bad_from":   "Дата начала — в формате ГГГГ-ММ-ДД",
	"bad_to":     "Дата конца — в формате ГГГГ-ММ-ДД",
	"bad_range":  "Конец периода раньше начала",
	"too_long":   fmt.Sprintf("Период длиннее %d дней", maxPeriodDays),
	"bad_preset": "Неизвестный период",
	"bad_month":  "Месяц — в формате ГГГГ-ММ и не позже текущего",
}

// monthPeriod — календарный месяц «ГГГГ-ММ» (З12-и4а, решение основателя 04.10: «Обзор» — по месяцам). Прошлый месяц —
// целиком, сравнение — с прошлым календарным месяцем целиком; текущий — по сегодняшние клубные сутки, как пресет
// «Этот месяц» (сравнение — тот же отрезок прошлого месяца); будущий — ошибка.
func monthPeriod(m string, reportHour int, now time.Time) (period, string) {
	loc := now.Location()
	first, err := time.ParseInLocation("2006-01", m, loc)
	if err != nil {
		return period{}, "bad_month"
	}
	cur := clubDayOf(now, reportHour)
	curFirst := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, loc)
	switch {
	case first.After(curFirst):
		return period{}, "bad_month"
	case first.Equal(curFirst):
		return makePeriod(curFirst, cur, reportHour, "month"), ""
	}
	return makePeriod(first, first.AddDate(0, 1, -1), reportHour, "cal_month"), ""
}

// periodFromQuery — общий разбор для всех отчётов: период, предыдущий период
// и граница клубных суток. При ошибке сам отвечает 400 и возвращает false.
func periodFromQuery(c *gin.Context) (period, period, int, bool) {
	reportHour, now := clubDayHour(), time.Now()
	var p period
	var code string
	if m := c.Query("month"); m != "" { // З12-и4а: календарный месяц — главнее пресета и дат
		p, code = monthPeriod(m, reportHour, now)
	} else {
		p, code = resolvePeriod(c.Query("preset"), c.Query("from"), c.Query("to"), reportHour, now)
	}
	if code != "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": code, "message": periodErrors[code]})
		return period{}, period{}, 0, false
	}
	prev := prevPeriod(p, reportHour)
	if p.Preset == "today" { // З19-и1: вчера до того же часа
		prev = cutPrevAt(prev, now.AddDate(0, 0, -1))
	}
	return p, prev, reportHour, true
}

// pctDelta — рост в процентах к предыдущему периоду. nil (JSON null) означает
// «не с чем сравнивать»: в прошлом периоде был ноль, процент бессмыслен.
func pctDelta(now, prev float64) *float64 {
	if prev == 0 {
		if now == 0 {
			z := 0.0
			return &z
		}
		return nil
	}
	v := math.Round((now-prev)/prev*1000) / 10
	return &v
}

// dayExpr — выражение «клубные сутки» для GROUP BY по колонке ts. З2-и2: часы
// отнимаются от стенных часов клуба (ts::timestamp — в поясе соединения, З2-и1),
// а не от момента: в дни перевода часов «момент − 8 ч» уводил час у границы в
// соседние сутки (29.03 08:30 — во вчера, 26.10 07:30 — в сегодня), тогда как
// окно периода (makePeriod) и clubDayOf считают по стенным часам.
func dayExpr(col string, reportHour int) string {
	return fmt.Sprintf("(%s::timestamp - INTERVAL '%d hours')::date", col, reportHour)
}

// fillDays — ряд по дням без дыр: дни без данных дают ноль, иначе график врёт.
func fillDays(p period, byDay map[string][2]float64) []gin.H {
	out := make([]gin.H, 0, p.Days)
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		v := byDay[key]
		out = append(out, gin.H{"date": key, "value": v[0], "count": int64(v[1])})
	}
	return out
}

// ── Разрез «Деньги» ───────────────────────────────────────────────────

type moneyAgg struct {
	Revenue     float64 `json:"revenue_pln"` // итого: пополнения + товары + пакеты
	DepositsPLN float64 `json:"deposits_pln"`
	Deposits    int64   `json:"deposits"`
	GoodsPLN    float64 `json:"goods_pln"`
	GoodsSales  int64   `json:"goods_sales"`
	GoodsItems  int64   `json:"goods_items"`
	// Е2-и6: пакеты времени. Отдельной строкой, а не внутри «товаров»: это
	// проданное ВРЕМЯ, и владелец должен видеть, сколько его ушло вперёд.
	PacksPLN float64 `json:"packs_pln"`
	Packs    int64   `json:"packs"`
	PackMin  int64   `json:"pack_minutes"`
	Guests   int64   `json:"guests"`
	AvgCheck float64 `json:"avg_check_pln"` // средний чек пополнения
	Cash     float64 `json:"cash_pln"`
	Card     float64 `json:"card_pln"`
	Blik     float64 `json:"blik_pln"`
}

// ── З-Р4: одна выручка ──
//
// Выручка клуба — пополнения + товары + пакеты времени (решение основателя):
// без отменённых и без оплат кошельком (Г-Р7: те злотые клуб получил
// пополнением). Правило записано здесь один раз, и из этих строк считаются
// итог (aggMoney), шапка, сводка смены, ряд по дням, «Кто оформлял»,
// «Персонал → по сменам» и «по людям» — поэтому суммы разрезов сходятся с
// итогом. З4-и2 (№5): пакеты были в итоге и шапке, но не в разрезах и не в
// сводке смены — Σ дней, Σ смен и сводка расходились с итогом на цену пакетов.

// moneySource — один канал живых денег.
type moneySource struct {
	table  string
	at     string // момент денег: товар — по расчёту у стойки (Ж6), а не по выдаче
	pln    string // сумма
	by     string // кто принял деньги
	filter string // что в канале не выручка
}

const (
	moneyDeposit = iota // пополнения
	moneyGood           // товары и кухня
	moneyPack           // пакеты времени (Е2-и6)
)

var moneySources = [...]moneySource{
	moneyDeposit: {"deposits", "created_at", "amount_pln", "created_by", "voided_at IS NULL"}, // Ж1: отменённый — не выручка
	moneyGood: {"sales", "paid_at", "total_pln", "COALESCE(paid_by, created_by)",
		"voided_at IS NULL AND method NOT IN ('wallet', 'comp')"}, // Р7 (Г7): кошельком — погашение обязательства; З11-и5: подарок — не выручка
	moneyPack: {"user_packages", "created_at", "price_pln", "created_by",
		"voided_at IS NULL AND method NOT IN ('wallet', 'comp')"},
}

// in — выручечные строки канала в окне [from, to).
func (s moneySource) in(from, to time.Time) *gorm.DB {
	return db.Table(s.table).Where(s.at+" >= ? AND "+s.at+" < ? AND "+s.filter, from, to)
}

// moneySplit — деньги строки разреза по каналам: суммы и число операций.
type moneySplit struct {
	PLN [3]float64
	Cnt [3]int64
}

// total — выручка строки: все три канала.
func (m *moneySplit) total() float64 {
	return round2(m.PLN[moneyDeposit] + m.PLN[moneyGood] + m.PLN[moneyPack])
}

// out — выручка строки и из чего она сложилась (подсказки в админке).
func (m *moneySplit) out(row gin.H) gin.H {
	row["revenue_pln"] = m.total()
	row["deposits_pln"], row["deposits"] = round2(m.PLN[moneyDeposit]), m.Cnt[moneyDeposit]
	row["goods_pln"], row["sales"] = round2(m.PLN[moneyGood]), m.Cnt[moneyGood]
	row["packs_pln"], row["packs"] = round2(m.PLN[moneyPack]), m.Cnt[moneyPack]
	return row
}

// moneyBy — выручка окна, разложенная по ключу: key(s) — SQL-выражение над
// колонками канала s (день, автор, способ оплаты), текстом.
func moneyBy(from, to time.Time, key func(s moneySource) string) map[string]*moneySplit {
	out := map[string]*moneySplit{}
	for kind, s := range moneySources {
		var rows []struct {
			K   string
			Pln float64
			Cnt int64
		}
		k := key(s)
		s.in(from, to).Select(k + " AS k, COALESCE(SUM(" + s.pln + "),0) AS pln, COUNT(*) AS cnt").Group(k).Scan(&rows)
		for _, r := range rows {
			if out[r.K] == nil {
				out[r.K] = &moneySplit{}
			}
			out[r.K].PLN[kind] += r.Pln
			out[r.K].Cnt[kind] += r.Cnt
		}
	}
	return out
}

// moneyByAuthor — кто принял деньги: id сотрудника текстом, "" — строки без автора.
func moneyByAuthor(from, to time.Time) map[string]*moneySplit {
	return moneyBy(from, to, func(s moneySource) string { return "COALESCE(CAST(" + s.by + " AS text), '')" })
}

// aggMoney — деньги за период: выручка, её каналы и способы оплаты (те же
// строки moneySources, что у всех разрезов).
func aggMoney(p period) moneyAgg {
	var a moneyAgg
	var all moneySplit
	for method, m := range moneyBy(p.From, p.To, func(moneySource) string { return "method" }) {
		for k := range m.PLN {
			all.PLN[k] += m.PLN[k]
			all.Cnt[k] += m.Cnt[k]
		}
		switch method { // пакеты — третий канал живых денег у стойки (Е2-и6)
		case "cash":
			a.Cash = m.total()
		case "card":
			a.Card = m.total()
		case "blik":
			a.Blik = m.total()
		}
	}
	a.Revenue = all.total()
	a.DepositsPLN, a.Deposits = round2(all.PLN[moneyDeposit]), all.Cnt[moneyDeposit]
	a.GoodsPLN, a.GoodsSales = round2(all.PLN[moneyGood]), all.Cnt[moneyGood]
	a.PacksPLN, a.Packs = round2(all.PLN[moneyPack]), all.Cnt[moneyPack]
	if a.Deposits > 0 {
		a.AvgCheck = math.Round(a.DepositsPLN/float64(a.Deposits)*100) / 100
	}
	// кто платил: З19-и1 (№40) — живыми деньгами в окне, пополнением, товаром или пакетом (раньше — только пополнением),
	// один гость — один раз; продажа без гостя — не в счёт
	parts, args := make([]string, 0, len(moneySources)), make([]any, 0, 2*len(moneySources))
	for _, s := range moneySources {
		parts = append(parts, "SELECT user_id FROM "+s.table+" WHERE "+s.at+" >= ? AND "+s.at+" < ? AND "+s.filter+" AND user_id IS NOT NULL")
		args = append(args, p.From, p.To)
	}
	db.Raw("SELECT COUNT(DISTINCT user_id) FROM ("+strings.Join(parts, " UNION ALL ")+") payers", args...).Scan(&a.Guests)
	// сколько штук и минут продано
	moneySources[moneyGood].in(p.From, p.To).Select("COALESCE(SUM(qty),0)").Scan(&a.GoodsItems)
	moneySources[moneyPack].in(p.From, p.To).Select("COALESCE(SUM(minutes_total),0)").Scan(&a.PackMin)
	return a
}

// GET /admin/reports/money — выручка за период (owner).
func handleReportMoney(c *gin.Context) {
	p, prev, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	cur, old := aggMoney(p), aggMoney(prev)
	sc := deltaScale(p, prev) // З19-и1

	// ряд по дням: столбик — выручка дня, в подсказке — из чего она (З4-и2: и пакеты)
	perDay := moneyBy(p.From, p.To, func(s moneySource) string {
		return "to_char(" + dayExpr(s.at, reportHour) + ", 'YYYY-MM-DD')"
	})
	days := make([]gin.H, 0, p.Days)
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		v := perDay[key]
		if v == nil {
			v = &moneySplit{}
		}
		days = append(days, gin.H{"date": key, "value": v.total(),
			"deposits_pln": round2(v.PLN[moneyDeposit]), "goods_pln": round2(v.PLN[moneyGood]),
			"packs_pln": round2(v.PLN[moneyPack]), "count": v.Cnt[moneyDeposit] + v.Cnt[moneyGood] + v.Cnt[moneyPack]})
	}

	// «Кто оформлял» — кто принял деньги: пополнение, товар (по расчёту) или
	// пакет. Строки без автора — одной строкой «—»: без неё Σ по людям не
	// сошлась бы с кассой (нал + карта + BLIK).
	byAuthor := moneyByAuthor(p.From, p.To)
	ids := make([]string, 0, len(byAuthor))
	for id := range byAuthor {
		if id != "" {
			ids = append(ids, id)
		}
	}
	nick := nicknamesByID(ids)
	byAdmin := make([]gin.H, 0, len(byAuthor))
	for id, m := range byAuthor {
		byAdmin = append(byAdmin, m.out(gin.H{"nickname": nick[id]}))
	}
	sort.Slice(byAdmin, func(i, j int) bool {
		return byAdmin[i]["revenue_pln"].(float64) > byAdmin[j]["revenue_pln"].(float64)
	})

	// «Что покупают»: штуки — полные (и кухня с кошельков тоже: владельцу важно,
	// что едят), по моменту продажи; выручка — только живые деньги (Р7:
	// wallet-продажи в неё не входят) и по моменту расчёта (Ж6: долг считается,
	// когда его закрыли). З10-и8 (№70): и продано на, себестоимость, маржа — по
	// позиции, по категориям и итогом (margin.go)
	topGoods, topCats, goodsMargin, marginDelta := goodsReport(p, prev)

	// потери: всё, что ушло со склада корректировкой, а не через кассу. З6-и2:
	// по цене в момент движения (stock_moves.price_pln), а не по нынешней —
	// перецена позиции прошлые потери не переписывает.
	var loss struct {
		Units int64
		Pln   float64
		Moves int64
	}
	db.Model(&models.StockMove{}).
		Select("COALESCE(-SUM(stock_moves.delta),0) AS units, COUNT(*) AS moves, COALESCE(-SUM(stock_moves.delta * stock_moves.price_pln),0) AS pln").
		Where("stock_moves.created_at >= ? AND stock_moves.created_at < ? AND stock_moves.reason = ? AND stock_moves.delta < 0",
			p.From, p.To, "adjust").Scan(&loss)
	// З10-и7 (№70): и по себестоимости — по цене партий, из которых ушло; ушедшее
	// из партий без цены (и до партий) — «без себестоимости»
	lossCost, lossCosted := lossesByCost(p.From, p.To)

	// Ж1-и2 (Ж-Р4): отменённые депозиты. Сам депозит из выручки ушёл целиком
	// (voided_at IS NULL во всех выручечных запросах выше), но если гость
	// успел отыграть часть денег до отмены, с кошелька снялся только остаток:
	// разница долгом не стала (Р8 трека Г) и в кассе её нет — это потеря
	// клуба, отдельной строкой рядом со складскими. Считается по моменту
	// отмены: потеря случилась, когда её признали.
	var depVoid struct {
		Cnt         int64
		Pln         float64
		Unrecovered float64
	}
	db.Model(&models.Deposit{}).
		Select("COUNT(*) AS cnt, COALESCE(SUM(amount_pln),0) AS pln, COALESCE(SUM(amount_pln - refund_grosz / 100.0),0) AS unrecovered").
		Where("voided_at >= ? AND voided_at < ?", p.From, p.To).Scan(&depVoid)

	// Г0-и4 (Р7 GUEST.md): кошельки гостей — ОБЯЗАТЕЛЬСТВО клуба, не выручка.
	// Выручка признана в момент пополнения (принцип В1 не меняется); остатки
	// кошельков — деньги, которые клуб ещё должен временем/кухней. Списания
	// за период появятся со спринтов Г1 (session_spend) и Г7 (kitchen_spend) —
	// они тоже НЕ выручка, иначе пополнение посчиталось бы дважды.
	// З19-и1 (№40): «Списано с кошельков» — траты: время и кухня. Отмена ошибочного депозита (денег не было) и покупка
	// пакета с кошелька (минуты ещё впереди) — не трата; операций — сессий и заказов, а не строк: время списывается
	// поминутно, у одной сессии их десятки
	wal := walletsNow()
	var walSpent struct {
		Grosz int64
		Cnt   int64
	}
	spendKinds := []string{string(models.WalletTxSessionSpend), string(models.WalletTxKitchenSpend)}
	db.Model(&models.WalletTransaction{}).
		Select("COALESCE(-SUM(amount_grosz),0) AS grosz, "+
			"COUNT(DISTINCT COALESCE(ref_type, '') || ':' || COALESCE(CAST(ref_id AS text), CAST(id AS text))) AS cnt").
		Where("created_at >= ? AND created_at < ? AND amount_grosz < 0 AND kind IN ?", p.From, p.To, spendKinds).
		Scan(&walSpent)
	// Г7: разрез списаний — время отдельно, кухня отдельно (Р7-строка в UI)
	var walKinds []struct {
		Kind  string
		Grosz int64
	}
	db.Model(&models.WalletTransaction{}).
		Select("kind, COALESCE(-SUM(amount_grosz),0) AS grosz").
		Where("created_at >= ? AND created_at < ? AND amount_grosz < 0 AND kind IN ?", p.From, p.To, spendKinds).
		Group("kind").Scan(&walKinds)
	spentSession, spentKitchen := int64(0), int64(0)
	for _, k := range walKinds {
		switch k.Kind {
		case string(models.WalletTxSessionSpend):
			spentSession = k.Grosz
		case string(models.WalletTxKitchenSpend):
			spentKitchen = k.Grosz
		}
	}

	packLia := packsOwedNow() // Е2-и6: «Долг временем»

	c.JSON(http.StatusOK, gin.H{
		"wallet": gin.H{
			"liability_pln":       models.PLNFromGrosz(wal.Total),
			"guests_with_balance": wal.Guests,
			"period_spent_pln":    models.PLNFromGrosz(walSpent.Grosz),
			"period_spends":       walSpent.Cnt,
			"spent_session_pln":   models.PLNFromGrosz(spentSession), // время (Г1)
			"spent_kitchen_pln":   models.PLNFromGrosz(spentKitchen), // кухня (Г7)
		},
		"packages": gin.H{
			"liability_pln":  models.PLNFromGrosz(packLia.Grosz),
			"minutes_left":   packLia.Min,
			"guests":         packLia.Guests,
			"period_pln":     cur.PacksPLN, // продано за период живыми деньгами
			"period_count":   cur.Packs,
			"period_minutes": cur.PackMin,
		},
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"delta": gin.H{ // З19-и1: суммы и счёты — на среднедневных (deltaScale), средний чек — как есть
			"revenue_pln":   pctDelta(cur.Revenue, old.Revenue*sc),
			"deposits_pln":  pctDelta(cur.DepositsPLN, old.DepositsPLN*sc),
			"goods_pln":     pctDelta(cur.GoodsPLN, old.GoodsPLN*sc),
			"packs_pln":     pctDelta(cur.PacksPLN, old.PacksPLN*sc), // Е2-и6
			"deposits":      pctDelta(float64(cur.Deposits), float64(old.Deposits)*sc),
			"guests":        pctDelta(float64(cur.Guests), float64(old.Guests)*sc),
			"avg_check_pln": pctDelta(cur.AvgCheck, old.AvgCheck),
			"margin_pln":    marginDelta, // З10-и8
		},
		"days": days, "by_admin": byAdmin, "top_goods": topGoods,
		"top_categories": topCats, "goods_margin": goodsMargin, "stock_value": stockValue(), // З10-и8 (№70)
		"losses": gin.H{"units": loss.Units, "pln": math.Round(loss.Pln*100) / 100, "moves": loss.Moves,
			"cost_pln": models.PLNFromGrosz(lossCost), "no_cost_units": loss.Units - lossCosted, // З10-и7
			// Ж1: отменённые депозиты за период и «отыграно по ошибочному депозиту»
			"deposit_voids": depVoid.Cnt, "deposit_void_pln": math.Round(depVoid.Pln*100) / 100,
			"unrecovered_pln": math.Round(depVoid.Unrecovered*100) / 100},
		// К8 (Р4/Р7): компенсации за проблемы с ПК — рост обязательства клуба,
		// не выручка и не выручка со знаком минус; отдельной строкой.
		"incidents": incidentsReport(p.From, p.To),
		// З11-и4 (№15): «Дебиторка сейчас» — снимок, от периода не зависит;
		// списанные за период (не отменённые) — «Потери → долги» по дню списания
		"debts": gin.H{"receivable": receivablesNow(), "written_off": writtenOffIn(p.From, p.To)},
		// З11-и5 (№18): подарки периода — по цене и по себестоимости, с причинами; не выручка и не ящик
		"gifts": giftsIn(p.From, p.To),
		// З12-и1 (№49): «Обязательства клуба» — снимок, от периода не зависит; «Сгорело» — за период
		"liabilities": liabilitiesNow(), "burned": burnedIn(p.From, p.To),
		// З12-и2 (№49): «Отработанная выручка» за период и «отдано временем» — на экран выйдет в «Прибыли» (З12-и4)
		"earned": earnedIn(p.From, p.To),
	})
}

// ── Разрез «Гости» ────────────────────────────────────────────────────

type guestsAgg struct {
	Unique      int64   `json:"unique"`
	Sessions    int64   `json:"sessions"`
	Hours       float64 `json:"hours"`
	AvgSessions float64 `json:"avg_sessions"`
	New         int64   `json:"new"`
	Returning   int64   `json:"returning"`
}

// activeUsers — кто играл в окне (id → минуты, сессии).
func activeUsers(p period) map[string][2]int64 {
	var rows []struct {
		UserID  string
		Minutes int64
		Cnt     int64
	}
	guestSessions().
		Select("sessions.user_id, COALESCE(SUM("+sessionMinutes+"),0) AS minutes, COUNT(*) AS cnt").
		Where("sessions.started_at >= ? AND sessions.started_at < ?", p.From, p.To).
		Group("sessions.user_id").Scan(&rows)
	out := make(map[string][2]int64, len(rows))
	for _, r := range rows {
		out[r.UserID] = [2]int64{r.Minutes, r.Cnt}
	}
	return out
}

func aggGuests(p period, lostDays int) (guestsAgg, map[string][2]int64) {
	act := activeUsers(p)
	var a guestsAgg
	for _, v := range act {
		a.Sessions += v[1]
		a.Hours += float64(v[0])
	}
	a.Hours = math.Round(a.Hours/60*10) / 10
	a.Unique = int64(len(act))
	if a.Unique > 0 {
		a.AvgSessions = math.Round(float64(a.Sessions)/float64(a.Unique)*10) / 10
	}
	a.New = newGuestsIn(p.From, p.To) // З5-и1: новые на конец периода

	// вернувшиеся: играли в окне, а до этого пропадали на lost_days и дольше
	var lastBefore []struct {
		UserID string
		LastAt time.Time
	}
	guestSessions().Select("sessions.user_id, MAX(sessions.started_at) AS last_at").
		Where("sessions.started_at < ?", p.From).Group("sessions.user_id").Scan(&lastBefore)
	gap := time.Duration(lostDays) * 24 * time.Hour
	for _, r := range lastBefore {
		if _, played := act[r.UserID]; played && p.From.Sub(r.LastAt) >= gap {
			a.Returning++
		}
	}
	return a, act
}

// GET /admin/reports/guests — гости за период (owner).
func handleReportGuests(c *gin.Context) {
	p, prev, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	lostDays := int(settingInt64("seg_lost_days", 21))
	newDays := segNewDays()

	cur, act := aggGuests(p, lostDays)
	old, prevAct := aggGuests(prev, lostDays)

	// удержание: сколько гостей прошлого периода вернулись в этот
	var retention *float64
	if len(prevAct) > 0 {
		back := 0
		for id := range prevAct {
			if _, ok := act[id]; ok {
				back++
			}
		}
		v := math.Round(float64(back)/float64(len(prevAct))*1000) / 10
		retention = &v
	}

	// пропавшие — НА КОНЕЦ периода и ЗА период (З19-и1, №40): последний визит до конца окна был lost_days суток и
	// больше назад, а за lost_days до начала окна гость ещё ходил — пропал в этом периоде. Раньше в «последний визит»
	// шли и визиты после конца периода (список жил «на сейчас»), а без нижней границы в нём копились все, кто ушёл
	// когда-либо. Забаненных не считаем: они не «пропали», их увели (открытый вопрос QA Б9–Б11 закрыт здесь).
	var lostRows []struct {
		UserID string
		LastAt time.Time
	}
	guestSessions().Select("sessions.user_id, MAX(sessions.started_at) AS last_at").
		Where("users.status <> ? AND sessions.started_at < ?", models.UserStatusBanned, p.To).
		Group("sessions.user_id").
		Having("MAX(sessions.started_at) >= ? AND MAX(sessions.started_at) < ?", p.From.AddDate(0, 0, -lostDays), p.To.AddDate(0, 0, -lostDays)).
		Scan(&lostRows)

	ids := make([]string, 0, len(act)+len(lostRows))
	for id := range act {
		ids = append(ids, id)
	}
	for _, r := range lostRows {
		ids = append(ids, r.UserID)
	}
	nick := nicknamesByID(ids)

	// топ по деньгам за период
	var moneyRows []struct {
		UserID string
		Pln    float64
	}
	db.Model(&models.Deposit{}).Select("user_id, COALESCE(SUM(amount_pln),0) AS pln").
		Where("created_at >= ? AND created_at < ? AND voided_at IS NULL", p.From, p.To).
		Group("user_id").Order("pln DESC").Limit(10).Scan(&moneyRows)
	moneyIDs := make([]string, 0, len(moneyRows))
	for _, r := range moneyRows {
		moneyIDs = append(moneyIDs, r.UserID)
	}
	for id, n := range nicknamesByID(moneyIDs) {
		nick[id] = n
	}
	topMoney := make([]gin.H, 0, len(moneyRows))
	for _, r := range moneyRows {
		v := act[r.UserID]
		topMoney = append(topMoney, gin.H{"user_id": r.UserID, "nickname": nick[r.UserID],
			"deposited_pln": r.Pln, "sessions": v[1], "hours": math.Round(float64(v[0])/60*10) / 10})
	}

	// топ по часам за период
	type hourItem struct {
		id string
		m  int64
		s  int64
	}
	hrs := make([]hourItem, 0, len(act))
	for id, v := range act {
		hrs = append(hrs, hourItem{id, v[0], v[1]})
	}
	sort.Slice(hrs, func(i, j int) bool { return hrs[i].m > hrs[j].m })
	if len(hrs) > 10 {
		hrs = hrs[:10]
	}
	topHours := make([]gin.H, 0, len(hrs))
	for _, h := range hrs {
		topHours = append(topHours, gin.H{"user_id": h.id, "nickname": nick[h.id],
			"hours": math.Round(float64(h.m)/60*10) / 10, "sessions": h.s})
	}

	lostList := make([]gin.H, 0, len(lostRows))
	sort.Slice(lostRows, func(i, j int) bool { return lostRows[i].LastAt.After(lostRows[j].LastAt) })
	for i, r := range lostRows {
		if i >= 100 {
			break
		}
		lostList = append(lostList, gin.H{"user_id": r.UserID, "nickname": nick[r.UserID], "last_visit": r.LastAt})
	}

	// С4-и1 (SENS.md): сколько раз гости калибровали сенсу — счётчик
	// использования AI-калибратора (синтетические прогоны стенда не в счёт)
	aimN, aimU := aimReportCounts(p.From, p.To)
	aimPrevN, _ := aimReportCounts(prev.From, prev.To)
	sc := deltaScale(p, prev) // З19-и1: на среднедневных

	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"aim": gin.H{"sessions": aimN, "guests": aimU, "prev_sessions": aimPrevN, "delta": pctDelta(float64(aimN), float64(aimPrevN)*sc)},
		"delta": gin.H{
			"unique":    pctDelta(float64(cur.Unique), float64(old.Unique)*sc),
			"sessions":  pctDelta(float64(cur.Sessions), float64(old.Sessions)*sc),
			"hours":     pctDelta(cur.Hours, old.Hours*sc),
			"new":       pctDelta(float64(cur.New), float64(old.New)*sc),
			"returning": pctDelta(float64(cur.Returning), float64(old.Returning)*sc),
		},
		"retention_pct": retention,
		"thresholds":    gin.H{"new_days": newDays, "lost_days": lostDays},
		"lost":          gin.H{"count": len(lostRows), "items": lostList},
		"top_money":     topMoney, "top_hours": topHours,
	})
}

// ── Разрез «Загрузка зала» ────────────────────────────────────────────

// openMinutesPerDay — сколько минут клуб открыт в календарный день d по
// активным шаблонам смен: минуты, покрытые сменой этого дня, плюс утренний
// хвост ночной смены, начатой накануне. Пересечения смен не удваиваются.
// Смен нет вовсе — считаем круглосуточно. Чистая функция (тест).
func openMinutesPerDay(shifts []models.Shift, d time.Time) int {
	n := 0
	for _, v := range openCoverage(shifts, d) {
		if v {
			n++
		}
	}
	return n
}

// openCoverage — какие минуты календарного дня d покрыты активными сменами: то же, что openMinutesPerDay, но
// поминутно — для нарезки по часам (З17-и2). Смен нет вовсе — все.
func openCoverage(shifts []models.Shift, d time.Time) [1440]bool {
	var covered [1440]bool
	active := false
	for _, s := range shifts {
		if s.Active {
			active = true
			break
		}
	}
	if !active {
		for i := range covered {
			covered[i] = true
		}
		return covered
	}
	mark := func(from, to int) {
		if from < 0 {
			from = 0
		}
		if to > 1440 {
			to = 1440
		}
		for i := from; i < to; i++ {
			covered[i] = true
		}
	}
	today, yesterday := dowMonday(d), dowMonday(d.AddDate(0, 0, -1))
	for _, s := range shifts {
		if !s.Active {
			continue
		}
		if s.DaysMask&(1<<today) != 0 {
			if s.EndMin > s.StartMin {
				mark(s.StartMin, s.EndMin)
			} else {
				mark(s.StartMin, 1440)
			}
		}
		if s.EndMin <= s.StartMin && s.DaysMask&(1<<yesterday) != 0 {
			mark(0, s.EndMin)
		}
	}
	return covered
}

func openMinutesInPeriod(shifts []models.Shift, p period) int64 {
	var total int64
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		total += int64(openMinutesPerDay(shifts, d))
	}
	return total
}

// ── Нарезка по часам (З17-и2, №13, З-Р6 «б») ──────────────────────────

// hourSlice — кусок сессии в часе стенных часов клуба: клубные сутки куска (полночь), час и минуты игры в нём.
type hourSlice struct {
	Day     time.Time
	Hour    int
	Minutes float64
}

// sliceSession — нарезка сессии по часам стенных часов (часы — в поясе start; З17-и2, №13): played минут игры делятся
// по часам её отрезка [start, end) пропорционально длине куска — когда были паузы, не знаем, поэтому они ложатся
// равномерно. Раньше вся длина сессии ложилась в час её начала: пять часов с 18:00 — столбик «18:00», 19–22 пусто.
// Отрезка нет (end не позже start) — все минуты в час начала. Клубные сутки куска — по reportHour: ночь после пятницы —
// пятница, как у смен и «Наиграно». Чистая (юнит).
func sliceSession(start, end time.Time, played float64, reportHour int) []hourSlice {
	if played <= 0 {
		return nil
	}
	if !end.After(start) {
		return []hourSlice{{Day: clubDayOf(start, reportHour), Hour: start.Hour(), Minutes: played}}
	}
	loc := start.Location()
	total := end.Sub(start).Seconds()
	var out []hourSlice
	for t := start; t.Before(end); {
		lt := t.In(loc)
		// до конца часа стенных часов — отсчётом времени, а не time.Date: осенью час «2» идёт дважды (оба раза — в «2»),
		// весной его нет — после «1» сразу «3»
		next := lt.Truncate(time.Minute).Add(time.Duration(60-lt.Minute()) * time.Minute)
		if next.After(end) {
			next = end
		}
		out = append(out, hourSlice{Day: clubDayOf(lt, reportHour), Hour: lt.Hour(), Minutes: played * next.Sub(t).Seconds() / total})
		t = next
	}
	return out
}

// loadSession — сессия для нарезки: отрезок игры и минуты (у идущей — учтённые биллингом, конец — «сейчас»).
type loadSession struct {
	Start, End time.Time
	Minutes    float64
}

// hourCell — ячейка нарезки: минуты игры, сколько сессий шло в этом часе, ёмкость (минут работы клуба × ПК в ёмкости).
type hourCell struct {
	Minutes  float64
	Sessions int64
	Open     int64
}

// loadByHour — нарезка сессий: по часам суток (hours) и по «день недели клубных суток × час» (heat, понедельник —
// первый). Сессия считается в часе, если в нём есть её кусок. Ёмкость — open (минут работы по сменам за период,
// openByClubHour) × pcs. Чистая (юнит).
func loadByHour(sessions []loadSession, reportHour int, open [7][24]int64, pcs int64) (hours [24]hourCell, heat [7][24]hourCell) {
	for _, s := range sessions {
		var inHour [24]bool
		var inCell [7][24]bool
		for _, sl := range sliceSession(s.Start, s.End, s.Minutes, reportHour) {
			dw := dowMonday(sl.Day)
			hours[sl.Hour].Minutes += sl.Minutes
			heat[dw][sl.Hour].Minutes += sl.Minutes
			if !inHour[sl.Hour] {
				inHour[sl.Hour] = true
				hours[sl.Hour].Sessions++
			}
			if !inCell[dw][sl.Hour] {
				inCell[dw][sl.Hour] = true
				heat[dw][sl.Hour].Sessions++
			}
		}
	}
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			heat[d][h].Open = open[d][h] * pcs
			hours[h].Open += open[d][h] * pcs
		}
	}
	return hours, heat
}

// openByClubHour — минуты работы клуба по сменам за период, разложенные по «день недели клубных суток × час»: час h
// клубных суток d — календарный день d, если h ≥ reportHour, иначе d+1 (утро после ночи). Чистая (юнит).
func openByClubHour(shifts []models.Shift, p period, reportHour int) [7][24]int64 {
	var out [7][24]int64
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		dw := dowMonday(d)
		today, tomorrow := openCoverage(shifts, d), openCoverage(shifts, d.AddDate(0, 0, 1))
		for h := 0; h < 24; h++ {
			cov := &today
			if h < reportHour {
				cov = &tomorrow
			}
			for m := h * 60; m < h*60+60; m++ {
				if cov[m] {
					out[dw][h]++
				}
			}
		}
	}
	return out
}

// cellOut — ячейка нарезки в JSON: часы, сессии, занятость; занятость nil — клуб в этот час закрыт или ПК нет.
func cellOut(c hourCell) gin.H {
	var occ any
	if c.Open > 0 {
		occ = math.Round(c.Minutes/float64(c.Open)*1000) / 10
	}
	return gin.H{"hours": math.Round(c.Minutes/60*10) / 10, "sessions": c.Sessions, "occupancy_pct": occ}
}

type loadAgg struct {
	Hours     float64 `json:"hours"`
	Sessions  int64   `json:"sessions"`
	AvgMin    float64 `json:"avg_minutes"`
	Occupancy float64 `json:"occupancy_pct"`
}

func aggLoad(p period, shifts []models.Shift, computers int64) loadAgg {
	var a loadAgg
	var head struct {
		Minutes int64
		Cnt     int64
	}
	countedSessions().
		Select("COALESCE(SUM("+sessionMinutes+"),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).Scan(&head)
	a.Sessions = head.Cnt
	a.Hours = math.Round(float64(head.Minutes)/60*10) / 10
	if head.Cnt > 0 {
		a.AvgMin = math.Round(float64(head.Minutes) / float64(head.Cnt))
	}
	if capacity := openMinutesInPeriod(shifts, p) * computers; capacity > 0 {
		a.Occupancy = math.Round(float64(head.Minutes)/float64(capacity)*1000) / 10
	}
	return a
}

// ── Зоны и ПК в «Загрузке» (З17-и1, №7, З-Р6) ─────────────────────────

// loadPCs — ПК зоны сейчас: всего и из них в ремонте. Истории статусов нет — «на момент отчёта».
type loadPCs struct {
	Total, Repair int64
}

// loadUse — наигранное за период по зоне сессии: её снимок (З16), у сессии без снимка — зона её ПК. ZoneID — nil у
// сессии без зоны и у удалённой зоны (её имя осталось в снимке).
type loadUse struct {
	ZoneID  *string
	Zone    string
	Minutes int64
	Cnt     int64
}

// loadZones — таблица «Зоны» (З17-и1, №7, З-Р6): у зоны — все её ПК, ёмкость — ПК не в ремонте × часы работы
// (раньше делитель — только ПК, на которых были сессии: 6 часов на одном ПК из трёх давали 25 %, а не 8,3 %); зоны без
// сессий — с нулями; «без зоны» — если есть такие ПК или сессии; сессии удалённой зоны — своей строкой без ПК.
// Занятость без ёмкости (нет ПК или все в ремонте) — nil, а не 0: делить не на что. pcs — по id зоны, "" — без зоны.
// Порядок — по наигранному, при равенстве — по имени. Чистая (юнит).
func loadZones(zones []models.Zone, pcs map[string]loadPCs, use []loadUse, openMin int64) []gin.H {
	type zoneRow struct {
		id, name     string
		pcs          loadPCs
		minutes, cnt int64
		deleted      bool
	}
	var rows []*zoneRow
	byKey := map[string]*zoneRow{}
	add := func(key string, r *zoneRow) *zoneRow {
		rows = append(rows, r)
		byKey[key] = r
		return r
	}
	for _, z := range zones {
		id := z.ID.String()
		add(id, &zoneRow{id: id, name: z.Name, pcs: pcs[id]})
	}
	if p := pcs[""]; p.Total > 0 {
		add("", &zoneRow{name: zoneLabel(""), pcs: p})
	}
	for _, u := range use {
		key := ""
		switch {
		case u.ZoneID != nil:
			key = *u.ZoneID
		case u.Zone != "":
			key = "удалена:" + u.Zone
		}
		r := byKey[key]
		if r == nil {
			r = add(key, &zoneRow{name: zoneLabel(u.Zone), deleted: key != ""})
		}
		r.minutes += u.Minutes
		r.cnt += u.Cnt
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].minutes != rows[j].minutes {
			return rows[i].minutes > rows[j].minutes
		}
		return rows[i].name < rows[j].name
	})
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		var occ any // ёмкости нет — nil
		if capacity := openMin * (r.pcs.Total - r.pcs.Repair); capacity > 0 {
			occ = math.Round(float64(r.minutes)/float64(capacity)*1000) / 10
		}
		row := gin.H{"zone": r.name, "computers": r.pcs.Total, "repair": r.pcs.Repair,
			"hours": math.Round(float64(r.minutes)/60*10) / 10, "sessions": r.cnt, "occupancy_pct": occ}
		if r.id != "" {
			row["zone_id"] = r.id
		}
		if r.deleted {
			row["deleted"] = true
		}
		out = append(out, row)
	}
	return out
}

// GET /admin/reports/load — загрузка зала за период (owner).
func handleReportLoad(c *gin.Context) {
	p, prev, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	var shifts []models.Shift
	db.Order("sort, start_min").Find(&shifts)
	// З17-и1 (№7, З-Р6): ПК в ремонте сейчас — не в ёмкости (истории статусов нет — на момент отчёта)
	var computers, repair int64
	db.Model(&models.Computer{}).Count(&computers)
	db.Model(&models.Computer{}).Where("status = ?", models.ComputerStatusMaintenance).Count(&repair)
	inService := computers - repair

	cur, old := aggLoad(p, shifts, inService), aggLoad(prev, shifts, inService)

	var dayRows []struct {
		D       time.Time
		Minutes int64
		Cnt     int64
	}
	countedSessions().
		Select(dayExpr("started_at", reportHour)+" AS d, COALESCE(SUM("+sessionMinutes+"),0) AS minutes, COUNT(*) AS cnt").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).
		Group("d").Order("d").Scan(&dayRows)
	byDay := map[string][2]float64{}
	for _, r := range dayRows {
		byDay[r.D.Format("2006-01-02")] = [2]float64{math.Round(float64(r.Minutes)/60*10) / 10, float64(r.Cnt)}
	}

	// З17-и2 (№13, З-Р6 «б»): часы суток — нарезкой сессий по часам стенных часов клуба (паузы — пропорционально,
	// идущая — до «сейчас»), сессии те же, что в «Наиграно» (по старту в периоде); пик — по нарезке; heat — день недели
	// клубных суток × час
	var lsRows []struct {
		StartedAt time.Time
		EndedAt   *time.Time
		Status    string
		Minutes   int64
	}
	countedSessions().
		Select("sessions.started_at, sessions.ended_at, sessions.status, "+sessionMinutes+" AS minutes").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).Scan(&lsRows)
	now := time.Now()
	ls := make([]loadSession, 0, len(lsRows))
	for _, r := range lsRows {
		end := r.StartedAt // завершённая без ended_at — всё в час начала
		if r.Status == string(models.SessionStatusActive) {
			end = now
		} else if r.EndedAt != nil {
			end = *r.EndedAt
		}
		ls = append(ls, loadSession{Start: r.StartedAt.In(models.ClubLocation), End: end.In(models.ClubLocation),
			Minutes: float64(r.Minutes)})
	}
	byHour, heatCells := loadByHour(ls, reportHour, openByClubHour(shifts, p, reportHour), inService)
	peakHour, peakVal := -1, 0.0
	hours := make([]gin.H, 0, 24)
	for h, c := range byHour {
		row := cellOut(c)
		row["hour"] = h
		hours = append(hours, row)
		if c.Minutes > peakVal {
			peakHour, peakVal = h, c.Minutes
		}
	}
	heat := make([][]gin.H, 7)
	for d := range heatCells {
		heat[d] = make([]gin.H, 24)
		for h, c := range heatCells[d] {
			heat[d][h] = cellOut(c)
		}
	}

	// З17-и1 (№7, З-Р6): зоны — от всех их ПК, сессии — по снимку зоны (З16), зоны без сессий — с нулями
	var zoneList []models.Zone
	db.Order("sort, name").Find(&zoneList)
	var pcCounts []struct {
		ZoneID *string
		Total  int64
		Repair int64
	}
	db.Model(&models.Computer{}).
		Select("zone_id::text AS zone_id, COUNT(*) AS total, COUNT(*) FILTER (WHERE status = 'maintenance') AS repair").
		Group("zone_id").Scan(&pcCounts)
	pcsByZone := map[string]loadPCs{}
	for _, r := range pcCounts {
		key := ""
		if r.ZoneID != nil {
			key = *r.ZoneID
		}
		pcsByZone[key] = loadPCs{Total: r.Total, Repair: r.Repair}
	}
	var use []loadUse
	countedSessions().
		Select("(CASE WHEN sessions.xp_per_min IS NOT NULL THEN sessions.zone_id ELSE computers.zone_id END)::text AS zone_id, "+
			"CASE WHEN sessions.xp_per_min IS NOT NULL THEN sessions.zone_name ELSE COALESCE(computers.zone, '') END AS zone, "+
			"COALESCE(SUM("+sessionMinutes+"),0) AS minutes, COUNT(*) AS cnt").
		Joins("LEFT JOIN computers ON computers.id = sessions.computer_id").
		Where("sessions.started_at >= ? AND sessions.started_at < ?", p.From, p.To).
		Group("1, 2").Scan(&use)
	openMin := openMinutesInPeriod(shifts, p)
	zones := loadZones(zoneList, pcsByZone, use, openMin)

	// З17-и1: все ПК — машина без сессий видна с нулями; «в ремонте» — сейчас
	var pcRows []struct {
		Name    string
		Zone    string
		Status  string
		Minutes int64
		Cnt     int64
	}
	db.Raw("SELECT computers.name, COALESCE(computers.zone, '') AS zone, computers.status::text AS status, "+
		"COALESCE(SUM("+sessionMinutes+"), 0) AS minutes, COUNT(sessions.id) AS cnt FROM computers "+
		"LEFT JOIN sessions ON sessions.computer_id = computers.id AND "+sessionCounted+
		" AND sessions.started_at >= ? AND sessions.started_at < ? "+
		"GROUP BY computers.id ORDER BY minutes DESC, computers.name", p.From, p.To).Scan(&pcRows)
	pcs := make([]gin.H, 0, len(pcRows))
	for _, r := range pcRows {
		occ := 0.0
		if openMin > 0 {
			occ = math.Round(float64(r.Minutes)/float64(openMin)*1000) / 10
		}
		row := gin.H{"name": r.Name, "zone": zoneLabel(r.Zone), "sessions": r.Cnt,
			"hours": math.Round(float64(r.Minutes)/60*10) / 10, "occupancy_pct": occ}
		if r.Status == string(models.ComputerStatusMaintenance) {
			row["repair"] = true
		}
		pcs = append(pcs, row)
	}

	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": cur, "prev": old,
		"delta": gin.H{ // З19-и1: часы и сессии — на среднедневных, занятость и средняя сессия — как есть
			"hours":         pctDelta(cur.Hours, old.Hours*deltaScale(p, prev)),
			"sessions":      pctDelta(float64(cur.Sessions), float64(old.Sessions)*deltaScale(p, prev)),
			"occupancy_pct": pctDelta(cur.Occupancy, old.Occupancy),
			"avg_minutes":   pctDelta(cur.AvgMin, old.AvgMin),
		},
		"capacity": gin.H{"computers": inService, "total": computers, "repair": repair, "open_minutes": openMin,
			"open_hours_per_day": math.Round(float64(openMin)/float64(p.Days)/60*10) / 10},
		"days": fillDays(p, byDay), "hours": hours, "zones": zones, "computers": pcs,
		"peak_hour": peakHour, "heat": heat, // З17-и2: heat[день недели клубных суток, пн — 0][час]
	})
}

// ── Разрез «Персонал и смены» ─────────────────────────────────────────

// workSpan — запись табеля в разрезе «по сменам» (З15-и4): чья, с какого по какой момент, имя смены (снимок).
type workSpan struct {
	User       string
	Start, End time.Time
	Shift      string
}

// workSpansIn — записи табеля с именем смены, задевающие окно [from, to); открытая — до now.
func workSpansIn(from, to, now time.Time) []workSpan {
	var entries []models.WorkEntry
	db.Where("started_at < ? AND (ended_at IS NULL OR ended_at > ?) AND shift_name <> ''", to, from).Find(&entries)
	out := make([]workSpan, 0, len(entries))
	for _, w := range entries {
		end := now
		if w.EndedAt != nil {
			end = *w.EndedAt
		}
		out = append(out, workSpan{User: w.UserID.String(), Start: w.StartedAt, End: end, Shift: w.ShiftName})
	}
	return out
}

// shiftByTimesheet — смена момента at по табелю (чистая, тест). Деньги — к смене того, кто их принял (author): его
// запись в этот момент. Иначе — смена, которая тогда работала: из идущих записей — смена по графику этого момента
// (onDuty — имя активного шаблона, "" — такого нет), а если её людей на месте нет — заступившая последней. Записи без
// имени смены не в счёт, конец записи — уже не её момент. "" — по табелю в этот момент никого.
func shiftByTimesheet(spans []workSpan, at time.Time, author, onDuty string) string {
	own, last, duty := -1, -1, false
	for i, w := range spans {
		if w.Shift == "" || at.Before(w.Start) || !at.Before(w.End) {
			continue
		}
		if author != "" && w.User == author && (own < 0 || w.Start.After(spans[own].Start)) {
			own = i
		}
		if onDuty != "" && w.Shift == onDuty {
			duty = true
		}
		if last < 0 || w.Start.After(spans[last].Start) {
			last = i
		}
	}
	switch {
	case own >= 0:
		return spans[own].Shift
	case duty:
		return onDuty
	case last >= 0:
		return spans[last].Shift
	}
	return ""
}

// GET /admin/reports/staff — кто работал и что принесла каждая смена (owner).
func handleReportStaff(c *gin.Context) {
	p, prev, _, ok := periodFromQuery(c)
	if !ok {
		return
	}
	var shifts []models.Shift
	db.Order("sort, start_min").Find(&shifts)

	// выручка и сессии по сменам: момент операции раскладываем по шаблонам
	type bucket struct {
		money    moneySplit // пополнения, товары и пакеты (З4-и2)
		sessions int64
		minutes  int64
	}
	buckets := map[string]*bucket{}
	get := func(name string) *bucket {
		if buckets[name] == nil {
			buckets[name] = &bucket{}
		}
		return buckets[name]
	}
	const outside = "вне смен"

	// З15-и4 (№30): смена момента — по табелю, по имени-снимку в записи: удалённый, переименованный или выключенный
	// шаблон больше не уносит свою историю «вне смен» или в соседнюю смену. Деньги — к смене того, кто их принял (его
	// запись табеля в этот момент), сессии — к смене, которая тогда работала (shiftByTimesheet); по табелю в этот
	// момент никого — как раньше, по активным шаблонам.
	spans := workSpansIn(p.From, p.To, time.Now())
	shiftOf := func(at time.Time, author string) string {
		onDuty := ""
		for _, s := range shifts {
			if !s.Active {
				continue
			}
			if on, _ := shiftActiveAt(s.StartMin, s.EndMin, s.DaysMask, at); on {
				onDuty = s.Name
				break
			}
		}
		if name := shiftByTimesheet(spans, at, author, onDuty); name != "" {
			return name
		}
		if onDuty != "" {
			return onDuty
		}
		return outside
	}

	// З-Р4: те же строки, что итог — Σ смен сходится с KPI «Выручка» над таблицей
	for kind, src := range moneySources {
		var rows []struct {
			At     time.Time
			Pln    float64
			Author string
		}
		src.in(p.From, p.To).Select(src.at + " AS at, " + src.pln + " AS pln, COALESCE(CAST(" + src.by + " AS text), '') AS author").Scan(&rows)
		for _, r := range rows {
			b := get(shiftOf(r.At, r.Author)) // Ж6: товар — по моменту расчёта; З15-и4: к смене того, кто принял
			b.money.PLN[kind] += r.Pln
			b.money.Cnt[kind]++
		}
	}

	// З4-и3: идущие сессии — «на сейчас», как в загрузке и сводке смены
	var sess []struct {
		StartedAt time.Time
		Minutes   int64
	}
	countedSessions().Select("sessions.started_at, "+sessionMinutes+" AS minutes").
		Where("started_at >= ? AND started_at < ?", p.From, p.To).Scan(&sess)
	for _, s := range sess {
		b := get(shiftOf(s.StartedAt, "")) // З15-и4: у сессии автора нет — смена, что тогда работала
		b.sessions++
		b.minutes += s.Minutes
	}

	byShift := make([]gin.H, 0, len(buckets))
	add := func(name string) {
		b := buckets[name]
		if b == nil {
			b = &bucket{}
		}
		avg := 0.0
		if n := b.money.Cnt[moneyDeposit]; n > 0 {
			avg = math.Round(b.money.PLN[moneyDeposit]/float64(n)*100) / 100
		}
		byShift = append(byShift, b.money.out(gin.H{"shift": name,
			"sessions": b.sessions, "hours": math.Round(float64(b.minutes)/60*10) / 10, "avg_check_pln": avg}))
	}
	listed := map[string]bool{}
	for _, s := range shifts {
		if s.Active && !listed[s.Name] {
			listed[s.Name] = true
			add(s.Name)
		}
	}
	// З15-и4: смены из табеля, которых среди активных шаблонов уже нет (удалён, переименован, выключен), — по имени
	others := make([]string, 0)
	for name := range buckets {
		if !listed[name] && name != outside {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	for _, name := range others {
		add(name)
	}
	if buckets[outside] != nil {
		add(outside)
	}

	// по людям: смены в графике, выручка, операции
	shiftLen := map[string]int{}
	for _, s := range shifts {
		l := s.EndMin - s.StartMin
		if l <= 0 {
			l += 1440
		}
		shiftLen[s.ID.String()] = l
	}
	var asg []models.ShiftAssignment
	db.Where("date >= ? AND date <= ?", p.FromDay, p.ToDay).Find(&asg)
	type person struct {
		shifts   int64
		schedMin int64
		factMin  int64 // В3-4: факт из табеля; З3-и2 — подтверждённый
		factCnt  int64 // З3-и2: дни с подтверждёнными минутами
		autoMin  int64 // З3-и2: автозакрытые минуты — вне выплаты
		autoCnt  int64
		money    moneySplit // что принял: пополнения, товары, пакеты (З4-и2)
		actions  int64
		grants   int64
	}
	people := map[string]*person{}
	who := func(id string) *person {
		if people[id] == nil {
			people[id] = &person{}
		}
		return people[id]
	}
	for _, a := range asg {
		w := who(a.UserID.String())
		w.shifts++
		w.schedMin += int64(shiftLen[a.ShiftID.String()])
	}
	for id, m := range moneyByAuthor(p.From, p.To) {
		if id != "" { // строки без автора — не человек; они видны в «Кто оформлял»
			who(id).money = *m
		}
	}
	var actRows []struct {
		AdminID string
		Cnt     int64
	}
	db.Model(&models.AdminAction{}).Select("admin_id, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ? AND action <> ?", p.From, p.To, "login_failed"). // З0-и3
		Group("admin_id").Scan(&actRows)
	for _, r := range actRows {
		who(r.AdminID).actions = r.Cnt
	}
	actRows = nil
	db.Model(&models.AdminGrant{}).Select("admin_id, COUNT(*) AS cnt").
		Where("created_at >= ? AND created_at < ?", p.From, p.To).Group("admin_id").Scan(&actRows)
	for _, r := range actRows {
		who(r.AdminID).grants = r.Cnt
	}

	// факт из табеля (В3-4): без него ставка из кадровой карточки бесполезна;
	// З3-и2 — к выплате только подтверждённое, автозакрытое отдельно
	for id, v := range factByUser(p) {
		w := who(id)
		w.factMin, w.factCnt, w.autoMin, w.autoCnt = v.Minutes, v.Shifts, v.AutoMinutes, v.AutoEntries
	}

	ids := make([]string, 0, len(people))
	for id := range people {
		ids = append(ids, id)
	}
	nick := nicknamesByID(ids)
	roles := rolesByID(ids)
	cards := map[string]models.StaffProfile{}
	var profiles []models.StaffProfile
	db.Find(&profiles)
	for _, pr := range profiles {
		cards[pr.UserID.String()] = pr
	}
	byPerson := make([]gin.H, 0, len(people))
	for id, w := range people {
		row := w.money.out(gin.H{
			"nickname": nick[id], "role": roles[id],
			"shifts": w.shifts, "scheduled_hours": math.Round(float64(w.schedMin)/60*10) / 10,
			"fact_hours": math.Round(float64(w.factMin)/60*10) / 10, "fact_shifts": w.factCnt,
			"actions": w.actions + w.grants + w.money.Cnt[moneyGood],
		})
		if w.autoCnt > 0 { // З3-и2: не подтверждены — в выплату не вошли
			row["auto_closed_hours"] = math.Round(float64(w.autoMin)/60*10) / 10
			row["auto_closed_entries"] = w.autoCnt
		}
		if card, ok := cards[id]; ok {
			amount, kind := payoutFor(card.RateType, card.RateAmount, w.factMin, w.factCnt)
			if kind != "" {
				row["payout_pln"] = amount
				row["payout_kind"] = kind
			}
			if card.Position != "" {
				row["position"] = card.Position
			}
		}
		byPerson = append(byPerson, row)
	}
	sort.Slice(byPerson, func(i, j int) bool {
		return byPerson[i]["revenue_pln"].(float64) > byPerson[j]["revenue_pln"].(float64)
	})

	curMoney, oldMoney := aggMoney(p), aggMoney(prev)
	c.JSON(http.StatusOK, gin.H{
		"period": p.out(), "prev_period": prev.out(),
		"totals": gin.H{"revenue_pln": curMoney.Revenue, "deposits": curMoney.Deposits, "deposits_pln": curMoney.DepositsPLN,
			"goods_pln": curMoney.GoodsPLN, "packs_pln": curMoney.PacksPLN, "packs": curMoney.Packs,
			"people": len(people), "assignments": len(asg)},
		"prev": gin.H{"revenue_pln": oldMoney.Revenue, "deposits": oldMoney.Deposits, "goods_pln": oldMoney.GoodsPLN,
			"packs_pln": oldMoney.PacksPLN},
		"delta":     gin.H{"revenue_pln": pctDelta(curMoney.Revenue, oldMoney.Revenue*deltaScale(p, prev))}, // З19-и1
		"by_shift":  byShift,
		"by_person": byPerson,
	})
}

// ── Мелкие помощники ──────────────────────────────────────────────────

func nicknamesByID(ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var users []models.User
	db.Select("id, nickname").Where("id IN ?", ids).Find(&users)
	for i := range users {
		out[users[i].ID.String()] = users[i].Nickname
	}
	return out
}

func rolesByID(ids []string) map[string]string {
	out := map[string]string{}
	if len(ids) == 0 {
		return out
	}
	var users []models.User
	db.Select("id, role").Where("id IN ?", ids).Find(&users)
	for i := range users {
		out[users[i].ID.String()] = string(users[i].Role)
	}
	return out
}

func idsOf[T any](rows []T, key func(T) string) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, key(r))
	}
	return out
}
