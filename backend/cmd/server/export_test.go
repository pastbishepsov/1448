package main

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// З13-и4: колонки Excel, даты Excel, деньги текстом, текст для XML.
func TestWorkbookBasics(t *testing.T) {
	for i, want := range map[int]string{0: "A", 25: "Z", 26: "AA", 51: "AZ", 52: "BA", 701: "ZZ", 702: "AAA"} {
		if got := xcol(i); got != want {
			t.Errorf("xcol(%d) = %q; ждали %q", i, got, want)
		}
	}
	loc, _ := time.LoadLocation("Europe/Warsaw")
	if got := excelSerial(time.Date(2026, 10, 6, 21, 30, 0, 0, loc), false); got != "46301" {
		t.Errorf("дата Excel 06.10.2026 = %s; ждали 46301", got)
	}
	if got := excelSerial(time.Date(2026, 1, 1, 18, 0, 0, 0, loc), true); got != "46023.750000" {
		t.Errorf("дата и время Excel 01.01.2026 18:00 = %s; ждали 46023.750000 (по стенным часам)", got)
	}
	for _, tc := range []struct {
		g    int64
		d    byte
		want string
	}{{123456, '.', "1234.56"}, {-5, ',', "-0,05"}, {0, ',', "0,00"}, {-123400, '.', "-1234.00"}} {
		if got := moneyText(tc.g, tc.d); got != tc.want {
			t.Errorf("moneyText(%d, %c) = %q; ждали %q", tc.g, tc.d, got, tc.want)
		}
	}
	if got := xmlText("a & b < c > \"d\"\x01\x0b\tе"); got != "a &amp; b &lt; c &gt; &quot;d&quot;\tе" {
		t.Errorf("xmlText = %q", got)
	}
	if got := xlsxSheetName("Продажи: по [дням] / ставкам и ещё очень длинное имя"); len([]rune(got)) != 31 || strings.ContainsAny(got, "[]:/") {
		t.Errorf("имя листа: %q", got)
	}
}

// З13-и4: Windows-1250 — польские буквы своими байтами, кириллица латиницей по-польски, чего нет — «?».
func TestToW1250(t *testing.T) {
	if got := toW1250("Zażółć gęślą jaźń"); string(got) != "Za\xbf\xf3\xb3\xe6 g\xea\x9cl\xb9 ja\x9f\xf1" {
		t.Errorf("польские буквы: % x", got)
	}
	for in, want := range map[string]string{"Кола": "Kola", "Щука и Жук": "Szczuka i \xafuk", "чай": "czaj", "№ 5": "Nr  5", "→": "?", "−2,50": "-2,50"} {
		if got := string(toW1250(in)); got != want {
			t.Errorf("toW1250(%q) = %q; ждали %q", in, got, want)
		}
	}
}

// З13-и4: облагается ли строка журнала — те же правила, что «Продажи по ставкам VAT» (шаги e2e_z13_vat_sales).
func TestExportTaxed(t *testing.T) {
	spend, topup := vatConfig{Payer: true, TopupVat: 23}, vatConfig{Payer: true, Topup: true, TopupVat: 23}
	cases := []struct {
		kind, method, refund string
		sum                  int64
		cfg                  vatConfig
		taxed, topupRate     bool
	}{
		{"deposit", "card", "", 10000, spend, false, false},
		{"deposit", "card", "", 10000, topup, true, false},
		{"deposit_void", "card", "card", -4000, topup, true, false},
		{"deposit_void", "card", "none", 0, topup, false, false},
		{"deposit_void", "card", "card", -4000, spend, false, false},
		{"time", "wallet", "", 1000, spend, true, false},
		{"time", "wallet", "", 1000, topup, false, false},
		{"sale", "wallet", "", 1000, spend, true, false},
		{"kitchen", "wallet", "", 1000, topup, false, false},
		{"kitchen", "card", "", 1000, topup, true, false},
		{"pack", "wallet", "", 5000, spend, true, false},
		{"pack", "wallet", "", 5000, topup, false, false},
		{"sale_void", "card", "card", -1000, topup, true, false},
		{"sale_void", "wallet", "wallet", -1000, spend, true, false},
		{"sale_void", "wallet", "wallet", -1000, topup, false, true},
		{"pack_void", "wallet", "cash", -5000, topup, true, true},
		{"pack_void", "wallet", "cash", -5000, spend, true, false},
		{"pack_void", "card", "none", 0, spend, false, false},
		{"gift", "comp", "", 250, spend, false, false},
		{"gift", "comp", "", 250, vatConfig{Payer: true, Gifts: true}, true, false},
		{"gift", "comp", "", 250, vatConfig{Gifts: true}, false, false},
		{"gift", "comp", "", 0, vatConfig{Payer: true, Gifts: true}, false, false},
		{"expense", "till", "", -5000, spend, false, false},
	}
	for _, tc := range cases {
		taxed, tr := exportTaxed(tc.kind, tc.method, tc.refund, tc.sum, tc.cfg)
		if taxed != tc.taxed || tr != tc.topupRate {
			t.Errorf("exportTaxed(%s, %s, %s, %d, %+v) = %v, %v; ждали %v, %v", tc.kind, tc.method, tc.refund, tc.sum, tc.cfg, taxed, tr, tc.taxed, tc.topupRate)
		}
	}
}

// З13-и4: книга XLSX — архив с частями Office Open XML, каждая часть — правильный XML; листы и ячейки на месте.
func TestWriteXLSX(t *testing.T) {
	now := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	sheets := []xsheet{
		{name: "О выгрузке", file: "o", head: []string{"Что", "Значение"}, rows: []xrow{{cells: []xcell{cText("Клуб"), cText("14:48 & <Ко>")}},
			{cells: []xcell{cText("Адрес"), cText("")}}}},
		{name: "Все операции", file: "ops", table: true, head: []string{"День", "Сумма", "Кол-во", "Когда"}, widths: []int{12, 12, 7, 17},
			rows: []xrow{{cells: []xcell{cDay(now), cMoney(-1250), cInt(3), cStamp(now)}}, {cells: []xcell{cText("Итого"), cMoney(1250), {}, {}}, bold: true}}},
	}
	var buf bytes.Buffer
	if err := writeXLSX(&buf, sheets, bookFormat{now: now}); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range zr.File {
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		rc.Close()
		files[f.Name] = string(b)
		d := xml.NewDecoder(bytes.NewReader(b))
		for {
			if _, err := d.Token(); err == io.EOF {
				break
			} else if err != nil {
				t.Errorf("%s — не XML: %v", f.Name, err)
				break
			}
		}
	}
	for _, name := range []string{"[Content_Types].xml", "_rels/.rels", "xl/workbook.xml", "xl/_rels/workbook.xml.rels", "xl/styles.xml",
		"xl/worksheets/sheet1.xml", "xl/worksheets/sheet2.xml"} {
		if _, ok := files[name]; !ok {
			t.Errorf("нет части %s", name)
		}
	}
	if w := files["xl/workbook.xml"]; !strings.Contains(w, `name="О выгрузке"`) || !strings.Contains(w, `name="Все операции"`) {
		t.Errorf("листы: %s", w)
	}
	s2 := files["xl/worksheets/sheet2.xml"]
	for _, want := range []string{`<dimension ref="A1:D3"/>`, `<c r="A2" s="5"><v>46301</v></c>`, `<c r="B2" s="2"><v>-12.50</v></c>`, `<c r="C2" s="4"><v>3</v></c>`,
		`<c r="D2" s="6"><v>46301.500000</v></c>`, `<c r="B3" s="3"><v>12.50</v></c>`, `<autoFilter ref="A1:D3"/>`, `state="frozen"`} {
		if !strings.Contains(s2, want) {
			t.Errorf("в листе нет %s", want)
		}
	}
	if !strings.Contains(files["xl/worksheets/sheet1.xml"], "14:48 &amp; &lt;Ко&gt;") {
		t.Errorf("текст не экранирован: %s", files["xl/worksheets/sheet1.xml"])
	}
	if s1 := files["xl/worksheets/sheet1.xml"]; !strings.Contains(s1, `r="A3"`) || strings.Contains(s1, `r="B3"`) {
		t.Errorf("пустой текст — пустая ячейка, без строки в файле: %s", s1)
	}
}

// З13-и4: CSV — разделитель, десятичный знак, формат даты; UTF-8 с BOM или Windows-1250; строки через CRLF.
func TestCSVBytes(t *testing.T) {
	day := time.Date(2026, 10, 6, 21, 5, 0, 0, time.UTC)
	sh := xsheet{head: []string{"Dzień", "Opis", "Kwota"}, rows: []xrow{{cells: []xcell{cDay(day), cText("Кола; «лёд»"), cMoney(-250)}}}}
	got := string(csvBytes(sh, bookFormat{sep: ';', decimal: ','}))
	if got != "\xef\xbb\xbfDzień;Opis;Kwota\r\n06.10.2026;\"Кола; «лёд»\";-2,50\r\n" {
		t.Errorf("UTF-8, «;», «,»: %q", got)
	}
	got = string(csvBytes(sh, bookFormat{sep: ',', decimal: '.', w1250: true, iso: true}))
	if got != "Dzie\xf1,Opis,Kwota\r\n2026-10-06,Kola; \xabljod\xbb,-2.50\r\n" {
		t.Errorf("Windows-1250, «,», «.», ISO: %q", got)
	}
	if exportFormatText("xlsx", bookFormat{}, 1) != "XLSX" || !strings.Contains(exportFormatText("csv", bookFormat{sep: ';', decimal: ','}, 0), "UTF-8 BOM") {
		t.Errorf("формат в «О выгрузке»")
	}
}

// З13-и4: имя файла латиницей — месяц или свои даты.
func TestExportFileName(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Warsaw")
	sep := makePeriod(time.Date(2026, 9, 1, 0, 0, 0, 0, loc), time.Date(2026, 9, 30, 0, 0, 0, 0, loc), 8, "cal_month")
	oct := makePeriod(time.Date(2026, 10, 1, 0, 0, 0, 0, loc), time.Date(2026, 10, 6, 0, 0, 0, 0, loc), 8, "month")
	own := makePeriod(time.Date(2026, 9, 5, 0, 0, 0, 0, loc), time.Date(2026, 9, 20, 0, 0, 0, 0, loc), 8, "")
	for _, tc := range []struct {
		p            period
		format, want string
		lang         int
	}{
		{sep, "xlsx", "1448-eksport-2026-09.xlsx", 1},
		{oct, "csv", "1448-vygruzka-2026-10.zip", 0},
		{own, "xlsx", "1448-eksport-2026-09-05_2026-09-20.xlsx", 1},
	} {
		if got := exportFileName(tc.p, tc.format, tc.lang); got != tc.want {
			t.Errorf("exportFileName = %q; ждали %q", got, tc.want)
		}
	}
	if models.VatLabel(23) != "23 %" || rateLabel(1, 23) != "23%" || rateLabel(0, models.VatZW) != "zw" {
		t.Errorf("ставки в файле: «23 %%» по-русски, «23%%» по-польски, «zw»")
	}
}
