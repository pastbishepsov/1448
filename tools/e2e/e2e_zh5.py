#!/usr/bin/env python3
"""Ж5 «Шторки знают друг о друге» (№15, 18, 19, 23, 43): деньги гостя в /admin/computers и
карточке тем же прогнозом, что видит гость; список гостей — role=player, фильтры и сортировка
на сервере; резолвер гостя в очереди и продаже. Живой e2e против стенда с моком агента.
Идемпотентен: сессии Guest29/Guest28 снимает, очередь чистит, продажи отменяет, ПК возвращает."""
import os, json, subprocess, sys, time, os, urllib.request, urllib.error, datetime as dt

from _env import API, BASE, HOST, PORT, DB, SRV_LOG, MOCK_AGENT, agent_log  # стенд задаётся окружением, см. README
def sql(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr + "\n" + q)
    return r.stdout.strip().splitlines()[0] if r.stdout.strip() else ""
def sql_rows(q):
    r = subprocess.run(["psql", DB, "-tAc", q], capture_output=True, text=True)
    if r.returncode: raise SystemExit("SQL: " + r.stderr + "\n" + q)
    return [l for l in r.stdout.strip().splitlines() if l]
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
def ts(s): return dt.datetime.fromisoformat(s.replace("Z", "+00:00")) if s else None
def now(): return dt.datetime.now(dt.timezone.utc)

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

a1 = login("admin1", "Admin12345"); ot = login("boss", "Boss12345"); gt = login("Guest29", "Guest12345")
g29 = sql("SELECT id FROM users WHERE nickname='Guest29'"); g28 = sql("SELECT id FROM users WHERE nickname='Guest28'"); g27 = sql("SELECT id FROM users WHERE nickname='Guest27'")
pcs = {p["name"]: p for p in api("GET", "/admin/computers", None, a1)[1]["computers"]}
P9 = pcs["PC-09"]
STD = sql("SELECT id FROM zones WHERE name='Standard'")
maint_before = sql_rows("SELECT id FROM computers WHERE status='maintenance'")

def reset():
    for name in list(agents): agent_stop(name)
    for uid in (g29, g28, g27):
        for sid in sql(f"SELECT COALESCE(string_agg(id::text, ' '), '') FROM sessions WHERE user_id='{uid}' AND status='active'").split():
            api("POST", f"/admin/sessions/{sid}/end", {}, a1)
    sql("UPDATE computers SET status='available' WHERE status IN ('maintenance','in_session','reserved') AND id NOT IN (SELECT computer_id FROM sessions WHERE status='active')"
        + (" AND id NOT IN (" + ",".join(f"'{i}'" for i in maint_before) + ")" if maint_before else ""))
    # кошельки правим напрямую — и чистим их журнал: инвариант «сумма журнала = баланс» (e2e_deposit_void) не должен ломаться от тестовых данных
    sql(f"UPDATE users SET wallet_grosz=3000, coin_minutes=0, coins_balance=5000, created_at=now() - interval '1 day' WHERE id='{g29}'")
    sql(f"UPDATE users SET wallet_grosz=0, created_at=now() - interval '10 days', registered_at=now() - interval '400 days' WHERE id='{g28}'")  # З5-и1: без визитов «новизна» — по регистрации
    sql(f"DELETE FROM wallet_transactions WHERE user_id IN ('{g29}','{g28}')")  # Guest27 (e2e_deposit_void) не трогаем
    sql(f"DELETE FROM minute_grants WHERE user_id='{g29}'")
    sql(f"UPDATE waitlist SET status='removed', resolved_at=now() WHERE status='waiting' AND user_id IN ('{g29}','{g28}','{g27}')")
    sql(f"UPDATE bookings SET status='cancelled' WHERE status IN ('pending','confirmed') AND computer_id='{P9['id']}'")
reset()

def sess_of(pcs_resp, name):
    for c in pcs_resp["computers"]:
        if c["name"] == name: return c.get("session")
    return None
def client_left():
    """Прогноз, как его считает шелл (renderWallet): монеты + зонные минуты здесь + пакет + floor(кошелёк*60/ставка)."""
    me = api("GET", "/me", None, gt)[1]
    ss = api("GET", "/me/sessions", None, gt)[1]
    act = next(s for s in ss["sessions"] if s["status"] == "active")
    rate_gr = round(act["effective_rate_pln"] * 100)
    return (me.get("coin_minutes") or 0) + (ss.get("zone_minutes") or 0) + (ss.get("pack_minutes") or 0) + (me["wallet_grosz"] * 60) // rate_gr

# ── A: и1 — деньги гостя в /admin/computers ──
agent_start(P9["id"], "pc09")
s, d = api("POST", f"/admin/computers/{P9['id']}/session", {"guest_id": g29, "planned_min": 60}, a1)
check("посадка Guest29 за PC-09 (шелл online) → 201, окно [Готов!]", s == 201 and d.get("ready_deadline") and not d.get("shell_wait_until"), f"{s} {d.get('code')}")
sid = d.get("session_id") or d.get("id")
pc = api("GET", "/admin/computers", None, a1)[1]
ss = sess_of(pc, "PC-09") or {}
check("session: user_id, rate_pln 20, wallet_grosz 3000, zone/pack 0, minutes_left 90 (30 zł по 20 zł/ч)",
      ss.get("user_id") == g29 and ss.get("rate_pln") == 20 and ss.get("wallet_grosz") == 3000 and ss.get("zone_minutes") == 0
      and ss.get("pack_minutes") == 0 and ss.get("minutes_left") == 90, json.dumps({k: ss.get(k) for k in ("user_id","rate_pln","wallet_grosz","zone_minutes","pack_minutes","minutes_left")}))
check("до [Готов!] «until» не обещаем (деньги не идут)", "until" not in ss, str(ss.get("until")))
card = api("GET", f"/admin/users/{g29}", None, a1)[1]
asx = card.get("active_session") or {}
check("карточка: active_session {id, computer PC-09, computer_id, waiting=true, minutes_left 90, started_at}",
      asx.get("id") and asx.get("computer") == "PC-09" and asx.get("computer_id") == P9["id"] and asx.get("waiting") is True
      and asx.get("minutes_left") == 90 and asx.get("started_at") and "until" not in asx, json.dumps(asx, default=str)[:300])

# гость нажал [Готов!] — время пошло
s, d = api("POST", "/me/sessions/ready", {}, gt)
check("гость нажал [Готов!] → 200", s == 200, str(s))
time.sleep(1.0)
pc = api("GET", "/admin/computers", None, a1)[1]; ss = sess_of(pc, "PC-09") or {}
t0 = now()
check("после [Готов!]: until есть, minutes_left 90", ss.get("until") and ss.get("minutes_left") == 90, json.dumps({k: ss.get(k) for k in ("until","minutes_left")}))
until_srv = ts(ss.get("until"))
till_client = t0 + dt.timedelta(minutes=client_left())
diff = abs((until_srv - till_client).total_seconds()) if until_srv else 9999
check("«хватит до» сервера = прогноз шелла (монеты+зона+пакет+кошелёк/ставка) ±60 с", diff <= 60, f"расхождение {diff:.0f} с")

# зонные минуты сдвигают прогноз ровно на выданное
s, d = api("POST", f"/admin/users/{g29}/redeem", {"zone_id": STD, "minutes": 40}, a1)
check("выдали 40 мин Standard за монеты → 201", s in (200, 201), f"{s} {d.get('code')} {d.get('message')}")
pc2 = api("GET", "/admin/computers", None, a1)[1]; ss2 = sess_of(pc2, "PC-09") or {}
check("session: zone_minutes 40, minutes_left 130 (90 + 40)", ss2.get("zone_minutes") == 40 and ss2.get("minutes_left") == 130, json.dumps({k: ss2.get(k) for k in ("zone_minutes","minutes_left")}))
shift = (ts(ss2["until"]) - until_srv).total_seconds() if ss2.get("until") else -1
check("until сдвинулся на +40 мин (±60 с)", abs(shift - 2400) <= 60, f"сдвиг {shift:.0f} с")
card = api("GET", f"/admin/users/{g29}", None, a1)[1]; asx = card.get("active_session") or {}
d_card = abs((ts(asx["until"]) - ts(ss2["until"])).total_seconds()) if asx.get("until") else 9999
check("карточка: waiting=false, minutes_left 130, until = until шторки ПК (±3 с)", asx.get("waiting") is False and asx.get("minutes_left") == 130 and d_card <= 3, f"{asx.get('minutes_left')} Δ{d_card:.1f} с")
lst = api("GET", "/admin/users?filter=at_pc", None, a1)[1]
na = (lst.get("now_at") or {}).get(g29) or {}
check("список filter=at_pc: Guest29 в списке, now_at → PC-09, minutes_left 130, until как у шторки (±3 с)",
      any(u["id"] == g29 for u in lst["users"]) and na.get("computer") == "PC-09" and na.get("minutes_left") == 130
      and na.get("until") and abs((ts(na["until"]) - ts(ss2["until"])).total_seconds()) <= 3, json.dumps(na, default=str))
check("filter=at_pc: у каждого в списке есть now_at", all(u["id"] in (lst.get("now_at") or {}) for u in lst["users"]), str(lst["count"]))

# чужая бронь режет прогноз: booking_deadline = start − booking_lock_min
start = (now() + dt.timedelta(minutes=40)).replace(microsecond=0)
s, bk = api("POST", "/admin/bookings", {"guest_id": g28, "computer_id": P9["id"], "start_time": start.isoformat().replace("+00:00", "Z"), "duration_min": 60}, a1)
if s == 201:
    pc3 = api("GET", "/admin/computers", None, a1)[1]; ss3 = sess_of(pc3, "PC-09") or {}
    lock = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='booking_lock_min'), '10')") or 10)
    bd = ts(ss3.get("booking_deadline")) if ss3.get("booking_deadline") else None
    check("booking_deadline в session = start − booking_lock_min", bd is not None and abs((bd - (start - dt.timedelta(minutes=lock))).total_seconds()) <= 2, f"{ss3.get('booking_deadline')} lock={lock}")
    api("POST", f"/admin/bookings/{bk.get('booking_id') or bk.get('id')}/cancel", {}, a1)
else:
    check("walk-in бронь Guest28 на PC-09 через 40 мин (для booking_deadline)", False, f"{s} {bk.get('code')} {bk.get('message')}")

# пауза: время стоит — until не обещаем
s, d = api("POST", "/me/sessions/pause", {}, gt)
if s == 200:
    pc4 = api("GET", "/admin/computers", None, a1)[1]; ss4 = sess_of(pc4, "PC-09") or {}
    check("на паузе: paused_at есть, until нет, minutes_left 130", ss4.get("paused_at") and "until" not in ss4 and ss4.get("minutes_left") == 130, json.dumps({k: ss4.get(k) for k in ("paused_at","until","minutes_left")}))
    api("POST", "/me/sessions/resume", {}, gt)
else:
    check("пауза на стенде выключена (pause_disabled) — ветку паузы пропускаем", d.get("code") == "pause_disabled", f"{s} {d.get('code')}")

# ── B: и2 — завершение из карточки тем же POST /admin/sessions/:id/end ──
s, d = api("POST", f"/admin/sessions/{asx['id']}/end", {}, a1)
check("завершение сессии по id из карточки → 200", s == 200, f"{s} {d.get('code')}")
card = api("GET", f"/admin/users/{g29}", None, a1)[1]
check("карточка: active_session пуст", not (card.get("active_session") or {}).get("id"), json.dumps(card.get("active_session")))
pc5 = api("GET", "/admin/computers", None, a1)[1]
check("PC-09 без сессии", sess_of(pc5, "PC-09") is None, "")
agent_stop("pc09")

# ── C: и3 — список гостей ──
lst = api("GET", "/admin/users", None, a1)[1]
check("по умолчанию только гости: ни admin, ни owner; role=player в ответе", all(u["role"] == "player" for u in lst["users"]) and lst.get("role") == "player" and lst["count"] > 0, str(lst["count"]))
s, lst_all = api("GET", "/admin/users?role=all", None, ot)
s_a, d_a = api("GET", "/admin/users?role=all", None, a1)
check("role=all — персонал тоже, но только владельцу (служебно); админу — 403 owner_only (З15-и3)",
      s == 200 and any(u["role"] in ("admin", "owner") for u in lst_all.get("users", [])) and s_a == 403 and d_a.get("code") == "owner_only",
      f"{s} {s_a} {d_a.get('code')}")
s, d = api("GET", "/admin/users?role=hacker", None, a1); check("role вне списка → 400 bad_role", s == 400 and d.get("code") == "bad_role", f"{s} {d.get('code')}")
s, d = api("GET", "/admin/users?filter=bogus", None, a1); check("filter вне списка → 400 bad_filter", s == 400 and d.get("code") == "bad_filter", f"{s} {d.get('code')}")
s, d = api("GET", "/admin/users?sort=password_hash", None, a1); check("sort вне списка → 400 bad_sort", s == 400 and d.get("code") == "bad_sort", f"{s} {d.get('code')}")
s, d = api("GET", "/admin/users?sort=wallet&dir=sideways", None, a1); check("dir вне списка → 400 bad_sort", s == 400 and d.get("code") == "bad_sort", f"{s} {d.get('code')}")
m = api("GET", "/admin/users?filter=money", None, a1)[1]
ids = {u["id"] for u in m["users"]}
check("filter=money: все с деньгами, Guest29 (30 zł) есть, Guest28 (0) нет", all(u["wallet_grosz"] > 0 for u in m["users"]) and g29 in ids and g28 not in ids, f"{m['count']}")
n = api("GET", "/admin/users?filter=new", None, a1)[1]
ids = {u["id"] for u in n["users"]}
nd = n.get("new_days", 14)  # З5-и1 (З-Р10): новые — первое появление (первая сессия, без сессий — регистрация) не раньше seg_new_days назад
FIRST = "COALESCE((SELECT MIN(s.started_at) FROM sessions s WHERE s.user_id = users.id AND s.status <> 'cancelled'), users.registered_at)"
# начало окна — как newSince на сервере: N суток по часам клуба (AddDate), а не N×24 ч сессии psql в UTC — иначе две недели
# после перевода часов границы расходятся на час, и гость с первым появлением в этот час «новый» только для сервера (З16-и1г)
SINCE = f"(((now() AT TIME ZONE 'Europe/Warsaw') - interval '{nd} days') AT TIME ZONE 'Europe/Warsaw')"
stale = sql(f"SELECT count(*) FROM users WHERE id IN ({','.join(repr(i) for i in ids) or 'NULL'}) AND {FIRST} < {SINCE}")
# Не от календаря: у Guest29 первое появление — его первая сессия на стенде (с каждым днём старше), поэтому «новый» он или нет —
# по правилу; заведомо новый и заведомо давний — Guest28 без визитов: регистрация вчера и 400 дней назад (поиском — мимо лимита в 100 строк)
new_ids = lambda q: {u["id"] for u in api("GET", f"/admin/users?filter=new&q={q}", None, a1)[1]["users"]}
visits28 = sql(f"SELECT count(*) FROM sessions WHERE user_id = '{g28}' AND status <> 'cancelled'")
sql(f"UPDATE users SET registered_at = now() - interval '1 day' WHERE id = '{g28}'")
fresh28 = g28 in new_ids("Guest28")
sql(f"UPDATE users SET registered_at = now() - interval '400 days' WHERE id = '{g28}'")  # как в reset()
old28 = g28 in new_ids("Guest28")
want29 = sql(f"SELECT {FIRST} >= {SINCE} FROM users WHERE id = '{g29}'") == "t"
in29 = g29 in new_ids("Guest29")
check(f"filter=new: гость без визитов (Guest28) с регистрацией вчера — новый, 400 дней назад — нет; Guest29 — по своему первому появлению; "
      f"у всех в списке первое появление не раньше {nd} дней",
      visits28 == "0" and fresh28 and not old28 and in29 == want29 and stale == "0" and set(n.get("first_seen", {})) == ids,
      f"{n['count']} · давних {stale} · Guest28 вчера/давно: {fresh28}/{old28} · Guest29 {in29} (ждали {want29}) · визитов Guest28 {visits28}")
w = api("GET", "/admin/users?sort=wallet", None, a1)[1]
vals = [u["wallet_grosz"] for u in w["users"]]
check("sort=wallet: по убыванию (dir по умолчанию desc)", vals == sorted(vals, reverse=True) and w.get("dir") == "desc", str(vals[:5]))
w = api("GET", "/admin/users?sort=wallet&dir=asc", None, a1)[1]
vals = [u["wallet_grosz"] for u in w["users"]]
check("sort=wallet&dir=asc: по возрастанию", vals == sorted(vals), str(vals[:5]))
nk = api("GET", "/admin/users?sort=nickname", None, a1)[1]
names = [u["nickname"].lower() for u in nk["users"]]
check("sort=nickname: по алфавиту (dir по умолчанию asc)", names == sorted(names) and nk.get("dir") == "asc", str(names[:4]))
lv = api("GET", "/admin/users?sort=level&dir=desc", None, a1)[1]
lvls = [u["level"] for u in lv["users"]]
check("sort=level&dir=desc: по убыванию", lvls == sorted(lvls, reverse=True), str(lvls[:6]))
cr = api("GET", "/admin/users?sort=created&filter=new", None, a1)[1]
crs = [ts(u["created_at"]) for u in cr["users"]]
check("sort=created + filter=new: новые сверху", crs == sorted(crs, reverse=True), str(len(crs)))
q = api("GET", "/admin/users?q=Guest2&filter=money&sort=wallet", None, a1)[1]
check("q + filter + sort вместе: все Guest2x с деньгами по убыванию", q["count"] > 0 and all(u["nickname"].startswith("Guest2") and u["wallet_grosz"] > 0 for u in q["users"])
      and [u["wallet_grosz"] for u in q["users"]] == sorted([u["wallet_grosz"] for u in q["users"]], reverse=True) and "matched" in q, f"{q['count']}")
check("без сессий now_at не отдаётся", "now_at" not in api("GET", "/admin/users?q=Guest27", None, a1)[1], "")

# ── D: и4 — резолвер в очереди ──
free_ids = sql_rows("SELECT id FROM computers WHERE status='available'")
sql("UPDATE computers SET status='maintenance' WHERE status='available'")  # очередь принимает только когда свободных нет
try:
    s, d = api("POST", "/admin/waitlist", {"guest": "+48 500 120 028"}, a1)
    check("очередь по телефону (с пробелами) → 201, nickname Guest28", s == 201 and d.get("nickname") == "Guest28", f"{s} {d.get('code')} {d.get('message')}")
    wl28 = d.get("id")
    s, d = api("POST", "/admin/waitlist", {"guest": "Guest2"}, a1)
    cands = d.get("candidates") or []
    check("очередь «Guest2» → 409 guest_ambiguous с кандидатами (≤5, у каждого id и nickname)", s == 409 and d.get("code") == "guest_ambiguous" and 2 <= len(cands) <= 5 and all(c.get("id") and c.get("nickname") for c in cands), f"{s} {d.get('code')} {len(cands)}")
    s, d = api("POST", "/admin/waitlist", {"guest_id": g27}, a1)
    check("очередь по guest_id (выбор кандидата) → 201 Guest27", s == 201 and d.get("nickname") == "Guest27", f"{s} {d.get('code')}")
    wl27 = d.get("id")
    s, d = api("POST", "/admin/waitlist", {"nickname": "guest27"}, a1)
    check("строгий nickname остаётся строгим: «guest27» → 404", s == 404, f"{s} {d.get('code')}")
    s, d = api("POST", "/admin/waitlist", {"guest": "admin2"}, a1)
    check("персонал резолвер не находит: «admin2» → 404", s == 404, f"{s} {d.get('code')}")
    s, d = api("POST", "/admin/waitlist", {}, a1)
    check("пустое тело → 400 guest_empty", s == 400 and d.get("code") == "guest_empty", f"{s} {d.get('code')}")
    s, d = api("POST", "/admin/waitlist", {"guest": "+48500120028"}, a1)
    check("повтор Guest28 → 409 already_waiting (резолвер нашёл, правило очереди сработало)", s == 409 and d.get("code") == "already_waiting", f"{s} {d.get('code')}")
    wl = api("GET", "/admin/waitlist", None, a1)[1]
    check("в очереди Guest28 и Guest27, у записей user_id", {w["nickname"] for w in wl["items"]} >= {"Guest28", "Guest27"} and all(w.get("user_id") for w in wl["items"]), str([w["nickname"] for w in wl["items"]]))
    for wid in (wl28, wl27):
        if wid: api("DELETE", f"/admin/waitlist/{wid}", None, a1)
finally:
    if free_ids: sql("UPDATE computers SET status='available' WHERE id IN (" + ",".join(f"'{i}'" for i in free_ids) + ")")

# ── E: и4 — резолвер в продаже ──
goods = api("GET", "/admin/goods", None, a1)[1]["goods"]
good = next(x for x in goods if x["active"] and x["stock"] >= 3)
stock0 = good["stock"]
sales = []
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "cash", "guest": "+48500120028"}, a1)
check("продажа «кому» по телефону → 201, sale.nickname Guest28, sale.user_id", s == 201 and d.get("sale", {}).get("nickname") == "Guest28" and d.get("sale", {}).get("user_id") == g28, f"{s} {d.get('code')}")
if s == 201: sales.append(d["sale"]["id"])
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "cash", "guest": "Guest2"}, a1)
check("продажа «Guest2» → 409 guest_ambiguous, склад не тронут", s == 409 and d.get("code") == "guest_ambiguous" and int(sql(f"SELECT stock FROM goods WHERE id='{good['id']}'")) == stock0 - 1, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "card", "guest_id": g27}, a1)
check("продажа по guest_id → 201 Guest27", s == 201 and d.get("sale", {}).get("nickname") == "Guest27", f"{s} {d.get('code')}")
if s == 201: sales.append(d["sale"]["id"])
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "cash", "nickname": "guest28"}, a1)
check("строгий nickname «guest28» → 404 (регистр)", s == 404, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "cash", "nickname": "Guest28"}, a1)
check("строгий nickname «Guest28» → 201", s == 201 and d.get("sale", {}).get("nickname") == "Guest28", f"{s} {d.get('code')}")
if s == 201: sales.append(d["sale"]["id"])
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "cash", "guest": "admin2"}, a1)
check("продажа на персонал → 404", s == 404, f"{s} {d.get('code')}")
s, d = api("POST", "/admin/sales", {"good_id": good["id"], "qty": 1, "method": "cash"}, a1)
check("продажа без гостя — как раньше, 201", s == 201 and "nickname" not in d.get("sale", {}), f"{s} {d.get('code')}")
if s == 201: sales.append(d["sale"]["id"])
for sid_ in sales: api("POST", f"/admin/sales/{sid_}/void", {}, a1)
check("уборка: продажи отменены, склад вернулся", int(sql(f"SELECT stock FROM goods WHERE id='{good['id']}'")) == stock0, "")

reset()
print(f"\nИтого: ok={ok} fail={fail}")
sys.exit(1 if fail else 0)
