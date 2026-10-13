package main

// З13-и4 «Выгрузка: движок и «Все операции»» (OWNER.md, трек З; №75; Д8; решения основателя 04.10, З-Р37).
//
// GET /admin/export?month=ГГГГ-ММ | from&to | preset & format=xlsx|csv & lang=pl|ru — выгрузка бухгалтеру за период
// (владелец). Листы:
//
//	«О выгрузке» — клуб, период, когда и кем сделана, формат, настройки VAT, итоги продаж, листы, как читать: бухгалтер
//	видит, как посчитано;
//	«Все операции» — универсальный журнал: каждая денежная операция строкой — клубный день и время, операция, способ,
//	гость или контрагент, документ, описание, количество, сумма брутто со знаком, облагается ли VAT сейчас (по
//	настройкам «Бухгалтерии» — те же правила, что «Продажи по ставкам VAT»), ставка, нетто, VAT, движение ящика, кто,
//	примечание, ID; любой бухгалтер разберёт его в Excel или загрузит к себе с сопоставлением колонок;
//	«Продажи по дням и ставкам» — «Продажи по ставкам VAT» (vat.go) по дням: брутто, нетто и VAT по ставкам, итог и
//	по способам оплаты, строка «Итого».
//
// З13-и6: сразу после «О выгрузке» — листы профиля учёта (export_profiles.go), за ними — эти два и листы З13-и5.
//
// Одна цифра везде: журнал считается теми же правилами, что экраны — доходы и возвраты как «Прибыль» (З12-и3),
// облагаемое — как «Продажи по ставкам VAT» (З13-и3), движение ящика — как кассовая книга (drawer.go), расходы — по дню
// операции без сторно, как «Прибыль» и «Расходы». Файл — workbook.go.

import (
	"bytes"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/pastbishepsov/1448/backend/internal/models"
)

// ── слова: [русский, польский] ─────────────────────────────────────────────────────────────────

var exportWords = map[string][2]string{
	// листы
	"sheet_about": {"О выгрузке", "O eksporcie"},
	"sheet_ops":   {"Все операции", "Wszystkie operacje"},
	"sheet_vat":   {"Продажи по дням и ставкам", "Sprzedaż wg dni i stawek"},
	// шапка журнала
	"h_n": {"№", "Lp."}, "h_day": {"Клубный день", "Dzień"}, "h_at": {"Время", "Data i godzina"},
	"h_kind": {"Операция", "Operacja"}, "h_method": {"Способ", "Forma płatności"},
	"h_party": {"Гость или контрагент", "Klient lub kontrahent"}, "h_doc": {"Документ", "Dokument"},
	"h_desc": {"Описание", "Opis"}, "h_qty": {"Кол-во", "Ilość"}, "h_sum": {"Сумма брутто", "Kwota brutto"},
	"h_taxed": {"Облагается VAT", "Sprzedaż opodatkowana"}, "h_rate": {"Ставка VAT", "Stawka VAT"},
	"h_net": {"Нетто", "Netto"}, "h_vat": {"VAT", "VAT"}, "h_drawer": {"Касса", "Kasa"}, "h_who": {"Кто", "Kto"},
	"h_note": {"Примечание", "Uwagi"}, "h_id": {"ID операции", "ID operacji"},
	// шапка «О выгрузке» и продаж по дням
	"h_item": {"Что", "Pozycja"}, "h_value": {"Значение", "Wartość"},
	"h_gross": {"Брутто", "Brutto"}, "h_total": {"Итого", "Razem"}, "h_pay": {"Брутто: ", "Brutto: "},
	// операции
	"k_deposit": {"Пополнение кошелька", "Doładowanie portfela"}, "k_deposit_void": {"Отмена пополнения", "Anulowanie doładowania"},
	"k_sale": {"Продажа", "Sprzedaż"}, "k_kitchen": {"Кухня", "Kuchnia"}, "k_sale_void": {"Отмена продажи", "Anulowanie sprzedaży"},
	"k_pack": {"Пакет времени", "Pakiet czasu"}, "k_pack_void": {"Отмена пакета", "Anulowanie pakietu"},
	"k_time": {"Время с кошелька", "Czas gry z portfela"}, "k_gift": {"Подарок товара", "Nieodpłatne przekazanie towaru"},
	"k_expense": {"Расход", "Wydatek"}, "k_purchase": {"Закупка товара", "Zakup towaru"},
	"k_collect": {"Инкассация", "Wypłata z kasy (inkaso)"}, "k_correct": {"Коррекция ящика", "Korekta stanu kasy"},
	"k_owner_repay": {"Возврат владельцу", "Zwrot właścicielowi"},
	// способы
	"m_cash": {"наличные", "gotówka"}, "m_card": {"карта", "karta"}, "m_blik": {"BLIK", "BLIK"},
	"m_wallet": {"кошелёк", "portfel"}, "m_comp": {"подарено", "gratis"}, "m_later": {"в долг", "na kredyt"},
	"m_till": {"из кассы", "z kasy"}, "m_bank": {"банк", "bank"}, "m_owner": {"личные владельца", "środki właściciela"},
	"m_safe": {"в сейф", "do sejfu"}, "m_none": {"не возвращали", "nie zwrócono"},
	// да/нет
	"yes": {"да", "tak"}, "no": {"нет", "nie"},
	// примечания
	"n_paid":        {"оплачено: %s", "zapłacono: %s"},
	"n_unpaid":      {"не оплачен", "nieopłacony"},
	"n_drawer_only": {"расход от %s — из кассы в этом периоде", "wydatek z dnia %s — z kasy w tym okresie"},
	"n_price":       {"по цене %s zł", "cena sprzedaży %s zł"},
	"n_nocost":      {"себестоимость неизвестна", "koszt nieznany"},
	"n_stated":      {"в ящике названо %s zł", "stan podany %s zł"},
	"n_session":     {"сессия с %s", "sesja od %s"},
	"n_dest_bank":   {"в банк", "do banku"},
	"n_dest_safe":   {"в сейф", "do sejfu"},
}

// tr — слово выгрузки: lang 0 — русский, 1 — польский.
func tr(lang int, key string) string { return exportWords[key][lang] }

func trMethod(lang int, m string) string {
	if w, ok := exportWords["m_"+m]; ok {
		return w[lang]
	}
	return m
}

// rateLabel — ставка для файла: по-русски «23 %», по-польски «23%»; zw — «zw».
func rateLabel(lang, r int) string {
	l := models.VatLabel(r)
	if lang == 1 {
		l = strings.ReplaceAll(l, " ", "")
	}
	return l
}

// ── журнал «Все операции» ──────────────────────────────────────────────────────────────────────

// exportRaw — операция из базы, как её отдаёт exportOpsSQL.
type exportRaw struct {
	Kind    string
	At      time.Time
	Method  string
	Refund  string
	PartyID *uuid.UUID
	Party   string
	Doc     string
	Descr   string
	Qty     int64
	Sum     int64
	Drawer  int64
	Vat     *int
	WhoID   *uuid.UUID
	Note    string
	ID      string
	RefAt   *time.Time // сессия — её начало; расход — день операции
	Extra   int64      // подарок — цена продажи; расход — VAT документа; коррекция — сколько названо в ящике
	Flag    bool       // расход — оплачен
}

// exportOp — строка журнала.
type exportOp struct {
	exportRaw
	Day   time.Time
	Taxed int8 // 1 — облагается VAT сейчас, 0 — нет, −1 — не продажа (расход, касса)
	Rate  int
	Net   *int64
	VatG  *int64
}

// exportOpsSQL — денежные операции за [@from, @to): пополнения и их отмены, продажи (по моменту оплаты) и их отмены,
// пакеты и их отмены, время с кошелька (сессия за клубные сутки — строкой), подарки товаров, расходы (по дню
// операции и по моменту, когда ушли из кассы), инкассации и коррекции ящика, возвраты владельцу. Суммы и движение
// ящика — теми же условиями, что «Прибыль», возвраты (returnsSQL) и ящик (drawerFlowFor). %[1]s — клубные сутки
// списания с кошелька.
const exportOpsSQL = `SELECT * FROM (
  SELECT 'deposit'::text AS kind, d.created_at AS at, d.method::text AS method, ''::text AS refund, d.user_id AS party_id,
         ''::text AS party, ''::text AS doc, ''::text AS descr, 0::bigint AS qty, ROUND(d.amount_pln * 100)::bigint AS sum,
         (CASE WHEN d.method = 'cash' THEN ROUND(d.amount_pln * 100) ELSE 0 END)::bigint AS drawer,
         COALESCE(d.vat_pct, @topup)::int AS vat, d.created_by AS who_id, ''::text AS note, d.id::text AS id,
         NULL::timestamptz AS ref_at, 0::bigint AS extra, false AS flag
    FROM deposits d WHERE d.created_at >= @from AND d.created_at < @to
  UNION ALL
  SELECT 'deposit_void', d.voided_at, d.method, COALESCE(d.refund_method, d.method), d.user_id, '', '', '', 0,
         CASE WHEN COALESCE(d.refund_method, d.method) IN ('cash', 'card', 'blik', 'wallet') THEN -d.refund_grosz ELSE 0 END,
         CASE WHEN COALESCE(d.refund_method, d.method) = 'cash' THEN -d.refund_grosz ELSE 0 END,
         COALESCE(d.vat_pct, @topup), d.voided_by, '', d.id::text, d.created_at, 0, false
    FROM deposits d WHERE d.voided_at >= @from AND d.voided_at < @to
  UNION ALL
  SELECT CASE WHEN o.id IS NULL THEN 'sale' ELSE 'kitchen' END, s.paid_at, s.method, '', s.user_id, '', COALESCE(s.receipt_id::text, ''),
         s.name, s.qty, ROUND(s.total_pln * 100)::bigint, CASE WHEN s.method = 'cash' THEN ROUND(s.total_pln * 100)::bigint ELSE 0 END,
         COALESCE(s.vat_pct, g.vat_pct, @def), COALESCE(s.paid_by, s.created_by), '', s.id::text, NULL, 0, false
    FROM sales s LEFT JOIN goods g ON g.id = s.good_id LEFT JOIN kitchen_orders o ON o.sale_id = s.id
   WHERE s.method <> 'comp' AND s.paid_at >= @from AND s.paid_at < @to
  UNION ALL
  SELECT 'sale_void', s.voided_at, s.method, COALESCE(s.refund_method, s.method), s.user_id, '', COALESCE(s.receipt_id::text, ''),
         s.name, s.qty,
         CASE WHEN COALESCE(s.refund_method, s.method) IN ('cash', 'card', 'blik', 'wallet') THEN -ROUND(s.total_pln * 100)::bigint ELSE 0 END,
         CASE WHEN COALESCE(s.refund_method, s.method) = 'cash' THEN -ROUND(s.total_pln * 100)::bigint ELSE 0 END,
         COALESCE(s.vat_pct, g.vat_pct, @def), s.voided_by, '', s.id::text, s.paid_at, 0, false
    FROM sales s LEFT JOIN goods g ON g.id = s.good_id
   WHERE s.paid_at IS NOT NULL AND s.method <> 'comp' AND s.voided_at >= @from AND s.voided_at < @to
  UNION ALL
  SELECT 'pack', p.created_at, p.method, '', p.user_id, '', '', p.name, 1, ROUND(p.price_pln * 100)::bigint,
         CASE WHEN p.method = 'cash' THEN ROUND(p.price_pln * 100)::bigint ELSE 0 END,
         COALESCE(p.vat_pct, tp.vat_pct, z.vat_pct, @def), p.created_by, '', p.id::text, NULL, 0, false
    FROM user_packages p LEFT JOIN time_packages tp ON tp.id = p.package_id LEFT JOIN zones z ON z.id = p.zone_id
   WHERE p.method <> 'comp' AND p.created_at >= @from AND p.created_at < @to
  UNION ALL
  SELECT 'pack_void', p.voided_at, p.method, COALESCE(p.refund_method, p.method), p.user_id, '', '', p.name, 1,
         CASE WHEN COALESCE(p.refund_method, p.method) IN ('cash', 'card', 'blik', 'wallet') THEN -p.refund_grosz ELSE 0 END,
         CASE WHEN COALESCE(p.refund_method, p.method) = 'cash' THEN -p.refund_grosz ELSE 0 END,
         COALESCE(p.vat_pct, tp.vat_pct, z.vat_pct, @def), p.voided_by, '', p.id::text, p.created_at, 0, false
    FROM user_packages p LEFT JOIN time_packages tp ON tp.id = p.package_id LEFT JOIN zones z ON z.id = p.zone_id
   WHERE p.method <> 'comp' AND p.voided_at >= @from AND p.voided_at < @to
  UNION ALL
  SELECT 'time', MIN(w.created_at), 'wallet', '', w.user_id, '', COALESCE(MAX(pc.name), ''), '', 0, SUM(-w.amount_grosz)::bigint, 0,
         COALESCE(MAX(se.vat_pct), MAX(z.vat_pct), @def), NULL::uuid, '', COALESCE(se.id::text, ''), MIN(se.started_at), 0, false
    FROM wallet_transactions w LEFT JOIN sessions se ON se.id = w.ref_id
    LEFT JOIN computers pc ON pc.id = se.computer_id LEFT JOIN zones z ON z.id = pc.zone_id
   WHERE w.kind = 'session_spend' AND w.amount_grosz < 0 AND w.created_at >= @from AND w.created_at < @to
   GROUP BY w.user_id, se.id, %[1]s
  UNION ALL
  SELECT 'gift', s.created_at, 'comp', '', s.user_id, '', COALESCE(s.receipt_id::text, ''), s.name, s.qty, 0, 0,
         COALESCE(s.vat_pct, g.vat_pct, @def), s.created_by, COALESCE(s.comp_reason, ''), s.id::text, NULL, ROUND(s.total_pln * 100)::bigint, false
    FROM sales s LEFT JOIN goods g ON g.id = s.good_id
   WHERE s.method = 'comp' AND s.voided_at IS NULL AND s.created_at >= @from AND s.created_at < @to
  UNION ALL
  SELECT CASE WHEN c.purchase THEN 'purchase' ELSE 'expense' END, COALESCE(e.drawer_at, e.created_at), e.method, '', NULL::uuid,
         COALESCE(cp.name, ''), e.doc_no, c.name || CASE WHEN e.note <> '' THEN ': ' || e.note ELSE '' END, 0,
         CASE WHEN e.op_date >= @fromDay AND e.op_date <= @toDay THEN -e.gross_grosz ELSE 0 END,
         CASE WHEN e.drawer_at >= @from AND e.drawer_at < @to THEN -e.gross_grosz ELSE 0 END,
         NULL::int, e.created_by, '', e.id::text, e.op_date::timestamptz, e.vat_grosz, e.paid
    FROM expenses e JOIN expense_categories c ON c.id = e.category_id LEFT JOIN counterparties cp ON cp.id = e.counterparty_id
   WHERE e.storno_at IS NULL AND ((e.op_date >= @fromDay AND e.op_date <= @toDay) OR (e.drawer_at >= @from AND e.drawer_at < @to))
  UNION ALL
  SELECT CASE WHEN ce.kind = 'collect' THEN 'collect' ELSE 'correct' END, ce.created_at, COALESCE(ce.dest, ''), '', NULL::uuid, '', '', '', 0, 0,
         CASE WHEN ce.kind = 'collect' THEN -ce.amount_grosz ELSE ce.amount_grosz END,
         NULL::int, ce.created_by, ce.note, ce.id::text, NULL, COALESCE(ce.stated_grosz, 0), false
    FROM cash_events ce WHERE ce.voided_at IS NULL AND ce.created_at >= @from AND ce.created_at < @to
  UNION ALL
  SELECT 'owner_repay', r.created_at, r.method, '', NULL::uuid, '', '', '', 0, -r.amount_grosz,
         CASE WHEN r.method = 'till' THEN -r.amount_grosz ELSE 0 END, NULL::int, r.created_by, r.note, r.id::text, NULL, 0, false
    FROM owner_repayments r WHERE r.voided_at IS NULL AND r.created_at >= @from AND r.created_at < @to
) x ORDER BY at, kind, id`

// exportTaxed — облагается ли продажа VAT сейчас и по ставке пополнения ли (чистая, тест): те же правила, что у
// «Продаж по ставкам VAT» (vatSalesSQL). sum — сумма строки со знаком (у отмены — сколько вернули).
func exportTaxed(kind, method, refund string, sum int64, cfg vatConfig) (taxed, topupRate bool) {
	live := refund == "cash" || refund == "card" || refund == "blik"
	switch kind {
	case "deposit":
		return cfg.Topup, false
	case "deposit_void":
		return cfg.Topup && live && sum != 0, false
	case "time":
		return !cfg.Topup, false
	case "sale", "kitchen", "pack":
		return method != models.SaleMethodComp && !(cfg.Topup && method == "wallet"), false
	case "sale_void", "pack_void":
		return sum != 0 && (live || (!cfg.Topup && refund == "wallet")), cfg.Topup && method == "wallet"
	case "gift":
		return cfg.Gifts && cfg.Payer && sum != 0, false
	}
	return false, false
}

// exportSale — вид строки — продажа (то, что может облагаться VAT), а не расход и не движение ящика.
func exportSale(kind string) bool {
	switch kind {
	case "deposit", "deposit_void", "sale", "kitchen", "sale_void", "pack", "pack_void", "time", "gift":
		return true
	}
	return false
}

// exportOpsIn — журнал периода p: строки по времени, у каждой — клубный день, облагается ли и VAT по строке.
func exportOpsIn(p period, reportHour int, cfg vatConfig) []exportOp {
	args := profitArgs(p)
	args["topup"], args["def"] = cfg.TopupVat, models.VatDefault
	var raws []exportRaw
	db.Raw(fmt.Sprintf(exportOpsSQL, dayExpr("w.created_at", reportHour)), args).Scan(&raws)
	// подарки — по себестоимости партий тем же счётом, что «Подарено» и маржа (goodsEconSQL)
	cost := map[string]int64{}
	var gifts []goodsEcon
	db.Raw(fmt.Sprintf(goodsEconSQL, "s.id::text", "", "method = '"+models.SaleMethodComp+"'"), map[string]any{"from": p.From, "to": p.To}).Scan(&gifts)
	for _, x := range gifts {
		cost[x.Key] = x.Cost
	}
	loc := p.FromDay.Location()
	ops := make([]exportOp, 0, len(raws))
	for _, r := range raws {
		op := exportOp{exportRaw: r}
		op.At = r.At.In(loc)
		op.Day = clubDayOf(op.At, reportHour)
		if r.Kind == "gift" {
			op.Sum = cost[r.ID]
		}
		if (r.Kind == "expense" || r.Kind == "purchase") && r.RefAt != nil && r.Sum != 0 {
			t := r.RefAt.In(loc)
			op.Day = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc) // расход — по дню операции
		}
		op.Taxed = -1
		if exportSale(r.Kind) {
			op.Taxed = 0
			taxed, topup := exportTaxed(r.Kind, r.Method, r.Refund, op.Sum, cfg)
			if taxed {
				op.Taxed, op.Rate = 1, models.VatDefault
				if r.Vat != nil {
					op.Rate = *r.Vat
				}
				if topup {
					op.Rate = cfg.TopupVat
				}
				if !cfg.Payer {
					op.Rate = models.VatZW
				}
				v := vatOf(op.Sum, op.Rate)
				net := op.Sum - v
				op.VatG, op.Net = &v, &net
			}
		} else if (r.Kind == "expense" || r.Kind == "purchase") && op.Sum != 0 { // VAT документа расхода — со знаком суммы
			v := -r.Extra
			net := op.Sum - v
			op.VatG, op.Net = &v, &net
		}
		ops = append(ops, op)
	}
	return ops
}

// exportJournal — лист «Все операции».
func exportJournal(ops []exportOp, lang int) xsheet {
	ids := []string{}
	for _, o := range ops {
		for _, u := range []*uuid.UUID{o.PartyID, o.WhoID} {
			if u != nil {
				ids = append(ids, u.String())
			}
		}
	}
	nick := nicknamesByID(ids)
	who := func(u *uuid.UUID) string {
		if u == nil {
			return ""
		}
		return nick[u.String()]
	}
	h := func(keys ...string) []string {
		out := make([]string, len(keys))
		for i, k := range keys {
			out[i] = tr(lang, k)
		}
		return out
	}
	sh := xsheet{name: tr(lang, "sheet_ops"), file: []string{"vse_operacii", "wszystkie_operacje"}[lang], table: true,
		head:   h("h_n", "h_day", "h_at", "h_kind", "h_method", "h_party", "h_doc", "h_desc", "h_qty", "h_sum", "h_taxed", "h_rate", "h_net", "h_vat", "h_drawer", "h_who", "h_note", "h_id"),
		widths: []int{7, 12, 17, 26, 14, 24, 14, 36, 7, 14, 12, 10, 14, 12, 12, 14, 40, 38}}
	day := func(t time.Time) string { return t.Format("02.01.2006") }
	for i, o := range ops {
		method := trMethod(lang, o.Method)
		note := o.Note
		party := o.Party
		if party == "" {
			party = who(o.PartyID)
		}
		switch o.Kind {
		case "deposit_void", "sale_void", "pack_void":
			method = trMethod(lang, o.Refund)
			if o.Refund != o.Method {
				note = fmt.Sprintf(tr(lang, "n_paid"), trMethod(lang, o.Method))
			}
		case "time":
			if o.RefAt != nil {
				note = fmt.Sprintf(tr(lang, "n_session"), o.RefAt.In(o.At.Location()).Format("02.01 15:04"))
			}
		case "gift":
			extra := fmt.Sprintf(tr(lang, "n_price"), moneyText(o.Extra, ','))
			if o.Sum == 0 {
				extra += " · " + tr(lang, "n_nocost")
			}
			note = strings.Trim(note+" · "+extra, " ·")
		case "expense", "purchase":
			if o.Sum == 0 && o.RefAt != nil {
				note = fmt.Sprintf(tr(lang, "n_drawer_only"), day(o.RefAt.In(o.At.Location())))
			} else if !o.Flag {
				note = tr(lang, "n_unpaid")
			}
		case "collect":
			method = trMethod(lang, "till")
			dest := ""
			if o.Method == "bank" || o.Method == "safe" {
				dest = tr(lang, "n_dest_"+o.Method)
			}
			note = strings.Trim(dest+" · "+note, " ·")
		case "correct":
			method = ""
			note = strings.Trim(fmt.Sprintf(tr(lang, "n_stated"), moneyText(o.Extra, ','))+" · "+note, " ·")
		}
		qty := xcell{}
		if o.Qty != 0 {
			qty = cInt(o.Qty)
		}
		taxed, rate, net, vat := xcell{}, xcell{}, xcell{}, xcell{}
		switch o.Taxed {
		case 1:
			taxed, rate = cText(tr(lang, "yes")), cText(rateLabel(lang, o.Rate))
		case 0:
			taxed = cText(tr(lang, "no"))
		default:
			taxed = cText("—")
		}
		if o.Net != nil {
			net, vat = cMoney(*o.Net), cMoney(*o.VatG)
		}
		sh.rows = append(sh.rows, xrow{cells: []xcell{
			cInt(int64(i + 1)), cDay(o.Day), cStamp(o.At), cText(tr(lang, "k_"+o.Kind)), cText(method), cText(party),
			cText(o.Doc), cText(o.Descr), qty, cMoney(o.Sum), taxed, rate, net, vat, cMoney(o.Drawer), cText(who(o.WhoID)),
			cText(note), cText(o.ID)}})
	}
	sh.n = len(sh.rows)
	return sh
}

// ── «Продажи по дням и ставкам» ────────────────────────────────────────────────────────────────

// vatPayOrder — способы оплаты колонками: наличные, карта, BLIK, кошелёк, подарено, остальные — по алфавиту.
func vatPayOrder(m map[string]int64) []string {
	fixed := []string{"cash", "card", "blik", "wallet", models.SaleMethodComp}
	out := []string{}
	for _, k := range fixed {
		if _, ok := m[k]; ok {
			out = append(out, k)
		}
	}
	rest := []string{}
	for k := range m {
		found := false
		for _, f := range fixed {
			found = found || f == k
		}
		if !found {
			rest = append(rest, k)
		}
	}
	sort.Strings(rest)
	return append(out, rest...)
}

// exportVatDays — лист «Продажи по дням и ставкам»: дни с продажами, по ставкам брутто, нетто и VAT (VAT дня — из
// брутто дня, как в «Продажах по ставкам VAT»), итог дня и брутто по способам оплаты; внизу — «Итого» за период.
func exportVatDays(p period, a vatAgg, lang int) xsheet {
	rates, pays := vatOrder(a.Rates), vatPayOrder(a.Methods)
	sh := xsheet{name: tr(lang, "sheet_vat"), file: []string{"prodazhi_po_dnyam", "sprzedaz_wg_dni"}[lang], table: true}
	sh.head = []string{tr(lang, "h_day")}
	sh.widths = []int{12}
	for _, r := range rates {
		l := rateLabel(lang, r)
		sh.head = append(sh.head, tr(lang, "h_gross")+" "+l, tr(lang, "h_net")+" "+l, "VAT "+l)
		sh.widths = append(sh.widths, 13, 13, 12)
	}
	sh.head = append(sh.head, tr(lang, "h_total")+" "+strings.ToLower(tr(lang, "h_gross")), tr(lang, "h_total")+" "+strings.ToLower(tr(lang, "h_net")),
		tr(lang, "h_total")+" VAT")
	sh.widths = append(sh.widths, 14, 14, 13)
	for _, m := range pays {
		sh.head = append(sh.head, tr(lang, "h_pay")+trMethod(lang, m))
		sh.widths = append(sh.widths, 15)
	}
	line := func(first xcell, cells map[int]*vatCell, methods map[string]int64, bold bool) xrow {
		row := []xcell{first}
		var tg, tv int64
		for _, r := range rates {
			c := cells[r]
			if c == nil {
				row = append(row, xcell{}, xcell{}, xcell{})
				continue
			}
			row = append(row, cMoney(c.Gross), cMoney(c.Gross-c.Vat), cMoney(c.Vat))
			tg, tv = tg+c.Gross, tv+c.Vat
		}
		row = append(row, cMoney(tg), cMoney(tg-tv), cMoney(tv))
		for _, m := range pays {
			if g, ok := methods[m]; ok {
				row = append(row, cMoney(g))
			} else {
				row = append(row, xcell{})
			}
		}
		return xrow{cells: row, bold: bold}
	}
	for d := p.FromDay; !d.After(p.ToDay); d = d.AddDate(0, 0, 1) {
		k := d.Format("2006-01-02")
		if len(a.Days[k]) == 0 {
			continue
		}
		sh.rows = append(sh.rows, line(cDay(d), a.Days[k], a.DayMethods[k], false))
	}
	sh.n = len(sh.rows)
	sh.rows = append(sh.rows, line(cText(tr(lang, "h_total")), a.Rates, a.Methods, true))
	return sh
}

// ── «О выгрузке» ───────────────────────────────────────────────────────────────────────────────

// exportAboutWords — строки «О выгрузке»: [русский, польский].
var exportAboutWords = map[string][2]string{
	"club": {"Клуб", "Klub"}, "address": {"Адрес", "Adres"}, "nip": {"NIP", "NIP"}, "nip_none": {"не задан", "nie podano"},
	"period": {"Период", "Okres"}, "day": {"Клубные сутки", "Doba klubowa"},
	"day_v": {"с %02d:00 до %02d:00 следующего дня", "od %02d:00 do %02d:00 dnia następnego"},
	"made":  {"Сделана", "Wygenerowano"}, "format": {"Формат", "Format"}, "lang": {"Язык", "Język"},
	"lang_v":  {"русский", "polski"},
	"profile": {"Профиль учёта", "Profil ewidencji"}, // З13-и6: значения и строки профиля — export_profiles.go
	"payer":   {"Плательщик VAT", "Podatnik VAT"}, "payer_yes": {"да — czynny podatnik VAT", "tak — czynny podatnik VAT"},
	"payer_no":     {"нет — освобождён (zwolniony z VAT)", "nie — zwolniony z VAT"},
	"moment":       {"VAT с пополнения кошелька", "VAT od doładowań portfela"},
	"moment_spend": {"при трате — bon różnego przeznaczenia", "przy wykorzystaniu — bon różnego przeznaczenia"},
	"moment_topup": {"при пополнении — bon jednego przeznaczenia", "przy doładowaniu — bon jednego przeznaczenia"},
	"topup_rate":   {"Ставка VAT пополнения", "Stawka VAT doładowań"},
	"gifts":        {"Подарки товаров в VAT", "Nieodpłatne przekazanie towarów w VAT"},
	"gifts_yes":    {"да — по себестоимости", "tak — według kosztu zakupu"}, "gifts_no": {"нет", "nie"},
	"rates": {"Ставки VAT в периоде", "Stawki VAT w okresie"}, "rates_none": {"продаж не было", "brak sprzedaży"},
	"gross": {"Продажи брутто", "Sprzedaż brutto"}, "net": {"Продажи нетто", "Sprzedaż netto"}, "vat": {"VAT от продаж", "VAT należny"},
	"rules": {"Как посчитан VAT", "Zasady VAT"},
	"rules_spend": {"при трате: время с кошелька — по ставке зоны в момент списания, пакеты — при выдаче, товары и кухня — при оплате, и с кошелька; пополнения — без VAT",
		"przy wykorzystaniu: czas gry z portfela — wg stawki strefy w chwili pobrania, pakiety — przy wydaniu, towary i kuchnia — przy zapłacie, także z portfela; doładowania — bez VAT"},
	"rules_topup": {"при пополнении: пополнения — по ставке пополнения, пакеты, товары и кухня за живые деньги — по своей ставке; оплаченное с кошелька — без VAT",
		"przy doładowaniu: doładowania — wg stawki doładowań, pakiety, towary i kuchnia za gotówkę, kartą i BLIK — wg własnej stawki; opłacone z portfela — bez VAT"},
	"rules_common": {"долг — когда рассчитан; возвраты — минусом в день отмены по ставке, с которой брали VAT; «не возвращали» — ничего; VAT дня по ставке — из брутто дня, как дневной отчёт кассы",
		"sprzedaż na kredyt — przy zapłacie; zwroty — z minusem w dniu anulowania wg stawki, z którą naliczono VAT; bez zwrotu pieniędzy — nic; VAT dnia wg stawki — z brutto dnia, jak raport dobowy kasy"},
	"rules_gifts": {"подарки товаров — по себестоимости партий, по ставке позиции", "nieodpłatne przekazanie towarów — wg kosztu zakupu partii, wg stawki towaru"},
	"rules_zw":    {"клуб освобождён от VAT: все продажи — «zw», VAT ноль", "klub zwolniony z VAT: cała sprzedaż — «zw», VAT zero"},
	"sheets":      {"Листы", "Arkusze"},
	"sheets_rows": {"строк", "wierszy"}, "sheets_days": {"дней с продажами", "dni ze sprzedażą"},
	"sheets_docs": {"документов", "dokumentów"},
	"at_end":      {"На конец периода", "Na koniec okresu"},
	"at_end_now":  {"период ещё идёт — на момент выгрузки", "okres trwa — stan w chwili eksportu"},
	"kpkw":        {"Номера KP/KW", "Numeracja KP/KW"},
	"kpkw_0":      {"с начала месяца: KP 1/%02d/%d, KW 1/%02d/%d", "od początku miesiąca: KP 1/%02d/%d, KW 1/%02d/%d"},
	"kpkw_1":      {"с начала года: KP 1/%d, KW 1/%d", "od początku roku: KP 1/%d, KW 1/%d"},
	"kpkw_2":      {"без номеров", "bez numerów"},
	"sums":        {"Суммы", "Kwoty"},
	"sums_v": {"в злотых (PLN), брутто со знаком: + пришло или продано, − возвращено или потрачено; «Касса» — движение наличных в ящике клуба",
		"w złotych (PLN), brutto ze znakiem: + wpływ lub sprzedaż, − zwrot lub wydatek; «Kasa» — ruch gotówki w kasie klubu"},
	"vat_note": {"VAT в «Все операции»", "VAT w «Wszystkie operacje»"},
	"vat_note_v": {"по каждой операции, до гроша; итог по ставкам — лист «Продажи по дням и ставкам», из брутто дня, как касса: разница — гроши округления",
		"dla każdej operacji, do grosza; suma wg stawek — arkusz «Sprzedaż wg dni i stawek», z brutto dnia, jak kasa: różnica — grosze zaokrągleń"},
	"expenses_note": {"Расходы", "Wydatki"},
	"expenses_note_v": {"по дню операции, без сторно, оплаченные и нет; VAT — по документу; закупка товара — отдельной операцией; лист «Расходы» — все расходы периода, сторнированные — строками с пометкой, в итог не входят",
		"wg daty operacji, bez storn, opłacone i nie; VAT — z dokumentu; zakup towaru — osobna operacja; arkusz «Wydatki» — wszystkie wydatki okresu, stornowane — wierszami z adnotacją, poza sumą"},
	"earned_note": {"Отработано", "Przychód wykonany"},
	"earned_note_v": {"время с кошелька, пакеты по мере игры (до журнала расхода пакетов — оценка по средней цене минуты), товары у стойки и кухня в момент выдачи; пополнения и проданные пакеты — предоплата, сюда не входят; «отдано временем» (монеты, компенсации) — не выручка, рядом",
		"czas gry z portfela, pakiety w miarę wykorzystania (sprzed ewidencji zużycia pakietów — szacunek wg średniej ceny minuty), towary i kuchnia w chwili wydania; doładowania i sprzedane pakiety to przedpłata — tu nie wchodzą; czas oddany (monety, rekompensaty) — nie przychód, obok"},
	"cash_note": {"Касса", "Raport kasowy"},
	"cash_note_v": {"наличные ящика клуба по дням: остаток на начало, наличные за день по видам — одним документом KP (как дневной отчёт кассы), возвраты наличными, расходы из кассы, возвраты владельцу, инкассации и коррекции — каждый своим документом, расхождение пересчётов дня — строкой, остаток на конец; номера — по порядку на момент выгрузки: правка прошлых дней сдвигает номера после неё",
		"gotówka w kasie klubu wg dni: stan na początek, gotówka za dzień wg rodzaju — jednym dokumentem KP (jak raport dobowy kasy fiskalnej), zwroty gotówką, wydatki z kasy, zwroty właścicielowi, wypłaty (inkaso) i korekty — każdy osobnym dokumentem, różnica kasowa dnia — wierszem, stan na koniec; numery — kolejno w chwili eksportu: korekta minionych dni przesuwa numery po niej"},
	"stock_note": {"Склад", "Magazyn"},
	"stock_note_v": {"остатки по партиям на конец периода, цена закупки за штуку — брутто; партий без цены — %d шт., в стоимость не вошли",
		"stan partii na koniec okresu, cena zakupu za szt. — brutto; w partiach bez ceny — %d szt., poza wartością"},
	"owed_note": {"Предоплаты и долги", "Przedpłaty i należności"},
	"owed_note_v": {"кошельки и неотыгранные пакеты (по доле цены, подаренные — 0 zł) — что клуб должен гостям за оплаченное; долги гостей — бар в долг и выданная кухня, не рассчитанные и не списанные",
		"portfele i niewykorzystane pakiety (proporcjonalnie do ceny, nieodpłatne — 0 zł) — co klub winien klientom za opłacone; należności od klientów — bar na kredyt i wydana kuchnia, nierozliczone i niespisane"},
}

func ta(lang int, key string) string { return exportAboutWords[key][lang] }

// exportAbout — лист «О выгрузке»: как посчитано; pd — профиль учёта (З13-и6); data — листы выгрузки, у каждого —
// сколько строк данных; end — «на конец периода»; noCost — штук в партиях без цены на этот момент.
func exportAbout(p period, reportHour int, cfg vatConfig, a vatAgg, pd profileData, data []xsheet, made, end time.Time, scheme int,
	noCost int64, by, format string, lang int) xsheet {
	var club models.Club
	db.First(&club, "is_active = ?", true)
	req := requisitesNow()
	firm, address := firmOf(req, club) // З13-и6: реквизиты фирмы; не заданы — название и адрес клуба
	sh := xsheet{name: tr(lang, "sheet_about"), file: []string{"o_vygruzke", "o_eksporcie"}[lang],
		head: []string{tr(lang, "h_item"), tr(lang, "h_value")}, widths: []int{32, 110}}
	add := func(key string, v xcell) { sh.rows = append(sh.rows, xrow{cells: []xcell{cText(ta(lang, key)), v}}) }
	add("club", cText(club.Name))
	add("firm", cText(firm))
	nip := ta(lang, "nip_none")
	if req.NIP != "" {
		nip = req.NIP
	}
	add("nip", cText(nip))
	add("address", cText(address))
	add("period", cText(p.FromDay.Format("02.01.2006")+" — "+p.ToDay.Format("02.01.2006")))
	add("day", cText(fmt.Sprintf(ta(lang, "day_v"), reportHour, reportHour)))
	add("made", cText(made.In(p.FromDay.Location()).Format("02.01.2006 15:04")+" · "+by)) // по часам клуба, как журнал
	add("format", cText(format))
	add("lang", cText(ta(lang, "lang_v")))
	sh.rows = append(sh.rows, profileAbout(pd, lang)...)
	payer, moment, gifts := "payer_yes", "moment_spend", "gifts_no"
	if !cfg.Payer {
		payer = "payer_no"
	}
	if cfg.Topup {
		moment = "moment_topup"
	}
	if cfg.Gifts {
		gifts = "gifts_yes"
	}
	add("payer", cText(ta(lang, payer)))
	add("moment", cText(ta(lang, moment)))
	add("topup_rate", cText(rateLabel(lang, cfg.TopupVat)))
	add("gifts", cText(ta(lang, gifts)))
	labels := []string{}
	var g, v int64
	for _, r := range vatOrder(a.Rates) {
		labels = append(labels, rateLabel(lang, r))
		g, v = g+a.Rates[r].Gross, v+a.Rates[r].Vat
	}
	if len(labels) == 0 {
		labels = append(labels, ta(lang, "rates_none"))
	}
	add("rates", cText(strings.Join(labels, ", ")))
	add("gross", cMoney(g))
	add("net", cMoney(g-v))
	add("vat", cMoney(v))
	rules := ta(lang, "rules_spend")
	if cfg.Topup {
		rules = ta(lang, "rules_topup")
	}
	rules += "; " + ta(lang, "rules_common")
	if cfg.Gifts && cfg.Payer {
		rules += "; " + ta(lang, "rules_gifts")
	}
	if !cfg.Payer {
		rules = ta(lang, "rules_zw") + "; " + rules
	}
	add("rules", cText(rules))
	list := make([]string, 0, len(data))
	for _, d := range data {
		unit := ta(lang, "sheets_rows")
		switch d.name {
		case tr(lang, "sheet_vat"):
			unit = ta(lang, "sheets_days")
		case tr(lang, "sheet_cash"):
			unit = ta(lang, "sheets_docs")
		}
		list = append(list, fmt.Sprintf("«%s» — %s: %d", d.name, unit, d.n))
	}
	add("sheets", cText(strings.Join(list, "; ")))
	atEnd := end.In(p.FromDay.Location()).Format("02.01.2006 15:04")
	if end.Equal(made) {
		atEnd += " — " + ta(lang, "at_end_now")
	}
	add("at_end", cText(atEnd))
	kpkw := ta(lang, "kpkw_2")
	switch scheme {
	case kpkwMonth:
		kpkw = fmt.Sprintf(ta(lang, "kpkw_0"), int(p.FromDay.Month()), p.FromDay.Year(), int(p.FromDay.Month()), p.FromDay.Year())
	case kpkwYear:
		kpkw = fmt.Sprintf(ta(lang, "kpkw_1"), p.FromDay.Year(), p.FromDay.Year())
	}
	add("kpkw", cText(kpkw))
	add("sums", cText(ta(lang, "sums_v")))
	add("vat_note", cText(ta(lang, "vat_note_v")))
	add("expenses_note", cText(ta(lang, "expenses_note_v")))
	add("earned_note", cText(ta(lang, "earned_note_v")))
	add("cash_note", cText(ta(lang, "cash_note_v")))
	add("stock_note", cText(fmt.Sprintf(ta(lang, "stock_note_v"), noCost)))
	add("owed_note", cText(ta(lang, "owed_note_v")))
	return sh
}

// ── ручка ────────────────────────────────────────────────────────────────────────────────────────

// exportFormat — как писать файл по настройкам «Бухгалтерии» (З13-и4): разделитель CSV (0 «;», 1 «,», 2 табуляция),
// десятичный знак (0 «,», 1 «.»), кодировка (0 UTF-8 с BOM, 1 Windows-1250), даты (0 ДД.ММ.ГГГГ, 1 ГГГГ-ММ-ДД).
func exportFormat(now time.Time) bookFormat {
	set := exportSetting
	f := bookFormat{sep: []rune{';', ',', '\t'}[set("acc_csv_sep")], decimal: []byte{',', '.'}[set("acc_csv_decimal")],
		w1250: set("acc_csv_enc") == 1, iso: set("acc_date_fmt") == 1, now: now}
	return f
}

// exportSetting — числовая настройка выгрузки; строку руками в базе не туда — как по умолчанию, а не паника на индексе.
func exportSetting(key string) int64 {
	v, b := settingInt64(key, settingDefaults[key]), settingBounds[key]
	if v < b[0] || v > b[1] {
		return settingDefaults[key]
	}
	return v
}

// exportFormatText — формат для «О выгрузке» (чистая, тест).
func exportFormatText(format string, f bookFormat, lang int) string {
	if format == "xlsx" {
		return "XLSX"
	}
	sep := map[rune]string{';': "«;»", ',': "«,»", '\t': "Tab"}[f.sep]
	enc, date := "UTF-8 BOM", "31.12.2026"
	if f.w1250 {
		enc = "Windows-1250"
	}
	if f.iso {
		date = "2026-12-31"
	}
	if lang == 1 {
		return fmt.Sprintf("CSV (ZIP): separator %s, separator dziesiętny «%c», %s, daty %s", sep, f.decimal, enc, date)
	}
	return fmt.Sprintf("CSV (ZIP): разделитель %s, десятичный знак «%c», %s, даты %s", sep, f.decimal, enc, date)
}

// exportFileName — имя файла латиницей (чистая, тест): месяц — «1448-eksport-2026-09», свои даты — с первого по последний день.
func exportFileName(p period, format string, lang int) string {
	span := p.FromDay.Format("2006-01-02") + "_" + p.ToDay.Format("2006-01-02")
	last := time.Date(p.FromDay.Year(), p.FromDay.Month()+1, 0, 0, 0, 0, 0, p.FromDay.Location())
	if p.FromDay.Day() == 1 && (p.ToDay.Equal(last) || p.Preset == "month") && p.FromDay.Month() == p.ToDay.Month() && p.FromDay.Year() == p.ToDay.Year() {
		span = p.FromDay.Format("2006-01")
	}
	ext := ".xlsx"
	if format == "csv" {
		ext = ".zip"
	}
	return "1448-" + []string{"vygruzka", "eksport"}[lang] + "-" + span + ext
}

// exportPreview — что будет в файле (чистая, тест): имя файла, форма учёта и листы по порядку — имя в книге, файл в
// архиве CSV, сколько строк данных и чего: у «О выгрузке» — строк листа, у «Продаж по дням и ставкам» — дней с
// продажами, у «Кассы» — документов, у остальных — строк без итогов.
func exportPreview(p period, format string, lang, form int, sheets []xsheet) gin.H {
	out := make([]gin.H, 0, len(sheets))
	for i, sh := range sheets {
		n, unit := sh.n, "rows"
		switch {
		case i == 0:
			n = len(sh.rows)
		case sh.name == tr(lang, "sheet_vat"):
			unit = "days"
		case sh.name == tr(lang, "sheet_cash"):
			unit = "docs"
		}
		out = append(out, gin.H{"name": sh.name, "file": csvFileName(i, sh), "rows": n, "unit": unit})
	}
	return gin.H{"file": exportFileName(p, format, lang), "period": p.out(), "form": form, "format": format,
		"lang": []string{"ru", "pl"}[lang], "sheets": out}
}

// exportMaxDays — выгрузка не длиннее года (с високосным): файл собирается в памяти сервера, за больший срок — частями.
const exportMaxDays = 366

// GET /admin/export — выгрузка бухгалтеру за период (owner): XLSX или архив CSV, заголовки PL или RU; &preview=1 (З13-и7)
// — вместо файла листы с числом строк для вкладки «Выгрузка», в журнал не пишется.
func handleExport(c *gin.Context) {
	p, _, reportHour, ok := periodFromQuery(c)
	if !ok {
		return
	}
	if p.Days > exportMaxDays {
		c.JSON(http.StatusBadRequest, gin.H{"code": "too_long", "message": "Выгрузка — не длиннее года; за больший срок — частями"})
		return
	}
	format := strings.ToLower(strings.TrimSpace(c.DefaultQuery("format", "xlsx")))
	if format != "xlsx" && format != "csv" {
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_format", "message": "Формат: xlsx или csv"})
		return
	}
	lang := 1
	switch strings.ToLower(strings.TrimSpace(c.DefaultQuery("lang", "pl"))) {
	case "pl":
	case "ru":
		lang = 0
	default:
		c.JSON(http.StatusBadRequest, gin.H{"code": "bad_lang", "message": "Язык заголовков: pl или ru"})
		return
	}
	now := time.Now()
	f := exportFormat(now)
	cfg := vatConfigNow()
	vrows := vatRowsIn(p, reportHour, cfg)
	a := vatSum(vrows, cfg.Payer)
	ops := exportOpsIn(p, reportHour, cfg)
	pd := loadProfile(p, cfg, vrows) // З13-и6: форма учёта, продажи по частям, расходы периода
	journal, days := exportJournal(ops, lang), exportVatDays(p, a, lang)
	// З13-и5: листы на тех же функциях, что экраны; «на конец периода» — конец последних суток или «сейчас»
	book, end, scheme := cashBookIn(p, reportHour, now), periodEnd(p, now), int(exportSetting("acc_kpkw"))
	stock, noCost := sheetStock(end, lang)
	cash := sheetCash(p, reportHour, now, book, scheme, lang)
	// З13-и6: листы профиля — сразу после «О выгрузке»; у полной бухгалтерии «Касса» переезжает к реестрам
	data, moved := profileSheets(pd, cash, p.FromDay.Location(), lang)
	data = append(data, journal, days, sheetEarned(p, reportHour, now, lang), sheetExpenses(p, pd.Form, lang), sheetPurchases(p, lang))
	if !moved {
		data = append(data, cash)
	}
	data = append(data, sheetCloseouts(p, book, lang), sheetReturns(p, lang), sheetWriteOffs(p, lang), sheetGifts(p, lang), stock,
		sheetOwed(owedAtMoment(end), end, lang))
	by := ""
	if me, err := uuid.Parse(c.GetString("user_id")); err == nil {
		by = nicknamesByID([]string{me.String()})[me.String()]
	}
	about := exportAbout(p, reportHour, cfg, a, pd, data, now, end, scheme, noCost, by, exportFormatText(format, f, lang), lang)
	sheets := append([]xsheet{about}, data...)
	if c.Query("preview") == "1" { // З13-и7: вкладка «Выгрузка» — листы с числом строк до «Скачать», без файла и без журнала
		c.JSON(http.StatusOK, exportPreview(p, format, lang, pd.Form, sheets))
		return
	}
	var buf bytes.Buffer
	var err error
	ctype := "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	if format == "xlsx" {
		err = writeXLSX(&buf, sheets, f)
	} else {
		ctype = "application/zip"
		err = writeCSVZip(&buf, sheets, f)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "export_failed", "message": "Файл не собрался — попробуй ещё раз"})
		return
	}
	name := exportFileName(p, format, lang)
	logAdminAction(c, "export", nil, fmt.Sprintf("%s — %s · %s · %s · операций: %d",
		p.FromDay.Format("02.01.2006"), p.ToDay.Format("02.01.2006"), strings.ToUpper(format), []string{"RU", "PL"}[lang], len(ops)))
	c.Header("Content-Disposition", `attachment; filename="`+name+`"`)
	c.Data(http.StatusOK, ctype, buf.Bytes())
}
