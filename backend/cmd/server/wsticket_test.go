package main

// Ж0-и5: тикет живого канала — одноразовый и короткоживущий.

import (
	"testing"
	"time"
)

func TestWSTicketStore(t *testing.T) {
	s := newWSTicketStore()
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	tk, err := s.issue("u1", "admin", now)
	if err != nil || len(tk) < 20 {
		t.Fatalf("issue: %v %q", err, tk)
	}
	if v, ok := s.redeem(tk, now.Add(5*time.Second)); !ok || v.UserID != "u1" || v.Role != "admin" {
		t.Fatalf("первое гашение должно пройти: %v %+v", ok, v)
	}
	if _, ok := s.redeem(tk, now.Add(6*time.Second)); ok {
		t.Error("второе гашение того же тикета должно провалиться")
	}
	late, _ := s.issue("u2", "owner", now)
	if _, ok := s.redeem(late, now.Add(wsTicketTTL)); ok {
		t.Error("просроченный тикет должен провалиться")
	}
	if _, ok := s.redeem("no-such-ticket", now); ok {
		t.Error("чужой тикет должен провалиться")
	}
	// выдача подметает протухшие: старый тикет исчезает из карты
	old, _ := s.issue("u3", "admin", now)
	_, _ = s.issue("u4", "admin", now.Add(wsTicketTTL+time.Second))
	if _, ok := s.redeem(old, now.Add(wsTicketTTL+2*time.Second)); ok {
		t.Error("протухший тикет пережил уборку")
	}
	a, _ := s.issue("u5", "admin", now)
	b, _ := s.issue("u5", "admin", now)
	if a == b {
		t.Error("тикеты должны быть случайными")
	}
}
