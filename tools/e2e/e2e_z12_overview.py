#!/usr/bin/env python3
"""З12-и4 «Экран «Прибыль»» — сервер «Обзора» (решение основателя 04.10: «общий дайджест, когда входишь в бухгалтерию,
где будет график общих данных»). GET /admin/reports/profit рядом с итогами отдаёт: ряд по клубным суткам (`days`: доходы,
возвраты, себестоимость, потери, расходы, затраты и результат дня — по тем же правилам, что итоги, Σ дней = итог; до 92
дней, длиннее — без ряда), затраты одной цифрой (`costs_pln` = возвраты + себестоимость + потери + расходы; у прошлого
периода — тоже, рост — `delta.costs`) и «сейчас» (`now.drawer_pln` — «должно быть в ящике», как в кассовой книге).
Шаги двигают ровно свою строку ряда: пополнение картой — доход сегодня, его отмена с возвратом на карту — возврат
сегодня; продажа и подарок позиции с ценой партии — доход и себестоимость сегодня; бой — потери сегодня; расход
вчерашним днём операции — в строке вчера, сегодня не тронут.
Данные: гость Guest43; позиция «z12o Товар» (6 zł) и её поставка 5 шт по 2 zł (расход поставки сторнируется) заводятся
каждый прогон и в конце удаляются; расход 30 zł вчерашним днём в первой видимой категории не-закупки (заметка «z12o…»,
в конце сторнируется); продажи отменяются в уборке; наличные не трогаются — всё картой и банком."""
from _z import *

NAME, MARK = "z12o Товар", "z12o"
CAT = (sql("SELECT id FROM expense_categories WHERE NOT purchase AND NOT hidden ORDER BY sort, name LIMIT 1") or "")
TODAY, YDAY = club_day(), club_day(-1)
LINES = ("income", "returns", "cogs", "losses", "expenses", "costs", "result")
g = lambda v: round((v or 0) * 100)


def gone():
    for sid in rows(f"SELECT id FROM sales WHERE name = '{NAME}' AND voided_at IS NULL"):
        api("POST", f"/admin/sales/{sid}/void", {"refund_method": "none"}, "boss")
    for eid in rows(f"SELECT id FROM expenses WHERE storno_at IS NULL AND (note LIKE '{MARK}%' OR note LIKE '%{NAME}%')"):
        api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{NAME}'"):
        drop_good(gid)


gone()
at_end(gone)


def profit(q="preset=today"):
    return api("GET", f"/admin/reports/profit?{q}", None, "boss")[1]


def totals(p):
    """Итоги «Прибыли» в грошах — те строки, что есть у дня."""
    return {"income": g((p.get("income") or {}).get("pln")), "returns": g((p.get("returns") or {}).get("pln")),
            "cogs": g((p.get("cogs") or {}).get("pln")), "losses": g((p.get("losses") or {}).get("pln")),
            "expenses": g((p.get("expenses") or {}).get("pln")), "costs": g(p.get("costs_pln")), "result": g(p.get("result_pln"))}


def day(r):
    return {k: g(r.get(k + "_pln")) for k in LINES}


def series_ok(p):
    """Ряд без дыр от начала до конца периода; у каждого дня затраты = возвраты + себестоимость + потери + расходы,
    результат = доходы − затраты; Σ дней = итог по каждой строке."""
    per, ds = p.get("period") or {}, p.get("days") or []
    dates = [r.get("date") for r in ds]
    want = rows(f"SELECT to_char(d, 'YYYY-MM-DD') FROM generate_series('{per.get('from')}'::date, '{per.get('to')}'::date, '1 day') d")
    bad = []
    for r in ds:
        x = day(r)
        if x["costs"] != x["returns"] + x["cogs"] + x["losses"] + x["expenses"] or x["result"] != x["income"] - x["costs"]:
            bad.append(r.get("date"))
    t = totals(p)
    sums = {k: sum(day(r)[k] for r in ds) for k in LINES}
    diff = {k: (sums[k], t[k]) for k in LINES if sums[k] != t[k]}
    return dates == want and len(ds) == per.get("days") and not bad and not diff, {"days": len(ds), "period": per, "bad": bad, "diff": diff}


section("ряд по дням: Σ дней = итог, строки дня сходятся")
for preset in ("today", "yesterday", "d7", "d30", "month", "prev_month", "d90"):
    p = profit("preset=" + preset)
    ok, why = series_ok(p)
    check(f"{preset}: ряд без дыр на все {(p.get('period') or {}).get('days')} дн.; Σ дней = итог по доходам, возвратам, "
          f"себестоимости, потерям, расходам, затратам и результату", bool(p.get("days")) and ok, J(why))

section("затраты одной цифрой, прошлый период, длинные периоды, «сейчас»")
P = profit("preset=d30")
t, pv, dl = totals(P), P.get("prev") or {}, P.get("delta") or {}
old = {k: g(pv.get(k + "_pln")) for k in ("returns", "cogs", "losses", "expenses", "costs", "income", "result")}
pct = None if old["costs"] == 0 and t["costs"] != 0 else 0.0 if old["costs"] == 0 else round((t["costs"] - old["costs"]) / old["costs"] * 1000) / 10
check("затраты = возвраты + себестоимость + потери + расходы — и за период, и за прошлый; рост затрат — как у строк, в процентах",
      t["costs"] == t["returns"] + t["cogs"] + t["losses"] + t["expenses"] and t["costs"] == t["income"] - t["result"]
      and old["costs"] == old["returns"] + old["cogs"] + old["losses"] + old["expenses"] and old["costs"] == old["income"] - old["result"]
      and (dl.get("costs") == pct or (pct is not None and dl.get("costs") is not None and abs(dl["costs"] - pct) < 0.051)),
      J({"t": t, "old": old, "delta": dl.get("costs"), "ждали": pct}))
Y = profit("preset=year")
c92 = profit(f"from={club_day(-91)}&to={TODAY}")
c93 = profit(f"from={club_day(-92)}&to={TODAY}")
check("ряд — до 92 дней, как кассовая книга: 92 дня — с рядом, 93 и год (длиннее 92 дней) — без ряда, итоги есть",
      len(c92.get("days") or []) == 92 and c93.get("days") is None and c93.get("income") is not None
      and ((Y.get("period") or {}).get("days", 0) <= 92 or (Y.get("days") is None and Y.get("income") is not None)),
      J({"92": len(c92.get("days") or []), "93": c93.get("days"), "year": [(Y.get("period") or {}).get("days"), Y.get("days") is None]}))
bk = api("GET", "/admin/cash/book?preset=today", None, "boss")[1].get("now") or {}
nw = profit().get("now") or {}
check("«сейчас»: в ящике — «должно быть» кассовой книги, ящик не заведён — так же",
      bool(nw) and abs((nw.get("drawer_pln") or 0) - (bk.get("expected_pln") or 0)) < 0.005 and bool(nw.get("baseline")) == bool(bk.get("baseline")),
      J({"now": nw, "book": bk}))


def snap():
    p = profit("preset=d7")
    by = {r.get("date"): day(r) for r in p.get("days") or []}
    return {"today": by.get(TODAY), "yday": by.get(YDAY), "total": totals(p), "ok": series_ok(p)[0]}


def step(title, act, today=None, yday=None):
    """act() → что сделали; today / yday — ожидаемые приращения строк дня (итог недели — их сумма), остальное — 0."""
    a = snap()
    r = act()
    b = snap()
    zero = {k: 0 for k in LINES}
    want_t, want_y = {**zero, **(today or {})}, {**zero, **(yday or {})}
    dt = {k: b["today"][k] - a["today"][k] for k in LINES} if a["today"] and b["today"] else None
    dy = {k: b["yday"][k] - a["yday"][k] for k in LINES} if a["yday"] and b["yday"] else None
    dtot = {k: b["total"][k] - a["total"][k] for k in LINES}
    check(title, r and dt == want_t and dy == want_y and dtot == {k: want_t[k] + want_y[k] for k in LINES} and b["ok"],
          J({"сегодня": dt, "ждали": want_t, "вчера": dy, "ждали вчера": want_y, "неделя": dtot, "r": r}))
    return r


section("шаги: строка дня двигается вместе с итогом")
dep = step("пополнение 100 zł картой — сегодня доходы +100,00, результат +100,00",
           lambda: topup("Guest43", 100), {"income": 10000, "result": 10000})
step("отмена пополнения с возвратом на карту — сегодня возвраты +100,00, затраты +100,00, результат −100,00 (доход дня остаётся)",
     lambda: api("POST", f"/admin/deposits/{dep}/void", {"refund_method": "card"}, "boss")[1].get("refund_method") == "card",
     {"returns": 10000, "costs": 10000, "result": -10000})
good = (api("POST", "/admin/goods", {"name": NAME, "category": "z12", "price_pln": 6}, "boss")[1].get("good") or {}).get("id")


def supply():
    sup = (api("POST", "/admin/supplies", {"lines": [{"good_id": good, "qty": 5}], "note": f"{MARK}: приход"}, "boss")[1].get("supply") or {}).get("id")
    return api("PUT", f"/admin/supplies/{sup}/pricing", {"lines": [{"good_id": good, "prices": [{"qty": 5, "price_pln": 2}]}], "method": "bank"}, "boss")[0] == 200


step("поставка 5 шт по 2 zł с ценой — закупка не расход (Д9): ряд не тронут", supply)
sell = lambda method, **kw: api("POST", "/admin/sales", {"good_id": good, "qty": 1, "method": method, **kw}, "boss")
step("продажа 1 × 6 zł картой — сегодня доходы +6,00, себестоимость +2,00 (цена партии), затраты +2,00, результат +4,00",
     lambda: sell("card")[0] == 201, {"income": 600, "cogs": 200, "costs": 200, "result": 400})
step("подарок 1 шт — сегодня себестоимость подаренного +2,00, результат −2,00",
     lambda: sell("comp", comp_reason=f"{MARK}: проверка обзора")[0] == 201, {"cogs": 200, "costs": 200, "result": -200})
step("бой 1 шт — сегодня потери склада +2,00 по цене партии, результат −2,00",
     lambda: api("POST", f"/admin/goods/{good}/stock", {"delta": -1, "reason": "adjust", "note": f"{MARK}: бой"}, "boss")[0] == 200,
     {"losses": 200, "costs": 200, "result": -200})
step("расход 30 zł днём операции «вчера» — в строке вчера расходы +30,00, результат −30,00; сегодня не тронут",
     lambda: (api("POST", "/admin/expenses", {"op_date": YDAY, "category_id": CAT, "amount_pln": 30, "method": "bank",
                                               "note": f"{MARK}: проверка обзора"}, "boss")[1].get("expense") or {}).get("id"),
     None, {"expenses": 3000, "costs": 3000, "result": -3000})
done()
