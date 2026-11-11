package main

import (
	"strings"
	"testing"
)

// З23-и1а: классы ABC, запас склада в днях и доли.

func TestAbcClasses(t *testing.T) {
	cases := []struct {
		name string
		sold []int64
		want string
	}{
		{"60 + 25 + 10 + 5: A — пока выше меньше 80 %, B — меньше 95 %", []int64{60, 25, 10, 5}, "A,A,B,C"},
		{"одна позиция даёт почти всё — она A, остальные C", []int64{99, 1}, "A,C"},
		{"поровну пять позиций: 0, 20, 40, 60 % выше — A; 80 % — B", []int64{20, 20, 20, 20, 20}, "A,A,A,A,B"},
		{"без продаж — без класса", []int64{10, 0, 0}, "A,,"},
		{"продаж нет ни у кого", []int64{0, 0}, ","},
	}
	for _, tc := range cases {
		if got := strings.Join(abcClasses(tc.sold), ","); got != tc.want {
			t.Errorf("%s: %q, ожидалось %q", tc.name, got, tc.want)
		}
	}
}

func TestCoverDays(t *testing.T) {
	if v := coverDays(20, 70, 7); v == nil || *v != 2 {
		t.Errorf("70 штук за 7 дней, склад 20 — на 2 дня: %v", v)
	}
	if v := coverDays(10, 3, 7); v == nil || *v != 23.3 {
		t.Errorf("3 штуки за неделю, склад 10 — 23,3 дня: %v", v)
	}
	if coverDays(10, 0, 7) != nil {
		t.Error("продаж не было — темпа нет, nil")
	}
	if v := coverDays(0, 5, 7); v == nil || *v != 0 {
		t.Errorf("склад пуст — 0 дней: %v", v)
	}
	if sharePct(1, 3) != 33.3 || sharePct(5, 0) != 0 {
		t.Error("sharePct: 1 из 3 — 33,3 %, от нуля — 0")
	}
}
