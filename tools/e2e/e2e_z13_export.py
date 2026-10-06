#!/usr/bin/env python3
"""З13-и4 «Выгрузка: движок и «Все операции»» (№75; Д8; решения основателя 04.10, З-Р37). GET /admin/export — выгрузка
бухгалтеру за период (только владельцу): XLSX (листы в одном файле) или архив CSV (файл на лист; разделитель, десятичный
знак, кодировка — UTF-8 с BOM или Windows-1250, формат даты — настройками), заголовки PL или RU. Листы: «О выгрузке» (клуб,
период, настройки VAT, итоги, как читать), «Все операции» (каждая денежная операция строкой), «Продажи по дням и ставкам».
Файл разбирается стандартной библиотекой Python (zipfile + XML, csv); суммы листов равны экранам за тот же период — за
сегодня и за месяц: пополнения, товары и кухня, пакеты, возвраты, расходы и закупки — «Прибыли»; облагаемое по ставкам —
«Продажам по ставкам VAT» (за сегодня — в обоих режимах и с подарками); колонка «Касса» — кассовой книге; «Продажи по дням
и ставкам» — отчёту VAT по дням. Выгрузка — в журнале действий владельца, админ её не видит.
Данные: гость Guest43 — кошелёк докладывается картой до 20 zł, пополнение 60 zł наличными (в конце отменяется с
возвратом наличными), сессия за PC-05 на полчаса назад с кошелька, продажи наличными и картой (наличная отменяется с
возвратом наличными — одна по ходу, другая в уборке), подарок, кухня от стойки картой, пакет картой (отменяется с
возвратом на карту); расход «из кассы» 12,30 zł с VAT 2,30 и расход «личными» 5 zł с возвратом владельцу 5 zł из кассы,
инкассация 1 zł в банк — в конце сторно и отмены: наличные в ящике туда и обратно сходятся; позиции «z13e Кола» (23 %,
имя кириллицей — для Windows-1250) и «z13e Кухня» (8 %) заводятся каждый прогон и в конце удаляются `drop_good`;
настройки «Бухгалтерии» и файла возвращаются."""
import csv, io, urllib.error, urllib.request, xml.etree.ElementTree as ET, zipfile
from datetime import date, timedelta

from _z import *

MARK = "z13e"
NICK, PC = "Guest43", "PC-05"
G = uid(NICK)
NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
RID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"
KEYS = ("acc_vat_moment", "acc_vat_payer", "acc_gift_vat", "acc_topup_vat", "acc_csv_sep", "acc_csv_decimal", "acc_csv_enc", "acc_date_fmt")
ORIG = {k: (rows(f"SELECT value FROM settings WHERE key = '{k}'") or [None])[0] for k in KEYS}
made = {"deps": []}


def put(key, value):
    """Настройка прямо в базе: выгрузка и отчёты читают её на каждом запросе, а журналу действий шум не нужен."""
    sql(f"INSERT INTO settings (key, value) VALUES ('{key}', '{value}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value")


def back():
    for k, v in ORIG.items():
        if v is None:
            sql(f"DELETE FROM settings WHERE key = '{k}'")
        else:
            put(k, v)


def gone():
    end_all(NICK)
    for r in rows(f"SELECT id, method FROM sales WHERE name LIKE '{MARK} %' AND voided_at IS NULL AND write_off_id IS NULL"):
        sid, method = r.split("|")  # наличные — обратно из ящика: ящик сходится
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "cash" if method == "cash" else "none"}, "boss")
    for oid in rows(f"SELECT id FROM kitchen_orders WHERE name LIKE '{MARK} %' AND status IN ('new','accepted','preparing','delivering')"):
        api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE user_id = '{G}' AND voided_at IS NULL"):  # и хвосты упавшего прогона
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for did in made["deps"]:
        if sql(f"SELECT voided_at IS NULL FROM deposits WHERE id = '{did}'") == "t":
            api("POST", f"/admin/deposits/{did}/void", {"refund_method": "none"}, "boss")
    for i in rows(f"SELECT id FROM owner_repayments WHERE note LIKE '{MARK}%' AND voided_at IS NULL"):
        api("POST", f"/admin/owner-repayments/{i}/void", None, "boss")
    for i in rows(f"SELECT id FROM cash_events WHERE note LIKE '{MARK}%' AND voided_at IS NULL"):
        api("POST", f"/admin/cash/events/{i}/void", {}, "boss")
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{MARK} %'"):
        drop_good(gid)
    back()


gone()
at_end(gone)
T0 = sql("SELECT now()::text")  # с этой минуты — выгрузки этого прогона в журнале действий
for k in KEYS:  # с известного начала: плательщик VAT, при трате, подарки без VAT; CSV — «;», «,», UTF-8, ДД.ММ.ГГГГ
    sql(f"DELETE FROM settings WHERE key = '{k}'")
g = lambda v: round((v or 0) * 100)
drawer_now = lambda: g((api("GET", "/admin/cash/book?preset=today", None, "boss")[1].get("now") or {}).get("expected_pln"))
CASH0 = drawer_now()  # «должно быть в ящике» до проверки — после уборки должно быть столько же
D = club_day(0)
DAY, MONTH = f"from={D}&to={D}", f"month={D[:7]}"
DMY = f"{D[8:]}.{D[5:7]}.{D[:4]}"


# ── файл ─────────────────────────────────────────────────────────────────────────────────────────
def raw(q=DAY, fmt="xlsx", lang="pl"):
    """Файл байтами: (статус, Content-Type, Content-Disposition, тело); ошибка сервера — статус и пустое тело."""
    req = urllib.request.Request(f"{API}/admin/export?{q}&format={fmt}&lang={lang}")
    req.add_header("Authorization", "Bearer " + tok("boss"))
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.headers.get("Content-Type") or "", r.headers.get("Content-Disposition") or "", r.read()
    except urllib.error.HTTPError as e:
        return e.code, "", "", b""


def unzip(data):
    try:
        return zipfile.ZipFile(io.BytesIO(data))
    except zipfile.BadZipFile:
        return None


EMPTY = {"rows": [[]], "frozen": False, "filter": None}


def xlsx(data):
    """Книга стандартной библиотекой: ({имя листа: {rows, frozen, filter}}, styles.xml); текст — строкой, число —
    float, ячейки, которой в файле нет, — None."""
    z = unzip(data)
    if not z:
        return {}, ""
    wb = ET.fromstring(z.read("xl/workbook.xml"))
    rels = {r.get("Id"): r.get("Target") for r in ET.fromstring(z.read("xl/_rels/workbook.xml.rels"))}
    out = {}
    for sh in wb.find("m:sheets", NS):
        root = ET.fromstring(z.read("xl/" + rels[sh.get(RID)]))
        table = []
        for row in root.find("m:sheetData", NS):
            cells = {}
            for c in row:
                n = 0
                for ch in c.get("r"):
                    if ch.isalpha():
                        n = n * 26 + ord(ch) - 64
                if c.get("t") == "inlineStr":
                    cells[n - 1] = "".join(t.text or "" for t in c.iter("{%s}t" % NS["m"]))
                else:
                    v = c.find("m:v", NS)
                    cells[n - 1] = float(v.text) if v is not None else None
            table.append([cells.get(i) for i in range(max(cells) + 1 if cells else 0)])
        af = root.find("m:autoFilter", NS)
        out[sh.get("name")] = {"rows": table, "frozen": root.find(".//m:pane[@state='frozen']", NS) is not None,
                               "filter": af.get("ref") if af is not None else None}
    return out, z.read("xl/styles.xml").decode()


def table(sheet):
    """Строки листа словарями по шапке."""
    head = sheet["rows"][0]
    return [dict(zip(head, r + [None] * (len(head) - len(r)))) for r in sheet["rows"][1:]]


def pairs(sheet):
    """«О выгрузке»: «что» → «значение»."""
    return {r[0]: (r[1] if len(r) > 1 else None) for r in sheet["rows"][1:] if r}


def money(v):
    return g(v) if isinstance(v, float) else g(float(str(v).replace(",", "."))) if v not in (None, "") else 0


def serial_day(v):
    return (date(1899, 12, 30) + timedelta(days=int(v))).isoformat() if isinstance(v, float) else None


def report(path):
    s, d = api("GET", path, None, "boss")
    if s != 200:
        raise SystemExit(f"{path}: {s} {J(d)}")
    return d


def journal(q):
    """XLSX по-польски за период q: (книга, строки «Wszystkie operacje»)."""
    book, _ = xlsx(raw(q)[3])
    return book, table(book.get("Wszystkie operacje") or EMPTY)


def vat_steady(q):
    """Отчёт VAT и выгрузка с одного среза: отчёт до файла и после совпал (время с кошелька не тикнуло между ними)."""
    key = lambda v: J([v.get("rates"), v.get("total"), v.get("methods"), v.get("days")], 10 ** 7)
    for _ in range(3):
        v1 = report(f"/admin/reports/vat?{q}")
        book, ops = journal(q)
        v2 = report(f"/admin/reports/vat?{q}")
        if key(v1) == key(v2):
            break
    return v2, book, ops


# ── сверки ───────────────────────────────────────────────────────────────────────────────────────
LIVE = ("gotówka", "karta", "BLIK")


def by(ops, kinds, pays=None, skip=()):
    return sum(money(o["Kwota brutto"]) for o in ops if o["Operacja"] in kinds and (pays is None or o["Forma płatności"] in pays)
               and o["Forma płatności"] not in skip)


def vs_profit(ops, q):
    """Суммы журнала по видам и «Прибыль» за тот же период, в грошах."""
    P = report(f"/admin/reports/profit?{q}")
    inc, exp = P.get("income") or {}, P.get("expenses") or {}
    got = {"deposits": by(ops, {"Doładowanie portfela"}), "goods": by(ops, {"Sprzedaż", "Kuchnia"}, skip=("portfel",)),
           "packs": by(ops, {"Pakiet czasu"}, skip=("portfel",)),
           "returns": -by(ops, {"Anulowanie doładowania", "Anulowanie sprzedaży", "Anulowanie pakietu"}, LIVE),
           "expenses": -by(ops, {"Wydatek"}), "purchases": -by(ops, {"Zakup towaru"})}
    want = {"deposits": g(inc.get("deposits_pln")), "goods": g(inc.get("goods_pln")), "packs": g(inc.get("packs_pln")),
            "returns": g((P.get("returns") or {}).get("pln")), "expenses": g(exp.get("pln")), "purchases": g(exp.get("purchases_pln"))}
    return got, want


def vs_book(ops, q):
    """Σ «Kasa» журнала и движение ящика по кассовой книге (пришло − ушло ± коррекции), в грошах."""
    B = report(f"/admin/cash/book?{q}").get("totals") or {}
    return sum(money(o["Kasa"]) for o in ops), g(B.get("in_pln")) - g(B.get("out_pln")) + g(B.get("adjust_pln"))


def taxed_by_rate(ops):
    out = {}
    for o in ops:
        if o["Sprzedaż opodatkowana"] == "tak":
            out[o["Stawka VAT"]] = out.get(o["Stawka VAT"], 0) + money(o["Kwota brutto"])
    return {k: v for k, v in out.items() if v}


def vat_by_rate(v):
    return {x["label"].replace(" ", ""): g(x["gross_pln"]) for x in v.get("rates") or [] if g(x["gross_pln"])}


PAYS = {"cash": "gotówka", "card": "karta", "blik": "BLIK", "wallet": "portfel", "comp": "gratis"}


def days_vs_vat(book, V):
    """«Sprzedaż wg dni i stawek» против отчёта VAT: строка «Razem» — ставки (брутто, нетто, VAT), итоги и брутто по
    способам; строки дней — дни отчёта, где есть ставки, с тем же брутто и VAT дня."""
    sh = book.get("Sprzedaż wg dni i stawek") or EMPTY
    head, body = sh["rows"][0], sh["rows"][1:]
    tot = dict(zip(head, body[-1])) if body else {}
    bad = []
    for x in V.get("rates") or []:
        l = x["label"].replace(" ", "")
        if [money(tot.get(f"{k} {l}")) for k in ("Brutto", "Netto", "VAT")] != [g(x["gross_pln"]), g(x["net_pln"]), g(x["vat_pln"])]:
            bad.append(l)
    for m, v in (V.get("methods") or {}).items():
        if money(tot.get(f"Brutto: {PAYS.get(m, m)}")) != g(v):
            bad.append(m)
    t = V.get("total") or {}
    if tot.get("Dzień") != "Razem" or [money(tot.get(k)) for k in ("Razem brutto", "Razem netto", "Razem VAT")] != [g(t.get(k)) for k in ("gross_pln", "net_pln", "vat_pln")]:
        bad.append("Razem")
    sheet_days = {}
    for r in body[:-1]:
        x = dict(zip(head, r))
        sheet_days[serial_day(x.get("Dzień"))] = (money(x.get("Razem brutto")), money(x.get("Razem VAT")))
    api_days = {d["date"]: (g(d["gross_pln"]), g(d["vat_pln"])) for d in V.get("days") or [] if d.get("rates")}
    if sheet_days != api_days:
        bad.append({"лист": sheet_days, "отчёт": api_days})
    return not bad and sh["frozen"] and bool(body), bad


section("права и запрос")
sa = api("GET", f"/admin/export?{DAY}", None, "admin1", raw=True)[0]
sf, df = api("GET", f"/admin/export?{DAY}&format=pdf", None, "boss")
sl, dl = api("GET", f"/admin/export?{DAY}&lang=de", None, "boss")
sm = api("GET", "/admin/export?month=2026-1", None, "boss")[0]
sy, dy = api("GET", f"/admin/export?from=2025-01-01&to={D}", None, "boss")
check("выгрузка — только владельцу (админу 403); формат не xlsx/csv — 400 bad_format, язык не pl/ru — 400 bad_lang, месяц не "
      "«ГГГГ-ММ» — 400, длиннее года — 400 too_long", sa == 403 and sf == 400 and df.get("code") == "bad_format" and sl == 400
      and dl.get("code") == "bad_lang" and sm == 400 and sy == 400 and dy.get("code") == "too_long", J([sa, sf, df, sl, dl, sm, sy, dy]))

section("операции сегодня: пополнение, время, продажи, кухня, пакет, подарок, расходы, касса")
cats = {c.get("code"): c.get("id") for c in report("/admin/expense-categories").get("categories", [])}
new_good = lambda name, price, vat: (api("POST", "/admin/goods", {"name": name, "category": MARK, "price_pln": price, "stock": 20, "vat_pct": vat}, "boss")[1].get("good") or {}).get("id")
A, K = new_good(f"{MARK} Кола", 6, 23), new_good(f"{MARK} Кухня", 10, 8)
sql(f"UPDATE stock_lots SET price_grosz = 250 WHERE good_id = '{A}'")  # себестоимость «Колы» — 2,50 zł за штуку
sql(f"UPDATE users SET coin_minutes = 0 WHERE id = '{G}'")  # сессия идёт с кошелька
ensure_wallet(NICK, 20)  # картой: после сессии в кошельке хватит, чтобы отмена пополнения вернула все 60 наличными
dep = topup(NICK, 60, "cash")
made["deps"].append(dep)
sid = seat(NICK, PC, minutes_ago=30)
end(sid)
sell = lambda method, qty=1, **kw: api("POST", "/admin/sales", {"good_id": A, "qty": qty, "method": method, **kw}, "boss")
s_cash, s_card = sell("cash"), sell("card", 2)
s_back = sell("cash")
gift = sell("comp", comp_reason=f"{MARK}: проверка выгрузки")[0]
kit = api("POST", "/admin/kitchen/orders", {"guest_id": G, "good_id": K, "qty": 1, "method": "card"}, "admin1")
pk = (api("POST", f"/admin/users/{G}/packages", {"package_id": sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1"),
                                                  "method": "card"}, "boss")[1].get("package") or {}).get("id")
vs = api("POST", f"/admin/sales/{(s_back[1].get('sale') or {}).get('id')}/void", {"refund_method": "cash"}, "boss")[0]
vp = api("POST", f"/admin/user-packages/{pk}/void", {"refund_method": "card"}, "boss")[0]
ex1 = api("POST", "/admin/expenses", {"op_date": D, "category_id": cats.get("other"), "amount_pln": 12.30, "vat_pln": 2.30, "method": "till",
                                     "doc_no": f"{MARK}-1", "note": f"{MARK} проверка: из кассы"}, "boss")
ex2 = api("POST", "/admin/expenses", {"op_date": D, "category_id": cats.get("equipment") or cats.get("other"), "amount_pln": 5, "method": "owner",
                                     "note": f"{MARK} проверка: личными"}, "boss")
rp = api("POST", "/admin/owner-repayments", {"amount_pln": 5, "method": "till", "note": f"{MARK} проверка: вернули"}, "boss")
collect = {"kind": "collect", "amount_pln": 1, "dest": "bank", "note": f"{MARK} проверка: инкассация"}
ce = api("POST", "/admin/cash/events", collect, "boss")
if ce[0] == 409:  # ящика на стенде ещё нет — заводим, как кассовые проверки З8
    hire_temp("z13ex")
    tok("z13ex")
    drawer_ready("z13ex")
    ce = api("POST", "/admin/cash/events", collect, "boss")
vd = api("POST", f"/admin/deposits/{dep}/void", {"refund_method": "cash"}, "boss")[0]
check("операции записались: пополнение, сессия, продажи, подарок, кухня, пакет, отмены, расходы, возврат владельцу, инкассация, "
      "отмена пополнения", A and K and dep and s_cash[0] == 201 and s_card[0] == 201 and s_back[0] == 201 and gift == 201 and kit[0] == 201 and pk
      and vs == 200 and vp == 200 and ex1[0] == 201 and ex2[0] == 201 and rp[0] == 201 and ce[0] == 201 and vd == 200,
      J([A, K, s_cash[0], s_card[0], s_back[0], gift, kit[0], pk, vs, vp, ex1, ex2[0], rp, ce, vd], 900))

section("XLSX по-польски: листы, шапки, «O eksporcie»")
st, ctype, disp, data = raw()
book, styles = xlsx(data)
OPS = book.get("Wszystkie operacje") or EMPTY
ops = table(OPS)
check("файл XLSX «1448-eksport-<день>_<день>.xlsx»: три листа — «O eksporcie», «Wszystkie operacje», «Sprzedaż wg dni i stawek»",
      st == 200 and ctype.startswith("application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
      and f'filename="1448-eksport-{D}_{D}.xlsx"' in disp and list(book) == ["O eksporcie", "Wszystkie operacje", "Sprzedaż wg dni i stawek"],
      J([st, ctype, disp, list(book)]))
HEAD = ["Lp.", "Dzień", "Data i godzina", "Operacja", "Forma płatności", "Klient lub kontrahent", "Dokument", "Opis", "Ilość", "Kwota brutto",
        "Sprzedaż opodatkowana", "Stawka VAT", "Netto", "VAT", "Kasa", "Kto", "Uwagi", "ID operacji"]
check("«Wszystkie operacje»: 18 колонок с польской шапкой, шапка закреплена, фильтр на всю таблицу; № по порядку, клубный день — "
      "сегодня", OPS["rows"][0] == HEAD and OPS["frozen"] and OPS["filter"] == f"A1:R{len(ops) + 1}" and ops
      and [o["Lp."] for o in ops] == [float(i + 1) for i in range(len(ops))] and all(serial_day(o["Dzień"]) == D for o in ops),
      J([OPS["rows"][0], OPS["filter"], len(ops)]))
kinds = {o["Operacja"] for o in ops}
want_kinds = {"Doładowanie portfela", "Anulowanie doładowania", "Sprzedaż", "Kuchnia", "Anulowanie sprzedaży", "Pakiet czasu", "Anulowanie pakietu",
              "Czas gry z portfela", "Nieodpłatne przekazanie towaru", "Wydatek", "Wypłata z kasy (inkaso)", "Zwrot właścicielowi"}
check("в журнале — все виды сегодняшних операций", ops and want_kinds <= kinds, J(sorted(want_kinds - kinds)))
about = pairs(book.get("O eksporcie") or EMPTY)
check("«O eksporcie»: клуб, период, клубные сутки, плательщик VAT, «przy wykorzystaniu — bon różnego przeznaczenia», ставка "
      "пополнения 23%, подарки «nie», формат, число строк журнала",
      about.get("Okres") == f"{DMY} — {DMY}" and about.get("Klub") and "dnia następnego" in (about.get("Doba klubowa") or "")
      and about.get("Podatnik VAT") == "tak — czynny podatnik VAT" and about.get("VAT od doładowań portfela") == "przy wykorzystaniu — bon różnego przeznaczenia"
      and about.get("Stawka VAT doładowań") == "23%" and about.get("Nieodpłatne przekazanie towarów w VAT") == "nie" and about.get("Format") == "XLSX"
      and f"wierszy: {len(ops)}" in (about.get("Arkusze") or ""), J(about, 1500))

section("строки журнала: расход, возврат владельцу, инкассация, подарок, время")
e1 = next((o for o in ops if o["Dokument"] == f"{MARK}-1"), {})
check("расход «из кассы» 12,30 с VAT 2,30: −12,30, нетто −10,00, VAT −2,30, «Kasa» −12,30; «z kasy», «—» в «облагается», "
      "ставки нет", money(e1.get("Kwota brutto")) == -1230 and money(e1.get("Netto")) == -1000 and money(e1.get("VAT")) == -230
      and money(e1.get("Kasa")) == -1230 and e1.get("Stawka VAT") is None and e1.get("Sprzedaż opodatkowana") == "—"
      and e1.get("Forma płatności") == "z kasy" and (e1.get("Opis") or "").endswith(f"{MARK} проверка: из кассы"), J(e1, 600))
mine = lambda kind: next((o for o in ops if o["Operacja"] == kind and MARK in (o["Uwagi"] or "") + (o["Opis"] or "")), {})
r1, c1, gf = mine("Zwrot właścicielowi"), mine("Wypłata z kasy (inkaso)"), mine("Nieodpłatne przekazanie towaru")
check("возврат владельцу из кассы — −5,00 и «Kasa» −5,00; инкассация в банк — сумма 0 (деньги клуба не потрачены), «Kasa» −1,00, "
      "«do banku» в примечании", money(r1.get("Kwota brutto")) == -500 and money(r1.get("Kasa")) == -500
      and money(c1.get("Kwota brutto")) == 0 and money(c1.get("Kasa")) == -100 and c1.get("Forma płatności") == "z kasy"
      and "do banku" in (c1.get("Uwagi") or ""), J([r1, c1], 900))
check("подарок «z13e Кола»: сумма — себестоимость 2,50, в примечании — причина и «cena sprzedaży 6,00 zł»; «nie» в «облагается» "
      "при подарках без VAT", money(gf.get("Kwota brutto")) == 250 and "cena sprzedaży 6,00 zł" in (gf.get("Uwagi") or "")
      and f"{MARK}: проверка выгрузки" in (gf.get("Uwagi") or "") and gf.get("Sprzedaż opodatkowana") == "nie" and gf.get("Forma płatności") == "gratis",
      J(gf, 600))
tm = next((o for o in ops if o["Operacja"] == "Czas gry z portfela" and o["Klient lub kontrahent"] == NICK and o["Dokument"] == PC), {})
spent = int(sql(f"SELECT COALESCE(-SUM(amount_grosz), 0) FROM wallet_transactions WHERE ref_id = '{sid}' AND kind = 'session_spend'") or 0)
check(f"время с кошелька — строкой на сессию: гость, {PC}, списано как в сессии, «tak» при трате, ставка зоны 23%, «sesja od …»",
      spent > 0 and money(tm.get("Kwota brutto")) == spent and tm.get("Sprzedaż opodatkowana") == "tak" and tm.get("Stawka VAT") == "23%"
      and (tm.get("Uwagi") or "").startswith("sesja od "), J([spent, tm], 600))

section("сверки за сегодня: «Прибыль», кассовая книга, «Продажи по ставкам VAT»")
got, want = vs_profit(ops, DAY)
check("суммы журнала = «Прибыль» за тот же день: пополнения, товары и кухня и пакеты (кроме кошелька), возвраты живыми деньгами, "
      "расходы и закупки — до гроша", ops and got == want and want["expenses"] > 0, J({"журнал": got, "прибыль": want}))
kasa, drawer = vs_book(ops, DAY)
check("колонка «Kasa» в сумме = движение ящика за день по кассовой книге (пришло − ушло ± коррекции)", ops and kasa == drawer, J([kasa, drawer]))
for moment, gifts, name in ((0, 0, "при трате"), (1, 0, "при пополнении"), (0, 1, "при трате, подарки в VAT")):
    put("acc_vat_moment", moment)
    put("acc_gift_vat", gifts)
    V, bk, jt = vat_steady(DAY)
    ab, t = pairs(bk.get("O eksporcie") or EMPTY), V.get("total") or {}
    check(f"«{name}»: облагаемое журнала по ставкам = «Продажи по ставкам VAT» до гроша; «Sprzedaż brutto» и «VAT należny» в "
          "«O eksporcie» — итоги отчёта", vat_by_rate(V) and taxed_by_rate(jt) == vat_by_rate(V)
          and money(ab.get("Sprzedaż brutto")) == g(t.get("gross_pln")) and money(ab.get("VAT należny")) == g(t.get("vat_pln")),
          J({"журнал": taxed_by_rate(jt), "отчёт": vat_by_rate(V), "итог": t, "лист": [ab.get("Sprzedaż brutto"), ab.get("VAT należny")]}, 900))
    if moment == 0 and gifts == 0:
        ok, bad = days_vs_vat(bk, V)
        check("«Sprzedaż wg dni i stawek»: строка дня и «Razem» — как «Продажи по ставкам VAT»: брутто, нетто и VAT по ставкам, "
              "итоги, брутто по способам", ok, J(bad, 900))
put("acc_vat_moment", 0)
put("acc_gift_vat", 0)

section("сверки за месяц")
V, mb, mops = vat_steady(MONTH)
got, want = vs_profit(mops, MONTH)
check(f"месяц {D[:7]}: суммы журнала = «Прибыль» месяца по всем видам — до гроша", mops and got == want, J({"журнал": got, "прибыль": want, "строк": len(mops)}))
kasa, drawer = vs_book(mops, MONTH)
check("месяц: «Kasa» = движение ящика по кассовой книге за месяц", mops and kasa == drawer, J([kasa, drawer]))
check("месяц: облагаемое журнала по ставкам = «Продажи по ставкам VAT» месяца", vat_by_rate(V) and taxed_by_rate(mops) == vat_by_rate(V),
      J({"журнал": taxed_by_rate(mops), "отчёт": vat_by_rate(V)}, 600))
ok, bad = days_vs_vat(mb, V)
check("месяц: «Sprzedaż wg dni i stawek» — день в день как отчёт VAT, «Razem» — его итоги", ok, J(bad, 900))

section("CSV: по-русски, UTF-8 с BOM, «;», «,»; Windows-1250 по-польски, «,», «.», ГГГГ-ММ-ДД")
st, ctype, disp, data = raw(fmt="csv", lang="ru")
z = unzip(data)
files = z.namelist() if z else []
body = z.read(files[1]) if len(files) > 1 else b""
crows = list(csv.reader(io.StringIO(body.decode("utf-8-sig")), delimiter=";"))
ch = crows[0] if crows else []
cops = [dict(zip(ch, r)) for r in crows[1:]]
check("архив CSV «1448-vygruzka-<день>_<день>.zip»: три файла по листам, UTF-8 с BOM, строки через CRLF, «;», русская шапка",
      st == 200 and ctype == "application/zip" and f'filename="1448-vygruzka-{D}_{D}.zip"' in disp
      and files == ["01_o_vygruzke.csv", "02_vse_operacii.csv", "03_prodazhi_po_dnyam.csv"] and body.startswith(b"\xef\xbb\xbf") and b"\r\n" in body
      and ch[:4] == ["№", "Клубный день", "Время", "Операция"] and ch[9:11] == ["Сумма брутто", "Облагается VAT"], J([st, ctype, disp, files, ch]))
csum = lambda names: sum(money(o["Сумма брутто"]) for o in cops if o["Операция"] in names)
check("CSV по-русски — те же строки и суммы, что XLSX: даты ДД.ММ.ГГГГ, деньги с запятой",
      cops and len(cops) == len(ops) and csum({"Пополнение кошелька"}) == by(ops, {"Doładowanie portfela"}) and csum({"Расход"}) == by(ops, {"Wydatek"})
      and all(o["Клубный день"] == DMY for o in cops) and any("," in o["Сумма брутто"] for o in cops),
      J([len(cops), len(ops), csum({"Пополнение кошелька"}), csum({"Расход"})]))
put("acc_csv_enc", 1)
put("acc_csv_sep", 1)
put("acc_csv_decimal", 1)
put("acc_date_fmt", 1)
st, ctype, disp, data = raw(fmt="csv", lang="pl")
z = unzip(data)
body = z.read(z.namelist()[1]) if z and len(z.namelist()) > 1 else b""
wrows = list(csv.reader(io.StringIO(body.decode("cp1250")), delimiter=","))
wops = [dict(zip(wrows[0], r)) for r in wrows[1:]] if wrows else []
cola = next((o for o in wops if o["Operacja"] == "Sprzedaż" and o["Opis"].startswith(MARK)), {})
check("Windows-1250: без BOM, польские буквы своими байтами («Dzień», «Ilość»), «,» между полями, деньги с точкой, даты "
      "ГГГГ-ММ-ДД, кириллица латиницей («z13e Кола» → «z13e Kola»)",
      body and not body.startswith(b"\xef\xbb\xbf") and b"Dzie\xf1" in body and b"Ilo\x9c\xe6" in body and wrows[0][1] == "Dzień"
      and cola.get("Opis") == f"{MARK} Kola" and cola.get("Dzień") == D and "." in cola.get("Kwota brutto", "") and len(wops) == len(ops),
      J([wrows[0] if wrows else None, cola]))
_, styles = xlsx(raw()[3])
check("ГГГГ-ММ-ДД — и в XLSX: формат ячеек дат «yyyy-mm-dd»", 'formatCode="yyyy-mm-dd"' in styles and 'formatCode="dd.mm.yyyy"' not in styles, styles[:300])
for k in ("acc_csv_enc", "acc_csv_sep", "acc_csv_decimal", "acc_date_fmt"):
    sql(f"DELETE FROM settings WHERE key = '{k}'")
put("acc_csv_sep", 9)  # такую строку PUT не пропустит — только руками в базе
st, _, _, data = raw(fmt="csv", lang="ru")
z = unzip(data)
head9 = z.read(z.namelist()[1]).decode("utf-8-sig").split("\r\n")[0] if z else ""
sql("DELETE FROM settings WHERE key = 'acc_csv_sep'")
check("разделитель не из списка (строка в базе руками) — файл как по умолчанию, с «;», а не ошибка сервера",
      st == 200 and head9.startswith("№;Клубный день;"), J([st, head9[:60]]))

section("журнал действий")
mine_ = rows(f"SELECT details FROM admin_actions WHERE action = 'export' AND created_at >= '{T0}' ORDER BY created_at")
adm = audit("admin1", kind="export", limit=5)
own = audit(kind="export", limit=5)
check("каждая выгрузка — строкой в журнале владельца «<период> · <формат> · <язык> · операций: N»; админ их не видит",
      f"{DMY} — {DMY} · XLSX · PL · операций: {len(ops)}" in mine_ and f"{DMY} — {DMY} · CSV · RU · операций: {len(ops)}" in mine_
      and not adm and own and own[0].get("kind") == "export" and own[0].get("text") == mine_[-1], J([mine_[-3:], adm, own[:1]], 600))

section("уборка")
gone()
check("после уборки «должно быть в ящике» — как до проверки: наличные туда и обратно сошлись", drawer_now() == CASH0, J([CASH0, drawer_now()]))
done()
