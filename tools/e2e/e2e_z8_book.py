#!/usr/bin/env python3
"""З8-и3 «Кассовая книга» (№78): GET /admin/cash/book — ящик клуба по клубным дням, только владельцу (admin → 403).
Строка дня: на начало, пришло по видам, возвраты, инкассации, коррекции, расхождения, на конец. Дни сшиты
(конец дня — начало следующего), строка сходится (на начало + пришло − ушло ± коррекции ± расхождения = на
конец), суммы дня — те же, что в базе за окно дня, «на конец» сегодня — «должно быть» сдачи. Сдачи и события
дня — списком; отменённое событие — с пометкой и вне сумм. День нулевой точки начинается с «ящика ещё не было»
и начального остатка (изображается: у прошлых сдач на время прячется остаток на начало).
Временный сотрудник z8bk, гость Guest39: пополнение отменяется, события уходят в прошлый период сдачей z8bk,
z8bk уволен."""
from _z import *

hire_temp("z8bk")
tok("z8bk")  # вход в админку открывает смену
drawer_ready("z8bk")
deps = []
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in reversed(deps)])
book = lambda q="preset=today", who="boss": api("GET", f"/admin/cash/book?{q}", None, who)
preview = lambda: api("GET", "/admin/work/closeout", None, "z8bk")[1]
cents = lambda q: round(float(sql(q) or 0), 2)


def adds_up(r):
    """строка дня сходится: на начало + пришло − ушло ± коррекции ± расхождения = на конец"""
    if r.get("opening_pln") is None:
        return True
    return near(r["opening_pln"] + r["in_pln"] - r["out_pln"] + r["adjust_pln"] + r["diff_pln"], r["closing_pln"])


def from_db(r):
    """те же суммы дня прямо из базы — за окно [from, to); нет окна — None"""
    if not r.get("from") or not r.get("to"):
        return None
    w = lambda col: f"{col} >= '{r['from']}' AND {col} < '{r['to']}'"
    return {
        "deposits_pln": cents(f"SELECT COALESCE(SUM(amount_pln),0) FROM deposits WHERE method='cash' AND {w('created_at')}"),
        "goods_pln": cents(f"SELECT COALESCE(SUM(total_pln),0) FROM sales WHERE method='cash' AND {w('paid_at')}"),
        "packs_pln": cents(f"SELECT COALESCE(SUM(price_pln),0) FROM user_packages WHERE method='cash' AND {w('created_at')}"),
        "refunds_pln": cents(f"SELECT (SELECT COALESCE(SUM(refund_grosz),0) FROM deposits WHERE method='cash' AND {w('voided_at')})"
                             f" / 100.0 + (SELECT COALESCE(SUM(total_pln),0) FROM sales WHERE method='cash' AND paid_at IS NOT NULL"
                             f" AND {w('voided_at')}) + (SELECT COALESCE(SUM(refund_grosz),0) FROM user_packages WHERE method='cash'"
                             f" AND {w('voided_at')}) / 100.0"),
        "collections_pln": cents(f"SELECT COALESCE(SUM(amount_grosz),0) / 100.0 FROM cash_events WHERE kind='collect' AND voided_at IS NULL AND {w('created_at')}"),
        "adjust_pln": cents(f"SELECT COALESCE(SUM(amount_grosz),0) / 100.0 FROM cash_events WHERE kind='correct' AND voided_at IS NULL AND {w('created_at')}"),
    }


section("только владельцу")
s, d = book(who="admin2")
check("admin2 → 403", s == 403, f"{s} {J(d)}")
s, d = book("preset=year")
check("год — длиннее 92 дней → 400 too_long", s == 400 and d.get("code") == "too_long", f"{s} {J(d)}")

section("сегодня: движение, события, «на конец» = «должно быть»")
s, d = book()
t0 = (d.get("days") or [{}])[0]
check("владелец → 200: один день — сегодня, строка сходится", s == 200 and len(d.get("days", [])) == 1 and t0.get("today") is True
      and adds_up(t0), f"{s} {J(t0, 300)}")
deps.append(topup("Guest39", 40, method="cash", who="z8bk"))
s, e1 = api("POST", "/admin/cash/events", {"kind": "collect", "amount_pln": 30, "dest": "safe", "note": "по указанию владельца"}, "z8bk")
p = preview()
s, e2 = api("POST", "/admin/cash/events", {"kind": "correct", "stated_pln": round(p.get("expected_pln", 0) + 5, 2), "note": "нашлось 5 zł"}, "boss")
s, _ = api("POST", f"/admin/cash/events/{e1.get('event', {}).get('id')}/void", {}, "boss")
t1 = book()[1].get("days", [{}])[0]
check("пополнение 40 — в «пришло», отменённая инкассация — вне сумм, коррекция +5",
      near(t1.get("deposits_pln", 0) - t0.get("deposits_pln", 0), 40) and near(t1.get("collections_pln", -1), t0.get("collections_pln", 0))
      and near(t1.get("adjust_pln", 0) - t0.get("adjust_pln", 0), 5), J({k: t1.get(k) for k in ("deposits_pln", "collections_pln", "adjust_pln")}))
ev = {x.get("id"): x for x in t1.get("events", [])}
c1, c2 = ev.get(e1.get("event", {}).get("id"), {}), ev.get(e2.get("event", {}).get("id"), {})
check("события дня: инкассация z8bk — отменена владельцем, коррекция владельца — «в ящике …», поправка +5",
      c1.get("nickname") == "z8bk" and c1.get("voided_at") and c1.get("voided_by_nickname") == "boss"
      and c2.get("nickname") == "boss" and near(c2.get("amount_pln", 0), 5) and c2.get("note") == "нашлось 5 zł", J([c1, c2], 400))
check("«на конец» сегодня = «должно быть» сдачи, строка сходится", near(t1.get("closing_pln", -1), preview().get("expected_pln", -2)) and adds_up(t1),
      f"{t1.get('closing_pln')} / {preview().get('expected_pln')}")
db1 = from_db(t1)
check("суммы дня — те же, что в базе за окно дня", db1 is not None and all(near(t1.get(k, -1), v) for k, v in db1.items()),
      J({"книга": {k: t1.get(k) for k in (db1 or {})}, "база": db1}, 500))

section("сдача с недостачей — в расхождениях дня")
exp = preview().get("expected_pln", 0)
s, co = api("POST", "/admin/work/stop", {"counted_pln": round(exp - 3, 2)}, "z8bk")
t2 = book()[1].get("days", [{}])[0]
mine = [x for x in t2.get("closeouts", []) if x.get("nickname") == "z8bk"]
check("сдачи дня: z8bk — должно, пересчитано, расхождение −3", mine and near(mine[-1].get("diff_pln", 0), -3)
      and near(mine[-1].get("counted_pln", 0), exp - 3), J(mine[-1:] or t2.get("closeouts"), 300))
check("расхождения дня −3 к прежним, «на конец» — пересчитанное, строка сходится", near(t2.get("diff_pln", 0) - t1.get("diff_pln", 0), -3)
      and near(t2.get("closing_pln", -1), exp - 3) and adds_up(t2), J({k: t2.get(k) for k in ("diff_pln", "closing_pln")}))

section("неделя: дни сшиты, каждый сходится и совпадает с базой")
s, d = book("preset=d7")
days = d.get("days", [])
check("7 дней, последний — сегодня", s == 200 and len(days) == 7 and days[-1].get("today") is True, f"{s} {len(days)}")
check("конец дня = начало следующего", all(near(a.get("closing_pln") or 0, b.get("opening_pln") or 0)
                                            for a, b in zip(days, days[1:])), J([(x["date"], x.get("opening_pln"), x.get("closing_pln")) for x in days]))
check("каждая строка сходится", all(adds_up(x) for x in days), J([x["date"] for x in days if not adds_up(x)]))
yday = days[-2] if len(days) > 1 else {}
dby = from_db(yday)
check(f"вчера ({yday.get('date')}): суммы — те же, что в базе", dby is not None and all(near(yday.get(k, -1), v) for k, v in dby.items()),
      J({"книга": {k: yday.get(k) for k in (dby or {})}, "база": dby}, 500))
check("итог периода: пришло — сумма дней, на конец — последний день",
      near(d.get("totals", {}).get("in_pln", -1), sum(x.get("in_pln", 0) for x in days)) and near(d.get("closing_pln", -1), days[-1].get("closing_pln", -2)))

section("день нулевой точки: «ящика ещё не было» и начальный остаток")
hidden = [r for r in rows("WITH old AS (SELECT id, opening_grosz FROM shift_closeouts WHERE opening_grosz IS NOT NULL) "
                          "UPDATE shift_closeouts s SET opening_grosz = NULL FROM old WHERE s.id = old.id "
                          "RETURNING s.id || '|' || old.opening_grosz") if "|" in r]


def unhide():
    if hidden:
        vals = ",".join(f"('{i}'::uuid, {v}::bigint)" for i, v in (h.split("|") for h in hidden))
        sql(f"UPDATE shift_closeouts s SET opening_grosz = v.o FROM (VALUES {vals}) AS v(id, o) WHERE s.id = v.id")
        hidden.clear()
at_end(unhide)
api("POST", "/admin/work/start", {}, "z8bk")
s, _ = api("POST", "/admin/work/stop", {"counted_pln": 350}, "z8bk")
t3 = book()[1].get("days", [{}])[0]
unhide()
b = t3.get("baseline") or {}
check("сегодня: на начало — ящика ещё не было, начальный остаток 350 (z8bk), на конец 350", t3.get("opening_pln") is None
      and near(b.get("pln", -1), 350) and b.get("by") == "z8bk" and near(t3.get("closing_pln", -1), 350), J({k: t3.get(k) for k in ("opening_pln", "baseline", "closing_pln")}, 300))
done()
