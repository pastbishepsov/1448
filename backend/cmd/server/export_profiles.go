package main

// З13-и6 «Профили учёта» (OWNER.md, трек З; №76; решения основателя 04.10, З-Р37 — бухгалтерия под любого
// бухгалтера, только Польша). Профиль формы учёта (настройка acc_form) решает, какие листы идут сразу после «О
// выгрузке» и какие в них колонки (устройство выгрузки — решено 04.10):
//
//	KPiR (по умолчанию) — «Записи для KPiR»: основа книги по образцу 2026 года (rozporządzenie MF z 6.09.2025, 19
//	  колонок): выручка дня — в колонке 9, расход — в колонке своей категории (12 — закупка товаров и материалов,
//	  13 — побочные затраты закупки, 14 — зарплаты, 15 — прочие; 16 = 14 + 15), категории «не в книгу» — нет;
//	ryczałt — «Доходы по ставкам ryczałtu» (ewidencja przychodów): выручка дня по ставкам — время и пакеты по ставке
//	  времени (настройка acc_ryczalt_time), товары и кухня — по ставке позиции;
//	полная бухгалтерия — «Реестр продаж VAT» (дни по ставкам, подарки — своей строкой), «Реестр покупок VAT» (расходы
//	  с VAT по документу, со счётом категории) и «Касса» — raport kasowy с KP/KW переезжает сюда же;
//	освобождённому от VAT — без колонок VAT: «Учёт продаж» (брутто по дням) и «Покупки и расходы» (все расходы,
//	  брутто, со счётом категории); KPiR и ewidencja у него — по брутто.
//
// Выручка дня — по тому же моменту, что VAT (настройка «VAT у пополнения»): одна цифра с «Продажами по ставкам VAT» и
// фискальной кассой. Нетто плательщику VAT: VAT дня по ставке — из брутто дня (vatOf), как в «Продажах по дням и
// ставкам»; внутри ставки VAT делится по частям — продажа или подарок, ставка ryczałtu — каждой vatOf своего брутто, а
// грош округления — самой большой части: части в сумме дают VAT дня до гроша. Подарки — не выручка: в KPiR и в
// ewidencji их нет, в реестре продаж — своей строкой. Расходы — по дню операции, без сторно; в книгу — нетто (брутто
// минус VAT документа), освобождённому — брутто. В «Расходах» — колонка формы учёта: у KPiR — колонка категории, у
// полной — счёт. Итоги листов — по месяцам, если период длиннее месяца, и за период.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// формы учёта — значения настройки acc_form
const (
	formKpir    = 0
	formRyczalt = 1
	formFull    = 2
)

// ── слова: [русский, польский] ─────────────────────────────────────────────────────────────────

var profileWords = map[string][2]string{
	"pf_sheet_kpir":    {"Записи для KPiR", "Zapisy do KPiR"},
	"pf_sheet_ewid":    {"Доходы по ставкам ryczałtu", "Ewidencja przychodów"},
	"pf_sheet_sreg":    {"Реестр продаж VAT", "Rejestr sprzedaży VAT"},
	"pf_sheet_preg":    {"Реестр покупок VAT", "Rejestr zakupów VAT"},
	"pf_sheet_sreg_zw": {"Учёт продаж", "Ewidencja sprzedaży"},
	"pf_sheet_preg_zw": {"Покупки и расходы", "Zakupy i wydatki"},
	// «Записи для KPiR»: номера колонок — по образцу книги с 2026 года
	"pf_k2": {"2 · Дата события", "2 · Data zdarzenia gospodarczego"}, "pf_k4": {"4 · Номер документа", "4 · Numer dowodu księgowego"},
	"pf_k5": {"5 · NIP контрагента", "5 · NIP kontrahenta"}, "pf_k6": {"6 · Контрагент", "6 · Kontrahent"},
	"pf_k8":  {"8 · Описание события", "8 · Opis zdarzenia gospodarczego"},
	"pf_k9":  {"9 · Продажа товаров и услуг", "9 · Wartość sprzedanych towarów i usług"},
	"pf_k10": {"10 · Прочие доходы", "10 · Pozostałe przychody"}, "pf_k11": {"11 · Доход всего", "11 · Razem przychód"},
	"pf_k12": {"12 · Закупка товаров и материалов", "12 · Zakup towarów handlowych i materiałów"},
	"pf_k13": {"13 · Побочные затраты закупки", "13 · Koszty uboczne zakupu"},
	"pf_k14": {"14 · Зарплаты", "14 · Wynagrodzenia w gotówce i w naturze"},
	"pf_k15": {"15 · Прочие расходы", "15 · Pozostałe wydatki"}, "pf_k16": {"16 · Расходы всего", "16 · Razem wydatki"},
	"pf_k19": {"19 · Примечание", "19 · Uwagi"},
	"pf_day_sales": {"Продажи дня: время, пакеты, товары и кухня — сводка 1448",
		"Sprzedaż dnia: czas gry, pakiety, towary i kuchnia — zestawienie 1448"},
	"pf_day_gifts": {"Подарки товаров дня — сводка 1448", "Nieodpłatne przekazanie towarów — zestawienie 1448"},
	"pf_unpaid":    {"не оплачен", "nieopłacony"}, "pf_month": {"Итого %s", "Razem %s"},
	// ewidencja przychodów
	"pf_e_day": {"Дата дохода", "Data uzyskania przychodu"}, "pf_e_rate": {"Доход по ставке %s", "Przychód wg stawki %s"},
	"pf_e_total": {"Доход всего", "Ogółem przychód"},
	// реестры
	"pf_r_day": {"Дата продажи", "Data sprzedaży"}, "pf_r_net": {"Нетто %s", "Netto %s"},
	"pf_r_zw": {"Продажа zw", "Sprzedaż zwolniona"}, "pf_r_tnet": {"Итого нетто", "Razem netto"},
	"pf_r_tvat": {"Итого VAT", "Razem VAT"}, "pf_r_value": {"Сумма продаж", "Wartość sprzedaży"},
	"pf_b_doc": {"Номер документа", "Nr dokumentu"}, "pf_account": {"Счёт", "Konto"},
	// «Расходы»
	"pf_kpir_col": {"Колонка KPiR", "Kolumna KPiR"}, "pf_kpir_none": {"не в книгу", "poza KPiR"},
}

// profileAboutWords — строки «О выгрузке» о профиле и реквизитах: [русский, польский].
var profileAboutWords = map[string][2]string{
	"firm": {"Фирма", "Firma"},
	"profile_0": {"KPiR — книга доходов и расходов (podatkowa księga przychodów i rozchodów)",
		"KPiR — podatkowa księga przychodów i rozchodów"},
	"profile_1": {"ryczałt — налог с доходов по ставкам (ryczałt od przychodów ewidencjonowanych)",
		"ryczałt od przychodów ewidencjonowanych"},
	"profile_2": {"полная бухгалтерия (pełna księgowość)", "pełna księgowość (księgi rachunkowe)"},
	"pf_income": {"Выручка для книги", "Przychód"},
	"pf_income_v": {"по клубным суткам и тому же моменту, что VAT (%s); %s; подарки — не выручка",
		"wg doby klubowej i tego samego momentu co VAT (%s); %s; nieodpłatne przekazania — nie są przychodem"},
	"pf_moment_spend": {"при трате", "przy wykorzystaniu"}, "pf_moment_topup": {"при пополнении", "przy doładowaniu"},
	"pf_net_payer": {"нетто: брутто дня минус VAT дня по ставке, как в «Продажах по дням и ставкам»",
		"netto: brutto dnia minus VAT dnia wg stawki, jak w «Sprzedaż wg dni i stawek»"},
	"pf_net_zw": {"брутто: клуб освобождён от VAT", "brutto: klub zwolniony z VAT"},
	"pf_kpir":   {"Записи для KPiR", "Zapisy do KPiR"},
	"pf_kpir_v": {"колонки — по образцу книги с 2026 года: 9 — продажи дня; расходы — по дню операции, без сторно, в колонке своей категории (12 — закупка товаров и материалов, 13 — побочные затраты закупки, 14 — зарплаты, 15 — прочие; 16 = 14 + 15), %s; колонки 3 (KSeF), 7 (адрес), 17 и 18 заполняет бухгалтер; колонки категорий — в «Справочниках»",
		"kolumny wg wzoru księgi od 2026 r.: 9 — sprzedaż dnia; wydatki — wg daty operacji, bez storn, w kolumnie swojej kategorii (12 — zakup towarów handlowych i materiałów, 13 — koszty uboczne zakupu, 14 — wynagrodzenia, 15 — pozostałe; 16 = 14 + 15), %s; kolumny 3 (KSeF), 7 (adres), 17 i 18 uzupełnia księgowy; kolumny kategorii — w ustawieniach klubu"},
	"pf_exp_net":   {"нетто (брутто минус VAT документа)", "netto (brutto minus VAT z dokumentu)"},
	"pf_exp_gross": {"брутто", "brutto"},
	"pf_outside":   {"Не в книгу", "Poza KPiR"}, "pf_outside_none": {"нет", "brak"},
	"pf_outside_v": {"«%s» — %d шт., %s zł", "«%s» — %d szt., %s zł"},
	"pf_ry":        {"Ставки ryczałtu", "Stawki ryczałtu"},
	"pf_ry_v": {"время за ПК и пакеты — %s (настройка); товары и кухня — по ставке позиции: %s; пополнения «при пополнении» — по ставке времени; ставки нынешние: смена ставки меняет и прошлые периоды выгрузки",
		"czas gry i pakiety — %s (ustawienie); towary i kuchnia — wg stawki pozycji: %s; doładowania «przy doładowaniu» — wg stawki czasu; stawki bieżące: zmiana stawki zmienia też wcześniejsze okresy w eksporcie"},
	"pf_ry_goods": {"%s — %d поз.", "%s — %d poz."}, "pf_ry_nogoods": {"позиций нет", "brak pozycji"},
	"pf_regs": {"Реестры", "Rejestry"},
	"pf_regs_v": {"продажи — по дням и ставкам, как «Продажи по дням и ставкам», подарки товаров — своей строкой; покупки — расходы периода с VAT по документу, по дню операции, без сторно, со счётом категории; касса — «Касса» с KP/KW",
		"sprzedaż — wg dni i stawek, jak «Sprzedaż wg dni i stawek», nieodpłatne przekazania — osobnym wierszem; zakupy — wydatki okresu z VAT z dokumentu, wg daty operacji, bez storn, z kontem kategorii; kasa — «Raport kasowy» z KP/KW"},
	"pf_regs_zw": {"клуб освобождён от VAT: учёт продаж — брутто по дням; покупки и расходы — все расходы периода, брутто, по дню операции, без сторно, со счётом категории; касса — «Касса» с KP/KW",
		"klub zwolniony z VAT: ewidencja sprzedaży — brutto wg dni; zakupy i wydatki — wszystkie wydatki okresu, brutto, wg daty operacji, bez storn, z kontem kategorii; kasa — «Raport kasowy» z KP/KW"},
	"pf_accounts":   {"Счета категорий", "Konta kategorii"},
	"pf_accounts_v": {"заданы у %d из %d категорий; у остальных — пусто, счёт выберет бухгалтер", "ustawione w %d z %d kategorii; w pozostałych — puste, konto wybierze księgowy"},
}

func init() {
	for k, v := range profileWords {
		exportWords[k] = v
	}
	for k, v := range profileAboutWords {
		exportAboutWords[k] = v
	}
}

// ── продажи по частям ───────────────────────────────────────────────────────────────────────────

// profKey — часть продаж дня: ставка VAT (у освобождённого — zw), подарок или нет, ставка ryczałtu (у подарка — 0).
type profKey struct {
	D    string
	Vat  int
	Gift bool
	Ry   int
}

// profCell — брутто части и её доля VAT дня, гроши.
type profCell struct {
	Gross, Vat int64
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// profileSplit — облагаемое периода по частям (чистая, тест): VAT дня по ставке — vatOf от брутто дня, как в
// «Продажах по дням и ставкам»; у части — vatOf своего брутто, а грош округления — самой большой по модулю части (при
// равных — продаже раньше подарка, большей ставке ryczałtu раньше меньшей): части в сумме — VAT дня до гроша.
func profileSplit(rows []vatRow, payer bool) map[profKey]*profCell {
	type dayRate struct {
		D   string
		Vat int
	}
	cells, groups := map[profKey]*profCell{}, map[dayRate][]profKey{}
	for _, x := range rows {
		k := profKey{D: x.D, Vat: x.Vat, Gift: x.Kind == "gifts"}
		if !payer {
			k.Vat = models.VatZW
		}
		if !k.Gift {
			k.Ry = x.Ry
		}
		if cells[k] == nil {
			cells[k] = &profCell{}
			g := dayRate{k.D, k.Vat}
			groups[g] = append(groups[g], k)
		}
		cells[k].Gross += x.G
	}
	for g, keys := range groups {
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].Gift != keys[j].Gift {
				return !keys[i].Gift
			}
			return keys[i].Ry > keys[j].Ry
		})
		var total, sum int64
		big := keys[0]
		for _, k := range keys {
			c := cells[k]
			total += c.Gross
			c.Vat = vatOf(c.Gross, g.Vat)
			sum += c.Vat
			if abs64(c.Gross) > abs64(cells[big].Gross) {
				big = k
			}
		}
		cells[big].Vat += vatOf(total, g.Vat) - sum
	}
	return cells
}

// profDay — продажи клубных суток для листов профиля.
type profDay struct {
	Day   string            // ГГГГ-ММ-ДД
	Net   int64             // выручка без подарков: нетто (освобождённому — брутто)
	Ry    map[int]int64     // выручка по ставкам ryczałtu
	Sales map[int]*profCell // ставка VAT → продажи без подарков: брутто и VAT
	Gifts map[int]*profCell // ставка VAT → подарки товаров
}

// profileDays — части по дням, дни по порядку (чистая, тест).
func profileDays(cells map[profKey]*profCell) []profDay {
	byDay := map[string]*profDay{}
	for k, c := range cells {
		d := byDay[k.D]
		if d == nil {
			d = &profDay{Day: k.D, Ry: map[int]int64{}, Sales: map[int]*profCell{}, Gifts: map[int]*profCell{}}
			byDay[k.D] = d
		}
		m := d.Sales
		if k.Gift {
			m = d.Gifts
		} else {
			d.Net += c.Gross - c.Vat
			d.Ry[k.Ry] += c.Gross - c.Vat
		}
		if m[k.Vat] == nil {
			m[k.Vat] = &profCell{}
		}
		m[k.Vat].Gross += c.Gross
		m[k.Vat].Vat += c.Vat
	}
	out := make([]profDay, 0, len(byDay))
	for _, d := range byDay {
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Day < out[j].Day })
	return out
}

// ── расходы ─────────────────────────────────────────────────────────────────────────────────────

// profExpense — расход периода для листов профиля (без сторно).
type profExpense struct {
	Day                               time.Time // день операции
	Doc, NIP, Party, Cat, Descr, Acct string
	ID                                string
	Col                               int // колонка KPiR категории
	Gross, Vat                        int64
	Paid                              bool
}

// amount — сумма расхода для книги: плательщику VAT — нетто, освобождённому — брутто.
func (e profExpense) amount(payer bool) int64 {
	if payer {
		return e.Gross - e.Vat
	}
	return e.Gross
}

// profileExpenses — расходы периода по дню операции без сторно, по порядку, как в «Расходах».
func profileExpenses(p period) []profExpense {
	var list []models.Expense
	expensesOfPeriod(p).Where("storno_at IS NULL").Order("op_date, created_at, id").Find(&list)
	var cats []models.ExpenseCategory
	db.Find(&cats)
	cat := map[uuid.UUID]models.ExpenseCategory{}
	for _, c := range cats {
		cat[c.ID] = c
	}
	cps := counterpartiesByID()
	out := make([]profExpense, 0, len(list))
	for i := range list {
		e := &list[i]
		c := cat[e.CategoryID]
		x := profExpense{Day: e.OpDate, Doc: e.DocNo, Cat: c.Name, Descr: c.Name, Acct: c.Account, ID: e.ID.String(), Col: c.KpirCol,
			Gross: e.GrossGrosz, Vat: e.VatGrosz, Paid: e.Paid}
		if e.Note != "" {
			x.Descr += ": " + e.Note
		}
		if e.CounterpartyID != nil {
			cp := cps[e.CounterpartyID.String()]
			x.Party = cp.Name
			if cp.NIP != nil {
				x.NIP = *cp.NIP
			}
		}
		out = append(out, x)
	}
	return out
}

// ── итоги ───────────────────────────────────────────────────────────────────────────────────────

// withTotals — строки листа с итогами (чистая, тест): если строки захватывают больше одного месяца — «Итого ММ.ГГГГ»
// после каждого месяца, в конце — «Итого» за период; суммы — денежных колонок cols. months — месяц строки, ГГГГ-ММ.
func withTotals(rows []xrow, months []string, cols []int, width, lang int) []xrow {
	multi := false
	for i := 1; i < len(months); i++ {
		multi = multi || months[i] != months[0]
	}
	sum := func(from, to int, label string) xrow {
		cells := make([]xcell, width)
		cells[0] = cText(label)
		for _, c := range cols {
			var s int64
			for _, r := range rows[from:to] {
				if c < len(r.cells) && r.cells[c].k == 'm' {
					s += r.cells[c].n
				}
			}
			cells[c] = cMoney(s)
		}
		return xrow{cells: cells, bold: true}
	}
	out := make([]xrow, 0, len(rows)+4)
	start := 0
	for i := range rows {
		out = append(out, rows[i])
		if multi && (i+1 == len(rows) || months[i+1] != months[i]) {
			m := months[i]
			out = append(out, sum(start, i+1, fmt.Sprintf(tr(lang, "pf_month"), m[5:7]+"."+m[:4])))
			start = i + 1
		}
	}
	return append(out, sum(0, len(rows), tr(lang, "total_row")))
}

func moneyOpt(v int64, ok bool) xcell {
	if !ok {
		return xcell{}
	}
	return cMoney(v)
}

// ── листы ───────────────────────────────────────────────────────────────────────────────────────

func dayOfKey(k string, loc *time.Location) time.Time {
	t, _ := time.ParseInLocation("2006-01-02", k, loc)
	return t
}

func heads(lang int, keys ...string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = tr(lang, k)
	}
	return out
}

// sheetKpir — «Записи для KPiR» (чистая, тест): день — строка выручки (колонка 9), затем расходы дня в колонках своих
// категорий; категории «не в книгу» — нет.
func sheetKpir(days []profDay, exps []profExpense, payer bool, loc *time.Location, lang int) xsheet {
	sh := xsheet{name: tr(lang, "pf_sheet_kpir"), file: []string{"zapisi_kpir", "zapisy_kpir"}[lang], table: true,
		head: heads(lang, "h_n", "pf_k2", "pf_k4", "pf_k5", "pf_k6", "pf_k8", "pf_k9", "pf_k10", "pf_k11", "pf_k12", "pf_k13",
			"pf_k14", "pf_k15", "pf_k16", "pf_k19", "h_id"),
		widths: []int{7, 12, 18, 13, 24, 40, 15, 13, 14, 15, 14, 15, 15, 14, 16, 38}}
	colIdx := map[int]int{models.KpirGoods: 9, models.KpirSide: 10, models.KpirWages: 11, models.KpirOther: 12}
	byDay := map[string][]profExpense{}
	keys := []string{}
	for _, d := range days {
		keys = append(keys, d.Day)
	}
	for _, e := range exps {
		if e.Col == models.KpirNone || colIdx[e.Col] == 0 {
			continue
		}
		k := e.Day.Format("2006-01-02")
		if _, ok := byDay[k]; !ok {
			keys = append(keys, k)
		}
		byDay[k] = append(byDay[k], e)
	}
	sort.Strings(keys)
	sales := map[string]profDay{}
	for _, d := range days {
		sales[d.Day] = d
	}
	rows, months := []xrow{}, []string{}
	add := func(cells []xcell, day string) {
		cells[0] = cInt(int64(len(rows) + 1))
		rows, months = append(rows, xrow{cells: cells}), append(months, day[:7])
	}
	for i, k := range keys {
		if i > 0 && k == keys[i-1] {
			continue
		}
		if d, ok := sales[k]; ok && len(d.Sales) > 0 {
			cells := make([]xcell, len(sh.head))
			cells[1], cells[5] = cDay(dayOfKey(k, loc)), cText(tr(lang, "pf_day_sales"))
			cells[6], cells[8] = cMoney(d.Net), cMoney(d.Net)
			add(cells, k)
		}
		for _, e := range byDay[k] {
			cells := make([]xcell, len(sh.head))
			v := e.amount(payer)
			cells[1], cells[2], cells[3], cells[4], cells[5] = cDay(e.Day), cText(e.Doc), cText(e.NIP), cText(e.Party), cText(e.Descr)
			cells[colIdx[e.Col]] = cMoney(v)
			if e.Col == models.KpirWages || e.Col == models.KpirOther {
				cells[13] = cMoney(v)
			}
			if !e.Paid {
				cells[14] = cText(tr(lang, "pf_unpaid"))
			}
			cells[15] = cText(e.ID)
			add(cells, k)
		}
	}
	sh.n = len(rows)
	sh.rows = withTotals(rows, months, []int{6, 7, 8, 9, 10, 11, 12, 13}, len(sh.head), lang)
	return sh
}

// sheetEwid — «Доходы по ставкам ryczałtu» (чистая, тест): день — строка, выручка по ставкам от большей к меньшей
// (ставка времени — всегда колонкой), «Доход всего».
func sheetEwid(days []profDay, timeRate int, loc *time.Location, lang int) xsheet {
	set := map[int]bool{timeRate: true}
	for _, d := range days {
		for r := range d.Ry {
			set[r] = true
		}
	}
	rates := make([]int, 0, len(set))
	for r := range set {
		rates = append(rates, r)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(rates)))
	sh := xsheet{name: tr(lang, "pf_sheet_ewid"), file: []string{"dohody_ryczalt", "ewidencja_przychodow"}[lang], table: true,
		head: heads(lang, "h_n", "pf_e_day", "h_desc"), widths: []int{7, 13, 52}}
	cols := []int{}
	for _, r := range rates {
		cols = append(cols, len(sh.head))
		sh.head = append(sh.head, fmt.Sprintf(tr(lang, "pf_e_rate"), ryLabel(lang, r)))
		sh.widths = append(sh.widths, 15)
	}
	cols = append(cols, len(sh.head))
	sh.head = append(sh.head, tr(lang, "pf_e_total"))
	sh.widths = append(sh.widths, 15)
	rows, months := []xrow{}, []string{}
	for _, d := range days {
		if len(d.Sales) == 0 {
			continue
		}
		cells := []xcell{cInt(int64(len(rows) + 1)), cDay(dayOfKey(d.Day, loc)), cText(tr(lang, "pf_day_sales"))}
		for _, r := range rates {
			v, ok := d.Ry[r]
			cells = append(cells, moneyOpt(v, ok))
		}
		cells = append(cells, cMoney(d.Net))
		rows, months = append(rows, xrow{cells: cells}), append(months, d.Day[:7])
	}
	sh.n = len(rows)
	sh.rows = withTotals(rows, months, cols, len(sh.head), lang)
	return sh
}

// ryLabel — ставка ryczałtu для файла: по-русски «8,5 %», по-польски «8,5%».
func ryLabel(lang, r int) string {
	l := models.RyczaltLabel(r)
	if lang == 1 {
		l = strings.ReplaceAll(l, " ", "")
	}
	return l
}

// sheetSalesReg — «Реестр продаж VAT» (чистая, тест): день — строка продаж и строка подарков товаров (если были), по
// ставкам нетто и VAT (у 0 % и zw — только нетто), итоги нетто, VAT и брутто. Освобождённому — «Учёт продаж»: день —
// одна строка брутто, без колонок VAT.
func sheetSalesReg(days []profDay, payer bool, loc *time.Location, lang int) xsheet {
	if !payer {
		sh := xsheet{name: tr(lang, "pf_sheet_sreg_zw"), file: []string{"uchet_prodazh", "ewidencja_sprzedazy"}[lang], table: true,
			head: heads(lang, "h_n", "pf_r_day", "h_desc", "pf_r_value"), widths: []int{7, 13, 52, 16}}
		rows, months := []xrow{}, []string{}
		for _, d := range days {
			if len(d.Sales) == 0 {
				continue
			}
			rows = append(rows, xrow{cells: []xcell{cInt(int64(len(rows) + 1)), cDay(dayOfKey(d.Day, loc)), cText(tr(lang, "pf_day_sales")), cMoney(d.Net)}})
			months = append(months, d.Day[:7])
		}
		sh.n = len(rows)
		sh.rows = withTotals(rows, months, []int{3}, len(sh.head), lang)
		return sh
	}
	all := map[int]*vatCell{}
	for _, d := range days {
		for _, m := range []map[int]*profCell{d.Sales, d.Gifts} {
			for r := range m {
				all[r] = &vatCell{}
			}
		}
	}
	rates := vatOrder(all)
	sh := xsheet{name: tr(lang, "pf_sheet_sreg"), file: []string{"reestr_prodazh_vat", "rejestr_sprzedazy_vat"}[lang], table: true,
		head: heads(lang, "h_n", "pf_r_day", "h_desc"), widths: []int{7, 13, 52}}
	netCol, vatCol, cols := map[int]int{}, map[int]int{}, []int{}
	for _, r := range rates {
		netCol[r] = len(sh.head)
		cols = append(cols, len(sh.head))
		if r == models.VatZW {
			sh.head = append(sh.head, tr(lang, "pf_r_zw"))
		} else {
			sh.head = append(sh.head, fmt.Sprintf(tr(lang, "pf_r_net"), rateLabel(lang, r)))
		}
		sh.widths = append(sh.widths, 14)
		if r > 0 {
			vatCol[r] = len(sh.head)
			cols = append(cols, len(sh.head))
			sh.head = append(sh.head, "VAT "+rateLabel(lang, r))
			sh.widths = append(sh.widths, 13)
		}
	}
	tn := len(sh.head)
	sh.head = append(sh.head, tr(lang, "pf_r_tnet"), tr(lang, "pf_r_tvat"), tr(lang, "h_gross"))
	sh.widths = append(sh.widths, 14, 13, 14)
	cols = append(cols, tn, tn+1, tn+2)
	rows, months := []xrow{}, []string{}
	line := func(d profDay, m map[int]*profCell, text string) {
		cells := make([]xcell, len(sh.head))
		cells[0], cells[1], cells[2] = cInt(int64(len(rows)+1)), cDay(dayOfKey(d.Day, loc)), cText(text)
		var g, v int64
		for r, c := range m {
			cells[netCol[r]] = cMoney(c.Gross - c.Vat)
			if i, ok := vatCol[r]; ok {
				cells[i] = cMoney(c.Vat)
			}
			g, v = g+c.Gross, v+c.Vat
		}
		cells[tn], cells[tn+1], cells[tn+2] = cMoney(g-v), cMoney(v), cMoney(g)
		rows, months = append(rows, xrow{cells: cells}), append(months, d.Day[:7])
	}
	for _, d := range days {
		if len(d.Sales) > 0 {
			line(d, d.Sales, tr(lang, "pf_day_sales"))
		}
		if len(d.Gifts) > 0 {
			line(d, d.Gifts, tr(lang, "pf_day_gifts"))
		}
	}
	sh.n = len(rows)
	sh.rows = withTotals(rows, months, cols, len(sh.head), lang)
	return sh
}

// sheetBuyReg — «Реестр покупок VAT» (чистая, тест): расходы с VAT по документу — нетто, VAT, брутто, счёт категории.
// Освобождённому — «Покупки и расходы»: все расходы, брутто, без колонок VAT.
func sheetBuyReg(exps []profExpense, payer bool, lang int) xsheet {
	sh := xsheet{name: tr(lang, "pf_sheet_preg"), file: []string{"reestr_pokupok_vat", "rejestr_zakupow_vat"}[lang], table: true,
		head:   heads(lang, "h_n", "x_date", "pf_b_doc", "x_party", "x_nip", "h_desc", "pf_account", "h_net", "h_vat", "h_gross", "h_note", "h_id"),
		widths: []int{7, 13, 18, 26, 13, 40, 10, 14, 12, 14, 14, 38}}
	cols := []int{7, 8, 9}
	if !payer {
		sh = xsheet{name: tr(lang, "pf_sheet_preg_zw"), file: []string{"pokupki_i_rashody", "zakupy_i_wydatki"}[lang], table: true,
			head:   heads(lang, "h_n", "x_date", "pf_b_doc", "x_party", "x_nip", "h_desc", "pf_account", "h_sum", "h_note", "h_id"),
			widths: []int{7, 13, 18, 26, 13, 40, 10, 14, 14, 38}}
		cols = []int{7}
	}
	rows, months := []xrow{}, []string{}
	for _, e := range exps {
		if payer && e.Vat == 0 {
			continue
		}
		cells := []xcell{cInt(int64(len(rows) + 1)), cDay(e.Day), cText(e.Doc), cText(e.Party), cText(e.NIP), cText(e.Descr), cText(e.Acct)}
		if payer {
			cells = append(cells, cMoney(e.Gross-e.Vat), cMoney(e.Vat), cMoney(e.Gross))
		} else {
			cells = append(cells, cMoney(e.Gross))
		}
		note := ""
		if !e.Paid {
			note = tr(lang, "pf_unpaid")
		}
		cells = append(cells, cText(note), cText(e.ID))
		rows, months = append(rows, xrow{cells: cells}), append(months, e.Day.Format("2006-01"))
	}
	sh.n = len(rows)
	sh.rows = withTotals(rows, months, cols, len(sh.head), lang)
	return sh
}

// ── профиль целиком ─────────────────────────────────────────────────────────────────────────────

// profileData — всё для листов профиля и его строк в «О выгрузке».
type profileData struct {
	Form     int
	Cfg      vatConfig
	Days     []profDay
	Exps     []profExpense
	GoodsRy  map[int]int // ставка ryczałtu → позиций с ней
	Accounts [2]int      // категорий со счётом, всего категорий
}

// loadProfile — форма учёта из настроек, продажи по частям из облагаемых строк периода, расходы периода, ставки
// позиций и счета категорий.
func loadProfile(p period, cfg vatConfig, rows []vatRow) profileData {
	pd := profileData{Form: int(exportSetting("acc_form")), Cfg: cfg, Days: profileDays(profileSplit(rows, cfg.Payer)),
		Exps: profileExpenses(p), GoodsRy: map[int]int{}}
	var goods []struct {
		Rate int
		N    int
	}
	db.Raw("SELECT ryczalt_rate AS rate, COUNT(*) AS n FROM goods GROUP BY 1").Scan(&goods)
	for _, g := range goods {
		pd.GoodsRy[g.Rate] = g.N
	}
	db.Raw("SELECT COUNT(*) FILTER (WHERE account <> ''), COUNT(*) FROM expense_categories").Row().Scan(&pd.Accounts[0], &pd.Accounts[1])
	return pd
}

// profileSheets — листы профиля сразу после «О выгрузке» (чистая, тест); cash — «Касса»: у полной бухгалтерии она
// переезжает к реестрам (moved — да).
func profileSheets(pd profileData, cash xsheet, loc *time.Location, lang int) (sheets []xsheet, moved bool) {
	switch pd.Form {
	case formRyczalt:
		return []xsheet{sheetEwid(pd.Days, pd.Cfg.RyTime, loc, lang)}, false
	case formFull:
		return []xsheet{sheetSalesReg(pd.Days, pd.Cfg.Payer, loc, lang), sheetBuyReg(pd.Exps, pd.Cfg.Payer, lang), cash}, true
	}
	return []xsheet{sheetKpir(pd.Days, pd.Exps, pd.Cfg.Payer, loc, lang)}, false
}

// profileAbout — строки «О выгрузке» о профиле (чистая, тест): форма учёта и как читать её листы.
func profileAbout(pd profileData, lang int) []xrow {
	row := func(key, v string) xrow { return xrow{cells: []xcell{cText(ta(lang, key)), cText(v)}} }
	moment, net := ta(lang, "pf_moment_spend"), ta(lang, "pf_net_payer")
	if pd.Cfg.Topup {
		moment = ta(lang, "pf_moment_topup")
	}
	if !pd.Cfg.Payer {
		net = ta(lang, "pf_net_zw")
	}
	out := []xrow{row("profile", ta(lang, "profile_"+strconv.Itoa(pd.Form))), row("pf_income", fmt.Sprintf(ta(lang, "pf_income_v"), moment, net))}
	switch pd.Form {
	case formRyczalt:
		rates := make([]int, 0, len(pd.GoodsRy))
		for r := range pd.GoodsRy {
			rates = append(rates, r)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(rates)))
		goods := []string{}
		for _, r := range rates {
			goods = append(goods, fmt.Sprintf(ta(lang, "pf_ry_goods"), ryLabel(lang, r), pd.GoodsRy[r]))
		}
		if len(goods) == 0 {
			goods = append(goods, ta(lang, "pf_ry_nogoods"))
		}
		out = append(out, row("pf_ry", fmt.Sprintf(ta(lang, "pf_ry_v"), ryLabel(lang, pd.Cfg.RyTime), strings.Join(goods, ", "))))
	case formFull:
		regs := ta(lang, "pf_regs_v")
		if !pd.Cfg.Payer {
			regs = ta(lang, "pf_regs_zw")
		}
		out = append(out, row("pf_regs", regs), row("pf_accounts", fmt.Sprintf(ta(lang, "pf_accounts_v"), pd.Accounts[0], pd.Accounts[1])))
	default:
		amount := ta(lang, "pf_exp_net")
		if !pd.Cfg.Payer {
			amount = ta(lang, "pf_exp_gross")
		}
		out = append(out, row("pf_kpir", fmt.Sprintf(ta(lang, "pf_kpir_v"), amount)))
		type agg struct {
			n   int
			sum int64
		}
		outside, names := map[string]*agg{}, []string{}
		for _, e := range pd.Exps {
			if e.Col != models.KpirNone {
				continue
			}
			if outside[e.Cat] == nil {
				outside[e.Cat] = &agg{}
				names = append(names, e.Cat)
			}
			outside[e.Cat].n++
			outside[e.Cat].sum += e.amount(pd.Cfg.Payer)
		}
		parts := []string{}
		for _, n := range names {
			parts = append(parts, fmt.Sprintf(ta(lang, "pf_outside_v"), n, outside[n].n, moneyText(outside[n].sum, ',')))
		}
		if len(parts) == 0 {
			parts = append(parts, ta(lang, "pf_outside_none"))
		}
		out = append(out, row("pf_outside", strings.Join(parts, "; ")))
	}
	return out
}

// expenseFormCol — колонка формы учёта в «Расходах» (чистая, тест): у KPiR — колонка категории («не в книгу» —
// словами), у полной — счёт; у ryczałtu колонки нет (ok — false).
func expenseFormCol(form int, c models.ExpenseCategory, lang int) (xcell, bool) {
	switch form {
	case formKpir:
		if c.KpirCol == models.KpirNone {
			return cText(tr(lang, "pf_kpir_none")), true
		}
		return cText(strconv.Itoa(c.KpirCol)), true
	case formFull:
		return cText(c.Account), true
	}
	return xcell{}, false
}
