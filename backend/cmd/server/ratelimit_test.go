package main

import (
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter()
	cur := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	rl.now = func() time.Time { return cur }

	// burst из 20 мгновенных запросов проходит
	for i := 0; i < 20; i++ {
		if !rl.Allow("ip1") {
			t.Fatalf("burst: запрос %d отклонён", i+1)
		}
	}
	// 21-й в тот же момент — отклонён
	if rl.Allow("ip1") {
		t.Error("21-й мгновенный запрос прошёл")
	}
	// другой IP независим
	if !rl.Allow("ip2") {
		t.Error("другой IP пострадал от чужого лимита")
	}
	// через секунду накапает ровно ~10 токенов
	cur = cur.Add(time.Second)
	ok := 0
	for i := 0; i < 15; i++ {
		if rl.Allow("ip1") {
			ok++
		}
	}
	if ok != 10 {
		t.Errorf("после секунды прошло %d запросов, ожидалось 10", ok)
	}
}
