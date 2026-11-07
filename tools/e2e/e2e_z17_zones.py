#!/usr/bin/env python3
"""З17-и1 «Зоны и ПК» (№7, З-Р6), «Отчёты → Загрузка». Занятость зоны считалась от числа ПК, на которых за период были
сессии: 6 часов на VIP-03 за сутки — 25 % при трёх ПК в зоне (честно — 8,3 %); ПК без сессий в таблице не было, ПК в
ремонте входили в ёмкость. Теперь зона — от всех её ПК, ПК и зоны без сессий — с нулями, ПК в ремонте сейчас — с
пометкой и не в ёмкости (и у итога), сессия — в зоне своего снимка (З16), без снимка — в зоне своего ПК. Пустые
клубные сутки в прошлом: сессии Guest47 (SQL, без денег, ended_reason z17) — 6 ч на VIP-03, 2 ч на PC-07, 1 ч на PC-08
по снимку зоны VIP и 30 минут на PC-01 без снимка; VIP-02 на время — в ремонте (API). В конце сессии удаляются, VIP-02
— снова свободен (и при падении тоже)."""
import math

from _z import *

G = "Guest47"
P = pcs()
STD, VIP = sql("SELECT id FROM zones WHERE name = 'Standard'"), sql("SELECT id FROM zones WHERE name = 'VIP'")
NSTD, NVIP = (int(sql(f"SELECT count(*) FROM computers WHERE zone_id = '{z}'")) for z in (STD, VIP))
NALL = int(sql("SELECT count(*) FROM computers"))
drop = lambda: sql(f"DELETE FROM sessions WHERE ended_reason = 'z17' AND user_id = '{uid(G)}'")
drop()
at_end(drop)
repair = lambda st: api("PATCH", f"/admin/computers/{P['VIP-02']['id']}/status", {"status": st}, "admin1")[0]
at_end(lambda: repair("available"))
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key = 'report_hour'), '0')"))
win = (f"started_at >= ((g + interval '{rh} hours') AT TIME ZONE '{TZ}') "
       f"AND started_at < ((g + interval '1 day {rh} hours') AT TIME ZONE '{TZ}')")
EMPTY = rows(f"""SELECT g::date::text FROM generate_series(((now() AT TIME ZONE '{TZ}')::date - 400)::timestamp,
    ((now() AT TIME ZONE '{TZ}')::date - 3)::timestamp, interval '1 day') AS g
    WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE {win}) ORDER BY g DESC LIMIT 2""")
D, D0 = (EMPTY + ["", ""])[:2]  # сутки под сессии проверки и ещё одни пустые


def ins(pc, frm, to, mins, zone=None):
    """Завершённая сессия Guest47 за D без денег; zone — (id, имя) снимка, None — без снимка, как до 091."""
    snap = (f"'{zone[0]}', '{zone[1]}', 10, 2" if zone else "NULL, '', NULL, NULL")
    return sql(f"""INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ready_at, ended_at, minutes_total,
        minutes_used, billed_minutes, base_rate_pln, effective_rate_pln, ended_reason, zone_id, zone_name, xp_per_min,
        coins_per_min) SELECT '{uid(G)}', id, club_id, 'completed', {at_club(D, frm)}, {at_club(D, frm)}, {at_club(D, to)},
        {mins}, {mins}, {mins}, 0, 0, 'z17', {snap} FROM computers WHERE name = '{pc}' RETURNING id""")


def load(day):
    s, d = api("GET", f"/admin/reports/load?from={day}&to={day}", None, "boss")
    return s, d, {z.get("zone"): z for z in d.get("zones") or []}, {c.get("name"): c for c in d.get("computers") or []}


pct = lambda minutes, capacity: math.floor(minutes / capacity * 1000 + 0.5) / 10  # как math.Round сервера
zone = lambda zs, name: tuple(zs.get(name, {}).get(k) for k in ("computers", "repair", "hours", "sessions", "occupancy_pct"))
pc = lambda cs, name: tuple(cs.get(name, {}).get(k) for k in ("hours", "sessions", "occupancy_pct"))

section("пустые сутки")
s, d, zs, cs = load(D0)
OM = (d.get("capacity") or {}).get("open_minutes")
check(f"пустые сутки {D0}: обе зоны — со всеми своими ПК (Standard {NSTD}, VIP {NVIP}) и нулями, в «Компьютерах» — все "
      f"{NALL} ПК с нулями; сутки клуба — {OM} минут (смены «День» и «Ночь» — круглые сутки)",
      s == 200 and bool(D0) and OM == 1440 and zone(zs, "Standard") == (NSTD, 0, 0, 0, 0) and zone(zs, "VIP") == (NVIP, 0, 0, 0, 0)
      and len(cs) == NALL and all(pc(cs, n) == (0, 0, 0) for n in cs), f"{s} {D0} {OM} · {J(d.get('zones'))} · {len(cs)} ПК")

section("6 часов на VIP-03")
ins("VIP-03", "10:00", "16:00", 360, (VIP, "VIP"))
ins("PC-07", "18:00", "20:00", 120, (STD, "Standard"))
s, d, zs, cs = load(D)
cap = d.get("capacity") or {}
check(f"{D}: 6 ч на VIP-03 — у зоны VIP все {NVIP} ПК и занятость {pct(360, NVIP * 1440)} % (раньше — 25 %); Standard — "
      f"{NSTD} ПК, 2 ч, {pct(120, NSTD * 1440)} %".replace(".", ","),
      s == 200 and zone(zs, "VIP") == (NVIP, 0, 6, 1, pct(360, NVIP * 1440))
      and zone(zs, "Standard") == (NSTD, 0, 2, 1, pct(120, NSTD * 1440)), f"{s} · {J(d.get('zones'))}")
others = [n for n in cs if n not in ("VIP-03", "PC-07")]
check(f"«Компьютеры»: все {NALL} ПК — VIP-03 6 ч 25 %, PC-07 2 ч 8,3 %, остальные {NALL - 2} — нули; итог — 8 ч, "
      f"{pct(480, NALL * 1440)} % от {NALL} ПК, в ремонте 0".replace(".", ","),
      len(cs) == NALL and pc(cs, "VIP-03") == (6, 1, 25) and pc(cs, "PC-07") == (2, 1, pct(120, 1440))
      and all(pc(cs, n) == (0, 0, 0) for n in others) and d.get("totals", {}).get("hours") == 8
      and d.get("totals", {}).get("occupancy_pct") == pct(480, NALL * 1440)
      and (cap.get("computers"), cap.get("total"), cap.get("repair")) == (NALL, NALL, 0),
      f"{len(cs)} ПК · {J(d.get('computers'), 200)} · итог {J(d.get('totals'))} · {J(cap)}")

section("ПК в ремонте — не в ёмкости")
s1 = repair("maintenance")
s, d, zs, cs = load(D)
cap = d.get("capacity") or {}
check(f"VIP-02 в ремонте: у VIP «{NVIP} ПК · в ремонте 1» и {pct(360, (NVIP - 1) * 1440)} % (6 ч на {NVIP - 1} ПК); у VIP-02 "
      f"— пометка; итог — от {NALL - 1} ПК: {pct(480, (NALL - 1) * 1440)} %".replace(".", ","),
      s1 == 200 and zone(zs, "VIP") == (NVIP, 1, 6, 1, pct(360, (NVIP - 1) * 1440)) and cs.get("VIP-02", {}).get("repair") is True
      and not any(c.get("repair") for n, c in cs.items() if n != "VIP-02")
      and (cap.get("computers"), cap.get("total"), cap.get("repair")) == (NALL - 1, NALL, 1)
      and d.get("totals", {}).get("occupancy_pct") == pct(480, (NALL - 1) * 1440),
      f"{s1} · VIP {zone(zs, 'VIP')} · VIP-02 {J(cs.get('VIP-02'))} · {J(cap)} · {J(d.get('totals'))}")
s2 = repair("available")
s, d, zs, cs = load(D)
check(f"VIP-02 снова свободен — у VIP снова {pct(360, NVIP * 1440)} %, пометки нет".replace(".", ","),
      s2 == 200 and zone(zs, "VIP") == (NVIP, 0, 6, 1, pct(360, NVIP * 1440)) and not cs.get("VIP-02", {}).get("repair"),
      f"{s2} · VIP {zone(zs, 'VIP')} · VIP-02 {J(cs.get('VIP-02'))}")

section("сессия — в зоне своего снимка")
ins("PC-08", "21:00", "22:00", 60, (VIP, "VIP"))  # PC-08 в Standard, сессия шла по тарифу VIP (пересадка или смена зоны ПК)
ins("PC-01", "23:00", "23:30", 30)                 # без снимка — как вставленные до 091 и в старых проверках
s, d, zs, cs = load(D)
check(f"час на PC-08 по снимку VIP — в зоне VIP (7 ч, {pct(420, NVIP * 1440)} %), полчаса на PC-01 без снимка — в зоне "
      f"своего ПК, Standard (2,5 ч); в «Компьютерах» — у самих машин: PC-08 1 ч, PC-01 0,5 ч".replace(".", ",", 1),
      zone(zs, "VIP") == (NVIP, 0, 7, 2, pct(420, NVIP * 1440)) and zone(zs, "Standard") == (NSTD, 0, 2.5, 2, pct(150, NSTD * 1440))
      and pc(cs, "PC-08") == (1, 1, pct(60, 1440)) and pc(cs, "PC-01") == (0.5, 1, pct(30, 1440)),
      f"VIP {zone(zs, 'VIP')} · Standard {zone(zs, 'Standard')} · PC-08 {pc(cs, 'PC-08')} · PC-01 {pc(cs, 'PC-01')}")
done()
