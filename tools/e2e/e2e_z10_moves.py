#!/usr/bin/env python3
"""З10-и1 «Единая точка движения склада»: все семь мест, где меняется остаток, — одна функция stockMove (изменение и
строка stock_moves вместе). Поведение не меняется — проверка идёт и по HEAD до рефакторинга: у каждого движения те же
знак и количество, причина, заметка, автор, цена, продажа; остаток позиции меняется ровно на него. Новое — только
kitchen_order_id: у заказа кухни и его отмены движение знает свой заказ (миграция 075). Отказы те же: списание в минус
— 409, приход выше потолка — 409, две кассы разом не продадут больше остатка; отмена заказа от стойки,
оплаченного сразу, возвращает товар один раз. Итог: остаток позиции = сумма её движений.
Данные: позиция «z10 Движение склада» (категория z10) живёт между прогонами, в конце выключена; «z10 Однодневка» и
«z10 Пустая» заводятся каждый прогон и удаляются SQL вместе со своим движением, партией и поставкой (продаж у них нет); Guest40 за PC-04
(кошелёк докладывается до 30 zł, сессия в конце завершается)."""
import threading

from _z import *

G, ONE, EMPTY, MARK = "z10 Движение склада", "z10 Однодневка", "z10 Пустая", "z10 проверка"
BOSS, A1, GUEST = uid("boss"), uid("admin1"), uid("Guest40")
COLS = "delta, reason, note, COALESCE(created_by::text, '-'), price_pln::text, COALESCE(sale_id::text, '-'), COALESCE(kitchen_order_id::text, '-')"


def drop(name):
    """одноразовую позицию — вместе с её движениями, партиями и поставкой (продаж и заказов у неё нет)"""
    for gid in rows(f"SELECT id FROM goods WHERE name = '{name}'"):
        drop_good(gid)


def stock(gid):
    return int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'"))


def moves_after(gid, t):
    """движения позиции после момента t — по порядку, полями COLS через «|»"""
    return rows(f"SELECT {COLS} FROM stock_moves WHERE good_id = '{gid}' AND created_at > '{t}' ORDER BY created_at")


def now():
    return sql("SELECT clock_timestamp()::text")


def row(delta, reason, note="", by="-", price="4.50", sale="-", order="-"):
    return f"{delta}|{reason}|{note}|{by}|{price}|{sale}|{order}"


for n in (ONE, EMPTY):
    drop(n)
at_end(lambda: [drop(n) for n in (ONE, EMPTY)])

section("создание позиции")
t = now()
s, d = api("POST", "/admin/goods", {"name": ONE, "category": "z10", "price_pln": 3.2, "stock": 7}, "boss")
one = (d.get("good") or {}).get("id") or sql(f"SELECT id FROM goods WHERE name = '{ONE}'")
m = rows(f"SELECT {COLS} FROM stock_moves WHERE good_id = '{one}'") if one else []
check("новая позиция с остатком 7 → 201, в ответе и в базе 7; одно движение «+7 · приход · начальный остаток» от владельца по цене 3,20",
      s == 201 and (d.get("good") or {}).get("stock") == 7 and stock(one) == 7 and m == [row(7, "supply", "начальный остаток", BOSS, "3.20")],
      J([s, (d.get("good") or {}).get("stock"), m]))
s, d = api("POST", "/admin/goods", {"name": EMPTY, "category": "z10", "price_pln": 1}, "boss")
empty = (d.get("good") or {}).get("id") or sql(f"SELECT id FROM goods WHERE name = '{EMPTY}'")
n = sql(f"SELECT count(*) FROM stock_moves WHERE good_id = '{empty}'") if empty else "?"
check("новая позиция без остатка → 201, остаток 0, движений нет", s == 201 and (d.get("good") or {}).get("stock") == 0 and n == "0", J([s, n]))
for x in (ONE, EMPTY):
    drop(x)

section("приход и корректировка")
s, d = api("POST", "/admin/goods", {"name": G, "category": "z10", "price_pln": 4.5, "stock": 20}, "boss")
gid = (d.get("good") or {}).get("id") or sql(f"SELECT id FROM goods WHERE name = '{G}'")
api("PATCH", f"/admin/goods/{gid}", {"active": True, "price_pln": 4.5}, "boss")
at_end(lambda: api("PATCH", f"/admin/goods/{gid}", {"active": False}, "boss"))
if stock(gid) != 20:  # повторный прогон: остаток — к 20 приходом или корректировкой, как у стойки (продажа «всего» ≤ 99 шт)
    k = 20 - stock(gid)
    api("POST", f"/admin/goods/{gid}/stock", {"delta": k, "reason": "supply" if k > 0 else "adjust", "note": f"{MARK}: к 20"}, "admin1")
st0, t = stock(gid), now()
s1, _ = api("POST", f"/admin/goods/{gid}/stock", {"delta": 10, "reason": "supply", "note": f"{MARK}: приход"}, "admin1")
s2, _ = api("POST", f"/admin/goods/{gid}/stock", {"delta": -3, "reason": "adjust", "note": f"{MARK}: бой"}, "admin1")
m = moves_after(gid, t)
check("приход +10 и корректировка −3 от admin1 → 200 и 200; остаток +7; движения «+10 · приход», «−3 · корректировка» с заметкой, по цене 4,50",
      s1 == 200 and s2 == 200 and stock(gid) == st0 + 7
      and m == [row(10, "supply", f"{MARK}: приход", A1), row(-3, "adjust", f"{MARK}: бой", A1)], J([s1, s2, stock(gid) - st0, m], 500))
st1, t = stock(gid), now()
s1, d1 = api("POST", f"/admin/goods/{gid}/stock", {"delta": -(st1 + 1), "reason": "adjust", "note": f"{MARK}: в минус"}, "admin1")
s2, d2 = api("POST", f"/admin/goods/{gid}/stock", {"delta": 1000000, "reason": "supply", "note": f"{MARK}: потолок"}, "admin1")
check("списание больше остатка → 409 negative_stock; приход выше потолка → 409 stock_limit; остаток тот же, движений нет",
      (s1, d1.get("code"), s2, d2.get("code")) == (409, "negative_stock", 409, "stock_limit") and stock(gid) == st1 and moves_after(gid, t) == [],
      J([s1, d1.get("code"), s2, d2.get("code")]))

section("продажа и её отмена")
st1, t = stock(gid), now()
s, d = api("POST", "/admin/sales", {"good_id": gid, "qty": 2, "method": "cash"}, "admin1")
sid = (d.get("sale") or {}).get("id")
m1, st2 = moves_after(gid, t), stock(gid)
s2, _ = api("POST", f"/admin/sales/{sid}/void", {}, "admin1")
m2 = moves_after(gid, t)
check("продажа ×2 налом → 201, остаток −2, движение «−2 · продажа» со ссылкой на продажу; отмена → остаток как был, «+2 · возврат · отмена продажи»",
      s == 201 and st2 == st1 - 2 and m1 == [row(-2, "sale", "", A1, sale=sid)] and s2 == 200 and stock(gid) == st1
      and m2 == [row(-2, "sale", "", A1, sale=sid), row(2, "void", "отмена продажи", A1, sale=sid)], J([s, st2 - st1, m1, s2, m2], 600))
st1, t = stock(gid), now()
res = []


def sell(who):
    s, d = api("POST", "/admin/sales", {"good_id": gid, "qty": st1, "method": "cash"}, who)
    res.append((s, d.get("code"), (d.get("sale") or {}).get("id"), who))


tok("admin1"), tok("admin2")
th = [threading.Thread(target=sell, args=(w,)) for w in ("admin1", "admin2")]
[x.start() for x in th]
[x.join() for x in th]
won = next((r for r in res if r[0] == 201), (0, None, None, None))
m, st2 = moves_after(gid, t), stock(gid)
sv = api("POST", f"/admin/sales/{won[2]}/void", {}, "boss")[0] if won[2] else 0
check(f"две кассы разом продают весь остаток ({st1}) → 201 и 409 out_of_stock; остаток 0, не в минус; одно движение «−{st1} · продажа»; "
      "отмена владельцем возвращает всё",
      sorted(r[0] for r in res) == [201, 409] and any(r[1] == "out_of_stock" for r in res) and st2 == 0
      and m == [row(-st1, "sale", "", uid(won[3]) if won[3] else "?", sale=won[2])] and sv == 200 and stock(gid) == st1,
      J([[r[:2] for r in res], st2, m, sv], 500))

section("заказ кухни гостя и его отмена")
end_all("Guest40")
at_end(lambda: end_all("Guest40"))
at_end(lambda: [api("POST", f"/admin/kitchen/orders/{o}/cancel", {}, "admin1") for o in  # упавший прогон — заказы не висят в очереди
                 rows(f"SELECT id FROM kitchen_orders WHERE user_id = '{uid('Guest40')}' AND status IN ('new','accepted','preparing','delivering')")])
ensure_wallet("Guest40", 30)
seat("Guest40", "PC-04")
gs, gd = login("Guest40")
gt = gd.get("access_token")
st1, t = stock(gid), now()
s, d = api("POST", "/me/kitchen/orders", {"good_id": gid, "qty": 2}, token=gt)
oid = (d.get("order") or {}).get("id")
m1, st2 = moves_after(gid, t), stock(gid)
s2, _ = api("POST", f"/me/kitchen/orders/{oid}/cancel", {}, token=gt)
m2 = moves_after(gid, t)
check("Guest40 из-за PC-04 заказал ×2 → 201, остаток −2, «−2 · заказ кухни» без автора, движение знает заказ; "
      "отменил → остаток как был, «+2 · возврат · отмена заказа кухни», тот же заказ",
      s == 201 and st2 == st1 - 2 and m1 == [row(-2, "order", "заказ кухни", order=oid)] and s2 == 200 and stock(gid) == st1
      and m2 == [row(-2, "order", "заказ кухни", order=oid), row(2, "void", "отмена заказа кухни", order=oid)], J([gs, s, st2 - st1, m1, s2, m2], 700))

section("заказ от стойки и его отмена")
st1, t = stock(gid), now()
s, d = api("POST", "/admin/kitchen/orders", {"guest_id": GUEST, "good_id": gid, "qty": 1, "method": "cash"}, "admin1")
oid = (d.get("order") or {}).get("id")
osale = sql(f"SELECT COALESCE(sale_id::text, '') FROM kitchen_orders WHERE id = '{oid}'") if oid else ""
m1, st2 = moves_after(gid, t), stock(gid)
s2, _ = api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "admin1")
m2 = moves_after(gid, t)
voided = sql(f"SELECT voided_at IS NOT NULL FROM sales WHERE id = '{osale}'") if osale else ""
check("admin1: заказ ×1 от стойки налом → 201, остаток −1, «−1 · заказ кухни от стойки» от admin1, движение знает заказ",
      s == 201 and osale and st2 == st1 - 1 and m1 == [row(-1, "order", "заказ кухни от стойки", A1, order=oid)], J([s, st2 - st1, m1], 500))
check("отмена оплаченного сразу заказа → продажа отменена, товар вернулся один раз: «+1 · возврат · отмена заказа кухни», у продажи движений нет",
      s2 == 200 and voided == "t" and stock(gid) == st1 and m2 == [row(-1, "order", "заказ кухни от стойки", A1, order=oid),
                                                                   row(1, "void", "отмена заказа кухни", A1, order=oid)], J([s2, voided, m2], 600))

section("итог")
tot = sql(f"SELECT COALESCE(SUM(delta), 0) FROM stock_moves WHERE good_id = '{gid}'")
check(f"«{G}»: остаток {stock(gid)} = сумма всех его движений", str(stock(gid)) == tot, J([stock(gid), tot]))
done()
