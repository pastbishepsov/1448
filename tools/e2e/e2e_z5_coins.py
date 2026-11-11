#!/usr/bin/env python3
"""З5-и2 «Монеты и минуты сходятся» (№10, З-Р9, Д3): отчёт «Монеты» сходится с балансами. Отмена выдачи
времени возвращает монеты пропорционально неотыгранному, и «Погашено» теперь уменьшается ровно на вернувшееся,
а не на всю выдачу: на приращениях «на руках» = выпущено − погашено − сгорело − потрачено. «Отдано временем» —
отыгранная часть выдачи. «Должны временем» — ещё и в злотых: зонные минуты по тарифу выдачи
(minute_grants.rate_pln), старый пул без зоны — по клубному тарифу. Guest37 на Standard (20 zł/ч, 20 монет
за 1 zł): выдача 60 минут → игра 30 минут за PC-06 → отмена. Идемпотентен: сессия удаляется, монеты
докладываются до 1000."""
from _z import *

g = uid("Guest37")
STD = sql("SELECT id FROM zones WHERE name='Standard'")
end_all("Guest37")
sql(f"UPDATE users SET coins_balance = GREATEST(coins_balance, 1000) WHERE id='{g}'")
has_rate = sql("SELECT count(*) FROM information_schema.columns WHERE table_name='minute_grants' AND column_name='rate_pln'") == "1"
has_ret = sql("SELECT count(*) FROM information_schema.columns WHERE table_name='coin_redemptions' AND column_name='coins_returned'") == "1"

def report():
    d = api("GET", "/admin/reports/coins?preset=today", None, "boss")[1]
    t, ow = d.get("totals", {}), d.get("minutes_owed", {})
    return d, {"на руках": d.get("on_hand", {}).get("coins", 0), "выпущено": t.get("issued", 0),
               "погашено": t.get("redeemed", 0), "сгорело": t.get("burned", 0), "потрачено": t.get("spent", 0),
               "отдано мин": t.get("minutes", 0), "отдано zł": t.get("given_pln", 0),
               "должны мин": ow.get("total", 0), "должны zł": ow.get("value_pln")}

def owed_pln_db():
    """«Должны временем» в злотых прямо по базе: гранты по своему тарифу, пул без зоны — по клубному."""
    if not has_rate:
        return None
    return float(sql("""SELECT ROUND(COALESCE((SELECT SUM(minutes_left * rate_pln / 60.0) FROM minute_grants), 0)
        + COALESCE((SELECT SUM(coin_minutes) FROM users), 0) * (SELECT base_rate_pln FROM clubs ORDER BY created_at LIMIT 1) / 60.0, 2)"""))

section("выдача 60 минут в Standard")
fresh_day(30)  # З19-и9: ночью игра «30 минут назад» и её монеты — ещё в сегодняшних сутках
_, b = report()
s, r = api("POST", f"/admin/users/{g}/redeem", {"zone_id": STD, "minutes": 60}, "admin1")
check("выдача → 200: 400 монет, 20 zł", s == 200 and r.get("coins") == 400 and near(r.get("value_pln", 0), 20), f"{s} {J(r)}")
_, a = report()
check("«Должны временем» +60 минут и +20,00 zł", a["должны мин"] - b["должны мин"] == 60 and a["должны zł"] is not None
      and b["должны zł"] is not None and near(a["должны zł"] - b["должны zł"], 20), J({k: (b[k], a[k]) for k in ("должны мин", "должны zł")}))
check("«Должны временем» в злотых = база (гранты по тарифу, пул по клубному)", a["должны zł"] is not None
      and near(a["должны zł"], owed_pln_db() or -1), f"{a['должны zł']} / {owed_pln_db()}")

section("игра 30 минут на выданном и отмена выдачи")
s, d = api("POST", f"/admin/computers/{pcs()['PC-06']['id']}/session", {"guest": "Guest37"}, "admin1")
sid = d.get("session_id")
check("посадка Guest37 за PC-06 → 201 (зонные минуты — запас для старта)", s == 201 and sid, f"{s} {d.get('code')}")
at_end(lambda: sql(f"DELETE FROM sessions WHERE id='{sid}'"))
sql(f"UPDATE sessions SET started_at = now() - interval '29 minutes 10 seconds', ready_at = now() - interval '29 minutes 10 seconds' WHERE id='{sid}'")
s, _ = end(sid)
billed = sql(f"SELECT billed_minutes || '|' || coin_minutes_used FROM sessions WHERE id='{sid}'")
check("сессия 30 минут целиком из выданного", s == 200 and billed == "30|30", billed)
s, v = api("POST", f"/admin/coin-redemptions/{r.get('id')}/void", {}, "admin1")
check("отмена → 200: забрано 30 минут, вернулось 200 монет", s == 200 and v.get("minutes_taken") == 30 and v.get("coins_returned") == 200, f"{s} {J(v)}")
rep, c = report()
dd = {k: round(c[k] - b[k], 2) for k in c if c[k] is not None and b[k] is not None}
check("«Погашено» +200 — выдача минус вернувшееся, а не 0", dd.get("погашено") == 200, J(dd))
check("«Отдано временем» +30 минут и +10,00 zł — отыгранная часть", dd.get("отдано мин") == 30 and near(dd.get("отдано zł", 0), 10), J(dd))
check("на руках = выпущено − погашено − сгорело − потрачено (приращения)",
      dd.get("на руках") == dd.get("выпущено", 0) - dd.get("погашено", 0) - dd.get("сгорело", 0) - dd.get("потрачено", 0), J(dd))
check("«Должны временем» вернулось к исходному: 0 минут и 0,00 zł разницы", dd.get("должны мин") == 0 and "должны zł" in dd
      and near(dd["должны zł"], 0), J(dd))
row = next((x for x in rep.get("redemptions", []) if x.get("id") == r.get("id")), {})
check("в списке погашений: отменена, вернулось 200", row.get("voided") is True and row.get("coins_returned") == 200, J(row))
check("в базе у выдачи coins_returned = 200", has_ret and sql(f"SELECT coins_returned FROM coin_redemptions WHERE id='{r.get('id')}'") == "200", has_ret)
done()
