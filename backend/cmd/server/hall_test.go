package main

import "testing"

func TestPosInBounds(t *testing.T) {
	cases := []struct {
		name       string
		x, y, w, h int
		want       bool
	}{
		{"угол 0,0", 0, 0, 12, 8, true},
		{"дальний угол", 11, 7, 12, 8, true},
		{"за шириной", 12, 0, 12, 8, false},
		{"за высотой", 0, 8, 12, 8, false},
		{"отрицательный x", -1, 3, 12, 8, false},
		{"отрицательный y", 3, -1, 12, 8, false},
	}
	for _, tc := range cases {
		if got := posInBounds(tc.x, tc.y, tc.w, tc.h); got != tc.want {
			t.Errorf("%s: posInBounds(%d,%d,%d,%d) = %v, ожидалось %v",
				tc.name, tc.x, tc.y, tc.w, tc.h, got, tc.want)
		}
	}
}
