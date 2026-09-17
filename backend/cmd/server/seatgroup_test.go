package main

import "testing"

func ipt(v int) *int { return &v }

// Ж8-и2: соседние машины — минимум суммарного попарного расстояния; без
// координат — в хвост.
func TestPickAdjacent(t *testing.T) {
	free := []pcPoint{
		{ID: "a", Name: "PC-01", X: ipt(0), Y: ipt(0)},
		{ID: "b", Name: "PC-02", X: ipt(1), Y: ipt(0)},
		{ID: "c", Name: "PC-05", X: ipt(6), Y: ipt(0)},
		{ID: "d", Name: "PC-06", X: ipt(7), Y: ipt(0)},
		{ID: "e", Name: "PC-07", X: ipt(8), Y: ipt(0)},
		{ID: "f", Name: "VIP-01"}, // без координат
	}
	got := pickAdjacent(free, 3)
	names := map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	if len(got) != 3 || !names["PC-05"] || !names["PC-06"] || !names["PC-07"] {
		t.Errorf("тройка должна быть PC-05/06/07 (подряд), получили %+v", got)
	}
	got = pickAdjacent(free, 2)
	names = map[string]bool{}
	for _, p := range got {
		names[p.Name] = true
	}
	if len(got) != 2 || !((names["PC-01"] && names["PC-02"]) || (names["PC-05"] && names["PC-06"]) || (names["PC-06"] && names["PC-07"])) {
		t.Errorf("пара должна быть соседней, получили %+v", got)
	}
	// координат не хватает: две с координатами + одна без
	got = pickAdjacent(free[:2], 3)
	if len(got) != 2 {
		t.Errorf("из двух машин трёх не собрать: %+v", got)
	}
	got = pickAdjacent(append(free[:2:2], free[5]), 3)
	if len(got) != 3 || got[2].Name != "VIP-01" {
		t.Errorf("машина без координат идёт последней: %+v", got)
	}
	if pickAdjacent(nil, 2) != nil {
		t.Errorf("пусто → nil")
	}
}

// Ж8-и3: пустая настройка — табло выключено; токен сравнивается целиком.
func TestBoardTokenOK(t *testing.T) {
	cases := []struct {
		setting, presented string
		want               bool
	}{
		{"", "", false}, {"", "abc", false}, {"abc", "", false},
		{"abc", "abc", true}, {"abc", "abd", false}, {"abc", "ab", false}, {" abc ", "abc", true},
	}
	for _, tc := range cases {
		if got := boardTokenOK(tc.setting, tc.presented); got != tc.want {
			t.Errorf("boardTokenOK(%q,%q) = %v, ожидалось %v", tc.setting, tc.presented, got, tc.want)
		}
	}
	tok, err := newBoardToken()
	if err != nil || len(tok) != 48 {
		t.Errorf("newBoardToken: %q %v", tok, err)
	}
}
