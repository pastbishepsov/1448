#!/usr/bin/env python3
"""З15-и6 «Мелочи персонала — экран» (№30, З-Р20): «Сегодня отработано» после ночной смены. Запись табеля датирована
днём начала смены: ночная 20:00–08:00 записана вчерашним днём, и утром после ухода «Моя смена» показывала «Сегодня
отработано 0 ч 00 м». Теперь в «Сегодня отработано» (/admin/work/me, minutes_today) — записи этих клубных суток и те,
что в них закончились. Проверка готовит записи табеля временного z15o SQL-ом: вчерашняя дневная и позавчерашняя ночная
(закончились до начала сегодняшних суток — не считаются), вчерашняя ночная (закончилась сегодня — считается, в том
числе ровно в начале суток, как закрывает забытый уход; за минуту до — уже вчерашняя), сегодняшняя короткая —
считается. Вход z15o — без признака админки: такой вход смену не открывает. Его записи табеля удаляются в начале и в
конце; денег не трогает."""
from _z import *

Z = "z15o"
zid = hire_temp(Z)
club = sql("SELECT id FROM clubs ORDER BY created_at LIMIT 1")
drop = lambda: sql(f"DELETE FROM work_entries WHERE user_id = '{zid}'")
drop()
at_end(drop)
RH = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')"))
D0, D1, D2 = club_day(0), club_day(-1), club_day(-2)
T0 = at_club(D0, f"{RH:02d}:00")  # начало сегодняшних клубных суток


def entry(day, start, end, note):
    """Закрытая запись табеля z15o, открытая им самим; минуты — по времени. Возвращает (id, минуты)."""
    r = sql(f"INSERT INTO work_entries (club_id, user_id, date, started_at, ended_at, minutes, note, created_by) "
            f"VALUES ('{club}', '{zid}', '{day}', {start}, {end}, (extract(epoch FROM ({end}) - ({start})) / 60)::int, "
            f"'{note}', '{zid}') RETURNING id || '|' || minutes")
    i, m = r.split("|")
    return i, int(m)


def night_to(end):
    """Вчерашняя ночная заканчивается в end: минуты — по времени."""
    sql(f"UPDATE work_entries SET ended_at = {end}, minutes = (extract(epoch FROM ({end}) - started_at) / 60)::int "
        f"WHERE id = '{night}'")
    return int(sql(f"SELECT minutes FROM work_entries WHERE id = '{night}'"))


section("записи табеля z15o: вчера днём, позавчера ночью, вчера ночью, сегодня")
_, m_day = entry(D1, at_club(D1, "08:00"), at_club(D1, "20:00"), "z15o день вчера")
_, m_old = entry(D2, at_club(D2, "20:00"), at_club(D1, f"{RH:02d}:05"), "z15o ночь позавчера")
night, m_night = entry(D1, at_club(D1, "20:00"), f"{T0} + interval '5 minutes'", "z15o ночь вчера")
_, m_today = entry(D0, f"{T0} + interval '10 minutes'", f"{T0} + interval '40 minutes'", "z15o сегодня")
s, d = login(Z)  # без client — вход не админки, смену не открывает
tk = (d or {}).get("access_token", "")
me = lambda: api("GET", "/admin/work/me", None, token=tk)[1]
check(f"записи готовы: вчера днём {m_day} мин, позавчера ночью {m_old} мин, вчера ночью {m_night} мин (до {RH:02d}:05 "
      f"сегодня), сегодня {m_today} мин; вход z15o — 200, смена не открылась",
      s == 200 and (m_day, m_today) == (720, 30) and m_old > 0 and m_night > 0
      and sql(f"SELECT count(*) FROM work_entries WHERE user_id = '{zid}' AND ended_at IS NULL") == "0", f"{s}")

section("«Сегодня отработано»: смены этих клубных суток и те, что в них закончились")
w = me()
check(f"не на смене, «Сегодня отработано» — {m_night + m_today} мин: ночная со вчера ({m_night}) и сегодняшняя "
      f"({m_today}); вчерашняя дневная и позавчерашняя ночная — нет",
      w.get("on_shift") is False and w.get("date") == D0 and w.get("minutes_today") == m_night + m_today, J(w))
m_at = night_to(T0)
w2 = me()
check(f"ночная закрыта ровно в начале суток (так закрывает забытый уход по графику) — тоже сегодня: {m_at + m_today} мин",
      w2.get("minutes_today") == m_at + m_today, J(w2))
night_to(f"{T0} - interval '1 minute'")
w3 = me()
check(f"закончилась за минуту до начала суток — это ещё вчера: «Сегодня отработано» — {m_today} мин (а до этого ночная "
      f"считалась)", w3.get("minutes_today") == m_today and w.get("minutes_today") == m_night + m_today, J(w3))
done()
