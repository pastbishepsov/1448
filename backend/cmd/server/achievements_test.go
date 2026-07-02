package main

import "testing"

func TestConditionMet(t *testing.T) {
	cases := []struct {
		name  string
		ctype string
		cval  string
		stats playerStats
		want  bool
	}{
		{"часы достигнуты", "hours_played", `{"min":10}`, playerStats{HoursPlayed: 10}, true},
		{"часов не хватает", "hours_played", `{"min":10}`, playerStats{HoursPlayed: 9}, false},
		{"часов с запасом", "hours_played", `{"min":1}`, playerStats{HoursPlayed: 100}, true},
		{"первый вход", "login_count", `{"min":1}`, playerStats{LoginCount: 1}, true},
		{"входов нет", "login_count", `{"min":1}`, playerStats{}, false},
		{"первый депозит", "deposit_count", `{"min":1}`, playerStats{DepositCount: 1}, true},
		{"депозитов нет", "deposit_count", `{"min":1}`, playerStats{HoursPlayed: 999, LoginCount: 999}, false},
		{"phone_verified пока не поддержан", "phone_verified", `{"verified":true}`, playerStats{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := conditionMet(tc.ctype, tc.cval, tc.stats); got != tc.want {
				t.Errorf("conditionMet(%q,%s,%+v) = %v, хотим %v",
					tc.ctype, tc.cval, tc.stats, got, tc.want)
			}
		})
	}
}
