package main

// З21-и1а (OWNER.md, трек З; №46, З-Р28): пульс дня — сегодняшние клубные сутки до этого момента против нормы:
// медианы того же дня недели за 8 недель до того же момента (выручка, сессии, новые гости), и прогноз месяца по
// дням недели с разбросом. Владелец видел цифры, но не знал, хорошие ли они: 640 zł к обеду — это много для
// вторника? Экран — З21-и1б (блок сверху «Отчётов»); «выручка ниже нормы» — алерт З22-и3, та же pulseAt.

import (
	"database/sql"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
)

const pulseWeeks = 8 // сколько прошлых таких же дней недели — норма

// revenueIn — выручка окна [from, to): пополнения + товары + пакеты — те же каналы, фильтры и сложение, что у
// «Денег» (aggMoney): сегодняшние сутки пульса и «Деньги» за сегодня — одно число.
func revenueIn(from, to time.Time) float64 {
	var all moneySplit
	for _, m := range moneyBy(from, to, func(moneySource) string { return "method" }) {
		for k := range m.PLN {
			all.PLN[k] += m.PLN[k]
		}
	}
	return all.total()
}

// pulseValues — выручка, сессии и новые гости одного окна.
type pulseValues struct{ Revenue, Sessions, New float64 }

// pulseWindow — окно [from, to): выручка, засчитанные сессии, начатые в окне (как «Сессии» в «Загрузке»), и новые
// гости (newGuestsIn — те же «новые», что в отчёте «Гости» и сводке смены).
func pulseWindow(from, to time.Time) pulseValues {
	var sessions int64
	countedSessions().Where("started_at >= ? AND started_at < ?", from, to).Count(&sessions)
	return pulseValues{Revenue: revenueIn(from, to), Sessions: float64(sessions), New: float64(newGuestsIn(from, to))}
}

// quantile — p-квантиль (0…1) выборки с линейной интерполяцией; медиана — p = 0,5 (у чётного числа — среднее двух
// средних). Пустая выборка — 0. Чистая функция (тест).
func quantile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	pos := p * float64(len(s)-1)
	lo := int(math.Floor(pos))
	if lo >= len(s)-1 {
		return s[len(s)-1]
	}
	return s[lo] + (s[lo+1]-s[lo])*(pos-float64(lo))
}

// pulseBand — середина и разброс: медиана, нижняя и верхняя четверти.
type pulseBand struct{ Mid, Low, High float64 }

func bandOf(xs []float64) pulseBand {
	return pulseBand{Mid: quantile(xs, 0.5), Low: quantile(xs, 0.25), High: quantile(xs, 0.75)}
}

// pulseForecast — прогноз месяца (чистая функция, тест): уже есть soFar, остаток сегодняшних суток — rest (как обычно
// добирает такой же день недели после этого часа), каждый оставшийся день месяца — по своему дню недели (byWeekday:
// медиана и четверти его полных суток за 8 недель). Нижняя и верхняя границы — те же суммы по четвертям: «если все
// оставшиеся дни пойдут как их худшая или лучшая четверть». Дня недели без истории в карте нет — он даёт 0.
func pulseForecast(soFar float64, rest pulseBand, days []time.Weekday, byWeekday map[time.Weekday]pulseBand) pulseBand {
	f := pulseBand{Mid: soFar + rest.Mid, Low: soFar + rest.Low, High: soFar + rest.High}
	for _, d := range days {
		b := byWeekday[d]
		f.Mid, f.Low, f.High = f.Mid+b.Mid, f.Low+b.Low, f.High+b.High
	}
	return pulseBand{Mid: round2(f.Mid), Low: round2(f.Low), High: round2(f.High)}
}

// pulseSince — с какого момента у клуба есть история: первое пополнение, продажа или сессия; нулевое — истории нет.
func pulseSince() time.Time {
	var t sql.NullTime
	db.Raw(`SELECT MIN(t) FROM (SELECT MIN(created_at) AS t FROM deposits UNION ALL SELECT MIN(created_at) FROM sales
		UNION ALL SELECT MIN(started_at) FROM sessions) x`).Row().Scan(&t)
	return t.Time
}

// pulseDayCounts — идут ли клубные сутки d в норму и прогноз: к их концу клуб уже работал (since — первое движение
// в базе). Иначе пустые дни до открытия тянули бы медиану к нулю. Чистая функция (тест).
func pulseDayCounts(d time.Time, reportHour int, since time.Time) bool {
	return !since.IsZero() && makePeriod(d, d, reportHour, "").To.After(since)
}

// pulseWeekOffsets — какие из прошлых таких же дней недели (1…8 недель назад, свежий первым) идут в норму.
// Чистая функция (тест).
func pulseWeekOffsets(cur time.Time, reportHour int, since time.Time) []int {
	ks := []int{}
	for k := 1; k <= pulseWeeks; k++ {
		if pulseDayCounts(cur.AddDate(0, 0, -7*k), reportHour, since) {
			ks = append(ks, k)
		}
	}
	return ks
}

// pulseMetric — метрика пульса: сегодня до этого момента, норма (медиана тех же окон прошлых недель), отклонение от
// нормы в процентах (nil — норма 0, а сегодня не 0; нет недель — тоже nil) и сами недели, свежая первой.
func pulseMetric(today float64, weeks []float64) gin.H {
	norm := round2(quantile(weeks, 0.5))
	out := gin.H{"today": round2(today), "norm": norm, "weeks": weeks, "pct": nil}
	if len(weeks) > 0 {
		out["pct"] = pctDelta(today, norm)
	}
	return out
}

// pulseAt — пульс на момент now: окно сегодняшних клубных суток до now и те же окна (тот же день недели, тот же час
// по стенным часам — перевод часов его не сдвигает) 1…8 недель назад; прогноз месяца. Отдельно от ручки — для
// алерта «выручка ниже нормы» (З22-и3).
func pulseAt(now time.Time, reportHour int) gin.H {
	cur := clubDayOf(now, reportHour)
	since := pulseSince()
	today := pulseWindow(makePeriod(cur, cur, reportHour, "").From, now)

	revW, sesW, newW, restW, dates := []float64{}, []float64{}, []float64{}, []float64{}, []string{}
	for _, k := range pulseWeekOffsets(cur, reportHour, since) {
		d := cur.AddDate(0, 0, -7*k)
		w := makePeriod(d, d, reportHour, "")
		at := now.AddDate(0, 0, -7*k)
		v := pulseWindow(w.From, at)
		revW, sesW, newW = append(revW, v.Revenue), append(sesW, v.Sessions), append(newW, v.New)
		restW = append(restW, revenueIn(at, w.To)) // что такой день обычно добирает после этого часа
		dates = append(dates, d.Format("2006-01-02"))
	}

	// прогноз месяца: полные сутки каждого дня недели за 8 недель — одним разрезом по дням
	monthFirst := time.Date(cur.Year(), cur.Month(), 1, 0, 0, 0, 0, cur.Location())
	soFar := revenueIn(makePeriod(monthFirst, cur, reportHour, "").From, now)
	hist := makePeriod(cur.AddDate(0, 0, -7*pulseWeeks), cur.AddDate(0, 0, -1), reportHour, "")
	perDay := moneyBy(hist.From, hist.To, func(s moneySource) string {
		return "to_char(" + dayExpr(s.at, reportHour) + ", 'YYYY-MM-DD')"
	})
	full := map[time.Weekday][]float64{}
	for d := hist.FromDay; !d.After(hist.ToDay); d = d.AddDate(0, 0, 1) {
		if !pulseDayCounts(d, reportHour, since) {
			continue
		}
		v := 0.0
		if m := perDay[d.Format("2006-01-02")]; m != nil {
			v = m.total()
		}
		full[d.Weekday()] = append(full[d.Weekday()], v)
	}
	byWeekday := map[time.Weekday]pulseBand{}
	for wd, xs := range full {
		byWeekday[wd] = bandOf(xs)
	}
	var left []time.Weekday
	for d := cur.AddDate(0, 0, 1); d.Month() == cur.Month(); d = d.AddDate(0, 0, 1) {
		left = append(left, d.Weekday())
	}

	out := gin.H{
		"date": cur.Format("2006-01-02"), "weekday": int(cur.Weekday()), "at": now.Format("15:04"), "as_of": now,
		"weeks": len(revW), "week_dates": dates,
		"revenue":  pulseMetric(today.Revenue, revW),
		"sessions": pulseMetric(today.Sessions, sesW),
		"new":      pulseMetric(today.New, newW),
		"forecast": nil,
	}
	if len(revW) > 0 {
		f := pulseForecast(soFar, bandOf(restW), left, byWeekday)
		out["forecast"] = gin.H{"month": cur.Format("2006-01"), "so_far_pln": soFar, "pln": f.Mid, "low_pln": f.Low,
			"high_pln": f.High, "days_left": len(left)}
	}
	return out
}

// GET /admin/reports/pulse — пульс дня и прогноз месяца (owner, З21-и1а). Момент — до микросекунды, как хранит база:
// as_of ответа — ровно та граница окон, от которой их можно пересчитать.
func handleReportPulse(c *gin.Context) {
	c.JSON(http.StatusOK, pulseAt(time.Now().Truncate(time.Microsecond), clubDayHour()))
}
