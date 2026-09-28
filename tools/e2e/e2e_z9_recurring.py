#!/usr/bin/env python3
"""З9-и5 «Регулярные расходы» (№67, Д10; решение 28.09 — только ежемесячные). Шаблон: название, категория, сумма, VAT,
способ, число месяца; то же имя в другом регистре — 409 с id, поля проверяются как у расхода (скрытая категория и
выключенный способ — 400). Ждёт с ближайшего срока: заведён сегодня на сегодняшнее число — ждёт этот месяц; пропущенный
месяц ждёт тоже, старые — первыми. «Внести» — расход с template_id и template_month: за месяц один (повтор и две записи
разом — 409), месяц вне «с первого месяца шаблона по текущий» — 400, сторно освобождает месяц. Напоминание — одно на срок,
«только владельцу»; внесённый месяц не напоминает. Выключенный не ждёт и не напоминает; включённый ждёт с ближайшего
срока (пропущенное не вешается). Бейдж: overview.expenses_due — владельцу. Админу — 403 и ни строки.
Шаблоны «z9c Аренда зала», «z9c Интернет» и скрытая категория «z9c Скрытая» живут между прогонами (удаления нет) — в
конце шаблоны выключены; расходы по ним сторнируются, способы оплаты — все включены."""
import threading

from _z import *

TPL, TPL2, HID, MARK = "z9c Аренда зала", "z9c Интернет", "z9c Скрытая", "z9c проверка"
NOID = "00000000-0000-0000-0000-000000000000"  # шаблона нет (сервер без ручек) — SQL ниже всё равно валиден
ALL = ["till", "bank", "card", "owner"]
_cats = [c for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", []) if c.get("code")]
std, RENT = {c.get("code"): c.get("id") for c in _cats}, next((c.get("name") for c in _cats if c.get("code") == "rent"), "")
CAL = sql(f"SELECT (now() AT TIME ZONE '{TZ}')::date::text")  # «сегодня» шаблонов — календарный день клуба
D = int(CAL[8:])
M, PM, PPM, NM, NNM = (sql(f"SELECT (date_trunc('month', '{CAL}'::date) + interval '{k} month')::date::text") for k in (0, -1, -3, 1, 2))
LAST = sql(f"SELECT (date_trunc('month', '{CAL}'::date) + interval '1 month - 1 day')::date::text")


def due_of(m, day):
    """Срок месяца m: число day, нет такого — последний день."""
    return sql(f"SELECT LEAST('{m}'::date + {day - 1}, (date_trunc('month', '{m}'::date) + interval '1 month - 1 day')::date)::text")


def tpl_row(tid):
    return next((t for t in api("GET", "/admin/expense-templates", None, "boss")[1].get("templates", []) if t.get("id") == tid), {})


def due(tid=None):
    items = api("GET", "/admin/expenses/due", None, "boss")[1].get("due", [])
    return [x for x in items if tid is None or x.get("template_id") == tid]


def enter(tid, month, **kw):
    body = {"op_date": club_day(0), "category_id": std.get("rent"), "amount_pln": 4500, "method": "bank", "note": MARK,
            "template_id": tid, "template_month": month, **kw}
    s, d = api("POST", "/admin/expenses", body, "boss")
    return s, (d.get("expense") if s == 201 else d) or {}


def storno(eid):
    return api("POST", f"/admin/expenses/{eid}/storno", {"reason": "уборка проверки"}, "boss")[0]


def reminders(since):
    return rows(f"SELECT visible_to || '|' || text FROM staff_messages WHERE text LIKE '%«{TPL}»%' AND created_at >= '{since}' ORDER BY created_at")


def ours():
    return rows(f"SELECT id FROM expense_templates WHERE name IN ('{TPL}', '{TPL2}')")


def cleanup():
    api("PUT", "/admin/expense-methods", {"on": ALL}, "boss")
    ids = ",".join(f"'{i}'" for i in ours()) or "NULL"
    for i in rows(f"SELECT id FROM expenses WHERE (template_id IN ({ids}) OR note LIKE '{MARK}%') AND storno_at IS NULL"):
        storno(i)
    for i in ours():
        api("PATCH", f"/admin/expense-templates/{i}", {"active": False}, "boss")
at_end(cleanup)
cleanup()

section("шаблоны")
s, d = api("POST", "/admin/expense-templates", {"name": TPL, "category_id": std.get("rent"), "amount_pln": 4500, "method": "bank",
                                                  "day": D, "note": MARK}, "boss")
tid = (d.get("template") or {}).get("id") or d.get("id") or sql(f"SELECT id FROM expense_templates WHERE name = '{TPL}'") or NOID
fresh_from = (d.get("template") or {}).get("from_month")
# повторный прогон: шаблон с прошлого раза — к виду «заведён сегодня на сегодняшнее число»
sql(f"UPDATE expense_templates SET category_id = '{std.get('rent')}', counterparty_id = NULL, gross_grosz = 450000, vat_grosz = 0, "
    f"method = 'bank', day = {D}, note = '{MARK}', active = true, from_month = '{M}', reminded_for = NULL WHERE id = '{tid}' RETURNING id")
row = tpl_row(tid)
check(f"«{TPL}» 4500 zł банком, число {D} (сегодня) → 201 (с прошлого прогона — 409 с id): ждёт с этого месяца, ближайший срок — сегодня, ждёт 1",
      ((s == 201 and fresh_from == M) or (s == 409 and d.get("code") == "template_exists" and tid)) and row.get("active") is True
      and row.get("from_month") == M and row.get("next_due") == CAL and row.get("waiting") == 1 and row.get("category") == RENT,
      f"{s} {J(d, 200)} {J(row)}")
s, d = api("POST", "/admin/expense-templates", {"name": TPL2, "category_id": std.get("other"), "amount_pln": 120, "vat_pln": 22.44,
                                                  "method": "card", "day": 15, "note": MARK}, "boss")
tid2 = (d.get("template") or {}).get("id") or d.get("id") or sql(f"SELECT id FROM expense_templates WHERE name = '{TPL2}'") or NOID
s2, d2 = api("POST", "/admin/expense-templates", {"name": " z9C аренда ЗАЛА ", "category_id": std.get("rent"), "amount_pln": 1,
                                                    "method": "bank", "day": 1}, "boss")
s3, d3 = api("PATCH", f"/admin/expense-templates/{tid2}", {"active": False}, "boss")
check(f"«{TPL2}» 120 zł (VAT 22,44) картой 15-го → 201/409; то же имя, что у первого, в другом регистре → 409 с его id; выключить второй → 200",
      (s == 201 or (s == 409 and d.get("code") == "template_exists")) and tid2 and s2 == 409 and d2.get("id") == tid
      and s3 == 200 and (d3.get("template") or {}).get("active") is False, J([s, s2, d2.get("id") == tid, s3]))
base = {"name": "z9c пробный", "category_id": std.get("other"), "amount_pln": 10, "method": "bank", "day": 5}
bad = [({"name": " "}, "bad_template_name"), ({"name": "я" * 65}, "bad_template_name"), ({"day": 0}, "bad_day"), ({"day": 32}, "bad_day"),
       ({"amount_pln": 0}, "bad_amount"), ({"vat_pln": 11}, "bad_vat"), ({"method": "cash"}, "bad_method"), ({"day": None}, "invalid_request"),
       ({"category_id": "кривой"}, "bad_category"), ({"category_id": "00000000-0000-0000-0000-000000000001"}, "bad_category")]
got = []
for patch, code in bad:
    body = {k: v for k, v in {**base, **patch}.items() if v is not None}
    s, d = api("POST", "/admin/expense-templates", body, "boss")
    got.append((s, d.get("code"), code))
check("пустое и длиннее 64 имя, число 0 и 32, сумма 0, VAT больше суммы, неизвестный способ, без числа, кривая и чужая категория → 400 с кодом",
      all(s == 400 and c == w for s, c, w in got), J(got, 600))
s, d = api("POST", "/admin/expense-categories", {"name": HID}, "boss")
hid = (d.get("category") or {}).get("id") or d.get("id")
api("PATCH", f"/admin/expense-categories/{hid}", {"hidden": True}, "boss")
s, d = api("POST", "/admin/expense-templates", {**base, "category_id": hid}, "boss")
api("PUT", "/admin/expense-methods", {"on": ["till", "bank", "owner"]}, "boss")
s2, d2 = api("POST", "/admin/expense-templates", {**base, "method": "card"}, "boss")
s3, d3 = api("PATCH", f"/admin/expense-templates/{tid2}", {"note": f"{MARK}: карта выключена"}, "boss")
api("PUT", "/admin/expense-methods", {"on": ALL}, "boss")
check("скрытая категория → 400 category_hidden; выключенная «карта» → 400 method_off, а шаблон с картой правится как есть",
      s == 400 and d.get("code") == "category_hidden" and s2 == 400 and d2.get("code") == "method_off" and s3 == 200,
      J([s, d.get("code"), s2, d2.get("code"), s3]))
s, d = api("PATCH", f"/admin/expense-templates/{tid}", {"name": "Z9C ИНТЕРНЕТ"}, "boss")
s2, d2 = api("PATCH", f"/admin/expense-templates/{tid}", {"day": 31}, "boss")
s3, d3 = api("PATCH", f"/admin/expense-templates/{tid}", {"day": D, "amount_pln": 4500, "method": "bank"}, "boss")
check(f"переименовать в имя второго → 409 с его id; число 31 → ближайший срок {LAST} (последний день месяца); вернуть число — «изменения» 1",
      s == 409 and d.get("id") == tid2 and s2 == 200 and (d2.get("template") or {}).get("next_due") == LAST
      and s3 == 200 and d3.get("changed") == 1, J([s, s2, (d2.get("template") or {}).get("next_due"), s3, d3.get("changed")]))

section("ждут внесения")
mine = due(tid)
cnt = api("GET", "/admin/expenses/due", None, "boss")[1].get("count")
ov = api("GET", "/admin/overview", None, "boss")[1].get("expenses_due")
lst = [x for x in api("GET", "/admin/expenses", None, "boss")[1].get("due", []) if x.get("template_id") == tid]
check("«Ждут внесения»: этот месяц, срок сегодня, 0 дней просрочки, 4500 zł банком «Аренда»; бейдж overview = число ждущих; то же в списке расходов",
      len(mine) == 1 and mine[0].get("month") == M and mine[0].get("due") == CAL and mine[0].get("overdue_days") == 0
      and near(mine[0].get("amount_pln", 0), 4500) and mine[0].get("method") == "bank" and mine[0].get("category") == RENT
      and ov == cnt and cnt >= 1 and len(lst) == 1, J({"due": mine, "count": cnt, "overview": ov}))
sql(f"UPDATE expense_templates SET from_month = '{PM}' WHERE id = '{tid}' RETURNING id")
mine, row = due(tid), tpl_row(tid)
check(f"пропущенный прошлый месяц тоже ждёт — первым, с просрочкой; в шаблоне «ждёт 2», ближайший срок {due_of(PM, D)}",
      [x.get("month") for x in mine] == [PM, M] and mine[0].get("overdue_days", 0) > 0 and mine[0].get("due") == due_of(PM, D)
      and row.get("waiting") == 2 and row.get("next_due") == due_of(PM, D), J([mine, row.get("waiting"), row.get("next_due")]))

section("внести по шаблону")
s, e = enter(tid, M)
line = [x.get("text", "") for x in audit("boss", kind="expense_create", limit=20) if TPL in (x.get("text") or "")]
check(f"«Внести» за этот месяц → 201: расход помнит шаблон и месяц {M}; ждёт только прошлый; в журнале «по шаблону «{TPL}» за {M[5:7]}.{M[:4]}»",
      s == 201 and e.get("template_id") == tid and e.get("template_month") == M and [x.get("month") for x in due(tid)] == [PM]
      and any(f"по шаблону «{TPL}» за {M[5:7]}.{M[:4]}" in t for t in line), f"{s} {J(e, 200)} {J(line[:1])}")
got = [enter(tid, M), enter(tid, NNM), enter(tid, PPM), enter("кривой", M), enter(tid, "2026-13-01")]  # следующий — можно заранее (З9-и5б)
got = [(s, d.get("code")) for s, d in got]
check("повтор за тот же месяц → 409 template_entered; через месяц и до первого месяца шаблона → 400 bad_template_month; "
      "кривой шаблон → 400 bad_template; кривой месяц → 400",
      got == [(409, "template_entered"), (400, "bad_template_month"), (400, "bad_template_month"), (400, "bad_template"),
              (400, "bad_template_month")], J(got))
s, e2 = enter(tid, PM[:7])
row = tpl_row(tid)
check(f"прошлый месяц строкой «ГГГГ-ММ» → 201; больше не ждёт ничего, ближайший срок — {due_of(NM, D)}",
      s == 201 and e2.get("template_month") == PM and due(tid) == [] and row.get("waiting") == 0 and row.get("next_due") == due_of(NM, D),
      J([s, e2.get("template_month"), row.get("waiting"), row.get("next_due")]))
st = storno(e.get("id"))
again = [x.get("month") for x in due(tid)]
tok("boss")
res = []
th = [threading.Thread(target=lambda: res.append(enter(tid, M)[0])) for _ in range(2)]
[t.start() for t in th]
[t.join() for t in th]
check("сторно освобождает месяц — снова ждёт; две записи за него разом → 201 и 409", st == 200 and again == [M] and sorted(res) == [201, 409],
      J([st, again, res]))

section("напоминание")
t0 = sql("SELECT now()::text")
sql(f"UPDATE expense_templates SET reminded_for = NULL WHERE id = '{tid}' RETURNING id")
api("POST", "/admin/expenses/due/remind", {}, "boss")
r1 = reminders(t0)
rf = sql(f"SELECT reminded_for::text FROM expense_templates WHERE id = '{tid}'")
check("месяц уже внесён → напоминания нет, месяц отмечен", r1 == [] and rf == M, J([r1, rf]))
eid = sql(f"SELECT id FROM expenses WHERE template_id = '{tid}' AND template_month = '{M}' AND storno_at IS NULL")
storno(eid)
sql(f"UPDATE expense_templates SET reminded_for = NULL WHERE id = '{tid}' RETURNING id")
api("POST", "/admin/expenses/due/remind", {}, "boss")
api("POST", "/admin/expenses/due/remind", {}, "boss")
r2 = reminders(t0)
check(f"ждёт → одно сообщение «только владельцу»: «Пора внести регулярный расход «{TPL}» — 4500,00 zł, банк; срок сегодня»; второй прогон молчит",
      len(r2) == 1 and r2[0].startswith("owner|") and f"«{TPL}» — 4500,00 zł, банк; срок сегодня" in r2[0], J(r2))

section("выключить и включить")
before = api("GET", "/admin/expenses/due", None, "boss")[1].get("count", 0)
s, d = api("PATCH", f"/admin/expense-templates/{tid}", {"active": False}, "boss")
after = api("GET", "/admin/expenses/due", None, "boss")[1].get("count", 0)
sql(f"UPDATE expense_templates SET reminded_for = NULL WHERE id = '{tid}' RETURNING id")
api("POST", "/admin/expenses/due/remind", {}, "boss")
row = tpl_row(tid)
check("выключить → не ждёт (ждущих на 1 меньше), без ближайшего срока, и не напоминает",
      s == 200 and after == before - 1 and due(tid) == [] and row.get("active") is False and "next_due" not in row and len(reminders(t0)) == 1,
      J([s, before, after, row.get("active"), len(reminders(t0))]))
sql(f"UPDATE expense_templates SET from_month = '{PPM}' WHERE id = '{tid}' RETURNING id")
storno(e2.get("id"))
s, d = api("PATCH", f"/admin/expense-templates/{tid}", {"active": True}, "boss")
s2, d2 = api("PATCH", f"/admin/expense-templates/{tid}", {"active": True}, "boss")
jr = [x.get("text", "") for x in audit("boss", kind="expense_template_update", limit=20) if TPL in (x.get("text") or "")]
check("включить → ждёт с ближайшего срока (этот месяц), пропущенные раньше не вешаются; повтор — «изменений нет»; в журнале «выключен» и «включён»",
      s == 200 and (d.get("template") or {}).get("from_month") == M and [x.get("month") for x in due(tid)] == [M] and d2.get("changed") == 0
      and any(t.endswith(": выключен") for t in jr) and any(t.endswith(": включён") for t in jr), J([s, d.get("template"), jr[:2]]))

section("только владельцу")
st = [api("GET", "/admin/expense-templates", None, "admin1")[0],
      api("POST", "/admin/expense-templates", {**base, "name": "z9c админ"}, "admin1")[0],
      api("PATCH", f"/admin/expense-templates/{tid}", {"active": False}, "admin1")[0],
      api("GET", "/admin/expenses/due", None, "admin1")[0], api("POST", "/admin/expenses/due/remind", {}, "admin1")[0]]
ova = api("GET", "/admin/overview", None, "admin1")[1]
seen = [x.get("text", "") for x in audit("admin1") if "z9c" in (x.get("text") or "")]
check("админу — 403 на все ручки шаблонов; в overview числа ждущих нет; строк журнала о шаблонах не видит",
      st == [403] * 5 and ova.get("expenses_due") is None and "day_from" in ova and not seen, J({"статусы": st, "видит": seen[:2]}))
done()
