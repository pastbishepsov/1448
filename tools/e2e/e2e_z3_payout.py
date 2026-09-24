#!/usr/bin/env python3
"""З3-и2 «К выплате» по подтверждённым часам (№20 «а»): в «Отчётах → Персонал → Люди» выплата считается только
по подтверждённым часам — человек сам отметил уход или запись поправил владелец. Автозакрытые (ушёл, не
отметившись, — часы поставлены по графику) видны отдельно и в выплату не входят. Смена — день, в котором есть
подтверждённые минуты: два захода за день — одна смена (сценарий I ревью: при ставке 200 zł за смену было
400 zł за один день). Временный z3pay со ставкой 200 zł за смену; его записи табеля удаляются."""
from _z import *

zid = hire_temp("z3pay")
at_end(lambda: sql(f"DELETE FROM work_entries WHERE user_id='{zid}'"))
sql(f"DELETE FROM work_entries WHERE user_id='{zid}'")
sql(f"UPDATE staff_profiles SET rate_type='shift', rate_amount=200 WHERE user_id='{zid}'")
d1, d2 = club_day(-3), club_day(-2)
row = lambda frm, to: next((x for x in api("GET", f"/admin/reports/staff?from={frm}&to={to}", None, "boss")[1]
                            .get("by_person", []) if x.get("nickname") == "z3pay"), {})

section(f"два захода за день {d1}: 09–13 и 14–18")
for start, end in (("09:00", "13:00"), ("14:00", "18:00")):
    s, d = api("POST", "/admin/work", {"nickname": "z3pay", "date": d1, "start": start, "end": end}, "boss")
    check(f"запись {start}–{end} → 201", s == 201, f"{s} {J(d)}")
r = row(d1, d1)
check("одна смена, 8 ч, к выплате 200 zł — а не 400", r.get("fact_shifts") == 1 and near(r.get("fact_hours", 0), 8)
      and near(r.get("payout_pln", -1), 200), J(r))

section(f"автозакрытая смена {d2}: «День 08–20», уход не отмечен")
tok("z3pay")  # вход в админку открывает запись табеля — её и делаем забытой
e2 = sql(f"SELECT id FROM work_entries WHERE user_id='{zid}' AND ended_at IS NULL")
sql(f"UPDATE work_entries SET date='{d2}', started_at={at_club(d2, '08:00')}, "
    f"shift_id=(SELECT id FROM shifts WHERE name='День') WHERE id='{e2}'")
s, d = api("POST", "/admin/work/start", {}, "z3pay")  # прошлая запись не закрыта — сервер закроет её по графику
check("забытая запись закрыта автоматически", s == 201 and sql(f"SELECT auto_closed FROM work_entries WHERE id='{e2}'") == "t", f"{s}")
r = row(d1, d2)
check("в выплату не вошла: смен 1, часов 8, к выплате 200 zł", r.get("fact_shifts") == 1 and near(r.get("fact_hours", 0), 8)
      and near(r.get("payout_pln", -1), 200), J(r))
check("видна отдельно: 12 ч не подтверждены (автозакрытие)", near(r.get("auto_closed_hours", -1), 12), J(r))

section("владелец поправил автозакрытую запись — часы подтверждены")
s, d = api("PATCH", f"/admin/work/{e2}", {"start": "08:00", "end": "20:00", "note": "z3: подтверждено"}, "boss")
r = row(d1, d2)
check("смен 2, часов 20, к выплате 400 zł, неподтверждённых нет", s == 200 and r.get("fact_shifts") == 2
      and near(r.get("fact_hours", 0), 20) and near(r.get("payout_pln", -1), 400) and not r.get("auto_closed_hours"), J(r))
done()
