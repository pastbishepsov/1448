#!/usr/bin/env python3
"""З8-и5 «Очередь кухни без потерь» (хвост З8, «Найдено по дороге» З8-и4): очередь кухни у админа
(GET /admin/kitchen/orders) — все открытые заказы и все «ждут оплаты», от старых к новым, плюс закрытые за клубные
сутки (выданы и рассчитаны или отменены) — последние 200; значок «в работе · ждут оплаты» — по всем заказам, число
закрытых за сутки — closed_total. Раньше всё шло одним списком от старых к новым с обрезкой на 200: в загруженный
день только что пришедший заказ в очередь не попадал.
Чтобы очередь была полной на любом стенде, проверка на время добавляет 250 отменённых заказов «z8 заглушка» на
boss, созданных в начале клубных суток (склад, деньги и долг они не трогают), и в конце их удаляет. Guest39 за
PC-06 заказывает «Woda 0.5» с оплатой при выходе; в конце заказ отменяется (товар — на склад), сессия завершается."""
import datetime as dt
from _z import *

STUB = "z8 заглушка"
OPEN = "('new','accepted','preparing','delivering')"
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')"))
F = at_club(club_day(0), f"{rh:02d}:00")
T = at_club(sql(f"SELECT ('{club_day(0)}'::date + 1)::text"), f"{rh:02d}:00")
CLOSED = f"created_at >= {F} AND created_at < {T} AND (status = 'cancelled' OR (status = 'done' AND paid_at IS NOT NULL))"
queue = lambda: api("GET", "/admin/kitchen/orders", None, "admin1")[1]
pin_shift("admin1")  # З18-и4г: очередь admin1 — сами клубные сутки, его смена не раздвигает окно


def truth():
    """то же, что должна показать очередь, — прямо из базы"""
    live = rows(f"SELECT id FROM kitchen_orders WHERE status IN {OPEN} OR (status = 'done' AND paid_at IS NULL)")
    return {"open": int(sql(f"SELECT COUNT(*) FROM kitchen_orders WHERE status IN {OPEN}")),
            "unpaid": int(sql("SELECT COUNT(*) FROM kitchen_orders WHERE status = 'done' AND paid_at IS NULL")),
            "closed_total": int(sql(f"SELECT COUNT(*) FROM kitchen_orders WHERE {CLOSED}")),
            "live": set(live),
            "closed_last": set(rows(f"SELECT id FROM kitchen_orders WHERE {CLOSED} ORDER BY created_at DESC LIMIT 200"))}


def split(d):
    ids = [o.get("id") for o in d.get("orders", [])]
    closed = {o.get("id") for o in d.get("orders", []) if o.get("status") in ("done", "cancelled") and not o.get("unpaid")}
    return ids, set(ids) - closed, closed


def matches(d, t):
    ids, live, closed = split(d)
    times = [dt.datetime.fromisoformat(o["created_at"].replace("Z", "+00:00")) for o in d.get("orders", [])]
    return (d.get("open") == t["open"] and d.get("unpaid") == t["unpaid"] and d.get("closed_total") == t["closed_total"]
            and live == t["live"] and closed == t["closed_last"] and len(ids) == len(set(ids)) and times == sorted(times))


shown = lambda d, t: J({"очередь": {k: d.get(k) for k in ("count", "open", "unpaid", "closed_total")},
                        "база": {k: (len(v) if isinstance(v, set) else v) for k, v in t.items()},
                        "живых нет в очереди": len(t["live"] - split(d)[1]),
                        "закрытых не тех": len(split(d)[2] ^ t["closed_last"])}, 500)

sql(f"DELETE FROM kitchen_orders WHERE name = '{STUB}'")  # хвост упавшего прогона
section("очередь — живые все, закрытые за сутки — последние 200, счётчики по всем")
d0, t0 = queue(), truth()
check("admin1: открытые, «ждут оплаты» и закрытые за сутки — как в базе, от старых к новым", matches(d0, t0), shown(d0, t0))

section("очередь переполнена: 250 отменённых заглушек с утра, новый заказ — виден")
g = uid("Guest39")
W = sql("SELECT id FROM goods WHERE name='Woda 0.5'")
stock0 = int(sql(f"SELECT stock FROM goods WHERE id='{W}'"))
end_all("Guest39")
at_end(lambda: end_all("Guest39"))
ensure_wallet("Guest39", 20)
seat("Guest39", "PC-06")
boss = uid("boss")
at_end(lambda: sql(f"DELETE FROM kitchen_orders WHERE name = '{STUB}'"))
sql(f"INSERT INTO kitchen_orders (club_id, user_id, name, qty, price_pln, total_pln, paid, status, status_at, created_at) "
    f"SELECT (SELECT id FROM clubs ORDER BY created_at LIMIT 1), '{boss}', '{STUB}', 1, 1, 1, 'postpay', 'cancelled', now(), "
    f"{F} + g * interval '1 second' FROM generate_series(1, 250) AS g")
s, o = api("POST", "/admin/kitchen/orders", {"guest_id": g, "good_id": W, "qty": 1, "method": "later"}, "admin1")
oid = (o.get("order") or {}).get("id")
at_end(lambda: oid and api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "admin1"))
check("Guest39 за PC-06: «Woda 0.5» с оплатой при выходе → 201, заказ принят", s == 201 and oid
      and (o.get("order") or {}).get("status") == "accepted", f"{s} {J(o, 200)}")
d1, t1 = queue(), truth()
mine = next((x for x in d1.get("orders", []) if x.get("id") == oid), None)
check("новый заказ — в очереди, «принят», к PC-06", mine is not None and mine.get("status") == "accepted"
      and mine.get("computer") == "PC-06", J(mine or {"закрытых в очереди": len(split(d1)[2]), "всего": d1.get("count")}, 200))
check("очередь = база: живые все, закрытые — 200 последних за сутки, счётчики и closed_total по всем",
      matches(d1, t1) and t1["closed_total"] >= t0["closed_total"] + 250 and len(split(d1)[2]) == 200, shown(d1, t1))

section("заказ отменён — ушёл из работы в «за смену»")
s, _ = api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "admin1")
d2, t2 = queue(), truth()
gone = next((x for x in d2.get("orders", []) if x.get("id") == oid), None)
check("отмена → 200: в работе на один меньше, заказ — среди закрытых за сутки, товар вернулся на склад",
      s == 200 and d2.get("open") == d1.get("open", 0) - 1 and gone is not None and gone.get("status") == "cancelled"
      and matches(d2, t2) and int(sql(f"SELECT stock FROM goods WHERE id='{W}'")) == stock0, shown(d2, t2))
oid = None  # уже отменён
done()
