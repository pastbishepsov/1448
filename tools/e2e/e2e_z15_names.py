#!/usr/bin/env python3
"""З15-и4а «Имена смен у старых записей табеля — из журнала прихода» (решение основателя 22.10). Миграция 086 дала
записям табеля имя смены снимком, а записи, чей шаблон удалили раньше, остались без имени: ссылку на шаблон удаление уже
обнулило. Миграция 087 берёт имя из строки журнала, которой человек сам открыл запись: его work_start в пределах двух
секунд от создания записи — «вход в админку · …», «пришёл · …», до 24.09 — «вход в систему · …»; из нескольких строк —
ближайшая. Ручные записи владельца, записи «вне смены» и записи с именем не трогает. Проверка готовит такие записи —
настоящими входом, «Я пришёл» и ручной записью, у которых потом удаляется шаблон и стирается имя (как до 086), и
подставными записями со строками журнала — и прогоняет 087 ещё раз (она идемпотентна). Временный z15n — сотрудник;
шаблон «z15n имена» (порядок 9000) удаляется по ходу; записи табеля z15n и подставные строки журнала удаляются в начале
и в конце; денег не трогает."""
import os
import subprocess

from _z import *

MIG = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..", "..", "backend", "migrations",
                   "087_work_shift_name_from_audit.sql")
Z, TPL = "z15n", "z15n имена"
FAKE = ("вход в админку (вне смены)", "вход в систему · z15n старое", "вход в админку · z15n чужое",
        "вход в админку · z15n дальняя", "вход в админку · z15n ближняя", "вход в админку · z15n поздно")
zid = hire_temp(Z)
club = sql("SELECT id FROM clubs ORDER BY created_at LIMIT 1")


def drop():
    for i in rows(f"SELECT id FROM shifts WHERE name LIKE 'z15n %'"):
        api("DELETE", f"/admin/shifts/{i}", None, "boss")
    sql(f"DELETE FROM work_entries WHERE user_id = '{zid}'")
    fake = ", ".join(f"'{x}'" for x in FAKE)
    sql(f"DELETE FROM admin_actions WHERE admin_id = '{zid}' AND action = 'work_start' AND details IN ({fake})")


drop()  # хвосты упавшего прогона
at_end(drop)
name = lambda e: sql(f"SELECT shift_name FROM work_entries WHERE id = '{e}'")


def synth(n, hhmm, label=""):
    """Подставная запись табеля z15n, открытая им самим: закрытая, без шаблона, создана в начале — n дней назад, hhmm по
    часам клуба."""
    d = club_day(-n)
    return sql(f"INSERT INTO work_entries (club_id, user_id, date, started_at, ended_at, minutes, note, shift_name, created_by, "
               f"created_at) VALUES ('{club}', '{zid}', '{d}', {at_club(d, hhmm)}, {at_club(d, hhmm)} + interval '1 hour', 60, "
               f"'z15n подставная', '{label}', '{zid}', {at_club(d, hhmm)}) RETURNING id")


def journal(e, details, sec):
    """Строка журнала work_start от z15n через sec секунд после создания записи e."""
    sql(f"INSERT INTO admin_actions (admin_id, action, details, created_at) SELECT '{zid}', 'work_start', '{details}', "
        f"created_at + interval '{sec} seconds' FROM work_entries WHERE id = '{e}'")


def migrate():
    r = subprocess.run(["psql", "-v", "ON_ERROR_STOP=1", DB, "-f", MIG], capture_output=True, text=True)
    return r.returncode, (r.stdout + r.stderr).strip()


section("записи, как их оставил шаблон, удалённый до 086")
now_min = int(sql(f"SELECT (extract(hour FROM now() AT TIME ZONE '{TZ}') * 60 + extract(minute FROM now() AT TIME ZONE '{TZ}'))::int"))
sday = sql(f"SELECT ((now() AT TIME ZONE '{TZ}') - interval '60 minutes')::date::text")  # день начала смены
s, t = api("POST", "/admin/shifts", {"name": TPL, "start_min": (now_min - 60) % 1440, "end_min": (now_min + 120) % 1440,
                                     "sort": 9000, "force": True}, "boss")
tid = t.get("id")
s2, _ = api("POST", f"/admin/shifts/{tid}/assign", {"date": sday, "nickname": Z}, "boss")
tok(Z)  # вход в админку открывает смену по назначению — «вход в админку · z15n имена»
e1 = sql(f"SELECT id FROM work_entries WHERE user_id = '{zid}' AND ended_at IS NULL")
sql(f"UPDATE work_entries SET ended_at = now(), started_at = started_at - interval '1 hour' WHERE id = '{e1}'")  # владелец поправил начало
s3, d3 = api("POST", "/admin/work/start", {}, Z)  # «Я пришёл» — «пришёл · z15n имена»
e2 = ((d3 or {}).get("entry") or {}).get("id", "")
sql(f"UPDATE work_entries SET ended_at = now() WHERE id = '{e2}'")
s4, d4 = api("POST", "/admin/work", {"nickname": Z, "date": club_day(-2), "start": "10:00", "end": "12:00",
                                     "note": "z15n ручная"}, "boss")  # ручная запись владельца — в журнале без смены
e3 = ((d4 or {}).get("entry") or {}).get("id", "")
check("вход z15n и «Я пришёл» открыли записи «z15n имена», владелец добавил ручную",
      (s, s2, s3, s4) == (201, 201, 201, 201) and name(e1) == TPL and name(e2) == TPL and e3 != "", f"{(s, s2, s3, s4)}")
s, _ = api("DELETE", f"/admin/shifts/{tid}", None, "boss")
sql(f"UPDATE work_entries SET shift_id = NULL, shift_name = '' WHERE id IN ('{e1}', '{e2}', '{e3}')")  # как до 086
e4 = synth(4, "10:00")
journal(e4, "вход в админку (вне смены)", 0.002)
e5 = synth(5, "10:00")
journal(e5, "вход в систему · z15n старое", 0.002)
e6 = synth(6, "10:00", "z15n своё")
journal(e6, "вход в админку · z15n чужое", 0.002)
e7 = synth(7, "10:00")
journal(e7, "вход в админку · z15n дальняя", 1.5)
journal(e7, "вход в админку · z15n ближняя", 0.002)
e8 = synth(8, "10:00")
journal(e8, "вход в админку · z15n поздно", 3)
blank = sql(f"SELECT count(*) FROM work_entries WHERE id IN ('{e1}', '{e2}', '{e3}', '{e4}', '{e5}', '{e7}', '{e8}') "
            f"AND shift_id IS NULL AND shift_name = ''")
check("шаблон удалён, имена стёрты: семь записей без шаблона и без имени, у одной имя есть",
      s == 200 and blank == "7" and name(e6) == "z15n своё", f"{s} {blank}")

section("миграция 087: имя — из строки журнала, которой запись открыли")
have = os.path.exists(MIG)
check("миграция 087 есть", have, MIG)
rc1, out1 = migrate() if have else (-1, "")
got = {k: name(v) for k, v in {"вход": e1, "пришёл": e2, "ручная": e3, "вне смены": e4, "вход в систему": e5,
                                  "с именем": e6, "две строки": e7, "3 с": e8}.items()}
check("вход в админку (начало потом поправлено) — «z15n имена»: ищется по моменту создания записи, а не по началу",
      rc1 == 0 and got["вход"] == TPL, f"{rc1} {out1[:120]} {J(got)}")
check("«Я пришёл» — «z15n имена»", got["пришёл"] == TPL, J(got))
check("старый формат журнала «вход в систему · …» (до 24.09) — «z15n старое»", got["вход в систему"] == "z15n старое", J(got))
check("две строки журнала рядом — имя из ближайшей: «z15n ближняя», не «z15n дальняя» (+1,5 с)",
      got["две строки"] == "z15n ближняя", J(got))
check("не трогает: ручная запись владельца (добавлена сразу после «Я пришёл» — открыл её не сам человек) и «вне смены» — "
      "без имени, строка журнала позже двух секунд — без имени, запись с именем — «z15n своё»",
      got["ручная"] == "" and got["вне смены"] == "" and got["3 с"] == "" and got["с именем"] == "z15n своё", J(got))
rc2, out2 = migrate() if have else (-1, "")
check("повторный прогон 087 ничего не меняет — UPDATE 0", rc1 == 0 and rc2 == 0 and out2.endswith("UPDATE 0"), f"{rc2} {out2[-60:]}")
d = api("GET", f"/admin/work?from={club_day(-1)}&to={club_day(1)}", None, "boss")[1]
row = next((x for x in d.get("entries", []) if x.get("id") == e1), {})
check("табель — «z15n имена» у записи входа, шаблона которой больше нет", row.get("shift") == TPL, J(row, 200))
done()
