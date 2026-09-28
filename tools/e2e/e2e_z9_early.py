#!/usr/bin/env python3
"""З9-и5б «Внести заранее» (№67; решение основателя 28.09). У шаблона поле early — ближайший месяц со сроком впереди:
этот (срок ещё не наступил) или следующий (этот внесён); пока что-то ждёт — «заранее» нет, сначала «Ждут внесения»;
следующий внесён — дальше рано. Расход за следующий месяц сервер принимает (template_month — по следующий за текущим),
через месяц — 400; повтор — 409; сторно возвращает «заранее». Внесённый заранее месяц в срок не ждёт, прогон
напоминаний отмечает его без сообщения.
Шаблон «z9c Бухгалтер» живёт между прогонами (удаления нет): в начале его число, месяц начала и отметка напоминания
ставятся SQL, в конце он выключен; расходы по нему и с заметкой «z9c заранее» сторнируются."""
from _z import *

TPL, MARK = "z9c Бухгалтер", "z9c заранее"
NOID = "00000000-0000-0000-0000-000000000000"
other = next((c.get("id") for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", []) if c.get("code") == "other"), None)
CAL = sql(f"SELECT (now() AT TIME ZONE '{TZ}')::date::text")  # «сегодня» шаблонов — календарный день клуба
D = int(CAL[8:])
M, NM, NNM = (sql(f"SELECT (date_trunc('month', '{CAL}'::date) + interval '{k} month')::date::text") for k in (0, 1, 2))
LAST = sql(f"SELECT (date_trunc('month', '{CAL}'::date) + interval '1 month - 1 day')::date::text")


def due_of(m, day):
    """Срок месяца m: число day, нет такого — последний день."""
    return sql(f"SELECT LEAST('{m}'::date + {day - 1}, (date_trunc('month', '{m}'::date) + interval '1 month - 1 day')::date)::text")


def tpl_row(tid):
    return next((t for t in api("GET", "/admin/expense-templates", None, "boss")[1].get("templates", []) if t.get("id") == tid), {})


def enter(tid, month):
    body = {"op_date": club_day(0), "category_id": other, "amount_pln": 800, "method": "bank", "note": MARK,
            "template_id": tid, "template_month": month}
    s, d = api("POST", "/admin/expenses", body, "boss")
    return s, (d.get("expense") if s == 201 else d) or {}


def storno(eid):
    return api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")[0]


def cleanup():
    tid = sql(f"SELECT id FROM expense_templates WHERE name = '{TPL}'") or NOID
    for i in rows(f"SELECT id FROM expenses WHERE (template_id = '{tid}' OR note LIKE '{MARK}%') AND storno_at IS NULL"):
        storno(i)
    if tid != NOID:
        api("PATCH", f"/admin/expense-templates/{tid}", {"active": False}, "boss")
at_end(cleanup)
cleanup()

section("что можно внести заранее")
s, d = api("POST", "/admin/expense-templates", {"name": TPL, "category_id": other, "amount_pln": 800, "method": "bank", "day": D, "note": MARK}, "boss")
tid = (d.get("template") or {}).get("id") or d.get("id") or sql(f"SELECT id FROM expense_templates WHERE name = '{TPL}'") or NOID
# повторный прогон: шаблон с прошлого раза — к виду «заведён сегодня на сегодняшнее число»
sql(f"UPDATE expense_templates SET category_id = '{other}', counterparty_id = NULL, gross_grosz = 80000, vat_grosz = 0, method = 'bank', "
    f"day = {D}, note = '{MARK}', active = true, from_month = '{M}', reminded_for = NULL WHERE id = '{tid}' RETURNING id")
row = tpl_row(tid)
check(f"«{TPL}» на сегодняшнее число ({D}) → 201 (с прошлого прогона — 409): срок сегодня — ждёт, «заранее» нет — сначала «Ждут внесения»",
      (s == 201 or (s == 409 and d.get("code") == "template_exists")) and row.get("waiting") == 1 and "early" not in row and row.get("next_due") == CAL,
      J([s, row]))
s, em = enter(tid, M)
row = tpl_row(tid)
check(f"внесли этот месяц → ждать нечего, «заранее» — следующий месяц {NM}, ближайший срок {due_of(NM, D)}",
      s == 201 and row.get("waiting") == 0 and row.get("early") == NM and row.get("next_due") == due_of(NM, D), J([s, row]))

section("внести заранее")
s, en = enter(tid, NM)
row = tpl_row(tid)
line = [x.get("text", "") for x in audit("boss", kind="expense_create", limit=20) if TPL in (x.get("text") or "")]
check(f"следующий месяц по шаблону → 201, расход помнит {NM}; дальше «заранее» нет (следующий внесён), ближайший срок {due_of(NNM, D)}; "
      f"в журнале «по шаблону «{TPL}» за {NM[5:7]}.{NM[:4]}»",
      s == 201 and en.get("template_month") == NM and "early" not in row and row.get("waiting") == 0 and row.get("next_due") == due_of(NNM, D)
      and any(f"по шаблону «{TPL}» за {NM[5:7]}.{NM[:4]}" in t for t in line), J([s, en.get("template_month"), row, line[:1]]))
got = [(x[0], x[1].get("code")) for x in (enter(tid, NNM), enter(tid, NM))]
check("через месяц → 400 bad_template_month; следующий повтором → 409 template_entered",
      got == [(400, "bad_template_month"), (409, "template_entered")], J(got))
st = storno(en.get("id"))
row = tpl_row(tid)
check(f"сторно внесённого заранее → «заранее» снова {NM}, ближайший срок {due_of(NM, D)}",
      st == 200 and row.get("early") == NM and row.get("next_due") == due_of(NM, D), J([st, row]))

section("этот месяц — до срока")
storno(em.get("id"))
ahead = CAL < LAST  # в последний день месяца срока «впереди» в этом месяце не бывает
day = D + 1 if ahead else 31
s, d = api("PATCH", f"/admin/expense-templates/{tid}", {"day": day}, "boss")
row = tpl_row(tid)
if ahead:
    check(f"срок этого месяца впереди (число {day}) → ждать нечего, «заранее» — этот месяц {M}, ближайший срок — завтра",
          s == 200 and row.get("waiting") == 0 and row.get("early") == M and row.get("next_due") == due_of(M, day), J([s, row]))
else:
    check(f"сегодня последний день месяца: число 31 → срок сегодня, ждёт, «заранее» нет",
          s == 200 and row.get("waiting") == 1 and "early" not in row and row.get("next_due") == CAL, J([s, row]))
s, em = enter(tid, M)
sql(f"UPDATE expense_templates SET reminded_for = NULL WHERE id = '{tid}' RETURNING id")
t0 = sql("SELECT now()::text")
api("POST", "/admin/expenses/due/remind", {}, "boss")
msgs = rows(f"SELECT text FROM staff_messages WHERE text LIKE '%«{TPL}»%' AND created_at >= '{t0}'")
rf = sql(f"SELECT reminded_for::text FROM expense_templates WHERE id = '{tid}'")
row = tpl_row(tid)
check(f"внесли этот месяц{' заранее' if ahead else ''} → 201; прогон напоминаний отмечает месяц без сообщения; в срок не ждёт",
      s == 201 and em.get("template_month") == M and msgs == [] and rf == M and row.get("waiting") == 0, J([s, msgs, rf, row.get("waiting")]))
done()
