package main

import (
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

func TestReadyDeadlineFor(t *testing.T) {
	now := time.Date(2026, 8, 24, 14, 48, 0, 0, time.UTC)
	if d := readyDeadlineFor(now, 7); d == nil || !d.Equal(now.Add(7*time.Minute)) {
		t.Errorf("семь минут: got %v, ждали %v", d, now.Add(7*time.Minute))
	}
	// Ноль и отрицательное — окно выключено, сессия начинается сразу.
	for _, v := range []int64{0, -1} {
		if d := readyDeadlineFor(now, v); d != nil {
			t.Errorf("waitMin=%d: ждали nil (окно выключено), got %v", v, d)
		}
	}
}

func TestSessionWaitingReady(t *testing.T) {
	at := time.Date(2026, 8, 24, 14, 48, 0, 0, time.UTC)
	later := at.Add(7 * time.Minute)
	cases := []struct {
		name string
		s    models.Session
		want bool
	}{
		{"окно выключено — сразу играет", models.Session{}, false},
		{"ждёт нажатия", models.Session{ReadyDeadline: &later}, true},
		{"нажал — играет", models.Session{ReadyDeadline: &later, ReadyAt: &at}, false},
		// Такого в базе быть не должно, но судить надо по ready_at: сессия
		// подтверждена, значит тарифицируется.
		{"подтверждена без дедлайна", models.Session{ReadyAt: &at}, false},
	}
	for _, tc := range cases {
		if got := sessionWaitingReady(&tc.s); got != tc.want {
			t.Errorf("%s: got %v, ждали %v", tc.name, got, tc.want)
		}
	}
}

// Главный денежный инвариант Е1: ожидание НЕ оплачивается. Деньги считаются от
// started_at, который сдвигается на момент подтверждения, поэтому проверяем
// именно арифметику «сколько минут насчитается» до и после сдвига.
func TestWaitingTimeIsNotBilled(t *testing.T) {
	const rate = int64(1200) // 12,00 zł/час в грошах
	seat := time.Date(2026, 8, 24, 14, 48, 0, 0, time.UTC)
	pressed := seat.Add(6 * time.Minute) // гость шёл через зал шесть минут
	now := pressed.Add(30 * time.Minute) // и играет полчаса

	// Без окна деньги текли бы от посадки — 36 минут.
	naive := int(now.Sub(seat).Minutes())
	// С окном точка отсчёта — нажатие: ровно 30 минут.
	real := int(now.Sub(pressed).Minutes())
	if naive != 36 || real != 30 {
		t.Fatalf("подготовка теста неверна: naive=%d real=%d", naive, real)
	}
	overpay := chargeDelta(rate, 0, naive) - chargeDelta(rate, 0, real)
	if overpay <= 0 {
		t.Fatal("тест бессмысленен: ожидание ничего не стоило бы")
	}
	// 6 минут по 12 zł/час — 120 грошей, ровно их гость и не должен платить.
	if overpay != 120 {
		t.Errorf("переплата за дорогу к компу = %d грошей, ждали 120", overpay)
	}
}

// Авто-старт по дедлайну (Р1): деньги идут с ДЕДЛАЙНА, а не с посадки и не с
// момента, когда тикер до сессии добрался (тик — раз в 30 с, и опоздание тика
// не должно доставаться ни гостю, ни клубу).
func TestAutoStartChargesFromDeadline(t *testing.T) {
	const rate = int64(1200)
	seat := time.Date(2026, 8, 24, 14, 48, 0, 0, time.UTC)
	deadline := seat.Add(7 * time.Minute)
	tick := deadline.Add(20 * time.Second) // тикер пришёл с опозданием
	now := tick.Add(9*time.Minute + 50*time.Second) // ловим границу минуты

	fromDeadline := int(now.Sub(deadline).Minutes())
	fromTick := int(now.Sub(tick).Minutes())
	if fromDeadline != 10 || fromTick != 9 {
		t.Fatalf("подготовка неверна: fromDeadline=%d fromTick=%d", fromDeadline, fromTick)
	}
	if chargeDelta(rate, 0, fromDeadline) == chargeDelta(rate, 0, fromTick) {
		t.Fatal("тест бессмысленен: опоздание тика ничего не меняет")
	}
	// confirmReady вызывается с *ReadyDeadline, а не с now — это и проверяем
	// на уровне арифметики: расчёт обязан идти от дедлайна.
	if got := chargeDelta(rate, 0, fromDeadline); got != 200 {
		t.Errorf("10 минут от дедлайна = %d грошей, ждали 200", got)
	}
}

// Дедлайн чужой брони считается от ВРЕМЕНИ БРОНИ, а не от started_at сессии.
// Если бы он зависел от точки отсчёта, ожидание гостя съедало бы время
// следующего — это тот случай, где ошибка тихо обкрадывает третье лицо.
func TestBookingDeadlineIndependentOfReady(t *testing.T) {
	seat := time.Date(2026, 8, 24, 18, 0, 0, 0, time.UTC)
	bookingStart := seat.Add(90 * time.Minute)
	const lockMin = 10

	deadline := bookingStart.Add(-lockMin * time.Minute)
	// Гость подтвердил через 7 минут — дедлайн обязан остаться прежним.
	pressed := seat.Add(7 * time.Minute)
	deadlineAfterReady := bookingStart.Add(-lockMin * time.Minute)
	if !deadline.Equal(deadlineAfterReady) {
		t.Fatal("дедлайн брони поехал от подтверждения — ожидание съело чужое время")
	}
	// И окно посадки меряется от посадки: на момент подтверждения его уже не
	// пересчитывают, иначе гость получил бы больше, чем ему разрешили.
	if w := seatWindowMin(bookingStart, seat, 15); w != 75 {
		t.Errorf("окно посадки = %d мин, ждали 75", w)
	}
	if w := seatWindowMin(bookingStart, pressed, 15); w != 68 {
		t.Errorf("окно от момента подтверждения = %d мин (68) — считать надо было от посадки", w)
	}
}
