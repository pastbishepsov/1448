#!/usr/bin/env python3
"""З23-и1а «Товары в «Отчётах» — сервер» (№48): GET /admin/reports/goods (владелец) — по позиции за период: штук и на
сколько продано, доля в продажах и в штуках, класс ABC (A — позиции, которые дают первые 80 % продаж, B — до 95 %,
C — остальные, без продаж — без класса), в скольких днях периода продажи были, сколько дней с последней продажи, склад
и на сколько дней его хватит, маржа по партиям. Проверка — на пустой неделе в прошлом: временные позиции «z23 …» и их
продажи от владельца картой, сдвинутые SQL-ом в эту неделю (у стенда в ней продаж нет — иначе классы зависели бы от
стенда); в конце продажи отменяет владелец, позиции удаляются SQL-ом (`drop_good`)."""
from _z import *

PRE = "z23 "
GOODS = [("z23 Альфа", 10, 7), ("z23 Бета", 5, 3), ("z23 Гамма", 10, 1), ("z23 Дельта", 5, 1)]  # имя, цена, продано штук


def wipe():
    for gid in rows(f"SELECT id FROM goods WHERE name LIKE '{PRE}%'"):
        drop_good(gid)


wipe()
at_end(wipe)  # в конце — после отмены продаж (уборка идёт в обратном порядке)
sales = []
at_end(lambda: [api("POST", f"/admin/sales/{s}/void", {}, "boss") for s in sales if s])

# пустая неделя в прошлом: понедельник, от которого 7 дней без единой живой продажи (свои прошлые — отменены)
week = None
for k in range(0, 30):
    d0 = sql(f"SELECT (date '2024-02-05' - {7 * k})::text")
    # отменённые продажи — свои хвосты прошлых прогонов, не мешают
    if sql(f"SELECT count(*) FROM sales WHERE voided_at IS NULL AND created_at >= {at_club(d0, '00:00')} - interval "
           f"'1 day' AND created_at < {at_club(d0, '00:00')} + interval '8 days'") == "0":
        week = d0
        break
check("нашлась неделя без продаж стенда", bool(week), week)
d1 = sql(f"SELECT ('{week}'::date + 1)::text")
d6 = sql(f"SELECT ('{week}'::date + 6)::text")

ids = {}
for name, price, n in GOODS + [("z23 Тишина", 4, 0)]:
    s, d = api("POST", "/admin/goods", {"name": name, "category": "z23", "price_pln": price, "stock": 50}, "boss")
    ids[name] = (d.get("good") or {}).get("id")
s, d = api("POST", "/admin/goods", {"name": "z23 Выкл", "category": "z23", "price_pln": 4, "stock": 5}, "boss")
off = (d.get("good") or {}).get("id")
api("PATCH", f"/admin/goods/{off}", {"active": False}, "boss")
for name, price, n in GOODS:
    s, d = api("POST", "/admin/sales", {"good_id": ids[name], "method": "card", "qty": n}, "boss")
    sid = (d.get("sale") or {}).get("id")
    sales.append(sid)
    sql(f"UPDATE sales SET created_at = {at_club(week, '12:00')}, paid_at = {at_club(week, '12:00')} WHERE id = '{sid}'")
# у «Альфы» — ещё одна продажа во второй день недели: продажи в 2 днях из 7
s, d = api("POST", "/admin/sales", {"good_id": ids["z23 Альфа"], "method": "card", "qty": 1}, "boss")
sid = (d.get("sale") or {}).get("id")
sales.append(sid)
sql(f"UPDATE sales SET created_at = {at_club(d1, '18:30')}, paid_at = {at_club(d1, '18:30')} WHERE id = '{sid}'")

s, r = api("GET", f"/admin/reports/goods?from={week}&to={d6}", None, "boss")
rows_ = {x.get("name"): x for x in (r.get("rows") or []) if str(x.get("name", "")).startswith(PRE)}
check("GET /admin/reports/goods — 200, период — неделя", s == 200 and (r.get("period") or {}).get("days") == 7,
      J([s, r.get("period")]))
cls = [rows_.get(n, {}).get("abc") for n in ("z23 Альфа", "z23 Бета", "z23 Гамма", "z23 Дельта", "z23 Тишина")]
check("классы: Альфа 80 zł — A; Бета 15 — A (выше неё 72,7 % продаж — меньше 80 %); Гамма 10 — B (выше — 86,4 %); "
      "Дельта 5 — C (выше — 95,5 %); Тишина без продаж — без класса", cls == ["A", "A", "B", "C", ""], J(cls))
a = rows_.get("z23 Альфа", {})
check("Альфа: 8 штук на 80 zł, доля продаж 72,7 %, штук 61,5 %; продажи в 2 днях из 7",
      a.get("items") == 8 and a.get("sold_pln") == 80 and a.get("share_pct") == 72.7 and a.get("units_share_pct") == 61.5
      and a.get("days_sold") == 2 and a.get("days") == 7, J(a))
since = int(sql(f"SELECT (date '{club_day()}' - date '{d1}')"))
check(f"Альфа: склад 42, хватит на 36,8 дня при темпе недели; последняя продажа — {since} дн. назад",
      a.get("stock") == 42 and a.get("cover_days") == 36.8 and a.get("days_since") == since, J(a))
t = rows_.get("z23 Тишина", {})
check("Тишина: живая, продаж нет — в отчёте, без класса, «хватит на» — нет темпа (null), последней продажи нет",
      t.get("items") == 0 and t.get("abc") == "" and t.get("cover_days") is None and t.get("days_since") is None, J(t))
check("выключенная без продаж в периоде — не в отчёте", "z23 Выкл" not in rows_, J(sorted(rows_)))
c = r.get("classes") or {}
check("итог по классам: A — 2 позиции, 86,4 % продаж; B — 1, 9,1 %; C — 1, 4,5 %",
      (c.get("A") or {}).get("count") == 2 and (c.get("A") or {}).get("share_pct") == 86.4
      and (c.get("B") or {}).get("share_pct") == 9.1 and (c.get("C") or {}).get("share_pct") == 4.5, J(c))
check("маржа — по партиям: у позиций без цены закупки себестоимости нет (null), «без себестоимости» — все проданные штуки",
      a.get("cost_pln") is None and a.get("no_cost_units") == 8,
      J({k: a.get(k) for k in ("cost_pln", "margin_pct", "no_cost_units")}))
s2, _ = api("GET", f"/admin/reports/goods?from={week}&to={d6}", None, "admin1")
s3, d3 = api("GET", f"/admin/reports/goods?from={d6}&to={week}", None, "boss")
check("админу — 403; конец раньше начала — 400 bad_range", s2 == 403 and s3 == 400 and d3.get("code") == "bad_range",
      J([s2, s3, d3]))
done()
