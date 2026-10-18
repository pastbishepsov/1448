#!/usr/bin/env python3
"""З15-и4 «Шаблоны смен: порядок, дни, имя в табеле» (№30, З-Р20). Три дыры шаблонов смен. Порядок нельзя было вернуть
в 0: PATCH sort 0 молча оставлял прежний. Убранный из шаблона день недели оставлял назначения на него в базе, но не в
сетке — невидимые, они давали смену при входе, «по графику» в сводке и плановые часы. Удаление шаблона обнуляло смену в
записи табеля: имя уходило из табеля и из «Персонал → по сменам», переименование — переписывало историю. Теперь sort 0
сохраняется; сужение дней с будущими назначениями в убранные дни — 409 days_assigned со списком, drop_assignments снимает
их (прошедшие и уже идущие остаются), force этого не делает; имя смены — снимком в записи табеля, «по сменам» — по
табелю (деньги — к смене того, кто принял), и ни переименование, ни удаление шаблона его не трогают. Временный z15t —
сотрудник; шаблоны «z15t …» (порядок 9000 — после «Дня» и «Ночи») и записи табеля z15t удаляются в начале и в конце;
Guest46 — пополнение 1 zł картой от z15t и его отмена."""
from _z import *

Z, PFX = "z15t", "z15t "
zid = hire_temp(Z)
G = uid("Guest46")


def drop_tpls():
    for i in rows(f"SELECT id FROM shifts WHERE name LIKE '{PFX}%'"):
        api("DELETE", f"/admin/shifts/{i}", None, "boss")


def drop_entries():
    sql(f"DELETE FROM work_entries WHERE user_id = '{zid}'")


drop_tpls()  # хвосты упавшего прогона
drop_entries()
at_end(drop_tpls)
at_end(drop_entries)
cal = lambda n=0: sql(f"SELECT ((now() AT TIME ZONE '{TZ}')::date + {n})::text")  # календарный день клуба
wd = lambda d: int(sql(f"SELECT extract(isodow FROM date '{d}')::int - 1"))  # пн = 0, как в days_mask
ddmm = lambda d: f"{d[8:10]}.{d[5:7]}"
tpl = lambda name, start, end: api("POST", "/admin/shifts", {"name": PFX + name, "start_min": start, "end_min": end,
                                                             "sort": 9000, "force": True}, "boss")

section("порядок шаблона: 0 — тоже порядок")
s, t = tpl("порядок", 600, 660)
tid = t.get("id")
srt = lambda: sql(f"SELECT sort FROM shifts WHERE id = '{tid}'")
s1, d1 = api("PATCH", f"/admin/shifts/{tid}", {"sort": 0, "force": True}, "boss")
check("PATCH sort 0 — порядок 0, был 9000 (раньше ноль молча не сохранялся)",
      s == 201 and s1 == 200 and d1.get("sort") == 0 and srt() == "0", f"{s} {s1} {d1.get('sort')} {srt()}")
s2, _ = api("PATCH", f"/admin/shifts/{tid}", {"sort": 7, "force": True}, "boss")
s3, d3 = api("PATCH", f"/admin/shifts/{tid}", {"name": PFX + "порядок 2", "force": True}, "boss")
check("PATCH без sort порядок не трогает: 7 остаётся 7, имя поменялось",
      s2 == 200 and s3 == 200 and srt() == "7" and d3.get("name") == PFX + "порядок 2", f"{s2} {s3} {srt()}")
s4, d4 = api("PATCH", f"/admin/shifts/{tid}", {"sort": 10001, "force": True}, "boss")
check("порядок вне ±10 000 — по-прежнему 400 bad_sort, порядок прежний", s4 == 400 and d4.get("code") == "bad_sort" and srt() == "7",
      f"{s4} {J(d4)}")
api("DELETE", f"/admin/shifts/{tid}", None, "boss")

section("сужение дней: будущие назначения в убранные дни — со списком и подтверждением")
D = {n: cal(n) for n in (-1, 0, 1, 2, 3, 7)}
s, t = tpl("дни", 0, 1380)  # 00:00–23:00 каждый день: сегодняшняя смена уже идёт, завтрашняя — впереди
tid = t.get("id")
for n in (-1, 0, 1, 2, 7):
    api("POST", f"/admin/shifts/{tid}/assign", {"date": D[n], "nickname": Z}, "boss")
asg = lambda: sorted(rows(f"SELECT date::text FROM shift_assignments WHERE shift_id = '{tid}'"))
mask_now = lambda: sql(f"SELECT days_mask FROM shifts WHERE id = '{tid}'")
check("z15t в графике «z15t дни» вчера, сегодня, завтра, послезавтра и через неделю",
      s == 201 and asg() == sorted(D[n] for n in (-1, 0, 1, 2, 7)), f"{s} {J(asg())}")
mask = 127 & ~(1 << wd(D[0])) & ~(1 << wd(D[1]))  # убираем день недели сегодня и завтра
s, d = api("PATCH", f"/admin/shifts/{tid}", {"days_mask": mask, "force": True}, "boss")
listed = sorted(x.get("date") for x in d.get("assignments", []))
check("убрал сегодня и завтра — 409 days_assigned со списком: завтра и через неделю (сегодняшняя уже идёт, вчерашняя — "
      "история); force про пересечение людей с графика не снимает",
      s == 409 and d.get("code") == "days_assigned" and listed == [D[1], D[7]]
      and all(x.get("nickname") == Z for x in d.get("assignments", [])), f"{s} {J(d, 400)}")
msg = d.get("message", "")
check("в отказе — кто и когда: «В убранные дни на «z15t дни» уже назначены: … dd.mm z15t, … dd.mm z15t»",
      msg.startswith(f"В убранные дни на «{PFX}дни» уже назначены: ") and f"{ddmm(D[1])} {Z}" in msg and f"{ddmm(D[7])} {Z}" in msg, msg)
check("отказ ничего не сохранил: дни и все пять назначений прежние", mask_now() == "127" and len(asg()) == 5, f"{mask_now()} {J(asg())}")
seen = {x.get("id") for x in audit(kind="shift_tpl_update", limit=50)}
s, d = api("PATCH", f"/admin/shifts/{tid}", {"days_mask": mask, "force": True, "drop_assignments": True}, "boss")
check("«снять и сохранить» (drop_assignments) — 200, снято 2 (завтра и через неделю); вчера, сегодня и послезавтра на месте",
      s == 200 and d.get("unassigned") == 2 and d.get("days_mask") == mask and mask_now() == str(mask)
      and asg() == sorted([D[-1], D[0], D[2]]), f"{s} {J(d)} {J(asg())}")
texts = [x.get("text") or "" for x in audit(kind="shift_tpl_update", limit=50) if x.get("id") not in seen]
check("в журнале — новая строка, кого сняли: «z15t дни · … · сняты с графика в убранные дни: … dd.mm z15t, … dd.mm z15t»",
      len(texts) == 1 and texts[0].startswith(PFX + "дни · 00:00–23:00") and "· сняты с графика в убранные дни: " in texts[0]
      and f"{ddmm(D[1])} {Z}" in texts[0] and f"{ddmm(D[7])} {Z}" in texts[0], J(texts[:2], 400))
mask2 = mask & ~(1 << wd(D[3]))  # день недели через три дня — назначений нет
s, d = api("PATCH", f"/admin/shifts/{tid}", {"days_mask": mask2, "force": True}, "boss")
check("убрал день без будущих назначений — сохраняется сразу, без вопроса", s == 200 and d.get("days_mask") == mask2
      and "unassigned" not in d and len(asg()) == 3, f"{s} {J(d)}")
mask3 = mask2 & ~(1 << wd(D[2]))
s, d = api("PATCH", f"/admin/shifts/{tid}", {"days_mask": mask3, "force": True}, "boss")
check("убрал послезавтра с одним force — снова 409 days_assigned, послезавтра в графике",
      s == 409 and d.get("code") == "days_assigned" and [x.get("date") for x in d.get("assignments", [])] == [D[2]]
      and D[2] in asg() and mask_now() == str(mask2), f"{s} {J(d, 300)}")
api("DELETE", f"/admin/shifts/{tid}", None, "boss")

section("имя смены — снимком в табеле; «по сменам» — по табелю")
now_min = int(sql(f"SELECT (extract(hour FROM now() AT TIME ZONE '{TZ}') * 60 + extract(minute FROM now() AT TIME ZONE '{TZ}'))::int"))
sday = sql(f"SELECT ((now() AT TIME ZONE '{TZ}') - interval '60 minutes')::date::text")  # день начала смены
s, t = tpl("снимок", (now_min - 60) % 1440, (now_min + 120) % 1440)  # шла час назад, кончится через два
tid = t.get("id")
s2, _ = api("POST", f"/admin/shifts/{tid}/assign", {"date": sday, "nickname": Z}, "boss")
tok(Z)  # вход в админку открывает смену — по назначению
ent = lambda: sql(f"SELECT id || '|' || COALESCE(shift_id::text, '-') || '|' || shift_name FROM work_entries "
                  f"WHERE user_id = '{zid}' ORDER BY started_at DESC LIMIT 1")
eid, sid, sname = (ent().split("|") + ["", "", ""])[:3]
check("вход z15t открыл смену «z15t снимок»: в записи табеля — ссылка на шаблон и имя снимком",
      (s, s2) == (201, 201) and sid == tid and sname == PFX + "снимок", f"{(s, s2)} {ent()}")
day = club_day()


def by_shift():
    d = api("GET", f"/admin/reports/staff?from={day}&to={day}", None, "boss")[1]
    return {x.get("shift"): x for x in d.get("by_shift", [])}, d.get("totals", {})


def sheet():
    d = api("GET", f"/admin/work?from={club_day(-1)}&to={club_day(1)}", None, "boss")[1]
    return next((x for x in d.get("entries", []) if x.get("id") == eid), {})


me = lambda: (api("GET", "/admin/work/me", None, Z)[1].get("entry") or {}).get("shift")
pln = lambda b, k: (b.get(k) or {}).get("deposits_pln", 0)
b0, _ = by_shift()
s, dep = api("POST", f"/admin/users/{G}/deposit", {"amount_pln": 1, "method": "card"}, Z)
dep_id = dep.get("deposit_id")
at_end(lambda: api("POST", f"/admin/deposits/{dep_id}/void", {}, "boss"))
b1, _ = by_shift()
gain = {k: round(pln(b1, k) - pln(b0, k), 2) for k in set(b0) | set(b1)}
check("пополнение 1 zł от z15t — в «по сменам» у «z15t снимок» (+1 zł), у других смен без изменений",
      s == 201 and near(gain.get(PFX + "снимок", 0), 1) and all(near(v, 0) for k, v in gain.items() if k != PFX + "снимок"), J(gain))
s, _ = api("PATCH", f"/admin/shifts/{tid}", {"name": PFX + "переименован", "force": True}, "boss")
b2, _ = by_shift()
check("переименовал шаблон: в табеле и в «Моей смене» — «z15t снимок», имя, с которым работал",
      s == 200 and sheet().get("shift") == PFX + "снимок" and me() == PFX + "снимок", f"{s} {J(sheet())} {me()}")
check("переименовал шаблон: пополнение — по-прежнему у «z15t снимок» (+1 zł к началу), у «z15t переименован» его нет",
      near(pln(b2, PFX + "снимок") - pln(b0, PFX + "снимок"), 1) and near(pln(b2, PFX + "переименован"), 0),
      J({k: pln(b2, k) for k in b2}))
s, _ = api("DELETE", f"/admin/shifts/{tid}", None, "boss")
b3, tot3 = by_shift()
check("удалил шаблон: в записи табеля ссылки на шаблон нет, имя «z15t снимок» осталось",
      s == 200 and ent() == f"{eid}|-|{PFX}снимок", ent())
check("удалил шаблон: табель и «Моя смена» — «z15t снимок»", sheet().get("shift") == PFX + "снимок" and me() == PFX + "снимок",
      f"{J(sheet())} {me()}")
check("удалил шаблон: в «по сменам» строка «z15t снимок» с пополнением осталась (+1 zł к началу), «вне смен» не выросло",
      near(pln(b3, PFX + "снимок") - pln(b0, PFX + "снимок"), 1) and near(pln(b3, "вне смен"), pln(b1, "вне смен")),
      J({k: pln(b3, k) for k in b3}))
check("Σ «по сменам» = выручка периода (деньги не потерялись)",
      near(sum(x.get("revenue_pln", 0) for x in b3.values()), tot3.get("revenue_pln", 0)),
      f"{sum(x.get('revenue_pln', 0) for x in b3.values())} {tot3.get('revenue_pln')}")
done()
