#!/usr/bin/env python3
"""З15-и1 «Смена по назначению, пересечения шаблонов» (№22; З-Р16). Смену записи табеля брал шаблон, который шёл в минуту
входа: назначенный на «День 08–20», вошедший в 07:50, попадал в хвост «Ночи» вчерашнего дня, а в сводке смены — в «вне
графика»; не назначенный, вошедший в шаблонное время, в расхождениях не появлялся; пересекающиеся шаблоны («Вечер 15–23»
при «День 08–20») создавались молча, и табель брал первый по порядку. Теперь при входе — сначала назначение человека
(его смена идёт сейчас или начнётся в ближайшие три часа), потом шаблон по времени; «по графику» в сводке — назначен на
этот день, «вне графика» — работал без назначения; пересекающийся шаблон — 409 overlap с перечнем, «всё равно» — force.
«07:50 к смене 08:00» моделируется своим шаблоном, который начнётся через 20 минут (сдвинуть часы стенда нельзя), он
пересекается с «Днём» и «Ночью» стенда — поэтому создаётся с force. Временные сотрудники z15a (назначен) и z15b (без
назначения) уходят проверкой, шаблоны проверки удаляются; денег не трогает."""
from _z import *

A, B = "z15a", "z15b"
for nick in (A, B):
    hire_temp(nick)
made = []  # шаблоны проверки — в конце удаляются: назначения уходят каскадом, у записей табеля смена обнуляется
at_end(lambda: [api("DELETE", f"/admin/shifts/{i}", None, "boss") for i in made])
tpl_names = lambda: rows("SELECT name FROM shifts WHERE name LIKE 'z15 %'")
entry = lambda nick: sql(f"SELECT w.date::text || '|' || COALESCE(s.name, '') FROM work_entries w LEFT JOIN shifts s ON s.id = w.shift_id "
                         f"WHERE w.user_id = '{uid(nick)}' AND w.ended_at IS NULL")
summary = lambda day: api("GET", f"/admin/stats/shift?date={day}", None, "boss")[1]
row_of = lambda sm, nick: next((r for r in sm.get("worked", []) if r.get("nickname") == nick and r.get("open")), {})  # запись этого входа
for i in rows("SELECT id FROM shifts WHERE name LIKE 'z15 %'"):  # хвосты упавшего прогона
    api("DELETE", f"/admin/shifts/{i}", None, "boss")

section("пересечение шаблонов — предупреждение и «всё равно»")
s, d = api("POST", "/admin/shifts", {"name": "z15 Вечер", "start_min": 900, "end_min": 1380}, "boss")
check("«Вечер 15–23» при «День 08–20» — 409 overlap «Пересекается со сменами «День» 08:00–20:00, …», в ответе — с какими; шаблона нет",
      s == 409 and d.get("code") == "overlap" and d.get("message", "").startswith("Пересекается со сменами «День» 08:00–20:00")
      and "День" in [x.get("name") for x in d.get("with", [])] and "z15 Вечер" not in tpl_names(), f"{s} {J(d, 400)}")
seen = {x.get("id") for x in audit(limit=40)}
s, ev = api("POST", "/admin/shifts", {"name": "z15 Вечер", "start_min": 900, "end_min": 1380, "force": True}, "boss")
if ev.get("id"):
    made.append(ev["id"])
au = next((x for x in audit(limit=40) if x.get("kind") == "shift_tpl_create" and x.get("id") not in seen), {})
check("«всё равно создать» (force) — 201; в журнале: «z15 Вечер · 15:00–23:00 · пересекается со сменами «День» 08:00–20:00, …»",
      s == 201 and au.get("text", "").startswith("z15 Вечер · 15:00–23:00 · пересекается со сменами «День» 08:00–20:00"), f"{s} {J(au)}")
s, d = api("PATCH", f"/admin/shifts/{ev.get('id')}", {"start_min": 600, "end_min": 720}, "boss")
s2, d2 = api("PATCH", f"/admin/shifts/{ev.get('id')}", {"active": False}, "boss")
check("правка в 10–12 — тоже 409 overlap; выключить смену — 200: выключенной пересекаться не с чем",
      s == 409 and d.get("code") == "overlap" and s2 == 200 and d2.get("active") is False, f"{s} {J(d, 200)} · {s2} {J(d2, 200)}")

section("смена при входе — по назначению")
start = sql(f"SELECT to_char(now() AT TIME ZONE '{TZ}' + interval '20 minutes', 'YYYY-MM-DD HH24:MI')")
day, hm = start.split()
smin = int(hm[:2]) * 60 + int(hm[3:])
s, soon = api("POST", "/admin/shifts", {"name": "z15 скоро", "start_min": smin, "end_min": (smin + 120) % 1440, "force": True}, "boss")
if soon.get("id"):
    made.append(soon["id"])
sa = api("POST", f"/admin/shifts/{soon.get('id')}/assign", {"date": day, "nickname": A}, "boss")[0]
check(f"данные: шаблон «z15 скоро» начнётся через 20 минут ({hm}), z15a назначен на него {day}, z15b — без назначения",
      s == 201 and sa in (200, 201), f"{s} {J(soon, 200)} · {sa}")
tok(A)  # вход в админку открывает смену
tok(B)
ea, eb = entry(A), entry(B)
check("z15a вошёл за 20 минут до своей смены — запись в его смене («z15 скоро», день её начала), а не в шаблоне, что идёт сейчас",
      ea == f"{day}|z15 скоро", ea)
check("z15b без назначения — как раньше, шаблон по времени", eb.split("|")[1] in ("День", "Ночь") if eb else False, eb)
sa_, sb_ = summary(ea.split("|")[0]), summary(eb.split("|")[0]) if eb else {}
ra, rb = row_of(sa_, A), row_of(sb_, B)
offa, offb = (sa_.get("mismatch") or {}).get("off_schedule", []), (sb_.get("mismatch") or {}).get("off_schedule", [])
check("сводка смены: z15a — «по графику», в «вне графика» его нет", ra.get("in_schedule") is True and A not in offa, f"{J(ra, 200)} · {offa}")
check("z15b — «вне графика»: работал без назначения, хоть шаблон при входе и был", rb.get("in_schedule") is False and B in offb, f"{J(rb, 200)} · {offb}")
done()
