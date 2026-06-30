package main

import "testing"

func TestConditionMet(t *testing.T) {
	cases := []struct {
		name   string
		ctype  string
		cval   string
		hours  int
		logins int
		want   bool
	}{
		{"часы достигнуты", "hours_played", `{"min":10}`, 10, 0, true},
		{"часов не хватает", "hours_played", `{"min":10}`, 9, 0, false},
		{"часов с запасом", "hours_played", `{"min":1}`, 100, 0, true},
		{"первый вход", "login_count", `{"min":1}`, 0, 1, true},
		{"входов нет", "login_count", `{"min":1}`, 0, 0, false},
		{"неизвестный тип игнорируется", "deposit_count", `{"min":1}`, 999, 999, false},
		{"phone_verified пока не поддержан", "phone_verified", `{"verified":true}`, 0, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conditionMet(tc.ctype, tc.cval, tc.hours, tc.logins); got != tc.want {
				t.Errorf("conditionMet(%q,%s,h=%d,l=%d) = %v, хотим %v",
					tc.ctype, tc.cval, tc.hours, tc.logins, got, tc.want)
			}
		})
	}
}
