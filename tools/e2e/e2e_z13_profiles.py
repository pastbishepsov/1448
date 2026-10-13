#!/usr/bin/env python3
"""З13-и6 «Профили учёта» (№76; решения основателя 04.10, З-Р37). Форма учёта — настройка владельца (KPiR по умолчанию,
ryczałt, полная бухгалтерия), и профиль решает, какие листы идут сразу после «O eksporcie» и какие в них колонки:
KPiR — «Zapisy do KPiR» (выручка дня в колонке 9, расход — в колонке своей категории: 12, 13, 14, 15; категория «не в
книгу» — вне листа и строкой в «O eksporcie»), ryczałt — «Ewidencja przychodów» (выручка дня по ставкам: время — по
ставке времени из настроек, товары — по ставке позиции), полная — «Rejestr sprzedaży VAT», «Rejestr zakupów VAT» (со
счётом категории) и «Raport kasowy»; освобождённому от VAT — без колонок VAT. Выручка — тем же моментом, что VAT: KPiR и
ewidencja за день — нетто отчёта «Продажи по ставкам VAT» до гроша (освобождённому — брутто), реестр продаж с подарками
— день в день «Sprzedaż wg dni i stawek». Миграция 084: у категорий — колонка KPiR и счёт (стандартные — по умолчанию), у
позиций — ставка ryczałtu (3 %). Ручки: ставки ryczałtu и колонки KPiR, ставка ryczałtu категории позиций, колонка и счёт
категории, ставка позиции, настройки формы и ставки времени, реквизиты фирмы (NIP с контрольной суммой) — в «O
eksporcie»; всё — только владельцу.
Данные: гость Guest43 — кошелёк докладывается картой до 20 zł, сессия за PC-06 на полчаса назад с кошелька; позиции «z13p
Piwo» (23 %, ryczałt 2 % — только у неё, чтобы колонка 2 % была ровно её) и «z13p Chipsy» (23 %, ryczałt по умолчанию) —
продажи картой и подарок чипсов (себестоимость партии — 1,23 zł); расходы банком с заметкой «z13p…»: закупка товаров
12,30 (VAT 2,30, контрагент «z13p Dostawca», NIP 111-222-33-32), зарплата 100 zł (не оплачена), «z13p Доставка» 6,15
(VAT 1,15; колонка 13, счёт 402-05), «z13p Вне книги» 5 zł (колонка 0), аренда 24,60 (VAT 4,60) и сторнированный
расход 1,23. В уборке — продажи отменяются («не возвращали»), расходы сторнируются, позиции удаляются `drop_good`,
категории «z13p …» скрываются (временные для проверки «по умолчанию» — удаляются SQL), настройки и реквизиты
возвращаются."""
import csv, io, urllib.error, urllib.request, xml.etree.ElementTree as ET, zipfile
from datetime import date, timedelta

from _z import *

MARK = "z13p"
NICK, PC = "Guest43", "PC-06"
G = uid(NICK)
NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
RID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"
KEYS = ("acc_form", "acc_ryczalt_time", "acc_vat_payer", "acc_vat_moment", "acc_gift_vat", "acc_topup_vat",
        "acc_firm_name", "acc_firm_nip", "acc_firm_address")
ORIG = {k: (rows(f"SELECT value FROM settings WHERE key = '{k}'") or [None])[0] for k in KEYS}
CP_NAME, CP_NIP = f"{MARK} Dostawca", "111-222-33-32"
FIRM, FIRM_ADDR = f"{MARK} Jan Nowak Gry", "ul. Firmowa 2, 03-000 Warszawa"
made = {"cats": []}


def put(key, value):
    """Настройка прямо в базе: выгрузка читает её на каждом запросе, а журналу действий шум не нужен."""
    if value is None:
        sql(f"DELETE FROM settings WHERE key = '{key}'")
    else:
        sql(f"INSERT INTO settings (key, value) VALUES ('{key}', '{value}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value")


def gone():
    end_all(NICK)
    for sid in rows(f"SELECT id FROM sales WHERE name LIKE '{MARK} %' AND voided_at IS NULL AND write_off_id IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")  # картой и подарки — ящик не трогают
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{MARK} %'"):
        drop_good(gid)
    for cid in rows(f"SELECT id FROM expense_categories WHERE name LIKE '{MARK} %' AND NOT hidden"):
        api("PATCH", f"/admin/expense-categories/{cid}", {"hidden": True}, "boss")
    for cid in made["cats"]:  # временные — только для проверки «по умолчанию», расходов у них нет
        sql(f"DELETE FROM expense_categories c WHERE c.id = '{cid}' AND NOT EXISTS (SELECT 1 FROM expenses e WHERE e.category_id = c.id)")
    made["cats"] = []
    for k, v in ORIG.items():
        put(k, v)


gone()
at_end(gone)
for k in KEYS:  # с известного начала: KPiR, плательщик VAT, при трате, подарки без VAT, время — 15 %, реквизитов нет
    put(k, None)
g = lambda v: round((v or 0) * 100)
D = club_day(0)
DAY = f"from={D}&to={D}"


# ── файл ─────────────────────────────────────────────────────────────────────────────────────────
def raw(q=DAY, fmt="xlsx", lang="pl"):
    req = urllib.request.Request(f"{API}/admin/export?{q}&format={fmt}&lang={lang}")
    req.add_header("Authorization", "Bearer " + tok("boss"))
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, b""


def unzip(data):
    try:
        return zipfile.ZipFile(io.BytesIO(data))
    except zipfile.BadZipFile:
        return None


def xlsx(data):
    """Книга стандартной библиотекой: {имя листа: [[…]]}; текст — строкой, число — float, нет ячейки — None."""
    z = unzip(data)
    if not z:
        return {}
    wb = ET.fromstring(z.read("xl/workbook.xml"))
    rels = {r.get("Id"): r.get("Target") for r in ET.fromstring(z.read("xl/_rels/workbook.xml.rels"))}
    out = {}
    for sh in wb.find("m:sheets", NS):
        root = ET.fromstring(z.read("xl/" + rels[sh.get(RID)]))
        tbl = []
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
            tbl.append([cells.get(i) for i in range(max(cells) + 1 if cells else 0)])
        out[sh.get("name")] = tbl
    return out


def book(lang="pl"):
    return xlsx(raw(lang=lang)[1])


def table(bk, name):
    """Строки листа словарями по шапке (с итоговыми)."""
    rows_ = bk.get(name) or [[]]
    head = rows_[0]
    return [dict(zip(head, r + [None] * (len(head) - len(r)))) for r in rows_[1:]]


def pairs(bk):
    return {r[0]: (r[1] if len(r) > 1 else None) for r in (bk.get("O eksporcie") or [[]])[1:] if r}


money = lambda v: g(v) if isinstance(v, float) else 0
day_of = lambda v: (date(1899, 12, 30) + timedelta(days=int(v))).isoformat() if isinstance(v, float) else None


def report(path):
    """Ответ ручки владельца; не 200 — пустой ответ и строка в лог: проверки ниже упадут сами, прогон дойдёт до уборки."""
    s, d = api("GET", path, None, "boss")
    if s != 200:
        print(f"   {path}: {s} {J(d)}")
        return {}
    return d


def vat_day():
    """Отчёт «Продажи по ставкам VAT» за сегодня — когда два подряд одинаковы (на стенде идут чужие сессии)."""
    a = report(f"/admin/reports/vat?{DAY}")
    for _ in range(5):
        b = report(f"/admin/reports/vat?{DAY}")
        if b.get("total") == a.get("total"):
            return b
        a = b
    return a


def steady(build):
    """Файл, снятый между двумя одинаковыми отчётами VAT: выручка в файле и в отчёте — за один и тот же момент."""
    for _ in range(5):
        v1 = vat_day()
        bk = build()
        v2 = vat_day()
        if v1.get("total") == v2.get("total"):
            return bk, v2
    return bk, v2


section("справочники и права")
RR, KC = report("/admin/ryczalt/rates"), report("/admin/kpir/columns")
check("ставки ryczałtu с подписями — от 17 % до 2 %, позиция по умолчанию — 3 %, время — 15 %; колонки KPiR — 12, 13, 14, 15 "
      "и 0 «не в книгу»; закупка по умолчанию — 12, остальное — 15",
      [r["rate"] for r in RR.get("rates", [])] == [170, 150, 140, 125, 120, 100, 85, 55, 30, 20] and RR["rates"][6]["label"] == "8,5 %"
      and RR.get("default_goods") == 30 and RR.get("default_time") == 150
      and [c["col"] for c in KC.get("columns", [])] == [12, 13, 14, 15, 0] and KC["columns"][0]["name_pl"] == "zakup towarów handlowych i materiałów"
      and KC.get("default_purchase") == 12 and KC.get("default") == 15, J([RR, KC], 900))
deny = [api(m, p, b, "admin1")[0] for m, p, b in (("GET", "/admin/ryczalt/rates", None), ("GET", "/admin/kpir/columns", None),
                                                  ("PUT", "/admin/ryczalt/goods", {"category": MARK, "ryczalt_rate": 30}),
                                                  ("GET", "/admin/export/requisites", None), ("PUT", "/admin/export/requisites", {"nip": ""}))]
check("админу — 403 на всё новое: ставки, колонки, ставка категории позиций, реквизиты", deny == [403] * 5, J(deny))
CATS = {c.get("code"): c for c in report("/admin/expense-categories").get("categories", []) if c.get("code")}
std = {k: (c.get("kpir_col"), c.get("account")) for k, c in CATS.items()}
check("миграция 084: стандартные категории — закупки товаров и кухни в 12 (счета 330 и 310), зарплата в 14 (404), аренда, связь, "
      "ПО, маркетинг в 15 (402), коммуналка — 401, прочее — 409; «ZUS и налоги» и «Оборудование и ремонт» — 15 без счёта",
      std.get("goods") == (12, "330") and std.get("kitchen") == (12, "310") and std.get("salary") == (14, "404")
      and std.get("rent") == (15, "402") and std.get("internet") == (15, "402") and std.get("software") == (15, "402")
      and std.get("marketing") == (15, "402") and std.get("utilities") == (15, "401") and std.get("other") == (15, "409")
      and std.get("zus_tax") == (15, "") and std.get("equipment") == (15, ""), J(std, 900))

section("категории: колонка KPiR и счёт")
stamp = sql("SELECT to_char(now(), 'HH24MISSMS')")
t1 = api("POST", "/admin/expense-categories", {"name": f"{MARK} Закупка {stamp}", "purchase": True}, "boss")
t2 = api("POST", "/admin/expense-categories", {"name": f"{MARK} Услуга {stamp}"}, "boss")
t3 = api("POST", "/admin/expense-categories", {"name": f"{MARK} Явно {stamp}", "kpir_col": 0, "account": " 409-1 "}, "boss")
made["cats"] += [(x[1].get("category") or {}).get("id") for x in (t1, t2, t3)]
c1, c2, c3 = ((x[1].get("category") or {}) for x in (t1, t2, t3))
bad_col = api("POST", "/admin/expense-categories", {"name": f"{MARK} Плохая {stamp}", "kpir_col": 11}, "boss")
check("новая категория без колонки: закупка — 12, остальное — 15, счёт пустой; присланные «0» и « 409-1 » — как есть (без "
      "краевых пробелов); колонка 11 — 400 bad_kpir_col со списком", t1[0] == 201 and c1.get("kpir_col") == 12 and c1.get("account") == ""
      and t2[0] == 201 and c2.get("kpir_col") == 15 and t3[0] == 201 and c3.get("kpir_col") == 0 and c3.get("account") == "409-1"
      and bad_col[0] == 400 and bad_col[1].get("code") == "bad_kpir_col" and "12, 13, 14, 15, 0 — не в книгу" in bad_col[1].get("message", ""),
      J([t1, t2, t3, bad_col], 900))


def category(name, purchase=False):
    s, d = api("POST", "/admin/expense-categories", {"name": name, "purchase": purchase}, "boss")
    cid = (d.get("category") or {}).get("id") if s == 201 else d.get("id")
    api("PATCH", f"/admin/expense-categories/{cid}", {"hidden": False}, "boss")
    return cid


SIDE, OUT = category(f"{MARK} Доставка"), category(f"{MARK} Вне книги")
api("PATCH", f"/admin/expense-categories/{SIDE}", {"kpir_col": 15, "account": ""}, "boss")  # с известного начала
T_CAT = sql("SELECT now()::text")
p1 = api("PATCH", f"/admin/expense-categories/{SIDE}", {"kpir_col": 13, "account": "402-05"}, "boss")
p2 = api("PATCH", f"/admin/expense-categories/{OUT}", {"kpir_col": 0}, "boss")
p3 = api("PATCH", f"/admin/expense-categories/{SIDE}", {"account": "4" * 33}, "boss")
p4 = api("PATCH", f"/admin/expense-categories/{SIDE}", {"kpir_col": 16}, "boss")
log = rows(f"SELECT details FROM admin_actions WHERE action = 'expense_category_update' AND created_at >= '{T_CAT}' ORDER BY created_at")
check("правка категории: колонка 13 и счёт 402-05, «не в книгу» — 0; счёт длиннее 32 — 400 bad_account, колонка 16 — 400; в журнале "
      "владельца — «KPiR 15 → 13; счёт — → 402-05»", p1[0] == 200 and (p1[1].get("category") or {}).get("kpir_col") == 13
      and (p1[1].get("category") or {}).get("account") == "402-05" and p2[0] == 200 and (p2[1].get("category") or {}).get("kpir_col") == 0
      and p3[0] == 400 and p3[1].get("code") == "bad_account" and p4[0] == 400 and p4[1].get("code") == "bad_kpir_col"
      and f"{MARK} Доставка: KPiR 15 → 13; счёт — → 402-05" in log, J([p1, p2, p3, p4, log], 900))

section("позиции: ставка ryczałtu")
new_good = lambda name, cat, price, **kw: api("POST", "/admin/goods", {"name": name, "category": cat, "price_pln": price, "stock": 20, "vat_pct": 23, **kw}, "boss")
gb = api("POST", "/admin/goods", {"name": f"{MARK} Zła", "category": MARK, "price_pln": 1, "ryczalt_rate": 33}, "boss")
pw, ch = new_good(f"{MARK} Piwo", f"{MARK} piwo", 1.23, ryczalt_rate=20), new_good(f"{MARK} Chipsy", MARK, 2.46)
PW, CH = (pw[1].get("good") or {}).get("id"), (ch[1].get("good") or {}).get("id")
check("новая позиция: ставка ryczałtu — присланная (2 %) или 3 % по умолчанию; не из списка (33) — 400 bad_ryczalt со списком",
      pw[0] == 201 and (pw[1].get("good") or {}).get("ryczalt_rate") == 20 and ch[0] == 201 and (ch[1].get("good") or {}).get("ryczalt_rate") == 30
      and gb[0] == 400 and gb[1].get("code") == "bad_ryczalt" and "12,5 %" in gb[1].get("message", ""), J([pw, ch, gb], 900))
T_G = sql("SELECT now()::text")
u1 = api("PATCH", f"/admin/goods/{CH}", {"ryczalt_rate": 85}, "boss")
u2 = api("PATCH", f"/admin/goods/{CH}", {"ryczalt_rate": 86}, "boss")
b1 = api("PUT", "/admin/ryczalt/goods", {"category": MARK, "ryczalt_rate": 30}, "boss")
b2 = api("PUT", "/admin/ryczalt/goods", {"category": MARK, "ryczalt_rate": 7}, "boss")
lst = {x.get("id"): x for x in report("/admin/goods").get("goods", [])}
glog = rows(f"SELECT details FROM admin_actions WHERE action IN ('good_update', 'goods_ryczalt') AND created_at >= '{T_G}' ORDER BY created_at")
check("правка позиции — 8,5 % («ryczałt 3 % → 8,5 %» в журнале), 86 — 400; ставка всей категории «z13p» одной кнопкой — 3 % "
      "(поменялась одна позиция, пиво в своей категории осталось на 2 %), не из списка — 400; список позиций отдаёт ставку",
      u1[0] == 200 and (u1[1].get("good") or {}).get("ryczalt_rate") == 85 and u2[0] == 400 and u2[1].get("code") == "bad_ryczalt"
      and b1[0] == 200 and b1[1].get("changed") == 1 and b1[1].get("total") == 1 and b2[0] == 400 and b2[1].get("code") == "bad_ryczalt"
      and (lst.get(CH) or {}).get("ryczalt_rate") == 30 and (lst.get(PW) or {}).get("ryczalt_rate") == 20
      and f"{MARK} Chipsy: ryczałt 3 % → 8,5 %" in glog and f"категория «{MARK}»: ryczałt → 3 % (1 позиция)" in glog, J([u1, u2, b1, b2, glog], 900))

section("настройки и реквизиты")
s_form = api("PUT", "/admin/settings", {"acc_form": 3}, "boss")
s_ry = api("PUT", "/admin/settings", {"acc_ryczalt_time": 86}, "boss")
s_ok = api("PUT", "/admin/settings", {"acc_ryczalt_time": 85, "acc_form": 1}, "boss")
cur = report("/admin/settings").get("settings", {})
check("настройки: форма учёта 0–2 (3 — 400), ставка времени — только ставка ryczałtu (86 — 400 со списком), 8,5 % и ryczałt "
      "сохраняются", s_form[0] == 400 and s_ry[0] == 400 and "Ставка ryczałtu — одна из" in s_ry[1].get("message", "")
      and s_ok[0] == 200 and cur.get("acc_ryczalt_time") == 85 and cur.get("acc_form") == 1, J([s_form, s_ry, s_ok], 600))
put("acc_ryczalt_time", None)
put("acc_form", None)
R0 = report("/admin/export/requisites")
club = sql("SELECT name || '|' || address FROM clubs WHERE is_active ORDER BY id LIMIT 1").split("|")  # как db.First
r_bad = api("PUT", "/admin/export/requisites", {"nip": "123"}, "boss")
r_sum = api("PUT", "/admin/export/requisites", {"nip": "1112223333"}, "boss")
r_long = api("PUT", "/admin/export/requisites", {"name": "x" * 201}, "boss")
T_R = sql("SELECT now()::text")
r_ok = api("PUT", "/admin/export/requisites", {"name": f"  {FIRM} ", "nip": CP_NIP, "address": FIRM_ADDR}, "boss")
r_same = api("PUT", "/admin/export/requisites", {"name": FIRM}, "boss")
R1 = report("/admin/export/requisites")
own_log, adm_log = audit(kind="requisites", limit=5), audit("admin1", kind="requisites", limit=5)
check("реквизиты: пустые — название и адрес клуба рядом; NIP не из 10 цифр — 400 bad_nip, не сходится сумма — 400 bad_nip_sum, "
      "название длиннее 200 — 400; сохраняются без краевых пробелов, NIP — цифрами; то же ещё раз — ничего не меняет; в журнале — "
      "владельцу, админу — нет", R0.get("name") == "" and R0.get("nip") == "" and [R0.get("club_name"), R0.get("club_address")] == club
      and r_bad[0] == 400 and r_bad[1].get("code") == "bad_nip" and r_sum[0] == 400 and r_sum[1].get("code") == "bad_nip_sum"
      and r_long[0] == 400 and r_long[1].get("code") == "bad_firm_name" and r_ok[0] == 200 and r_ok[1].get("changed") == 3
      and r_same[1].get("changed") == 0 and [R1.get("name"), R1.get("nip"), R1.get("address")] == [FIRM, "1112223332", FIRM_ADDR]
      and own_log and own_log[0].get("text", "").startswith(f"фирма: — → {FIRM}; NIP: — → 1112223332") and not adm_log,
      J([R0, r_bad, r_sum, r_long, r_ok, r_same, R1, own_log[:1], adm_log], 1200))

section("данные дня: сессия, продажи, подарок, расходы")
cps = report("/admin/counterparties").get("counterparties", [])
CP = next((c.get("id") for c in cps if c.get("name") == CP_NAME), None)
if not CP:
    CP = (api("POST", "/admin/counterparties", {"name": CP_NAME, "nip": CP_NIP}, "boss")[1].get("counterparty") or {}).get("id")
sql(f"UPDATE stock_lots SET price_grosz = 123 WHERE good_id = '{CH}'")  # себестоимость чипсов — подарок попадёт в VAT
sql(f"UPDATE users SET coin_minutes = 0 WHERE id = '{G}'")  # сессия идёт с кошелька
ensure_wallet(NICK, 20)
sid = seat(NICK, PC, minutes_ago=30)
end(sid)
sell = lambda gid, method, **kw: api("POST", "/admin/sales", {"good_id": gid, "qty": 1, "method": method, **kw}, "boss")[0]
sales = [sell(PW, "card"), sell(CH, "card"), sell(CH, "comp", comp_reason=f"{MARK}: проверка профилей")]


def expense(code_or_id, amount, vat=0.0, doc="", paid=True, cp=None, note=""):
    cid = (CATS.get(code_or_id) or {}).get("id") or code_or_id
    body = {"op_date": D, "category_id": cid, "amount_pln": amount, "vat_pln": vat, "method": "bank", "paid": paid,
            "doc_no": doc, "note": f"{MARK} {note}"}
    if cp:
        body["counterparty_id"] = cp
    s, d = api("POST", "/admin/expenses", body, "boss")
    return (d.get("expense") or {}).get("id") if s == 201 else None


E = {"goods": expense("goods", 12.30, 2.30, f"{MARK}-1", cp=CP, note="закупка"), "salary": expense("salary", 100, 0, f"{MARK}-2", paid=False, note="зарплата"),
     "side": expense(SIDE, 6.15, 1.15, f"{MARK}-3", note="доставка"), "out": expense(OUT, 5, note="вне книги"),
     "rent": expense("rent", 24.60, 4.60, f"{MARK}-4", note="аренда"), "storno": expense("rent", 1.23, 0.23, f"{MARK}-5", note="сторно")}
st = api("POST", f"/admin/expenses/{E['storno']}/storno", {"reason": "проверка: сторно не в книгу"}, "boss")[0]
check("данные записались: сессия, две продажи картой и подарок, шесть расходов (один сторнирован)", sid and sales == [201] * 3
      and all(E.values()) and st == 200, J([sid, sales, E, st]))

section("KPiR — по умолчанию")
B, V = steady(book)
names, K, AB = list(B), table(B, "Zapisy do KPiR"), pairs(B)
by_id = {r.get("ID operacji"): r for r in K}
rev = [r for r in K if day_of(r.get("2 · Data zdarzenia gospodarczego")) == D and (r.get("8 · Opis zdarzenia gospodarczego") or "").startswith("Sprzedaż dnia")]
NET = g((V.get("total") or {}).get("net_pln"))
check("листы: «O eksporcie», «Zapisy do KPiR», «Wszystkie operacje», «Sprzedaż wg dni i stawek» — дальше как было",
      names[:4] == ["O eksporcie", "Zapisy do KPiR", "Wszystkie operacje", "Sprzedaż wg dni i stawek"] and names.count("Raport kasowy") == 1, J(names))
check("выручка дня — одной строкой в колонке 9 (и 11) = нетто «Продаж по ставкам VAT» за день до гроша; колонки 10 нет в строке",
      len(rev) == 1 and money(rev[0].get("9 · Wartość sprzedanych towarów i usług")) == NET > 0
      and money(rev[0].get("11 · Razem przychód")) == NET and rev[0].get("10 · Pozostałe przychody") is None, J([rev, V.get("total")], 900))
K12, K13, K14, K15, K16 = ("12 · Zakup towarów handlowych i materiałów", "13 · Koszty uboczne zakupu", "14 · Wynagrodzenia w gotówce i w naturze",
                           "15 · Pozostałe wydatki", "16 · Razem wydatki")
eg, es, ed, er = (by_id.get(E[k], {}) for k in ("goods", "salary", "side", "rent"))
check("расходы — нетто в колонке своей категории: закупка 10,00 в 12 (документ, NIP и контрагент), зарплата 100,00 в 14 и 16 "
      "(«nieopłacony»), доставка 5,00 в 13, аренда 20,00 в 15 и 16; «вне книги» и сторнированный — нет",
      money(eg.get(K12)) == 1000 and eg.get(K16) is None and eg.get("4 · Numer dowodu księgowego") == f"{MARK}-1"
      and eg.get("5 · NIP kontrahenta") == "1112223332" and eg.get("6 · Kontrahent") == CP_NAME
      and money(es.get(K14)) == 10000 and money(es.get(K16)) == 10000 and es.get("19 · Uwagi") == "nieopłacony"
      and money(ed.get(K13)) == 500 and money(er.get(K15)) == 2000 and money(er.get(K16)) == 2000
      and E["out"] not in by_id and E["storno"] not in by_id, J([eg, es, ed, er], 1200))
tot = K[-1] if K else {}
data = [r for r in K if isinstance(r.get("Lp."), float)]
sums_ok = all(sum(money(r.get(c)) for r in data) == money(tot.get(c)) for c in ("9 · Wartość sprzedanych towarów i usług", K12, K13, K14, K15, K16))
check("строка «Razem» — суммы колонок 9–16; Lp. по порядку", tot.get("Lp.") == "Razem" and sums_ok
      and [r.get("Lp.") for r in data] == [float(i + 1) for i in range(len(data))], J(tot, 600))
check("«O eksporcie»: фирма, NIP и адрес — реквизиты; профиль KPiR; выручка — «przy wykorzystaniu», нетто; «Poza KPiR» — «z13p Вне "
      "книги» 5,00 zł", AB.get("Firma") == FIRM and AB.get("NIP") == "1112223332" and AB.get("Adres") == FIRM_ADDR and AB.get("Klub") == club[0]
      and AB.get("Profil ewidencji") == "KPiR — podatkowa księga przychodów i rozchodów" and "przy wykorzystaniu" in (AB.get("Przychód") or "")
      and "«Zapisy do KPiR» — wierszy: " in (AB.get("Arkusze") or "") and f"«{MARK} Вне книги» — 1 szt., 5,00 zł" in (AB.get("Poza KPiR") or ""),
      J(AB, 1500))
W = {r.get("ID"): r for r in table(B, "Wydatki")}
check("«Wydatki» — колонка «Kolumna KPiR» после категории: 13 у доставки, «poza KPiR» у «вне книги», 12 у закупки",
      (B.get("Wydatki") or [[]])[0][:3] == ["Data operacji", "Kategoria", "Kolumna KPiR"] and (W.get(E["side"]) or {}).get("Kolumna KPiR") == "13"
      and (W.get(E["out"]) or {}).get("Kolumna KPiR") == "poza KPiR" and (W.get(E["goods"]) or {}).get("Kolumna KPiR") == "12",
      J([(B.get("Wydatki") or [[]])[0][:4], W.get(E["out"])], 600))
z = unzip(raw(fmt="csv", lang="ru")[1])
files = z.namelist() if z else []
kh = list(csv.reader(io.StringIO(z.read(files[1]).decode("utf-8-sig")), delimiter=";"))[0] if len(files) > 1 else []
check("CSV по-русски: второй файл — «02_zapisi_kpir.csv» с русской шапкой «2 · Дата события», «9 · Продажа товаров и услуг»",
      files[:3] == ["01_o_vygruzke.csv", "02_zapisi_kpir.csv", "03_vse_operacii.csv"] and kh[1:2] == ["2 · Дата события"]
      and "9 · Продажа товаров и услуг" in kh, J([files[:4], kh]))

section("освобождённый от VAT — брутто")
put("acc_vat_payer", 0)
B0, V0 = steady(book)
K0 = table(B0, "Zapisy do KPiR")
r0 = next((r for r in K0 if day_of(r.get("2 · Data zdarzenia gospodarczego")) == D and (r.get("8 · Opis zdarzenia gospodarczego") or "").startswith("Sprzedaż dnia")), {})
e0 = next((r for r in K0 if r.get("ID operacji") == E["goods"]), {})
check("освобождённому: выручка дня = брутто отчёта (всё «zw»), закупка — брутто 12,30; в «O eksporcie» — «brutto: klub zwolniony z VAT»",
      money(r0.get("9 · Wartość sprzedanych towarów i usług")) == g((V0.get("total") or {}).get("gross_pln")) > 0 and money(e0.get(K12)) == 1230
      and "brutto: klub zwolniony z VAT" in (pairs(B0).get("Przychód") or ""), J([r0, e0, V0.get("total")], 900))
put("acc_vat_payer", None)

section("ryczałt — ewidencja przychodów")
put("acc_form", 1)
B1, V1 = steady(book)
EW = table(B1, "Ewidencja przychodów")
er1 = next((r for r in EW if day_of(r.get("Data uzyskania przychodu")) == D), {})
rates1 = [h for h in (B1.get("Ewidencja przychodów") or [[]])[0] if h.startswith("Przychód wg stawki")]
check("листы: «O eksporcie», «Ewidencja przychodów», «Wszystkie operacje»; колонки ставок — от большей к меньшей: 15 % (время по "
      "умолчанию), 3 % (чипсы), 2 % (пиво)", list(B1)[:3] == ["O eksporcie", "Ewidencja przychodów", "Wszystkie operacje"]
      and rates1[:1] == ["Przychód wg stawki 15%"] and rates1[-2:] == ["Przychód wg stawki 3%", "Przychód wg stawki 2%"], J([list(B1)[:3], rates1]))
check("день: 2 % — ровно пиво (нетто 1,00 zł), «Ogółem» — нетто отчёта VAT за день, ставки в сумме — «Ogółem»",
      money(er1.get("Przychód wg stawki 2%")) == 100 and money(er1.get("Ogółem przychód")) == g((V1.get("total") or {}).get("net_pln")) > 0
      and sum(money(er1.get(h)) for h in rates1) == money(er1.get("Ogółem przychód")), J([er1, V1.get("total")], 900))
check("«O eksporcie»: ryczałt, ставки — «czas gry i pakiety — 15% (ustawienie)», у позиций — «2% — 1 poz.»",
      pairs(B1).get("Profil ewidencji") == "ryczałt od przychodów ewidencjonowanych"
      and (pairs(B1).get("Stawki ryczałtu") or "").startswith("czas gry i pakiety — 15% (ustawienie)") and "2% — 1 poz." in (pairs(B1).get("Stawki ryczałtu") or ""),
      J(pairs(B1).get("Stawki ryczałtu")))
put("acc_ryczalt_time", 85)
B2, V2 = steady(book)
er2 = next((r for r in table(B2, "Ewidencja przychodów") if day_of(r.get("Data uzyskania przychodu")) == D), {})
rates2 = [h for h in (B2.get("Ewidencja przychodów") or [[]])[0] if h.startswith("Przychód wg stawki")]
same_net = g((V1.get("total") or {}).get("net_pln")) == g((V2.get("total") or {}).get("net_pln"))
check("ставка времени 8,5 %: колонки 15 % больше нет, время ушло в 8,5 % той же суммой; 3 %, 2 % и «Ogółem» — как были",
      "Przychód wg stawki 15%" not in rates2 and (not same_net or (money(er2.get("Przychód wg stawki 8,5%")) == money(er1.get("Przychód wg stawki 15%")) > 0
      and money(er2.get("Przychód wg stawki 2%")) == 100 and money(er2.get("Przychód wg stawki 3%")) == money(er1.get("Przychód wg stawki 3%"))
      and money(er2.get("Ogółem przychód")) == money(er1.get("Ogółem przychód")))), J([rates2, er1, er2], 900))
put("acc_ryczalt_time", None)

section("полная бухгалтерия — реестры")
put("acc_form", 2)
put("acc_gift_vat", 1)
B3, V3 = steady(book)
n3 = list(B3)
SR, PR = table(B3, "Rejestr sprzedaży VAT"), table(B3, "Rejestr zakupów VAT")
check("листы: «O eksporcie», «Rejestr sprzedaży VAT», «Rejestr zakupów VAT», «Raport kasowy», «Wszystkie operacje» — «Raport "
      "kasowy» переехал, второй раз его нет", n3[:5] == ["O eksporcie", "Rejestr sprzedaży VAT", "Rejestr zakupów VAT", "Raport kasowy",
                                                        "Wszystkie operacje"] and n3.count("Raport kasowy") == 1, J(n3))
VD = next((r for r in table(B3, "Sprzedaż wg dni i stawek") if day_of(r.get("Dzień")) == D), {})
today = [r for r in SR if day_of(r.get("Data sprzedaży")) == D]
gift_rows = [r for r in today if (r.get("Opis") or "").startswith("Nieodpłatne przekazanie")]
bad = []
for lbl in [h[len("Netto "):] for h in (B3.get("Rejestr sprzedaży VAT") or [[]])[0] if h.startswith("Netto ")]:
    net = sum(money(r.get(f"Netto {lbl}")) for r in today)
    vat = sum(money(r.get(f"VAT {lbl}")) for r in today)
    if net != money(VD.get(f"Netto {lbl}")) or (lbl not in ("0%",) and vat != money(VD.get(f"VAT {lbl}"))):
        bad.append([lbl, net, vat, VD.get(f"Netto {lbl}"), VD.get(f"VAT {lbl}")])
check("реестр продаж: за день — строка продаж и строка подарков товаров; по каждой ставке нетто и VAT строк в сумме — как "
      "«Sprzedaż wg dni i stawek» до гроша, брутто — тоже", len(today) == 2 and len(gift_rows) == 1 and not bad
      and sum(money(r.get("Brutto")) for r in today) == money(VD.get("Razem brutto")) > 0, J([today, VD, bad], 1200))
pr = {r.get("ID operacji"): r for r in PR}
pg, pd_, pe = pr.get(E["goods"], {}), pr.get(E["side"], {}), pr.get(E["rent"], {})
check("реестр покупок: только расходы с VAT — закупка (счёт 330, нетто 10,00, VAT 2,30, брутто 12,30, NIP), доставка (счёт 402-05), "
      "аренда (402); зарплаты и «вне книги» (без VAT) и сторно — нет",
      pg.get("Konto") == "330" and money(pg.get("Netto")) == 1000 and money(pg.get("VAT")) == 230 and money(pg.get("Brutto")) == 1230
      and pg.get("NIP kontrahenta") == "1112223332" and pd_.get("Konto") == "402-05" and money(pd_.get("VAT")) == 115 and pe.get("Konto") == "402"
      and E["salary"] not in pr and E["out"] not in pr and E["storno"] not in pr, J([pg, pd_, pe], 900))
acc = [int(x) for x in sql("SELECT COUNT(*) FILTER (WHERE account <> '') || '|' || COUNT(*) FROM expense_categories").split("|")]
check("«O eksporcie»: полная бухгалтерия, «Rejestry», «Konta kategorii» — сколько категорий со счётом; «Wydatki» — колонка «Konto»",
      pairs(B3).get("Profil ewidencji") == "pełna księgowość (księgi rachunkowe)" and (pairs(B3).get("Rejestry") or "").startswith("sprzedaż — wg dni i stawek")
      and pairs(B3).get("Konta kategorii") == f"ustawione w {acc[0]} z {acc[1]} kategorii; w pozostałych — puste, konto wybierze księgowy"
      and (B3.get("Wydatki") or [[]])[0][:3] == ["Data operacji", "Kategoria", "Konto"], J([pairs(B3).get("Konta kategorii"), acc]))
put("acc_vat_payer", 0)
B4, V4 = steady(book)
SZ, PZ = table(B4, "Ewidencja sprzedaży"), table(B4, "Zakupy i wydatki")
heads4 = (B4.get("Ewidencja sprzedaży") or [[]])[0] + (B4.get("Zakupy i wydatki") or [[]])[0]
sz = next((r for r in SZ if day_of(r.get("Data sprzedaży")) == D), {})
pz = {r.get("ID operacji"): r for r in PZ}
check("освобождённому: «Ewidencja sprzedaży» и «Zakupy i wydatki» — ни одной колонки VAT; продажи дня — брутто отчёта; в покупках "
      "и расходах — все расходы брутто (зарплата 100,00, «вне книги» 5,00), сторно нет",
      list(B4)[1:4] == ["Ewidencja sprzedaży", "Zakupy i wydatki", "Raport kasowy"] and not [h for h in heads4 if "VAT" in h]
      and money(sz.get("Wartość sprzedaży")) == g((V4.get("total") or {}).get("gross_pln")) > 0
      and money((pz.get(E["salary"]) or {}).get("Kwota brutto")) == 10000 and money((pz.get(E["out"]) or {}).get("Kwota brutto")) == 500
      and (pz.get(E["side"]) or {}).get("Konto") == "402-05" and E["storno"] not in pz, J([list(B4)[:4], heads4, sz], 900))
put("acc_vat_payer", None)
put("acc_gift_vat", None)
put("acc_form", None)

section("уборка")
gone()
left = rows(f"SELECT key FROM settings WHERE key IN ({','.join(repr(k) for k in KEYS)})")
check("после уборки: настройки и реквизиты — как до проверки, позиций «z13p» нет, категории «z13p» скрыты",
      sorted(left) == sorted(k for k, v in ORIG.items() if v is not None) and not rows(f"SELECT id FROM goods WHERE name LIKE '{MARK} %'")
      and not rows(f"SELECT id FROM expense_categories WHERE name LIKE '{MARK} %' AND NOT hidden"), J(left))
done()
