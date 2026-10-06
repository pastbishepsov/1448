package main

// З13-и4 «Выгрузка: движок» (OWNER.md, трек З; №75; Д8 — XLSX пишем сами на стандартной библиотеке, без новых
// зависимостей). Книга — листы со шапкой и строками ячеек; из неё — XLSX (все листы в одном файле: шапка закреплена,
// у таблиц — фильтр, деньги — числами с двумя знаками, даты — датами Excel) или архив CSV (файл на лист; разделитель,
// десятичный знак, кодировка — UTF-8 с BOM или Windows-1250 для старых польских программ — и формат даты —
// настройками). Что в листах — export.go.

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// xcell — ячейка: текст, деньги (гроши), целое, дата, дата и время; нулевая — пустая.
type xcell struct {
	k byte // 's' текст, 'm' деньги, 'i' целое, 'd' дата, 't' дата и время; 0 — пусто
	s string
	n int64
	t time.Time
}

func cText(s string) xcell     { return xcell{k: 's', s: s} }
func cMoney(g int64) xcell     { return xcell{k: 'm', n: g} }
func cInt(n int64) xcell       { return xcell{k: 'i', n: n} }
func cDay(t time.Time) xcell   { return xcell{k: 'd', t: t} }
func cStamp(t time.Time) xcell { return xcell{k: 't', t: t} }

// xrow — строка листа; bold — итог.
type xrow struct {
	cells []xcell
	bold  bool
}

// xsheet — лист: имя в книге, имя файла в архиве CSV (латиницей), шапка, ширины колонок (символы), строки.
// table — таблица: шапка закреплена, у неё фильтр.
type xsheet struct {
	name, file string
	head       []string
	widths     []int
	rows       []xrow
	table      bool
}

// bookFormat — как писать файл: CSV — разделитель, десятичный знак, Windows-1250; даты — ГГГГ-ММ-ДД (iso) или ДД.ММ.ГГГГ.
type bookFormat struct {
	sep     rune
	decimal byte
	w1250   bool
	iso     bool
	now     time.Time
}

// ── XLSX ─────────────────────────────────────────────────────────────────────────────────────────

// стили ячеек (cellXfs в styles.xml)
const (
	xsPlain = iota
	xsHead
	xsMoney
	xsMoneyBold
	xsInt
	xsDay
	xsStamp
	xsBold
)

const xmlHead = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n"

// xcol — буква колонки по номеру с нуля (чистая, тест): 0 — A, 25 — Z, 26 — AA.
func xcol(i int) string {
	s := ""
	for i++; i > 0; i = (i - 1) / 26 {
		s = string(rune('A'+(i-1)%26)) + s
	}
	return s
}

var excelEpoch = time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)

// excelSerial — дата Excel (чистая, тест): дни от 30.12.1899 по стенным часам момента; withTime — с долей суток.
func excelSerial(t time.Time, withTime bool) string {
	wall := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	days := int64(wall.Sub(excelEpoch).Hours() / 24)
	if !withTime {
		return strconv.FormatInt(days, 10)
	}
	sec := t.Hour()*3600 + t.Minute()*60 + t.Second()
	return strconv.FormatFloat(float64(days)+float64(sec)/86400, 'f', 6, 64)
}

// moneyText — гроши числом: «-1234.50» в XLSX, в CSV — с десятичным знаком настройки (чистая, тест).
func moneyText(g int64, decimal byte) string {
	sign := ""
	if g < 0 {
		sign, g = "-", -g
	}
	return fmt.Sprintf("%s%d%c%02d", sign, g/100, decimal, g%100)
}

// xmlText — текст для XML: без запрещённых XML 1.0 символов, с экранированием (чистая, тест).
func xmlText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToValidUTF8(s, "?") {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case r == '<':
			b.WriteString("&lt;")
		case r == '>':
			b.WriteString("&gt;")
		case r == '"':
			b.WriteString("&quot;")
		case r == '\t' || r == '\n' || r == '\r':
			b.WriteRune(r)
		case r < 0x20 || r == 0xFFFE || r == 0xFFFF:
			// управляющие символы в XML нельзя — пропускаем
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func xlsxCell(b io.Writer, ref string, c xcell, bold bool) {
	switch c.k {
	case 's':
		if c.s == "" { // пустой текст — пустая ячейка: в фильтре Excel она «(Пустые)», а не строка без букв
			return
		}
		st := xsPlain
		if bold {
			st = xsBold
		}
		fmt.Fprintf(b, `<c r="%s" t="inlineStr" s="%d"><is><t xml:space="preserve">%s</t></is></c>`, ref, st, xmlText(c.s))
	case 'm':
		st := xsMoney
		if bold {
			st = xsMoneyBold
		}
		fmt.Fprintf(b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, st, moneyText(c.n, '.'))
	case 'i':
		fmt.Fprintf(b, `<c r="%s" s="%d"><v>%d</v></c>`, ref, xsInt, c.n)
	case 'd':
		fmt.Fprintf(b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, xsDay, excelSerial(c.t, false))
	case 't':
		fmt.Fprintf(b, `<c r="%s" s="%d"><v>%s</v></c>`, ref, xsStamp, excelSerial(c.t, true))
	}
}

// xlsxSheetWrite — лист потоком в w: строки не копятся в памяти целиком — выгрузка за год не раздувает сервер.
func xlsxSheetWrite(w io.Writer, sh xsheet) error {
	b := bufio.NewWriterSize(w, 64<<10)
	b.WriteString(xmlHead + `<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
		`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">`)
	ncol := len(sh.head)
	for _, r := range sh.rows {
		ncol = max(ncol, len(r.cells))
	}
	if ncol > 0 { // размер листа — потоковым читалкам (openpyxl, pandas) не надо считать его самим
		fmt.Fprintf(b, `<dimension ref="A1:%s%d"/>`, xcol(ncol-1), len(sh.rows)+1)
	}
	if sh.table {
		b.WriteString(`<sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" activePane="bottomLeft" state="frozen"/>` +
			`<selection pane="bottomLeft" activeCell="A2" sqref="A2"/></sheetView></sheetViews>`)
	}
	if len(sh.widths) > 0 {
		b.WriteString("<cols>")
		for i, w := range sh.widths {
			fmt.Fprintf(b, `<col min="%d" max="%d" width="%d" customWidth="1"/>`, i+1, i+1, w)
		}
		b.WriteString("</cols>")
	}
	b.WriteString("<sheetData>")
	b.WriteString(`<row r="1">`)
	for i, h := range sh.head {
		fmt.Fprintf(b, `<c r="%s1" t="inlineStr" s="%d"><is><t xml:space="preserve">%s</t></is></c>`, xcol(i), xsHead, xmlText(h))
	}
	b.WriteString("</row>")
	for ri, row := range sh.rows {
		n := ri + 2
		fmt.Fprintf(b, `<row r="%d">`, n)
		for ci, c := range row.cells {
			if c.k != 0 {
				xlsxCell(b, xcol(ci)+strconv.Itoa(n), c, row.bold)
			}
		}
		b.WriteString("</row>")
	}
	b.WriteString("</sheetData>")
	if sh.table && len(sh.head) > 0 {
		fmt.Fprintf(b, `<autoFilter ref="A1:%s%d"/>`, xcol(len(sh.head)-1), len(sh.rows)+1)
	}
	b.WriteString("</worksheet>")
	return b.Flush()
}

func xlsxStyles(iso bool) string {
	day, stamp := "dd.mm.yyyy", "dd.mm.yyyy hh:mm"
	if iso {
		day, stamp = "yyyy-mm-dd", "yyyy-mm-dd hh:mm"
	}
	return xmlHead + `<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<numFmts count="2"><numFmt numFmtId="164" formatCode="` + day + `"/><numFmt numFmtId="165" formatCode="` + stamp + `"/></numFmts>` +
		`<fonts count="2"><font><sz val="11"/><name val="Calibri"/><family val="2"/></font>` +
		`<font><b/><sz val="11"/><name val="Calibri"/><family val="2"/></font></fonts>` +
		`<fills count="2"><fill><patternFill patternType="none"/></fill><fill><patternFill patternType="gray125"/></fill></fills>` +
		`<borders count="1"><border><left/><right/><top/><bottom/><diagonal/></border></borders>` +
		`<cellStyleXfs count="1"><xf numFmtId="0" fontId="0" fillId="0" borderId="0"/></cellStyleXfs>` +
		`<cellXfs count="8">` +
		`<xf numFmtId="0" fontId="0" fillId="0" borderId="0" xfId="0"/>` + // xsPlain
		`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` + // xsHead
		`<xf numFmtId="4" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` + // xsMoney: #,##0.00
		`<xf numFmtId="4" fontId="1" fillId="0" borderId="0" xfId="0" applyNumberFormat="1" applyFont="1"/>` + // xsMoneyBold
		`<xf numFmtId="1" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` + // xsInt
		`<xf numFmtId="164" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` + // xsDay
		`<xf numFmtId="165" fontId="0" fillId="0" borderId="0" xfId="0" applyNumberFormat="1"/>` + // xsStamp
		`<xf numFmtId="0" fontId="1" fillId="0" borderId="0" xfId="0" applyFont="1"/>` + // xsBold
		`</cellXfs><cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles></styleSheet>`
}

// writeXLSX — книга одним файлом XLSX (Office Open XML): все листы, шапка и стили.
func writeXLSX(w io.Writer, sheets []xsheet, f bookFormat) error {
	zw := zip.NewWriter(w)
	put := func(name, body string) error {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: f.now})
		if err != nil {
			return err
		}
		_, err = io.WriteString(fw, body)
		return err
	}
	var types, rels, list strings.Builder
	types.WriteString(xmlHead + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
		`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
		`<Default Extension="xml" ContentType="application/xml"/>` +
		`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
		`<Override PartName="/xl/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.styles+xml"/>`)
	rels.WriteString(xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	for i, sh := range sheets {
		n := i + 1
		fmt.Fprintf(&types, `<Override PartName="/xl/worksheets/sheet%d.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>`, n)
		fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet%d.xml"/>`, n, n)
		fmt.Fprintf(&list, `<sheet name="%s" sheetId="%d" r:id="rId%d"/>`, xmlText(xlsxSheetName(sh.name)), n, n)
	}
	types.WriteString(`</Types>`)
	fmt.Fprintf(&rels, `<Relationship Id="rId%d" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/></Relationships>`, len(sheets)+1)
	parts := [][2]string{
		{"[Content_Types].xml", types.String()},
		{"_rels/.rels", xmlHead + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`},
		{"xl/workbook.xml", xmlHead + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" ` +
			`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets>` + list.String() + `</sheets></workbook>`},
		{"xl/_rels/workbook.xml.rels", rels.String()},
		{"xl/styles.xml", xlsxStyles(f.iso)},
	}
	for _, p := range parts {
		if err := put(p[0], p[1]); err != nil {
			return err
		}
	}
	for i, sh := range sheets {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("xl/worksheets/sheet%d.xml", i+1), Method: zip.Deflate, Modified: f.now})
		if err != nil {
			return err
		}
		if err := xlsxSheetWrite(fw, sh); err != nil {
			return err
		}
	}
	return zw.Close()
}

// xlsxSheetName — имя листа по правилам Excel: до 31 символа, без []:*?/\.
func xlsxSheetName(s string) string {
	s = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`[]:*?/\`, r) {
			return ' '
		}
		return r
	}, s)
	if utf8.RuneCountInString(s) > 31 {
		s = string([]rune(s)[:31])
	}
	return s
}

// ── CSV ──────────────────────────────────────────────────────────────────────────────────────────

// csvValue — ячейка текстом для CSV: деньги — с десятичным знаком настройки, даты — в формате настройки.
func csvValue(c xcell, f bookFormat) string {
	day := "02.01.2006"
	if f.iso {
		day = "2006-01-02"
	}
	switch c.k {
	case 's':
		return c.s
	case 'm':
		return moneyText(c.n, f.decimal)
	case 'i':
		return strconv.FormatInt(c.n, 10)
	case 'd':
		return c.t.Format(day)
	case 't':
		return c.t.Format(day + " 15:04")
	}
	return ""
}

// csvWrite — лист в CSV потоком, строка за строкой: строки через CRLF, разделитель настройки; UTF-8 с BOM или
// Windows-1250 (строка перекодируется целиком — буквы не рвутся между записями).
func csvWrite(w io.Writer, sh xsheet, f bookFormat) error {
	out := bufio.NewWriterSize(w, 64<<10)
	if !f.w1250 {
		out.Write([]byte{0xEF, 0xBB, 0xBF})
	}
	var line bytes.Buffer
	cw := csv.NewWriter(&line)
	cw.Comma, cw.UseCRLF = f.sep, true
	put := func(vals []string) error {
		line.Reset()
		if err := cw.Write(vals); err != nil {
			return err
		}
		cw.Flush()
		if f.w1250 {
			_, err := out.Write(toW1250(line.String()))
			return err
		}
		_, err := out.Write(line.Bytes())
		return err
	}
	if err := put(sh.head); err != nil {
		return err
	}
	vals := []string{}
	for _, r := range sh.rows {
		vals = vals[:0]
		for _, c := range r.cells {
			vals = append(vals, csvValue(c, f))
		}
		if err := put(vals); err != nil {
			return err
		}
	}
	return out.Flush()
}

// csvBytes — лист в CSV целиком (чистая, тест).
func csvBytes(sh xsheet, f bookFormat) []byte {
	var buf bytes.Buffer
	_ = csvWrite(&buf, sh, f)
	return buf.Bytes()
}

// writeCSVZip — книга архивом: файл CSV на лист, по порядку листов.
func writeCSVZip(w io.Writer, sheets []xsheet, f bookFormat) error {
	zw := zip.NewWriter(w)
	for i, sh := range sheets {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: fmt.Sprintf("%02d_%s.csv", i+1, sh.file), Method: zip.Deflate, Modified: f.now})
		if err != nil {
			return err
		}
		if err := csvWrite(fw, sh, f); err != nil {
			return err
		}
	}
	return zw.Close()
}

// ── Windows-1250 ─────────────────────────────────────────────────────────────────────────────────

// w1250High — руны байтов 0x80…0xFF кодировки Windows-1250 (0 — байт не определён).
var w1250High = [128]rune{
	0x20AC, 0x0000, 0x201A, 0x0000, 0x201E, 0x2026, 0x2020, 0x2021,
	0x0000, 0x2030, 0x0160, 0x2039, 0x015A, 0x0164, 0x017D, 0x0179,
	0x0000, 0x2018, 0x2019, 0x201C, 0x201D, 0x2022, 0x2013, 0x2014,
	0x0000, 0x2122, 0x0161, 0x203A, 0x015B, 0x0165, 0x017E, 0x017A,
	0x00A0, 0x02C7, 0x02D8, 0x0141, 0x00A4, 0x0104, 0x00A6, 0x00A7,
	0x00A8, 0x00A9, 0x015E, 0x00AB, 0x00AC, 0x00AD, 0x00AE, 0x017B,
	0x00B0, 0x00B1, 0x02DB, 0x0142, 0x00B4, 0x00B5, 0x00B6, 0x00B7,
	0x00B8, 0x0105, 0x015F, 0x00BB, 0x013D, 0x02DD, 0x013E, 0x017C,
	0x0154, 0x00C1, 0x00C2, 0x0102, 0x00C4, 0x0139, 0x0106, 0x00C7,
	0x010C, 0x00C9, 0x0118, 0x00CB, 0x011A, 0x00CD, 0x00CE, 0x010E,
	0x0110, 0x0143, 0x0147, 0x00D3, 0x00D4, 0x0150, 0x00D6, 0x00D7,
	0x0158, 0x016E, 0x00DA, 0x0170, 0x00DC, 0x00DD, 0x0162, 0x00DF,
	0x0155, 0x00E1, 0x00E2, 0x0103, 0x00E4, 0x013A, 0x0107, 0x00E7,
	0x010D, 0x00E9, 0x0119, 0x00EB, 0x011B, 0x00ED, 0x00EE, 0x010F,
	0x0111, 0x0144, 0x0148, 0x00F3, 0x00F4, 0x0151, 0x00F6, 0x00F7,
	0x0159, 0x016F, 0x00FA, 0x0171, 0x00FC, 0x00FD, 0x0163, 0x02D9,
}

var w1250Byte = func() map[rune]byte {
	m := make(map[rune]byte, 128)
	for i, r := range w1250High {
		if r != 0 {
			m[r] = byte(0x80 + i)
		}
	}
	return m
}()

// cyrLatin — кириллица латиницей для Windows-1250, где кириллицы нет: по-польски (ш — sz, ч — cz, ж — ż).
var cyrLatin = map[rune]string{
	'а': "a", 'б': "b", 'в': "w", 'г': "g", 'д': "d", 'е': "e", 'ё': "jo", 'ж': "ż", 'з': "z", 'и': "i", 'й': "j",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f",
	'х': "ch", 'ц': "c", 'ч': "cz", 'ш': "sz", 'щ': "szcz", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "ju", 'я': "ja",
	'і': "i", 'ї': "ji", 'є': "je", 'ґ': "g", 'ў': "u", '№': "Nr ",
}

// toW1250 — текст в Windows-1250 (чистая, тест): польские буквы — своими байтами, кириллица — латиницей,
// остальное, чего в кодировке нет, — «?».
func toW1250(s string) []byte {
	out := make([]byte, 0, len(s))
	var put func(r rune)
	put = func(r rune) {
		switch {
		case r < 0x80:
			out = append(out, byte(r))
		case w1250Byte[r] != 0:
			out = append(out, w1250Byte[r])
		case r == '−': // минус — дефисом
			out = append(out, '-')
		default:
			lower := []rune(strings.ToLower(string(r)))[0]
			lat, ok := cyrLatin[lower]
			if !ok {
				out = append(out, '?')
				return
			}
			if lower != r && lat != "" { // заглавная — первая буква латиницей заглавной
				rs := []rune(lat)
				lat = strings.ToUpper(string(rs[0])) + string(rs[1:])
			}
			for _, x := range lat {
				put(x)
			}
		}
	}
	for _, r := range s {
		put(r)
	}
	return out
}
