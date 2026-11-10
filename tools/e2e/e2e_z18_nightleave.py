#!/usr/bin/env python3
"""З18-и4з «Уход посреди ночной смены» (из «Найдено по дороге» З18-и4е; решение основателя 08.11): «посреди смены» — смена
идёт: запись табеля этих клубных суток или по графику она ещё не кончилась. Раньше — только совпадение даты записи с
клубными сутками, и с сутками с полуночи ночная смена после 00:00 (запись — днём её начала) была «давней»: шторка
увольнения и карточка писали «уход не отмечен», «закроет по графику», владельцу в канал — «смена … осталась незакрытой».
Здесь временные z18n и z18m входят в админку — открывается смена по графику стенда («День» 08–20 или «Ночь» 20–08),
датой её начала. Ночью (до 08:00) при сутках с полуночи это уже вчерашняя дата; днём сутки делаются вчерашними своей
границей на два часа позже «сейчас» (смена, начатая раньше своего часа границы, — тот же случай). Предпросмотр — ongoing
(same_day — то же, для открытых вкладок прошлой версии), закроет сейчас; «Уволить» z18n и «Снять права» z18m закрывают
смену сейчас, владельцу — «посреди смены». В 23:xx дату смены с сутками не развести (граница 24 — это 0): проверки идут
на смене этих суток; в последние полторы минуты часа проверка ждёт следующий — смена по графику не кончится посреди неё.
Временные z18n и z18m уходят проверкой; их записи табеля удаляются в начале и в конце; граница суток — как была."""
import time

from _z import *

N, M = "z18n", "z18m"
H, LEFT = (int(x) for x in sql(f"SELECT extract(hour FROM now() AT TIME ZONE '{TZ}')::int || ' ' || "
                                "(3600 - extract(epoch FROM now() - date_trunc('hour', now())))::int").split())
if LEFT < 90:  # час вот-вот сменится — ждём его: ни граница суток, ни конец смены по графику не наступят посреди проверки
    time.sleep(LEFT + 2)
    H = (H + 1) % 24
RH = 0 if H < 8 else H + 2 if H <= 21 else 23 if H == 22 else None  # ночью — сутки с полуночи; днём — граница позже «сейчас»
ENTRIES = f"DELETE FROM work_entries WHERE user_id IN (SELECT id FROM users WHERE nickname IN ('{N}', '{M}'))"
sql(ENTRIES)  # хвост прошлого прогона
at_end(lambda: sql(ENTRIES))  # в конце — уже после увольнения (уборка идёт в обратном порядке)
for nick in (N, M):
    hire_temp(nick)
if RH is not None:
    set_settings(report_hour=RH)
chat = lambda: api("GET", "/admin/staff-chat", None, "boss")[1].get("messages", [])
owner_lines = lambda seen: [m["text"] for m in chat() if m["id"] not in seen and m.get("kind") == "system" and m.get("visible_to") == "owner"]
preview = lambda nick: api("GET", f"/admin/staff/{uid(nick)}/shift", None, "boss")


def opened(nick):
    """Вход в админку открывает смену; → id записи, её дата, смена и «с какого момента» — как у сервера (leaveSince)."""
    tok(nick)
    r = sql(f"SELECT w.id || '|' || w.date || '|' || COALESCE(w.shift_name, '') || '|' || CASE WHEN (w.started_at AT TIME ZONE '{TZ}')::date "
            f"= (now() AT TIME ZONE '{TZ}')::date THEN to_char(w.started_at AT TIME ZONE '{TZ}', 'HH24:MI') ELSE to_char(w.started_at "
            f"AT TIME ZONE '{TZ}', 'DD.MM HH24:MI') END FROM work_entries w WHERE w.user_id = '{uid(nick)}' AND w.ended_at IS NULL")
    return r.split("|") if r else ["", "", "", ""]


CD = club_day(0)
en, dn, shn, sn = opened(N)
em, dm_, shm, sm = opened(M)
section(f"смена z18n — «{shn}» с {sn}, дата записи {dn}; клубные сутки {CD}" + (f" (граница {RH:02d}:00)" if RH else " (с полуночи)"))
check("данные: у z18n и z18m открыты смены по графику; дата записи — не сегодняшние клубные сутки" if RH is None or H < 23
      else "данные: у z18n и z18m открыты смены по графику", bool(en) and bool(em) and shn != "" and shm != ""
      and (H == 23 or (dn != CD and dm_ != CD)), J([en, dn, shn, em, dm_, shm, CD, H, RH]))

s, pv = preview(N)
w = pv.get("open_shift") or {}
check("шторка увольнения: смена идёт — ongoing (и same_day — то же для открытых вкладок прошлой версии), закроет сейчас: "
      "0–5 минут", s == 200 and w.get("id") == en and w.get("ongoing") is True and w.get("same_day") is True
      and 0 <= (w.get("close_minutes") if w.get("close_minutes") is not None else -1) <= 5, J([s, w], 500))
seen = {m["id"] for m in chat()}
t0 = sql("SELECT now()::text")
s, d = api("POST", f"/admin/staff/{uid(N)}/dismiss", {"reason": "z18 проверка"}, "boss")
row = sql(f"SELECT ended_at >= '{t0}'::timestamptz, auto_closed, note FROM work_entries WHERE id = '{en}'")
check("«Уволить» — 200: смена закрыта сейчас (не по графику), «закрыта увольнением»",
      s == 200 and (d.get("closed_shift") or {}).get("id") == en and row == "t|t|закрыта увольнением", J([s, row]))
lines = owner_lines(seen)
check(f"владельцу в канал — «{N} уволен посреди смены (с {sn}) — осталось за него: касса не сдана …; часы — смена закрыта "
      "увольнением, … в табеле на проверку»", len(lines) == 1 and lines[0].startswith(f"⚠ {N} уволен посреди смены (с {sn}) — осталось за него: касса не сдана")
      and "; часы — смена закрыта увольнением, " in lines[0] and lines[0].endswith(" в табеле на проверку"), J(lines))

s, pv = preview(M)
w = pv.get("open_shift") or {}
seen = {m["id"] for m in chat()}
t0 = sql("SELECT now()::text")
s2, d2 = api("DELETE", f"/admin/staff/{uid(M)}", None, "boss")
row = sql(f"SELECT ended_at >= '{t0}'::timestamptz, auto_closed, note FROM work_entries WHERE id = '{em}'")
lines = owner_lines(seen)
check(f"«Снять права» z18m — так же: в карточке смена идёт, закрыта сейчас «закрыта снятием прав»; в канал — «{M} снят с прав "
      f"посреди смены (с {sm})»", s == 200 and w.get("ongoing") is True and s2 == 200 and row == "t|t|закрыта снятием прав"
      and len(lines) == 1 and lines[0].startswith(f"⚠ {M} снят с прав посреди смены (с {sm}) — осталось за него: касса не сдана"),
      J([w.get("ongoing"), s2, row, lines]))
done()
