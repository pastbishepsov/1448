#!/usr/bin/env python3
"""З19-и8 «До того же часа — для всех периодов» (решение основателя 10.11). Период, который кончается сегодняшними
(неполными) сутками, сравнивается с прошлым до того же момента: у «7 дней», «30 дней», «Этот месяц», года и своих дат по
сегодня prev_period.until — «ЧЧ:ММ», в прошлом периоде есть пополнение за полчаса до соответствующего момента, а через
две минуты после — нет; «Сегодня» — как было (З19-и1); «Вчера», «Прошлый месяц» и свои даты по вчера — целиком, until
нет. Гость z19ca заводится один раз (удалить аккаунт нельзя — FK); его пополнения — SQL-ом, удаляются в начале, после
каждого периода и в конце."""
import time
from datetime import date

from _z import *

G = "z19ca"
if not uid(G):
    s, d = api("POST", "/auth/register", {"nickname": G, "password": GUEST_PASS})
    if s not in (200, 201):
        raise SystemExit(f"регистрация {G}: {s} {J(d)}")
U, ADM1 = uid(G), uid("admin1")


def wipe():
    sql(f"DELETE FROM deposits WHERE user_id = '{U}'")


wipe()
at_end(wipe)
money = lambda q: api("GET", "/admin/reports/money?" + q, None, "boss")[1]
gr = lambda x: int(round((x or 0) * 100))

# у границы суток — подождать, пока «полчаса до этого момента» и «через 2 минуты» влезут в сутки (как в e2e_z19_reports)
RH = int(settings().get("report_hour", 0))
el, left = (int(x) for x in sql(
    f"SELECT (extract(epoch FROM now() - (date_trunc('day', now() AT TIME ZONE '{TZ}' - interval '{RH} hours') + interval '{RH} hours') "
    f"AT TIME ZONE '{TZ}'))::int || ' ' || (extract(epoch FROM (date_trunc('day', now() AT TIME ZONE '{TZ}' - interval '{RH} hours') "
    f"+ interval '1 day {RH} hours') AT TIME ZONE '{TZ}' - now()))::int").split())
if el < 240 or left < 240:
    time.sleep((240 - el if el < 240 else left + 5) + 2)
    el = 240
half = min(30, el // 120)  # минут до «того же момента», но не раньше начала его суток


def at(days, minutes):
    """SQL: тот же момент по стенным часам клуба days дней назад, плюс minutes минут."""
    return f"(((now() AT TIME ZONE '{TZ}') - interval '{days} days' + interval '{minutes} minutes') AT TIME ZONE '{TZ}')"


section("по сегодня — с прошлым до того же момента")
for q, name in (("preset=d7", "«7 дней»"), ("preset=d30", "«30 дней»"), ("preset=month", "«Этот месяц»"), ("preset=year", "год"),
                (f"from={club_day(-2)}&to={club_day(0)}", "свои даты по сегодня")):
    p0 = money(q)
    per, pv = p0.get("period") or {}, p0.get("prev_period") or {}
    if not per.get("to") or not pv.get("to"):
        check(f"{name}: отчёт с периодом и прошлым", False, J(p0, 300))
        continue
    n = (date.fromisoformat(per["to"]) - date.fromisoformat(pv["to"])).days  # сегодня ↔ соответствующий день прошлого
    sql(f"INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at) VALUES "
        f"('{U}', 7.01, 0, 'card', '{ADM1}', {at(n, -half)}), ('{U}', 3.02, 0, 'card', '{ADM1}', {at(n, 2)})")
    hm = sql(f"SELECT to_char(now() AT TIME ZONE '{TZ}', 'HH24:MI') || ' ' || "
             f"to_char(now() AT TIME ZONE '{TZ}' - interval '1 minute', 'HH24:MI')").split()
    p1 = money(q)
    u = (p1.get("prev_period") or {}).get("until")
    check(f"{name}: сравнение с {pv['from']} — {pv['to']} до {u}: пополнение за {half} мин до того же момента (7,01) — в "
          "прошлом периоде, через 2 минуты после (3,02) — нет; until — «ЧЧ:ММ» сейчас",
          gr((p1.get("prev") or {}).get("deposits_pln")) - gr((p0.get("prev") or {}).get("deposits_pln")) == 701 and u in hm,
          J([n, (p0.get("prev") or {}).get("deposits_pln"), (p1.get("prev") or {}).get("deposits_pln"), p1.get("prev_period"), hm]))
    wipe()

section("целые периоды и «сегодня» — как были")
t = money("preset=today").get("prev_period") or {}
check("«Сегодня» — со «вчера до того же часа», как в З19-и1", bool(t.get("until")) and t.get("from") == t.get("to") == club_day(-1),
      J(t))
for q, name in (("preset=yesterday", "«Вчера»"), ("preset=prev_month", "«Прошлый месяц»"),
                (f"from={club_day(-3)}&to={club_day(-1)}", "свои даты по вчера")):
    pv = money(q).get("prev_period") or {}
    check(f"{name} — с прошлым целиком, until нет", bool(pv.get("to")) and "until" not in pv, J(pv))
done()
