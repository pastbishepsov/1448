#!/usr/bin/env python3
"""Ж8 «Стойка вперёд» (Ж-Р12, №37; №33, №34): табло зала под токеном (без ников на ПК, с никами
в очереди), посадка компании (`/admin/seat-group` + suggest соседних ПК). Живой e2e против стенда.
Идемпотентен: сессии Guest18/19/23/30 снимает, брони на PC-09 отменяет, очередь чистит, табло гасит."""
import os, json, subprocess, sys, time, os, urllib.request, urllib.error, datetime as dt, math

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr + "\n" + q)
    return r.stdout.strip().splitlines()[0] if r.stdout.strip() else ""
def sql_rows(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr + "\n" + q)
    return [x for x in r.stdout.strip().splitlines() if x]
def api(m, p, b=None, t=None, raw=False):
    req = urllib.request.Request(API + p, data=json.dumps(b).encode() if b is not None else None, method=m)
    req.add_header("Content-Type", "application/json")
    if t: req.add_header("Authorization", "Bearer " + t)
    try:
        with urllib.request.urlopen(req) as r:
            body = r.read()
            return (r.status, body.decode()) if raw else (r.status, json.loads(body or b"{}"))
    except urllib.error.HTTPError as e:
        try:
            body = e.read()
            return (e.code, body.decode()) if raw else (e.code, json.loads(body or b"{}"))
        except Exception: return e.code, {}
    finally: time.sleep(0.12)
def login(n, p): return api("POST", "/auth/login", {"login": n, "password": p})[1]["access_token"]
ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    cond = bool(cond); ok += cond; fail += (not cond)
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))

agents = {}
def agent_start(pc_id, name):
    log = agent_log(name)
    if os.path.exists(log): os.remove(log)
    p = subprocess.Popen(["node", MOCK_AGENT, pc_id, log], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, start_new_session=True)
    agents[name] = (p, log)
    for _ in range(30):
        if os.path.exists(log) and '"open"' in open(log).read(): break
        time.sleep(0.1)
    time.sleep(0.4)
def agent_stop(name):
    if name in agents:
        agents[name][0].terminate()
        try: agents[name][0].wait(3)
        except Exception: pass
        del agents[name]
        time.sleep(0.6)

a1 = login("admin1", "Admin12345"); ot = login("boss", "Boss12345")
G = {n: sql(f"SELECT id FROM users WHERE nickname='{n}'") for n in ("Guest18", "Guest19", "Guest23", "Guest30")}
pcs = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
VIP = sql("SELECT id FROM zones WHERE name='VIP'"); STD = sql("SELECT id FROM zones WHERE name='Standard'")
maint_before = sql_rows("SELECT id FROM computers WHERE status='maintenance'")

def reset():
    for name in list(agents): agent_stop(name)
    for uid in G.values():
        for sid in sql(f"SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='{uid}' AND status='active'").split():
            api("POST", f"/admin/sessions/{sid}/end", {}, a1)
    sql("UPDATE computers SET status='available' WHERE status IN ('maintenance','in_session','reserved') AND id NOT IN (SELECT computer_id FROM sessions WHERE status='active')"
        + (" AND id NOT IN (" + ",".join(f"'{i}'" for i in maint_before) + ")" if maint_before else ""))
    ids = ",".join(f"'{i}'" for i in G.values())
    sql(f"UPDATE waitlist SET status='removed', resolved_at=now() WHERE status='waiting' AND user_id IN ({ids})")
    sql(f"UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id IN ({ids})")
    api("DELETE", "/admin/board/token", None, ot)
reset()
# кошельков хватит на посадку (min_start_minutes): Guest18 — 30 zł, остальные ≥ 60
def dist(a, b): return math.hypot(a["pos_x"] - b["pos_x"], a["pos_y"] - b["pos_y"])
def pair_sum(lst): return sum(dist(lst[i], lst[j]) for i in range(len(lst)) for j in range(i + 1, len(lst)))
def state(tok): return api("GET", "/board/state" + (f"?token={tok}" if tok is not None else ""))

# ── A: табло — токен владельца ─────────────────────────────────────────────
print("── A: табло: токен ──")
s, d = api("GET", "/admin/board/token", None, a1)
check("GET /admin/board/token админом → 403 (только владелец)", s == 403, f"{s} {d.get('code')}")
s, d = api("GET", "/admin/board/token", None, ot)
check("владелец: табло выключено — enabled=false, path пуст", s == 200 and d.get("enabled") is False and d.get("path") == "" and d.get("token") == "", f"{s} {d}")
s, d = state(None)
check("GET /board/state без токена → 401 board_token", s == 401 and d.get("code") == "board_token", f"{s} {d.get('code')}")
s, d = state("1234")
check("с левым токеном → 401", s == 401, f"{s}")
s, d = state("")
check("с пустым токеном → 401 (пустая настройка ничему не равна)", s == 401, f"{s}")
s, d = api("POST", "/admin/board/token", None, ot)
tok = d.get("token", "")
check("POST → выпущен токен: 48 hex, enabled, path=/board?token=…", s == 200 and len(tok) == 48 and all(c in "0123456789abcdef" for c in tok) and d.get("enabled") is True and d.get("path") == "/board?token=" + tok, f"{s} {d}")
s, d = api("GET", "/admin/board/token", None, ot)
check("GET после выпуска — тот же токен", s == 200 and d.get("token") == tok, f"{d.get('token')}")
r = urllib.request.urlopen(BASE + "/board?token=" + tok); page = r.read().decode()
check("страница /board отдаётся (200), это табло 14:48", r.status == 200 and "табло" in page.lower() and "board/state" in page, f"{r.status} {len(page)}")

# ── B: табло — состояние зала без ников ────────────────────────────────────
print("── B: табло: зал, очередь, брони ──")
s, d = state(tok)
check("с токеном → 200, refresh_sec=10, club, now", s == 200 and d.get("refresh_sec") == 10 and d.get("club") and d.get("now"), f"{s} {list(d)}")
zones = d.get("zones") or []
total_pcs = int(sql("SELECT count(*) FROM computers WHERE status<>'maintenance'"))
check("зоны есть, total = все ПК не в ремонте, free = сумма по зонам", zones and d.get("total") == total_pcs and d.get("free") == sum(z["free"] for z in zones) and all(z["free"] + len(z["busy_list"]) == z["total"] for z in zones), f"{d.get('total')}/{total_pcs} {[(z['name'], z['free'], z['total']) for z in zones]}")
check("в зонах — имя, цена, списки ПК; никаких ников/user_id", all(set(z) == {"name", "rate_pln", "free", "total", "free_list", "busy_list"} for z in zones), f"{[sorted(z) for z in zones]}")
std = next((z for z in zones if z["name"] == "Standard"), None)
check("Standard: 20 zł/ч, PC-01…PC-09 свободны", std and abs(std["rate_pln"] - 20) < 0.01 and "PC-05" in std["free_list"] and std["busy_list"] == [], f"{std}")

# гость за PC-05 — на табло машина занята, ника нет
agent_start(pcs["PC-05"]["id"], "p5")
gt30 = login("Guest30", "Guest12345")
s, sd = api("POST", f"/admin/computers/{pcs['PC-05']['id']}/session", {"guest": "Guest30", "planned_min": 60}, a1)
check("посадка Guest30 за PC-05 → 201", s == 201, f"{s} {sd.get('code')}")
api("POST", "/me/sessions/ready", {}, gt30)
s, d = state(tok)
std = next(z for z in d["zones"] if z["name"] == "Standard")
check("табло: PC-05 в busy_list, не в free_list; free уменьшился", "PC-05" in std["busy_list"] and "PC-05" not in std["free_list"] and d["free"] == total_pcs - 1, f"{std['busy_list']} free={d['free']}")
check("ника Guest30 в ответе табло нет вообще", "Guest30" not in json.dumps(d, ensure_ascii=False) and G["Guest30"] not in json.dumps(d), "")

# очередь — по никам
sql("UPDATE computers SET status='maintenance' WHERE status='available'")  # очередь принимает, только когда свободных нет
s, w = api("POST", "/admin/waitlist", {"guest": "Guest18"}, a1)
check("Guest18 в очередь → 201", s in (200, 201), f"{s} {w.get('code')}")
s, d = state(tok)
q = d.get("queue") or []
check("табло: очередь с ником — position 1, nickname Guest18, waited_min, called=false", q and q[0]["nickname"] == "Guest18" and q[0]["position"] == 1 and q[0].get("called") is False and "waited_min" in q[0], f"{q}")
check("в ремонте ПК табло не считает: total упал, free=0", d["total"] == 1 and d["free"] == 0, f"total={d['total']} free={d['free']}")
sql("UPDATE computers SET status='available' WHERE status='maintenance'" + (" AND id NOT IN (" + ",".join(f"'{i}'" for i in maint_before) + ")" if maint_before else ""))
wid = sql(f"SELECT id FROM waitlist WHERE status='waiting' AND user_id='{G['Guest18']}' ORDER BY created_at DESC LIMIT 1")
sql(f"UPDATE waitlist SET notified_at=now() WHERE id='{wid}'")
s, d = state(tok)
check("после вызова (notified_at) — called=true", (d.get("queue") or [{}])[0].get("called") is True, f"{d.get('queue')}")
api("DELETE", f"/admin/waitlist/{wid}", None, a1)
s, d = state(tok)
check("сняли из очереди — на табло пусто", d.get("queue") == [], f"{d.get('queue')}")

# брони — ближайшие 3 часа, по ПК, без ников
t1 = (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=70)).replace(second=0, microsecond=0)
t5 = t1 + dt.timedelta(hours=4)
s, b1 = api("POST", "/admin/bookings", {"guest_id": G["Guest23"], "computer_id": pcs["PC-09"]["id"], "start_time": t1.isoformat().replace("+00:00", "Z"), "duration_min": 60}, a1)
s2, b2 = api("POST", "/admin/bookings", {"guest_id": G["Guest19"], "computer_id": pcs["PC-08"]["id"], "start_time": t5.isoformat().replace("+00:00", "Z"), "duration_min": 60}, a1)
check("две брони созданы (через 70 мин на PC-09, через 5 ч на PC-08)", s in (200, 201) and s2 in (200, 201), f"{s} {b1.get('code')} {s2} {b2.get('code')}")
s, d = state(tok)
bk = d.get("bookings") or []
check("табло: бронь PC-09 в списке (3 ч), PC-08 через 5 ч — нет; без ников", any(b.get("computer") == "PC-09" and b["duration_min"] == 60 for b in bk) and not any(b.get("computer") == "PC-08" for b in bk) and "Guest23" not in json.dumps(d), f"{bk}")
check("список броней отсортирован по времени", [b["start_time"] for b in bk] == sorted(b["start_time"] for b in bk), "")

# ── C: перевыпуск и выключение ─────────────────────────────────────────────
print("── C: перевыпуск / выключение ──")
s, d = api("POST", "/admin/board/token", None, ot)
tok2 = d.get("token", "")
check("перевыпуск → новый токен, не равен старому", s == 200 and len(tok2) == 48 and tok2 != tok, f"{tok[:6]}→{tok2[:6]}")
check("старый токен → 401, новый → 200", state(tok)[0] == 401 and state(tok2)[0] == 200, "")
s, d = api("DELETE", "/admin/board/token", None, ot)
check("DELETE → выключено", s == 200 and d.get("enabled") is False, f"{s} {d}")
check("после выключения новый токен тоже 401", state(tok2)[0] == 401, "")
s, d = api("GET", "/admin/audit?kind=board_token&limit=30", None, ot)
acts = [a for a in d.get("items", []) if a.get("kind") == "board_token"]
check("в аудите board_token: выпуск ×2 и выключение", len(acts) >= 3 and any("выключено" in (a.get("text") or "") for a in acts) and sum(1 for a in acts if "новый токен" in (a.get("text") or "")) >= 2, f"{[(a.get('kind'), a.get('text')) for a in acts[:4]]}")

# ── D: компания — suggest ──────────────────────────────────────────────────
print("── D: компания: suggest ──")
s, d = api("GET", "/admin/seat-group/suggest?n=1", None, a1)
check("n=1 → 400 bad_size", s == 400 and d.get("code") == "bad_size", f"{s} {d.get('code')}")
s, d = api("GET", "/admin/seat-group/suggest?n=7", None, a1)
check("n=7 → 400 bad_size", s == 400 and d.get("code") == "bad_size", f"{s}")
s, d = api("GET", "/admin/seat-group/suggest?n=2&zone_id=abc", None, a1)
check("zone_id не UUID → 400 bad_zone", s == 400 and d.get("code") == "bad_zone", f"{s} {d.get('code')}")
s, d = api("GET", "/admin/seat-group/suggest?n=3", None, a1)
sug = d.get("suggested") or []
check("n=3 → 200, enough, 3 ПК предложено, все из free", s == 200 and d.get("enough") is True and len(sug) == 3 and all(x["id"] in {f["id"] for f in d["free"]} for x in sug), f"{s} {[x['name'] for x in sug]}")
check("предложены соседи: сумма попарных расстояний минимальна (ряд из трёх = 8)", all("pos_x" in x for x in sug) and abs(pair_sum(sug) - 8) < 1e-6, f"{[x['name'] for x in sug]} sum={pair_sum(sug) if all('pos_x' in x for x in sug) else '?'}")
check("PC-05 (занят Guest30) в free нет", "PC-05" not in {f["name"] for f in d["free"]}, "")
s, d = api("GET", f"/admin/seat-group/suggest?n=2&zone_id={VIP}", None, a1)
check("зона VIP → только VIP-машины, пара соседей", s == 200 and len(d["suggested"]) == 2 and all(x["name"].startswith("VIP") for x in d["suggested"]) and abs(pair_sum(d["suggested"]) - 2) < 1e-6, f"{[x['name'] for x in d.get('suggested', [])]}")
s, d = api("GET", "/admin/seat-group/suggest?n=2&planned_min=120", None, a1)
check("planned 120 мин: PC-09 (бронь через 70 мин) в free нет", "PC-09" not in {f["name"] for f in d["free"]}, f"{[f['name'] for f in d['free']]}")
s, d = api("GET", "/admin/seat-group/suggest?n=2&planned_min=30", None, a1)
check("planned 30 мин: PC-09 успеет — в free есть", "PC-09" in {f["name"] for f in d["free"]}, f"{[f['name'] for f in d['free']]}")
s, d = api("GET", "/admin/seat-group/suggest?n=6", None, a1)
check("n=6 → шесть ПК (хватает)", s == 200 and len(d["suggested"]) == 6 and d["enough"] is True, f"{len(d.get('suggested', []))}")

# ── E: компания — посадка ──────────────────────────────────────────────────
print("── E: компания: посадка ──")
P1, P2, P3, P7, P8 = (pcs[n]["id"] for n in ("PC-01", "PC-02", "PC-03", "PC-07", "PC-08"))
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P1}]}, a1)
check("одно место → 400 bad_size", s == 400 and d.get("code") == "bad_size", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P1}, {"guest": "Guest23", "computer_id": P1}]}, a1)
check("один ПК дважды → 400 dup_computer", s == 400 and d.get("code") == "dup_computer", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P1}, {"guest": "Guest23"}]}, a1)
check("без ПК → 400 need_computer", s == 400 and d.get("code") == "need_computer", f"{s} {d.get('code')}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "admin2", "computer_id": P7}, {"guest": "admin2", "computer_id": P8}]}, a1)
rs = d.get("results") or []
check("персонал в компании → 200, seated 0; резолвер ищет только гостей → user_not_found (как у очереди, Ж5)", s == 200 and d.get("seated") == 0 and len(rs) == 2 and all(r.get("code") == "user_not_found" for r in rs), f"{s} {[(r.get('code')) for r in rs]}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest30", "computer_id": P7}, {"guest": "+48 500 120 030", "computer_id": P8}]}, a1)
rs = d.get("results") or []
check("один гость дважды (ник и телефон) → строка 0 с кодом (уже сидит), строка 1 dup_guest", s == 200 and d.get("seated") == 0 and rs[0].get("ok") is False and rs[0].get("code") and rs[0].get("code") != "dup_guest" and rs[1].get("code") == "dup_guest", f"{s} {[(r.get('code')) for r in rs]}")
check("ничего не посажено", sql("SELECT count(*) FROM sessions WHERE status='active' AND computer_id IN ('%s','%s')" % (P7, P8)) == "0", "")

agent_start(P1, "p1"); agent_start(P2, "p2")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P1}, {"nickname": "Guest23", "computer_id": P2}, {"guest": "NoSuchGuest", "computer_id": P3}], "planned_min": 90}, a1)
rs = d.get("results") or []
check("компания из трёх, один не найден → 200 seated=2 total=3", s == 200 and d.get("seated") == 2 and d.get("total") == 3 and len(rs) == 3, f"{s} {d.get('seated')}/{d.get('total')}")
check("строки: 0 и 1 ok с session_id, ники и ПК; 2 — user_not_found", rs[0].get("ok") and rs[0].get("session_id") and rs[0].get("nickname") == "Guest19" and rs[0].get("computer") == "PC-01" and rs[1].get("ok") and rs[1].get("nickname") == "Guest23" and rs[2].get("ok") is False and rs[2].get("code") == "user_not_found" and rs[2].get("computer") == "PC-03", f"{[(r.get('ok'), r.get('code'), r.get('nickname'), r.get('computer')) for r in rs]}")
check("шеллы на связи — окно [Готов!], без shell_wait_until", not rs[0].get("shell_wait_until") and not rs[1].get("shell_wait_until"), f"{rs[0].get('shell_wait_until')}")
check("в базе две активные сессии Guest19/Guest23", sql(f"SELECT count(*) FROM sessions WHERE status='active' AND user_id IN ('{G['Guest19']}','{G['Guest23']}')") == "2", "")
comp = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
check("/admin/computers: PC-01 и PC-02 с сессиями компании, PC-03 свободен", (comp["PC-01"].get("session") or {}).get("user_id") == G["Guest19"] and comp["PC-02"].get("session") and not comp["PC-03"].get("session"), f"{comp['PC-01'].get('status')} {comp['PC-02'].get('status')} {comp['PC-03'].get('status')}")
s, d = api("GET", "/admin/audit?kind=session_start,seat_group&limit=30", None, ot)
acts = d.get("items", [])
check("аудит: session_start «… · компания» ×2 и seat_group «посажено 2 из 3»", sum(1 for a in acts if a.get("kind") == "session_start" and "компания" in (a.get("text") or "")) >= 2 and any(a.get("kind") == "seat_group" and "2 из 3" in (a.get("text") or "") for a in acts), f"{[(a.get('kind'), a.get('text')) for a in acts[:5]]}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P3}, {"guest": "Guest18", "computer_id": P7}]}, a1)
rs = d.get("results") or []
check("Guest19 уже сидит → его строка с кодом, Guest18 сел за PC-07 (seated 1 из 2)", s == 200 and d.get("seated") == 1 and rs[0].get("ok") is False and rs[0].get("code") and rs[1].get("ok") is True, f"{[(r.get('ok'), r.get('code')) for r in rs]}")
check("PC-03 остался свободен (сессии сорванной строки нет)", sql(f"SELECT count(*) FROM sessions WHERE status='active' AND computer_id='{P3}'") == "0", "")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest30", "computer_id": P3}, {"guest": "Guest18", "computer_id": P8}]}, a1)
rs = d.get("results") or []
check("оба уже сидят → 200 seated 0, обе строки с кодами", s == 200 and d.get("seated") == 0 and all(r.get("ok") is False and r.get("code") for r in rs), f"{[(r.get('code')) for r in rs]}")
s, d = api("GET", "/admin/seat-group/suggest?n=2", None, a1)
check("suggest после посадок: PC-01/02/05/07 не в free", not ({"PC-01", "PC-02", "PC-05", "PC-07"} & {f["name"] for f in d["free"]}), f"{[f['name'] for f in d['free']]}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P1}, {"guest": "Guest23", "computer_id": P2}]}, ot)
check("владелец тоже может (роль admin+)", s == 200, f"{s}")
s, d = api("POST", "/admin/seat-group", {"seats": [{"guest": "Guest19", "computer_id": P1}, {"guest": "Guest23", "computer_id": P2}]}, gt30)
check("гость → 403", s == 403, f"{s}")

reset()
check("после reset активных сессий у тестовых гостей нет, табло выключено", sql("SELECT count(*) FROM sessions WHERE status='active' AND user_id IN (%s)" % ",".join(f"'{i}'" for i in G.values())) == "0" and state("x")[0] == 401, "")
print(f"\n{ok} ok, {fail} FAIL")
sys.exit(1 if fail else 0)
