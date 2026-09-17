#!/usr/bin/env python3
"""Ж0-и3/и4 (Ж-Р5): гонка броней закрыта базой (миграция 061), walk-in бронь под теми же
правилами, что гостевая: бан, лимит живых броней, «одна бронь на одно время».
Живой e2e против стенда. Идемпотентен: свои брони снимает, настройки возвращает."""
import os, json, subprocess, sys, time, urllib.request, urllib.error, datetime as dt
import concurrent.futures as cf

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr)
    return r.stdout.strip()
def api(m, p, b=None, t=None, pause=0.13):
    req = urllib.request.Request(API + p, data=json.dumps(b).encode() if b is not None else None, method=m)
    req.add_header("Content-Type", "application/json")
    if t: req.add_header("Authorization", "Bearer " + t)
    try:
        with urllib.request.urlopen(req) as r: return r.status, json.loads(r.read() or b"{}")
    except urllib.error.HTTPError as e:
        try: return e.code, json.loads(e.read() or b"{}")
        except Exception: return e.code, {}
    finally: time.sleep(pause)
def login(n, p): return api("POST", "/auth/login", {"login": n, "password": p})[1]["access_token"]
ok = fail = 0
def check(name, cond, detail=""):
    global ok, fail
    ok += cond; fail += (not cond)
    print(("ok   " if cond else "FAIL ") + name + (f" — {detail}" if detail else ""))
def iso(hours, minutes=0):
    return (dt.datetime.now(dt.timezone.utc) + dt.timedelta(hours=hours, minutes=minutes)).replace(microsecond=0).isoformat().replace("+00:00", "Z")

at = login("admin1", "Admin12345"); ot = login("boss", "Boss12345")
club = sql("SELECT id FROM clubs LIMIT 1")
pcs = api("GET", "/admin/computers", None, at)[1]["computers"]
pc = {p["name"]: p["id"] for p in pcs}
# чистый старт: снимаем живые брони тестовых гостей, настройки по умолчанию
for n in ("Guest11", "Guest12", "Guest13", "Guest14", "Guest15", "Guest16", "Guest17"):
    sql(f"UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id=(SELECT id FROM users WHERE nickname='{n}')")
sql("DELETE FROM settings WHERE key='max_active_bookings'")
sql("UPDATE users SET status='active' WHERE nickname IN ('Guest16')")
check("ограничение bookings_no_overlap стоит", sql("SELECT count(*) FROM pg_constraint WHERE conname='bookings_no_overlap'") == "1")

# ── и3: гонка — 6 параллельных walk-in на один ПК и время ──
t5 = iso(5)
def book(nick): return api("POST", "/admin/bookings", {"guest": nick, "computer_id": pc["PC-05"], "start_time": t5, "duration_min": 60}, at, pause=0)
with cf.ThreadPoolExecutor(6) as ex: res = list(ex.map(book, ["Guest11", "Guest12", "Guest13", "Guest14", "Guest15", "Guest16"]))
codes = sorted(r[0] for r in res)
check(f"6 параллельных броней PC-05 → одна 201, остальные 409: {codes}", codes.count(201) == 1 and codes.count(409) == 5)
race_codes = {r[1].get("code") for r in res if r[0] == 409}
check(f"отказы человеческие: {race_codes}", race_codes <= {"slot_busy"})
live = sql(f"SELECT count(*) FROM bookings WHERE computer_id='{pc['PC-05']}' AND status IN ('pending','confirmed') AND start_time='{t5}'")
check(f"в базе живых броней PC-05 на это время: {live}", live == "1")
time.sleep(0.5)
# ── и3: restore в занятый слот ──
winner = [r for r in res if r[0] == 201][0][1]
s, d = api("POST", f"/admin/bookings/{winner['booking_id']}/cancel", {}, at)
s2, d2 = api("POST", "/admin/bookings", {"guest": "Guest17", "computer_id": pc["PC-05"], "start_time": t5, "duration_min": 60}, at)
check("после отмены слот свободен → 201 другому гостю", s2 == 201, f"{s2} {d2.get('code')}")
s3, d3 = api("POST", f"/admin/bookings/{winner['booking_id']}/restore", {}, at)
check("вернуть отменённую в занятый слот → 409 slot_busy", s3 == 409 and d3.get("code") == "slot_busy", f"{s3} {d3.get('code')}")

# ── и4: бан ──
g16 = sql("SELECT id FROM users WHERE nickname='Guest16'")
api("POST", f"/admin/users/{g16}/ban", {}, at)
s, d = api("POST", "/admin/bookings", {"guest": "Guest16", "start_time": iso(7), "duration_min": 60}, at)
check("walk-in на забаненного → 409 banned", s == 409 and d.get("code") == "banned", f"{s} {d.get('code')}")
api("POST", f"/admin/users/{g16}/unban", {}, at)

# ── и4: пересечение по гостю ──
s1, d1 = api("POST", "/admin/bookings", {"guest": "Guest15", "computer_id": pc["PC-06"], "start_time": iso(8), "duration_min": 60}, at)
s2, d2 = api("POST", "/admin/bookings", {"guest": "Guest15", "computer_id": pc["PC-07"], "start_time": iso(8, 30), "duration_min": 60}, at)
check("тот же гость, другой ПК, время пересекается → 409 guest_busy", s2 == 409 and d2.get("code") == "guest_busy", f"{s2} {d2.get('code')} {d2.get('message')}")
s3, d3 = api("POST", "/admin/bookings", {"guest": "Guest15", "computer_id": pc["PC-07"], "start_time": iso(9), "duration_min": 60}, at)
check("тот же гость, встык (конец 1-й = начало 2-й) → 201", s3 == 201, f"{s3} {d3.get('code')}")
# гостевой путь — те же правила
gt = login("Guest15", "Guest12345")
api("PUT", "/admin/settings", {"booking_min_level": 0, "max_active_bookings": 0}, ot)  # Guest15 — LVL 1 и уже 2 живых брони; здесь проверяем именно пересечение
s4, d4 = api("POST", f"/clubs/{club}/bookings", {"computer_id": pc["PC-08"], "start_time": iso(8, 15), "duration_min": 60}, gt)
check("гость из PWA на своё же время → 409 guest_busy", s4 == 409 and d4.get("code") == "guest_busy", f"{s4} {d4.get('code')}")
sql("DELETE FROM settings WHERE key IN ('booking_min_level','max_active_bookings')")
# ── и4: лимит живых броней действует и на walk-in ──
api("PUT", "/admin/settings", {"max_active_bookings": 2}, ot)
s5, d5 = api("POST", "/admin/bookings", {"guest": "Guest15", "computer_id": pc["PC-08"], "start_time": iso(12), "duration_min": 60}, at)
check("третья живая бронь при лимите 2 → 409 booking_limit", s5 == 409 and d5.get("code") == "booking_limit", f"{s5} {d5.get('code')}")
api("PUT", "/admin/settings", {"max_active_bookings": 0}, ot)
s6, d6 = api("POST", "/admin/bookings", {"guest": "Guest15", "computer_id": pc["PC-08"], "start_time": iso(12), "duration_min": 60}, at)
check("лимит 0 (без лимита) → 201", s6 == 201, f"{s6} {d6.get('code')}")
# восстановление под лимит: две живые + restore третьей при лимите 2 → 409
for bid in (d6.get("booking_id"),):
    api("POST", f"/admin/bookings/{bid}/cancel", {}, at)
api("PUT", "/admin/settings", {"max_active_bookings": 2}, ot)
s7, d7 = api("POST", f"/admin/bookings/{d6.get('booking_id')}/restore", {}, at)
check("restore третьей при лимите 2 → 409 booking_limit", s7 == 409 and d7.get("code") == "booking_limit", f"{s7} {d7.get('code')}")
sql("DELETE FROM settings WHERE key='max_active_bookings'")

# уборка
for n in ("Guest11", "Guest12", "Guest13", "Guest14", "Guest15", "Guest16", "Guest17"):
    sql(f"UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND user_id=(SELECT id FROM users WHERE nickname='{n}')")
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
