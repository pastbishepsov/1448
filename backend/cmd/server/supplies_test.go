package main

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// З10-и4: строки приёмки — повтор позиции при приёмке складывается, при правке —
// ошибка; штуки — от minQty до maxStockQty; строк — не больше maxSupplyLines.
func TestSupplyLines(t *testing.T) {
	a, b := uuid.NewString(), uuid.NewString()
	many := func(n int) []supplyLineReq {
		out := []supplyLineReq{}
		for i := 0; i < n; i++ {
			out = append(out, supplyLineReq{uuid.NewString(), 1})
		}
		return out
	}
	cases := []struct {
		name  string
		req   []supplyLineReq
		min   int
		merge bool
		want  []supplyLineReq
		code  string
	}{
		{"приёмка: повтор складывается, порядок — первого появления", []supplyLineReq{{a, 30}, {" " + b + " ", 20}, {a, 5}}, 1, true,
			[]supplyLineReq{{a, 35}, {b, 20}}, ""},
		{"правка: повтор — ошибка", []supplyLineReq{{a, 30}, {a, 30}}, 0, false, nil, "bad_lines"},
		{"правка: ноль можно", []supplyLineReq{{a, 0}}, 0, false, []supplyLineReq{{a, 0}}, ""},
		{"приёмка: ноль нельзя", []supplyLineReq{{a, 0}}, 1, true, nil, "bad_qty"},
		{"отрицательное", []supplyLineReq{{a, -1}}, 0, false, nil, "bad_qty"},
		{"выше потолка", []supplyLineReq{{a, maxStockQty + 1}}, 1, true, nil, "bad_qty"},
		{"сумма повторов выше потолка", []supplyLineReq{{a, maxStockQty}, {a, 1}}, 1, true, nil, "bad_qty"},
		{"пусто", nil, 1, true, nil, "bad_lines"},
		{"кривой id", []supplyLineReq{{"кривой", 1}}, 1, true, nil, "bad_lines"},
		{"строк больше предела", many(maxSupplyLines + 1), 1, true, nil, "bad_lines"},
		{"ровно предел", many(maxSupplyLines), 1, true, nil, ""},
	}
	for _, c := range cases {
		got, code := supplyLines(c.req, c.min, c.merge)
		if code != c.code {
			t.Errorf("%s: код %q, ждали %q", c.name, code, c.code)
			continue
		}
		if c.want != nil && (len(got) != len(c.want) || func() bool {
			for i := range got {
				if got[i] != c.want[i] {
					return true
				}
			}
			return false
		}()) {
			t.Errorf("%s: %v, ждали %v", c.name, got, c.want)
		}
	}
}

// З10-и4: заголовок поставки — по часам клуба, как бы ни пришло время из базы.
func TestSupplyTitle(t *testing.T) {
	at := time.Date(2026, 9, 28, 16, 40, 0, 0, time.UTC) // 18:40 в Варшаве (летнее время)
	got := supplyTitle(at)
	want := "поставка от " + at.In(clubLocation).Format("02.01 15:04")
	if got != want || !strings.HasPrefix(got, "поставка от 28.09 ") {
		t.Errorf("supplyTitle = %q, ждали %q", got, want)
	}
	if clubLocation.String() == "Europe/Warsaw" && got != "поставка от 28.09 18:40" {
		t.Errorf("по Варшаве: %q", got)
	}
}
