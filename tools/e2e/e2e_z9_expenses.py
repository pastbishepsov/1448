#!/usr/bin/env python3
"""З9-и1 «Расходы: схема и API» (З-Р29, З-Р30; №61, №62, №65, №66): расходы вносит и видит только владелец — у
admin все ручки 403, строки журнала о расходах — только владельцу. Категории — 11 стандартных (закупка товаров и
кухни помечены «закупка»), контрагенты — справочником (NIP — 10 цифр, то же имя — 409). Расход: день операции,
категория, контрагент, сумма брутто и VAT, способ (из кассы, банк, карта, личные деньги владельца), номер
документа, заметка, фото до 500 КБ. Кривые суммы, даты, способ, категория → 400. Список за период — с итогами без
сторно, всего и по категориям, фильтры по категории и способу. Правка — журнал «было → стало». Удаления нет —
сторно с причиной: строка остаётся, из сумм уходит, править и сторнировать повторно нельзя.
Гости и сотрудники не нужны; контрагент «z9 Арендодатель» живёт между прогонами, расходы проверки с заметкой
«z9 проверка» в конце сторнируются — итоги периода как были."""
import urllib.request, urllib.error
from _z import *

MARK = "z9 проверка"
today, d2 = club_day(0), club_day(-2)
exp = lambda method, path, body=None, who="boss": api(method, path, body, who)


def raw(method, path, data=None, ctype=None, who="boss"):
    """сырое тело (фото): (статус, байты)"""
    req = urllib.request.Request(API + path, data=data, method=method)
    req.add_header("Authorization", "Bearer " + tok(who))
    if ctype:
        req.add_header("Content-Type", ctype)
    try:
        with urllib.request.urlopen(req) as r:
            return r.status, r.read()
    except urllib.error.HTTPError as e:
        return e.code, e.read()


def cleanup():
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        exp("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"})
at_end(cleanup)
cleanup()  # хвост упавшего прогона

section("только владельцу: у admin — 403 везде")
some = sql("SELECT id FROM expenses LIMIT 1") or "00000000-0000-0000-0000-000000000000"
codes = [exp(m, p, b, "admin2")[0] for m, p, b in [
    ("GET", "/admin/expense-categories", None), ("GET", "/admin/counterparties", None), ("POST", "/admin/counterparties", {"name": "x"}),
    ("GET", "/admin/expenses?preset=d7", None), ("POST", "/admin/expenses", {}), ("PATCH", f"/admin/expenses/{some}", {}),
    ("POST", f"/admin/expenses/{some}/storno", {"reason": "x"}), ("GET", f"/admin/expenses/{some}/photo", None),
    ("DELETE", f"/admin/expenses/{some}/photo", None)]] + [raw("PUT", f"/admin/expenses/{some}/photo", b"x", "image/jpeg", "admin2")[0]]
check("admin2: категории, контрагенты, расходы, правка, сторно, фото — все 10 ручек → 403", codes == [403] * 10, J(codes))

section("справочники")
s, d = exp("GET", "/admin/expense-categories")
cats = {c.get("code"): c for c in d.get("categories", [])}
std = ["rent", "utilities", "internet", "goods", "kitchen", "salary", "zus_tax", "equipment", "software", "marketing", "other"]
check("11 стандартных категорий по порядку, «закупка» — только товары и кухня", s == 200 and all(k in cats for k in std)
      and [c.get("code") for c in d["categories"] if c.get("code")][:11] == std
      and {k for k in std if cats[k].get("purchase")} == {"goods", "kitchen"}, J([(c.get("code"), c.get("name"), c.get("purchase")) for c in d.get("categories", [])], 500))
rent, other = cats.get("rent", {}).get("id"), cats.get("other", {}).get("id")
s, d = exp("POST", "/admin/counterparties", {"name": "z9 Арендодатель", "nip": "123-456-32-18", "contact": "+48 600 000 000"})
cp = (d.get("counterparty") or {}).get("id") or d.get("id")
check("контрагент «z9 Арендодатель» с NIP 123-456-32-18 → 201, NIP без дефисов (или уже есть — 409 с id)",
      (s == 201 and d["counterparty"].get("nip") == "1234563218") or (s == 409 and d.get("code") == "counterparty_exists" and cp), f"{s} {J(d)}")
s, d = exp("POST", "/admin/counterparties", {"name": "Z9 АРЕНДОДАТЕЛЬ"})
check("то же имя другим регистром → 409 counterparty_exists с id того же", s == 409 and d.get("id") == cp, f"{s} {J(d)}")
bad = [({"name": ""}, "bad_name"), ({"name": "z9 x", "nip": "12345"}, "bad_nip"), ({"name": "я" * 129}, "bad_name")]
got = [(exp("POST", "/admin/counterparties", b)[1].get("code"), want) for b, want in bad]
check("пустое имя, NIP не из 10 цифр, имя длиннее 128 → 400 с кодом", all(g == w for g, w in got), J(got))

section("внести расход")
body = {"op_date": d2, "category_id": rent, "counterparty_id": cp, "amount_pln": 3000, "vat_pln": 0, "method": "bank",
        "doc_no": "FV z9/1", "note": MARK + ": аренда"}
s, d = exp("POST", "/admin/expenses", body)
e = d.get("expense", {})
eid = e.get("id")
check("аренда 3000 zł банком за позавчера → 201: категория, контрагент, нетто, «оплачен», кто внёс",
      s == 201 and e.get("category") == "Аренда" and e.get("counterparty") == "z9 Арендодатель" and near(e.get("amount_pln", 0), 3000)
      and near(e.get("net_pln", 0), 3000) and e.get("paid") is True and e.get("op_date") == d2 and e.get("created_by") == "boss", f"{s} {J(d, 500)}")
s, d = exp("POST", "/admin/expenses", {**body, "category_id": other, "counterparty_id": "", "amount_pln": 123.45, "vat_pln": 23.08,
                                       "method": "owner", "paid": False, "op_date": today, "doc_no": "", "note": MARK + ": прочее"})
e2 = d.get("expense", {})
check("прочее 123,45 (VAT 23,08) личными деньгами владельца, не оплачен → 201, нетто 100,37", s == 201 and near(e2.get("net_pln", 0), 100.37)
      and e2.get("method") == "owner" and e2.get("paid") is False and "counterparty" not in e2, f"{s} {J(d, 400)}")
NIL = "00000000-0000-0000-0000-000000000000"
wrong = [({"amount_pln": 0}, "bad_amount"), ({"amount_pln": 10.005}, "bad_amount"), ({"amount_pln": 1000000.01}, "bad_amount"),
         ({"vat_pln": 3000.01}, "bad_vat"), ({"method": "cash"}, "bad_method"), ({"op_date": "27.09.2026"}, "bad_date"),
         ({"op_date": club_day(-6 * 366)}, "bad_date"), ({"category_id": NIL}, "bad_category"),
         ({"counterparty_id": NIL}, "bad_counterparty"), ({"method": None}, "invalid_request")]
got = []
for patch, want in wrong:
    b = {k: v for k, v in {**body, **patch}.items() if v is not None}  # None — поля нет
    s, d = exp("POST", "/admin/expenses", b)
    got.append((s, d.get("code"), want))
check("кривые сумма, VAT, способ, дата, категория, контрагент, нет способа → 400 с кодом, ничего не записано",
      all(s == 400 and c == w for s, c, w in got) and int(sql(f"SELECT count(*) FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL")) == 2, J(got, 600))

section("список за период: итоги без сторно, по категориям, фильтры")
q = f"/admin/expenses?from={d2}&to={today}"
s, d = exp("GET", q)
ids = [x.get("id") for x in d.get("expenses", [])]
tot = d.get("totals", {})
by = {x.get("category"): x for x in d.get("by_category", [])}
cents = lambda sq: round(float(sql(sq) or 0), 2)
db_total = cents(f"SELECT COALESCE(SUM(gross_grosz),0)/100.0 FROM expenses WHERE storno_at IS NULL AND op_date BETWEEN '{d2}' AND '{today}'")
check("оба расхода в списке, свежий день сверху; итог = база за период, «Аренда» — в разбивке",
      s == 200 and eid in ids and e2.get("id") in ids and ids.index(e2.get("id")) < ids.index(eid) and near(tot.get("amount_pln", -1), db_total)
      and by.get("Аренда", {}).get("amount_pln", 0) >= 3000, J({"итог": tot, "база": db_total, "Аренда": by.get("Аренда")}, 400))
s, d = exp("GET", q + "&method=bank")
s2, d3 = exp("GET", q + f"&category_id={other}")
check("фильтры: способ «банк» — аренда без «прочего», категория «Прочее» — наоборот",
      eid in [x["id"] for x in d.get("expenses", [])] and e2.get("id") not in [x["id"] for x in d.get("expenses", [])]
      and e2.get("id") in [x["id"] for x in d3.get("expenses", [])] and eid not in [x["id"] for x in d3.get("expenses", [])])

section("поправить: журнал «было → стало»")
s, d = exp("PATCH", f"/admin/expenses/{eid}", {"amount_pln": 3100, "counterparty_id": "", "doc_no": "FV z9/1a"})
e = d.get("expense", {})
check("сумма 3000 → 3100, контрагент убран, документ другой → 200, три изменения", s == 200 and d.get("changed") == 3
      and near(e.get("amount_pln", 0), 3100) and "counterparty" not in e and e.get("doc_no") == "FV z9/1a", f"{s} {J(d, 300)}")
s, d = exp("PATCH", f"/admin/expenses/{eid}", {"vat_pln": 3100.01})
check("VAT больше суммы на правке → 400 bad_vat", s == 400 and d.get("code") == "bad_vat", f"{s} {J(d)}")
lines = [i.get("text", "") for i in audit(kind="expense_update", limit=5)]
check("журнал: «Аренда · 3100.00 zł: сумма 3000.00 → 3100.00 zł; контрагент z9 Арендодатель → —; документ …»",
      any("сумма 3000.00 → 3100.00 zł" in t and "контрагент z9 Арендодатель → —" in t and "документ «FV z9/1» → «FV z9/1a»" in t for t in lines), J(lines[:2], 400))

section("фото документа — до 500 КБ, только владельцу")
jpg = b"\xff\xd8\xff\xe0" + b"z9" * 2000
s, d = raw("PUT", f"/admin/expenses/{eid}/photo", jpg, "image/jpeg")
s2, got = raw("GET", f"/admin/expenses/{eid}/photo")
check("фото JPEG → 200, отдаётся владельцу те же байты", s == 200 and s2 == 200 and got == jpg, f"{s} {s2} {len(got)}")
check("фото длиннее 500 КБ → 413 photo_too_big, текст вместо картинки → 400 bad_photo",
      raw("PUT", f"/admin/expenses/{eid}/photo", b"\xff" * (500 * 1024 + 1), "image/jpeg")[0] == 413
      and raw("PUT", f"/admin/expenses/{eid}/photo", b"hello", "text/plain")[0] == 400)
s, d = exp("DELETE", f"/admin/expenses/{eid}/photo")
check("убрать фото → 200, фото больше нет (404)", s == 200 and d.get("expense", {}).get("has_photo") is False
      and raw("GET", f"/admin/expenses/{eid}/photo")[0] == 404, f"{s} {J(d, 200)}")

section("сторно вместо удаления")
s, d = exp("POST", f"/admin/expenses/{eid}/storno", {})
check("без причины → 400 need_reason", s == 400 and d.get("code") == "need_reason", f"{s} {J(d)}")
before = exp("GET", q)[1].get("totals", {}).get("amount_pln", 0)
s, d = exp("POST", f"/admin/expenses/{eid}/storno", {"reason": "внесли дважды"})
after_list = exp("GET", q)[1]
row = next((x for x in after_list.get("expenses", []) if x.get("id") == eid), {})
check("сторно → 200: строка осталась с «кто и почему», из итога ушла (−3100)", s == 200 and row.get("storno_by") == "boss"
      and row.get("storno_reason") == "внесли дважды" and near(after_list.get("totals", {}).get("amount_pln", 0), before - 3100), f"{s} {J(row, 300)}")
check("сторнированный не правится (409 storno) и повторно не сторнируется (409 already_storno)",
      exp("PATCH", f"/admin/expenses/{eid}", {"amount_pln": 1})[1].get("code") == "storno"
      and exp("POST", f"/admin/expenses/{eid}/storno", {"reason": "ещё"})[1].get("code") == "already_storno")

section("журнал расходов — только владельцу")
kinds = "expense_create,expense_update,expense_storno,counterparty_create"
own_lines = audit(kind=kinds, limit=20)
adm_lines = audit("admin2", kind=kinds, limit=20)
check("владелец видит внесение, правку и сторно; admin2 — ни одной строки", {i.get("kind") for i in own_lines} >= {"expense_create", "expense_update", "expense_storno"}
      and adm_lines == [], J({"владелец": [i.get("kind") for i in own_lines[:6]], "admin2": len(adm_lines)}))
done()
