#!/usr/bin/env python3
"""З13-и5 «Листы выгрузки» (№75; решения основателя 04.10, З-Р37). К трём листам З13-и4 выгрузка добавляет десять — на тех
же функциях, что экраны, и каждый лист сходится со своим экраном за тот же период: «Отработано по дням» — с «Отработанной
выручкой» «Обзора» (день и месяц, оценка пакетов до журнала разложена по дням до гроша); «Расходы» — со списком «Расходов»
(сторнированные — с пометкой, вне итога; NIP контрагента); «Закупки» — с ценами поставки в «Закупках»; «Касса» — с
кассовой книгой (остатки дня, наличные за день по видам, возвраты, расходы, возвраты владельцу, инкассации, коррекции,
расхождение пересчётов; номера KP/KW — с начала месяца, с начала года или без номеров, одни и те же в выгрузке дня, месяца и
года); «Сдачи касс» — со сдачами книги; «Возвраты» — с блоком «Возвраты» месяца; «Списанные долги» и «Подарено» — с
«Деньгами»; «Склад на конец периода» — со «Складом сейчас»; «Предоплаты и долги на конец» — с «Обязательствами клуба» и
«Дебиторкой». На прошлый момент (конец вчерашних клубных суток) — откат от «сейчас» по журналам: кошельки — по операциям
кошелька, вчерашний долг — есть, сегодняшние пакет, списанный долг и партии — нет.
Данные: гость Guest44 — кошелёк докладывается картой до 20 zł, пополнение 50 zł наличными (в уборке отменяется с возвратом
наличными), сессия за PC-02 на полчаса назад с кошелька; продажа «в долг» за ПК, списанная владельцем; вторая — «в долг»
со вчерашнего вечера (момент продажи сдвигается SQL-ом); продажи наличными (одна отменяется с возвратом наличными) и
подарок; пакет картой; поставка «z13h Кола» 10 шт у контрагента «z13h Hurtownia» (NIP 111-222-33-32, живёт между
прогонами) с ценами 6 × 2,50 + 4 × 2,80; расход «из кассы» 7,40 zł с VAT 1,38 (документ «z13h-1»), инкассация 2 zł в банк,
коррекция остатка +3 zł; временный z13hx на смене сдаёт кассу с недостачей 1 zł. В уборке — отмены, сторно и отмена
списания; позиция «z13h Кола» удаляется `drop_good` вместе с поставкой; настройки `acc_kpkw` и `acc_form` (проверка идёт на
KPiR по умолчанию — лист профиля вторым, З13-и6) возвращаются."""
import csv, io, urllib.error, urllib.request, xml.etree.ElementTree as ET, zipfile
from datetime import date, timedelta

from _z import *

MARK = "z13h"
NICK, PC = "Guest44", "PC-02"
G = uid(NICK)
CP_NAME, CP_NIP = f"{MARK} Hurtownia", "111-222-33-32"
NS = {"m": "http://schemas.openxmlformats.org/spreadsheetml/2006/main"}
RID = "{http://schemas.openxmlformats.org/officeDocument/2006/relationships}id"
KPKW0 = (rows("SELECT value FROM settings WHERE key = 'acc_kpkw'") or [None])[0]
FORM0 = (rows("SELECT value FROM settings WHERE key = 'acc_form'") or [None])[0]  # З13-и6: проверка — на KPiR по умолчанию
made = {"deps": []}
SHEETS = ["O eksporcie", "Zapisy do KPiR", "Wszystkie operacje", "Sprzedaż wg dni i stawek", "Przychód wykonany wg dni", "Wydatki", "Zakupy towarów",
          "Raport kasowy", "Rozliczenia zmian", "Zwroty", "Należności spisane", "Przekazane nieodpłatnie", "Magazyn na koniec okresu",
          "Stan przedpłat i należności"]
FILES_RU = ["01_o_vygruzke.csv", "02_zapisi_kpir.csv", "03_vse_operacii.csv", "04_prodazhi_po_dnyam.csv", "05_otrabotano_po_dnyam.csv",
            "06_rashody.csv", "07_zakupki.csv", "08_kassa.csv", "09_sdachi_kass.csv", "10_vozvraty.csv", "11_spisannye_dolgi.csv",
            "12_podareno.csv", "13_sklad_na_konec.csv", "14_predoplaty_i_dolgi.csv"]


def kpkw(v, key="acc_kpkw"):
    if v is None:
        sql(f"DELETE FROM settings WHERE key = '{key}'")
    else:
        sql(f"INSERT INTO settings (key, value) VALUES ('{key}', '{v}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value")


def gone():
    end_all(NICK)
    for w in rows(f"SELECT id FROM debt_write_offs WHERE user_id = '{G}' AND cancelled_at IS NULL"):
        api("POST", f"/admin/debt-write-offs/{w}/cancel", None, "boss")
    for r in rows(f"SELECT id, method FROM sales WHERE name LIKE '{MARK} %' AND voided_at IS NULL"):
        sid, method = r.split("|")  # наличные — обратно из ящика
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "cash" if method == "cash" else "none"}, "boss")
    for pid in rows(f"SELECT id FROM user_packages WHERE user_id = '{G}' AND voided_at IS NULL"):
        api("POST", f"/admin/user-packages/{pid}/void", {"refund_method": "none"}, "boss")
    for did in made["deps"]:
        if sql(f"SELECT voided_at IS NULL FROM deposits WHERE id = '{did}'") == "t":
            api("POST", f"/admin/deposits/{did}/void", {"refund_method": "cash"}, "boss")
    for i in rows(f"SELECT id FROM cash_events WHERE note LIKE '{MARK}%' AND voided_at IS NULL"):
        api("POST", f"/admin/cash/events/{i}/void", {}, "boss")
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '%{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{MARK} %'"):
        drop_good(gid)
    kpkw(KPKW0)
    kpkw(FORM0, "acc_form")


gone()
at_end(gone)
kpkw(None)  # по умолчанию — номера с начала месяца
kpkw(None, "acc_form")  # и KPiR: лист профиля — второй (З13-и6)
g = lambda v: round((v or 0) * 100)
D, Y = club_day(0), club_day(-1)
DAY, YDAY, MONTH = f"from={D}&to={D}", f"from={Y}&to={Y}", f"month={D[:7]}"
RH = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '8')"))
T_DAY = f"{D[8:]}.{D[5:7]}.{D[:4]} {RH:02d}:00"  # конец вчерашних клубных суток — «на конец» вчерашнего периода
T_SQL = at_club(D, f"{RH:02d}:00")


# ── файл ─────────────────────────────────────────────────────────────────────────────────────────
def raw(q, fmt="xlsx", lang="pl"):
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


EMPTY = {"rows": [[]]}


def xlsx(data):
    """Книга стандартной библиотекой: {имя листа: {"rows": [[…]]}}; текст — строкой, число — float, нет ячейки — None."""
    z = unzip(data)
    if not z:
        return {}
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
        out[sh.get("name")] = {"rows": table}
    return out


def table(book, name):
    """Строки листа словарями по шапке: (данные, итоговые строки). Итоговые — те, что после последней строки данных
    (первая ячейка — текст «Razem…» / «… — razem…»)."""
    rows_ = (book.get(name) or EMPTY)["rows"]
    head = rows_[0]
    data = [dict(zip(head, r + [None] * (len(head) - len(r)))) for r in rows_[1:]]
    return data


def money(v):
    return g(v) if isinstance(v, float) else 0


def day_of(v):
    return (date(1899, 12, 30) + timedelta(days=int(v))).isoformat() if isinstance(v, float) else None


def report(path):
    s, d = api("GET", path, None, "boss")
    if s != 200:
        raise SystemExit(f"{path}: {s} {J(d)}")
    return d


def book_of(q, fmt="xlsx", lang="pl"):
    return xlsx(raw(q, fmt, lang)[1])


section("данные: поставка, пополнение, сессия, долги, продажи, подарок, пакет, расход, касса, сдача")
cps = report("/admin/counterparties").get("counterparties", [])
cp = next((c.get("id") for c in cps if c.get("name") == CP_NAME), None)
if not cp:
    s_, d_ = api("POST", "/admin/counterparties", {"name": CP_NAME, "nip": CP_NIP}, "boss")
    cp = (d_.get("counterparty") or {}).get("id") or d_.get("id")
cats = {c.get("code"): c.get("id") for c in report("/admin/expense-categories").get("categories", [])}
A = (api("POST", "/admin/goods", {"name": f"{MARK} Кола", "category": MARK, "price_pln": 6, "vat_pct": 23}, "boss")[1].get("good") or {}).get("id")
ss, sd = api("POST", "/admin/supplies", {"lines": [{"good_id": A, "qty": 10}], "note": f"{MARK}: поставка"}, "admin1")
sid = (sd.get("supply") or {}).get("id")
sp = api("PUT", f"/admin/supplies/{sid}/pricing", {"lines": [{"good_id": A, "prices": [{"qty": 6, "price_pln": 2.5}, {"qty": 4, "price_pln": 2.8}]}],
                                                   "counterparty_id": cp, "doc_no": f"FV {MARK}/1", "doc_date": D, "method": "bank", "vat_pln": 4.9}, "boss")
sql(f"UPDATE users SET coin_minutes = 0 WHERE id = '{G}'")  # сессия идёт с кошелька
ensure_wallet(NICK, 20)
dep = topup(NICK, 50, "cash")
made["deps"].append(dep)
sell = lambda method, qty=1, **kw: api("POST", "/admin/sales", {"good_id": A, "qty": qty, "method": method, **kw}, "boss")
sid1 = seat(NICK, PC, minutes_ago=30)
x1 = sell("later", guest_id=G)
end(sid1)
wo = api("POST", f"/admin/users/{G}/debt/write-off", None, "boss")
sid2 = seat(NICK, PC)
x2 = sell("later", guest_id=G)
end(sid2)
X2 = (x2[1].get("sale") or {}).get("id")
sql(f"UPDATE sales SET created_at = {T_SQL} - interval '1 hour' WHERE id = '{X2}'")  # долг со вчерашнего вечера
s_cash, s_back = sell("cash", 2), sell("cash")
gift = sell("comp", comp_reason=f"{MARK}: проверка листов")
pk = (api("POST", f"/admin/users/{G}/packages", {"package_id": sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1"),
                                                  "method": "card"}, "boss")[1].get("package") or {}).get("id")
vs = api("POST", f"/admin/sales/{(s_back[1].get('sale') or {}).get('id')}/void", {"refund_method": "cash"}, "boss")
ex = api("POST", "/admin/expenses", {"op_date": D, "category_id": cats.get("other"), "counterparty_id": cp, "amount_pln": 7.40, "vat_pln": 1.38,
                                    "method": "till", "doc_no": f"{MARK}-1", "note": f"{MARK} проверка: из кассы"}, "boss")
ce = api("POST", "/admin/cash/events", {"kind": "collect", "amount_pln": 2, "dest": "bank", "note": f"{MARK} проверка: инкассация"}, "boss")
if ce[0] == 409:  # ящика на стенде ещё нет — заводим, как кассовые проверки З8
    hire_temp("z13hx")
    tok("z13hx")
    drawer_ready("z13hx")
    ce = api("POST", "/admin/cash/events", {"kind": "collect", "amount_pln": 2, "dest": "bank", "note": f"{MARK} проверка: инкассация"}, "boss")
exp_now = (api("GET", "/admin/cash/book?preset=today", None, "boss")[1].get("now") or {}).get("expected_pln") or 0
cc = api("POST", "/admin/cash/events", {"kind": "correct", "stated_pln": round(exp_now + 3, 2), "note": f"{MARK} проверка: коррекция"}, "boss")
hire_temp("z13hx")
tok("z13hx")
drawer_ready("z13hx")
pre = api("GET", "/admin/work/closeout", None, "z13hx")[1]
co = api("POST", "/admin/work/stop", {"counted_pln": round((pre.get("expected_pln") or 0) - 1, 2)}, "z13hx")
CO = ((co[1] or {}).get("closeout") or co[1] or {}).get("id")
check("данные записались: поставка оценена, пополнение, две сессии, долг списан, долг со вчера, продажи, возврат, подарок, пакет, "
      "расход, инкассация, коррекция, сдача с недостачей 1 zł", A and sid and sp[0] == 200 and dep and x1[0] == 201 and wo[0] in (200, 201)
      and X2 and s_cash[0] == 201 and vs[0] == 200 and gift[0] == 201 and pk and ex[0] == 201 and ce[0] == 201 and cc[0] == 201 and co[0] == 200,
      J([A, sid, sp[0], x1[0], wo, X2, s_cash[0], vs, gift[0], pk, ex[0], ce, cc, co], 900))

section("листы: четырнадцать по порядку — в XLSX и в архиве CSV")
E1 = book_of(DAY)
csv_ru = unzip(raw(DAY, "csv", "ru")[1])
check("XLSX по-польски: «O eksporcie», «Zapisy do KPiR» (профиль З13-и6), «Wszystkie operacje», «Sprzedaż wg dni i stawek» и десять "
      "листов З13-и5 — по порядку",
      list(E1) == SHEETS, J(list(E1)))
check("архив CSV по-русски — четырнадцать файлов по листам, латиницей и по порядку", csv_ru and csv_ru.namelist() == FILES_RU,
      J(csv_ru.namelist() if csv_ru else None))

section("«Отработано по дням» — «Отработанная выручка» «Обзора»")
M_DAY, M_MONTH = report(f"/admin/reports/money?{DAY}"), report(f"/admin/reports/money?{MONTH}")
EARN_KEYS = [("Czas gry z portfela", "wallet_pln"), ("Pakiety — wykorzystane", "packs_pln"), ("Pakiety sprzed ewidencji (szacunek)", "estimate_pln"),
             ("Towary", "goods_pln"), ("Kuchnia", "kitchen_pln"), ("Razem wykonano", "pln")]


def earned_ok(rows_, e):
    tot = rows_[-1] if rows_ else {}
    return (tot.get("Dzień") == "Razem" and all(money(tot.get(k)) == g(e.get(f)) for k, f in EARN_KEYS)
            and tot.get("Czas oddany (monety, rekompensaty), min") == float((e.get("given") or {}).get("minutes") or 0)
            and money(tot.get("Czas oddany, zł")) == g((e.get("given") or {}).get("pln")))


er = table(E1, "Przychód wykonany wg dni")
check("за день: строка дня и «Razem» — как «Обзор» за день: время с кошелька, пакеты, оценка, товары, кухня, итог, отдано временем",
      len(er) == 2 and day_of(er[0].get("Dzień")) == D and earned_ok(er, M_DAY["earned"])
      and all(er[0].get(k) == er[1].get(k) for k, _ in EARN_KEYS), J([er, M_DAY["earned"]], 900))
E2 = book_of(MONTH)
em = table(E2, "Przychód wykonany wg dni")
sums_ok = all(sum(money(r.get(k)) for r in em[:-1]) == money(em[-1].get(k)) for k, _ in EARN_KEYS) if em else False
check("за месяц: «Razem» — как «Обзор» за месяц, дни без дыр и в сумме дают «Razem» до гроша (оценка пакетов до журнала — тоже)",
      earned_ok(em, M_MONTH["earned"]) and sums_ok
      and [day_of(r.get("Dzień")) for r in em[:-1]] == [(date.fromisoformat(D[:8] + "01") + timedelta(days=i)).isoformat()
                                                       for i in range(int(D[8:]))],
      J([em[-1] if em else None, M_MONTH["earned"]], 900))

section("«Расходы» — список «Расходов»")
EXP = report(f"/admin/expenses?{DAY}")
xr = table(E1, "Wydatki")
mine = next((r for r in xr if r.get("ID") == (ex[1].get("expense") or {}).get("id")), {})
tot = xr[-1] if xr else {}
check("строк — сколько расходов за день на экране (со сторнированными), «Razem bez storn» — итоги экрана: брутто, VAT, нетто",
      len(xr) - 1 == len(EXP.get("expenses") or []) and tot.get("Data operacji") == "Razem bez storn"
      and money(tot.get("Kwota brutto")) == g(EXP["totals"]["amount_pln"]) and money(tot.get("VAT")) == g(EXP["totals"]["vat_pln"])
      and money(tot.get("Netto")) == g(EXP["totals"]["net_pln"]), J([len(xr), len(EXP.get("expenses") or []), tot, EXP.get("totals")]))
check("расход «из кассы» 7,40 с VAT 1,38: контрагент с NIP 1112223332, нетто 6,02, «z kasy», оплачен, когда ушёл из кассы; не закупка",
      money(mine.get("Kwota brutto")) == 740 and money(mine.get("VAT")) == 138 and money(mine.get("Netto")) == 602
      and mine.get("Kontrahent") == CP_NAME and mine.get("NIP kontrahenta") == "1112223332" and mine.get("Forma płatności") == "z kasy"
      and mine.get("Opłacony") == "tak" and isinstance(mine.get("Wypłata z kasy"), float) and mine.get("Zakup towaru") == "nie"
      and mine.get("Storno — kiedy") is None, J(mine, 700))
pexp = next((r for r in xr if r.get("ID") == sql(f"SELECT id FROM expenses WHERE supply_id = '{sid}' AND storno_at IS NULL")), {})
check("расход поставки — закупка товара: 26,20 zł (6 × 2,50 + 4 × 2,80), VAT 4,90, банк", money(pexp.get("Kwota brutto")) == 2620
      and pexp.get("Zakup towaru") == "tak" and money(pexp.get("VAT")) == 490, J(pexp, 600))

section("«Закупки» — цены поставки в «Закупках»")
pr = table(E1, "Zakupy towarów")
ours = [r for r in pr if r.get("ID") == sid]
PRC = report(f"/admin/supplies/{sid}/pricing")["supply"]
check("поставка — два куска: 6 × 2,50 = 15,00 и 4 × 2,80 = 11,20; поставщик, документ, дата документа, «wyceniona», банк; сумма "
      "кусков — как у поставки в «Закупках»", [(r.get("Ilość"), money(r.get("Cena za szt. brutto")), money(r.get("Wartość brutto"))) for r in ours]
      == [(6.0, 250, 1500), (4.0, 280, 1120)] and all(r.get("Dostawca") == CP_NAME and r.get("Dokument") == f"FV {MARK}/1"
      and day_of(r.get("Data dokumentu")) == D and r.get("Status") == "wyceniona" for r in ours)
      and sum(money(r.get("Wartość brutto")) for r in ours) == g(PRC.get("total_pln")), J([ours, PRC.get("total_pln")], 900))
ids = sorted({r.get("ID") for r in pr[:-1] if r.get("ID")})
priced = sum(g(report(f"/admin/supplies/{i}/pricing")["supply"].get("total_pln")) for i in ids)
check("«Razem wycenione» — сумма оценённых поставок периода по «Закупкам»", pr and money(pr[-1].get("Wartość brutto")) == priced,
      J([pr[-1] if pr else None, priced]))

section("«Касса» — кассовая книга")
BK = report(f"/admin/cash/book?{DAY}")
bd = (BK.get("days") or [{}])[-1]
kr = table(E1, "Raport kasowy")
docs = [r for r in kr if r.get("Rodzaj") in ("KP", "KW")]
by_text = lambda pfx, kind: sum(money(r.get("Przychód") if kind == "KP" else r.get("Rozchód")) for r in docs
                                if r.get("Rodzaj") == kind and r.get("Treść operacji", "").startswith(pfx))
opening = next((r for r in kr if (r.get("Treść operacji") or "").startswith("Stan kasy na początek dnia")), {})
closing = next((r for r in kr if (r.get("Treść operacji") or "").startswith("Stan kasy na koniec dnia")), {})
got = {"opening": money(opening.get("Stan kasy")), "closing": money(closing.get("Stan kasy")),
       "deposits": by_text("Doładowania portfeli", "KP"), "goods": by_text("Sprzedaż towarów i kuchni", "KP"), "packs": by_text("Pakiety czasu", "KP"),
       "refunds": by_text("Zwrot: ", "KW"), "expenses": by_text("Wydatek: ", "KW"), "owner": by_text("Zwrot właścicielowi", "KW"),
       "collections": by_text("Wypłata z kasy", "KW"), "adjust": by_text("Korekta", "KP") - by_text("Korekta", "KW"),
       "diff": by_text("Różnica kasowa dnia: nadwyżka", "KP") - by_text("Różnica kasowa dnia: niedobór", "KW")}
want = {"opening": g(bd.get("opening_pln")), "closing": g(bd.get("closing_pln")), "deposits": g(bd.get("deposits_pln")), "goods": g(bd.get("goods_pln")),
        "packs": g(bd.get("packs_pln")), "refunds": g(bd.get("refunds_pln")), "expenses": g(bd.get("expenses_pln")), "owner": g(bd.get("owner_pln")),
        "collections": g(bd.get("collections_pln")), "adjust": g(bd.get("adjust_pln")), "diff": g(bd.get("diff_pln"))}
check("день — как в кассовой книге: остатки на начало и конец, наличные за день по видам (KP), возвраты, расходы из кассы, "
      "возвраты владельцу, инкассации (KW), коррекции, расхождение пересчётов", got == want and docs
      and any(f"{MARK} проверка: коррекция" in (r.get("Treść operacji") or "") and r.get("Rodzaj") == "KP" for r in docs)
      and any(f"{MARK} проверка: из кассы" in (r.get("Treść operacji") or "") and r.get("Rodzaj") == "KW" for r in docs),
      J({"лист": got, "книга": want}, 900))
if bd.get("opening_pln") is not None:  # остаток на начало известен — после каждого документа свой остаток
    run, want_bal = g(bd.get("opening_pln")), []
    for r in docs:
        run += money(r.get("Przychód")) - money(r.get("Rozchód"))
        want_bal.append(run)
    got_bal = [money(r.get("Stan kasy")) if isinstance(r.get("Stan kasy"), float) else None for r in docs]
    check("остаток после каждого документа — на начало ± документ; после последнего — остаток на конец",
          docs and got_bal == want_bal and want_bal[-1] == money(closing.get("Stan kasy")), J([got_bal[-3:], want_bal[-3:], closing.get("Stan kasy")]))
else:  # стенд первого дня (З15-и1а): ящика до этих суток не было — считать остатки не от чего
    bl = bd.get("baseline") or {}
    base = next((r for r in kr if (r.get("Treść operacji") or "").startswith("Stan początkowy — pierwsze przeliczenie")), {})
    base_want = (f"Stan początkowy — pierwsze przeliczenie: {bl.get('by')}, "
                 + sql(f"SELECT to_char(timestamptz '{bl.get('at')}' AT TIME ZONE '{TZ}', 'HH24:MI')")) if bl else None
    check("остатка на начало нет (ящик заведён в эти сутки): «Stan kasy na początek dnia: kasa jeszcze nie prowadzona» без суммы, "
          "после документов остатков нет; начальный остаток — первая сдача, как в книге",
          opening.get("Treść operacji") == "Stan kasy na początek dnia: kasa jeszcze nie prowadzona" and opening.get("Stan kasy") is None
          and docs and all(r.get("Stan kasy") is None for r in docs)
          and (not bl or (base.get("Treść operacji") == base_want and money(base.get("Stan kasy")) == g(bl.get("pln")))),
          J([opening, base, bl, [r.get("Stan kasy") for r in docs][-3:]]))
mm = f"{D[5:7]}/{D[:4]}"
mine_docs = {r.get("ID"): r.get("Nr dokumentu") for r in docs if r.get("ID")}
km = table(E2, "Raport kasowy")
mdocs = [r for r in km if r.get("Rodzaj") in ("KP", "KW")]


def consecutive(rows_, kind, fmt):
    nums = [r.get("Nr dokumentu") for r in rows_ if r.get("Rodzaj") == kind]
    return nums == [fmt.format(i + 1) for i in range(len(nums))]


month_ok = consecutive(mdocs, "KP", "KP {}/" + mm) and consecutive(mdocs, "KW", "KW {}/" + mm)
same = all({r.get("ID"): r.get("Nr dokumentu") for r in mdocs if r.get("ID")}.get(i) == n for i, n in mine_docs.items())
day_first = [r.get("Nr dokumentu") for r in docs if r.get("Rodzaj") == "KP"][:1]
m_today = [r.get("Nr dokumentu") for r in mdocs if r.get("Rodzaj") == "KP" and day_of(r.get("Dzień")) == D][:1]
check(f"номера с начала месяца: KP 1/{mm}, KP 2/{mm}… и KW так же — подряд за весь месяц; у выгрузки дня — те же номера, что в "
      "выгрузке месяца", month_ok and same and day_first == m_today and mine_docs, J([day_first, m_today, len(mdocs)]))
kpkw(1)
Ey = book_of(f"from={D[:4]}-01-01&to={D}")
Edy = book_of(DAY)
ydocs = [r for r in table(Ey, "Raport kasowy") if r.get("Rodzaj") in ("KP", "KW")]
dydocs = [r for r in table(Edy, "Raport kasowy") if r.get("Rodzaj") in ("KP", "KW")]
year_ok = consecutive(ydocs, "KP", "KP {}/" + D[:4]) and consecutive(ydocs, "KW", "KW {}/" + D[:4])
ymap = {r.get("ID"): r.get("Nr dokumentu") for r in ydocs if r.get("ID")}
yday_kp = [r.get("Nr dokumentu") for r in ydocs if r.get("Rodzaj") == "KP" and day_of(r.get("Dzień")) == D][:1]
check(f"«с начала года» (acc_kpkw = 1): KP 1/{D[:4]}… подряд с 1 января; у выгрузки дня — те же номера, что в выгрузке с начала года",
      year_ok and all(ymap.get(r.get("ID")) == r.get("Nr dokumentu") for r in dydocs if r.get("ID"))
      and [r.get("Nr dokumentu") for r in dydocs if r.get("Rodzaj") == "KP"][:1] == yday_kp and len(ydocs) > len(dydocs),
      J([yday_kp, [r.get("Nr dokumentu") for r in dydocs][:3], len(ydocs)]))
kpkw(2)
nd = [r for r in table(book_of(DAY), "Raport kasowy") if r.get("Rodzaj") in ("KP", "KW")]
check("«без номеров» (acc_kpkw = 2): документы те же, номеров нет", nd and len(nd) == len(docs) and all(r.get("Nr dokumentu") is None for r in nd),
      J(nd[:2]))
kpkw(None)

section("«Сдачи касс» — сдачи кассовой книги")
zr = table(E1, "Rozliczenia zmian")
bco = bd.get("closeouts") or []
ours = next((r for r in zr if r.get("ID") == CO), {})
bours = next((x for x in bco if x.get("id") == CO), {})
check("строк — сколько сдач за день в книге, у каждой — те же «powinno być», «przeliczono», «różnica»; наша — z13hx, недостача 1,00",
      len(zr) - 1 == len(bco) and all(money(next((r for r in zr if r.get("ID") == x.get("id")), {}).get("Różnica")) == g(x.get("diff_pln"))
      and money(next((r for r in zr if r.get("ID") == x.get("id")), {}).get("Powinno być")) == g(x.get("expected_pln")) for x in bco)
      and ours.get("Kto") == "z13hx" and money(ours.get("Różnica")) == -100 and money(ours.get("Przeliczono")) == g(bours.get("counted_pln"))
      and money(zr[-1].get("Różnica")) == sum(g(x.get("diff_pln")) for x in bco), J([ours, bours], 900))

section("«Возвраты» — блок «Возвраты» месяца")
RT = report(f"/admin/reports/returns?{MONTH}")
rr = table(E2, "Zwroty")
t = RT.get("totals") or {}
back = next((r for r in rr if r.get("ID") == (s_back[1].get("sale") or {}).get("id")), {})
check("за месяц: строк — сколько отмен с деньгами, «вернули» в сумме — живыми и на кошелёк, как блок; наша продажа — 6,00 "
      "наличными, отменил boss", len(rr) - 1 == RT.get("items_total") and money(rr[-1].get("Zwrócono")) == g(t.get("live_pln")) + g(t.get("wallet_pln"))
      and money(back.get("Zwrócono")) == 600 and back.get("Forma zwrotu") == "gotówka" and back.get("Anulował") == "boss"
      and back.get("Rodzaj") == "sprzedaż", J([len(rr), RT.get("items_total"), rr[-1] if rr else None, t, back], 900))

section("«Списанные долги» и «Подарено» — «Деньги»")
wr = table(E1, "Należności spisane")
WO = M_DAY["debts"]["written_off"]
x1r = next((r for r in wr if r.get("ID") == (x1[1].get("sale") or {}).get("id")), {})
check("списанное за день: итог — «Потери → долги»; наша продажа — бар, 6,00, гость, дата продажи и дата списания",
      wr and money(wr[-1].get("Kwota brutto")) == g(WO.get("pln")) and len(wr) - 1 >= WO.get("count", 0) and money(x1r.get("Kwota brutto")) == 600
      and x1r.get("Miejsce") == "bar" and x1r.get("Klient") == NICK and isinstance(x1r.get("Data sprzedaży"), float)
      and isinstance(x1r.get("Data spisania"), float), J([wr[-1] if wr else None, WO.get("pln"), x1r], 700))
gr = table(E1, "Przekazane nieodpłatnie")
GF = M_DAY["gifts"]
gx = next((r for r in gr if r.get("ID") == (gift[1].get("sale") or {}).get("id")), {})
check("подарено за день: строк и сумма по цене — как «Подарено», себестоимость — тоже; наш подарок — 6,00 по цене, 2,50 по партии",
      len(gr) - 1 == GF.get("count") and money(gr[-1].get("Cena sprzedaży")) == g(GF.get("pln")) and money(gr[-1].get("Koszt zakupu")) == g(GF.get("cost_pln"))
      and money(gx.get("Cena sprzedaży")) == 600 and money(gx.get("Koszt zakupu")) == 250 and gx.get("Rodzaj") == "towar", J([gr[-1] if gr else None, GF, gx], 700))

section("на конец периода: сейчас — как «Склад» и «Обязательства клуба»")
sr = table(E1, "Magazyn na koniec okresu")
SV = M_DAY["stock_value"]
about1 = {r[0]: (r[1] if len(r) > 1 else None) for r in (E1.get("O eksporcie") or EMPTY)["rows"][1:] if r}
lots = [r for r in sr if r.get("Towar") == f"{MARK} Кола"]
left = int(sql(f"SELECT COALESCE(SUM(qty_left), 0) FROM stock_lots WHERE good_id = '{A}'"))
check("склад сейчас: штук и стоимость — как «Склад сейчас» в «Деньгах», без цены — столько же штук; наша поставка — куски с ценой и "
      "остатком", sr and sr[-1].get("Ilość, szt.") == float(SV.get("units")) and money(sr[-1].get("Wartość brutto")) == g(SV.get("cost_pln"))
      and f"— {SV.get('no_cost_units')} szt." in (about1.get("Magazyn") or "") and sum(r.get("Ilość, szt.") for r in lots) == left
      and {money(r.get("Cena zakupu za szt. brutto")) for r in lots} <= {250, 280} and lots, J([sr[-1] if sr else None, SV, lots], 900))
orr = table(E1, "Stan przedpłat i należności")
LB = M_DAY["liabilities"]
tot = {r.get("Rodzaj"): r for r in orr if (r.get("Rodzaj") or "").endswith("razem") or "— razem" in (r.get("Rodzaj") or "")}
wal = next((r for r in orr if r.get("Rodzaj") == "portfel (przedpłata)" and r.get("Klient") == NICK), {})
pkr = next((r for r in orr if r.get("ID") == pk), {})
dbt = next((r for r in orr if r.get("Rodzaj") == "należność od klienta" and r.get("Klient") == NICK), {})
mleft = int(sql(f"SELECT minutes_left FROM user_packages WHERE id = '{pk}'") or 0)
check("предоплаты и долги сейчас — как «Обязательства клуба»: кошельки, пакеты (злотые и минуты), долги гостей; гость — его кошелёк, "
      "пакет с остатком минут и долг «bar: 1»", money((tot.get("Portfele — razem") or {}).get("Kwota")) == g(LB["wallets"]["pln"])
      and money((tot.get("Pakiety — razem (nieodpłatne — 0 zł)") or {}).get("Kwota")) == g(LB["packages"]["pln"])
      and (tot.get("Pakiety — razem (nieodpłatne — 0 zł)") or {}).get("Minuty") == float(LB["packages"]["minutes"])
      and money((tot.get("Należności od klientów — razem") or {}).get("Kwota")) == g(LB["receivable"]["pln"])
      and money(wal.get("Kwota")) == wallet(NICK) and pkr.get("Minuty") == float(mleft) and mleft > 0
      and (dbt.get("Opis") or "").startswith("bar: ") and money(dbt.get("Kwota")) >= 600
      and "w chwili eksportu" in (about1.get("Na koniec okresu") or ""),
      J([tot, wal, pkr, dbt, about1.get("Na koniec okresu")], 1200))

section("на конец вчерашних суток — откат по журналам")
E3 = book_of(YDAY)
about3 = {r[0]: (r[1] if len(r) > 1 else None) for r in (E3.get("O eksporcie") or EMPTY)["rows"][1:] if r}
o3 = table(E3, "Stan przedpłat i należności")
since = int(sql(f"SELECT COALESCE(SUM(amount_grosz), 0) FROM wallet_transactions WHERE created_at >= {T_SQL}"))
since_g = int(sql(f"SELECT COALESCE(SUM(amount_grosz), 0) FROM wallet_transactions WHERE user_id = '{G}' AND created_at >= {T_SQL}"))
w3 = next((r for r in o3 if r.get("Rodzaj") == "portfel (przedpłata)" and r.get("Klient") == NICK), {})
tot3 = {r.get("Rodzaj"): r for r in o3}
check(f"«на конец» — {T_DAY}; кошельки тогда + операции кошельков с тех пор = кошельки сейчас; у гостя — сейчас минус его операции "
      "с тех пор", about3.get("Na koniec okresu") == T_DAY
      and money((tot3.get("Portfele — razem") or {}).get("Kwota")) + since == g(LB["wallets"]["pln"])
      and money(w3.get("Kwota")) == wallet(NICK) - since_g, J([about3.get("Na koniec okresu"), tot3.get("Portfele — razem"), since, w3, since_g]))
d3 = next((r for r in o3 if r.get("Rodzaj") == "należność od klienta" and r.get("Klient") == NICK), {})
owed_then = int(sql(f"""SELECT COALESCE(SUM(g), 0) FROM (
    SELECT ROUND(s.total_pln * 100) AS g FROM sales s LEFT JOIN debt_write_offs w ON w.id = s.write_off_id
     WHERE s.user_id = '{G}' AND s.created_at < {T_SQL} AND (s.paid_at IS NULL OR s.paid_at >= {T_SQL})
       AND (s.voided_at IS NULL OR s.voided_at >= {T_SQL}) AND (s.write_off_id IS NULL OR w.created_at >= {T_SQL})
    UNION ALL
    SELECT ROUND(o.total_pln * 100) FROM kitchen_orders o LEFT JOIN debt_write_offs w ON w.id = o.write_off_id
     WHERE o.user_id = '{G}' AND o.status = 'done' AND o.status_at < {T_SQL} AND (o.paid_at IS NULL OR o.paid_at >= {T_SQL})
       AND (o.write_off_id IS NULL OR w.created_at >= {T_SQL})) d""") or 0)
check("тогда: долг гостя — всё, что было продано в долг до того момента и тогда не оплачено, не отменено и не списано (в том "
      "числе долг со вчерашнего вечера); сегодняшнего пакета и списанного сегодня долга — нет",
      (d3.get("Opis") or "").startswith("bar: ") and money(d3.get("Kwota")) >= 600 and money(d3.get("Kwota")) == owed_then
      and not any(r.get("ID") == pk for r in o3), J([d3, owed_then, [r for r in o3 if r.get("Klient") == NICK]], 600))
s3 = table(E3, "Magazyn na koniec okresu")
moved = int(sql(f"SELECT COALESCE(SUM(lm.qty), 0) FROM lot_moves lm JOIN stock_moves sm ON sm.id = lm.stock_move_id WHERE sm.created_at >= {T_SQL}"))
check("склад тогда: сегодняшней поставки нет, штук — сейчас минус движения партий с тех пор",
      not any(r.get("Towar") == f"{MARK} Кола" for r in s3) and s3 and s3[-1].get("Ilość, szt.") == float(SV.get("units")) - moved,
      J([s3[-1] if s3 else None, SV.get("units"), moved]))
done()
