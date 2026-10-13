package main

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З13-и6: справочники форм учёта — ставки ryczałtu и колонки KPiR — и их настройки.
func TestRyczaltAndKpirChoices(t *testing.T) {
	for r, want := range map[int]string{150: "15 %", 85: "8,5 %", 125: "12,5 %", 30: "3 %", 20: "2 %"} {
		if got := models.RyczaltLabel(r); got != want {
			t.Errorf("RyczaltLabel(%d) = %q; ждали %q", r, got, want)
		}
	}
	if got := ryczaltRatesText(); got != "17 %, 15 %, 14 %, 12,5 %, 12 %, 10 %, 8,5 %, 5,5 %, 3 %, 2 %" {
		t.Errorf("ryczaltRatesText() = %q", got)
	}
	if !models.ValidRyczalt(85) || models.ValidRyczalt(86) || models.ValidRyczalt(0) || models.ValidRyczalt(8) {
		t.Errorf("ValidRyczalt: 8,5 %% — в десятых долях, 85")
	}
	if v, ok := ryczaltFromRequest(nil, 30); v != 30 || !ok {
		t.Errorf("не прислана — по умолчанию")
	}
	if bad := 33; func() bool { _, ok := ryczaltFromRequest(&bad, 30); return ok }() {
		t.Errorf("33 — не ставка ryczałtu")
	}
	if !models.ValidKpir(0) || !models.ValidKpir(13) || models.ValidKpir(11) || models.ValidKpir(16) ||
		models.KpirDefault(true) != 12 || models.KpirDefault(false) != 15 {
		t.Errorf("колонки KPiR: 12, 13, 14, 15 или 0; закупка — 12, остальное — 15")
	}
	if kpirText(0) != "не в книгу" || kpirText(14) != "14" || kpirColsText() != "12, 13, 14, 15, 0 — не в книгу" {
		t.Errorf("подписи колонок: %q, %q", kpirText(0), kpirColsText())
	}
	if settingDefaults["acc_form"] != formKpir || settingDefaults["acc_ryczalt_time"] != 150 {
		t.Errorf("по умолчанию: KPiR и 15 %% у времени")
	}
	choose := settingChoices["acc_ryczalt_time"]
	if choose == nil || !choose(85) || choose(86) || settingChoiceMsg["acc_ryczalt_time"] != ryczaltBadMessage ||
		settingChoiceMsg["acc_topup_vat"] != vatBadMessage {
		t.Errorf("ставка времени — только из ставок ryczałtu, ответ — свой")
	}
	if a, code := checkKpirAccount(nil, ptr(" 402-01 ")); a != "402-01" || code != "" {
		t.Errorf("счёт без краевых пробелов: %q %q", a, code)
	}
	if _, code := checkKpirAccount(ptr(11), nil); code != "bad_kpir_col" {
		t.Errorf("колонка 11: %q", code)
	}
	if _, code := checkKpirAccount(nil, ptr(strings.Repeat("4", 33))); code != "bad_account" {
		t.Errorf("счёт в 33 знака: %q", code)
	}
}

func ptr[T any](v T) *T { return &v }

// З13-и6: реквизиты фирмы — что задано; чего нет — название и адрес клуба.
func TestFirmOf(t *testing.T) {
	club := models.Club{Name: "14:48 Białołęka", Address: "Warszawa, ul. Demo 1"}
	if n, a := firmOf(requisites{}, club); n != club.Name || a != club.Address {
		t.Errorf("пустые реквизиты: %q, %q", n, a)
	}
	if n, a := firmOf(requisites{Name: "Jan Nowak Gry", Address: "ul. Firmowa 2"}, club); n != "Jan Nowak Gry" || a != "ul. Firmowa 2" {
		t.Errorf("свои реквизиты: %q, %q", n, a)
	}
}

// З13-и6: облагаемое по частям — VAT дня по ставке как в «Продажах по дням и ставкам», части в сумме — он же до гроша;
// грош округления — самой большой части (при равных — продаже, у неё — большей ставке ryczałtu).
func TestProfileSplit(t *testing.T) {
	rows := []vatRow{
		{Kind: "time", D: "2026-10-06", Vat: 23, Ry: 150, G: 3},
		{Kind: "sales", D: "2026-10-06", Vat: 23, Ry: 30, G: 3},
		{Kind: "gifts", D: "2026-10-06", Vat: 23, G: 3},
		{Kind: "sales", D: "2026-10-06", Vat: 8, Ry: 30, G: 1080},
		{Kind: "returns", D: "2026-10-06", Vat: 8, Ry: 30, G: -540},
		{Kind: "sales", D: "2026-10-07", Vat: 23, Ry: 85, G: 1230},
		{Kind: "packs", D: "2026-10-07", Vat: 23, Ry: 150, G: 2460},
	}
	got := profileSplit(rows, true)
	want := map[profKey]profCell{
		{D: "2026-10-06", Vat: 23, Ry: 150}:    {3, 0}, // vatOf(9) = 2, частям — по 1: лишний грош снят с первой из равных
		{D: "2026-10-06", Vat: 23, Ry: 30}:     {3, 1},
		{D: "2026-10-06", Vat: 23, Gift: true}: {3, 1},
		{D: "2026-10-06", Vat: 8, Ry: 30}:      {540, 40}, // продажа и возврат одной части
		{D: "2026-10-07", Vat: 23, Ry: 85}:     {1230, 230},
		{D: "2026-10-07", Vat: 23, Ry: 150}:    {2460, 460},
	}
	if len(got) != len(want) {
		t.Fatalf("частей %d; ждали %d: %+v", len(got), len(want), got)
	}
	for k, w := range want {
		if c := got[k]; c == nil || *c != w {
			t.Errorf("%+v: %+v; ждали %+v", k, c, w)
		}
	}
	for k, c := range profileSplit(rows, false) {
		if k.Vat != models.VatZW || c.Vat != 0 {
			t.Errorf("освобождённому — всё zw без VAT: %+v %+v", k, c)
		}
	}
	days := profileDays(got)
	if len(days) != 2 || days[0].Day != "2026-10-06" || days[0].Net != 3+2+500 || days[0].Ry[150] != 3 || days[0].Ry[30] != 2+500 ||
		days[0].Gifts[23].Gross != 3 || days[0].Sales[23].Gross != 6 || days[0].Sales[23].Vat != 1 || days[1].Net != 1000+2000 {
		t.Errorf("дни: %+v", days)
	}
}

// З13-и6: итоги листов — по месяцам, если строк больше одного месяца, и за период.
func TestWithTotals(t *testing.T) {
	r := func(v int64) xrow { return xrow{cells: []xcell{cInt(1), cMoney(v), {}}} }
	out := withTotals([]xrow{r(100), r(50), r(7)}, []string{"2026-09", "2026-09", "2026-10"}, []int{1, 2}, 3, 1)
	if len(out) != 6 || out[2].cells[0].s != "Razem 09.2026" || out[2].cells[1].n != 150 || out[4].cells[0].s != "Razem 10.2026" ||
		out[5].cells[0].s != "Razem" || out[5].cells[1].n != 157 || out[5].cells[2].k != 'm' || out[5].cells[2].n != 0 || !out[5].bold {
		t.Errorf("два месяца: %+v", out)
	}
	if out := withTotals([]xrow{r(100)}, []string{"2026-10"}, []int{1}, 3, 0); len(out) != 2 || out[1].cells[0].s != "Итого" {
		t.Errorf("один месяц — только «Итого»: %+v", out)
	}
	if out := withTotals(nil, nil, []int{1}, 3, 1); len(out) != 1 || out[0].cells[1].n != 0 {
		t.Errorf("пусто — «Razem» с нулями: %+v", out)
	}
}

func profFixture() ([]profDay, []profExpense, *time.Location) {
	loc, _ := time.LoadLocation("Europe/Warsaw")
	days := profileDays(profileSplit([]vatRow{
		{Kind: "time", D: "2026-10-06", Vat: 23, Ry: 150, G: 2460},
		{Kind: "sales", D: "2026-10-06", Vat: 23, Ry: 85, G: 1230},
		{Kind: "gifts", D: "2026-10-07", Vat: 23, G: 246},
		{Kind: "sales", D: "2026-10-08", Vat: 8, Ry: 30, G: 1080},
	}, true))
	d := func(day int) time.Time { return time.Date(2026, 10, day, 0, 0, 0, 0, time.UTC) }
	exps := []profExpense{
		{Day: d(6), Doc: "FV 1", NIP: "1112223332", Party: "Hurtownia", Cat: "Закупка товаров", Descr: "Закупка товаров", Acct: "330",
			ID: "e1", Col: 12, Gross: 1230, Vat: 230, Paid: true},
		{Day: d(7), Doc: "LP 7", Cat: "Зарплата", Descr: "Зарплата: октябрь", Acct: "404", ID: "e2", Col: 14, Gross: 300000, Paid: false},
		{Day: d(7), Cat: "ZUS и налоги", Descr: "ZUS и налоги: PIT", ID: "e3", Col: 0, Gross: 50000},
		{Day: d(9), Doc: "FV 9", Cat: "Доставка", Descr: "Доставка", ID: "e4", Col: 13, Gross: 1230, Vat: 230, Paid: true},
	}
	return days, exps, loc
}

// З13-и6: «Записи для KPiR» — день: выручка в колонке 9, затем расходы дня в колонках категорий; «не в книгу» — нет; 16 =
// 14 + 15; плательщику VAT — нетто, освобождённому — брутто.
func TestSheetKpir(t *testing.T) {
	days, exps, loc := profFixture()
	sh := sheetKpir(days, exps, true, loc, 1)
	if sh.name != "Zapisy do KPiR" || sh.head[6] != "9 · Wartość sprzedanych towarów i usług" || sh.head[9] != "12 · Zakup towarów handlowych i materiałów" ||
		sh.n != 5 || len(sh.rows) != 6 {
		t.Fatalf("лист: %q, %v, %d строк", sh.name, sh.head, len(sh.rows))
	}
	type line struct {
		Lp       int64
		Day      string
		Doc      string
		Col      int
		V, Total int64
	}
	got := []line{}
	for _, r := range sh.rows[:sh.n] {
		l := line{Lp: r.cells[0].n, Day: r.cells[1].t.Format("02.01"), Doc: r.cells[2].s}
		for i := 6; i <= 12; i++ {
			if r.cells[i].k == 'm' && i != 8 {
				l.Col, l.V = i+3, r.cells[i].n
				break
			}
		}
		if r.cells[13].k == 'm' {
			l.Total = r.cells[13].n
		}
		got = append(got, l)
	}
	want := []line{{1, "06.10", "", 9, 2000 + 1000, 0}, {2, "06.10", "FV 1", 12, 1000, 0}, {3, "07.10", "LP 7", 14, 300000, 300000},
		{4, "08.10", "", 9, 1000, 0}, {5, "09.10", "FV 9", 13, 1000, 0}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("строки: %+v; ждали %+v", got, want)
	}
	if sh.rows[2].cells[14].s != "nieopłacony" || sh.rows[1].cells[3].s != "1112223332" || sh.rows[1].cells[15].s != "e1" {
		t.Errorf("примечание, NIP, ID: %+v", sh.rows[1:3])
	}
	tot := sh.rows[5].cells
	if tot[0].s != "Razem" || tot[6].n != 4000 || tot[8].n != 4000 || tot[9].n != 1000 || tot[10].n != 1000 || tot[11].n != 300000 ||
		tot[12].n != 0 || tot[13].n != 300000 {
		t.Errorf("итог: %+v", tot)
	}
	zw := sheetKpir(days, exps, false, loc, 0)
	if zw.rows[1].cells[9].n != 1230 || zw.head[0] != "№" || zw.name != "Записи для KPiR" {
		t.Errorf("освобождённому — брутто: %+v", zw.rows[1])
	}
}

// З13-и6: «Ewidencja przychodów» — ставки от большей к меньшей, ставка времени — всегда колонкой; день — строка.
func TestSheetEwid(t *testing.T) {
	days, _, loc := profFixture()
	sh := sheetEwid(days, 120, loc, 1)
	if !reflect.DeepEqual(sh.head, []string{"Lp.", "Data uzyskania przychodu", "Opis", "Przychód wg stawki 15%", "Przychód wg stawki 12%",
		"Przychód wg stawki 8,5%", "Przychód wg stawki 3%", "Ogółem przychód"}) || sh.n != 2 {
		t.Fatalf("шапка: %v, строк %d", sh.head, sh.n)
	}
	r := sh.rows[0].cells
	if r[3].n != 2000 || r[4].k != 0 || r[5].n != 1000 || r[6].k != 0 || r[7].n != 3000 || sh.rows[1].cells[6].n != 1000 ||
		sh.rows[2].cells[7].n != 4000 || sh.rows[2].cells[4].n != 0 {
		t.Errorf("строки: %+v", sh.rows)
	}
}

// З13-и6: реестр продаж — продажи и подарки дня строками, по ставкам нетто и VAT; освобождённому — «Учёт продаж» без VAT.
func TestSheetSalesReg(t *testing.T) {
	days, _, loc := profFixture()
	sh := sheetSalesReg(days, true, loc, 1)
	if !reflect.DeepEqual(sh.head, []string{"Lp.", "Data sprzedaży", "Opis", "Netto 23%", "VAT 23%", "Netto 8%", "VAT 8%", "Razem netto",
		"Razem VAT", "Brutto"}) || sh.n != 3 {
		t.Fatalf("шапка: %v, строк %d", sh.head, sh.n)
	}
	g := sh.rows[1].cells
	if !strings.HasPrefix(g[2].s, "Nieodpłatne przekazanie") || g[3].n != 200 || g[4].n != 46 || g[9].n != 246 || g[5].k != 0 {
		t.Errorf("строка подарков: %+v", g)
	}
	tot := sh.rows[3].cells
	if tot[3].n != 3000+200 || tot[4].n != 690+46 || tot[5].n != 1000 || tot[6].n != 80 || tot[7].n != 4200 || tot[8].n != 816 || tot[9].n != 5016 {
		t.Errorf("итог: %+v", tot)
	}
	zw := sheetSalesReg(profileDays(profileSplit([]vatRow{{Kind: "sales", D: "2026-10-06", Vat: 23, Ry: 30, G: 1230}}, false)), false, loc, 1)
	if zw.name != "Ewidencja sprzedaży" || len(zw.head) != 4 || zw.rows[0].cells[3].n != 1230 {
		t.Errorf("освобождённому: %q %v %+v", zw.name, zw.head, zw.rows)
	}
	for _, h := range zw.head {
		if strings.Contains(h, "VAT") {
			t.Errorf("у освобождённого колонка VAT: %q", h)
		}
	}
}

// З13-и6: реестр покупок — расходы с VAT по документу со счётом; освобождённому — все расходы, брутто, без VAT.
func TestSheetBuyReg(t *testing.T) {
	_, exps, _ := profFixture()
	sh := sheetBuyReg(exps, true, 1)
	if sh.name != "Rejestr zakupów VAT" || sh.n != 2 || sh.rows[0].cells[6].s != "330" || sh.rows[0].cells[7].n != 1000 ||
		sh.rows[0].cells[8].n != 230 || sh.rows[0].cells[9].n != 1230 || sh.rows[2].cells[9].n != 2460 {
		t.Errorf("реестр покупок: %+v", sh.rows)
	}
	zw := sheetBuyReg(exps, false, 1)
	if zw.name != "Zakupy i wydatki" || zw.n != 4 || zw.rows[1].cells[7].n != 300000 || zw.rows[1].cells[8].s != "nieopłacony" ||
		zw.rows[4].cells[7].n != 1230+300000+50000+1230 {
		t.Errorf("освобождённому: %+v", zw.rows)
	}
	for _, h := range zw.head {
		if strings.Contains(h, "VAT") {
			t.Errorf("у освобождённого колонка VAT: %q", h)
		}
	}
}

// З13-и6: листы профиля по форме учёта: KPiR — «Записи для KPiR», ryczałt — ewidencja, полная — реестры и «Касса».
func TestProfileSheetsAndAbout(t *testing.T) {
	days, exps, loc := profFixture()
	cash := xsheet{name: "Raport kasowy"}
	names := func(form int, payer bool) []string {
		sh, _ := profileSheets(profileData{Form: form, Cfg: vatConfig{Payer: payer, RyTime: 150}, Days: days, Exps: exps}, cash, loc, 1)
		out := []string{}
		for _, s := range sh {
			out = append(out, s.name)
		}
		return out
	}
	if got := names(formKpir, true); !reflect.DeepEqual(got, []string{"Zapisy do KPiR"}) {
		t.Errorf("KPiR: %v", got)
	}
	if got := names(formRyczalt, true); !reflect.DeepEqual(got, []string{"Ewidencja przychodów"}) {
		t.Errorf("ryczałt: %v", got)
	}
	if got := names(formFull, true); !reflect.DeepEqual(got, []string{"Rejestr sprzedaży VAT", "Rejestr zakupów VAT", "Raport kasowy"}) {
		t.Errorf("полная: %v", got)
	}
	if got := names(formFull, false); !reflect.DeepEqual(got, []string{"Ewidencja sprzedaży", "Zakupy i wydatki", "Raport kasowy"}) {
		t.Errorf("полная, освобождён: %v", got)
	}
	about := func(pd profileData) map[string]string {
		out := map[string]string{}
		for _, r := range profileAbout(pd, 1) {
			out[r.cells[0].s] = r.cells[1].s
		}
		return out
	}
	k := about(profileData{Form: formKpir, Cfg: vatConfig{Payer: true}, Exps: exps})
	if k["Profil ewidencji"] != "KPiR — podatkowa księga przychodów i rozchodów" || k["Poza KPiR"] != "«ZUS и налоги» — 1 szt., 500,00 zł" ||
		!strings.Contains(k["Przychód"], "przy wykorzystaniu") {
		t.Errorf("KPiR в «O eksporcie»: %v", k)
	}
	r := about(profileData{Form: formRyczalt, Cfg: vatConfig{Payer: true, Topup: true, RyTime: 85}, GoodsRy: map[int]int{30: 12, 85: 1}})
	if !strings.HasPrefix(r["Stawki ryczałtu"], "czas gry i pakiety — 8,5% (ustawienie); towary i kuchnia — wg stawki pozycji: 8,5% — 1 poz., 3% — 12 poz.") ||
		!strings.Contains(r["Przychód"], "przy doładowaniu") {
		t.Errorf("ryczałt в «O eksporcie»: %v", r)
	}
	f := about(profileData{Form: formFull, Cfg: vatConfig{}, Accounts: [2]int{9, 14}})
	if !strings.HasPrefix(f["Rejestry"], "klub zwolniony z VAT") || f["Konta kategorii"] != "ustawione w 9 z 14 kategorii; w pozostałych — puste, konto wybierze księgowy" ||
		!strings.Contains(f["Przychód"], "brutto: klub zwolniony z VAT") {
		t.Errorf("полная в «O eksporcie»: %v", f)
	}
	if c, ok := expenseFormCol(formKpir, models.ExpenseCategory{KpirCol: 0}, 1); !ok || c.s != "poza KPiR" {
		t.Errorf("колонка «Wydatki» у KPiR: %+v", c)
	}
	if c, ok := expenseFormCol(formFull, models.ExpenseCategory{Account: "402"}, 0); !ok || c.s != "402" {
		t.Errorf("колонка «Расходов» у полной: %+v", c)
	}
	if _, ok := expenseFormCol(formRyczalt, models.ExpenseCategory{}, 0); ok {
		t.Errorf("у ryczałtu колонки в «Расходах» нет")
	}
}
