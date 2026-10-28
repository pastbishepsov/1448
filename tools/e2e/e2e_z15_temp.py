#!/usr/bin/env python3
"""З15-и6б «Смена при входе временным паролем» (из «Найдено по дороге» З15-и6, решение основателя 27.10). Вход в админку
временным паролем (после найма или сброса владельцем) сразу открывал смену, а «Выйти» с экрана «Задай свой пароль»
оставлял её открытой — у человека, который в админку так и не попал. Теперь вход временным паролем смену не открывает:
её открывает свой пароль, заданный в админке (/me/password с client «admin», как у входа), — с этой минуты и строкой
журнала «вход в админку». Смена пароля не из админки (киоск, PWA — без client) смену не открывает, обычная смена
пароля второй записи не заводит. Временный z15x — сотрудник: пароль и флаг временного пароля в конце — как у временных
сотрудников (SQL); его записи табеля удаляются в начале и в конце; денег не трогает."""
from _z import *

Z, OWN, OWN2 = "z15x", "Z15x-own-1", "Z15x-own-2"
zid = hire_temp(Z)
drop = lambda: sql(f"DELETE FROM work_entries WHERE user_id = '{zid}'")
reset = lambda: sql(f"UPDATE users SET password_hash = (SELECT password_hash FROM users WHERE nickname = 'admin1'), "
                    f"must_change_password = false WHERE id = '{zid}'")
temp = lambda: sql(f"UPDATE users SET password_hash = (SELECT password_hash FROM users WHERE nickname = 'admin1'), "
                   f"must_change_password = true WHERE id = '{zid}'")  # временный пароль — STAFF_PASS
drop()
at_end(drop)
at_end(reset)
now = lambda: sql("SELECT now()::text")
open_since = lambda t: sql(f"SELECT count(*) FROM work_entries WHERE user_id = '{zid}' AND ended_at IS NULL AND started_at >= '{t}'")
open_all = lambda: sql(f"SELECT count(*) FROM work_entries WHERE user_id = '{zid}' AND ended_at IS NULL")
notes = lambda: rows(f"SELECT note FROM work_entries WHERE user_id = '{zid}' AND ended_at IS NULL")
starts = lambda t: rows(f"SELECT details FROM admin_actions WHERE admin_id = '{zid}' AND action = 'work_start' AND created_at >= '{t}'")
chg = lambda cur, new, token, **extra: api("POST", "/me/password", {"current_password": cur, "new_password": new, **extra},
                                           token=token)

section("вход в админку временным паролем")
temp()
t0 = now()
s, lg = login(Z, STAFF_PASS, client="admin")
tk = (lg or {}).get("access_token", "")
check("вход временным паролем — 200, в ответе «задай свой»; смена не открылась, строки «вход в админку» нет",
      s == 200 and (lg.get("user") or {}).get("must_change_password") is True and open_all() == "0" and starts(t0) == [],
      f"{s} открытых: {open_all()} · {J(starts(t0))}")

section("свой пароль в админке — первый вход в неё")
t1 = now()
s2, d2 = chg(STAFF_PASS, OWN, tk, client="admin")
ok2 = (s2 == 200 and (d2.get("user") or {}).get("must_change_password") is False and open_all() == "1" and open_since(t1) == "1"
       and notes() == ["открыта входом в админку"] and len(starts(t0)) == 1 and starts(t0)[0].startswith("вход в админку"))
check("свой пароль из админки (client «admin») — 200: смена открылась с этой минуты, «открыта входом в админку», строка "
      "журнала «вход в админку» одна", ok2, f"{s2} открытых: {open_all()}, с минуты смены: {open_since(t1)} · {J(notes())} · "
      f"{J(starts(t0))}")
s3, lg3 = login(Z, OWN, client="admin")
s4, _ = chg(OWN, OWN2, (lg3 or {}).get("access_token", ""), client="admin")
check("следующий вход своим паролем в те же сутки и обычная смена пароля из админки — та же смена, второй записи и строки "
      "«вход в админку» нет", ok2 and (s3, s4) == (200, 200) and open_all() == "1" and len(starts(t0)) == 1,
      f"{s3} {s4} открытых: {open_all()} · {J(starts(t0))}")

section("временный пароль сменили не в админке")
drop()
temp()
t5 = now()
s5, lg5 = login(Z, STAFF_PASS)  # вход киоска или PWA — без client
s6, d6 = chg(STAFF_PASS, OWN, (lg5 or {}).get("access_token", ""))
check("вход и смена временного пароля без client (киоск, PWA) — 200, флага больше нет, смены нет (а из админки — "
      "открывается)", ok2 and (s5, s6) == (200, 200) and (d6.get("user") or {}).get("must_change_password") is False
      and open_all() == "0" and starts(t5) == [], f"{s5} {s6} открытых: {open_all()} · {J(starts(t5))}")
done()
