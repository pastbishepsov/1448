#!/usr/bin/env python3
"""Ж3: одно «сегодня» (клубные сутки от report_hour) и живой журнал. Живой e2e против стенда:
депозит до полуночи виден админу в журнале после полуночи тех же клубных суток (моделируем
сдвигом report_hour); дневной потолок админа считает те же сутки; KPI «Выручка сегодня» = итог
отчёта «Деньги» за сегодня; /admin/audit: страницы по before, лимит ≤200, kind — на сервере,
депозит со 160-й позиции дня находится фильтром «Деньги»; owner — любой день, admin — текущий.
Идемпотентен: свои строки удаляет, настройки возвращает. От загрузки стенда не зависит (З15-и2а): депозит для журнала —
свежий, 160 строк, которые его отодвигают, — между ним и «сейчас». От часа — тоже (З18-и4а): «сегодня» — клубные сутки,
а не дата календаря (до 08:00 они ещё вчерашние), в 23:xx — сценарий «в тот же день» вместо «через полночь»."""
import os, json, subprocess, sys, time, urllib.request, urllib.error, urllib.parse, datetime as dt

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr + "\n" + q)
    return r.stdout.strip().splitlines()[0] if r.stdout.strip() else ""
def api(m, p, b=None, t=None):
    req = urllib.request.Request(API + p, data=json.dumps(b).encode() if b is not None else None, method=m)
    req.add_header("Content-Type", "application/json")
    if t: req.add_header("Authorization", "Bearer " + t)
    try:
        with urllib.request.urlopen(req) as r: return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try: return e.code, json.loads(e.read() or b"{}")
        except Exception: return e.code, {}
    finally: time.sleep(0.13)
def login(n, p): return api("POST", "/auth/login", {"login": n, "password": p})[1]["access_token"]
ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    cond = bool(cond); ok += cond; fail += (not cond)
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))
def near(a, b): return abs(float(a) - float(b)) < 0.005

a1 = login("admin1", "Admin12345"); ot = login("boss", "Boss12345")
g = sql("SELECT id FROM users WHERE nickname='Guest28'")
admin1 = sql("SELECT id FROM users WHERE nickname='admin1'")
def cleanup():
    sql("DELETE FROM admin_actions WHERE details LIKE 'e2e-zh3%'")
    sql("DELETE FROM settings WHERE key IN ('admin_day_deposit_cap_pln')")
    api("PUT", "/admin/settings", {"report_hour": 8}, ot)
cleanup()
TZ = "Europe/Warsaw"  # часовой пояс клуба (CLUB_TZ сервера); сессия psql может жить в UTC
day_of = lambda off=0: sql(f"SELECT ((now() AT TIME ZONE '{TZ}')::date + {off})::text")  # дата по часам клуба
hour_now = int(sql(f"SELECT EXTRACT(HOUR FROM now() AT TIME ZONE '{TZ}')"))
# З18-и4а: сценарий — от часа. До 23:00 — «через полночь»: граница суток — следующий час, сутки начались вчера и держат
# вчерашние 23:50. В 23:xx границы позже текущего часа нет (раньше брали 22 — и депозит «вчера в 23:50» в сутки не
# попадал), поэтому «в тот же день»: граница — прошлый час, депозит — сегодня в нём же, в :50.
midnight = hour_now < 23
rh = hour_now + 1 if midnight else hour_now - 1
dep_sql = "- interval '10 minutes'" if midnight else f"+ interval '{hour_now - 1} hours 50 minutes'"
dep_txt = "со вчерашних 23:50" if midnight else f"с сегодняшних {hour_now - 1:02d}:50"
dep_at = "вчера 23:50" if midnight else f"сегодня {hour_now - 1:02d}:50"
dep_day = day_of(-1) if midnight else day_of()  # клубные сутки депозита — по дате их начала
rh2 = 0 if midnight else hour_now  # граница после депозита и не позже «сейчас»: депозит — уже в прошлых сутках
s, d = api("PUT", "/admin/settings", {"report_hour": rh}, ot)
check(f"настройка report_hour={rh} (сейчас {hour_now}:xx) → клубные сутки начались {'вчера' if midnight else 'сегодня'} в {rh}:00",
      s == 200, f"{s} {d}")

# ── A: депозит до границы суток виден админу в тех же клубных сутках («через полночь» — и после полуночи) ──
s, dep = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 7, "method": "cash"}, a1)
sql(f"UPDATE deposits SET created_at = (date_trunc('day', now() AT TIME ZONE '{TZ}') {dep_sql}) AT TIME ZONE '{TZ}' WHERE id='{dep['deposit_id']}'")
# курсор — сразу после этого депозита: страница не зависит от того, сколько пополнений стенд накопил за сутки
# (трек З, З8-и2: после десятка прогонов за день новые пополнения вытесняли его за 200-ю строку)
cur = sql(f"SELECT to_char((created_at + interval '1 second') AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM deposits WHERE id='{dep['deposit_id']}'")
s, au = api("GET", f"/admin/audit?kind=deposit&limit=200&before={cur}", None, a1)
ids = [x.get("id") for x in au.get("items", [])]
check(f"журнал админа: депозит {dep_txt} виден (клубные сутки ещё те же)", f"deposit:{dep['deposit_id']}" in ids, f"day={au.get('day')} from={au.get('from')} n={len(ids)}")
check("окно журнала админа начинается в report_hour суток депозита", str(au.get("from", "")).find(f"T{rh:02d}:00:00") > 0 and au.get("day") == dep_day, f"{au.get('from')} {au.get('day')}")
# дневной потолок админа — те же сутки: 7 zł уже «сегодня» → лимит 10 не пускает ещё 5
api("PUT", "/admin/settings", {"admin_day_deposit_cap_pln": 10}, ot)
s, d2 = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 5, "method": "cash"}, a1)
check(f"потолок депозитов админа считает клубные сутки: 7 ({dep_at}) + 5 > 10 → 403 day_cap", s == 403 and d2.get("code") == "day_cap", f"{s} {d2.get('code')} {d2.get('message')}")
sql("DELETE FROM settings WHERE key='admin_day_deposit_cap_pln'")
# граница — после депозита и не позже «сейчас» (полночь или текущий час): депозит — уже прошлые сутки, из журнала админа
# он уходит. З18-и4а: раньше здесь возвращали 8 — а до 08:00 сутки с границей 8 ещё вчерашние, и депозит оставался
api("PUT", "/admin/settings", {"report_hour": rh2}, ot)
s, au = api("GET", "/admin/audit?kind=deposit&limit=200", None, a1)
check(f"report_hour={rh2}: тот же депозит из журнала админа пропал (прошлые сутки)", f"deposit:{dep['deposit_id']}" not in [x.get("id") for x in au.get("items", [])] and au.get("day") == day_of(), f"day={au.get('day')}")
s, au_o = api("GET", f"/admin/audit?kind=deposit&limit=200&day={day_of(-1)}", None, ot)
check("владелец с ?day=вчера видит этот депозит", f"deposit:{dep['deposit_id']}" in [x.get("id") for x in au_o.get("items", [])], f"n={au_o.get('count')} day={au_o.get('day')}")
api("POST", f"/admin/deposits/{dep['deposit_id']}/void", {}, ot)  # уборка денег гостя (владелец: депозит прошлых суток)
api("PUT", "/admin/settings", {"report_hour": 8}, ot)  # дальше — граница стенда

# ── B: KPI «Выручка сегодня» = итог отчёта «Деньги» за сегодня ──
api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 12, "method": "card"}, a1)
goods = api("GET", "/admin/goods", None, a1)[1].get("goods", [])
gd = next((x for x in goods if x.get("active") and x.get("stock", 0) > 0), None)
if gd: api("POST", "/admin/sales", {"good_id": gd["id"], "qty": 1, "method": "cash"}, a1)
s, ov = api("GET", "/admin/overview", None, a1)
s, rm = api("GET", "/admin/reports/money?preset=today", None, ot)
check(f"KPI revenue_today_pln {ov.get('revenue_today_pln')} = отчёт «Деньги» за сегодня {rm['totals']['revenue_pln']} (пополнения+товары+пакеты)",
      near(ov.get("revenue_today_pln", -1), rm["totals"]["revenue_pln"]), f"{ov.get('revenue_today')}")
check("состав KPI совпадает с отчётом: депозиты, товары, пакеты", near(ov["revenue_today"]["deposits_pln"], rm["totals"]["deposits_pln"]) and near(ov["revenue_today"]["goods_pln"], rm["totals"]["goods_pln"]) and near(ov["revenue_today"]["packs_pln"], rm["totals"]["packs_pln"]), f"{ov.get('revenue_today')} vs {rm['totals']}")
check("окно KPI — клубные сутки от 08:00", str(ov.get("day_from", "")).find("T08:00:00") > 0 and ov.get("report_hour") == 8, f"{ov.get('day_from')}")

# ── C: журнал страницами; депозит со 160-й позиции находится фильтром «Деньги» ──
# З15-и2а: депозит свежий, а 160 строк журнала, которые его отодвигают, — между ним и «сейчас» (через микросекунду после
# него, по порядку). Раньше депозит уезжал на 400 с назад, и после плотной работы стенда — больше 100 денежных событий
# за эти 400 с — фильтр «Деньги» выталкивал его с первой страницы: проверка падала не из-за сервера.
s, dep2 = api("POST", f"/admin/users/{g}/deposit", {"amount_pln": 3, "method": "cash"}, a1)
sql(f"INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '{admin1}', 'pc_hold', 'e2e-zh3 #' || i, "
    f"d.created_at + (i || ' microseconds')::interval FROM generate_series(1, 160) i, deposits d WHERE d.id = '{dep2['deposit_id']}'")
s, p1 = api("GET", "/admin/audit", None, a1)
items1 = p1.get("items", [])
check(f"страница 1: {len(items1)} строк (лимит 100), has_more, next_before", len(items1) == 100 and p1.get("has_more") is True and p1.get("next_before"), f"{p1.get('count')} {p1.get('has_more')}")
check("депозит со 160+ позиции НЕ на первой странице", f"deposit:{dep2['deposit_id']}" not in [x["id"] for x in items1])
ts = [x["created_at"] for x in items1]
check("страница отсортирована по времени вниз", ts == sorted(ts, reverse=True))
s, p2 = api("GET", f"/admin/audit?before={urllib.parse.quote(p1['next_before'])}", None, a1)
items2 = p2.get("items", [])
check(f"страница 2 по before: {len(items2)} строк, все старше последней строки первой", len(items2) > 0 and all(x["created_at"] < items1[-1]["created_at"] for x in items2) and not (set(x["id"] for x in items2) & set(x["id"] for x in items1)), f"{p2.get('count')}")
allids = set(x["id"] for x in items1) | set(x["id"] for x in items2)
pages, cur = 2, p2
while f"deposit:{dep2['deposit_id']}" not in allids and cur.get("has_more") and pages < 10:  # день бойкий — листаем дальше
    s, cur = api("GET", f"/admin/audit?before={urllib.parse.quote(cur['next_before'])}", None, a1); pages += 1
    allids |= set(x["id"] for x in cur.get("items", []))
check(f"депозит найден листанием по before (страниц: {pages})", f"deposit:{dep2['deposit_id']}" in allids)
money = "deposit,deposit_void,sale,sale_void,coin_redeem,coin_redeem_void,coin_burn,incident_compensate,package_issue,package_void,package_warn,kitchen_done,kitchen_pay,kitchen_cancel"
s, pm = api("GET", f"/admin/audit?kind={money}", None, a1)
kinds = set(x["kind"] for x in pm.get("items", []))
check("фильтр «Деньги» на сервере: депозит с 160-й позиции на первой странице, чужих видов нет",
      f"deposit:{dep2['deposit_id']}" in [x["id"] for x in pm.get("items", [])] and kinds <= set(money.split(",")), f"{kinds}")
s, pl = api("GET", "/admin/audit?limit=500", None, a1)
check("limit=500 режется до 200", pl.get("limit") == 200 and pl.get("count") <= 200, f"{pl.get('limit')} {pl.get('count')}")
s, pb = api("GET", "/admin/audit?before=abc", None, a1)
check("before не по формату → 400 bad_before", s == 400 and pb.get("code") == "bad_before", f"{s} {pb.get('code')}")
s, pd = api("GET", "/admin/audit?day=2026-13-40", None, ot)
check("владелец: day не по формату → 400 bad_day", s == 400 and pd.get("code") == "bad_day", f"{s} {pd.get('code')}")
# З18-и4а: текущие клубные сутки — от 08:00 по часам клуба (до 08:00 это ещё вчерашняя дата), не дата календаря
club_now = sql(f"SELECT ((now() AT TIME ZONE '{TZ}') - interval '8 hours')::date::text")
s, pa = api("GET", f"/admin/audit?day={day_of(-3)}", None, a1)
check("админ: ?day игнорируется — всегда текущие клубные сутки", pa.get("day") == club_now, f"{pa.get('day')} при {club_now}")
s, po = api("GET", "/admin/audit?limit=30", None, ot)
check("владелец без day: всё время, 30 свежих, у каждой строки id", po.get("count") == 30 and all(x.get("id") for x in po.get("items", [])) and "day" not in po, f"{po.get('count')} {'day' in po}")
# бэкфилл ленты: 30 последних тем же запросом у админа
s, pf = api("GET", "/admin/audit?limit=30", None, a1)
check("админ ?limit=30 — 30 строк текущих суток для ленты зала", pf.get("count") == 30 and pf.get("day") == club_now, f"{pf.get('count')} {pf.get('day')}")

cleanup()
api("POST", f"/admin/deposits/{dep2['deposit_id']}/void", {}, a1)
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
