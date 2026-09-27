#!/usr/bin/env python3
"""З9-и3а «Расход из кассы и ящик» (З-Р30, №62, №63 «а»; решение основателя 27.09): расход способом «из кассы» —
деньги ушли из ящика. Внесён за сегодня — «должно быть в ящике» меньше с момента записи: в предпросмотре сдачи —
«расходы из кассы», в кассовой книге — сумма в строке дня и сам расход в её раскрытии, строка сходится. Внесён
задним числом за день, который уже закрыт сдачей, — текущий ящик не меняется, а у той сдачи в кассовой книге —
«объяснено расходом», расхождение её дня уменьшается на сумму. Сменили способ на банк — ящик вернулся, обратно на
кассу — как если бы так и внесли: пересчёта с записи не было — с этой минуты, был — объясняет его (нехватку он уже
увидел); не оплачен — ящик не трогает, отметили оплату — с этой минуты; сторно — ящик вернулся.
Временный сотрудник z9t на смене; он дважды сдаёт кассу с недостачей — 20 и 15 zł. Расходы с заметкой
«z9t проверка» в конце сторнируются, z9t уволен."""
from _z import *

MARK = "z9t проверка"
hire_temp("z9t")
tok("z9t")  # вход в админку открывает смену
drawer_ready("z9t")
cats = {c.get("code"): c.get("id") for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", [])}
preview = lambda: api("GET", "/admin/work/closeout", None, "z9t")[1]
book = lambda q="preset=today": api("GET", f"/admin/cash/book?{q}", None, "boss")[1]
made = []


def cleanup():
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
at_end(cleanup)
cleanup()


def expense(amount, day=None, **kw):
    body = {"op_date": day or club_day(0), "category_id": cats.get("other"), "amount_pln": amount, "method": "till",
            "note": f"{MARK}: {amount}", **kw}
    s, d = api("POST", "/admin/expenses", body, "boss")
    e = d.get("expense", {})
    made.append(e.get("id"))
    return s, e


def adds_up(r):
    if r.get("opening_pln") is None:
        return True
    return near(r["opening_pln"] + r["in_pln"] - r["out_pln"] + r["adjust_pln"] + r["diff_pln"], r["closing_pln"])


exp_ = lambda: preview().get("expected_pln", 0)

section("за сегодня — «должно быть в ящике» меньше сразу")
p0, t0 = preview(), book()["days"][-1]
s, e1 = expense(25)
p1, t1 = preview(), book()["days"][-1]
check("расход 25 zł «из кассы» за сегодня → 201, в ответе — «ящик меньше с момента записи»", s == 201 and (e1.get("till") or {}).get("live") is True,
      f"{s} {J(e1.get('till'))}")
check("предпросмотр сдачи (админ): «должно быть» −25, «расходы из кассы» +25", near(p1.get("expected_pln", 0), p0.get("expected_pln", 0) - 25)
      and near(p1.get("expenses_pln", 0), p0.get("expenses_pln", 0) + 25), f"{p0.get('expected_pln')}→{p1.get('expected_pln')} · {p1.get('expenses_pln')}")
mine = [x for x in t1.get("expenses", []) if x.get("id") == e1.get("id")]
check("кассовая книга, сегодня: расходы из кассы +25, расход — в раскрытии дня («Прочее»), строка сходится",
      near(t1.get("expenses_pln", 0) - t0.get("expenses_pln", 0), 25) and mine and mine[0].get("category") == "Прочее"
      and "explains_closeout" not in mine[0] and adds_up(t1) and near(t1.get("closing_pln", -1), p1.get("expected_pln", -2)),
      J({k: t1.get(k) for k in ("expenses_pln", "out_pln", "closing_pln")}))

section("способ, оплата, сторно — ящик следует за расходом")
s, _ = api("PATCH", f"/admin/expenses/{e1.get('id')}", {"method": "bank"}, "boss")
back = exp_()
s2, _ = api("PATCH", f"/admin/expenses/{e1.get('id')}", {"method": "till"}, "boss")
again = exp_()
check("банком — ящик как был; снова «из кассы» — снова −25", s == 200 and s2 == 200 and near(back, p0.get("expected_pln", 0))
      and near(again, p0.get("expected_pln", 0) - 25), f"{back} · {again}")
s, e2 = expense(10, paid=False)
unpaid = exp_()
s2, _ = api("PATCH", f"/admin/expenses/{e2.get('id')}", {"paid": True}, "boss")
paid = exp_()
check("«из кассы», не оплачен — ящик не трогает; отметили оплату — −10", s == 201 and "till" not in e2 and near(unpaid, again)
      and s2 == 200 and near(paid, again - 10), f"{unpaid} → {paid}")
s, _ = api("POST", f"/admin/expenses/{e2.get('id')}/storno", {"reason": "проверка сторно"}, "boss")
check("сторно — ящик вернулся (+10)", s == 200 and near(exp_(), again), str(exp_()))

section("задним числом за день, закрытый сдачей, — объясняет её расхождение")
pre = exp_()
s, co = api("POST", "/admin/work/stop", {"counted_pln": round(pre - 20, 2)}, "z9t")
api("POST", "/admin/work/start", {}, "z9t")
p2 = exp_()
d7_before = book("preset=d7").get("days", [])
# по правилу: последняя сдача вчерашних клубных суток, а нет её — первая после них (сегодняшняя, в т. ч. z9t); сдали
# без пересчёта — первый пересчёт после неё
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
y0, y1 = at_club(club_day(-1), f"{rh:02d}:00"), at_club(club_day(0), f"{rh:02d}:00")
pts = [r.split("|") for r in rows(f"SELECT id, counted_grosz IS NOT NULL, created_at < {y1} FROM shift_closeouts "
                                  f"WHERE opening_grosz IS NOT NULL AND created_at >= {y0} ORDER BY created_at")]
day_pts = [i for i, p in enumerate(pts) if p[2] == "t"]
pick = day_pts[-1] if day_pts else (0 if pts else None)
want = next((pts[i][0] for i in range(pick, len(pts)) if pts[i][1] == "t"), None) if pick is not None else None
s, e3 = expense(20, club_day(-1), category_id=cats.get("rent"))
till = e3.get("till") or {}
cid = till.get("closeout_id")
p3 = exp_()
check("расход 20 zł за вчера «из кассы» объясняет сдачу по правилу (последнюю вчерашнюю, иначе первую после; без пересчёта — первый пересчёт) — в ответе какую, кто, её расхождение",
      s == 201 and till.get("live") is False and cid and cid == want and till.get("closeout_by") and till.get("closeout_diff_pln") is not None,
      f"{s} {J(till)} · ждали {want}")
check("текущий ящик не изменился — «должно быть» как после сдачи", near(p3, p2), f"{p2} → {p3}")
d7 = book("preset=d7").get("days", [])
day = next((x for x in d7 if any(c.get("id") == cid for c in x.get("closeouts", []))), {})
was = next((x for x in d7_before if x.get("date") == day.get("date")), {})
cor = next((c for c in day.get("closeouts", []) if c.get("id") == cid), {})
check("кассовая книга: у той сдачи — «объяснено расходом» 20, расхождение её дня +20, расход — в раскрытии дня, строка сходится",
      near(cor.get("explained_pln", 0), 20) and near(day.get("diff_pln", 0) - was.get("diff_pln", 0), 20)
      and any(x.get("id") == e3.get("id") and x.get("explains_closeout") == cid for x in day.get("expenses", [])) and adds_up(day),
      J({"день": day.get("date"), "расхождение": [was.get("diff_pln"), day.get("diff_pln")], "сдача": {k: cor.get(k) for k in ("explained_pln", "diff_pln")}}, 400))
check("все дни недели сходятся", all(adds_up(x) for x in d7), J([x.get("date") for x in d7 if not adds_up(x)]))

section("правка способа после пересчёта — объясняет пересчёт, а не ящик")
s, e4 = expense(15, method="bank")
pre = exp_()
s1, st = api("POST", "/admin/work/stop", {"counted_pln": round(pre - 15, 2)}, "z9t")
api("POST", "/admin/work/start", {}, "z9t")
co4 = ((st or {}).get("closeout") or {}).get("id")
p4 = exp_()
s2, d = api("PATCH", f"/admin/expenses/{e4.get('id')}", {"method": "till"}, "boss")
t4 = (d.get("expense") or {}).get("till") or {}
p5 = exp_()
check("внесли банком, z9t пересчитал с недостачей 15, способ поправили на «из кассы» → объясняет этот пересчёт, «должно быть» как было",
      s == 201 and s1 == 200 and s2 == 200 and co4 and t4.get("live") is False and t4.get("closeout_id") == co4 and near(p5, p4),
      f"{J(t4)} · ждали {co4} · {p4} → {p5}")
cor4 = next((c for x in book().get("days", []) for c in x.get("closeouts", []) if c.get("id") == co4), {})
check("кассовая книга: у пересчёта z9t — «объяснено расходом» 15, сама недостача −15 в записи сдачи осталась",
      near(cor4.get("explained_pln", 0), 15) and near(cor4.get("diff_pln", 0), -15), J(cor4))
done()
