#!/usr/bin/env python3
"""З22-и4 «Напоминания о сменах» (№52, З-Р28). Тем же прогоном, что периодические алерты (POST /admin/alerts/check, фоном —
раз в 15 минут): за shift_remind_hours часов до назначенной смены — в канал, видят все: «📅 z22s, твоя смена «z22s Тест»
сегодня в ЧЧ:ММ (через …) — до ЧЧ:ММ», раз на назначение; кто уже на смене — не нужно; с часа shift_nobody_hour клуба —
владельцу: «🗓 Завтра (ДД.ММ, дн) на «z22n Пусто» … никто не назначен», раз на день и шаблон; 0 и «Не присылать» (−1) —
выключено. Фикстуры — временный админ z22s и шаблоны «z22s Тест», «z22n Пусто» (удаляются в начале и в конце вместе с
назначениями и записями табеля z22s); пороги — как были; напоминания, ушедшие в канал за время проверки (и про смены
стенда), в конце удаляются — журнал о них остаётся, фон их не повторит."""
from _z import *

NICK, TPL, NOB = "z22s", "z22s Тест", "z22n Пусто"


def wipe():
    for name in (TPL, NOB):
        for sid in rows(f"SELECT id FROM shifts WHERE name = '{name}'"):
            sql(f"DELETE FROM alerts_sent WHERE kind IN ('shift_soon', 'shift_nobody') AND (key LIKE '%/{sid}' OR key IN "
                f"(SELECT 'a/' || id FROM shift_assignments WHERE shift_id = '{sid}'))")
            sql(f"DELETE FROM shift_assignments WHERE shift_id = '{sid}'")
            sql(f"DELETE FROM shifts WHERE id = '{sid}'")
    sql(f"DELETE FROM work_entries WHERE user_id = (SELECT id FROM users WHERE nickname = '{NICK}')")
    sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND (text LIKE '📅 {NICK},%' OR text LIKE '🗓 Завтра%«{NOB}»%')")


wipe()
T0 = sql("SELECT now()")


def wipe_all():
    wipe()
    sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND created_at >= '{T0}' AND (text LIKE '📅 %' OR text LIKE '🗓 Завтра%')")


at_end(wipe_all)
hire_temp(NICK)
set_settings(shift_remind_hours=2, shift_nobody_hour=0, alerts_on=1)
check_now = lambda: api("POST", "/admin/alerts/check", {}, "boss")
soon = lambda: rows(f"SELECT visible_to || '|' || text FROM staff_messages WHERE kind = 'system' AND text LIKE '📅 {NICK},%'")
nobody = lambda: rows(f"SELECT visible_to || '|' || text FROM staff_messages WHERE kind = 'system' AND text LIKE '🗓 Завтра%«{NOB}»%'")

section("напоминание о смене")
# смена через полтора часа по часам клуба, на два часа; дата назначения — день её начала
st, dt, hm = sql(f"SELECT (extract(hour FROM t) * 60 + extract(minute FROM t))::int || ' ' || t::date || ' ' || to_char(t, 'HH24:MI') "
                 f"FROM (SELECT date_trunc('minute', now() AT TIME ZONE '{TZ}') + interval '90 minutes' AS t) x").split()
st = int(st)
s, tpl = api("POST", "/admin/shifts", {"name": TPL, "start_min": st, "end_min": (st + 120) % 1440, "force": True}, "boss")
s2, asg = api("POST", f"/admin/shifts/{tpl.get('id')}/assign", {"date": dt, "nickname": NICK}, "boss")
check("шаблон «z22s Тест» через 90 минут и назначение z22s на него", s == 201 and s2 in (200, 201), J([s, tpl, s2, asg]))
s3, r3 = check_now()
m = soon()
check(f"за 2 часа до смены — в канал, видят все: «📅 z22s, твоя смена «z22s Тест» … в {hm} (через …) — до …»",
      s3 == 200 and len(m) == 1 and m[0].startswith(f"all|📅 {NICK}, твоя смена «{TPL}» ") and f" в {hm} (через " in m[0],
      J([r3, m]))
check_now()
check("второй прогон — повтора нет (раз на назначение)", len(soon()) == 1, J(soon()))

tok(NICK)  # вход в админку открывает его смену — он уже на работе (сначала условие, потом ключ: фон шлёт то же)
sql(f"DELETE FROM alerts_sent WHERE kind = 'shift_soon' AND key = 'a/{(asg.get('assignment') or asg).get('id', '')}'")
sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND text LIKE '📅 {NICK},%'")
check_now()
check("z22s уже отметил приход — не напоминаем", soon() == [], J(soon()))
set_settings(shift_remind_hours=1)
sql(f"DELETE FROM work_entries WHERE user_id = (SELECT id FROM users WHERE nickname = '{NICK}')")
check_now()
check("порог 1 час, а до смены полтора — рано", soon() == [], J(soon()))
set_settings(shift_remind_hours=0)
check_now()
check("порог 0 — напоминания выключены", soon() == [], J(soon()))

section("завтра никто не назначен")
s4, nob = api("POST", "/admin/shifts", {"name": NOB, "start_min": 600, "end_min": 720, "force": True}, "boss")
r5 = check_now()[1]
n = nobody()
tm = sql(f"SELECT to_char(((now() AT TIME ZONE '{TZ}') - interval '{settings().get('report_hour', 0)} hours')::date + 1, 'DD.MM')")
check(f"«z22n Пусто» работает завтра, назначений нет — владельцу: «🗓 Завтра ({tm}, …) на «z22n Пусто» 10:00–12:00 никто не назначен»",
      s4 == 201 and len(n) == 1 and n[0].startswith(f"owner|🗓 Завтра ({tm}, ") and f"на «{NOB}» 10:00–12:00 никто не назначен" in n[0],
      J([r5, n]))
check_now()
check("второй прогон — повтора нет (раз на день и шаблон)", len(nobody()) == 1, J(nobody()))
day2 = sql(f"SELECT (((now() AT TIME ZONE '{TZ}') - interval '{settings().get('report_hour', 0)} hours')::date + 1)::text")
api("POST", f"/admin/shifts/{nob.get('id')}/assign", {"date": day2, "nickname": NICK}, "boss")
sql(f"DELETE FROM alerts_sent WHERE kind = 'shift_nobody' AND key LIKE '%/{nob.get('id')}'")
sql(f"DELETE FROM staff_messages WHERE kind = 'system' AND text LIKE '🗓 Завтра%«{NOB}»%'")
check_now()
check("назначили z22s на завтра — молчит", nobody() == [], J(nobody()))
set_settings(shift_nobody_hour=-1)
sql(f"DELETE FROM shift_assignments WHERE shift_id = '{nob.get('id')}'")
check_now()
check("«Не присылать» (−1) — молчит", nobody() == [], J(nobody()))
put = lambda kv: [api("PUT", "/admin/settings", {k: v}, "boss")[0] for k, v in kv]
edge = put((("shift_remind_hours", 24), ("shift_nobody_hour", 23), ("shift_nobody_hour", -1)))
bad = put((("shift_remind_hours", 25), ("shift_nobody_hour", -2), ("shift_nobody_hour", 24)))
check("границы: 24 ч, 23 и −1 — сохраняются, 25 ч, −2 и 24 — 400", edge == [200, 200, 200] and bad == [400, 400, 400],
      J([edge, bad]))
done()
