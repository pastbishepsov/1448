package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
)

// партии теста: A, B, C — по порядку FIFO
var (
	lotA = uuid.MustParse("00000000-0000-0000-0000-00000000000a")
	lotB = uuid.MustParse("00000000-0000-0000-0000-00000000000b")
	lotC = uuid.MustParse("00000000-0000-0000-0000-00000000000c")
)

func lotName(id uuid.UUID) string {
	switch id {
	case lotA:
		return "A"
	case lotB:
		return "B"
	case lotC:
		return "C"
	}
	return id.String()
}

func partsStr(ps []lotPart) string {
	s := ""
	for _, p := range ps {
		s += fmt.Sprintf("%s:%d ", lotName(p.ID), p.Qty)
	}
	return s
}

// З10-и2: списание — с первой партии, где что-то осталось.
func TestLotFIFO(t *testing.T) {
	cases := []struct {
		name string
		lots []lotLeft
		qty  int
		want string
		err  error
	}{
		{"одна партия, хватает", []lotLeft{{lotA, 10}}, 3, "A:3 ", nil},
		{"частичные партии — по порядку", []lotLeft{{lotA, 2}, {lotB, 5}, {lotC, 4}}, 6, "A:2 B:4 ", nil},
		{"ровно всё", []lotLeft{{lotA, 2}, {lotB, 3}}, 5, "A:2 B:3 ", nil},
		{"пустые пропускаются", []lotLeft{{lotA, 0}, {lotB, 3}}, 2, "B:2 ", nil},
		{"ноль штук — ничего", []lotLeft{{lotA, 5}}, 0, "", nil},
		{"не хватает", []lotLeft{{lotA, 2}, {lotB, 1}}, 4, "", errLotsShort},
		{"нулевой остаток во всех", []lotLeft{{lotA, 0}, {lotB, 0}}, 1, "", errLotsShort},
		{"партий нет", nil, 1, "", errLotsShort},
	}
	for _, c := range cases {
		got, err := lotFIFO(c.lots, c.qty)
		if !errors.Is(err, c.err) || partsStr(got) != c.want {
			t.Errorf("%s: %q %v, ждали %q %v", c.name, partsStr(got), err, c.want, c.err)
		}
	}
}

// З10-и2: возврат — в те же партии, последняя тронутая — первой; частичные отмены.
func TestLotBack(t *testing.T) {
	taken := []lotPart{{lotA, 2}, {lotB, 4}}
	cases := []struct {
		name     string
		taken    []lotPart
		returned []lotPart
		qty      int
		want     string
		err      error
	}{
		{"всё — туда же, в обратном порядке", taken, nil, 6, "B:4 A:2 ", nil},
		{"часть — из последней тронутой", taken, nil, 3, "B:3 ", nil},
		{"остаток после частичной отмены", taken, []lotPart{{lotB, 3}}, 3, "B:1 A:2 ", nil},
		{"одна партия дважды — одной строкой", []lotPart{{lotA, 1}, {lotA, 2}}, nil, 3, "A:3 ", nil},
		{"больше, чем взяло", taken, nil, 7, "", errLotsBack},
		{"уже вернули всё", taken, []lotPart{{lotA, 2}, {lotB, 4}}, 1, "", errLotsBack},
		{"вернули в партию больше, чем взяли из неё", taken, []lotPart{{lotA, 3}}, 1, "", errLotsBack},
		{"вернули туда, откуда не брали", taken, []lotPart{{lotC, 1}}, 1, "", errLotsBack},
		{"ноль штук — ничего", taken, nil, 0, "", nil},
	}
	for _, c := range cases {
		got, err := lotBack(c.taken, c.returned, c.qty)
		if !errors.Is(err, c.err) || partsStr(got) != c.want {
			t.Errorf("%s: %q %v, ждали %q %v", c.name, partsStr(got), err, c.want, c.err)
		}
	}
}

func piecesStr(ps []lotPiece) string {
	s := ""
	for _, p := range ps {
		s += fmt.Sprintf("%d/%d@%d ", p.Left, p.In, p.Price)
	}
	return s
}

func allocsStr(as [][]lotAlloc) string {
	s := ""
	for i, a := range as {
		if i > 0 {
			s += "| "
		}
		for _, x := range a {
			s += fmt.Sprintf("%d:%+d ", x.Piece, x.Qty)
		}
	}
	return s
}

// З10-и2: разбивка строки поставки на цены — проданное по первой цене, возврат туда, откуда взяли.
func TestLotSplit(t *testing.T) {
	m := func(n byte) uuid.UUID { return uuid.UUID{15: n} }
	in, s1, s2, s0 := m(1), m(2), m(3), m(9)
	two := []lotPrice{{20, 250}, {10, 280}}
	cases := []struct {
		name    string
		prices  []lotPrice
		entries []lotEntry
		pieces  string
		allocs  string
		err     error
	}{
		{"до продаж — приход по кускам", two, []lotEntry{{Move: in, Qty: 30, In: true}},
			"20/20@250 10/10@280 ", "0:+20 1:+10 ", nil},
		{"проданное — по первой цене", two, []lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -12}},
			"8/20@250 10/10@280 ", "0:+20 1:+10 | 0:-12 ", nil},
		{"продано больше первого куска — остальное из второго", two,
			[]lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -12}, {Move: s2, Qty: -13}},
			"0/20@250 5/10@280 ", "0:+20 1:+10 | 0:-12 | 0:-8 1:-5 ", nil},
		{"возврат — в те куски, откуда взяло, последний тронутый первым", two,
			[]lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -12}, {Move: s2, Qty: -13}, {Move: m(4), Qty: 13, Source: &s2}},
			"8/20@250 10/10@280 ", "0:+20 1:+10 | 0:-12 | 0:-8 1:-5 | 1:+5 0:+8 ", nil},
		{"частичный возврат — из последнего тронутого куска", two,
			[]lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -25}, {Move: m(4), Qty: 3, Source: &s1}},
			"0/20@250 8/10@280 ", "0:+20 1:+10 | 0:-20 1:-5 | 1:+3 ", nil},
		{"возврат продажи до партий (источник не в строке) — в первый кусок", two,
			[]lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -30}, {Move: m(4), Qty: 2, Source: &s0}},
			"2/20@250 0/10@280 ", "0:+20 1:+10 | 0:-20 1:-10 | 0:+2 ", nil},
		{"возврат без источника — в первый кусок", two,
			[]lotEntry{{Move: in, Qty: 30, In: true}, {Move: m(4), Qty: 1}},
			"21/20@250 10/10@280 ", "0:+20 1:+10 | 0:+1 ", nil},
		{"всё продано — нулевой остаток", two, []lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -30}},
			"0/20@250 0/10@280 ", "0:+20 1:+10 | 0:-20 1:-10 ", nil},
		{"одна цена — один кусок", []lotPrice{{30, 199}}, []lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -4}},
			"26/30@199 ", "0:+30 | 0:-4 ", nil},
		{"цена ноль — можно (подарок поставщика)", []lotPrice{{5, 0}}, []lotEntry{{Move: in, Qty: 5, In: true}},
			"5/5@0 ", "0:+5 ", nil},
		{"два прихода в строке — по кускам по порядку", []lotPrice{{15, 100}, {15, 120}},
			[]lotEntry{{Move: in, Qty: 10, In: true}, {Move: m(5), Qty: 20, In: true}},
			"15/15@100 15/15@120 ", "0:+10 | 0:+5 1:+15 ", nil},
		{"штуки кусков не сходятся с «пришло»", two, []lotEntry{{Move: in, Qty: 29, In: true}}, "", "", errLotSplit},
		{"кусок без штук", []lotPrice{{30, 250}, {0, 280}}, []lotEntry{{Move: in, Qty: 30, In: true}}, "", "", errLotSplit},
		{"цена меньше нуля", []lotPrice{{30, -1}}, []lotEntry{{Move: in, Qty: 30, In: true}}, "", "", errLotSplit},
		{"цен нет", nil, []lotEntry{{Move: in, Qty: 30, In: true}}, "", "", errLotSplit},
		{"списали больше, чем было", two, []lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -31}}, "", "", errLotSplit},
	}
	for _, c := range cases {
		ps, as, err := lotSplit(c.prices, c.entries)
		if !errors.Is(err, c.err) || piecesStr(ps) != c.pieces || allocsStr(as) != c.allocs {
			t.Errorf("%s: %q %q %v, ждали %q %q %v", c.name, piecesStr(ps), allocsStr(as), err, c.pieces, c.allocs, c.err)
		}
	}
}

// З10-и2: возврат после разбивки — продажа до оценки, отмена после: товар идёт в куски, куда
// разбивка перенесла её списание (lotSplit → доли движения → lotBack).
func TestLotBackAfterSplit(t *testing.T) {
	in, s1 := uuid.UUID{15: 1}, uuid.UUID{15: 2}
	ps, as, err := lotSplit([]lotPrice{{20, 250}, {10, 280}}, []lotEntry{{Move: in, Qty: 30, In: true}, {Move: s1, Qty: -25}})
	if err != nil || piecesStr(ps) != "0/20@250 5/10@280 " {
		t.Fatalf("разбивка: %q %v", piecesStr(ps), err)
	}
	pieceID := []uuid.UUID{lotA, lotB} // куски строки — партии A и B
	taken := []lotPart{}
	for _, a := range as[1] {
		taken = append(taken, lotPart{pieceID[a.Piece], -a.Qty})
	}
	got, err := lotBack(taken, nil, 25)
	if err != nil || partsStr(got) != "B:5 A:20 " {
		t.Errorf("отмена после разбивки: %q %v, ждали «B:5 A:20»", partsStr(got), err)
	}
	got, err = lotBack(taken, []lotPart{{lotB, 5}}, 4)
	if err != nil || partsStr(got) != "A:4 " {
		t.Errorf("вторая частичная отмена: %q %v, ждали «A:4»", partsStr(got), err)
	}
}
