#!/usr/bin/env python3
"""З21-и1а «Пульс дня и прогноз месяца — сервер» (№46, З-Р28). GET /admin/reports/pulse (владелец; админу — 403):
сегодняшние клубные сутки до этого момента против нормы — медианы того же дня недели 1…8 недель назад до того же
момента по стенным часам: выручка (пополнения + товары + пакеты), засчитанные сессии, новые гости; отклонение в
процентах; прогноз месяца — уже есть + обычный остаток дня + оставшиеся дни по своему дню недели (медиана и четверти
полных суток за 8 недель). Всё пересчитывается прямым SQL от as_of ответа. Фикстура — гость z21pa (заводится один раз,
удалить аккаунт нельзя — FK): пополнения на 101…108 zł в тот же день недели 1…8 недель назад незадолго до этого
момента, 50 zł сегодня, сессии 1…3 недели назад — SQL-ом, удаляются в начале и в конце."""
import math
import time
from datetime import date, timedelta

from _z import *

G = "z21pa"
if not uid(G):
    s, d = api("POST", "/auth/register", {"nickname": G, "password": GUEST_PASS})
    if s not in (200, 201):
        raise SystemExit(f"регистрация {G}: {s} {J(d)}")
U, ADM1 = uid(G), uid("admin1")
PC = sql("SELECT id FROM computers WHERE name = 'PC-01'")
CLUB = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")


def wipe():
    for t in ("deposits", "sessions"):
        sql(f"DELETE FROM {t} WHERE user_id = '{U}'")


wipe()
at_end(wipe)
pulse = lambda who="boss": api("GET", "/admin/reports/pulse", None, who)
RH = int(settings().get("report_hour", 0))
ND = int(settings().get("seg_new_days", 14))


def day0(base):
    """SQL: начало клубных суток момента base (timestamptz)."""
    return (f"((date_trunc('day', {base} AT TIME ZONE '{TZ}' - interval '{RH} hours') + interval '{RH} hours') "
            f"AT TIME ZONE '{TZ}')")


# у границы суток — подождать: фикстура «сегодня» и окна «до этого момента» должны лежать внутри одних суток
el, left = (int(x) for x in sql(f"SELECT extract(epoch FROM now() - {day0('now()')})::int || ' ' || "
                                f"extract(epoch FROM ({day0('now()')} AT TIME ZONE '{TZ}' + interval '1 day') "
                                f"AT TIME ZONE '{TZ}' - now())::int").split())
if el < 600 or left < 180:
    time.sleep((600 - el if el < 600 else left + 600) + 2)
    el = 600
OFF = min(60, el // 120)  # минут до «этого момента» у фикстуры — внутри сегодняшних суток


def ago(weeks, minutes, base="now()"):
    """SQL: тот же момент по стенным часам weeks недель назад минус minutes минут."""
    return f"((({base} AT TIME ZONE '{TZ}') - interval '{7 * weeks} days' - interval '{minutes} minutes') AT TIME ZONE '{TZ}')"


section("права и ответ")
s_adm = pulse("admin1")[0]
s0, P0 = pulse()
check("admin1 — 403 (пульс — владельцу), владелец — 200: сутки, момент, недели, выручка, сессии, новые, прогноз",
      s_adm == 403 and s0 == 200 and all(k in P0 for k in ("date", "as_of", "weeks", "week_dates", "revenue", "sessions", "new",
                                                           "forecast")), J([s_adm, s0, list(P0) if isinstance(P0, dict) else P0]))

section("фикстура: 101…108 zł в тот же день недели 1…8 недель назад, 50 zł сегодня, сессии 1…3 недели назад")
dep = ", ".join(f"('{U}', {100 + k}, 0, 'card', '{ADM1}', {ago(k, OFF)})" for k in range(1, 9))
sql(f"INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at) VALUES {dep}, "
    f"('{U}', 50, 0, 'card', '{ADM1}', {ago(0, OFF)})")
ses = ", ".join(f"('{U}', '{PC}', '{CLUB}', 'completed', {ago(k, OFF)}, {ago(k, OFF // 2)}, 30, 30, 20, 20, {ago(k, OFF)}, "
                "'Standard')" for k in (1, 2, 3))
sql("INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total, minutes_used, base_rate_pln, "
    f"effective_rate_pln, created_at, zone_name) VALUES {ses}")
s1, P = pulse()
if s1 != 200 or not isinstance(P, dict) or not P.get("as_of"):  # ручки нет (сервер до З21-и1а) — FAIL, а не обрыв
    for name in ("выручка сегодня", "недели нормы", "выручка по неделям", "норма и отклонение", "сессии", "новые", "прогноз"):
        check(f"{name}: пульса нет", False, J([s1, P]))
    done()
NOW = f"'{P['as_of']}'::timestamptz"
D0 = day0(NOW)


def start(weeks, plus=0):
    """SQL: начало клубных суток того же дня недели weeks недель назад (plus — дней вперёд) от as_of."""
    return f"(({D0} AT TIME ZONE '{TZ}' - interval '{7 * weeks} days' + interval '{plus} days') AT TIME ZONE '{TZ}')"


def revenue(frm, to, guest=None):
    q = " + ".join(f"(SELECT COALESCE(SUM({pln}), 0) FROM {t} WHERE {at} >= {frm} AND {at} < {to} AND {flt}"
                   + (f" AND user_id {guest}" if guest else "") + ")" for t, at, pln, flt in (
                       ("deposits", "created_at", "amount_pln", "voided_at IS NULL"),
                       ("sales", "paid_at", "total_pln", "voided_at IS NULL AND method NOT IN ('wallet', 'comp')"),
                       ("user_packages", "created_at", "price_pln", "voided_at IS NULL AND method NOT IN ('wallet', 'comp')")))
    return round(float(sql(f"SELECT {q}")), 2)


def sessions(frm, to, guest=None):
    return int(sql(f"SELECT COUNT(*) FROM sessions WHERE status <> 'cancelled' AND started_at >= {frm} AND started_at < {to}"
                   + (f" AND user_id {guest}" if guest else "")))


def new(frm, to, guest=None):
    since = f"(({to} AT TIME ZONE '{TZ}') - interval '{ND} days') AT TIME ZONE '{TZ}'"
    return int(sql(f"SELECT COUNT(*) FROM (SELECT s.user_id FROM sessions s JOIN users u ON u.id = s.user_id "
                   f"WHERE s.status <> 'cancelled' AND u.role = 'player'" + (f" AND s.user_id {guest}" if guest else "") +
                   f" GROUP BY s.user_id HAVING MIN(s.started_at) >= {since} AND bool_or(s.started_at >= {frm} AND "
                   f"s.started_at < {to})) x"))


SINCE = ("(SELECT MIN(t) FROM (SELECT MIN(created_at) AS t FROM deposits UNION ALL SELECT MIN(created_at) FROM sales "
         "UNION ALL SELECT MIN(started_at) FROM sessions) x)")
W = [k for k in range(1, 9) if sql(f"SELECT {start(k, 1)} > {SINCE}") == "t"]  # недели, к концу которых клуб уже работал
win = lambda k: (start(k), ago(k, 0, NOW))
OTHER = f"IS DISTINCT FROM '{U}'"  # у продажи гостя может не быть — NULL не «другой» для <>


def q(xs, p):
    """квантиль с линейной интерполяцией — как quantile сервера"""
    if not xs:
        return 0.0
    s = sorted(xs)
    pos = p * (len(s) - 1)
    lo = math.floor(pos)
    return s[-1] if lo >= len(s) - 1 else s[lo] + (s[lo + 1] - s[lo]) * (pos - lo)


def go_pct(now, prev):
    if prev == 0:
        return 0.0 if now == 0 else None
    x = (now - prev) / prev * 1000
    return (math.floor(x + 0.5) if x >= 0 else -math.floor(-x + 0.5)) / 10


R, S, N = P.get("revenue") or {}, P.get("sessions") or {}, P.get("new") or {}
cur = date.fromisoformat(P["date"])
section("выручка: сегодня и 8 недель — как прямой SQL от as_of, фикстура — в своих окнах")
want_today, want_w = revenue(D0, NOW), [revenue(*win(k)) for k in W]
check(f"сегодня {R.get('today')} zł до {P.get('at')} — как SQL, из них ровно 50 zł — z21pa",
      s1 == 200 and near(R.get("today", -1), want_today) and near(revenue(D0, NOW, OTHER) + 50, R.get("today", -1)),
      J({"server": R.get("today"), "sql": want_today}))
check(f"недель в норме — {len(W)} (все 8: фикстура сама даёт историю 8 недель), свежая первой, даты — тот же день недели",
      P.get("weeks") == len(W) == 8 and P.get("week_dates") == [str(cur - timedelta(days=7 * k)) for k in W],
      J({"weeks": P.get("weeks"), "dates": P.get("week_dates"), "W": W}))
check("каждая неделя до того же момента — как SQL; z21pa даёт в неделе k ровно 100 + k zł",
      len(R.get("weeks") or []) == len(W) and all(near(a, b) for a, b in zip(R["weeks"], want_w))
      and all(near(revenue(*win(k), OTHER) + 100 + k, R["weeks"][i]) for i, k in enumerate(W)),
      J({"server": R.get("weeks"), "sql": want_w}))
check(f"норма — медиана недель ({R.get('norm')}), отклонение {R.get('pct')}% — от неё",
      near(R.get("norm", -1), q(want_w, 0.5)) and R.get("pct") == go_pct(R.get("today"), R.get("norm")), J(R))

section("сессии и новые гости — как SQL")
want_s, want_n = [sessions(*win(k)) for k in W], [new(*win(k)) for k in W]
check("сессии: сегодня и недели — как SQL (засчитанные, по началу), сессии z21pa — +1 в неделях 1…3; норма — медиана",
      S.get("today") == sessions(D0, NOW) and S.get("weeks") == want_s and near(S.get("norm", -1), q(want_s, 0.5))
      and all(sessions(*win(k), OTHER) + (k <= 3) == S["weeks"][i] for i, k in enumerate(W)), J({"server": S, "sql": want_s}))
new_k = [k for k in (1, 2, 3) if 21 < 7 * k + ND]  # первая сессия z21pa — 3 недели назад: новый, если она не раньше ND дн до конца окна
check(f"новые: как в «Гостях» — первая сессия не раньше {ND} дн до конца окна; z21pa — новый в неделях {new_k}",
      N.get("today") == new(D0, NOW) and N.get("weeks") == want_n and near(N.get("norm", -1), q(want_n, 0.5))
      and all(new(*win(k), OTHER) + (k in new_k) == N["weeks"][i] for i, k in enumerate(W)), J({"server": N, "sql": want_n}))

section("прогноз месяца")
F = P.get("forecast") or {}
nxt = (cur.replace(day=1) + timedelta(days=32)).replace(day=1)
days_left = [cur + timedelta(days=i) for i in range(1, (nxt - cur).days)]
at_rh = lambda d: f"(('{d}'::date + time '{RH:02d}:00') AT TIME ZONE '{TZ}')"
so_far = revenue(at_rh(cur.replace(day=1)), NOW)
full = {}
for i in range(1, 7 * 8 + 1):
    d = cur - timedelta(days=i)
    if sql(f"SELECT {at_rh(d + timedelta(days=1))} > {SINCE}") == "t":
        full.setdefault(d.weekday(), []).append(revenue(at_rh(d), at_rh(d + timedelta(days=1))))
rest = [revenue(ago(k, 0, NOW), start(k, 1)) for k in W]
want = {key: round(so_far + q(rest, p) + sum(q(full.get(d.weekday(), []), p) for d in days_left), 2)
        for p, key in ((0.5, "pln"), (0.25, "low_pln"), (0.75, "high_pln"))}
check(f"«ожидаем к концу месяца»: уже есть {so_far} zł + обычный остаток дня + {len(days_left)} оставшихся дней по своему "
      "дню недели (медиана, разброс — четверти) — как SQL; нижняя ≤ середина ≤ верхняя",
      bool(F) and near(F.get("so_far_pln", -1), so_far) and F.get("days_left") == len(days_left) and F.get("month") == P["date"][:7]
      and all(abs(F.get(k, -1) - v) < 0.02 for k, v in want.items()) and F["low_pln"] <= F["pln"] <= F["high_pln"],
      J({"forecast": F, "ждали": want}))
done()
