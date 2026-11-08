#!/usr/bin/env python3
"""З18-и4д «Даты «Бухгалтерии» по клубным суткам» (З-Р38, пункт 4; решение основателя 07.11): «сегодня» «Бухгалтерии» —
клубные сутки, а не дата календаря. Своя граница суток — на час-два позже «сейчас» (как ночью до 08:00 у клуба с границей
8: клубные сутки ещё вчерашние). Сводка отдаёт club_today — клубные сутки. Шаблон на число клубных суток, заведённый
сейчас, ждёт с этого месяца: в «Ждут внесения» — «срок сегодня» (просрочки нет), бейдж +1, напоминание — «срок сегодня,
ДД.ММ»; шаблон на число следующих суток (по календарю — сегодняшнее) не ждёт и не напоминает: ближайший срок — завтра,
«Внести заранее». Внесённый по шаблону расход с датой клубных суток — в «Расходах» за сегодня, заранее — тоже. Приёмка
сейчас, цены без даты документа — расход днём приёмки по клубным суткам. В 23:xx своей границей сутки не сдвинуть
(граница 24 — это 0): проверки идут на сутках, равных календарю; в последние полторы минуты 22:xx проверка ждёт 23:00.
Шаблоны «z18d Аренда» и «z18d Связь» заводятся каждый прогон и удаляются SQL вместе с напоминаниями о них; расходы с
заметкой «z18d проверка» и расход поставки сторнируются, позиция «z18d Вода» удаляется drop_good вместе с поставкой;
граница суток — как была."""
import time

from _z import *

T1, T2, GOOD, MARK = "z18d Аренда", "z18d Связь", "z18d Вода", "z18d проверка"
H, LEFT = (int(x) for x in sql(f"SELECT extract(hour FROM now() AT TIME ZONE '{TZ}')::int || ' ' || "
                                "(3600 - extract(epoch FROM now() - date_trunc('hour', now())))::int").split())
if H == 22 and LEFT < 90:  # граница 23 вот-вот наступит — ждём её: проверка не должна застать смену суток
    time.sleep(LEFT + 2)
    H = 23
RH = H + 2 if H <= 21 else 23 if H == 22 else None  # граница позже «сейчас» — сутки ещё вчерашние; в 23:xx — не сдвинуть
cats = {c.get("code"): c.get("id") for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", []) if c.get("code")}
ru = lambda d: f"{d[8:10]}.{d[5:7]}"
exps = set()


def cleanup():
    for eid in set(exps) | set(rows(f"SELECT id FROM expenses WHERE storno_at IS NULL AND (note LIKE '{MARK}%' OR note LIKE '%{GOOD}%')")):
        api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{GOOD}'"):
        drop_good(gid)
    sql(f"DELETE FROM expense_templates WHERE name IN ('{T1}', '{T2}')")
    sql("DELETE FROM staff_messages WHERE text LIKE '%«z18d %'")


cleanup()  # хвост упавшего прогона
at_end(cleanup)
if RH is not None:
    set_settings(report_hour=RH)
t0 = sql("SELECT now()::text")
CD, NX, CAL = club_day(0), club_day(1), sql(f"SELECT (now() AT TIME ZONE '{TZ}')::date::text")
M, NM = CD[:8] + "01", NX[:8] + "01"  # месяц клубных суток и месяц следующих

section(f"своя граница {RH:02d}:00, сейчас {H:02d}:xx — клубные сутки {CD}, календарь {CAL}" if RH is not None
        else f"{H:02d}:xx — своей границей сутки не сдвинуть: клубные сутки {CD} = календарь")
s, ov = api("GET", "/admin/overview", None, "boss")
check(f"сводка: club_today — клубные сутки {CD}" + (f", а не дата календаря {CAL}" if RH is not None else ""),
      s == 200 and ov.get("club_today") == CD and (RH is None or CD != CAL), f"{s} {ov.get('club_today')} {ov.get('day_from')}")
n0 = ov.get("expenses_due") or 0

section("регулярные: срок — по клубным суткам")
mk = lambda name, day, cat, amount: api("POST", "/admin/expense-templates", {"name": name, "category_id": cat, "amount_pln": amount,
                                                                              "method": "bank", "day": day, "note": MARK}, "boss")
s1, d1 = mk(T1, int(CD[8:]), cats.get("rent"), 2100)
s2, d2 = mk(T2, int(NX[8:]), cats.get("other"), 90)
a, b = d1.get("template") or {}, d2.get("template") or {}
check(f"«{T1}» на {int(CD[8:])}-е (число клубных суток) заведён сейчас → 201: ждёт с {M}, ближайший срок {CD}, ждёт 1, «заранее» нет",
      s1 == 201 and a.get("from_month") == M and a.get("next_due") == CD and a.get("waiting") == 1 and "early" not in a, f"{s1} {J(d1)}")
check(f"«{T2}» на {int(NX[8:])}-е (число следующих суток{', по календарю — сегодняшнее' if RH is not None else ''}) → 201: "
      f"не ждёт, ближайший срок {NX} — завтра, «Внести заранее» за {NM}",
      s2 == 201 and b.get("from_month") == NM and b.get("next_due") == NX and b.get("waiting") == 0 and b.get("early") == NM, f"{s2} {J(d2)}")
due = [x for x in api("GET", "/admin/expenses/due", None, "boss")[1].get("due", []) if x.get("template_id") in (a.get("id"), b.get("id"))]
got = [(x.get("name"), x.get("month"), x.get("due"), x.get("overdue_days")) for x in due]
check(f"«Ждут внесения»: только «{T1}» за {M}, срок {CD}, просрочки нет — «срок сегодня»", got == [(T1, M, CD, 0)], J(got))
n1 = api("GET", "/admin/overview", None, "boss")[1].get("expenses_due") or 0
check(f"бейдж «Бухгалтерии»: ждут внесения {n0} → {n1} (+1 — «{T1}»)", n1 == n0 + 1, f"{n0} → {n1}")
s, r = api("POST", "/admin/expenses/due/remind", None, "boss")
msgs = rows(f"SELECT text FROM staff_messages WHERE text LIKE '%«z18d %' AND created_at >= '{t0}' ORDER BY created_at")
check(f"напоминание — одно, о «{T1}»: «… срок сегодня, {ru(CD)}»; о «{T2}» — ничего (срок завтра)",
      s == 200 and len(msgs) == 1 and f"«{T1}»" in msgs[0] and f"срок сегодня, {ru(CD)}" in msgs[0], J([s, r, msgs]))

section("внести — датой клубных суток")
enter = lambda tid, month, amount, note: api("POST", "/admin/expenses", {"op_date": CD, "category_id": cats.get("other"), "amount_pln": amount,
                                                                         "method": "bank", "note": f"{MARK}: {note}", "template_id": tid,
                                                                         "template_month": month}, "boss")
s1, e1 = enter(a.get("id"), M, 2100, "в срок")
s2, e2 = enter(b.get("id"), NM, 90, "заранее")
ids = [(e.get("expense") or {}).get("id") for e in (e1, e2)]
exps.update(i for i in ids if i)
lst = api("GET", "/admin/expenses?preset=today", None, "boss")[1]
per, seen = lst.get("period") or {}, [x.get("id") for x in lst.get("expenses", [])]
check(f"«{T1}» за {M} и «{T2}» за {NM} заранее датой {CD} → 201; оба — в «Расходах» за сегодня (период {per.get('from')}–{per.get('to')})",
      s1 == 201 and s2 == 201 and per.get("from") == CD and per.get("to") == CD and all(i in seen for i in ids), J([s1, e1, s2, e2, per]))
due = [x for x in lst.get("due", []) if x.get("template_id") in (a.get("id"), b.get("id"))]
check("больше ничего из них не ждёт", due == [], J(due))

section("приёмка — днём приёмки по клубным суткам")
gid = (api("POST", "/admin/goods", {"name": GOOD, "category": "z18", "price_pln": 3}, "boss")[1].get("good") or {}).get("id")
s, d = api("POST", "/admin/supplies", {"lines": [{"good_id": gid, "qty": 4}], "note": MARK}, "admin1")
sid = (d.get("supply") or {}).get("id")
s2, d2 = api("PUT", f"/admin/supplies/{sid}/pricing", {"lines": [{"good_id": gid, "prices": [{"qty": 4, "price_pln": 1.5}]}], "method": "bank"}, "boss")
e3 = (d2.get("supply") or {}).get("expense") or {}
if e3.get("id"):
    exps.add(e3.get("id"))
seen = [x.get("id") for x in api("GET", "/admin/expenses?preset=today", None, "boss")[1].get("expenses", [])]
check(f"admin1 принял «{GOOD}» × 4 сейчас, владелец оценил по 1,50 без даты документа → расход 6 zł датой {CD}, в «Расходах» за сегодня",
      s == 201 and s2 == 200 and e3.get("amount_pln") == 6 and e3.get("op_date") == CD and e3.get("id") in seen, J([s, s2, e3]))
done()
