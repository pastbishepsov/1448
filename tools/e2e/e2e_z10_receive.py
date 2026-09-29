#!/usr/bin/env python3
"""З10-и4 «Приёмка поставки админом» (№78; решения основателя 28.09). POST /admin/supplies — админ или владелец:
строки «позиция × штук» (повтор позиции складывается) и заметка → остаток растёт, у строки — партия без цены,
поставка «ждёт цен», в канал всем — «Принята поставка: …». В ответах админу нет ни одной цены закупки. Владельцу —
число «ждут цен» (overview.supplies_waiting), админу — нет. Пока ждёт цен, поставку можно исправить: штуки строки
(меньше ушедшего — 409), с пометкой «исправлено когда и кем», «было» у строки, строками в журнал и канал; правка — не
«Потери». Отменить целиком — только если из поставки ничего не ушло. Оценённую и отменённую не исправить. Старый
приход по позиции — поставка из одной строки, тоже со строкой в канал.
Данные: позиции «z10 Приёмка A» и «z10 Приёмка B» (категория z10) заводятся каждый прогон и в конце удаляются
drop_good вместе с движениями, партиями и поставками; продажа — картой и отменяется."""
import json
import threading
import uuid as uuidlib

from _z import *

A, B, MARK = "z10 Приёмка A", "z10 Приёмка B", "z10 приёмка"
A1, A2 = uid("admin1"), uid("admin2")


def gone():
    for n in (A, B):
        for gid in rows(f"SELECT id FROM goods WHERE name = '{n}'"):
            drop_good(gid)


gone()
at_end(gone)
ga = api("POST", "/admin/goods", {"name": A, "category": "z10", "price_pln": 3}, "boss")[1].get("good", {}).get("id")
gb = api("POST", "/admin/goods", {"name": B, "category": "z10", "price_pln": 4}, "boss")[1].get("good", {}).get("id")
stock = lambda gid: int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'"))
lots = lambda gid: rows(f"SELECT kind || ' ' || qty_in || '/' || qty_left || ' ' || COALESCE(price_grosz::text, '-') FROM stock_lots "
                        f"WHERE good_id = '{gid}' ORDER BY received_at, part, id")
waiting = lambda: api("GET", "/admin/overview", None, "boss")[1].get("supplies_waiting")
lots_ok = lambda: api("GET", "/admin/goods/lots/check", None, "boss")[1].get("ok") is True
t0 = sql("SELECT now()::text")
msgs = lambda: rows(f"SELECT visible_to || '|' || text FROM staff_messages WHERE text LIKE '%{MARK[:3]} Приёмка%' AND created_at > '{t0}' ORDER BY created_at")


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


def line(sup, gid):
    return next((l for l in (sup or {}).get("lines", []) if l.get("good_id") == gid), {})


section("приёмка")
w0 = waiting() or 0  # на HEAD поля нет
s, d = api("POST", "/admin/supplies", {"lines": [{"good_id": ga, "qty": 30}, {"good_id": gb, "qty": 20}, {"good_id": ga, "qty": 5}],
                                      "note": f"{MARK}: накладная 7"}, "admin1")
sup = d.get("supply") or {}
sid = sup.get("id")
check("admin1 принял A ×30, B ×20 и ещё A ×5 → 201: две строки (A 35, B 20), «ждёт цен», принял admin1; остаток вырос, у строк партии без цены",
      s == 201 and sup.get("status") == "waiting" and sup.get("received_by") == "admin1" and line(sup, ga).get("qty") == 35
      and line(sup, gb).get("qty") == 20 and line(sup, ga).get("received") == 35 and stock(ga) == 35 and stock(gb) == 20
      and lots(ga) == ["supply 35/35 -"] and lots(gb) == ["supply 20/20 -"], J([s, sup, lots(ga), lots(gb)], 600))
lst = api("GET", "/admin/supplies", None, "admin1")[1]
check("в ответах админу — ни одной цены закупки (приёмка и список поставок); поставка в «ждут цен»",
      no_prices(d) and no_prices(lst) and any(x.get("id") == sid for x in lst.get("supplies", [])), J(lst.get("supplies", [])[-1:], 300))
m = msgs()
check("в канал всем: «📦 Принята поставка: z10 Приёмка A ×35, z10 Приёмка B ×20 · admin1»; сверка партий зелёная",
      m[-1:] == [f"all|📦 Принята поставка: {A} ×35, {B} ×20 · admin1"] and lots_ok(), J(m))
ova = api("GET", "/admin/overview", None, "admin1")[1]
check("владельцу «ждут цен» +1 (overview.supplies_waiting), админу числа нет", waiting() == w0 + 1 and "supplies_waiting" in ova
      and ova.get("supplies_waiting") is None, J([w0, waiting(), ova.get("supplies_waiting")]))
bad = [api("POST", "/admin/supplies", b, "admin1") for b in (
    {"lines": []}, {"lines": [{"good_id": ga, "qty": 0}]}, {"lines": [{"good_id": "кривой", "qty": 1}]},
    {"lines": [{"good_id": str(uuidlib.uuid4()), "qty": 1}]}, {"lines": [{"good_id": str(uuidlib.uuid4()), "qty": 1} for _ in range(51)]},
    {"lines": [{"good_id": ga, "qty": 1}], "note": "я" * 201}, {"lines": [{"good_id": ga, "qty": 1000000}]})]
got = [(x[0], x[1].get("code")) for x in bad]
check("пусто, ноль штук, кривая и чужая позиция, 51 строка, заметка длиннее 200, остаток выше потолка → 400/404/409 с кодом",
      got == [(400, "bad_lines"), (400, "bad_qty"), (400, "bad_lines"), (404, "good_not_found"), (400, "bad_lines"), (400, "bad_note"),
              (409, "stock_limit")], J(got))

section("исправление")
s, sale = api("POST", "/admin/sales", {"good_id": ga, "qty": 5, "method": "card"}, "admin1")
sale_id = (sale.get("sale") or {}).get("id")
s1, d1 = api("PATCH", f"/admin/supplies/{sid}", {"lines": [{"good_id": ga, "qty": 4}]}, "admin2")
check("продали A ×5; исправить A на 4 → 409 line_sold «из строки уже ушло 5 шт»", s == 201 and s1 == 409 and d1.get("code") == "line_sold"
      and d1.get("gone") == 5 and "ушло 5" in (d1.get("message") or ""), J([s, s1, d1]))
adj0 = sql(f"SELECT count(*) FROM stock_moves WHERE good_id IN ('{ga}', '{gb}') AND reason = 'adjust'")
s, d = api("PATCH", f"/admin/supplies/{sid}", {"lines": [{"good_id": ga, "qty": 30}, {"good_id": gb, "qty": 22}]}, "admin2")
sup = d.get("supply") or {}
check("admin2: A 35 → 30, B 20 → 22 → 200, изменений 2; остатки A 25 (30 − продано 5), B 22; партии A 30/25, B 22/22; у строк «было» 35 и 20",
      s == 200 and d.get("changed") == 2 and stock(ga) == 25 and stock(gb) == 22 and lots(ga) == ["supply 30/25 -"] and lots(gb) == ["supply 22/22 -"]
      and line(sup, ga).get("received") == 35 and line(sup, ga).get("qty") == 30 and line(sup, gb).get("received") == 20, J([s, sup], 600))
check("пометка «исправлено» — когда и кем (admin2); в канал всем «✏️ Исправлена поставка от …: A 35 → 30, B 20 → 22 · admin2»; "
      "правка — движение прихода, не «Потери»; сверка зелёная",
      sup.get("edited_at") and sup.get("edited_by") == "admin2" and msgs()[-1].startswith("all|✏️ Исправлена поставка от ")
      and msgs()[-1].endswith(f": {A} 35 → 30, {B} 20 → 22 · admin2") and adj0 == sql(f"SELECT count(*) FROM stock_moves WHERE good_id IN ('{ga}', '{gb}') AND reason = 'adjust'")
      and sql(f"SELECT string_agg(delta || ' ' || reason || ' ' || note, ', ' ORDER BY created_at) FROM stock_moves WHERE good_id = '{ga}' AND note LIKE 'приёмка:%'") == "-5 supply приёмка: правка"
      and lots_ok(), J([sup.get("edited_at"), sup.get("edited_by"), msgs()[-1:]], 400))
got = [api("PATCH", f"/admin/supplies/{sid}", b, "admin1") for b in (
    {"lines": [{"good_id": str(uuidlib.uuid4()), "qty": 1}]}, {"lines": [{"good_id": ga, "qty": 30}, {"good_id": ga, "qty": 30}]},
    {"lines": [{"good_id": ga, "qty": 30}]}, {})]
got = [(x[0], x[1].get("code"), x[1].get("changed")) for x in got]
check("строки, которой в поставке нет, → 400 line_unknown; позиция дважды → 400; те же штуки — «изменений нет»; пустой запрос → 400",
      got == [(400, "line_unknown", None), (400, "bad_lines", None), (200, None, 0), (400, "invalid_request", None)], J(got))
res = []
th = [threading.Thread(target=lambda w=w: res.append(api("PATCH", f"/admin/supplies/{sid}", {"lines": [{"good_id": ga, "qty": 28}]}, w)[1].get("changed")))
      for w in ("admin1", "admin2")]
[t.start() for t in th]
[t.join() for t in th]
check("две правки разом «A 30 → 28» (admin1 и admin2) не складываются: одна меняет, другая — «изменений нет»; A пришло 28, остаток 23; сверка зелёная",
      sorted(res, key=str) == [0, 1] and lots(ga) == ["supply 28/23 -"] and stock(ga) == 23 and lots_ok(), J([res, lots(ga), stock(ga)]))

section("отмена")
s1, d1 = api("POST", f"/admin/supplies/{sid}/cancel", {}, "admin1")
s2, _ = api("POST", f"/admin/sales/{sale_id}/void", {}, "admin1")
s3, d3 = api("POST", f"/admin/supplies/{sid}/cancel", {}, "admin1")
sup = d3.get("supply") or {}
check("отменить, пока из поставки ушёл товар, → 409 supply_sold; вернули продажу → отмена 200: «отменена», остатки 0, у партий «пришло» и остаток — 0 (0/0)",
      s1 == 409 and d1.get("code") == "supply_sold" and s2 == 200 and s3 == 200 and sup.get("status") == "cancelled" and stock(ga) == 0
      and stock(gb) == 0 and lots(ga) == ["supply 0/0 -"] and lots(gb) == ["supply 0/0 -"], J([s1, d1.get("code"), s2, s3, sup.get("status"), stock(ga), stock(gb), lots(ga), lots(gb)]))
lst = api("GET", "/admin/supplies", None, "admin1")[1].get("supplies", [])
s4, d4 = api("PATCH", f"/admin/supplies/{sid}", {"lines": [{"good_id": ga, "qty": 1}]}, "admin1")
check("отменённой нет в «ждут цен», владельцу снова прежнее число; в канал «🚫 Отменена поставка от …: A ×28, B ×22 · admin1»; "
      "исправить её — 409 supply_cancelled; сверка зелёная",
      not any(x.get("id") == sid for x in lst) and waiting() == w0 and msgs()[-1].startswith("all|🚫 Отменена поставка от ")
      and msgs()[-1].endswith(f": {A} ×28, {B} ×22 · admin1") and s4 == 409 and d4.get("code") == "supply_cancelled" and lots_ok(),
      J([waiting(), w0, msgs()[-1:], s4, d4.get("code")], 400))
s, d = api("POST", "/admin/supplies", {"lines": [{"good_id": gb, "qty": 2}]}, "boss")
sid2 = (d.get("supply") or {}).get("id")
s1, d1 = api("PATCH", f"/admin/supplies/{sid2}", {"lines": [{"good_id": gb, "qty": 0}]}, "boss")
s2, _ = api("POST", f"/admin/supplies/{sid2}/cancel", {}, "boss")
check("владелец тоже принимает (201); все строки в ноль — 400 supply_empty «отмени целиком»; отмена — 200",
      s == 201 and s1 == 400 and d1.get("code") == "supply_empty" and s2 == 200, J([s, s1, d1.get("code"), s2]))

section("приход по позиции — поставка из одной строки")
s, _ = api("POST", f"/admin/goods/{ga}/stock", {"delta": 4, "reason": "supply", "note": f"{MARK}: приход"}, "admin1")
one = next((x for x in api("GET", "/admin/supplies", None, "admin1")[1].get("supplies", []) if x.get("note") == f"{MARK}: приход"), {})
check("приход A +4 → поставка из одной строки «ждёт цен» (A ×4, заметка прихода); в канал «📦 Принята поставка: z10 Приёмка A ×4 · admin1»",
      s == 200 and one.get("status") == "waiting" and [(l.get("good_id"), l.get("qty")) for l in one.get("lines", [])] == [(ga, 4)]
      and msgs()[-1] == f"all|📦 Принята поставка: {A} ×4 · admin1", J([s, one, msgs()[-1:]], 400))
if one.get("id"):
    sql(f"UPDATE supplies SET status = 'priced' WHERE id = '{one['id']}' RETURNING id")  # как будто оценили (З10-и5)
s1, d1 = api("PATCH", f"/admin/supplies/{one.get('id')}", {"lines": [{"good_id": ga, "qty": 3}]}, "admin1")
s2, d2 = api("POST", f"/admin/supplies/{one.get('id')}/cancel", {}, "admin1")
check("оценённую поставку не исправить и не отменить — 409 supply_priced", (s1, d1.get("code"), s2, d2.get("code")) == (409, "supply_priced", 409, "supply_priced"),
      J([s1, d1, s2, d2]))

section("журнал")
jr = rows(f"SELECT a.action || '|' || u.nickname || '|' || a.details FROM admin_actions a JOIN users u ON u.id = a.admin_id "
          f"WHERE a.created_at > '{t0}' AND a.details LIKE '%{A}%' ORDER BY a.created_at")
seen = [x.get("text", "") for x in audit("admin1", limit=60)]
check("журнал: «приёмка» — A ×35, B ×20 с заметкой; «правка» (admin2) — что было и что стало; «отмена» — что отменили; админ видит свои",
      f"supply_receive|admin1|{A} ×35, {B} ×20 · {MARK}: накладная 7" in jr and any(t.startswith("supply_edit|admin2|поставка от ")
      and t.endswith(f": {A} 35 → 30, {B} 20 → 22") for t in jr) and any(t.startswith("supply_cancel|admin1|поставка от ")
      and t.endswith(f": {A} ×28, {B} ×22") for t in jr) and jr[0].split("|", 2)[2] in seen, J(jr, 600))
done()
