#!/usr/bin/env python3
"""З10-и5 «Цены поставки — сервер» (№68, №69; решение основателя 29.09). PUT /admin/supplies/:id/pricing — только владелец:
у строки одна или несколько цен за штуку, штук в сумме — сколько пришло после правок приёмки; контрагент, номер и дата
документа, способ, «оплачен», категория расхода (закупочная, по умолчанию «Закупка товаров»), VAT. Строка становится
кусками-партиями с ценами, уже списанное переносится по порядку: проданное — по первой цене, отмена продажи возвращает в
те же куски. Поставка сама заводит один расход со ссылкой; переоценка его переписывает, пока сумму не поправили вручную;
поправили — не трогает, расхождение — флагом; сторнировали — тоже не трогает. Оценка в ноль — расхода нет. Правка
приёмки вниз после продажи и отмены (З10-и4): вторая продажа — всё равно по первой цене. Админ цен не видит нигде.
Данные: позиции «z10 Цены A», «z10 Цены B», «z10 Цены C» (категория z10) заводятся каждый прогон и в конце удаляются
drop_good вместе с поставками; контрагент «z10 Поставщик» живёт между прогонами (удаления нет); расходы поставок в конце
сторнируются; продажи — картой (ящик не трогают), отменённые остаются в истории; в уборке отменяются и остальные —
иначе «Что покупают» стенда копило «z10 Цены A» без себестоимости (З18-и4в)."""
import json
import threading
import uuid as uuidlib

from _z import *

A, B, C, MARK, CP = "z10 Цены A", "z10 Цены B", "z10 Цены C", "z10 цены", "z10 Поставщик"
exps = set()


def gone():  # расходы поставок проверки — и заведённые этим прогоном, и оставшиеся от упавшего (заметка «поставка от …: z10 Цены …»)
    for eid in set(exps) | set(rows("SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%z10 Цены%'")):
        if sql(f"SELECT count(*) FROM expenses WHERE id = '{eid}' AND storno_at IS NULL") == "1":
            api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for n in (A, B, C):
        for gid in rows(f"SELECT id FROM goods WHERE name = '{n}'"):
            for sid in rows(f"SELECT id FROM sales WHERE good_id = '{gid}' AND voided_at IS NULL"):  # З18-и4в
                api("POST", f"/admin/sales/{sid}/void", {}, "boss")
            drop_good(gid)


gone()
at_end(gone)
mk = lambda name, price: api("POST", "/admin/goods", {"name": name, "category": "z10", "price_pln": price}, "boss")[1].get("good", {}).get("id")
ga, gb, gc = mk(A, 5), mk(B, 4), mk(C, 6)
s, d = api("POST", "/admin/counterparties", {"name": CP}, "boss")
cpid = (d.get("counterparty") or {}).get("id") or d.get("id")
cats = {c.get("code"): c for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", []) if c.get("code")}
YDAY = sql(f"SELECT ((now() AT TIME ZONE '{TZ}')::date - 1)::text")
stock = lambda gid: int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'") or -1)
lots = lambda gid, sid: rows(f"SELECT part || ' ' || qty_in || '/' || qty_left || ' @' || COALESCE(price_grosz::text, '-') FROM stock_lots "
                             f"WHERE good_id = '{gid}' AND supply_id = '{sid}' ORDER BY part")
moves = lambda sale_id, sid: rows(f"SELECT lm.qty || ' part' || l.part FROM lot_moves lm JOIN stock_moves m ON m.id = lm.stock_move_id "
                                  f"JOIN stock_lots l ON l.id = lm.lot_id WHERE m.sale_id = '{sale_id}' AND l.supply_id = '{sid}' "
                                  f"ORDER BY m.delta, l.part")
lots_ok = lambda: api("GET", "/admin/goods/lots/check", None, "boss")[1].get("ok") is True
waiting = lambda: api("GET", "/admin/overview", None, "boss")[1].get("supplies_waiting") or 0
sell = lambda gid, n: (api("POST", "/admin/sales", {"good_id": gid, "qty": n, "method": "card"}, "admin1")[1].get("sale") or {}).get("id")
t0 = sql("SELECT now()::text")
acts = lambda: rows(f"SELECT action || '|' || details FROM admin_actions WHERE created_at > '{t0}' AND admin_id = '{uid('boss')}' ORDER BY created_at")
seen = lambda who: [x.get("kind", "") for x in audit(who, limit=200)]  # что журнал отдаёт этому человеку


def keys_of(x):
    if isinstance(x, dict):
        for k, v in x.items():
            yield k.lower()
            yield from keys_of(v)
    elif isinstance(x, list):
        for v in x:
            yield from keys_of(v)


def no_prices(x):
    """нигде в ответе нет цены закупки: ни одного ключа с price, grosz, cost, total, amount, vat, expense (статус «priced» — не ключ)"""
    return not any(w in k for k in keys_of(x) for w in ("price", "grosz", "cost", "total", "amount", "vat", "expense"))


def exp_row(eid):
    if not eid:
        return ""  # расхода нет (сервер без оценки)
    return sql(f"SELECT e.gross_grosz || '|' || e.vat_grosz || '|' || e.method || '|' || e.doc_no || '|' || e.op_date || '|' || e.paid || '|' || c.name "
               f"|| '|' || COALESCE(cp.name, '-') || '|' || COALESCE(e.supply_id::text, '-') || '|' || (e.storno_at IS NOT NULL) FROM expenses e "
               f"JOIN expense_categories c ON c.id = e.category_id LEFT JOIN counterparties cp ON cp.id = e.counterparty_id WHERE e.id = '{eid}'")


section("до оценки")
w_before = waiting()
s, d = api("POST", "/admin/supplies", {"lines": [{"good_id": ga, "qty": 30}, {"good_id": gb, "qty": 20}], "note": f"{MARK}: накладная 5"}, "admin1")
sid = (d.get("supply") or {}).get("id")
sale1 = sell(ga, 12)
pw = api("GET", "/admin/purchases", None, "boss")[1]
row = next((x for x in pw.get("waiting", []) if x.get("id") == sid), {})
la = next((l for l in row.get("lines", []) if l.get("good_id") == ga), {})
check("владельцу «Закупки»: поставка ждёт цен; у строки — пришло 30, ушло 12, кусок без цены (осталось 18)",
      s == 201 and sale1 and row.get("status") == "waiting" and la.get("qty") == 30 and la.get("gone") == 12
      and la.get("pieces") == [{"qty": 30, "left": 18, "price_pln": None}] and waiting() == w_before + 1, J([s, row], 500))
base = {"lines": [{"good_id": ga, "prices": [{"qty": 20, "price_pln": 2.5}, {"qty": 10, "price_pln": 2.8}]},
                  {"good_id": gb, "prices": [{"qty": 20, "price_pln": 3.1}]}],
        "counterparty_id": cpid, "doc_no": "FV z10/1", "doc_date": YDAY, "method": "bank", "vat_pln": 23}
st = [api(m, p, b, "admin1")[0] for m, p, b in (("GET", "/admin/purchases", None), ("GET", f"/admin/supplies/{sid}/pricing", None),
                                               ("PUT", f"/admin/supplies/{sid}/pricing", base))]
check("админу закупки и цены — 403 (GET «Закупки», GET и PUT цен поставки)", st == [403, 403, 403], J(st))

section("отказы")
bad = [
    ({**base, "lines": base["lines"][:1]}, 400, "pricing_lines", f"«{B}»: нет цен"),
    ({**base, "lines": [{"good_id": ga, "prices": [{"qty": 28, "price_pln": 2.5}]}, base["lines"][1]]}, 400, "pricing_qty",
     f"«{A}»: цен на 28 шт, а пришло 30"),
    ({**base, "lines": [{"good_id": ga, "prices": [{"qty": 30, "price_pln": 2.555}]}, base["lines"][1]]}, 400, "bad_prices", f"«{A}»:"),
    ({k: v for k, v in base.items() if k != "method"}, 400, "bad_method", ""),
    ({**base, "vat_pln": 141}, 400, "bad_vat", ""),
    ({**base, "category_id": (cats.get("rent") or {}).get("id")}, 400, "not_purchase", "закупочная"),
    ({**base, "counterparty_id": str(uuidlib.uuid4())}, 400, "bad_counterparty", ""),
    ({**base, "doc_date": "2020-01-01"}, 400, "bad_doc_date", ""),
    ({**base, "lines": []}, 400, "invalid_request", ""),
]
got = []
for body, want_s, want_c, want_m in bad:
    s, d = api("PUT", f"/admin/supplies/{sid}/pricing", body, "boss")
    got.append((s, d.get("code"), want_m in (d.get("message") or "")))
check("без строки, штук меньше, чем пришло, три знака в цене, без способа, VAT больше суммы, не закупочная категория, "
      "чужой контрагент, дата шесть лет назад, пустые строки → 400 с кодом; поставка всё ещё ждёт цен, партии не тронуты",
      got == [(w[1], w[2], True) for w in bad] and sql(f"SELECT status FROM supplies WHERE id = '{sid}'") == "waiting"
      and lots(ga, sid) == ["1 30/18 @-"] and lots(gb, sid) == ["1 20/20 @-"], J(got))

section("оценка")
s, d = api("PUT", f"/admin/supplies/{sid}/pricing", base, "boss")
sup = d.get("supply") or {}
eid = (sup.get("expense") or {}).get("id")
if eid:
    exps.add(eid)
check("владелец оценил: A 20 × 2,50 + 10 × 2,80, B 20 × 3,10 → 200, «оценена», сумма 140 zł, расход заведён; "
      "у A два куска — проданные 12 ушли по первой цене (20/8 @250, 10/10 @280), B 20/20 @310; сверка зелёная; «ждут цен» — как до приёмки",
      s == 200 and d.get("expense_action") == "created" and sup.get("status") == "priced" and sup.get("total_pln") == 140
      and lots(ga, sid) == ["1 20/8 @250", "2 10/10 @280"] and lots(gb, sid) == ["1 20/20 @310"] and moves(sale1, sid) == ["-12 part1"]
      and stock(ga) == 18 and stock(gb) == 20 and lots_ok() and waiting() == w_before, J([s, d.get("code"), d.get("message"), sup.get("total_pln"),
                                                                                           lots(ga, sid), moves(sale1, sid)], 500))
check(f"расход: 140,00 zł, VAT 23,00, банк, «FV z10/1» от вчера, оплачен, «Закупка товаров», контрагент «{CP}», ссылка на поставку; "
      "флагов нет", exp_row(eid) == f"14000|2300|bank|FV z10/1|{YDAY}|true|Закупка товаров|{CP}|{sid}|false"
      and (sup.get("expense") or {}).get("edited") is False and (sup.get("expense") or {}).get("diff_pln") == 0, exp_row(eid) if eid else "нет расхода")
note = sql(f"SELECT note FROM expenses WHERE id = '{eid}'") if eid else ""
ladm = api("GET", "/admin/supplies?status=all", None, "admin1")[1]
s1, d1 = api("PATCH", f"/admin/supplies/{sid}", {"lines": [{"good_id": ga, "qty": 29}]}, "admin1")
check("заметка расхода — «поставка от …: z10 Цены A ×30, z10 Цены B ×20»; админу поставка видна «оценена», без цен и расхода; "
      "исправить её — 409 supply_priced", note.startswith("поставка от ") and note.endswith(f": {A} ×30, {B} ×20")
      and any(x.get("id") == sid and x.get("status") == "priced" for x in ladm.get("supplies", [])) and no_prices(ladm)
      and (s1, d1.get("code")) == (409, "supply_priced"), J([note, s1, d1.get("code")], 300))
oa, aa = acts(), seen("admin1")
check("журнал владельца: «supply_price» — цены по строкам, сумма, способ, документ; «expense_create» — расход поставки; "
      "у админа их нет", any(x.startswith("supply_price|поставка от ") and x.endswith(
          f": {A} 20 × 2,50 + 10 × 2,80, {B} 20 × 3,10 · 140,00 zł · банк · FV z10/1") for x in oa)
      and any(x.startswith("expense_create|Закупка товаров · 140.00 zł") for x in oa)
      and "supply_price" in seen("boss") and not any(k == "supply_price" or k.startswith("expense_") for k in aa),
      J([x for x in oa if x.startswith(("supply_price", "expense_create"))][:3], 500))

section("продажи после оценки")
sale2 = sell(ga, 10)
l1 = lots(ga, sid)
s, _ = api("POST", f"/admin/sales/{sale1}/void", {}, "admin1")
check("продали A ×10 — 8 с первого куска и 2 со второго; отменили первую продажу — 12 вернулись в первый кусок; сверка зелёная",
      l1 == ["1 20/0 @250", "2 10/8 @280"] and moves(sale2, sid) == ["-8 part1", "-2 part2"] and s == 200
      and lots(ga, sid) == ["1 20/12 @250", "2 10/8 @280"] and lots_ok(), J([l1, moves(sale2, sid), s, lots(ga, sid)]))

section("переоценка")
re = {**base, "lines": [{"good_id": ga, "prices": [{"qty": 30, "price_pln": 2.7}]},
                        {"good_id": gb, "prices": [{"qty": 10, "price_pln": 3.0}, {"qty": 10, "price_pln": 3.2}]}],
      "category_id": (cats.get("kitchen") or {}).get("id"), "method": "card", "doc_no": "FV z10/2", "vat_pln": 0}
s, d = api("PUT", f"/admin/supplies/{sid}/pricing", re, "boss")
sup = d.get("supply") or {}
check("переоценка: A 30 × 2,70, B 10 × 3,00 + 10 × 3,20, «Закупка кухни», карта → 200, тот же расход переписан — 143,00 zł; "
      "у A один кусок 30/20 @270 (лишний удалён), B — два куска; продажа A — на нём; сверка зелёная",
      s == 200 and d.get("repriced") is True and d.get("expense_action") == "updated" and (sup.get("expense") or {}).get("id") == eid
      and exp_row(eid) == f"14300|0|card|FV z10/2|{YDAY}|true|Закупка кухни|{CP}|{sid}|false" and lots(ga, sid) == ["1 30/20 @270"]
      and lots(gb, sid) == ["1 10/10 @300", "2 10/10 @320"] and moves(sale2, sid) == ["-10 part1"] and stock(ga) == 20 and lots_ok(),
      J([s, d.get("message"), exp_row(eid), lots(ga, sid), lots(gb, sid), moves(sale2, sid)], 500))
oa = acts()
check("журнал: «переоценка — поставка от …» и правка расхода «сумма 140.00 → 143.00 zł»",
      any(x.startswith("supply_price|переоценка — поставка от ") for x in oa)
      and any(x.startswith("expense_update|") and "сумма 140.00 → 143.00 zł" in x and "переоценка — поставка от " in x for x in oa),
      J([x for x in oa if x.startswith(("supply_price", "expense_update"))][:3], 500))

res = {}
th = [threading.Thread(target=lambda: res.__setitem__("price", api("PUT", f"/admin/supplies/{sid}/pricing", re, "boss")[0])),
      threading.Thread(target=lambda: res.__setitem__("sale", sell(ga, 3)))]
[t.start() for t in th]
[t.join() for t in th]
check("переоценка и продажа A разом — обе проходят по очереди: продажа легла в кусок, остаток 17, сверка зелёная",
      res.get("price") == 200 and res.get("sale") and stock(ga) == 17 and lots(ga, sid) == ["1 30/17 @270"] and lots_ok(), J([res, lots(ga, sid)]))

section("расход поправили вручную")
s1, _ = api("PATCH", f"/admin/expenses/{eid}", {"amount_pln": 150}, "boss")
p1 = (api("GET", f"/admin/supplies/{sid}/pricing", None, "boss")[1].get("supply") or {}).get("expense") or {}
s2, d2 = api("PUT", f"/admin/supplies/{sid}/pricing", re, "boss")
e2 = (d2.get("supply") or {}).get("expense") or {}
check("сумму расхода поправили на 150 → флаг «поправлен вручную», расхождение +7,00; переоценка его не трогает (150), флаг остаётся",
      s1 == 200 and p1.get("edited") is True and p1.get("diff_pln") == 7 and s2 == 200 and d2.get("expense_action") == "edited"
      and e2.get("amount_pln") == 150 and e2.get("edited") is True and exp_row(eid).startswith("15000|"), J([s1, p1, s2, d2.get("expense_action"), e2]))
s1, _ = api("POST", f"/admin/expenses/{eid}/storno", {"reason": f"{MARK}: проверка"}, "boss")
s2, d2 = api("PUT", f"/admin/supplies/{sid}/pricing", re, "boss")
e2 = (d2.get("supply") or {}).get("expense") or {}
check("сторнировали расход — переоценка его не трогает и нового не заводит: «сторнирован»",
      s1 == 200 and s2 == 200 and d2.get("expense_action") == "storno" and e2.get("storno") is True and e2.get("id") == eid, J([s1, s2, d2.get("expense_action"), e2]))

section("оценка в ноль")
s, d = api("POST", "/admin/supplies", {"lines": [{"good_id": gb, "qty": 2}], "note": f"{MARK}: подарок"}, "admin1")
sid2 = (d.get("supply") or {}).get("id")
zero = {"lines": [{"good_id": gb, "prices": [{"qty": 2, "price_pln": 0}]}], "method": "bank"}
one = {"lines": [{"good_id": gb, "prices": [{"qty": 2, "price_pln": 1.5}]}], "method": "bank"}
s1, d1 = api("PUT", f"/admin/supplies/{sid2}/pricing", zero, "boss")
s2, d2 = api("PUT", f"/admin/supplies/{sid2}/pricing", one, "boss")
e2 = ((d2.get("supply") or {}).get("expense") or {}).get("id")
if e2:
    exps.add(e2)
s3, d3 = api("PUT", f"/admin/supplies/{sid2}/pricing", zero, "boss")
check("подарок поставщика: цена 0 → оценена, сумма 0, расхода нет; 1,50 → расход 3,00 заведён; снова 0 → свой расход сторнирован, ссылки нет",
      (s1, d1.get("expense_action"), (d1.get("supply") or {}).get("total_pln")) == (200, "none", 0) and "expense" not in (d1.get("supply") or {})
      and (s2, d2.get("expense_action")) == (200, "created") and e2 and exp_row(e2).startswith("300|0|bank|")
      and (s3, d3.get("expense_action")) == (200, "dropped") and "expense" not in (d3.get("supply") or {})
      and sql(f"SELECT storno_reason FROM expenses WHERE id = '{e2}'") == "поставка оценена в 0 zł", J([s1, d1.get("expense_action"), s2, s3, d3.get("expense_action")]))

section("правка приёмки после продажи и отмены")
s, d = api("POST", "/admin/supplies", {"lines": [{"good_id": gc, "qty": 35}], "note": f"{MARK}: пересчёт"}, "admin1")
sid3 = (d.get("supply") or {}).get("id")
sale3, sale4 = sell(gc, 25), sell(gc, 5)
s1, _ = api("POST", f"/admin/sales/{sale3}/void", {}, "admin1")
s2, d2 = api("PATCH", f"/admin/supplies/{sid3}", {"lines": [{"good_id": gc, "qty": 10}]}, "admin1")
s3, d3 = api("PUT", f"/admin/supplies/{sid3}/pricing", {"lines": [{"good_id": gc, "prices": [{"qty": 6, "price_pln": 1}, {"qty": 4, "price_pln": 2}]}],
                                                       "method": "bank"}, "boss")
e3 = ((d3.get("supply") or {}).get("expense") or {}).get("id")
if e3:
    exps.add(e3)
check("C: приняли 35, продали 25 и 5, первую продажу отменили, приёмку исправили до 10 → оценка 6 × 1,00 + 4 × 2,00: вторая продажа — "
      "по первой цене (куски 6/1 и 4/4), отменённая — туда и обратно; правки приёмки долей не держат; расход 14,00; сверка зелёная",
      (s1, s2, s3) == (200, 200, 200) and lots(gc, sid3) == ["1 6/1 @100", "2 4/4 @200"] and moves(sale4, sid3) == ["-5 part1"]
      and sorted(moves(sale3, sid3)) == sorted(["-6 part1", "-19 part2", "6 part1", "19 part2"]) and stock(gc) == 5
      and sql(f"SELECT count(*) FROM lot_moves lm JOIN stock_moves m ON m.id = lm.stock_move_id WHERE m.good_id = '{gc}' AND m.note = 'приёмка: правка'") == "0"
      and e3 and exp_row(e3).startswith("1400|0|bank|") and lots_ok(),
      J([s1, s2, d2.get("code"), s3, d3.get("code"), d3.get("message"), lots(gc, sid3), moves(sale4, sid3), moves(sale3, sid3)], 600))
done()
