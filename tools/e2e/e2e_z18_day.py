#!/usr/bin/env python3
"""З18-и4б «Сутки с полуночи» (решение основателя 07.11, З-Р38): большинство клубов работают 24/7 — сутки клуба по
умолчанию с полуночи, 00:00–24:00, как календарь; свой час границы — опция «Экономики». Раньше по умолчанию была 8
(Б10-и1): на клубе без своей настройки пополнение в 00:30 ложилось во вчерашние сутки — в отчёт за вчера, мимо «Выручки
сегодня» и журнала админа. Здесь: без строки report_hour — 0, сводка и сутки смены с 00:00; пополнения в 00:00:01 и
вчера в 23:59:59 — каждое в своих календарных сутках (KPI, журнал админа, «Деньги»), в дни перевода часов — тоже (сутки
25 и 23 ч); свой час 8 — сутки с 08:00, как раньше; граница 24 — 400; миграция 093: сохранённая 8 → 0, свой час и
«строки нет» не трогает, журнал не пишет. Гость Guest48 (пополнения отменяются в конце), настройки — как были."""
import os, subprocess

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "migrations", "093_report_hour_midnight.sql")
G = "Guest48"
TODAY, YDAY = sql(f"SELECT (now() AT TIME ZONE '{TZ}')::date::text"), sql(f"SELECT ((now() AT TIME ZONE '{TZ}')::date - 1)::text")
local = lambda expr: sql(f"SELECT to_char(({expr}) AT TIME ZONE '{TZ}', 'YYYY-MM-DD\"T\"HH24:MI:SS')")  # момент по часам клуба
deps = []


def dep(pln, at, who="admin1"):
    """Пополнение Guest48 картой от who, перенесённое на момент at (SQL timestamptz); в конце отменяется владельцем. Картой —
    не наличными: отмена пополнения, перенесённого в прошлое, вынула бы деньги из ящика стойки, куда они не входили."""
    d = topup(G, pln, method="card", who=who)
    deps.append(d)
    sql(f"UPDATE deposits SET created_at = {at} WHERE id = '{d}'")
    return d


at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in deps])
kpi = lambda: (api("GET", "/admin/overview", None, "admin1")[1].get("revenue_today") or {}).get("deposits_pln", -1)
day_dep = lambda d: (api("GET", f"/admin/reports/money?from={d}&to={d}", None, "boss")[1].get("totals") or {}).get("deposits_pln", -1)


def in_journal(d):
    """Видит ли admin1 пополнение в своём журнале: страница сразу после него — от него не зависит загрузка стенда."""
    cur = sql(f"SELECT to_char((created_at + interval '1 second') AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM deposits WHERE id = '{d}'")
    s, au = api("GET", f"/admin/audit?kind=deposit&limit=200&before={cur}", None, "admin1")
    return f"deposit:{d}" in [x.get("id") for x in au.get("items", [])], au.get("day")


section("клуб без своей настройки — сутки с полуночи")
set_settings(report_hour=0)  # запомнить строку как была (вернётся в конце)
sql("DELETE FROM settings WHERE key = 'report_hour'")
check("строки report_hour нет → в «Экономике» граница 0 — с полуночи", settings().get("report_hour") == 0, J(settings().get("report_hour")))
s, ov = api("GET", "/admin/overview", None, "admin1")
check(f"сводка: report_hour 0, сутки — с сегодняшних 00:00 ({TODAY})", s == 200 and ov.get("report_hour") == 0
      and str(ov.get("day_from", ""))[:19] == f"{TODAY}T00:00:00", f"{ov.get('report_hour')} {ov.get('day_from')}")
s, sh = api("GET", "/admin/stats/shift", None, "boss")
check(f"сводка смены — сегодняшняя календарная дата {TODAY}, с 0:00", s == 200 and sh.get("date") == TODAY and sh.get("report_hour") == 0,
      f"{sh.get('date')} {sh.get('report_hour')}")

section("пополнения у полуночи — в своих календарных сутках")
k0, y0 = kpi(), day_dep(YDAY)
a = dep(7, f"(date_trunc('day', now() AT TIME ZONE '{TZ}') + interval '1 second') AT TIME ZONE '{TZ}'")  # сегодня 00:00:01
k1 = kpi()
check(f"7 zł в {TODAY} 00:00:01 — в «Выручке сегодня»: пополнения {k0} → {k1}", near(k1, k0 + 7), f"{k0} → {k1}")
seen, day = in_journal(a)
check(f"admin1 видит его в своём журнале, сутки журнала — {TODAY}", seen and day == TODAY, f"{seen} {day}")
b = dep(3, f"(date_trunc('day', now() AT TIME ZONE '{TZ}') - interval '1 second') AT TIME ZONE '{TZ}'")  # вчера 23:59:59
k2, y1 = kpi(), day_dep(YDAY)
check(f"3 zł вчера в 23:59:59 — мимо «Выручки сегодня» ({k1} → {k2}), в «Деньгах» за {YDAY} ({y0} → {y1})",
      near(k2, k1) and near(y1, y0 + 3), f"KPI {k1} → {k2}, вчера {y0} → {y1}")
seen, day = in_journal(b)
check("в журнале admin1 его нет — это вчерашние сутки", not seen and day == TODAY, f"{seen} {day}")
s, m = api("GET", "/admin/reports/money?preset=today", None, "boss")
check("KPI = «Деньги» за сегодня (пресет «сегодня» — те же сутки)", s == 200 and near(kpi(), (m.get("totals") or {}).get("deposits_pln", -1)),
      f"{kpi()} vs {J(m.get('totals'))}")

section("дни перевода часов — сутки по стенным часам: 25 и 23 часа")
for when, hhmm, other, txt in (("2025-10-26", "23:30", "2025-10-27", "26.10.2025 23:30 (сутки 25 ч: 24 часа от полуночи — уже 23:00)"),
                               ("2026-03-30", "00:30", "2026-03-29", "30.03.2026 00:30 (прошлые сутки — 23 ч: 24 часа от их полуночи — 01:00)")):
    want = when
    w0, o0 = day_dep(want), day_dep(other)
    dep(11, at_club(when, hhmm), who="boss")
    w1, o1 = day_dep(want), day_dep(other)
    check(f"11 zł {txt} — в сутках {want}: {w0} → {w1}, {other} не тронуты", near(w1, w0 + 11) and near(o1, o0),
          f"{want} {w0} → {w1}, {other} {o0} → {o1}")

section("свой час границы — сутки с 08:00, как раньше")
s, d = set_settings(report_hour=8)
start8 = sql(f"SELECT to_char(((now() AT TIME ZONE '{TZ}') - interval '8 hours')::date + time '08:00', 'YYYY-MM-DD\"T\"HH24:MI:SS')")
s, ov = api("GET", "/admin/overview", None, "admin1")
check(f"report_hour 8 → сводка: сутки с {start8}", s == 200 and ov.get("report_hour") == 8 and str(ov.get("day_from", ""))[:19] == start8,
      f"{ov.get('report_hour')} {ov.get('day_from')}")
mine = local(f"(date_trunc('day', now() AT TIME ZONE '{TZ}') + interval '1 second') AT TIME ZONE '{TZ}'") >= start8  # 00:00:01 в текущих?
seen, day = in_journal(a)
check(f"пополнение в 00:00:01 — {'в текущих сутках (они начались вчера в 08:00)' if mine else 'во вчерашних сутках (до 08:00)'}: "
      f"журнал admin1 {'видит' if mine else 'не видит'}", seen == mine, f"{seen} {day}")
s, d = api("PUT", "/admin/settings", {"report_hour": 24}, "boss")
check("граница 24 — 400 bad_value («00:00–24:00» — это 0)", s == 400 and d.get("code") == "bad_value", f"{s} {J(d)}")
api("PUT", "/admin/settings", {"report_hour": 0}, "boss")

section("миграция 093: сохранённая 8 → 0")
have = os.path.exists(MIG)
check("миграция 093 есть", have, MIG)


def migrate(before):
    """Строка report_hour = before (None — строки нет) → прогон 093 → что стало (None — строки нет)."""
    sql("DELETE FROM settings WHERE key = 'report_hour'")
    if before is not None:
        sql(f"INSERT INTO settings (key, value) VALUES ('report_hour', '{before}')")
    if have:
        r = subprocess.run(["psql", "-v", "ON_ERROR_STOP=1", DB, "-q", "-f", MIG], capture_output=True, text=True)
        if r.returncode:
            return "ошибка: " + r.stderr.strip()
    v = rows("SELECT value FROM settings WHERE key = 'report_hour'")
    return v[0] if v else None


j0 = int(sql("SELECT count(*) FROM admin_actions WHERE action = 'settings_update'"))
got = {b: migrate(b) for b in ("8", "6", None, "0")}
check("8 → 0 (стенд, пилотный клуб); свой час 6 — 6; строки нет — нет; 0 — 0", got == {"8": "0", "6": "6", None: None, "0": "0"}, J(got))
again = migrate("8"), migrate(migrate("8"))
check("повторный прогон ничего не меняет, журнал «Экономики» миграция не пишет", again == ("0", "0")
      and int(sql("SELECT count(*) FROM admin_actions WHERE action = 'settings_update'")) == j0, J(again))
done()
