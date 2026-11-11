package main

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// З21-и1а: квантиль, прогноз месяца и какие прошлые дни идут в норму пульса.

func TestQuantile(t *testing.T) {
	cases := []struct {
		name string
		xs   []float64
		p    float64
		want float64
	}{
		{"пусто — 0", nil, 0.5, 0},
		{"одно значение — оно же", []float64{7}, 0.25, 7},
		{"медиана нечётного — среднее значение", []float64{3, 1, 2}, 0.5, 2},
		{"медиана чётного — среднее двух средних", []float64{4, 1, 3, 2}, 0.5, 2.5},
		{"нижняя четверть — с интерполяцией", []float64{1, 2, 3, 4}, 0.25, 1.75},
		{"верхняя четверть", []float64{1, 2, 3, 4}, 0.75, 3.25},
		{"восемь недель фикстуры 101…108 — 104,5", []float64{101, 102, 103, 104, 105, 106, 107, 108}, 0.5, 104.5},
		{"p = 1 — максимум", []float64{5, 9, 1}, 1, 9},
	}
	for _, tc := range cases {
		if got := quantile(tc.xs, tc.p); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: quantile = %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
	xs := []float64{3, 1, 2}
	quantile(xs, 0.5)
	if !reflect.DeepEqual(xs, []float64{3, 1, 2}) {
		t.Errorf("quantile переставил выборку: %v", xs)
	}
}

func TestPulseForecast(t *testing.T) {
	mon, tue := time.Monday, time.Tuesday
	by := map[time.Weekday]pulseBand{mon: {Mid: 50, Low: 40, High: 60}}
	got := pulseForecast(100, pulseBand{Mid: 10, Low: 5, High: 20}, []time.Weekday{mon, tue, mon}, by)
	// уже есть 100 + остаток дня + два понедельника; вторника без истории нет в карте — 0
	if want := (pulseBand{Mid: 210, Low: 185, High: 240}); got != want {
		t.Errorf("прогноз = %+v, ожидалось %+v", got, want)
	}
	if got := pulseForecast(0.1, pulseBand{Mid: 0.2}, nil, nil); got.Mid != 0.3 {
		t.Errorf("последний день месяца: только уже есть и остаток дня, до копейки — %v", got.Mid)
	}
}

func TestPulseWeekOffsets(t *testing.T) {
	cur := day(2026, 11, 10) // вторник; 8 недель назад — 15.09
	at := func(m time.Month, d, h int) time.Time { return time.Date(2026, m, d, h, 0, 0, 0, time.UTC) }
	cases := []struct {
		name  string
		rh    int
		since time.Time
		want  []int
	}{
		{"истории нет — норм нет", 0, time.Time{}, []int{}},
		{"клуб работал уже 15.09 — все 8 недель", 0, at(9, 15, 10), []int{1, 2, 3, 4, 5, 6, 7, 8}},
		{"первое движение ровно в конце 15.09 — его неделя не в счёт", 0, at(9, 16, 0), []int{1, 2, 3, 4, 5, 6, 7}},
		{"открылись вечером 03.11 — одна неделя", 0, at(11, 3, 23), []int{1}},
		{"открылись 04.11 — недель нет", 0, at(11, 4, 0), []int{}},
		{"своя граница 08:00: сутки 03.11 идут до 04.11 08:00", 8, at(11, 4, 7), []int{1}},
	}
	for _, tc := range cases {
		if got := pulseWeekOffsets(cur, tc.rh, tc.since); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: недели %v, ожидалось %v", tc.name, got, tc.want)
		}
	}
}

func TestPulseMetric(t *testing.T) {
	pct := func(h map[string]any) any { // *float64 — числом, nil-указатель — nil
		if p, ok := h["pct"].(*float64); ok {
			if p == nil {
				return nil
			}
			return *p
		}
		return h["pct"]
	}
	m := pulseMetric(300, []float64{100, 200})
	if m["norm"] != 150.0 || pct(m) != 100.0 {
		t.Errorf("норма — медиана 150, сегодня 300 — +100%%: %v", m)
	}
	// фикстура проверки: восемь недель по 101…108 — норма 104,5; сегодня 50 — ниже нормы на 52,2%
	m = pulseMetric(50, []float64{108, 107, 106, 105, 104, 103, 102, 101})
	if m["norm"] != 104.5 || pct(m) != -52.2 || len(m["weeks"].([]float64)) != 8 {
		t.Errorf("норма 104,5 и −52,2%%: %v", m)
	}
	if m = pulseMetric(5, []float64{}); m["pct"] != nil || m["norm"] != 0.0 {
		t.Errorf("недель нет — отклонения нет: %v", m)
	}
	if m = pulseMetric(5, []float64{0, 0}); pct(m) != nil {
		t.Errorf("норма 0, сегодня 5 — процента нет: %v", m)
	}
	if m = pulseMetric(0, []float64{0, 0}); pct(m) != 0.0 {
		t.Errorf("норма 0 и сегодня 0 — 0%%: %v", m)
	}
}
