#!/usr/bin/env python3
"""З8-и6 «Отмена оплаченного заказа кухни возвращает деньги» (хвост З8, «Найдено по дороге» З8-и5): заказ кухни от
стойки, оплаченный сразу (Ж6-и3: нал, карта, BLIK, кошелёк), при отмене отменяет и свою продажу в той же транзакции.
Кошелёк возвращается гостю, выручка и «должно быть в ящике» уменьшаются, товар возвращается на склад один раз; в
ответе — что вернули, в журнале — «возврат 4.00 zł …». Права — как у отмены продажи: чужую или вчерашнюю продажу
админ не отменяет — такой заказ отменяет владелец. Заказ с оплатой при выходе отменяется как раньше, без денег.
Раньше товар возвращался, а деньги — нет: продажа оставалась в силе.
Временный сотрудник z8rf, Guest39 (кошелёк докладывается до 10 zł), «Woda 0.5»; Guest39 за PC-06 — для заказа с
оплатой при выходе. Всё, что проверка оплатила, в конце отменено: деньги и товар — как были, z8rf уволен."""
from _z import *

hire_temp("z8rf")
tok("z8rf")  # вход в админку открывает смену
drawer_ready("z8rf")
g = uid("Guest39")
W = sql("SELECT id FROM goods WHERE name='Woda 0.5'")
stock = lambda: int(sql(f"SELECT stock FROM goods WHERE id='{W}'"))
preview = lambda: api("GET", "/admin/work/closeout", None, "z8rf")[1]
goods_today = lambda: api("GET", "/admin/reports/money?preset=today", None, "boss")[1]["totals"]["goods_pln"]
sale_live = lambda sid: sql(f"SELECT (voided_at IS NULL)::text FROM sales WHERE id='{sid}'") == "true"
row = lambda oid: sql(f"SELECT status || '|' || (paid_at IS NULL)::text || '|' || (sale_id IS NULL)::text FROM kitchen_orders WHERE id='{oid}'")
orders, sales = [], []


def cleanup():
    """заказы проверки — отменены, их продажи — отменены (на HEAD отмена заказа продажу не трогала)"""
    for oid in orders:
        if sql(f"SELECT status FROM kitchen_orders WHERE id='{oid}'") not in ("cancelled", "done"):
            api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for sid in sales:
        if sale_live(sid):
            api("POST", f"/admin/sales/{sid}/void", {}, "boss")
at_end(cleanup)


def order(method, who="z8rf"):
    s, d = api("POST", "/admin/kitchen/orders", {"guest_id": g, "good_id": W, "qty": 1, "method": method}, who)
    oid, sid = (d.get("order") or {}).get("id"), d.get("sale_id")
    orders.append(oid) if oid else None
    sales.append(sid) if sid else None
    return s, oid, sid


cancel = lambda oid, who="z8rf": api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, who)

section("кошелёк: отмена возвращает деньги на кошелёк гостя")
ensure_wallet("Guest39", 10)
w0, st0 = wallet("Guest39"), stock()
s, oid, sid = order("wallet")
check("заказ «Woda 0.5» кошельком от стойки → 201: с кошелька −4,00 zł, склад −1, продажа есть",
      s == 201 and sid and wallet("Guest39") == w0 - 400 and stock() == st0 - 1, f"{s} {wallet('Guest39')} из {w0} · склад {stock()}")
s, d = cancel(oid)
check("отмена → 200, в ответе — возврат 4,00 zł на кошелёк", s == 200 and (d.get("refund") or {}).get("method") == "wallet"
      and near((d.get("refund") or {}).get("pln", 0), 4), f"{s} {J(d)}")
check("кошелёк — как до заказа, продажа отменена, товар на складе один раз, заказ отменён без оплаты",
      wallet("Guest39") == w0 and not sale_live(sid) and stock() == st0 and row(oid) == "cancelled|true|true",
      f"кошелёк {wallet('Guest39')} из {w0} · продажа живая: {sale_live(sid)} · склад {stock()} из {st0} · {row(oid)}")
s, d = cancel(oid)
check("повторная отмена → 409, второй раз деньги не возвращаются", s == 409 and d.get("code") == "kitchen_state"
      and wallet("Guest39") == w0, f"{s} {J(d)} · кошелёк {wallet('Guest39')}")

section("наличные: отмена уменьшает выручку и «должно быть в ящике»")
e0, g0 = preview().get("expected_pln", 0), goods_today()
s, oid, sid = order("cash")
e1, g1 = preview().get("expected_pln", 0), goods_today()
check("заказ за наличные → 201: в ящике и в выручке товаров +4,00", s == 201 and near(e1, e0 + 4) and near(g1, g0 + 4),
      f"{s} ящик {e0}→{e1} · выручка {g0}→{g1}")
s, d = cancel(oid)
e2, g2 = preview().get("expected_pln", 0), goods_today()
check("отмена → 200, «верни 4,00 zł наличными»: в ящике и в выручке — как до заказа, продажа отменена",
      s == 200 and (d.get("refund") or {}).get("method") == "cash" and near(e2, e0) and near(g2, g0) and not sale_live(sid),
      f"{s} {J(d)} · ящик {e1}→{e2} · выручка {g1}→{g2}")

section("права — как у отмены продажи: чужую продажу отменяет владелец")
s, oid, sid = order("card")
s, d = cancel(oid, "admin2")
check("admin2 отменяет заказ, оплаченный продажей z8rf, → 403 not_yours «…может владелец», заказ и продажа на месте",
      s == 403 and d.get("code") == "not_yours" and "владелец" in d.get("message", "") and sale_live(sid)
      and row(oid).startswith("accepted|"), f"{s} {J(d)} · {row(oid)}")
s, d = cancel(oid, "boss")
check("владелец → 200, возврат 4,00 zł на карту, продажа отменена", s == 200 and (d.get("refund") or {}).get("method") == "card"
      and not sale_live(sid), f"{s} {J(d)}")

section("оплата при выходе — отмена как раньше, без денег")
end_all("Guest39")
at_end(lambda: end_all("Guest39"))
seat("Guest39", "PC-06")
st1 = stock()
s, oid, sid = order("later")
s, d = cancel(oid)
check("заказ «при выходе» → отмена 200 без возврата денег (продажи не было), товар — на склад",
      s == 200 and "refund" not in d and sid is None and stock() == st1 and row(oid) == "cancelled|true|true", f"{s} {J(d)} · склад {stock()} из {st1}")

section("журнал")
lines = [i.get("text", "") for i in audit(kind="kitchen_cancel", limit=10)]
check("«кухня: заказ отменён» — с суммой и куда вернули: на кошелёк гостя, наличными, на карту",
      any("возврат 4.00 zł на кошелёк гостя" in t for t in lines) and any("возврат 4.00 zł наличными" in t for t in lines)
      and any("возврат 4.00 zł на карту" in t for t in lines), J(lines[:4], 400))
done()
