#!/usr/bin/env python3
"""З15-и4б «Имена смен у записей переименованных шаблонов — из журнала прихода» (решение основателя 23.10). Миграция 086
заполнила имя смены в старых записях табеля текущим именем шаблона: если шаблон переименовали до неё, запись получила
новое имя, а человек работал под старым (и так же — если такой шаблон потом удалили). Миграция 088 берёт имя из строки
журнала, которой человек сам открыл запись (его work_start в пределах двух секунд от создания записи, из нескольких —
ближайшая), если оно другое. Ручные записи владельца, «вне смены» и записи без строки журнала рядом не трогает. Проверка
готовит такие записи — настоящими входом, «Я пришёл» и ручной записью (переименование шаблона и имя «как после 086» — SQL)
и подставными записями со строками журнала — и прогоняет 088 ещё раз (она идемпотентна). Временный z15p — сотрудник;
шаблон «z15p …» (порядок 9000), записи табеля z15p и подставные строки журнала удаляются в начале и в конце; денег не
трогает."""
import os
import subprocess

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "migrations",
                   "088_work_shift_name_renamed.sql")
Z, OLD, NEW = "z15p", "z15p старое", "z15p новое"
FAKE = ("вход в систему · z15p давнее", "пришёл · z15p исходное", "вход в админку (вне смены)",
        "вход в админку · z15p поздно", "вход в админку · z15p дальняя", "вход в админку · z15p ближняя")
zid = hire_temp(Z)
club = sql("SELECT id FROM clubs ORDER BY created_at LIMIT 1")


def drop():
    for i in rows(f"SELECT id FROM shifts WHERE name LIKE 'z15p %'"):
        api("DELETE", f"/admin/shifts/{i}", None, "boss")
    sql(f"DELETE FROM work_entries WHERE user_id = '{zid}'")
    fake = ", ".join(f"'{x}'" for x in FAKE)
    sql(f"DELETE FROM admin_actions WHERE admin_id = '{zid}' AND action = 'work_start' AND details IN ({fake})")


drop()  # хвосты упавшего прогона
at_end(drop)
name = lambda e: sql(f"SELECT shift_name FROM work_entries WHERE id = '{e}'")


def synth(n, label, tpl=None):
    """Подставная запись табеля z15p, открытая им самим: закрытая, создана в начале — n дней назад в 10:00 по часам клуба;
    tpl — живой шаблон (иначе без шаблона), label — имя «как после 086»."""
    d = club_day(-n)
    t = f"'{tpl}'" if tpl else "NULL"
    return sql(f"INSERT INTO work_entries (club_id, user_id, shift_id, date, started_at, ended_at, minutes, note, shift_name, "
               f"created_by, created_at) VALUES ('{club}', '{zid}', {t}, '{d}', {at_club(d, '10:00')}, "
               f"{at_club(d, '10:00')} + interval '1 hour', 60, 'z15p подставная', '{label}', '{zid}', {at_club(d, '10:00')}) "
               f"RETURNING id")


def journal(e, details, sec):
    """Строка журнала work_start от z15p через sec секунд после создания записи e."""
    sql(f"INSERT INTO admin_actions (admin_id, action, details, created_at) SELECT '{zid}', 'work_start', '{details}', "
        f"created_at + interval '{sec} seconds' FROM work_entries WHERE id = '{e}'")


def migrate():
    r = subprocess.run(["psql", "-v", "ON_ERROR_STOP=1", DB, "-f", MIG], capture_output=True, text=True)
    return r.returncode, (r.stdout + r.stderr).strip()


section("записи, как их оставила 086 у переименованного шаблона")
now_min = int(sql(f"SELECT (extract(hour FROM now() AT TIME ZONE '{TZ}') * 60 + extract(minute FROM now() AT TIME ZONE '{TZ}'))::int"))
sday = sql(f"SELECT ((now() AT TIME ZONE '{TZ}') - interval '60 minutes')::date::text")  # день начала смены
s, t = api("POST", "/admin/shifts", {"name": OLD, "start_min": (now_min - 60) % 1440, "end_min": (now_min + 120) % 1440,
                                     "sort": 9000, "force": True}, "boss")
tid = t.get("id")
s2, _ = api("POST", f"/admin/shifts/{tid}/assign", {"date": sday, "nickname": Z}, "boss")
tok(Z)  # вход в админку открывает смену по назначению — «вход в админку · z15p старое»
e1 = sql(f"SELECT id FROM work_entries WHERE user_id = '{zid}' AND ended_at IS NULL")
n1 = name(e1)
s3, _ = api("PATCH", f"/admin/shifts/{tid}", {"name": NEW, "force": True}, "boss")  # переименовали
sql(f"UPDATE work_entries SET ended_at = now(), shift_name = '{NEW}' WHERE id = '{e1}'")  # как 086: текущее имя шаблона
s4, d4 = api("POST", "/admin/work/start", {}, Z)  # «Я пришёл» уже под новым именем — «пришёл · z15p новое»
e2 = ((d4 or {}).get("entry") or {}).get("id", "")
sql(f"UPDATE work_entries SET ended_at = now() WHERE id = '{e2}'")
s5, d5 = api("POST", "/admin/work", {"nickname": Z, "date": club_day(-2), "start": "10:00", "end": "12:00",
                                     "note": "z15p ручная"}, "boss")  # ручная запись владельца — в журнале без смены
e3 = ((d5 or {}).get("entry") or {}).get("id", "")
sql(f"UPDATE work_entries SET shift_name = 'z15p ручное' WHERE id = '{e3}'")
check("вход z15p открыл запись «z15p старое», шаблон переименовали в «z15p новое», «Я пришёл» — уже «z15p новое», "
      "владелец добавил ручную",
      (s, s2, s3, s4, s5) == (201, 201, 200, 201, 201) and n1 == OLD and name(e2) == NEW and e3 != "", f"{(s, s2, s3, s4, s5)} {n1}")
e4 = synth(4, NEW, tid)  # шаблон жив, в журнале — давнее имя старым форматом
journal(e4, "вход в систему · z15p давнее", 0.002)
e5 = synth(5, NEW)  # шаблон переименовали до 086, а удалили после
journal(e5, "пришёл · z15p исходное", 0.002)
e6 = synth(6, "")
journal(e6, "вход в админку (вне смены)", 0.002)
e7 = synth(7, NEW)
journal(e7, "вход в админку · z15p поздно", 3)
e8 = synth(8, NEW)
journal(e8, "вход в админку · z15p дальняя", 1.5)
journal(e8, "вход в админку · z15p ближняя", 0.002)
before = {k: name(v) for k, v in {"вход": e1, "пришёл": e2, "ручная": e3, "давнее": e4, "удалён": e5, "вне смены": e6,
                                     "3 с": e7, "две строки": e8}.items()}
check("до миграции: у всех записей имя «z15p новое», кроме ручной («z15p ручное») и «вне смены» (пусто)",
      all(v == NEW for k, v in before.items() if k not in ("ручная", "вне смены")) and before["ручная"] == "z15p ручное"
      and before["вне смены"] == "", J(before))

section("миграция 088: имя — из строки журнала, которой запись открыли, если оно другое")
have = os.path.exists(MIG)
check("миграция 088 есть", have, MIG)
rc1, out1 = migrate() if have else (-1, "")
got = {k: name(v) for k, v in {"вход": e1, "пришёл": e2, "ручная": e3, "давнее": e4, "удалён": e5, "вне смены": e6,
                                  "3 с": e7, "две строки": e8}.items()}
check("вход до переименования, шаблон жив — «z15p старое», как в журнале", rc1 == 0 and got["вход"] == OLD,
      f"{rc1} {out1[:80]} {J(got)}")
check("старый формат журнала «вход в систему · …», шаблон жив — «z15p давнее»", got["давнее"] == "z15p давнее", J(got))
check("шаблон переименовали до 086, а удалили после — «z15p исходное» из «пришёл · …»", got["удалён"] == "z15p исходное", J(got))
check("две строки журнала рядом — имя из ближайшей: «z15p ближняя»", got["две строки"] == "z15p ближняя", J(got))
check("не трогает: «Я пришёл» с верным именем — «z15p новое», ручная запись владельца — «z15p ручное», «вне смены» — "
      "пусто, строка журнала позже двух секунд — «z15p новое»",
      got["пришёл"] == NEW and got["ручная"] == "z15p ручное" and got["вне смены"] == "" and got["3 с"] == NEW, J(got))
rc2, out2 = migrate() if have else (-1, "")
check("повторный прогон 088 ничего не меняет — UPDATE 0", rc1 == 0 and rc2 == 0 and out2.endswith("UPDATE 0"), f"{rc2} {out2[-60:]}")
d = api("GET", f"/admin/work?from={club_day(-1)}&to={club_day(1)}", None, "boss")[1]
row = next((x for x in d.get("entries", []) if x.get("id") == e1), {})
check("табель — «z15p старое» у записи входа, хотя шаблон теперь называется «z15p новое»", row.get("shift") == OLD, J(row, 200))
done()
