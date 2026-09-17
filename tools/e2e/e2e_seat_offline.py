#!/usr/bin/env python3
"""Ж4 (Ж-Р6, Ж-Р9): посадка на ПК без связи, авто-WoL, «Освободить ПК», вызов за гостем,
«на сколько» при посадке. Живой e2e против стенда с моком агента (node mock_agent.js).
Идемпотентен: сессии Guest29 снимает, ПК возвращает, настройки восстанавливает."""
import os, json, subprocess, sys, time, os, signal, urllib.request, urllib.error, datetime as dt

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
    return log
def agent_stop(name):
    if name in agents:
        agents[name][0].terminate()
        try: agents[name][0].wait(3)
        except Exception: pass
        del agents[name]
        time.sleep(0.6)
def agent_msgs(name, typ):
    log = agents[name][1] if name in agents else agent_log(name)
    if not os.path.exists(log): return []
    out = []
    for line in open(log):
        try: o = json.loads(line)
        except Exception: continue
        if o.get("ev") == "msg" and (o.get("msg") or {}).get("type") == typ: out.append(o["msg"])
    return out

a1 = login("admin1", "Admin12345"); ot = login("boss", "Boss12345"); gt = login("Guest29", "Guest12345")
g = sql("SELECT id FROM users WHERE nickname='Guest29'")
pcs = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
P7, P8, P9 = pcs["PC-07"], pcs["PC-08"], pcs["PC-09"]
sess_row = lambda sid: sql(f"SELECT status || '|' || COALESCE(ended_reason,'') || '|' || (ready_at IS NOT NULL)::text || '|' || (ready_deadline IS NOT NULL)::text || '|' || (shell_wait_until IS NOT NULL)::text || '|' || billed_minutes FROM sessions WHERE id='{sid}'")
def reset():
    for name in list(agents): agent_stop(name)
    for sid in sql(f"SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='{g}' AND status='active'").split():
        api("POST", f"/admin/sessions/{sid}/end", {}, a1)
    sql(f"UPDATE computers SET status='available' WHERE name IN ('PC-07','PC-08','PC-09')")
    sql(f"UPDATE computers SET mac='AA:BB:CC:DD:EE:08' WHERE name='PC-08'")
    sql(f"UPDATE computers SET mac='AA:BB:CC:DD:EE:07' WHERE name='PC-07'")
    sql(f"UPDATE users SET wallet_grosz=3000, coin_minutes=0 WHERE id='{g}'"); sql(f"DELETE FROM wallet_transactions WHERE user_id='{g}'")
    sql(f"UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND computer_id IN ('{P7['id']}','{P8['id']}','{P9['id']}')")
    sql(f"UPDATE chat_messages SET read_staff=true WHERE user_id='{g}'")
    api("PUT", "/admin/settings", {"ready_wait_min": 7}, ot)
reset()
wallet = lambda: int(sql(f"SELECT wallet_grosz FROM users WHERE id='{g}'"))

# ── A: посадка на ПК без связи — ждём шелл, WoL соседом, деньги не идут ──
agent_start(P9["id"], "pc09")  # живой сосед — есть кому послать WoL
s, d = api("POST", f"/admin/computers/{P8['id']}/session", {"guest": "Guest29", "planned_min": 30}, a1)
check("посадка на PC-08 (шелл офлайн, MAC есть, сосед PC-09 живой) → 201 с shell_wait_until, без ready_deadline, wol=sent",
      s == 201 and d.get("shell_wait_until") and not d.get("ready_deadline") and d.get("wol") == "sent", f"{s} {d.get('shell_wait_until')} {d.get('wol')} {d.get('shell_note')}")
check("пометка стойке: «ПК не на связи — ждём его 7 мин … включаю по сети (WoL)»", "7 мин" in (d.get("shell_note") or "") and "WoL" in (d.get("shell_note") or ""), d.get("shell_note"))
sid = d.get("session_id")
time.sleep(0.6)
wol = agent_msgs("pc09", "wol")
check("сосед PC-09 получил wol с MAC PC-08", len(wol) == 1 and (wol[0].get("payload") or {}).get("mac") == "AA:BB:CC:DD:EE:08", f"{wol}")
row = api("GET", "/admin/computers", None, a1)[1]["computers"]
p8 = next(p for p in row if p["name"] == "PC-08")
check("/admin/computers: у PC-08 сессия с shell_wait_until, stuck нет", (p8.get("session") or {}).get("shell_wait_until") and not p8.get("stuck"), f"{p8.get('session')}")
s, au = api("GET", "/admin/audit?kind=session_start&limit=5", None, a1)
check("журнал: посадка помечена «ПК не на связи, WoL: sent»", any("WoL: sent" in x.get("text", "") for x in au.get("items", [])), f"{[x.get('text') for x in au.get('items', [])][:2]}")
# пауза в ожидании — 409 not_started (как в окне [Готов!])
s, pz = api("POST", "/me/sessions/pause", {}, gt)
check("пауза в ожидании шелла → 409 (not_started; на стенде пауза выключена — pause_disabled раньше по порядку)", s == 409 and pz.get("code") in ("not_started", "pause_disabled"), f"{s} {pz.get('code')}")
# шелл появляется → окно [Готов!] с этого момента, агент получает session_start
agent_start(P8["id"], "pc08")
time.sleep(1.2)
check(f"агент PC-08 подключился → окно [Готов!]: {sess_row(sid)} (active||ready_at нет|deadline есть|wait нет|0)", sess_row(sid) == "active||false|true|false|0")
ss = agent_msgs("pc08", "session_start")
check("агент PC-08 получил session_start после подключения", len(ss) == 1 and (ss[0].get("payload") or {}).get("session_id") == sid, f"{ss}")
started = sql(f"SELECT EXTRACT(EPOCH FROM (now() - started_at))::int FROM sessions WHERE id='{sid}'")
check(f"started_at сдвинут на момент подключения ({started} с назад)", int(started) < 5)
s, sc = api("POST", f"/admin/sessions/{sid}/seat-cancel", {}, a1)
check("отмена посадки в окне [Готов!] → 200, кошелёк цел", s == 200 and wallet() == 3000, f"{s} {wallet()}")
agent_stop("pc08")

# ── B: шелл так и не пришёл — посадка снимается сама, без списания ──
s, d = api("POST", f"/admin/computers/{P8['id']}/session", {"guest": "Guest29"}, a1)
sid = d.get("session_id")
sql(f"UPDATE sessions SET shell_wait_until = now() - interval '1 second' WHERE id='{sid}'")
deadline = time.time() + 45
while time.time() < deadline and sql(f"SELECT status FROM sessions WHERE id='{sid}'") == "active": time.sleep(2)
check(f"срок ожидания вышел без шелла → cancelled|shell_offline, минут 0: {sess_row(sid)}", sess_row(sid).startswith("cancelled|shell_offline|") and sess_row(sid).endswith("|0"))
check("PC-08 снова available, кошелёк цел", sql(f"SELECT status FROM computers WHERE id='{P8['id']}'") == "available" and wallet() == 3000, f"{wallet()}")
s, n = api("GET", "/me/notifications?client=app", None, gt)
sc_n = [x for x in n.get("items", []) if x.get("type") == "seat_cancelled"]
check("гостю: seat_cancelled с reason=shell_offline и именем ПК", bool(sc_n) and sc_n[-1]["payload"].get("reason") == "shell_offline" and sc_n[-1]["payload"].get("computer") == "PC-08", f"{sc_n[-1] if sc_n else n}")

# ── C: варианты WoL и «Завершить» в ожидании ──
sql(f"UPDATE computers SET mac=NULL WHERE id='{P7['id']}'")
s, d = api("POST", f"/admin/computers/{P7['id']}/session", {"guest": "Guest29"}, a1)
check("PC-07 без MAC → 201, wol=no_mac, в пометке «нет MAC»", s == 201 and d.get("wol") == "no_mac" and "нет MAC" in (d.get("shell_note") or ""), f"{s} {d.get('wol')} {d.get('shell_note')}")
s, e = api("POST", f"/admin/sessions/{d['session_id']}/end", {}, a1)
check(f"«Завершить» в ожидании шелла = отмена посадки: {sess_row(d['session_id'])}", s == 200 and sess_row(d["session_id"]).startswith("cancelled|") and wallet() == 3000, f"{s} {e}")
sql(f"UPDATE computers SET mac='AA:BB:CC:DD:EE:07' WHERE id='{P7['id']}'")
agent_stop("pc09")
s, d = api("POST", f"/admin/computers/{P7['id']}/session", {"guest": "Guest29"}, a1)
check("агентов в клубе нет → 201, wol=no_agents", s == 201 and d.get("wol") == "no_agents" and "нет живых агентов" in (d.get("shell_note") or ""), f"{s} {d.get('wol')}")
api("POST", f"/admin/sessions/{d['session_id']}/seat-cancel", {}, a1)
api("PUT", "/admin/settings", {"ready_wait_min": 0}, ot)
s, d = api("POST", f"/admin/computers/{P7['id']}/session", {"guest": "Guest29"}, a1)
check("окно выключено (ready_wait_min=0) — шелл не ждём, окон нет, старт сразу как раньше", s == 201 and not d.get("shell_wait_until") and not d.get("ready_deadline"), f"{s} {d.get('shell_wait_until')} {d.get('ready_deadline')}")
api("POST", f"/admin/sessions/{d['session_id']}/end", {}, a1)
api("PUT", "/admin/settings", {"ready_wait_min": 7}, ot)
sql(f"UPDATE users SET wallet_grosz=3000 WHERE id='{g}'"); sql(f"DELETE FROM wallet_transactions WHERE user_id='{g}'")

# ── D: «Освободить ПК» ──
sql(f"UPDATE computers SET status='in_session' WHERE id='{P7['id']}'")
row = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
check("/admin/computers: PC-07 in_session без сессии → stuck=true", row["PC-07"].get("stuck") is True and not row["PC-07"].get("session"), f"{row['PC-07'].get('stuck')}")
s, r = api("POST", f"/admin/computers/{P7['id']}/release", {}, a1)
check("release → 200, was=in_session, статус available", s == 200 and r.get("was") == "in_session" and sql(f"SELECT status FROM computers WHERE id='{P7['id']}'") == "available", f"{s} {r}")
s, r2 = api("POST", f"/admin/computers/{P7['id']}/release", {}, a1)
check("повтор → 409 not_stuck", s == 409 and r2.get("code") == "not_stuck", f"{s} {r2.get('code')}")
sql(f"UPDATE computers SET status='reserved' WHERE id='{P7['id']}'")
row = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
s, r3 = api("POST", f"/admin/computers/{P7['id']}/release", {}, a1)
check("reserved без сессии: stuck=true и release → 200", row["PC-07"].get("stuck") is True and s == 200 and r3.get("was") == "reserved", f"{s} {r3}")
agent_start(P9["id"], "pc09b")
s, d = api("POST", f"/admin/computers/{P9['id']}/session", {"guest": "Guest29"}, a1)
s, r4 = api("POST", f"/admin/computers/{P9['id']}/release", {}, a1)
check("release ПК с живой сессией → 409 has_session", s == 409 and r4.get("code") == "has_session", f"{s} {r4.get('code')}")
s, au = api("GET", "/admin/audit?kind=pc_release&limit=5", None, a1)
check("журнал: pc_release записан", any(x.get("kind") == "pc_release" and "PC-07" in x.get("text", "") for x in au.get("items", [])), f"{[x.get('text') for x in au.get('items', [])][:2]}")

# ── E: вызов за гостем (Guest29 сидит за PC-09) ──
sql(f"UPDATE chat_messages SET created_at = created_at - interval '2 minutes' WHERE user_id='{g}'")  # кулдаун вызова 30 с
s, cm = api("POST", "/me/chat", {"kind": "call"}, gt)
check("гость зовёт админа → 201", s in (200, 201), f"{s} {cm}")
s, pend = api("GET", "/admin/chat/pending", None, a1)
call = next((x for x in pend.get("items", []) if x.get("kind") == "call" and x.get("user_id") == g), None)
check("pending: у вызова current_computer_id = PC-09 (где гость сидит)", call and call.get("current_computer_id") == P9["id"] and call.get("current_computer") == "PC-09", f"{call}")
s, ack = api("POST", f"/admin/chat/{call['id']}/ack", {}, a1)
s, n = api("GET", "/me/notifications?client=app", None, gt)
check("«Принял» → гостю уведомление admin_coming", s == 200 and any(x.get("type") == "admin_coming" for x in n.get("items", [])), f"{[x.get('type') for x in n.get('items', [])]}")
api("POST", f"/admin/sessions/{d['session_id']}/end", {}, a1)
sql(f"UPDATE chat_messages SET created_at = created_at - interval '2 minutes' WHERE user_id='{g}'")
s, cm = api("POST", "/me/chat", {"kind": "call"}, gt)
s, pend = api("GET", "/admin/chat/pending", None, a1)
call = next((x for x in pend.get("items", []) if x.get("kind") == "call" and x.get("user_id") == g), None)
check("гость без сессии: вызов в колоколе, current_computer_id нет (рамки не будет)", call and not call.get("current_computer_id"), f"{call}")
api("POST", f"/admin/chat/{call['id']}/ack", {}, a1)

# ── F: «на сколько» — planned_min против чужой брони ──
start = (dt.datetime.now(dt.timezone.utc) + dt.timedelta(minutes=60)).replace(microsecond=0).isoformat().replace("+00:00", "Z")
s, bk = api("POST", "/admin/bookings", {"guest": "Guest28", "computer_id": P9["id"], "start_time": start, "duration_min": 60}, a1)
s, d = api("POST", f"/admin/computers/{P9['id']}/session", {"guest": "Guest29", "planned_min": 180}, a1)
check("посадка на 180 мин при чужой брони через 60 → 409 booking_soon", s == 409 and d.get("code") == "booking_soon", f"{s} {d.get('code')} {d.get('message')}")
s, d = api("POST", f"/admin/computers/{P9['id']}/session", {"guest": "Guest29", "planned_min": 30}, a1)
check("посадка на 30 мин → 201", s == 201, f"{s} {d.get('code')} {d.get('message')}")
if s == 201: api("POST", f"/admin/sessions/{d['session_id']}/end", {}, a1)
if bk.get("booking_id"): api("POST", f"/admin/bookings/{bk['booking_id']}/cancel", {}, a1)

reset()
print(f"\nитого: {ok} ok, {fail} fail")
sys.exit(1 if fail else 0)
