#!/usr/bin/env python3
"""З22-и3 «Периодические алерты» (№59, З-Р28). POST /admin/alerts/check (владелец; админу — 403) — тот же прогон, что
фоновый раз в 15 минут: «долг больше N zł дольше суток» (гостю — раз за сутки), «доля отмен у админа за 7 дней выше N
%» (не меньше трёх отмен), «выручка сегодня ниже нормы на N %» (пульс: норма из четырёх недель и больше и не меньше
100 zł). Все — владельцу, раз за клубные сутки на предмет; порог 0 — вид выключен; «Молчать» — ничего. Фикстуры —
SQL-ом: гости z22d (долг 45 zł 30 часов), z22e (10 zł, давно), z22f (60 zł, 2 часа), z22r (пополнения по 20 000 zł в
тот же день недели 1…8 недель назад до этого часа — норма выше сегодняшней выручки) и временный админ z22v (10
пополнений за неделю, 4 отменены); гости заводятся один раз (удалить аккаунт нельзя — FK). В начале и в конце
удаляются строки фикстур, журнал алертов их предметов и их сообщения канала; в конце — и все алерты, ушедшие в канал
за время проверки (по стенду тоже: журнал о них остаётся, чтобы фон не прислал их снова); пороги — как были."""
import time

from _z import *

GUESTS = ["z22d", "z22e", "z22f", "z22r"]
for g in GUESTS:
    if not uid(g):
        s, d = api("POST", "/auth/register", {"nickname": g, "password": GUEST_PASS})
        if s not in (200, 201):
            raise SystemExit(f"регистрация {g}: {s} {J(d)}")
U = {g: uid(g) for g in GUESTS}
CLUB = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")
V = hire_temp("z22v")
U["z22v"] = V
IDS = ", ".join(f"'{u}'" for u in U.values())


def wipe():
    sql(f"DELETE FROM sales WHERE user_id IN ({IDS}) OR created_by = '{V}'")
    sql(f"DELETE FROM deposits WHERE user_id IN ({IDS}) OR created_by = '{V}'")
    keys = " OR ".join(f"key LIKE '%/{k}/{u}'" for k in ("debt", "void_share") for u in U.values())
    sql(f"DELETE FROM alerts_sent WHERE (kind IN ('debt', 'void_share') AND ({keys})) OR kind = 'revenue_low'")
    sql("DELETE FROM staff_messages WHERE kind = 'system' AND (text LIKE '💸 z22%' OR text LIKE '⚠ z22v:%' OR text LIKE '📉 Выручка%')")


T0 = sql("SELECT now()")  # с этого момента алерты в канале — от проверки (и от фона во время неё)


def wipe_all():
    wipe()
    sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND created_at >= '{T0}' AND (text LIKE '💸 %' OR text LIKE '⚠ %: за 7 дней "
        "отменено %' OR text LIKE '📉 Выручка%')")


wipe()
at_end(wipe_all)
check_now = lambda who="boss": api("POST", "/admin/alerts/check", {}, who)
msg = lambda like: rows(f"SELECT visible_to || '|' || text FROM staff_messages WHERE kind = 'system' AND text LIKE '{like}'")
set_settings(alert_debt_pln=30, alert_void_share_pct=30, alert_revenue_low_pct=50, alerts_on=1)

section("права и пороги")
s_adm = check_now("admin1")[0]
st = settings()
check("admin1 — 403 (алерты — владельцу); в «Экономике» пороги alert_debt_pln, alert_void_share_pct, alert_revenue_low_pct",
      s_adm == 403 and [st.get(k) for k in ("alert_debt_pln", "alert_void_share_pct", "alert_revenue_low_pct")] == [30, 30, 50],
      J([s_adm, {k: st.get(k) for k in st if k.startswith("alert")}]))
bad = [api("PUT", "/admin/settings", {k: v}, "boss")[0] for k, v in (("alert_debt_pln", -1), ("alert_void_share_pct", 101),
                                                                    ("alert_revenue_low_pct", 101))]
check("пороги вне границ — 400", bad == [400, 400, 400], J(bad))

section("фикстуры")
ADM1 = uid("admin1")
later = lambda u, pln, ago: (f"('{CLUB}', 'z22 в долг', 1, {pln}, {pln}, 'later', '{u}', '{ADM1}', now() - interval '{ago}', NULL, "
                             "true)")
sql("INSERT INTO sales (club_id, name, qty, price_pln, total_pln, method, user_id, created_by, created_at, paid_at, postpaid) VALUES "
    + ", ".join([later(U["z22d"], 45, "30 hours"), later(U["z22e"], 10, "3 days"), later(U["z22f"], 60, "2 hours")]))
# z22v: 10 своих пополнений за неделю, 4 отменены — 40 % при пороге 30
vdep = lambda i: (f"('{U['z22e']}', 5, 0, 'card', '{V}', now() - interval '{i} hours', "
                   + (f"now(), '{V}')" if i < 4 else "NULL, NULL)"))
sql("INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at, voided_at, voided_by) VALUES "
    + ", ".join(vdep(i) for i in range(10)))
RH = int(st.get("report_hour", 0))
D0 = (f"((date_trunc('day', now() AT TIME ZONE '{TZ}' - interval '{RH} hours') + interval '{RH} hours') AT TIME ZONE '{TZ}')")
el = int(sql(f"SELECT extract(epoch FROM now() - {D0})::int"))
if el < 600:  # окно «до этого часа» у самой границы суток пустое — подождать
    time.sleep(600 - el + 2)
    el = 600
OFF = min(60, el // 120)
ago = lambda k: f"(((now() AT TIME ZONE '{TZ}') - interval '{7 * k} days' - interval '{OFF} minutes') AT TIME ZONE '{TZ}')"
sql("INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at) VALUES "
    + ", ".join(f"('{U['z22r']}', 20000, 0, 'card', '{ADM1}', {ago(k)})" for k in range(1, 9)))

section("прогон")
s1, r1 = check_now()
sent = (r1 or {}).get("sent") or {}
d = msg("💸 z22%")
check("долг: z22d — 45 zł 30 часов, выше 30 — алерт владельцу; z22e (10 zł) и z22f (2 часа) — нет",
      s1 == 200 and len(d) == 1 and d[0].startswith("owner|💸 z22d должен клубу 45,00 zł дольше суток"), J([r1, d]))
v = msg("⚠ z22v:%")
check("доля отмен: z22v — 4 из 10 (40 %) при пороге 30 — алерт владельцу",
      v == ["owner|⚠ z22v: за 7 дней отменено 4 из 10 его пополнений и продаж (40%) — выше порога 30%"], J(v))
p = api("GET", "/admin/reports/pulse", None, "boss")[1]
rv = msg("📉 Выручка%")
pr = p.get("revenue") or {}
low = pr.get("norm", 0) >= 100 and p.get("weeks", 0) >= 4 and pr.get("today", 0) * 100 <= pr.get("norm", 0) * 50
check(f"выручка: сегодня {pr.get('today')} zł при норме {pr.get('norm')} (пополнения z22r по 20 000 zł в прошлые недели) — ниже на 50 % "
      "и больше — алерт владельцу, текст — из пульса", low and len(rv) == 1 and rv[0].startswith("owner|📉 Выручка сегодня до "),
      J([pr, rv]))

section("раз за сутки")
s2, r2 = check_now()
check("второй прогон за те же сутки — ничего нового: по одному сообщению на предмет",
      len(msg("💸 z22%")) == 1 and len(msg("⚠ z22v:%")) == 1 and len(msg("📉 Выручка%")) == 1 and ((r2 or {}).get("sent") or {}).get("revenue_low") == 0,
      J(r2))

section("порог 0 и «Молчать»")
# фон (раз в 15 минут) шлёт то же: сначала выключаем, потом стираем ключ — иначе он может успеть между ними


def wipe_keys():
    sql(f"DELETE FROM alerts_sent WHERE key LIKE '%/debt/{U['z22d']}'")
    sql("DELETE FROM staff_messages WHERE kind = 'system' AND text LIKE '💸 z22%'")


set_settings(alert_debt_pln=0)
wipe_keys()
r0 = check_now()[1]
check("alert_debt_pln 0 — долговой алерт не уходит", msg("💸 z22%") == [] and ((r0 or {}).get("sent") or {}).get("debt") == 0, J([r0, msg("💸 z22%")]))
set_settings(alert_debt_pln=30, alerts_on=0)
s3, r3 = check_now()
check("«Молчать» — прогон ничего не шлёт (reason off)", (r3 or {}).get("reason") == "off" and msg("💸 z22%") == [], J(r3))
set_settings(alerts_on=1)
check_now()
check("снова «Присылать» — долг z22d снова уходит, один раз", len(msg("💸 z22%")) == 1, J(msg("💸 z22%")))
done()
