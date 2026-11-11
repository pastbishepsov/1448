#!/usr/bin/env python3
"""З19-и1 «Отчёты» (№40, решение основателя 23.09; ответ «Пропали — за период» — 09.11, в отчёте итерации). «Сегодня»
сравнивается со «вчера до того же часа»: пополнение вчера за полчаса до этого часа в прошлом периоде есть, через две
минуты после — нет, у прошлого периода — until «ЧЧ:ММ»; «Вчера» целиком — оба. Периоды разной длины (месяц к прошлому
месяцу) — рост сумм и счётов на среднедневных, средний чек — как есть; неделя к неделе — без поправки. «Списано с
кошельков» — время и кухня, без отмены депозита и покупки пакета с кошелька; «операций» — сессий и заказов (три
поминутных списания одной сессии — одна). «Платили гостей» — и тот, кто только купил товар картой, и тот, кто только
купил пакет. «Пропали» — на конец периода и за этот период: последний визит до конца окна — lost_days и больше назад,
а за lost_days до его начала ещё ходил; визит после конца периода не «возвращает» гостя задним числом; ушедший раньше
нижней границы — не в списке. Рассчитанная постоплата: вчерашний долг, рассчитанный сегодня admin1, admin1 отменяет
(раньше — «слишком старая»), admin2 — нет («не его»). Гости z19ra…z19rd заводятся один раз (удалить аккаунт нельзя —
FK); их пополнения, продажи, пакеты, сессии и строки кошелька — SQL-ом, удаляются в начале и в конце; позиция «z19r Сок» —
drop_good."""
import math
import time

from _z import *

G = ["z19ra", "z19rb", "z19rc", "z19rd"]
GOOD = "z19r Сок"
for n in G:
    if not uid(n):
        s, d = api("POST", "/auth/register", {"nickname": n, "password": GUEST_PASS})
        if s not in (200, 201):
            raise SystemExit(f"регистрация {n}: {s} {J(d)}")
U = {n: uid(n) for n in G}
IDS = ",".join(f"'{u}'" for u in U.values())


def wipe():
    for t in ("wallet_transactions", "deposits", "user_packages", "sessions"):
        sql(f"DELETE FROM {t} WHERE user_id IN ({IDS})")
    sql(f"DELETE FROM stock_moves WHERE sale_id IN (SELECT id FROM sales WHERE user_id IN ({IDS}) OR name = '{GOOD}')")
    sql(f"DELETE FROM sales WHERE user_id IN ({IDS}) OR name = '{GOOD}'")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{GOOD}'"):
        drop_good(gid)


wipe()  # хвост прошлого прогона
at_end(wipe)
CLUB = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")
PC = sql("SELECT id FROM computers WHERE name = 'PC-01'")
ADM1, ADM2 = uid("admin1"), uid("admin2")
money = lambda q: api("GET", "/admin/reports/money?" + q, None, "boss")[1]
gr = lambda x: int(round((x or 0) * 100))


def go_pct(now, prev):
    """pctDelta сервера: рост в процентах, одна цифра после запятой, половина — от нуля; от нуля — None (0 — 0)."""
    if prev == 0:
        return 0.0 if now == 0 else None
    x = (now - prev) / prev * 1000
    return (math.floor(x + 0.5) if x >= 0 else -math.floor(-x + 0.5)) / 10


section("«сегодня» — со «вчера до того же часа»")
RH = int(settings().get("report_hour", 0))
el, left = (int(x) for x in sql(
    f"SELECT (extract(epoch FROM now() - (date_trunc('day', now() AT TIME ZONE '{TZ}' - interval '{RH} hours') + interval '{RH} hours') "
    f"AT TIME ZONE '{TZ}'))::int || ' ' || (extract(epoch FROM (date_trunc('day', now() AT TIME ZONE '{TZ}' - interval '{RH} hours') "
    f"+ interval '1 day {RH} hours') AT TIME ZONE '{TZ}' - now()))::int").split())
if el < 240 or left < 240:  # у границы суток: подождать, пока «вчера до этого часа» и «через 2 минуты» влезут в сутки
    time.sleep((240 - el if el < 240 else left + 5) + 2)
    el = 240
half = min(30, el // 120)  # минут до «этого часа» вчера, но не раньше начала вчерашних суток
p0 = money("preset=today")
y0 = money("preset=yesterday")["totals"]["deposits_pln"]
sql(f"INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at) VALUES "
    f"('{U['z19ra']}', 7.01, 0, 'card', '{ADM1}', now() - interval '1 day {half} minutes'), "
    f"('{U['z19ra']}', 3.02, 0, 'card', '{ADM1}', now() - interval '1 day' + interval '2 minutes')")
now_hm = sql(f"SELECT to_char(now() AT TIME ZONE '{TZ}', 'HH24:MI') || ' ' || to_char(now() AT TIME ZONE '{TZ}' - interval '1 minute', 'HH24:MI')").split()
p1 = money("preset=today")
y1 = money("preset=yesterday")["totals"]["deposits_pln"]
pv = p1.get("prev_period") or {}
check(f"«сегодня»: в прошлом периоде — пополнение за {half} мин до этого часа вчера (7,01), через 2 минуты после (3,02) — нет; "
      f"prev_period.until — «{now_hm[0]}»", gr(p1["prev"]["deposits_pln"]) - gr(p0["prev"]["deposits_pln"]) == 701
      and pv.get("until") in now_hm, J([p0["prev"]["deposits_pln"], p1["prev"]["deposits_pln"], pv]))
check("«вчера» целиком — оба пополнения (10,03), until нет", gr(y1) - gr(y0) == 1003 and "until" not in (money("preset=yesterday").get("prev_period") or {}),
      J([y0, y1]))

section("разной длины — на среднедневных")
# ближайший прошедший месяц, у которого длина не как у предыдущего; в предыдущий — пополнение z19ra 11,11 (15-го в
# 12:00), чтобы прошлому периоду было с чем сравнивать и на стенде без истории
m, pm = sql(f"SELECT to_char(m, 'YYYY-MM') || ' ' || to_char(m - interval '1 month', 'YYYY-MM') FROM generate_series(1, 13) k, "
            f"LATERAL (SELECT date_trunc('month', now() AT TIME ZONE '{TZ}') - k * interval '1 month' AS m) x "
            "WHERE extract(day FROM m + interval '1 month' - interval '1 day') <> extract(day FROM m - interval '1 day') ORDER BY k LIMIT 1").split()
pm15 = f"('{pm}-15 12:00'::timestamp AT TIME ZONE '{TZ}')"
sql(f"INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at) VALUES "
    f"('{U['z19ra']}', 11.11, 0, 'card', '{ADM1}', {pm15})")
M = money("month=" + m)
if M.get("prev", {}).get("revenue_pln", 0) > 0 and M["period"]["days"] != M["prev_period"]["days"]:
    t, w, dl = M["totals"], M["prev"], M["delta"]
    sc = M["period"]["days"] / M["prev_period"]["days"]
    want = {"revenue_pln": go_pct(t["revenue_pln"], w["revenue_pln"] * sc), "deposits": go_pct(t["deposits"], w["deposits"] * sc),
            "guests": go_pct(t["guests"], w["guests"] * sc), "avg_check_pln": go_pct(t["avg_check_pln"], w["avg_check_pln"])}
    check(f"месяц {M['period']['from'][:7]} ({M['period']['days']} дн) к {M['prev_period']['from'][:7]} ({M['prev_period']['days']} дн): "
          "выручка, пополнения, гости — на среднедневных, средний чек — как есть",
          all(dl.get(k) == v for k, v in want.items()), J({"delta": {k: dl.get(k) for k in want}, "ждали": want}))
else:
    check(f"месяц {m} к {pm}: прошлый пуст или той же длины", False, J([M.get("period"), M.get("prev_period"), M.get("prev")]))
W = money("preset=d7")
check("неделя к неделе — без поправки: рост выручки — как есть", W["delta"]["revenue_pln"] == go_pct(W["totals"]["revenue_pln"], W["prev"]["revenue_pln"]),
      J([W["delta"]["revenue_pln"], W["totals"]["revenue_pln"], W["prev"]["revenue_pln"]]))

section("«Списано с кошельков» и «операций»")
w0 = money("preset=today")["wallet"]
sid, oid, did, kid = (sql("SELECT uuid_generate_v4()") for _ in range(4))
sql("INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, ref_id, created_at) VALUES "
    + ", ".join(f"('{U['z19rb']}', '{k}', {a}, 0, '{r}', '{i}', now() - interval '1 minute')" for k, a, r, i in [
        ("session_spend", -100, "session", sid), ("session_spend", -100, "session", sid), ("session_spend", -100, "session", sid),
        ("kitchen_spend", -250, "kitchen", oid), ("deposit_void", -400, "deposit", did), ("package_spend", -500, "package", kid)]))
w1 = money("preset=today")["wallet"]
dd = {k: gr(w1[k]) - gr(w0[k]) for k in ("period_spent_pln", "spent_session_pln", "spent_kitchen_pln")}
check("три поминутных списания сессии (3,00) и заказ кухни (2,50) — «списано» +5,50 (время +3,00, кухня +2,50), «операций» +2; "
      "отмена депозита (4,00) и пакет с кошелька (5,00) — не трата",
      dd == {"period_spent_pln": 550, "spent_session_pln": 300, "spent_kitchen_pln": 250} and w1["period_spends"] - w0["period_spends"] == 2,
      J([dd, w1["period_spends"] - w0["period_spends"]]))

section("«Платили гостей»")
g0 = money("preset=today")["totals"]["guests"]
zone = sql("SELECT id || '|' || name FROM zones ORDER BY sort, name LIMIT 1").split("|")
sql(f"INSERT INTO sales (club_id, name, qty, price_pln, total_pln, method, user_id, created_by, paid_at, created_at) VALUES "
    f"('{CLUB}', '{GOOD}', 1, 5, 5, 'card', '{U['z19rc']}', '{ADM1}', now() - interval '1 minute', now() - interval '1 minute')")
sql(f"INSERT INTO user_packages (club_id, user_id, zone_id, name, zone_name, minutes_total, minutes_left, price_pln, method, created_by, created_at) "
    f"VALUES ('{CLUB}', '{U['z19rd']}', '{zone[0]}', 'z19r час', '{zone[1]}', 60, 60, 9, 'card', '{ADM1}', now() - interval '1 minute')")
g1 = money("preset=today")["totals"]["guests"]
check("z19rc только купил товар картой, z19rd — пакет: «Платили гостей» +2", g1 - g0 == 2, J([g0, g1]))

section("«Пропали» — на конец периода и за период")
L = int(settings().get("seg_lost_days", 21))
d_to, d_from = club_day(-7), club_day(-13)  # прошедшая неделя: конец окна — начало club_day(-6)
at = lambda day, hh: f"('{day} {hh}'::timestamp AT TIME ZONE '{TZ}')"
lost_in = sql(f"SELECT to_char(date '{d_to}' - {L} - 1, 'YYYY-MM-DD')")    # внутри: за L+1 день до конца
lost_old = sql(f"SELECT to_char(date '{d_from}' - {L} - 2, 'YYYY-MM-DD')")  # раньше нижней границы
sess = lambda nick, day: (f"('{U[nick]}', '{PC}', '{CLUB}', 'completed', {at(day, '12:00')}, {at(day, '13:00')}, 60, 60, 20, 20, "
                          f"{at(day, '12:00')}, 'Standard')")
sql("INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total, minutes_used, base_rate_pln, "
    "effective_rate_pln, created_at, zone_name) VALUES " + ", ".join([sess("z19ra", lost_in), sess("z19rb", lost_old), sess("z19rc", lost_in),
                                                                       sess("z19rc", club_day(0))]))
g = api("GET", f"/admin/reports/guests?from={d_from}&to={d_to}", None, "boss")[1]
names = {x.get("nickname") for x in (g.get("lost") or {}).get("items", [])}
check(f"неделя {d_from}…{d_to}, L = {L}: z19ra (последний визит {lost_in}) — пропал; z19rc — тоже, хоть и пришёл сегодня — уже после "
      f"конца периода; z19rb (последний визит {lost_old}, до нижней границы) — нет", {"z19ra", "z19rc"} <= names and "z19rb" not in names,
      J(sorted(n for n in names if n and n.startswith("z19r"))))

section("постоплата — по моменту и автору расчёта")
pin_shift("admin1")
tok("admin2")
gid = api("POST", "/admin/goods", {"name": GOOD, "category": "z19r", "price_pln": 4}, "boss")[1].get("good", {}).get("id")
sale = sql("INSERT INTO sales (club_id, good_id, name, qty, price_pln, total_pln, method, user_id, created_by, postpaid, paid_at, paid_by, created_at) VALUES "
           f"('{CLUB}', '{gid}', '{GOOD}', 1, 4, 4, 'card', '{U['z19rd']}', '{ADM2}', true, now() - interval '1 minute', '{ADM1}', "
           "now() - interval '1 day 2 hours') RETURNING id").split("\n")[0]
s2, d2 = api("POST", f"/admin/sales/{sale}/void", {}, "admin2")
s1, d1 = api("POST", f"/admin/sales/{sale}/void", {}, "admin1")
check("вчерашний долг, рассчитанный сегодня admin1: admin2 — 403 not_yours (расчёт не его), admin1 — 200 (раньше — «слишком старая»)",
      s2 == 403 and d2.get("code") == "not_yours" and s1 == 200, J([[s2, d2.get("code")], [s1, d1.get("code")]]))
done()
