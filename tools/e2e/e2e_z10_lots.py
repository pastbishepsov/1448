#!/usr/bin/env python3
"""З10-и3 «Партии в работе»: каждое движение склада знает свои партии. Новая позиция и приход — поставка из одной
строки «ждёт цен» и её партия без цены; продажа, заказ кухни и «бой» — со старых партий первыми (FIFO); отмена
продажи и заказа — в те же партии; «нашлось» — новая партия без цены (Д5); продажа, сделанная до партий, при отмене —
в самую новую партию. Сверка GET /admin/goods/lots/check (владелец): у каждой позиции сумма остатков партий = остаток
— зелёная после каждого шага; остаток, поднятый мимо склада, сверка видит, а следующее списание дописывает недостающее
начальной партией. Админу сверка — 403.
Данные: позиция «z10 Партии» заводится каждый прогон и в конце удаляется SQL вместе с движениями, партиями и
поставками (продажи и заказы остаются без позиции, все отменены); Guest40 за PC-04 (кошелёк докладывается до 30 zł,
сессия в конце завершается). Продажи — картой: ящик не трогают."""
from _z import *

G, MARK = "z10 Партии", "z10 партии"
GUEST = uid("Guest40")


def gone():
    for gid in rows(f"SELECT id FROM goods WHERE name = '{G}'"):
        drop_good(gid)


gone()
at_end(gone)


def check_ok():
    s, d = api("GET", "/admin/goods/lots/check", None, "boss")
    return s == 200 and d.get("ok") is True, d


def lots(gid):
    """партии позиции по порядку FIFO: «откуда пришло/осталось цена статус_поставки»"""
    return rows(f"SELECT l.kind || ' ' || l.qty_in || '/' || l.qty_left || ' ' || COALESCE(l.price_grosz::text, '-') || ' ' || "
                f"COALESCE(s.status, '-') FROM stock_lots l LEFT JOIN supplies s ON s.id = l.supply_id "
                f"WHERE l.good_id = '{gid}' ORDER BY l.received_at, l.part, l.id")


def lot_ids(gid):
    return rows(f"SELECT id FROM stock_lots WHERE good_id = '{gid}' ORDER BY received_at, part, id")


def last_move(gid):
    return sql(f"SELECT id FROM stock_moves WHERE good_id = '{gid}' ORDER BY created_at DESC LIMIT 1")


def split(gid, mv):
    """доли движения по партиям: «номер партии (1 — самая старая):штук со знаком», по порядку партий"""
    ids = lot_ids(gid)
    got = rows(f"SELECT lm.lot_id || '|' || lm.qty FROM lot_moves lm JOIN stock_lots l ON l.id = lm.lot_id "
               f"WHERE lm.stock_move_id = '{mv}' ORDER BY l.received_at, l.part, l.id")
    return [f"{ids.index(x.split('|')[0]) + 1}:{int(x.split('|')[1]):+d}" for x in got]


def sale(gid, qty):
    s, d = api("POST", "/admin/sales", {"good_id": gid, "qty": qty, "method": "card"}, "admin1")
    return s, (d.get("sale") or {}).get("id")


section("приход — поставка «ждёт цен»")
s, d = api("POST", "/admin/goods", {"name": G, "category": "z10", "price_pln": 2, "stock": 10}, "boss")
gid = (d.get("good") or {}).get("id")
sup = sql(f"SELECT s.note || '|' || COALESCE(s.received_by::text, '-') FROM supplies s JOIN stock_lots l ON l.supply_id = s.id "
          f"WHERE l.good_id = '{gid}'") if gid else ""
ok, cd = check_ok()
check("новая позиция с остатком 10 → партия поставки 10/10 без цены, поставка «ждёт цен» с заметкой «начальный остаток» от владельца; "
      "сверка зелёная", s == 201 and lots(gid) == ["supply 10/10 - waiting"] and sup == f"начальный остаток|{uid('boss')}" and ok,
      J([s, lots(gid) if gid else None, sup, cd], 500))
s, _ = api("POST", f"/admin/goods/{gid}/stock", {"delta": 5, "reason": "supply", "note": f"{MARK}: приход"}, "admin1")
ok, cd = check_ok()
check("приход +5 от admin1 → вторая поставка и её партия 5/5; сверка зелёная", s == 200 and lots(gid) == ["supply 10/10 - waiting", "supply 5/5 - waiting"]
      and ok, J([s, lots(gid), cd], 400))

section("продажа и отмена — со старых партий и в те же")
s, sid = sale(gid, 12)
mv = last_move(gid)
ok1, _ = check_ok()
l1, p1 = lots(gid), split(gid, mv)
s2, _ = api("POST", f"/admin/sales/{sid}/void", {}, "admin1")
ok2, cd = check_ok()
check("продажа ×12 → 10 из первой партии и 2 из второй (остатки 0/10 и 3/5); отмена → в те же партии, снова 10 и 5; сверка зелёная оба раза",
      s == 201 and p1 == ["1:-10", "2:-2"] and l1 == ["supply 10/0 - waiting", "supply 5/3 - waiting"] and s2 == 200
      and split(gid, last_move(gid)) == ["1:+10", "2:+2"] and lots(gid) == ["supply 10/10 - waiting", "supply 5/5 - waiting"] and ok1 and ok2,
      J([s, p1, l1, s2, split(gid, last_move(gid)), lots(gid), cd], 600))

section("заказы кухни")
end_all("Guest40")
at_end(lambda: end_all("Guest40"))
at_end(lambda: [api("POST", f"/admin/kitchen/orders/{o}/cancel", {}, "admin1") for o in  # упавший прогон — заказы не висят в очереди
                 rows(f"SELECT id FROM kitchen_orders WHERE user_id = '{uid('Guest40')}' AND status IN ('new','accepted','preparing','delivering')")])
ensure_wallet("Guest40", 30)
seat("Guest40", "PC-04")
gt = login("Guest40")[1].get("access_token")
s, d = api("POST", "/me/kitchen/orders", {"good_id": gid, "qty": 3}, token=gt)
oid = (d.get("order") or {}).get("id")
p1, l1, ok1 = split(gid, last_move(gid)), (lots(gid) or ["-"])[0], check_ok()[0]
s2, _ = api("POST", f"/me/kitchen/orders/{oid}/cancel", {}, token=gt)
ok2, cd = check_ok()
check("заказ гостя ×3 → из первой партии (7/10); отменил → туда же (10/10); сверка зелёная оба раза",
      s == 201 and p1 == ["1:-3"] and l1 == "supply 10/7 - waiting" and s2 == 200 and split(gid, last_move(gid)) == ["1:+3"]
      and (lots(gid) or ["-"])[0] == "supply 10/10 - waiting" and ok1 and ok2, J([s, p1, l1, s2, lots(gid), cd], 500))
s, d = api("POST", "/admin/kitchen/orders", {"guest_id": GUEST, "good_id": gid, "qty": 2, "method": "card"}, "admin1")
oid = (d.get("order") or {}).get("id")
p1, ok1 = split(gid, last_move(gid)), check_ok()[0]
s2, _ = api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "admin1")
ok2, cd = check_ok()
nback = sql(f"SELECT count(*) FROM stock_moves WHERE good_id = '{gid}' AND kitchen_order_id = '{oid}' AND delta > 0") if oid else "?"
check("заказ от стойки картой ×2 → из первой партии; отмена → туда же, склад вернулся один раз; сверка зелёная оба раза",
      s == 201 and p1 == ["1:-2"] and s2 == 200 and split(gid, last_move(gid)) == ["1:+2"] and nback == "1"
      and (lots(gid) or ["-"])[0] == "supply 10/10 - waiting" and ok1 and ok2, J([s, p1, s2, nback, lots(gid), cd], 500))

section("корректировки")
s, _ = api("POST", f"/admin/goods/{gid}/stock", {"delta": -4, "reason": "adjust", "note": f"{MARK}: бой"}, "admin1")
p1, ok1 = split(gid, last_move(gid)), check_ok()[0]
s2, _ = api("POST", f"/admin/goods/{gid}/stock", {"delta": 2, "reason": "adjust", "note": f"{MARK}: нашлось"}, "admin1")
ok2, cd = check_ok()
check("«бой» −4 → из первой партии (6/10); «нашлось» +2 → новая партия «нашлось» 2/2 без цены (Д5); сверка зелёная оба раза",
      s == 200 and p1 == ["1:-4"] and s2 == 200 and lots(gid) == ["supply 10/6 - waiting", "supply 5/5 - waiting", "found 2/2 - -"]
      and split(gid, last_move(gid)) == ["3:+2"] and ok1 and ok2, J([s, p1, s2, lots(gid), cd], 500))

section("продажа до партий — в самую новую партию")
s, sid = sale(gid, 1)
mv = last_move(gid)
sql(f"DELETE FROM lot_moves WHERE stock_move_id = '{mv}' RETURNING id")  # как будто продали до партий: у движения нет долей
s2, _ = api("POST", f"/admin/sales/{sid}/void", {}, "admin1")
ok, cd = check_ok()
check("отмена продажи, у которой нет партий (продана до них), → в самую новую партию («нашлось» 3/2); сверка зелёная",
      s == 201 and s2 == 200 and split(gid, last_move(gid)) == ["3:+1"]
      and lots(gid) == ["supply 10/5 - waiting", "supply 5/5 - waiting", "found 2/3 - -"] and ok, J([s, s2, lots(gid), cd], 500))

section("остаток мимо склада")
sql(f"UPDATE goods SET stock = stock + 3 WHERE id = '{gid}' RETURNING id")  # как старый сервер во время обновления
ok1, cd1 = check_ok()
bad = next((x for x in cd1.get("mismatch", []) if x.get("good_id") == gid), {})
st = int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'"))
s, sid = sale(gid, st)
ok2, cd = check_ok()
check(f"остаток подняли SQL на 3 → сверка красная, у позиции разница +3; продажа всего остатка ({st}) → недостающее дописано начальной "
      "партией, все партии пусты, сверка снова зелёная",
      not ok1 and bad.get("diff") == 3 and bad.get("stock") == st and s == 201 and (lots(gid) or ["-"])[-1] == "initial 3/0 - -"
      and all(x.split(" ")[1].endswith("/0") for x in lots(gid)) and ok2, J([ok1, bad, s, lots(gid), cd], 600))
s2, _ = api("POST", f"/admin/sales/{sid}/void", {}, "admin1")
ok, cd = check_ok()
check("её отмена — всё обратно в те же партии, в том числе в дописанную; сверка зелёная", s2 == 200 and ok
      and lots(gid) == ["supply 10/5 - waiting", "supply 5/5 - waiting", "found 2/3 - -", "initial 3/3 - -"], J([s2, lots(gid), cd], 500))

section("только владельцу")
s, d = api("GET", "/admin/goods/lots/check", None, "admin1")
check("админу сверка партий — 403", s == 403, f"{s} {J(d)}")
done()
