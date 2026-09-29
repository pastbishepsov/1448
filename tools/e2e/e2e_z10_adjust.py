#!/usr/bin/env python3
"""З10-и7 «Корректировки с выбором партии и «Потери» по себестоимости» (№68, №70; решение основателя 29.09). GET
/admin/goods/:id/lots — партии позиции с остатком по порядку списания (только владелец): откуда, пришло, осталось, цена.
POST /admin/goods/:id/stock с lot_id — «бой» из выбранной партии (только владелец, только минус, только партия этой
позиции; в партии меньше — 409); админ списывает со старых партий первыми (FIFO). «Потери» в отчёте «Деньги»: штуки и по
рознице — как были, плюс по себестоимости (по цене партий, из которых ушло) и «без себестоимости: N шт». PUT
/admin/lots/:id/price — цена за штуку партиям без поставки: начальный остаток и «нашлось» (партия поставки — 409), без
расхода; в «Закупках» они списком. Журнал «lot_price» — только владельцу.
Данные: позиции «z10 Бой A» и «z10 Бой B» (категория z10) заводятся каждый прогон и в конце удаляются drop_good вместе с
поставками; расходы их поставок в конце сторнируются (заметка «… z10 Бой …»). Потери считаются приростом за прогон."""
import uuid as uuidlib

from _z import *

A, B, MARK = "z10 Бой A", "z10 Бой B", "z10 бой"


def gone():
    for eid in rows("SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%z10 Бой%'"):
        api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for n in (A, B):
        for gid in rows(f"SELECT id FROM goods WHERE name = '{n}'"):
            drop_good(gid)


gone()
at_end(gone)
mk = lambda name: api("POST", "/admin/goods", {"name": name, "category": "z10", "price_pln": 5}, "boss")[1].get("good", {}).get("id")
ga, gb = mk(A), mk(B)
recv = lambda gid, n: (api("POST", "/admin/supplies", {"lines": [{"good_id": gid, "qty": n}], "note": f"{MARK}: приход"}, "admin1")[1].get("supply") or {}).get("id")
stock = lambda gid: int(sql(f"SELECT stock FROM goods WHERE id = '{gid}'") or -1)
left = lambda lid: int(sql(f"SELECT qty_left FROM stock_lots WHERE id = '{lid}'") or -1) if lid else -1  # партии нет (старый сервер)
lots_ok = lambda: api("GET", "/admin/goods/lots/check", None, "boss")[1].get("ok") is True
losses = lambda: api("GET", "/admin/reports/money?preset=today", None, "boss")[1].get("losses") or {}
adj = lambda gid, d, who, lot=None, reason="adjust": api("POST", f"/admin/goods/{gid}/stock",
                                                        {"delta": d, "reason": reason, "note": f"{MARK}: бой", **({"lot_id": lot} if lot else {})}, who)
L0 = losses()

section("партии позиции")
s1, s2 = recv(ga, 10), recv(ga, 6)
st1, _ = api("PUT", f"/admin/supplies/{s1}/pricing", {"lines": [{"good_id": ga, "prices": [{"qty": 10, "price_pln": 2}]}], "method": "bank"}, "boss")
st2, _ = adj(ga, 3, "admin1")  # «нашлось» — партия без цены (Д5)
s, d = api("GET", f"/admin/goods/{ga}/lots", None, "boss")
ls = d.get("lots", [])
view = [(l.get("kind"), l.get("qty_in"), l.get("qty_left"), l.get("price_pln"), (l.get("title") or "").split(" ")[0]) for l in ls]
check("владелец видит партии «окна» по порядку списания: поставка 10 @2,00, поставка 6 без цены, «нашлось» 3 без цены; админу — 403",
      (st1, st2, s) == (200, 200, 200) and view == [("supply", 10, 10, 2, "поставка"), ("supply", 6, 6, None, "поставка"), ("found", 3, 3, None, "нашлось")]
      and api("GET", f"/admin/goods/{ga}/lots", None, "admin1")[0] == 403, J([st1, st2, s, view]))
lot1, lot2, found = (ls + [{}, {}, {}])[0].get("id"), (ls + [{}, {}, {}])[1].get("id"), (ls + [{}, {}, {}])[2].get("id")

section("бой из выбранной партии")
s1_, d1 = adj(ga, -2, "boss", found)
s2_, d2 = adj(ga, -3, "boss", lot2)
s3_, d3 = adj(ga, -1, "admin1")
check("владелец: −2 из «нашлось», −3 из второй поставки — списано ровно оттуда; админ: −1 — со старых первыми (первая поставка); сверка зелёная",
      (s1_, s2_, s3_) == (200, 200, 200) and (left(lot1), left(lot2), left(found)) == (9, 3, 1) and stock(ga) == 13 and lots_ok(),
      J([s1_, s2_, s3_, left(lot1), left(lot2), left(found), stock(ga)]))
bj = [x.get("text", "") for x in audit("admin1", limit=40) if A in (x.get("text") or "")]
check("журнал корректировки — «из партии: нашлось …», без цены (его видит и админ)",
      any(t.startswith(f"{A} · -2 → ") and "из партии: нашлось " in t and "zł" not in t for t in bj), J(bj[:4], 400))
recv(gb, 2)
bad = [adj(ga, -1, "admin1", found), adj(gb, -1, "boss", found), adj(ga, -5, "boss", found), adj(ga, 1, "boss", found),
       adj(ga, 1, "boss", lot1, "supply"), adj(ga, -1, "boss", "кривой")]
got = [(x[0], x[1].get("code")) for x in bad]
check("админ с партией — 403; партия чужой позиции, плюс с партией, приход с партией, кривой id — 400; в партии меньше — 409 «осталось 1 шт»; "
      "ничего не списалось", got == [(403, "lot_owner_only"), (400, "bad_lot"), (409, "lot_short"), (400, "bad_lot"), (400, "bad_lot"), (400, "bad_lot")]
      and "осталось 1 шт" in (bad[2][1].get("message") or "") and bad[2][1].get("left") == 1 and stock(ga) == 13 and left(found) == 1, J(got))

section("потери по себестоимости")
d = lambda k, a, b: round((b.get(k) or 0) - (a.get(k) or 0), 2)
L1 = losses()
check("«Потери» за прогон: 6 шт, по рознице 30,00 zł; по себестоимости 2,00 (1 шт первой поставки по 2,00); без себестоимости 5 шт "
      "(«нашлось» и вторая поставка без цены)", (d("units", L0, L1), d("pln", L0, L1), d("cost_pln", L0, L1), d("no_cost_units", L0, L1)) == (6, 30, 2, 5)
      and "cost_pln" in L1, J([L0, L1], 400))
st, _ = api("PUT", f"/admin/supplies/{s2}/pricing", {"lines": [{"good_id": ga, "prices": [{"qty": 6, "price_pln": 1.5}]}], "method": "bank"}, "boss")
L2 = losses()
check("оценили вторую поставку по 1,50 — «Потери» по себестоимости 6,50 (+3 × 1,50), без себестоимости 2 шт",
      st == 200 and (d("cost_pln", L0, L2), d("no_cost_units", L0, L2)) == (6.5, 2), J([st, d("cost_pln", L0, L2), d("no_cost_units", L0, L2)]))

section("цена партий без поставки")
p = api("GET", "/admin/purchases", None, "boss")[1]
fl = next((l for l in p.get("lots", []) if l.get("id") == found), {})
check("«Закупки»: партия «нашлось» — в списке без поставки, без цены; число без цены — больше нуля",
      fl.get("price_pln") is None and fl.get("title", "").startswith("нашлось ") and fl.get("name") == A and (p.get("lots_unpriced") or 0) >= 1, J(fl))
got = [api("PUT", f"/admin/lots/{lid}/price", body, who) for lid, body, who in (
    (found, {"price_pln": 1}, "admin1"), (lot1, {"price_pln": 1}, "boss"), (found, {"price_pln": -1}, "boss"),
    (found, {"price_pln": 1.005}, "boss"), (found, {}, "boss"), (str(uuidlib.uuid4()), {"price_pln": 1}, "boss"))]
got = [(x[0], x[1].get("code")) for x in got]
check("цену ставит только владелец (админу 403); партии поставки — 409 «оценка поставки»; меньше нуля, три знака, без цены — 400; нет партии — 404",
      got == [(403, "owner_only"), (409, "lot_supply"), (400, "bad_lot_price"), (400, "bad_lot_price"), (400, "bad_lot_price"), (404, "lot_not_found")], J(got))
s, dd = api("PUT", f"/admin/lots/{found}/price", {"price_pln": 1}, "boss")
L3 = losses()
p = api("GET", "/admin/purchases", None, "boss")[1]
fl = next((l for l in p.get("lots", []) if l.get("id") == found), {})
exp_n = sql(f"SELECT count(*) FROM expenses WHERE created_at > now() - interval '1 minute' AND note LIKE '%{A}%' AND supply_id IS NULL")
check("владелец поставил «нашлось» 1,00 → партия с ценой (осталось 1 — всё ещё в списке), «Потери» по себестоимости 8,50 (+2 × 1,00), "
      "без себестоимости 0; расхода нет", s == 200 and (dd.get("lot") or {}).get("price_pln") == 1 and fl.get("price_pln") == 1
      and (d("cost_pln", L0, L3), d("no_cost_units", L0, L3)) == (8.5, 0) and exp_n == "0", J([s, dd, fl, d("cost_pln", L0, L3), d("no_cost_units", L0, L3)]))
oa = [x.get("kind", "") + "|" + x.get("text", "") for x in audit("boss", limit=30)]
check("журнал владельца — «lot_price»: «z10 Бой A · нашлось … · 3 шт — 1,00 zł за штуку (было без цены)»; у админа его нет",
      any(x.startswith(f"lot_price|{A} · нашлось ") and x.endswith("· 3 шт — 1,00 zł за штуку (было без цены)") for x in oa)
      and not any(x.get("kind") == "lot_price" for x in audit("admin1", limit=100)), J([x for x in oa if x.startswith("lot_price")][:2], 300))
done()
