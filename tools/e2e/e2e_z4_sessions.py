#!/usr/bin/env python3
"""З4-и1 «Сессии без отменённых, гости без персонала» (№6, №12): снятая посадка (status cancelled — ПК не вышел
на связь, посадку сняли, ничего не списано) — не сессия: её нет ни в «Сессиях», ни в «Новых», ни в «Средней
сессии», ни в сводке смены, и таяние монет не считает её активностью; в карточке гостя она остаётся в истории
с пометкой, но не в счётчике. Сессия персонала за ПК — не гость: в «Гостях» её нет, а в «Загрузке» она есть
(Д1). Всё — в пустом дне прошлого, чтобы чужие строки стенда не мешали: Guest34 — только снятая посадка,
Guest35 — настоящая сессия 2 ч, admin2 за ПК 1 ч. Идемпотентен: сессии удаляются, пополнение отменяется,
настройки таяния — как были."""
from _z import *

# последний день 3–25 дней назад без сессий: окно таяния (30 дней) его накрывает
D = sql(f"""SELECT g::date FROM generate_series(((now() AT TIME ZONE '{TZ}')::date - 25)::timestamp,
    ((now() AT TIME ZONE '{TZ}')::date - 3)::timestamp, interval '1 day') AS g
    WHERE NOT EXISTS (SELECT 1 FROM sessions WHERE started_at >= (g AT TIME ZONE '{TZ}')
      AND started_at < ((g + interval '2 days') AT TIME ZONE '{TZ}')) ORDER BY g DESC LIMIT 1""")
check(f"пустой день для проверки: {D}", D, D)
pc = pcs()["PC-08"]["id"]
ids = []
at_end(lambda: [sql(f"DELETE FROM sessions WHERE id='{i}'") for i in ids])

def put(nick, status, hhmm, minutes):
    ids.append(sql(f"""INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at,
        minutes_total, minutes_used, base_rate_pln, effective_rate_pln, ended_reason)
        SELECT '{uid(nick)}', id, club_id, '{status}', {at_club(D, hhmm)}, {at_club(D, hhmm)} + interval '{minutes} minutes',
        {minutes}, {minutes}, 0, 0, 'z4' FROM computers WHERE id='{pc}' RETURNING id"""))

put("Guest34", "cancelled", "12:00", 0)
put("admin2", "completed", "14:00", 60)
put("Guest35", "completed", "16:00", 120)

section(f"«Гости» за {D}")
g = api("GET", f"/admin/reports/guests?from={D}&to={D}", None, "boss")[1]
t = g.get("totals", {})
check("только Guest35: гостей 1, сессий 1, новых 1, часов 2", t.get("unique") == 1 and t.get("sessions") == 1
      and t.get("new") == 1 and near(t.get("hours", 0), 2), J(t))
check("топ по часам — Guest35, без admin2 и снятой посадки", [x.get("nickname") for x in g.get("top_hours", [])] == ["Guest35"],
      J(g.get("top_hours")))

section(f"«Загрузка» за {D}")
ld = api("GET", f"/admin/reports/load?from={D}&to={D}", None, "boss")[1].get("totals", {})
check("сессий 2 (Guest35 и admin2 — персонал в загрузке есть), часов 3, средняя 90 мин", ld.get("sessions") == 2
      and near(ld.get("hours", 0), 3) and ld.get("avg_minutes") == 90, J(ld))

section(f"сводка смены и «Персонал → по сменам» за {D}")
sh = api("GET", f"/admin/stats/shift?date={D}", None, "boss")[1]
check("сводка смены: сессий начато 2", sh.get("sessions", {}).get("started") == 2, J(sh.get("sessions")))
st = api("GET", f"/admin/reports/staff?from={D}&to={D}", None, "boss")[1]
check("по сменам: сессий 2", sum(x.get("sessions", 0) for x in st.get("by_shift", [])) == 2, J(st.get("by_shift")))

section("карточка Guest34")
card = api("GET", f"/admin/users/{uid('Guest34')}", None, "boss")[1]
cnt = card.get("stats", {}).get("sessions_count")
check("в счётчике сессий 0, в истории — снятая посадка со статусом cancelled", cnt == 0
      and any(s.get("status") == "cancelled" for s in card.get("sessions", [])), f"{cnt} {J(card.get('sessions'), 200)}")

section("таяние монет: активность — только настоящая сессия")
set_settings(coin_idle_days=30, coin_burn_pct_week=10)
dep = topup("Guest34", 20, who="boss")  # монеты на балансе — есть чему таять
at_end(lambda: api("POST", f"/admin/deposits/{dep}/void", {}, "boss"))
pv = api("GET", "/admin/coins/burn", None, "boss")[1]
check("Guest34 (зарегистрирован больше 30 дней назад, из сессий — одна снятая посадка) в предпросмотре таяния",
      any(x.get("nickname") == "Guest34" for x in pv.get("items", [])), J([x.get("nickname") for x in pv.get("items", [])]))
done()
