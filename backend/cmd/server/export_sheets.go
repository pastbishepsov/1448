package main

// З13-и5 «Листы выгрузки» (OWNER.md, трек З; №75; решения основателя 04.10, З-Р37). К трём листам З13-и4 —
// десять листов на тех же функциях, что экраны, чтобы цифры сходились:
//
//	«Отработано по дням» — «Отработанная выручка» «Обзора» (earnedAggIn) по клубным суткам, итог — за период;
//	«Расходы» — список «Расходов» периода по дню операции (expensesOfPeriod), сторнированные — с пометкой, вне итога;
//	«Закупки» — поставки, принятые в периоде, по кускам с ценами, как в «Закупках» (pricingOut);
//	«Касса» — кассовая книга (cashBookIn) документами: наличные за день по видам — по документу KP, возвраты,
//	  расходы из кассы, возвраты владельцу, инкассации, коррекции — каждый своим, расхождение пересчётов дня — строкой;
//	  номера KP/KW — настройкой (с начала месяца, с начала года или без номеров);
//	«Сдачи касс» — сдачи книги с расходами, которые их объяснили;
//	«Возвраты» — отмены с деньгами по дню отмены (returnsIn), как блок «Возвраты»;
//	«Списанные долги» — что списали в периоде (не отменённые списания) с датой продажи и датой списания;
//	«Подарено» — подарки товаров и пакетов (giftsSQL) с ценой и себестоимостью партий;
//	«Склад на конец периода» — партии с остатком и ценой закупки (для remanentu);
//	«Предоплаты и долги на конец» — кошельки, неотыгранные пакеты и долги гостей.
//
// «На конец периода» — конец последних клубных суток, у текущего периода — момент выгрузки. Сейчас это те же
// снимки, что на экране («Обязательства клуба», «Склад сейчас», «Дебиторка»); на прошлый момент — тем же правилом,
// откатом от «сейчас» по журналам: кошелёк — по операциям кошелька, пакет — по журналу расхода минут (З12-и2),
// партия — по движениям партий (З10-и3), долг — по моментам продажи, оплаты, отмены и списания.

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ── слова листов: [русский, польский] ───────────────────────────────────────────────────────────

var sheetWords = map[string][2]string{
	"sheet_earned": {"Отработано по дням", "Przychód wykonany wg dni"}, "sheet_expenses": {"Расходы", "Wydatki"},
	"sheet_purchases": {"Закупки", "Zakupy towarów"}, "sheet_cash": {"Касса", "Raport kasowy"},
	"sheet_closeouts": {"Сдачи касс", "Rozliczenia zmian"}, "sheet_returns": {"Возвраты", "Zwroty"},
	"sheet_writeoffs": {"Списанные долги", "Należności spisane"}, "sheet_gifts": {"Подарено", "Przekazane nieodpłatnie"},
	"sheet_stock": {"Склад на конец периода", "Magazyn na koniec okresu"},
	"sheet_owed":  {"Предоплаты и долги на конец", "Stan przedpłat i należności"},
	"total_row":   {"Итого", "Razem"},
	// «Отработано по дням»
	"e_wallet": {"Время с кошелька", "Czas gry z portfela"}, "e_packs": {"Пакеты по журналу", "Pakiety — wykorzystane"},
	"e_est":   {"Пакеты до журнала (оценка)", "Pakiety sprzed ewidencji (szacunek)"},
	"e_goods": {"Товары", "Towary"}, "e_kitchen": {"Кухня", "Kuchnia"}, "e_total": {"Отработано", "Razem wykonano"},
	"e_given_min": {"Отдано временем, мин", "Czas oddany (monety, rekompensaty), min"},
	"e_given":     {"Отдано временем, zł", "Czas oddany, zł"},
	// «Расходы»
	"x_date": {"Дата операции", "Data operacji"}, "x_cat": {"Категория", "Kategoria"}, "x_purchase": {"Закупка товара", "Zakup towaru"},
	"x_party": {"Контрагент", "Kontrahent"}, "x_nip": {"NIP контрагента", "NIP kontrahenta"}, "x_paid": {"Оплачен", "Opłacony"},
	"x_drawer": {"Ушёл из кассы", "Wypłata z kasy"}, "x_by": {"Внёс", "Wprowadził"},
	"x_storno": {"Сторно — когда", "Storno — kiedy"}, "x_storno_by": {"Сторно — кто", "Storno — kto"},
	"x_storno_why": {"Сторно — причина", "Storno — powód"}, "x_total": {"Итого без сторно", "Razem bez storn"},
	// «Закупки»
	"p_date": {"Дата приёмки", "Data przyjęcia"}, "p_supplier": {"Поставщик", "Dostawca"},
	"p_doc_date": {"Дата документа", "Data dokumentu"}, "p_good": {"Позиция", "Towar"},
	"p_price": {"Цена за шт., брутто", "Cena za szt. brutto"}, "p_value": {"Сумма, брутто", "Wartość brutto"},
	"p_status": {"Статус", "Status"}, "p_received_by": {"Принял", "Przyjął"}, "p_priced_by": {"Оценил", "Wycenił"},
	"st_waiting": {"ждёт цен", "czeka na ceny"}, "st_priced": {"оценена", "wyceniona"},
	"p_total": {"Итого оценено", "Razem wycenione"},
	// «Касса»
	"c_no": {"Номер документа", "Nr dokumentu"}, "c_kind": {"Документ", "Rodzaj"}, "c_text": {"Операция", "Treść operacji"},
	"c_in": {"Приход", "Przychód"}, "c_out": {"Расход", "Rozchód"}, "c_bal": {"Остаток в кассе", "Stan kasy"},
	"c_open": {"Остаток на начало дня", "Stan kasy na początek dnia"}, "c_close": {"Остаток на конец дня", "Stan kasy na koniec dnia"},
	"c_none":     {"ящика ещё не было", "kasa jeszcze nie prowadzona"},
	"c_base":     {"Начальный остаток — первая сдача с пересчётом: %s, %s", "Stan początkowy — pierwsze przeliczenie: %s, %s"},
	"c_deposits": {"Пополнения кошельков — наличные за день", "Doładowania portfeli — gotówka za dzień"},
	"c_goods":    {"Продажи товаров и кухни — наличные за день", "Sprzedaż towarów i kuchni — gotówka za dzień"},
	"c_packs":    {"Пакеты времени — наличные за день", "Pakiety czasu — gotówka za dzień"},
	"c_refund":   {"Возврат: %s", "Zwrot: %s"}, "c_expense": {"Расход: %s", "Wydatek: %s"},
	"c_owner":   {"Возврат владельцу", "Zwrot właścicielowi"},
	"c_collect": {"Инкассация", "Wypłata z kasy (inkaso)"},
	"c_correct": {"Коррекция остатка: в ящике названо %s zł", "Korekta stanu kasy: podano %s zł"},
	"c_surplus": {"Расхождение пересчётов дня: излишек", "Różnica kasowa dnia: nadwyżka"},
	"c_short":   {"Расхождение пересчётов дня: недостача", "Różnica kasowa dnia: niedobór"},
	// «Сдачи касс»
	"z_open": {"Остаток на начало", "Stan na początek"}, "z_in": {"Пришло", "Wpływy"}, "z_out": {"Ушло", "Wypłaty"},
	"z_adjust": {"Коррекции", "Korekty"}, "z_expected": {"Должно быть", "Powinno być"}, "z_counted": {"Пересчитано", "Przeliczono"},
	"z_diff": {"Расхождение", "Różnica"}, "z_explained": {"Объяснено расходами", "Wyjaśniono wydatkami"},
	"z_deposits": {"Пополнения", "Doładowania"}, "z_goods": {"Товары и кухня", "Towary i kuchnia"}, "z_packs": {"Пакеты", "Pakiety"},
	"z_refunds": {"Возвраты", "Zwroty"}, "z_collect": {"Инкассации", "Wypłaty (inkaso)"},
	"z_expenses": {"Расходы из кассы", "Wydatki z kasy"}, "z_owner": {"Возвраты владельцу", "Zwroty właścicielowi"},
	"z_skipped": {"Без пересчёта", "Bez przeliczenia"}, "z_baseline": {"Нулевая точка", "Stan początkowy"},
	// «Возвраты»
	"r_at": {"Когда отменили", "Data anulowania"}, "r_kind": {"Вид", "Rodzaj"}, "r_what": {"Что", "Pozycja"},
	"r_guest": {"Гость", "Klient"}, "r_paid": {"Оплачено", "Zapłacono"}, "r_paid_method": {"Как платили", "Forma zapłaty"},
	"r_paid_at": {"Когда платили", "Data zapłaty"}, "r_refund": {"Вернули", "Zwrócono"},
	"r_refund_method": {"Как вернули", "Forma zwrotu"}, "r_by": {"Кто отменил", "Anulował"},
	"r_earlier": {"Оплачено раньше периода", "Zapłacono przed okresem"}, "r_receipt": {"Чек", "Paragon"},
	"rk_deposit": {"пополнение", "doładowanie"}, "rk_sale": {"продажа", "sprzedaż"}, "rk_package": {"пакет", "pakiet"},
	// «Списанные долги»
	"w_at": {"Когда списали", "Data spisania"}, "w_where": {"Где", "Miejsce"}, "w_sold": {"Когда продали", "Data sprzedaży"},
	"w_by": {"Кто списал", "Spisał"}, "w_id": {"ID списания", "ID spisania"}, "w_bar": {"бар", "bar"}, "w_kitchen": {"кухня", "kuchnia"},
	// «Подарено»
	"g_at": {"Когда", "Data"}, "g_kind": {"Что", "Rodzaj"}, "g_name": {"Название", "Nazwa"}, "g_price": {"Цена", "Cena sprzedaży"},
	"g_cost": {"Себестоимость", "Koszt zakupu"}, "g_reason": {"Причина", "Powód"}, "g_by": {"Кто подарил", "Przekazał"},
	"g_good": {"товар", "towar"}, "g_package": {"пакет времени", "pakiet czasu"},
	// «Склад на конец периода»
	"s_cat": {"Категория", "Kategoria"}, "s_lot": {"Партия", "Partia"}, "s_received": {"Дата прихода", "Data przyjęcia"},
	"s_doc": {"Документ поставки", "Dokument dostawy"}, "s_qty": {"Остаток, шт.", "Ilość, szt."},
	"s_price": {"Цена закупки за шт., брутто", "Cena zakupu za szt. brutto"}, "s_value": {"Стоимость, брутто", "Wartość brutto"},
	"lot_supply": {"поставка", "dostawa"}, "lot_initial": {"начальный остаток", "stan początkowy"},
	"lot_found": {"нашлось", "nadwyżka inwentaryzacyjna"},
	// «Предоплаты и долги на конец»
	"o_kind": {"Вид", "Rodzaj"}, "o_desc": {"Описание", "Opis"}, "o_since": {"С какого времени", "Od kiedy"},
	"o_until": {"Действует до", "Ważny do"}, "o_min": {"Минут", "Minuty"}, "o_sum": {"Сумма", "Kwota"},
	"ok_wallet": {"кошелёк", "portfel (przedpłata)"}, "ok_pack": {"пакет времени", "pakiet czasu (przedpłata)"},
	"ok_debt": {"долг гостя", "należność od klienta"}, "ok_gift": {"подарен", "nieodpłatny"},
	"ok_debt_v":  {"бар: %d шт. · кухня: %d шт.", "bar: %d szt. · kuchnia: %d szt."},
	"ot_wallets": {"Кошельки — итого", "Portfele — razem"}, "ot_packs": {"Пакеты — итого (подаренные — 0 zł)", "Pakiety — razem (nieodpłatne — 0 zł)"},
	"ot_prepaid": {"Предоплаты — итого", "Przedpłaty — razem"}, "ot_debts": {"Долги гостей — итого", "Należności od klientów — razem"},
}

func init() {
	for k, v := range sheetWords {
		exportWords[k] = v
	}
}

func nickOf(nick map[string]string, u *uuid.UUID) string {
	if u == nil {
		return ""
	}
	return nick[u.String()]
}

func cOptStamp(t *time.Time) xcell {
	if t == nil {
		return xcell{}
	}
	return cStamp(*t)
}

func yesNo(lang int, v bool) xcell {
	if v {
		return cText(tr(lang, "yes"))
	}
	return cText(tr(lang, "no"))
}

// ── «Отработано по дням» ───────────────────────────────────────────────────────────────────────

// spreadEstimate — оценка пакетов до журнала (Д7) по дням так, чтобы дни в сумме дали оценку периода до гроша
// (чистая, тест): минуты дня × средняя цена минуты, округление — наибольшими остатками.
func spreadEstimate(minutes []int64, avg float64, total int64) []int64 {
	out := make([]int64, len(minutes))
	type rest struct {
		i    int
		frac float64
	}
	rests := []rest{}
	var sum int64
	for i, m := range minutes {
		if m <= 0 {
			continue
		}
		x := float64(m) * avg
		f := math.Floor(x)
		out[i] = int64(f)
		sum += out[i]
		rests = append(rests, rest{i, x - f})
	}
	if len(rests) == 0 {
		return out
	}
	sort.SliceStable(rests, func(a, b int) bool { return rests[a].frac > rests[b].frac })
	for k := 0; sum < total; k++ { // не хватает грошей — дням с самыми большими остатками
		out[rests[k%len(rests)].i]++
		sum++
	}
	for k := len(rests) - 1; sum > total; k-- { // лишние (дрейф float) — у самых маленьких остатков
		if k < 0 {
			k = len(rests) - 1
		}
		if out[rests[k].i] > 0 {
			out[rests[k].i]--
			sum--
		}
	}
	return out
}

func sheetEarned(p period, reportHour int, now time.Time, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_earned"), file: []string{"otrabotano_po_dnyam", "przychod_wykonany"}[lang], table: true,
		head: []string{tr(lang, "h_day"), tr(lang, "e_wallet"), tr(lang, "e_packs"), tr(lang, "e_est"), tr(lang, "e_goods"),
			tr(lang, "e_kitchen"), tr(lang, "e_total"), tr(lang, "e_given_min"), tr(lang, "e_given")},
		widths: []int{12, 15, 15, 17, 13, 13, 14, 16, 14}}
	total := earnedAggIn(p.From, p.To)
	wins := bookWindows(p, reportHour, now)
	days := make([]earnedAgg, len(wins))
	mins := make([]int64, len(wins))
	for i, w := range wins {
		days[i] = earnedAggIn(w.From, w.To)
		mins[i] = days[i].EstMin
	}
	est := spreadEstimate(mins, total.Avg, total.Est)
	line := func(first xcell, e earnedAgg, est int64, bold bool) xrow {
		e.Est = est
		return xrow{bold: bold, cells: []xcell{first, cMoney(e.Wallet), cMoney(e.Packs), cMoney(e.Est), cMoney(e.Goods),
			cMoney(e.Kitchen), cMoney(e.total()), cInt(e.GivenMin), cMoney(e.Given)}}
	}
	for i, w := range wins {
		sh.rows = append(sh.rows, line(cDay(w.Day), days[i], est[i], false))
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, line(cText(tr(lang, "total_row")), total, total.Est, true))
	return sh
}

// ── «Расходы» ──────────────────────────────────────────────────────────────────────────────────

func counterpartiesByID() map[string]models.Counterparty {
	var list []models.Counterparty
	db.Find(&list)
	out := make(map[string]models.Counterparty, len(list))
	for _, cp := range list {
		out[cp.ID.String()] = cp
	}
	return out
}

// sheetExpenses — «Расходы»; form — форма учёта (З13-и6): у KPiR после категории — её колонка KPiR, у полной — счёт.
func sheetExpenses(p period, form, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_expenses"), file: []string{"rashody", "wydatki"}[lang], table: true,
		head: []string{tr(lang, "x_date"), tr(lang, "x_cat"), tr(lang, "x_purchase"), tr(lang, "x_party"), tr(lang, "x_nip"),
			tr(lang, "h_doc"), tr(lang, "h_sum"), "VAT", tr(lang, "h_net"), tr(lang, "h_method"), tr(lang, "x_paid"),
			tr(lang, "x_drawer"), tr(lang, "h_note"), tr(lang, "x_by"), tr(lang, "x_storno"), tr(lang, "x_storno_by"),
			tr(lang, "x_storno_why"), "ID"},
		widths: []int{12, 24, 10, 26, 13, 16, 14, 12, 14, 14, 9, 17, 36, 12, 17, 12, 28, 38}}
	_, formCol := expenseFormCol(form, models.ExpenseCategory{}, lang)
	if formCol { // колонка формы учёта — после категории
		name := map[int]string{formKpir: tr(lang, "pf_kpir_col"), formFull: tr(lang, "pf_account")}[form]
		sh.head = append(sh.head[:2], append([]string{name}, sh.head[2:]...)...)
		sh.widths = append(sh.widths[:2], append([]int{12}, sh.widths[2:]...)...)
	}
	var list []models.Expense
	expensesOfPeriod(p).Order("op_date, created_at, id").Find(&list)
	var cats []models.ExpenseCategory
	db.Find(&cats)
	catName, purchase, catOf := map[uuid.UUID]string{}, map[uuid.UUID]bool{}, map[uuid.UUID]models.ExpenseCategory{}
	for _, c := range cats {
		catName[c.ID], purchase[c.ID], catOf[c.ID] = c.Name, c.Purchase, c
	}
	cps := counterpartiesByID()
	ids := []string{}
	for i := range list {
		for _, u := range []*uuid.UUID{list[i].CreatedBy, list[i].StornoBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	nick := nicknamesByID(ids)
	var gross, vat int64
	for i := range list {
		e := &list[i]
		party, nip := xcell{}, xcell{}
		if e.CounterpartyID != nil {
			cp := cps[e.CounterpartyID.String()]
			party = cText(cp.Name)
			if cp.NIP != nil {
				nip = cText(*cp.NIP)
			}
		}
		storno := []xcell{{}, {}, {}}
		if e.StornoAt != nil {
			storno = []xcell{cStamp(e.StornoAt.In(p.FromDay.Location())), cText(nickOf(nick, e.StornoBy)), cText(e.StornoReason)}
		} else {
			gross, vat = gross+e.GrossGrosz, vat+e.VatGrosz
		}
		var drawer *time.Time
		if e.DrawerAt != nil {
			t := e.DrawerAt.In(p.FromDay.Location())
			drawer = &t
		}
		cells := []xcell{cDay(e.OpDate), cText(catName[e.CategoryID])}
		if fc, ok := expenseFormCol(form, catOf[e.CategoryID], lang); ok {
			cells = append(cells, fc)
		}
		cells = append(cells, yesNo(lang, purchase[e.CategoryID]), party, nip, cText(e.DocNo),
			cMoney(e.GrossGrosz), cMoney(e.VatGrosz), cMoney(e.GrossGrosz-e.VatGrosz), cText(trMethod(lang, e.Method)),
			yesNo(lang, e.Paid), cOptStamp(drawer), cText(e.Note), cText(nickOf(nick, e.CreatedBy)))
		cells = append(append(cells, storno...), cText(e.ID.String()))
		sh.rows = append(sh.rows, xrow{cells: cells})
	}
	sh.n = len(sh.rows)
	total := []xcell{cText(tr(lang, "x_total")), {}, {}, {}, {}, {}}
	if formCol {
		total = append(total, xcell{})
	}
	sh.rows = append(sh.rows, xrow{bold: true, cells: append(total, cMoney(gross), cMoney(vat), cMoney(gross-vat))})
	return sh
}

// ── «Закупки» ──────────────────────────────────────────────────────────────────────────────────

// supplyPiece — кусок строки поставки из ответа «Закупок» (pricingOut): позиция, штук, цена за штуку (nil — без цены).
type supplyPiece struct {
	Name  string
	Qty   int64
	Price *int64
}

// supplyPiecesOf — куски строк поставки по порядку строк (чистая, тест).
func supplyPiecesOf(row gin.H) []supplyPiece {
	out := []supplyPiece{}
	lines, _ := row["lines"].([]gin.H)
	for _, l := range lines {
		name, _ := l["name"].(string)
		pieces, _ := l["pieces"].([]gin.H)
		for _, pc := range pieces {
			sp := supplyPiece{Name: name}
			switch q := pc["qty"].(type) {
			case int:
				sp.Qty = int64(q)
			case int64:
				sp.Qty = q
			}
			if pr, ok := pc["price_pln"].(float64); ok {
				g := models.GroszFromPLN(pr)
				sp.Price = &g
			}
			if sp.Qty > 0 {
				out = append(out, sp)
			}
		}
	}
	return out
}

func sheetPurchases(p period, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_purchases"), file: []string{"zakupki", "zakupy"}[lang], table: true,
		head: []string{tr(lang, "p_date"), tr(lang, "p_supplier"), tr(lang, "h_doc"), tr(lang, "p_doc_date"), tr(lang, "p_good"),
			tr(lang, "h_qty"), tr(lang, "p_price"), tr(lang, "p_value"), tr(lang, "p_status"), tr(lang, "h_method"),
			tr(lang, "p_received_by"), tr(lang, "p_priced_by"), "ID"},
		widths: []int{17, 26, 16, 13, 30, 8, 14, 14, 13, 14, 12, 12, 38}}
	var list []models.Supply
	db.Where("status IN ? AND created_at >= ? AND created_at < ?", []string{models.SupplyWaiting, models.SupplyPriced}, p.From, p.To).
		Order("created_at, id").Find(&list)
	var total int64
	for i, row := range pricingOut(list) {
		s := list[i]
		str := func(k string) string { v, _ := row[k].(string); return v }
		docDate := xcell{}
		if s.DocDate != nil {
			docDate = cDay(*s.DocDate)
		}
		status, method := tr(lang, "st_waiting"), ""
		if s.Status == models.SupplyPriced {
			status = tr(lang, "st_priced")
		}
		if s.Method != nil {
			method = trMethod(lang, *s.Method)
		}
		for _, pc := range supplyPiecesOf(row) {
			price, value := xcell{}, xcell{}
			if pc.Price != nil {
				price, value = cMoney(*pc.Price), cMoney(*pc.Price*pc.Qty)
				if s.Status == models.SupplyPriced {
					total += *pc.Price * pc.Qty
				}
			}
			sh.rows = append(sh.rows, xrow{cells: []xcell{cStamp(s.CreatedAt.In(p.FromDay.Location())), cText(str("counterparty")),
				cText(s.DocNo), docDate, cText(pc.Name), cInt(pc.Qty), price, value, cText(status), cText(method),
				cText(str("received_by")), cText(str("priced_by")), cText(s.ID.String())}})
		}
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(tr(lang, "p_total")), {}, {}, {}, {}, {}, {}, cMoney(total)}})
	return sh
}

// ── «Касса» и «Сдачи касс» ─────────────────────────────────────────────────────────────────────

// Номера документов кассы — настройка acc_kpkw (З13-и5; уточните у своего бухгалтера).
const (
	kpkwMonth = 0 // с начала месяца: KP 1/10/2026 — по умолчанию
	kpkwYear  = 1 // с начала года: KP 1/2026
	kpkwNone  = 2 // без номеров
)

// kpkwFrom — с какого дня считать номера, чтобы у первого дня периода был свой номер по порядку (чистая, тест).
func kpkwFrom(fromDay time.Time, scheme int) time.Time {
	switch scheme {
	case kpkwMonth:
		return time.Date(fromDay.Year(), fromDay.Month(), 1, 0, 0, 0, 0, fromDay.Location())
	case kpkwYear:
		return time.Date(fromDay.Year(), 1, 1, 0, 0, 0, 0, fromDay.Location())
	}
	return fromDay
}

// kpkwReset — с этого дня номера начинаются заново: первое число месяца (или 1 января).
func kpkwReset(day time.Time, scheme int) bool {
	return day.Day() == 1 && (scheme == kpkwMonth || day.Month() == time.January)
}

// kpkwNo — номер документа (чистая, тест): «KP 3/10/2026», «KW 1/2026»; без номеров — пусто.
func kpkwNo(kind string, n int, day time.Time, scheme int) string {
	switch scheme {
	case kpkwMonth:
		return fmt.Sprintf("%s %d/%02d/%d", kind, n, int(day.Month()), day.Year())
	case kpkwYear:
		return fmt.Sprintf("%s %d/%d", kind, n, day.Year())
	}
	return ""
}

// cashDoc — документ кассы: KP — пришло, KW — ушло; Amt > 0.
type cashDoc struct {
	Kind string
	Text string
	Who  string
	Amt  int64
	At   time.Time
	ID   string
	ord  int
}

func refundWhat(r returnRow, lang int, nick map[string]string) string {
	parts := []string{tr(lang, "rk_"+r.Kind)}
	if r.Name != "" {
		w := "«" + r.Name + "»"
		if r.Kind == "sale" && r.Qty > 1 {
			w += fmt.Sprintf(" ×%d", r.Qty)
		}
		parts = append(parts, w)
	}
	text := strings.Join(parts, " ")
	if g := nickOf(nick, r.UserID); g != "" {
		text += " · " + g
	}
	return text
}

// cashDocs — документы дня кассовой книги: наличные за день по видам — по документу KP (как дневной отчёт кассы),
// возвраты наличными, расходы из кассы, возвраты владельцу, инкассации и коррекции — каждый своим, по времени;
// расхождение пересчётов дня — последним. В сумме — ровно движение дня и расхождение: остаток на начало + KP − KW =
// остаток на конец.
func cashDocs(d bookDay, b cashBook, lang int) []cashDoc {
	docs := []cashDoc{}
	for i, x := range []struct {
		key string
		g   int64
	}{{"c_deposits", d.Flow.Deposits}, {"c_goods", d.Flow.Goods}, {"c_packs", d.Flow.Packs}} {
		if x.g != 0 {
			docs = append(docs, cashDoc{Kind: "KP", Text: tr(lang, x.key), Amt: x.g, At: d.From, ord: i})
		}
	}
	ind := []cashDoc{}
	for _, r := range d.Refunds {
		ind = append(ind, cashDoc{Kind: "KW", Text: fmt.Sprintf(tr(lang, "c_refund"), refundWhat(r, lang, b.Nick)),
			Who: nickOf(b.Nick, r.VoidedBy), Amt: r.RefundGrosz, At: r.VoidedAt, ID: r.ID.String(), ord: 1})
	}
	for _, e := range d.Tills {
		text := b.Cats[e.CategoryID.String()]
		if e.Note != "" {
			text += ": " + e.Note
		}
		if e.DocNo != "" {
			text += " · " + e.DocNo
		}
		ind = append(ind, cashDoc{Kind: "KW", Text: fmt.Sprintf(tr(lang, "c_expense"), text), Who: nickOf(b.Nick, e.CreatedBy),
			Amt: e.GrossGrosz, At: *e.DrawerAt, ID: e.ID.String(), ord: 2})
	}
	for _, r := range d.Owners {
		if r.VoidedAt != nil {
			continue
		}
		text := tr(lang, "c_owner")
		if r.Note != "" {
			text += ": " + r.Note
		}
		ind = append(ind, cashDoc{Kind: "KW", Text: text, Who: nickOf(b.Nick, r.CreatedBy), Amt: r.AmountGrosz, At: r.CreatedAt, ID: r.ID.String(), ord: 3})
	}
	for _, e := range d.Events {
		if e.VoidedAt != nil || e.AmountGrosz == 0 {
			continue
		}
		doc := cashDoc{Who: nickOf(b.Nick, e.CreatedBy), At: e.CreatedAt, ID: e.ID.String(), ord: 4}
		if e.Kind == models.CashEventCollect {
			doc.Kind, doc.Amt, doc.Text = "KW", e.AmountGrosz, tr(lang, "c_collect")
			if e.Dest != nil && (*e.Dest == "bank" || *e.Dest == "safe") {
				doc.Text += " " + tr(lang, "n_dest_"+*e.Dest)
			}
		} else {
			doc.Kind, doc.Amt = "KP", e.AmountGrosz
			if e.AmountGrosz < 0 {
				doc.Kind, doc.Amt = "KW", -e.AmountGrosz
			}
			stated := int64(0)
			if e.StatedGrosz != nil {
				stated = *e.StatedGrosz
			}
			doc.Text = fmt.Sprintf(tr(lang, "c_correct"), moneyText(stated, ','))
		}
		if e.Note != "" {
			doc.Text += " · " + e.Note
		}
		ind = append(ind, doc)
	}
	sort.SliceStable(ind, func(i, j int) bool {
		if !ind[i].At.Equal(ind[j].At) {
			return ind[i].At.Before(ind[j].At)
		}
		return ind[i].ord < ind[j].ord
	})
	docs = append(docs, ind...)
	switch {
	case d.Diff > 0:
		docs = append(docs, cashDoc{Kind: "KP", Text: tr(lang, "c_surplus"), Amt: d.Diff, At: d.To})
	case d.Diff < 0:
		docs = append(docs, cashDoc{Kind: "KW", Text: tr(lang, "c_short"), Amt: -d.Diff, At: d.To})
	}
	return docs
}

// kpkwCounts — сколько KP и KW уже было с начала нумерации до первого дня периода (дни книги до него).
func kpkwCounts(days []bookDay, scheme int) (kp, kw int) {
	for _, d := range days {
		if kpkwReset(d.Day, scheme) {
			kp, kw = 0, 0
		}
		for _, doc := range cashDocs(d, cashBook{Nick: map[string]string{}, Cats: map[string]string{}}, 0) {
			if doc.Kind == "KP" {
				kp++
			} else {
				kw++
			}
		}
	}
	return kp, kw
}

func sheetCash(p period, reportHour int, now time.Time, b cashBook, scheme, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_cash"), file: []string{"kassa", "raport_kasowy"}[lang], table: true,
		head: []string{tr(lang, "h_day"), tr(lang, "c_no"), tr(lang, "c_kind"), tr(lang, "c_text"), tr(lang, "h_who"),
			tr(lang, "c_in"), tr(lang, "c_out"), tr(lang, "c_bal"), "ID"},
		widths: []int{12, 16, 9, 56, 12, 13, 13, 14, 38}}
	kp, kw := 0, 0
	if scheme != kpkwNone {
		if start := kpkwFrom(p.FromDay, scheme); start.Before(p.FromDay) {
			pre := cashBookIn(makePeriod(start, p.FromDay.AddDate(0, 0, -1), reportHour, ""), reportHour, now)
			kp, kw = kpkwCounts(pre.Days, scheme)
		}
	}
	for _, d := range b.Days {
		if kpkwReset(d.Day, scheme) {
			kp, kw = 0, 0
		}
		var bal *int64
		openText, openBal := tr(lang, "c_open"), xcell{}
		if d.Opening != nil {
			v := *d.Opening
			bal, openBal = &v, cMoney(v)
		} else {
			openText += ": " + tr(lang, "c_none")
		}
		sh.rows = append(sh.rows, xrow{cells: []xcell{cDay(d.Day), {}, {}, cText(openText), {}, {}, {}, openBal}})
		if d.Start != nil {
			at := d.Start.CreatedAt.In(p.FromDay.Location()).Format("15:04")
			sh.rows = append(sh.rows, xrow{cells: []xcell{cDay(d.Day), {}, {}, cText(fmt.Sprintf(tr(lang, "c_base"),
				b.Nick[d.Start.UserID.String()], at)), cText(b.Nick[d.Start.UserID.String()]), {}, {}, cMoney(d.Base)}})
		}
		for _, doc := range cashDocs(d, b, lang) {
			n := 0
			in, out := xcell{}, xcell{}
			if doc.Kind == "KP" {
				kp++
				n, in = kp, cMoney(doc.Amt)
			} else {
				kw++
				n, out = kw, cMoney(doc.Amt)
			}
			balCell := xcell{}
			if bal != nil {
				if doc.Kind == "KP" {
					*bal += doc.Amt
				} else {
					*bal -= doc.Amt
				}
				balCell = cMoney(*bal)
			}
			sh.rows = append(sh.rows, xrow{cells: []xcell{cDay(d.Day), cText(kpkwNo(doc.Kind, n, d.Day, scheme)), cText(doc.Kind),
				cText(doc.Text), cText(doc.Who), in, out, balCell, cText(doc.ID)}})
			sh.n++
		}
		closeText, closeBal := tr(lang, "c_close"), xcell{}
		if d.Closing != nil {
			closeBal = cMoney(*d.Closing)
		} else {
			closeText += ": " + tr(lang, "c_none")
		}
		sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cDay(d.Day), {}, {}, cText(closeText), {}, {}, {}, closeBal}})
	}
	return sh
}

func sheetCloseouts(p period, b cashBook, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_closeouts"), file: []string{"sdachi_kass", "rozliczenia_zmian"}[lang], table: true,
		head: []string{tr(lang, "h_day"), tr(lang, "h_at"), tr(lang, "h_who"), tr(lang, "z_open"), tr(lang, "z_in"), tr(lang, "z_out"),
			tr(lang, "z_adjust"), tr(lang, "z_expected"), tr(lang, "z_counted"), tr(lang, "z_diff"), tr(lang, "z_explained"),
			tr(lang, "z_deposits"), tr(lang, "z_goods"), tr(lang, "z_packs"), tr(lang, "z_refunds"), tr(lang, "z_collect"),
			tr(lang, "z_expenses"), tr(lang, "z_owner"), tr(lang, "z_skipped"), tr(lang, "z_baseline"), tr(lang, "h_note"), "ID"},
		widths: []int{12, 17, 12, 13, 12, 12, 11, 13, 13, 12, 13, 12, 12, 11, 11, 12, 12, 12, 10, 10, 30, 38}}
	var diff, explained int64
	for _, d := range b.Days {
		for _, co := range d.Closeouts {
			opt := func(g *int64) xcell {
				if g == nil {
					return xcell{}
				}
				return cMoney(*g)
			}
			drawerModel := co.OpeningGrosz != nil
			mv := func(g int64) xcell {
				if !drawerModel {
					return xcell{}
				}
				return cMoney(g)
			}
			ex := xcell{}
			if x := b.Explained[co.ID]; x > 0 {
				ex, explained = cMoney(x), explained+x
			}
			if co.DiffGrosz != nil {
				diff += *co.DiffGrosz
			}
			sh.rows = append(sh.rows, xrow{cells: []xcell{cDay(d.Day), cStamp(co.CreatedAt.In(p.FromDay.Location())),
				cText(b.Nick[co.UserID.String()]), opt(co.OpeningGrosz), mv(co.InGrosz), mv(co.OutGrosz), mv(co.AdjustGrosz),
				cMoney(co.ExpectedGrosz), opt(co.CountedGrosz), opt(co.DiffGrosz), ex, cMoney(co.DepositsGrosz),
				cMoney(co.GoodsGrosz), cMoney(co.PacksGrosz), cMoney(co.RefundsGrosz),
				mv(co.OutGrosz - co.RefundsGrosz - co.ExpensesGrosz - co.OwnerGrosz), mv(co.ExpensesGrosz), mv(co.OwnerGrosz),
				yesNo(lang, co.Skipped), yesNo(lang, co.Baseline), cText(co.Note), cText(co.ID.String())}})
		}
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(tr(lang, "total_row")), {}, {}, {}, {}, {}, {}, {}, {},
		cMoney(diff), cMoney(explained)}})
	return sh
}

// ── «Возвраты», «Списанные долги», «Подарено» ─────────────────────────────────────────────────

func sheetReturns(p period, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_returns"), file: []string{"vozvraty", "zwroty"}[lang], table: true,
		head: []string{tr(lang, "r_at"), tr(lang, "h_day"), tr(lang, "r_kind"), tr(lang, "r_what"), tr(lang, "h_qty"),
			tr(lang, "r_guest"), tr(lang, "r_paid"), tr(lang, "r_paid_method"), tr(lang, "r_paid_at"), tr(lang, "r_refund"),
			tr(lang, "r_refund_method"), tr(lang, "r_by"), tr(lang, "r_earlier"), tr(lang, "r_receipt"), "ID"},
		widths: []int{17, 12, 13, 28, 7, 16, 12, 13, 17, 12, 14, 12, 12, 38, 38}}
	rows := returnsIn(db, p.From, p.To) // свежие сверху — лист по порядку
	nick := nicknamesByID(returnUsers(rows))
	loc := p.FromDay.Location()
	var paid, refund int64
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		receipt := xcell{}
		if r.ReceiptID != nil {
			receipt = cText(r.ReceiptID.String())
		}
		at := r.VoidedAt.In(loc)
		paid, refund = paid+r.PaidGrosz, refund+r.RefundGrosz
		sh.rows = append(sh.rows, xrow{cells: []xcell{cStamp(at), cDay(clubDayOf(at, p.From.Hour())), cText(tr(lang, "rk_"+r.Kind)),
			cText(r.Name), cInt(int64(r.Qty)), cText(nickOf(nick, r.UserID)), cMoney(r.PaidGrosz), cText(trMethod(lang, r.PaidMethod)),
			cStamp(r.PaidAt.In(loc)), cMoney(r.RefundGrosz), cText(trMethod(lang, r.Method)), cText(nickOf(nick, r.VoidedBy)),
			yesNo(lang, r.PaidAt.Before(p.From)), receipt, cText(r.ID.String())}})
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(tr(lang, "total_row")), {}, {}, {}, {}, {}, cMoney(paid), {}, {},
		cMoney(refund)}})
	return sh
}

// writeOffItem — что вошло в списание: продажа бара или заказ кухни.
type writeOffItem struct {
	WriteOffID uuid.UUID
	Kind       string // bar | kitchen
	ID         uuid.UUID
	Name       string
	Qty        int64
	Grosz      int64
	SoldAt     time.Time
}

func sheetWriteOffs(p period, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_writeoffs"), file: []string{"spisannye_dolgi", "naleznosci_spisane"}[lang], table: true,
		head: []string{tr(lang, "w_at"), tr(lang, "r_guest"), tr(lang, "w_where"), tr(lang, "r_what"), tr(lang, "h_qty"),
			tr(lang, "h_sum"), tr(lang, "w_sold"), tr(lang, "w_by"), tr(lang, "w_id"), "ID"},
		widths: []int{17, 16, 9, 28, 7, 13, 17, 12, 38, 38}}
	var wos []models.DebtWriteOff // как «Потери → долги»: списания периода, не отменённые
	db.Where("created_at >= ? AND created_at < ? AND cancelled_at IS NULL", p.From, p.To).Order("created_at, id").Find(&wos)
	var total int64
	if len(wos) > 0 {
		ids, users := make([]uuid.UUID, 0, len(wos)), []string{}
		for _, w := range wos {
			ids = append(ids, w.ID)
			total += w.TotalGrosz
			users = append(users, w.UserID.String())
			if w.CreatedBy != nil {
				users = append(users, w.CreatedBy.String())
			}
		}
		var items []writeOffItem
		db.Raw(`SELECT write_off_id, 'bar' AS kind, id, name, qty, ROUND(total_pln * 100)::bigint AS grosz, created_at AS sold_at
		          FROM sales WHERE write_off_id IN ?
		        UNION ALL
		        SELECT write_off_id, 'kitchen', id, name, qty, ROUND(total_pln * 100)::bigint, status_at
		          FROM kitchen_orders WHERE write_off_id IN ?
		        ORDER BY sold_at, id`, ids, ids).Scan(&items)
		nick := nicknamesByID(users)
		byWO := map[uuid.UUID][]writeOffItem{}
		for _, it := range items {
			byWO[it.WriteOffID] = append(byWO[it.WriteOffID], it)
		}
		loc := p.FromDay.Location()
		for _, w := range wos {
			for _, it := range byWO[w.ID] {
				sh.rows = append(sh.rows, xrow{cells: []xcell{cStamp(w.CreatedAt.In(loc)), cText(nick[w.UserID.String()]),
					cText(tr(lang, "w_"+it.Kind)), cText(it.Name), cInt(it.Qty), cMoney(it.Grosz), cStamp(it.SoldAt.In(loc)),
					cText(nickOf(nick, w.CreatedBy)), cText(w.ID.String()), cText(it.ID.String())}})
			}
		}
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(tr(lang, "total_row")), {}, {}, {}, {}, cMoney(total)}})
	return sh
}

func sheetGifts(p period, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_gifts"), file: []string{"podareno", "przekazane_nieodplatnie"}[lang], table: true,
		head: []string{tr(lang, "g_at"), tr(lang, "g_kind"), tr(lang, "g_name"), tr(lang, "h_qty"), tr(lang, "r_guest"),
			tr(lang, "g_price"), tr(lang, "g_cost"), tr(lang, "g_reason"), tr(lang, "g_by"), "ID"},
		widths: []int{17, 14, 28, 7, 16, 12, 13, 40, 12, 38}}
	var rows []giftRow // как «Подарено»: по моменту подарка, отменённые не считаются
	db.Raw(giftsSQL, p.From, p.To, p.From, p.To).Scan(&rows)
	cost := map[string]goodsEcon{} // себестоимость подаренного товара — по партиям, тем же счётом, что «Подарено»
	var econ []goodsEcon
	db.Raw(fmt.Sprintf(goodsEconSQL, "s.id::text", "", "method = '"+models.SaleMethodComp+"'"), map[string]any{"from": p.From, "to": p.To}).Scan(&econ)
	for _, x := range econ {
		cost[x.Key] = x
	}
	ids := []string{}
	for _, r := range rows {
		for _, u := range []*uuid.UUID{r.UserID, r.CreatedBy} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	nick := nicknamesByID(ids)
	loc := p.FromDay.Location()
	var price, costSum int64
	for i := len(rows) - 1; i >= 0; i-- {
		r := rows[i]
		kind, c := tr(lang, "g_package"), xcell{}
		if r.Kind == "sale" {
			kind = tr(lang, "g_good")
			if e, ok := cost[r.ID.String()]; ok && e.Costed > 0 {
				c, costSum = cMoney(e.Cost), costSum+e.Cost
			}
		}
		price += r.Grosz
		sh.rows = append(sh.rows, xrow{cells: []xcell{cStamp(r.CreatedAt.In(loc)), cText(kind), cText(r.Name), cInt(int64(r.Qty)),
			cText(nickOf(nick, r.UserID)), cMoney(r.Grosz), c, cText(r.Reason), cText(nickOf(nick, r.CreatedBy)), cText(r.ID.String())}})
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(tr(lang, "total_row")), {}, {}, {}, {}, cMoney(price), cMoney(costSum)}})
	return sh
}

// ── на конец периода: склад, кошельки, пакеты, долги ──────────────────────────────────────────

// periodEnd — «на конец периода»: конец последних клубных суток, у периода, который ещё идёт, — сейчас.
func periodEnd(p period, now time.Time) time.Time {
	if p.To.After(now) {
		return now
	}
	return p.To
}

// stockLotAt — партия с остатком на момент t.
type stockLotAt struct {
	ID         uuid.UUID
	GoodID     uuid.UUID
	Good       string
	Category   string
	Kind       string
	SupplyID   *uuid.UUID
	DocNo      string
	Supplier   string
	ReceivedAt time.Time
	Price      *int64
	Qty        int64
}

// stockAtSQL — партии на момент @t: нынешний остаток минус движения партии с этого момента (lot_moves по моменту
// движения склада); сейчас — ровно остатки «Склада» (stockValue).
const stockAtSQL = `
SELECT l.id, l.good_id, g.name AS good, g.category, l.kind, l.supply_id, COALESCE(s.doc_no, '') AS doc_no,
       COALESCE(cp.name, '') AS supplier, l.received_at, l.price_grosz AS price, (l.qty_left - COALESCE(mv.q, 0))::bigint AS qty
  FROM stock_lots l JOIN goods g ON g.id = l.good_id
  LEFT JOIN supplies s ON s.id = l.supply_id LEFT JOIN counterparties cp ON cp.id = s.counterparty_id
  LEFT JOIN (SELECT lm.lot_id, SUM(lm.qty) AS q FROM lot_moves lm JOIN stock_moves sm ON sm.id = lm.stock_move_id
              WHERE sm.created_at >= @t GROUP BY lm.lot_id) mv ON mv.lot_id = l.id
 WHERE l.qty_left - COALESCE(mv.q, 0) > 0
 ORDER BY lower(g.name), l.received_at, l.part, l.id`

func sheetStock(t time.Time, lang int) (xsheet, int64) {
	sh := xsheet{name: tr(lang, "sheet_stock"), file: []string{"sklad_na_konec", "magazyn_na_koniec"}[lang], table: true,
		head: []string{tr(lang, "p_good"), tr(lang, "s_cat"), tr(lang, "s_lot"), tr(lang, "s_received"), tr(lang, "s_doc"),
			tr(lang, "p_supplier"), tr(lang, "s_qty"), tr(lang, "s_price"), tr(lang, "s_value"), "ID"},
		widths: []int{30, 16, 22, 17, 16, 24, 11, 15, 14, 38}}
	var lots []stockLotAt
	db.Raw(stockAtSQL, map[string]any{"t": t}).Scan(&lots)
	var units, value, noCost int64
	for _, l := range lots {
		price, val := xcell{}, xcell{}
		if l.Price != nil {
			price, val = cMoney(*l.Price), cMoney(*l.Price*l.Qty)
			value += *l.Price * l.Qty
		} else {
			noCost += l.Qty
		}
		units += l.Qty
		sh.rows = append(sh.rows, xrow{cells: []xcell{cText(l.Good), cText(l.Category), cText(tr(lang, "lot_"+l.Kind)),
			cStamp(l.ReceivedAt.In(t.Location())), cText(l.DocNo), cText(l.Supplier), cInt(l.Qty), price, val, cText(l.ID.String())}})
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, xrow{bold: true, cells: []xcell{cText(tr(lang, "total_row")), {}, {}, {}, {}, {}, cInt(units), {}, cMoney(value)}})
	return sh, noCost
}

// walletsAtSQL — кошельки на момент @t: нынешний остаток минус операции кошелька с этого момента; сейчас — ровно
// «Кошельки гостей» (walletsNow).
const walletsAtSQL = `
SELECT u.id AS user_id, (u.wallet_grosz - COALESCE(w.s, 0))::bigint AS grosz
  FROM users u
  LEFT JOIN (SELECT user_id, SUM(amount_grosz) AS s FROM wallet_transactions WHERE created_at >= @t GROUP BY user_id) w ON w.user_id = u.id
 WHERE u.wallet_grosz - COALESCE(w.s, 0) <> 0
 ORDER BY 2 DESC, 1`

// packsAtSQL — неотыгранные пакеты на момент @t: выданы до него, не отменены и не просрочены к нему; минут —
// нынешний остаток плюс расход по журналу с этого момента (у отменённого позже — выдано минус расход до момента);
// злотых — доля цены, у подаренного — 0. Сейчас — ровно «Долг временем» (packsOwedNow).
const packsAtSQL = `
SELECT p.id, p.user_id, p.name, p.zone_name, p.method = 'comp' AS gift, p.created_at, p.expires_at, m.minutes,
       (CASE WHEN p.method = 'comp' THEN 0 ELSE ROUND(p.price_pln * 100 * m.minutes / p.minutes_total) END)::bigint AS grosz
  FROM user_packages p
  CROSS JOIN LATERAL (SELECT (CASE WHEN p.voided_at IS NULL
         THEN p.minutes_left + COALESCE((SELECT SUM(s.minutes) FROM pack_spends s WHERE s.user_package_id = p.id AND s.created_at >= @t), 0)
         ELSE p.minutes_total - COALESCE((SELECT SUM(s.minutes) FROM pack_spends s WHERE s.user_package_id = p.id AND s.created_at < @t), 0)
         END)::bigint AS minutes) m
 WHERE p.created_at < @t AND (p.voided_at IS NULL OR p.voided_at >= @t) AND (p.expires_at IS NULL OR p.expires_at > @t)
   AND m.minutes > 0
 ORDER BY p.created_at, p.id`

// receivablesAtSQL — долги гостей на момент @t: бар в долг и выданная кухня, не оплаченные, не отменённые и не
// списанные к нему; сейчас — ровно «Дебиторка сейчас» (receivablesSQL).
const receivablesAtSQL = `
SELECT user_id,
       COUNT(*) FILTER (WHERE kind = 'kitchen') AS kitchen_count, COALESCE(SUM(g) FILTER (WHERE kind = 'kitchen'), 0)::bigint AS kitchen_grosz,
       COUNT(*) FILTER (WHERE kind = 'bar') AS bar_count, COALESCE(SUM(g) FILTER (WHERE kind = 'bar'), 0)::bigint AS bar_grosz,
       MIN(created_at) AS since
  FROM (SELECT s.user_id, 'bar' AS kind, ROUND(s.total_pln * 100)::bigint AS g, s.created_at
          FROM sales s LEFT JOIN debt_write_offs w ON w.id = s.write_off_id
         WHERE s.user_id IS NOT NULL AND s.created_at < @t AND (s.paid_at IS NULL OR s.paid_at >= @t)
           AND (s.voided_at IS NULL OR s.voided_at >= @t) AND (s.write_off_id IS NULL OR w.created_at >= @t)
        UNION ALL
        SELECT o.user_id, 'kitchen', ROUND(o.total_pln * 100)::bigint, o.created_at
          FROM kitchen_orders o LEFT JOIN debt_write_offs w ON w.id = o.write_off_id
         WHERE o.status = 'done' AND o.status_at < @t AND (o.paid_at IS NULL OR o.paid_at >= @t)
           AND (o.write_off_id IS NULL OR w.created_at >= @t)) d
 GROUP BY user_id
 ORDER BY MIN(created_at), user_id`

type walletAt struct {
	UserID uuid.UUID
	Grosz  int64
}

type packAt struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Name      string
	ZoneName  string
	Gift      bool
	CreatedAt time.Time
	ExpiresAt *time.Time
	Minutes   int64
	Grosz     int64
}

// owedAt — предоплаты и долги на момент t: строки и итоги.
type owedAt struct {
	Wallets []walletAt
	Packs   []packAt
	Debts   []receivable
	Nick    map[string]string
}

func owedAtMoment(t time.Time) owedAt {
	var o owedAt
	args := map[string]any{"t": t}
	db.Raw(walletsAtSQL, args).Scan(&o.Wallets)
	db.Raw(packsAtSQL, args).Scan(&o.Packs)
	db.Raw(receivablesAtSQL, args).Scan(&o.Debts)
	ids := []string{}
	for _, w := range o.Wallets {
		ids = append(ids, w.UserID.String())
	}
	for _, p := range o.Packs {
		ids = append(ids, p.UserID.String())
	}
	for _, d := range o.Debts {
		ids = append(ids, d.UserID.String())
	}
	o.Nick = nicknamesByID(ids)
	return o
}

// sums — итоги (чистая, тест): кошельки, пакеты (злотые и минуты), долги.
func (o owedAt) sums() (wallets, packs, packMin, debts int64) {
	for _, w := range o.Wallets {
		wallets += w.Grosz
	}
	for _, p := range o.Packs {
		packs += p.Grosz
		packMin += p.Minutes
	}
	for _, d := range o.Debts {
		debts += d.total()
	}
	return
}

func sheetOwed(o owedAt, t time.Time, lang int) xsheet {
	sh := xsheet{name: tr(lang, "sheet_owed"), file: []string{"predoplaty_i_dolgi", "przedplaty_i_naleznosci"}[lang], table: true,
		head: []string{tr(lang, "o_kind"), tr(lang, "r_guest"), tr(lang, "o_desc"), tr(lang, "o_since"), tr(lang, "o_until"),
			tr(lang, "o_min"), tr(lang, "o_sum"), "ID"},
		widths: []int{26, 16, 40, 17, 17, 9, 13, 38}}
	loc := t.Location()
	for _, w := range o.Wallets {
		sh.rows = append(sh.rows, xrow{cells: []xcell{cText(tr(lang, "ok_wallet")), cText(o.Nick[w.UserID.String()]), {}, {}, {}, {},
			cMoney(w.Grosz), cText(w.UserID.String())}})
	}
	for _, p := range o.Packs {
		desc := p.Name
		if p.ZoneName != "" {
			desc += " · " + p.ZoneName
		}
		if p.Gift {
			desc += " · " + tr(lang, "ok_gift")
		}
		until := xcell{}
		if p.ExpiresAt != nil {
			until = cStamp(p.ExpiresAt.In(loc))
		}
		sh.rows = append(sh.rows, xrow{cells: []xcell{cText(tr(lang, "ok_pack")), cText(o.Nick[p.UserID.String()]), cText(desc),
			cStamp(p.CreatedAt.In(loc)), until, cInt(p.Minutes), cMoney(p.Grosz), cText(p.ID.String())}})
	}
	for _, d := range o.Debts {
		sh.rows = append(sh.rows, xrow{cells: []xcell{cText(tr(lang, "ok_debt")), cText(o.Nick[d.UserID.String()]),
			cText(fmt.Sprintf(tr(lang, "ok_debt_v"), d.BarCount, d.KitchenCount)), cStamp(d.Since.In(loc)), {}, {},
			cMoney(d.total()), cText(d.UserID.String())}})
	}
	sh.n = len(sh.rows)
	wallets, packs, packMin, debts := o.sums()
	total := func(key string, mins xcell, g int64) xrow {
		return xrow{bold: true, cells: []xcell{cText(tr(lang, key)), {}, {}, {}, {}, mins, cMoney(g)}}
	}
	sh.rows = append(sh.rows, total("ot_wallets", xcell{}, wallets), total("ot_packs", cInt(packMin), packs),
		total("ot_prepaid", xcell{}, wallets+packs), total("ot_debts", xcell{}, debts))
	return sh
}
