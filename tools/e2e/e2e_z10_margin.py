#!/usr/bin/env python3
"""З10-и8 «Маржа и стоимость склада» (№70; решение основателя 29.09). Отчёт «Деньги» — только владельцу: в «Что
покупают» у позиции выручка (живые деньги, как было), продано на (все способы — и кухня с кошелька, и в долг), штук,
себестоимость (по цене партий, из которых ушли проданные штуки) и маржа (продано на − себестоимость тех же штук, и в % от
продаж); то же по категориям и итогом. Штуки из партий без цены — «без себестоимости», в маржу не входят; оценили
партию или переоценили поставку — маржа пересчитана. «Стоимость склада сейчас» — остатки по себестоимости партий, без
цены — штуками, и по рознице. Админ не получает ни одной себестоимости: отчёт — 403, в его ответах (товары, поставки,
продажи, кухня, долг гостя, касса) нет ни цены закупки, ни маржи.
Данные: позиции «z10 Маржа A» (300 zł) и «z10 Маржа B» (200 zł) в категории «z10 Маржа» заводятся каждый прогон; поставка
A 10 шт оценена 6 × 100 + 4 × 120 (расход поставки в конце сторнируется); продажи A — BLIK, кухня с кошелька Guest40
(кошелёк докладывается до 900 zł), в долг (Guest40 за PC-04), картой, одна отменена; B — «нашлось» 5 шт без цены и 2
продажи картой. В конце заказы кухни отменяются (деньги — на кошелёк), продажи отменяются, позиции удаляются вместе с
поставкой, сессия Guest40 завершается. Итоги дня и склад — приростом за прогон."""
from _z import *

A, B, CAT, MARK, GUEST = "z10 Маржа A", "z10 Маржа B", "z10 Маржа", "z10 маржа", "Guest40"


def gone():
    for oid in rows(f"SELECT id FROM kitchen_orders WHERE name IN ('{A}', '{B}') AND status IN ('new','accepted','preparing','delivering')"):
        api("POST", f"/admin/kitchen/orders/{oid}/cancel", {}, "boss")
    for sid in rows(f"SELECT id FROM sales WHERE name IN ('{A}', '{B}') AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {}, "boss")
    for eid in rows("SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%z10 Маржа%'"):
        api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for n in (A, B):
        for gid in rows(f"SELECT id FROM goods WHERE name = '{n}'"):
            drop_good(gid)


gone()
end_all(GUEST)
at_end(lambda: end_all(GUEST))
at_end(gone)  # уборка идёт в обратном порядке: сначала продажи и долг, потом сессия
ensure_wallet(GUEST, 900)
guest = uid(GUEST)
mk = lambda name, price: (api("POST", "/admin/goods", {"name": name, "category": CAT, "price_pln": price}, "boss")[1].get("good") or {}).get("id")
money = lambda: api("GET", "/admin/reports/money?preset=today", None, "boss")[1]
row = lambda d, n: next((g for g in d.get("top_goods") or [] if g.get("name") == n), {})
cat = lambda d: next((g for g in d.get("top_categories") or [] if g.get("category") == CAT), {})
KEYS = ("revenue_pln", "sold_pln", "items", "cost_pln", "margin_pln", "margin_pct", "no_cost_units")
view = lambda r: tuple(r.get(k) for k in KEYS)
num = lambda v: v or 0
dd = lambda a, b, k: round(num(b.get(k)) - num(a.get(k)), 2)  # прирост за прогон (null — 0)
M0 = money()
G0, S0 = M0.get("goods_margin") or {}, M0.get("stock_value") or {}

section("поставка и «нашлось» — склад по себестоимости")
ga, gb = mk(A, 300), mk(B, 200)
sup = (api("POST", "/admin/supplies", {"lines": [{"good_id": ga, "qty": 10}], "note": f"{MARK}: приход"}, "admin1")[1].get("supply") or {}).get("id")
price = lambda a1: api("PUT", f"/admin/supplies/{sup}/pricing", {"lines": [{"good_id": ga, "prices": [{"qty": 6, "price_pln": a1},
                                                                                                   {"qty": 4, "price_pln": 120}]}], "method": "bank"}, "boss")
st1 = price(100)[0]
st2 = api("POST", f"/admin/goods/{gb}/stock", {"delta": 5, "reason": "adjust", "note": f"{MARK}: нашлось"}, "admin1")[0]
S1 = money().get("stock_value") or {}
check("«Стоимость склада сейчас» за прогон: +1 080,00 zł (6 × 100 + 4 × 120), по себестоимости +10 шт, без цены +5 шт («нашлось» B), "
      "по рознице +4 000,00 zł", (st1, st2) == (200, 200) and "cost_pln" in S1
      and (dd(S0, S1, "cost_pln"), dd(S0, S1, "costed_units"), dd(S0, S1, "no_cost_units"), dd(S0, S1, "retail_pln"), dd(S0, S1, "units")) == (1080, 10, 5, 4000, 15),
      J([st1, st2, S0, S1], 400))

section("продажи всеми способами")
seat(GUEST, "PC-04")
sale = lambda g, q, m, who="admin1", **kw: api("POST", "/admin/sales", {"good_id": g, "qty": q, "method": m, **kw}, who)
kitchen = lambda g, q: api("POST", "/admin/kitchen/orders", {"guest_id": guest, "good_id": g, "qty": q, "method": "wallet"}, "admin1")
got = [sale(ga, 3, "blik")[0], kitchen(ga, 2)[0], sale(ga, 1, "later", guest_id=guest)[0], sale(ga, 2, "card")[0]]
s5, d5 = sale(ga, 1, "blik")  # живые деньги мимо ящика — касса смены не меняется
got += [s5, api("POST", f"/admin/sales/{(d5.get('sale') or {}).get('id')}/void", {}, "admin1")[0], kitchen(ga, 1)[0], sale(gb, 2, "card")[0]]
M1 = money()
check("продано: A — BLIK 3, кухня с кошелька 2, в долг 1, карта 2, отменённая 1, кухня с кошелька 1; B — карта 2",
      got == [201, 201, 201, 201, 201, 200, 201, 201], J(got))
check("«Что покупают», A: выручка 1 500,00 (BLIK и карта — живые деньги, как было), продано на 2 700,00 (9 шт × 300 — и кошелёк, и долг), "
      "себестоимость 960,00 (6 × 100 + 3 × 120 — партии, из которых ушли), маржа 1 740,00 · 64,4%",
      view(row(M1, A)) == (1500, 2700, 9, 960, 1740, 64.4, 0), J(row(M1, A)))
check("B — из «нашлось» без цены: себестоимости и маржи нет (null), «без себестоимости» 2 шт",
      view(row(M1, B)) == (400, 400, 2, None, None, None, 2), J(row(M1, B)))
check("«По категориям», «z10 Маржа»: выручка 1 900,00, продано на 3 100,00, 11 шт, себестоимость 960,00, маржа 1 740,00 · 64,4% "
      "(штуки без цены в маржу не входят), без себестоимости 2 шт", view(cat(M1)) == (1900, 3100, 11, 960, 1740, 64.4, 2), J(cat(M1)))
G1 = M1.get("goods_margin") or {}
check("итог дня за прогон: продано на +3 100,00, штук +11, себестоимость +960,00, маржа +1 740,00, без себестоимости +2; выручка товаров "
      "+1 900,00 — как была (кошелёк и долг в неё не входят)", "margin_pln" in G1 and (dd(G0, G1, "sold_pln"), dd(G0, G1, "items"),
      dd(G0, G1, "cost_pln"), dd(G0, G1, "margin_pln"), dd(G0, G1, "no_cost_units")) == (3100, 11, 960, 1740, 2)
      and dd(M0.get("totals") or {}, M1.get("totals") or {}, "goods_pln") == 1900, J([G0, G1], 400))
S2 = M1.get("stock_value") or {}
check("склад за прогон: A — 1 шт по 120,00, B — 3 шт без цены; по рознице +900,00",
      (dd(S0, S2, "cost_pln"), dd(S0, S2, "costed_units"), dd(S0, S2, "no_cost_units"), dd(S0, S2, "retail_pln")) == (120, 1, 3, 900), J(S2))

section("оценили — маржа пересчитана")
lot = sql(f"SELECT id FROM stock_lots WHERE good_id = '{gb}' AND kind = 'found'") if gb else ""
st = api("PUT", f"/admin/lots/{lot}/price", {"price_pln": 50}, "boss")[0]
M2 = money()
check("«нашлось» B оценили по 50,00 → B: себестоимость 100,00, маржа 300,00 · 75%; категория: 1 060,00 и 2 040,00 · 65,8%, без себестоимости 0; "
      "склад: +270,00 (A 120 + B 3 × 50), без цены 0", st == 200 and view(row(M2, B)) == (400, 400, 2, 100, 300, 75, 0)
      and view(cat(M2)) == (1900, 3100, 11, 1060, 2040, 65.8, 0)
      and (dd(S0, M2.get("stock_value") or {}, "cost_pln"), dd(S0, M2.get("stock_value") or {}, "no_cost_units")) == (270, 0),
      J([st, row(M2, B), cat(M2), M2.get("stock_value")], 500))
st = price(90)[0]
M3 = money()
check("поставку A переоценили 6 × 90 + 4 × 120 → A: себестоимость 900,00 (6 × 90 + 3 × 120), маржа 1 800,00 · 66,7%; склад A — по-прежнему 120,00",
      st == 200 and view(row(M3, A)) == (1500, 2700, 9, 900, 1800, 66.7, 0) and dd(S0, M3.get("stock_value") or {}, "cost_pln") == 270,
      J([st, row(M3, A), M3.get("stock_value")], 400))

section("только владельцу")
s, _ = api("GET", "/admin/reports/money?preset=today", None, "admin1")
BAD = {"cost_pln", "margin_pln", "margin_pct", "price_grosz", "stock_value", "goods_margin", "top_categories", "costed_units",
       "no_cost_units", "expense_grosz", "lots", "lots_unpriced"}


def leaks(x, path=""):
    """ключи с себестоимостью или маржой в ответе (рекурсивно)"""
    if isinstance(x, dict):
        return [path + "." + k for k in x if k in BAD or "margin" in k or "cost" in k] + [p for k, v in x.items() for p in leaks(v, path + "." + k)]
    if isinstance(x, list):
        return [p for v in x for p in leaks(v, path + "[]")]
    return []


seen = {}
for path in ("/admin/goods", "/admin/supplies", "/admin/sales", "/admin/kitchen/orders", f"/admin/users/{guest}/debt", "/admin/work/closeout"):
    st, d = api("GET", path, None, "admin1")
    seen[path] = (st, leaks(d)[:3], "себестоим" in J(d, 10 ** 7))
check("админу отчёт «Деньги» — 403; в его ответах (товары, поставки, продажи, кухня, долг гостя, касса) — ни одной себестоимости и маржи",
      s == 403 and all(v[0] == 200 and not v[1] and not v[2] for v in seen.values()), J([s, seen], 600))
done()
