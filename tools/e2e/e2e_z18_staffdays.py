#!/usr/bin/env python3
"""З18-и4е «Даты «Персонала» по клубным суткам» (из «Найдено по дороге» З18-и4д; решение основателя 08.11): «сегодня»
«Персонала» — клубные сутки, как у увольнения (З14-и2). Своя граница суток — на час-два позже «сейчас» (как ночью до 08:00
у клуба с границей 8: клубные сутки ещё вчерашние). Наём без даты — «работает с» клубных суток, а не с даты календаря,
период работы — с них же; уволить его той же ночью без даты — 200, последний день — те же сутки (было 400 «Дата увольнения
раньше даты найма»); график без from — с клубных суток. В 23:xx своей границей сутки не сдвинуть (граница 24 — это 0):
проверки идут на сутках, равных календарю; в последние полторы минуты 22:xx проверка ждёт 23:00.
Временный z18h нанимается каждый прогон (в админку не входит) и в конце удаляется SQL вместе со строками журнала о нём
(карточка и периоды работы — каскадом); граница суток — как была."""
import time

from _z import *

NICK = "z18h"
H, LEFT = (int(x) for x in sql(f"SELECT extract(hour FROM now() AT TIME ZONE '{TZ}')::int || ' ' || "
                                "(3600 - extract(epoch FROM now() - date_trunc('hour', now())))::int").split())
if H == 22 and LEFT < 90:  # граница 23 вот-вот наступит — ждём её: проверка не должна застать смену суток
    time.sleep(LEFT + 2)
    H = 23
RH = H + 2 if H <= 21 else 23 if H == 22 else None  # граница позже «сейчас» — сутки ещё вчерашние; в 23:xx — не сдвинуть


def drop():
    """Временный z18h — SQL-ом: строки журнала о нём, потом аккаунт (карточка и периоды работы — каскадом)."""
    u = uid(NICK)
    if u:
        sql(f"DELETE FROM admin_actions WHERE target_user_id = '{u}'")
        sql(f"DELETE FROM users WHERE id = '{u}'")


drop()  # хвост упавшего прогона
at_end(drop)
if RH is not None:
    set_settings(report_hour=RH)
CD, CAL = club_day(0), sql(f"SELECT (now() AT TIME ZONE '{TZ}')::date::text")
periods = lambda u: rows(f"SELECT hired_on || '|' || COALESCE(dismissed_on::text, '-') FROM staff_periods WHERE user_id = '{u}' ORDER BY hired_on")

section(f"своя граница {RH:02d}:00, сейчас {H:02d}:xx — клубные сутки {CD}, календарь {CAL}" if RH is not None
        else f"{H:02d}:xx — своей границей сутки не сдвинуть: клубные сутки {CD} = календарь")
check("клубные сутки — не дата календаря" if RH is not None else "сутки = календарь (23:xx)", RH is None or CD != CAL, f"{CD} {CAL}")
s, d = api("POST", "/admin/staff/hire", {"nickname": NICK, "password": STAFF_PASS}, "boss")
u = uid(NICK)
prof = sql(f"SELECT hired_at FROM staff_profiles WHERE user_id = '{u}'") if u else ""
check(f"наём {NICK} без даты → 201: работает с {CD} — клубные сутки" + (f", а не дата календаря {CAL}" if RH is not None else "")
      + "; период работы — с них же", s == 201 and prof == CD and periods(u) == [f"{CD}|-"], J([s, d, prof, periods(u) if u else None]))
s, d = dismiss(NICK)
check(f"уволить его той же ночью без даты → 200, последний день {CD}, период закрыт им же (было 400 «Дата увольнения раньше "
      f"даты найма»)", s == 200 and d.get("dismissed_at") == CD and periods(u) == [f"{CD}|{CD}"], J([s, d, periods(u) if u else None]))
s, sc = api("GET", "/admin/shifts/schedule?days=1", None, "boss")
check(f"график без from — с клубных суток {CD}", s == 200 and sc.get("from") == CD and [x.get("date") for x in sc.get("days", [])] == [CD],
      J([s, sc.get("from"), sc.get("days")]))
done()
