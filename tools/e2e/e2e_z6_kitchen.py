#!/usr/bin/env python3
"""З6-и1 «Отмена расчёта кухни» (№14, З-Р11): продажа, которая рассчитала заказ кухни, при отмене склад не
трогает — заказ списал товар при заказе, съеденное назад не вернуть, — а заказ снова ждёт оплаты: оплата
сброшена, заказ в долге гостя, и его можно рассчитать заново правильным способом. Оплата кошельком
возвращается гостю, как и раньше. Раньше отмена возвращала товар на склад (фантомная «потеря» при
инвентаризации), а заказ оставался «оплаченным» — перерассчитать было нельзя (409 already_paid).
Guest38, «Woda 0.5» от стойки: картой и кошельком. Идемпотентен: остаток товара возвращается к исходному,
кошелёк докладывается до 10 zł; заказы остаются рассчитанными."""
from _z import *

g = uid("Guest38")
W = sql("SELECT id FROM goods WHERE name='Woda 0.5'")
stock = lambda: int(sql(f"SELECT stock FROM goods WHERE id='{W}'"))
stock0 = stock()
at_end(lambda: stock_to(W, stock0, "z6 уборка: остаток как до проверки"))  # З10-и3: не SQL — партии не разойдутся с остатком
order_row = lambda oid: sql(f"SELECT COALESCE(pay_method,'—') || '|' || (paid_at IS NULL)::text || '|' || (sale_id IS NULL)::text FROM kitchen_orders WHERE id='{oid}'")
in_debt = lambda oid: any(i.get("id") == oid and i.get("kind") == "kitchen"
                          for i in api("GET", f"/admin/users/{g}/debt", None, "admin1")[1].get("items", []))

def counter_order(method):
    """Заказ от стойки с оплатой сразу → (id заказа, id продажи); сразу «выдан»."""
    s, d = api("POST", "/admin/kitchen/orders", {"guest_id": g, "good_id": W, "qty": 1, "method": method}, "admin1")
    oid, sid = d.get("order", {}).get("id"), d.get("sale_id")
    if oid:
        api("POST", f"/admin/kitchen/orders/{oid}/status", {"status": "done"}, "admin1")
    return s, oid, sid

section("заказ картой → отмена продажи → заказ снова ждёт оплаты, склад не тронут")
s, oid, sid = counter_order("card")
check("заказ «Woda 0.5» картой от стойки → 201, продажа есть, остаток −1", s == 201 and sid and stock() == stock0 - 1, f"{s} {stock()} из {stock0}")
s, v = api("POST", f"/admin/sales/{sid}/void", {}, "admin1")
check("отмена продажи → 200", s == 200, f"{s} {J(v)}")
check("склад не тронут: выпитое назад не возвращается", stock() == stock0 - 1, f"{stock()} (до заказа {stock0})")
check("оплата у заказа сброшена: способа нет, paid_at и sale_id пусты", order_row(oid) == "—|true|true", order_row(oid))
check("заказ — в долге гостя («Должен»)", in_debt(oid))
check("в ответе отмены — «заказ кухни снова ждёт оплаты»", v.get("kitchen_unpaid") is True, J(v))
s, p = api("POST", f"/admin/kitchen/orders/{oid}/pay", {"method": "cash"}, "admin1")
check("повторный расчёт наличными → 200", s == 200 and p.get("sale_id"), f"{s} {J(p)}")
check("после расчёта: способ cash, долга нет, склад по-прежнему −1", order_row(oid) == "cash|false|false" and not in_debt(oid)
      and stock() == stock0 - 1, f"{order_row(oid)} · {stock()}")

section("кошельком: отмена возвращает деньги гостю, заказ снова в долге")
ensure_wallet("Guest38", 10)
w0 = wallet("Guest38")
s, oid2, sid2 = counter_order("wallet")
check("заказ кошельком → 201, с кошелька −4,00 zł", s == 201 and sid2 and wallet("Guest38") == w0 - 400, f"{s} {wallet('Guest38')} из {w0}")
st = stock()
s, v = api("POST", f"/admin/sales/{sid2}/void", {}, "admin1")
check("отмена → 200: 4,00 zł вернулись на кошелёк, склад не тронут, заказ в долге", s == 200 and wallet("Guest38") == w0
      and stock() == st and order_row(oid2) == "—|true|true" and in_debt(oid2), f"{s} кошелёк {wallet('Guest38')} склад {stock()} заказ {order_row(oid2)}")
s, p = api("POST", f"/admin/kitchen/orders/{oid2}/pay", {"method": "wallet"}, "admin1")
check("повторный расчёт кошельком → 200, с кошелька снова −4,00 zł", s == 200 and wallet("Guest38") == w0 - 400, f"{s} {wallet('Guest38')}")
done()
