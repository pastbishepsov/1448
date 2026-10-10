package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З13-и5: оценка пакетов до журнала по дням — дни в сумме дают оценку периода до гроша.
func TestSpreadEstimate(t *testing.T) {
	for _, tc := range []struct {
		minutes []int64
		avg     float64
		total   int64
		want    []int64
	}{
		{[]int64{0, 10, 5, 0, 7}, 2.3333, 51, []int64{0, 23, 12, 0, 16}}, // 23,333 + 11,667 + 16,333: грош — самому большому остатку
		{[]int64{3, 3, 3}, 1.0 / 3, 3, []int64{1, 1, 1}},
		{[]int64{4}, 2.5, 10, []int64{10}},
		{[]int64{0, 0}, 7, 0, []int64{0, 0}},
		{[]int64{1, 1}, 0.5, 1, []int64{1, 0}}, // поровну — первому по порядку
		{[]int64{2, 1}, 0.999999, 3, []int64{2, 1}},
	} {
		got := spreadEstimate(tc.minutes, tc.avg, tc.total)
		var sum int64
		for _, g := range got {
			sum += g
		}
		if !reflect.DeepEqual(got, tc.want) || sum != tc.total {
			t.Errorf("spreadEstimate(%v, %v, %d) = %v; ждали %v", tc.minutes, tc.avg, tc.total, got, tc.want)
		}
	}
}

// З13-и5: номера KP/KW — с начала месяца (по умолчанию), с начала года или без номеров.
func TestKpkw(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Warsaw")
	d := func(y int, m time.Month, day int) time.Time { return time.Date(y, m, day, 0, 0, 0, 0, loc) }
	if got := kpkwFrom(d(2026, 10, 15), kpkwMonth); !got.Equal(d(2026, 10, 1)) {
		t.Errorf("с начала месяца: %v", got)
	}
	if got := kpkwFrom(d(2026, 10, 15), kpkwYear); !got.Equal(d(2026, 1, 1)) {
		t.Errorf("с начала года: %v", got)
	}
	if got := kpkwFrom(d(2026, 10, 15), kpkwNone); !got.Equal(d(2026, 10, 15)) {
		t.Errorf("без номеров: %v", got)
	}
	for _, tc := range []struct {
		day    time.Time
		scheme int
		want   bool
	}{{d(2026, 10, 1), kpkwMonth, true}, {d(2026, 10, 2), kpkwMonth, false}, {d(2026, 10, 1), kpkwYear, false}, {d(2026, 1, 1), kpkwYear, true}} {
		if got := kpkwReset(tc.day, tc.scheme); got != tc.want {
			t.Errorf("kpkwReset(%s, %d) = %v", tc.day.Format("2006-01-02"), tc.scheme, got)
		}
	}
	if kpkwNo("KP", 3, d(2026, 10, 15), kpkwMonth) != "KP 3/10/2026" || kpkwNo("KW", 1, d(2026, 2, 3), kpkwMonth) != "KW 1/02/2026" ||
		kpkwNo("KW", 12, d(2026, 10, 15), kpkwYear) != "KW 12/2026" || kpkwNo("KP", 1, d(2026, 10, 15), kpkwNone) != "" {
		t.Errorf("номера документов кассы")
	}
}

// З13-и5: документы дня кассы — наличные за день по видам первыми, остальное по времени, расхождение последним; в
// сумме — остаток на конец.
func TestCashDocs(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Warsaw")
	at := func(h, m int) time.Time { return time.Date(2026, 10, 6, h, m, 0, 0, loc) }
	boss, guest, cat := uuid.New(), uuid.New(), uuid.New()
	t1, t3, t4, t5, t6 := at(9, 0), at(11, 0), at(12, 0), at(13, 0), at(14, 0)
	bank, opening, stated := "bank", int64(10000), int64(1000)
	d := bookDay{bookWindow: bookWindow{Day: time.Date(2026, 10, 6, 0, 0, 0, 0, loc), From: at(8, 0), To: at(23, 0)},
		Flow: drawerFlow{Deposits: 8000, Packs: 5000}, Opening: &opening, Diff: -100,
		Refunds: []returnRow{{Kind: "sale", ID: uuid.New(), Name: "Kola", Qty: 2, UserID: &guest, RefundGrosz: 1200, Method: "cash",
			VoidedAt: at(10, 0), VoidedBy: &boss}},
		Tills:  []*models.Expense{{ID: uuid.New(), CategoryID: cat, Note: "kurier", DocNo: "FV 1", GrossGrosz: 740, DrawerAt: &t1, CreatedBy: &boss}},
		Owners: []*models.OwnerRepayment{{ID: uuid.New(), AmountGrosz: 500, CreatedAt: t3}, {ID: uuid.New(), AmountGrosz: 900, CreatedAt: t3, VoidedAt: &t6}},
		Events: []*models.CashEvent{{ID: uuid.New(), Kind: models.CashEventCollect, AmountGrosz: 200, Dest: &bank, CreatedAt: t4},
			{ID: uuid.New(), Kind: models.CashEventCorrect, AmountGrosz: 300, StatedGrosz: &stated, CreatedAt: t5},
			{ID: uuid.New(), Kind: models.CashEventCollect, AmountGrosz: 700, CreatedAt: t5, VoidedAt: &t6}},
	}
	b := cashBook{Nick: map[string]string{boss.String(): "boss", guest.String(): "Guest43"}, Cats: map[string]string{cat.String(): "Inne"}}
	docs := cashDocs(d, b, 1)
	type short struct {
		Kind string
		Amt  int64
		Who  string
	}
	got := []short{}
	bal := opening
	for _, x := range docs {
		got = append(got, short{x.Kind, x.Amt, x.Who})
		if x.Kind == "KP" {
			bal += x.Amt
		} else {
			bal -= x.Amt
		}
	}
	want := []short{{"KP", 8000, ""}, {"KP", 5000, ""}, {"KW", 740, "boss"}, {"KW", 1200, "boss"}, {"KW", 500, ""}, {"KW", 200, ""},
		{"KP", 300, ""}, {"KW", 100, ""}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("документы дня: %+v; ждали %+v", got, want)
	}
	if bal != 10000+8000+5000-740-1200-500-200+300-100 {
		t.Errorf("остаток после документов: %d", bal)
	}
	for i, wantText := range []string{"Doładowania portfeli — gotówka za dzień", "Pakiety czasu — gotówka za dzień",
		"Wydatek: Inne: kurier · FV 1", "Zwrot: sprzedaż «Kola» ×2 · Guest43", "Zwrot właścicielowi",
		"Wypłata z kasy (inkaso) do banku", "Korekta stanu kasy: podano 10,00 zł", "Różnica kasowa dnia: niedobór"} {
		if docs[i].Text != wantText {
			t.Errorf("документ %d: %q; ждали %q", i, docs[i].Text, wantText)
		}
	}
	if n, kw := kpkwCounts([]bookDay{d}, kpkwMonth); n != 3 || kw != 5 {
		t.Errorf("KP и KW дня: %d и %d; ждали 3 и 5", n, kw)
	}
}

// З13-и5: куски строк поставки из ответа «Закупок» — с ценой и без.
func TestSupplyPiecesOf(t *testing.T) {
	row := gin.H{"lines": []gin.H{
		{"name": "Kola", "pieces": []gin.H{{"qty": 6, "left": 6, "price_pln": 2.5}, {"qty": 4, "left": 1, "price_pln": 2.8}}},
		{"name": "Chipsy", "pieces": []gin.H{{"qty": 3, "left": 3, "price_pln": nil}, {"qty": 0, "left": 0, "price_pln": nil}}},
	}}
	got := supplyPiecesOf(row)
	if len(got) != 3 || got[0].Name != "Kola" || got[0].Qty != 6 || *got[0].Price != 250 || *got[1].Price != 280 ||
		got[2].Name != "Chipsy" || got[2].Price != nil {
		t.Errorf("куски поставки: %+v", got)
	}
}

// З13-и5: «на конец периода» — конец последних суток, у идущего периода — сейчас; итоги предоплат и долгов.
func TestPeriodEndAndOwed(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Warsaw")
	sep := makePeriod(time.Date(2026, 9, 1, 0, 0, 0, 0, loc), time.Date(2026, 9, 30, 0, 0, 0, 0, loc), 8, "")
	now := time.Date(2026, 10, 10, 12, 0, 0, 0, loc)
	if got := periodEnd(sep, now); !got.Equal(time.Date(2026, 10, 1, 8, 0, 0, 0, loc)) {
		t.Errorf("конец сентября: %v", got)
	}
	oct := makePeriod(time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 10, 31, 0, 0, 0, 0, loc), 8, "")
	if got := periodEnd(oct, now); !got.Equal(now) {
		t.Errorf("идущий октябрь — сейчас: %v", got)
	}
	o := owedAt{Wallets: []walletAt{{Grosz: 5000}, {Grosz: 1666}}, Packs: []packAt{{Minutes: 90, Grosz: 2500}, {Minutes: 60, Gift: true}},
		Debts: []receivable{{BarGrosz: 600, KitchenGrosz: 1000}}}
	if w, p, m, d := o.sums(); w != 6666 || p != 2500 || m != 150 || d != 1600 {
		t.Errorf("итоги: %d %d %d %d", w, p, m, d)
	}
	sh := sheetOwed(o, now, 0)
	last := sh.rows[len(sh.rows)-4:]
	if sh.n != 5 || last[0].cells[6].n != 6666 || last[1].cells[5].n != 150 || last[2].cells[6].n != 9166 || last[3].cells[6].n != 1600 ||
		!strings.HasPrefix(last[1].cells[0].s, "Пакеты — итого") {
		t.Errorf("лист предоплат: %d строк, итоги %+v", sh.n, last)
	}
}
