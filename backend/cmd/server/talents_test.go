package main

import "testing"

func TestCanInvestTalent(t *testing.T) {
	cases := []struct {
		name                                          string
		userLevel, sp, cur, max, minLvl              int
		wantOK                                        bool
		wantCode                                      string
	}{
		{"ок: можно вложить", 10, 2, 1, 5, 1, true, ""},
		{"нет очков", 10, 0, 1, 5, 1, false, "no_skillpoints"},
		{"уровень мал", 2, 1, 0, 5, 5, false, "level_locked"},
		{"максимум", 10, 1, 5, 5, 1, false, "maxed"},
		{"приоритет: нет очков важнее блокировки уровня", 1, 0, 0, 5, 10, false, "no_skillpoints"},
		{"граница: ровно минимальный уровень", 5, 1, 0, 5, 5, true, ""},
		{"граница: предпоследний уровень", 10, 1, 4, 5, 1, true, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ok, code := canInvestTalent(tc.userLevel, tc.sp, tc.cur, tc.max, tc.minLvl)
			if ok != tc.wantOK || code != tc.wantCode {
				t.Errorf("canInvestTalent(lvl=%d,sp=%d,cur=%d,max=%d,min=%d) = (%v,%q), хотим (%v,%q)",
					tc.userLevel, tc.sp, tc.cur, tc.max, tc.minLvl, ok, code, tc.wantOK, tc.wantCode)
			}
		})
	}
}
