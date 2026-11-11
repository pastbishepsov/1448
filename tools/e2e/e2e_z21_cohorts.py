#!/usr/bin/env python3
"""З21-и2а «Когорты и LTV — сервер» (№47, З-Р28). GET /admin/reports/cohorts (владелец; админу — 403): 12 когорт —
гости по месяцу первого визита (клубных суток первой засчитанной сессии), этот месяц последним: размер, вернулись через
1, 2, 3 месяца (число и доля; месяц идёт — going, не начался — числа нет) и LTV — живые деньги гостя за всё время
(пополнения + товары + пакеты без отменённых и без кошелька) на гостя когорты. Всё пересчитывается прямым SQL от as_of
ответа. Фикстура — гости z21k1…z21k4 (заводятся один раз, удалить аккаунт нельзя — FK): их сессии три, два и один
месяц назад и в этом месяце, пополнения (одно отменённое), продажа картой и кошельком, пакет — SQL-ом, удаляются в
начале и в конце."""
import time
from datetime import date

from _z import *

G = ["z21k1", "z21k2", "z21k3", "z21k4"]
for g in G:
    if not uid(g):
        s, d = api("POST", "/auth/register", {"nickname": g, "password": GUEST_PASS})
        if s not in (200, 201):
            raise SystemExit(f"регистрация {g}: {s} {J(d)}")
U = {g: uid(g) for g in G}
IDS = ", ".join(f"'{u}'" for u in U.values())
ADM1 = uid("admin1")
PC = sql("SELECT id FROM computers WHERE name = 'PC-01'")
CLUB = sql("SELECT id FROM clubs WHERE is_active ORDER BY name LIMIT 1")
ZONE = sql("SELECT id FROM zones ORDER BY name LIMIT 1")


def wipe():
    for t in ("sessions", "deposits", "sales", "user_packages"):
        sql(f"DELETE FROM {t} WHERE user_id IN ({IDS})")


wipe()
at_end(wipe)
cohorts = lambda who="boss": api("GET", "/admin/reports/cohorts", None, who)
RH = int(settings().get("report_hour", 0))

# у границы месяца — подождать: «сейчас минус две минуты» должно лежать в этом же клубном месяце
el = int(sql(f"SELECT extract(epoch FROM now() - ((date_trunc('month', now() AT TIME ZONE '{TZ}' - interval '{RH} hours') "
             f"+ interval '{RH} hours') AT TIME ZONE '{TZ}'))::int"))
if el < 600:
    time.sleep(600 - el + 2)


def month_add(m, k):
    y, mo = divmod(int(m[:4]) * 12 + int(m[5:7]) - 1 + k, 12)
    return f"{y:04d}-{mo + 1:02d}"


section("права и ответ")
s_adm = cohorts("admin1")[0]
s0, P0 = cohorts()
ok0 = s0 == 200 and isinstance(P0, dict) and len(P0.get("cohorts") or []) == 12 and bool(P0.get("as_of"))
check("admin1 — 403 (когорты — владельцу), владелец — 200: 12 когорт, этот месяц последним, момент ответа",
      s_adm == 403 and ok0 and P0["cohorts"][-1]["month"] == P0.get("month")
      and [c["month"] for c in P0["cohorts"]] == [month_add(P0["month"], i - 11) for i in range(12)],
      J([s_adm, s0, P0 if not ok0 else [c["month"] for c in P0["cohorts"]]]))
if not ok0:  # ручки нет (сервер до З21-и2а) — FAIL, а не обрыв
    for name in ("фикстура в своих когортах", "вернулись", "LTV", "всё — как SQL", "будущее без чисел"):
        check(f"{name}: когорт нет", False)
    done()

CUR = P0["month"]
M3, M2, M1 = month_add(CUR, -3), month_add(CUR, -2), month_add(CUR, -1)
at10 = lambda m, d=10: f"(('{m}-{d:02d}'::date + time '12:00') AT TIME ZONE '{TZ}')"
ago = lambda minutes: f"(now() - interval '{minutes} minutes')"
section(f"фикстура: z21k1, z21k2 — впервые в {M3}, z21k3 — в {M1}, z21k4 — в {CUR}")
ses = lambda u, at: f"('{u}', '{PC}', '{CLUB}', 'completed', {at}, {at} + interval '30 minutes', 30, 30, 20, 20, {at}, 'Standard')"
k1, k2, k3, k4 = (U[g] for g in G)
rows_s = [ses(k1, at10(M3)), ses(k1, at10(M2)), ses(k1, ago(40)), ses(k2, at10(M3, 12)), ses(k3, at10(M1)), ses(k3, ago(40)),
          ses(k4, ago(45))]
sql("INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total, minutes_used, base_rate_pln, "
    "effective_rate_pln, created_at, zone_name) VALUES " + ", ".join(rows_s))
dep = lambda u, pln, at, void=False: f"('{u}', {pln}, 0, 'card', '{ADM1}', {at}, {at if void else 'NULL'})"
sql("INSERT INTO deposits (user_id, amount_pln, coins_granted, method, created_by, created_at, voided_at) VALUES "
    + ", ".join([dep(k1, 30, at10(M3)), dep(k1, 20, ago(30)), dep(k2, 10, at10(M3, 12)), dep(k2, 100, at10(M3, 12), True),
                 dep(k4, 5, ago(30))]))
sale = lambda u, pln, method, at: (f"('{CLUB}', 'z21k проверка', 1, {pln}, {pln}, '{method}', '{u}', '{ADM1}', {at}, {at}, "
                                   f"'{ADM1}')")
sql("INSERT INTO sales (club_id, name, qty, price_pln, total_pln, method, user_id, created_by, created_at, paid_at, paid_by) VALUES "
    + ", ".join([sale(k3, 7.5, "card", at10(M1)), sale(k2, 3, "wallet", at10(M3, 12))]))
sql("INSERT INTO user_packages (club_id, user_id, zone_id, name, minutes_total, minutes_left, price_pln, method, created_by, "
    f"created_at) VALUES ('{CLUB}', '{k3}', '{ZONE}', 'z21k пакет', 60, 0, 12, 'card', '{ADM1}', {at10(M1)})")
s1, P1 = cohorts()
C0 = {c["month"]: c for c in P0["cohorts"]}
C1 = {c["month"]: c for c in (P1.get("cohorts") or [])} if s1 == 200 else {}


def delta(m, key):
    a, b = (C0.get(m) or {}).get(key), (C1.get(m) or {}).get(key)
    return None if a is None or b is None else round(b - a, 2)


def back(c, k):
    return ((c or {}).get("back") or [{}] * 3)[k - 1]


check(f"размер: {M3} +2, {M1} +1, {CUR} +1; выручка когорт: +60 zł (отменённое пополнение и продажа кошельком — не в счёт), "
      "+19,5 zł (продажа и пакет картой), +5 zł",
      [delta(M3, "size"), delta(M1, "size"), delta(CUR, "size"), delta(M2, "size")] == [2, 1, 1, 0]
      and [delta(M3, "revenue_pln"), delta(M1, "revenue_pln"), delta(CUR, "revenue_pln")] == [60, 19.5, 5],
      J({m: [(C0.get(m) or {}).get("size"), (C1.get(m) or {}).get("size"), (C0.get(m) or {}).get("revenue_pln"),
             (C1.get(m) or {}).get("revenue_pln")] for m in (M3, M2, M1, CUR)}))
dn = lambda m, k: None if back(C1.get(m), k).get("n") is None else back(C1.get(m), k)["n"] - (back(C0.get(m), k).get("n") or 0)
check(f"вернулись: когорта {M3} — через 1 месяц +1 (z21k1), через 2 — +0, через 3 — +1 (этот месяц, going); {M1} — через 1 "
      "+1 (going), дальше — будущее без чисел",
      [dn(M3, 1), dn(M3, 2), dn(M3, 3), dn(M1, 1)] == [1, 0, 1, 1]
      and [back(C1.get(M3), k).get("state") for k in (1, 2, 3)] == ["done", "done", "going"]
      and [back(C1.get(M1), k).get("state") for k in (1, 2, 3)] == ["going", "future", "future"]
      and back(C1.get(M1), 2).get("n") is None and back(C1.get(M1), 2).get("pct") is None,
      J({M3: (C1.get(M3) or {}).get("back"), M1: (C1.get(M1) or {}).get("back")}))

section("всё — как прямой SQL от as_of")
NOW = f"'{P1.get('as_of')}'::timestamptz"
cm = lambda col: f"to_char(({col} AT TIME ZONE '{TZ}' - interval '{RH} hours')::date, 'YYYY-MM')"
want = {r[0]: r for r in (x.split("|") for x in rows(f"""
WITH f AS (SELECT s.user_id, {cm('MIN(s.started_at)')} AS m FROM sessions s JOIN users u ON u.id = s.user_id
           WHERE s.status <> 'cancelled' AND u.role = 'player' AND s.started_at < {NOW} GROUP BY s.user_id),
a AS (SELECT DISTINCT s.user_id, {cm('s.started_at')} AS m FROM sessions s JOIN users u ON u.id = s.user_id
      WHERE s.status <> 'cancelled' AND u.role = 'player' AND s.started_at < {NOW}),
mo AS (SELECT user_id, SUM(pln) AS pln FROM (
         SELECT user_id, amount_pln AS pln FROM deposits WHERE voided_at IS NULL AND created_at < {NOW}
         UNION ALL SELECT user_id, total_pln FROM sales WHERE voided_at IS NULL AND method NOT IN ('wallet', 'comp')
                   AND paid_at < {NOW} AND user_id IS NOT NULL
         UNION ALL SELECT user_id, price_pln FROM user_packages WHERE voided_at IS NULL AND method NOT IN ('wallet', 'comp')
                   AND created_at < {NOW}) x GROUP BY user_id)
SELECT f.m, COUNT(*),
  {", ".join(f"COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM a WHERE a.user_id = f.user_id AND a.m = "
             f"to_char(to_date(f.m, 'YYYY-MM') + interval '{k} month', 'YYYY-MM')))" for k in (1, 2, 3))},
  COALESCE(SUM(mo.pln), 0)
FROM f LEFT JOIN mo ON mo.user_id = f.user_id GROUP BY f.m"""))}


def want_row(m):
    r = want.get(m) or [m, "0", "0", "0", "0", "0"]
    size, b, rev = int(r[1]), [int(x) for x in r[2:5]], round(float(r[5]), 2)
    bs = []
    for k in (1, 2, 3):
        mk = month_add(m, k)
        st = "done" if mk < CUR else "going" if mk == CUR else "future"
        bs.append({"month": mk, "state": st, "n": None if st == "future" else b[k - 1],
                   "pct": None if st == "future" or not size else round(b[k - 1] / size * 1000) / 10})
    return {"month": m, "size": size, "revenue_pln": rev, "ltv_pln": round(rev / size, 2) if size else None, "back": bs}


got = P1.get("cohorts") or []
tup = lambda c: (c["month"], c["size"], [(b.get("month"), b.get("state"), b.get("n"), b.get("pct")) for b in c.get("back") or []])


def same(c, w):
    return (tup(c) == tup(w) and near(c["revenue_pln"], w["revenue_pln"]) and (c["ltv_pln"] is None) == (w["ltv_pln"] is None)
            and (c["ltv_pln"] is None or near(c["ltv_pln"], w["ltv_pln"], 0.011)))


bad = [(c, want_row(c["month"])) for c in got if not same(c, want_row(c["month"]))]
check("12 когорт: размер, вернулись через 1/2/3 месяца (число, доля, состояние), выручка и LTV — как прямой SQL",
      len(got) == 12 and not bad, J(bad[:2], 900))
c3 = C1.get(M3) or {}
check(f"LTV когорты {M3} — её выручка на гостя ({c3.get('revenue_pln')} / {c3.get('size')} = {c3.get('ltv_pln')}), доля "
      "вернувшихся — от размера когорты",
      c3.get("size") and near(c3.get("ltv_pln", -1), round(c3["revenue_pln"] / c3["size"], 2), 0.011)
      and back(c3, 1).get("pct") == round(back(c3, 1)["n"] / c3["size"] * 1000) / 10, J(c3))
cc = C1.get(CUR) or {}
check(f"когорта этого месяца ({CUR}): через 1/2/3 месяца — будущее, чисел нет",
      [b.get("state") for b in cc.get("back") or []] == ["future"] * 3 and all(b.get("n") is None for b in cc.get("back") or []),
      J(cc))
done()
