package main

import "testing"

// З15-и2: на открытии — все непрочитанные, но не меньше страницы и не больше staffChatMax.
func TestStaffPageSize(t *testing.T) {
	for _, c := range []struct {
		after int64
		want  int
	}{{0, 100}, {1, 100}, {100, 100}, {101, 101}, {130, 130}, {499, 499}, {500, 500}, {501, 500}, {5000, 500}} {
		if got := staffPageSize(c.after); got != c.want {
			t.Errorf("staffPageSize(%d) = %d, want %d", c.after, got, c.want)
		}
	}
}

// З15-и2: необъяснённая часть расхождения — недостачу закрывают расходы «из кассы», излишек — нет.
func TestCashDiffLeft(t *testing.T) {
	for _, c := range []struct {
		name            string
		diff, explained int64
		want            int64
	}{
		{"недостача без расходов", -1200, 0, -1200},
		{"недостача объяснена частично", -1200, 500, -700},
		{"недостача объяснена ровно", -1200, 1200, 0},
		{"объяснили больше — объяснено, не излишек", -1200, 2000, 0},
		{"излишек расходом не объяснить", 300, 300, 300},
		{"излишек", 250, 0, 250},
	} {
		if got := cashDiffLeft(c.diff, c.explained); got != c.want {
			t.Errorf("%s: cashDiffLeft(%d, %d) = %d, want %d", c.name, c.diff, c.explained, got, c.want)
		}
	}
}
