package main

// К6-и1 (KIOSK.md): вход по QR — жизненный цикл кода без ожиданий (now
// параметром): ждёт → подтверждён → выдан ровно один раз; TTL; лимит 6/мин.

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestQRCodeShape(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		code, err := qrRandomCode(qrCodeLen)
		if err != nil || len(code) != qrCodeLen {
			t.Fatalf("код: %q %v", code, err)
		}
		for _, ch := range code {
			if !strings.ContainsRune(qrCodeAlphabet, ch) {
				t.Fatalf("символ вне алфавита: %q", code)
			}
		}
		seen[code] = true
	}
	if len(seen) < 199 {
		t.Errorf("коды не случайны: %d уникальных из 200", len(seen))
	}
	for _, bad := range "0O1I" {
		if strings.ContainsRune(qrCodeAlphabet, bad) {
			t.Errorf("похожий символ %q в алфавите", bad)
		}
	}
	if normalizeQRCode(" ab7k2xyz ") != "AB7K2XYZ" {
		t.Error("normalizeQRCode: регистр и пробелы")
	}
}

func TestQRLifecycle(t *testing.T) {
	s := newQRStore()
	pc := uuid.New()
	guest := uuid.New()
	t0 := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)

	q, err := s.create(pc, t0)
	if err != nil || q.Code == "" || q.ComputerID != pc {
		t.Fatalf("create: %+v %v", q, err)
	}
	if got := s.get(q.Code, t0.Add(10*time.Second)); got == nil || got.UserID != nil {
		t.Fatalf("свежий код должен ждать: %+v", got)
	}
	if _, ok := s.consume(q.Code, t0.Add(10*time.Second)); ok {
		t.Error("до подтверждения токены не выдаются")
	}
	if _, ok := s.approve("NOPE1234", guest, t0); ok {
		t.Error("чужой код не подтверждается")
	}
	if _, ok := s.approve(q.Code, guest, t0.Add(30*time.Second)); !ok {
		t.Fatal("подтверждение ожидающего кода")
	}
	if _, ok := s.approve(q.Code, uuid.New(), t0.Add(31*time.Second)); ok {
		t.Error("второй телефон тот же код не подтверждает")
	}
	if got := s.get(q.Code, t0.Add(32*time.Second)); got == nil || got.UserID == nil || *got.UserID != guest {
		t.Fatalf("после approve код жив и знает гостя: %+v", got)
	}
	c1, ok := s.consume(q.Code, t0.Add(33*time.Second))
	if !ok || c1.UserID == nil || *c1.UserID != guest || !c1.Consumed {
		t.Fatalf("первый опрос забирает код: %+v %v", c1, ok)
	}
	if _, ok := s.consume(q.Code, t0.Add(34*time.Second)); ok {
		t.Error("второй опрос — пусто (одноразовый)")
	}
	if s.get(q.Code, t0.Add(34*time.Second)) != nil {
		t.Error("использованный код не виден")
	}

	// TTL: через 120 с код сгорает и не подтверждается
	q2, _ := s.create(pc, t0)
	if s.get(q2.Code, t0.Add(qrTTL-time.Second)) == nil {
		t.Error("за секунду до TTL код жив")
	}
	if s.get(q2.Code, t0.Add(qrTTL)) != nil {
		t.Error("на TTL код сгорел")
	}
	if _, ok := s.approve(q2.Code, guest, t0.Add(qrTTL)); ok {
		t.Error("сгоревший код не подтверждается")
	}
	// подтверждён, но киоск не забрал до TTL — тоже сгорает
	q3, _ := s.create(pc, t0)
	s.approve(q3.Code, guest, t0.Add(time.Minute))
	if _, ok := s.consume(q3.Code, t0.Add(qrTTL+time.Second)); ok {
		t.Error("подтверждённый, но просроченный код не выдаёт токены")
	}
	// уборка: сгоревшие исчезают из карты
	s.allowCreate("1.1.1.1", t0.Add(qrTTL+time.Minute))
	if len(s.codes) != 0 {
		t.Errorf("после sweep карта пуста, а в ней %d", len(s.codes))
	}
}

func TestQRCreateLimit(t *testing.T) {
	s := newQRStore()
	t0 := time.Date(2026, 9, 7, 16, 0, 0, 0, time.UTC)
	for i := 0; i < qrCreatePerMin; i++ {
		if !s.allowCreate("10.0.0.1", t0.Add(time.Duration(i)*5*time.Second)) {
			t.Fatalf("запрос %d в пределах лимита отклонён", i+1)
		}
	}
	if s.allowCreate("10.0.0.1", t0.Add(40*time.Second)) {
		t.Error("седьмой за минуту — отказ")
	}
	if !s.allowCreate("10.0.0.2", t0.Add(40*time.Second)) {
		t.Error("другой IP — своё окно")
	}
	if !s.allowCreate("10.0.0.1", t0.Add(61*time.Second)) {
		t.Error("через минуту после первого — окно освободилось")
	}
}

func TestQRCollisionRetry(t *testing.T) {
	s := newQRStore()
	calls := 0
	s.random = func(n int) (string, error) {
		calls++
		if calls == 1 {
			return "SAMESAME", nil
		}
		return "OTHER222", nil
	}
	t0 := time.Now()
	pc := uuid.New()
	s.codes["SAMESAME"] = &qrLogin{Code: "SAMESAME", ComputerID: pc, ExpiresAt: t0.Add(qrTTL)}
	q, err := s.create(pc, t0)
	if err != nil || q.Code != "OTHER222" {
		t.Errorf("коллизия должна перебросить кубик: %+v %v", q, err)
	}
}
