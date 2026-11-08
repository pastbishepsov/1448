#!/usr/bin/env python3
"""З6-и2 «Позиция с историей не удаляется» (№31): позицию со складской историей (приход, продажи, списания) или
с ненулевым остатком удалить нельзя — 409 has_history «выключи позицию». Раньше удаление уносило остаток и все
движения склада, и «Потери» за прошлые дни менялись задним числом. Движение склада помнит цену на свой момент
(stock_moves.price_pln), и «Потери» считаются по ней: новая цена позиции прошлые потери не переписывает. Новая
позиция без движений и с нулевым остатком удаляется. Позиции «z6 бой» и «z6 с остатком» (выключенные) живут
на стенде между прогонами — удалить их нельзя, в этом и смысл; цена и остаток после прогона — как были (остаток
возвращается корректировкой, а не приходом: приход ждал бы цен у владельца, З18-и4ж)."""
import time
from _z import *

def good(name, price, stock):
    """Выключенная позиция стенда: найти или завести."""
    gid = sql(f"SELECT id FROM goods WHERE name='{name}'")
    if not gid:
        s, d = api("POST", "/admin/goods", {"name": name, "category": "z6", "price_pln": price, "stock": stock, "active": False}, "boss")
        gid = d.get("good", {}).get("id", "")
    return gid

losses = lambda: api("GET", "/admin/reports/money?preset=today", None, "boss")[1].get("losses", {})
moves = lambda gid: int(sql(f"SELECT count(*) FROM stock_moves WHERE good_id='{gid}'") or 0)

section("позиция с историей: «бой −3», потом попытка удалить")
g = good("z6 бой", 5, 10)
check("позиция «z6 бой» есть (5,00 zł)", g, g)
sql(f"UPDATE goods SET price_pln=5 WHERE id='{g}'")
st0 = int(sql(f"SELECT stock FROM goods WHERE id='{g}'"))
if st0 < 3:
    api("POST", f"/admin/goods/{g}/stock", {"delta": 10, "reason": "adjust", "note": "z6"}, "boss")
    st0 = int(sql(f"SELECT stock FROM goods WHERE id='{g}'"))
b = losses()
s, _ = api("POST", f"/admin/goods/{g}/stock", {"delta": -3, "reason": "adjust", "note": "z6: бой"}, "boss")
at_end(lambda: api("POST", f"/admin/goods/{g}/stock", {"delta": 3, "reason": "adjust", "note": "z6: вернуть остаток"}, "boss"))
a = losses()
check("«бой −3» → «Потери» +3 шт и +15,00 zł", s == 200 and a.get("units", 0) - b.get("units", 0) == 3
      and near(a.get("pln", 0) - b.get("pln", 0), 15), f"{s} {J(b)} → {J(a)}")
n = moves(g)
s, d = api("DELETE", f"/admin/goods/{g}", None, "boss")
check("удаление позиции с историей → 409 has_history", s == 409 and d.get("code") == "has_history", f"{s} {J(d)}")
st = sql(f"SELECT stock FROM goods WHERE id='{g}'")
check("позиция, остаток и движения на месте", st == str(st0 - 3) and moves(g) == n, f"остаток {st or 'позиции нет'}, движений {moves(g)} из {n}")
c = losses()
check("«Потери» за сегодня не изменились", c.get("units") == a.get("units") and near(c.get("pln", 0), a.get("pln", 0)), f"{J(a)} → {J(c)}")

section("«Потери» — по цене в момент движения")
has_price = sql("SELECT count(*) FROM information_schema.columns WHERE table_name='stock_moves' AND column_name='price_pln'") == "1"
check("у движения «бой −3» записана цена 5,00", has_price and sql(f"SELECT price_pln FROM stock_moves WHERE good_id='{g}' "
      "AND reason='adjust' ORDER BY created_at DESC LIMIT 1") == "5.00", has_price)
s, _ = api("PATCH", f"/admin/goods/{g}", {"price_pln": 50}, "boss")
at_end(lambda: sql(f"UPDATE goods SET price_pln=5 WHERE id='{g}'"))
e = losses()
check("цена позиции 5 → 50: «Потери» за сегодня те же", s == 200 and near(e.get("pln", 0), c.get("pln", 0)), f"{s} {J(c)} → {J(e)}")

section("остаток без движений и новая пустая позиция")
g2 = good("z6 с остатком", 3, 5)
s, d = api("DELETE", f"/admin/goods/{g2}", None, "boss")
check("позиция с начальным остатком 5 → 409 has_history", s == 409 and d.get("code") == "has_history", f"{s} {J(d)}")
name = f"z6 пустая {int(time.time())}"
s, d = api("POST", "/admin/goods", {"name": name, "category": "z6", "price_pln": 2, "stock": 0, "active": False}, "boss")
g3 = d.get("good", {}).get("id", "")
s2, _ = api("DELETE", f"/admin/goods/{g3}", None, "boss")
check("новая позиция без движений и с нулевым остатком удаляется → 200", s == 201 and s2 == 200
      and not sql(f"SELECT id FROM goods WHERE id='{g3}'"), f"{s} {s2}")
done()
