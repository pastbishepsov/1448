#!/usr/bin/env python3
"""З9-и3в «Взяли до сдачи» (решение основателя 27.09, вопрос 2): оплаченный расход «из кассы» владелец относит к сдаче,
до которой взяли деньги (before_closeout). z9bt пересчитал ящик с недостачей 12 zł, владелец вносит расход 12 zł «из
кассы» за сегодня до этой сдачи — расход её объясняет, «должно быть» не меняется, в кассовой книге у сдачи — «объяснено
расходом» 12, сама недостача в записи сдачи остаётся. До сдачи без пересчёта, до сдачи другого дня, не из кассы, кривой
id — 400 bad_closeout, расход не записан. Правкой «после сдач» ("") — «должно быть» меньше с этой минуты, в журнале правки
— «ящик: до сдачи … → из ящика с …»; обратно «до сдачи» — снова объясняет её; правка без поля — место как было.
Временный z9bt; расходы с заметкой «z9b проверка» в конце сторнируются, z9bt уволен."""
from _z import *

MARK = "z9b проверка"
hire_temp("z9bt")
tok("z9bt")  # вход в админку открывает смену
drawer_ready("z9bt")
cats = {c.get("code"): c.get("id") for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", [])}
exp_ = lambda: api("GET", "/admin/work/closeout", None, "z9bt")[1].get("expected_pln", 0)
book = lambda: api("GET", "/admin/cash/book?preset=today", None, "boss")[1]


def cleanup():
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
at_end(cleanup)
cleanup()


def expense(amount, method, day=None, **kw):
    body = {"op_date": day or club_day(0), "category_id": cats.get("other"), "amount_pln": amount, "method": method,
            "note": f"{MARK}: {method} {amount}", **kw}
    s, d = api("POST", "/admin/expenses", body, "boss")
    return s, (d.get("expense") if s == 201 else d) or {}


section("до сдачи — расход объясняет её, ящик не трогает")
pre = exp_()
s, stp = api("POST", "/admin/work/stop", {"counted_pln": round(pre - 12, 2)}, "z9bt")
api("POST", "/admin/work/start", {}, "z9bt")
co = ((stp or {}).get("closeout") or {}).get("id")
pa = exp_()
s, e = expense(12, "till", before_closeout=co)
till = e.get("till") or {}
check("z9bt пересчитал с недостачей 12, расход 12 zł из кассы за сегодня «до сдачи» → объясняет её (кто, расхождение), «должно быть» как было",
      s == 201 and co and till.get("live") is False and till.get("closeout_id") == co and till.get("closeout_by") == "z9bt"
      and near(till.get("closeout_diff_pln", 0), -12) and near(exp_(), pa), f"{s} {J(till)} · {pa} → {exp_()}")
cor = next((c for c in book()["days"][-1].get("closeouts", []) if c.get("id") == co), {})
check("кассовая книга: у сдачи — «объяснено расходом» 12, недостача −12 в записи сдачи осталась",
      near(cor.get("explained_pln", 0), 12) and near(cor.get("diff_pln", 0), -12), J({k: cor.get(k) for k in ("explained_pln", "diff_pln")}))

section("не годится — 400, расход не записан")
s, stp2 = api("POST", "/admin/work/stop", {}, "z9bt")  # ушёл без пересчёта
api("POST", "/admin/work/start", {}, "z9bt")
skip_id = ((stp2 or {}).get("closeout") or {}).get("id")
n0 = int(sql(f"SELECT count(*) FROM expenses WHERE note LIKE '{MARK}%'"))
bad = [expense(5, "till", before_closeout=skip_id), expense(5, "till", day=club_day(-1), before_closeout=co),
       expense(5, "bank", before_closeout=co), expense(5, "till", paid=False, before_closeout=co), expense(5, "till", before_closeout="не-uuid")]
n1 = int(sql(f"SELECT count(*) FROM expenses WHERE note LIKE '{MARK}%'"))
check("«до сдачи» без пересчёта, другого дня, не из кассы, не оплачен, кривой id → 400 bad_closeout, ничего не записано",
      all(s == 400 and d.get("code") == "bad_closeout" for s, d in bad) and n1 == n0, J([(s, d.get("code")) for s, d in bad]))

section("передумали — правкой")
pb = exp_()
s, u = api("PATCH", f"/admin/expenses/{e.get('id')}", {"before_closeout": ""}, "boss")
t = (u.get("expense") or {}).get("till") or {}
lines = [x.get("text", "") for x in audit("boss", limit=50) if "ящик: до сдачи" in (x.get("text") or "")]
check("«после сдач» → changed 1, «должно быть» −12 с этой минуты, в журнале правки — «ящик: до сдачи … → из ящика с …»",
      s == 200 and u.get("changed") == 1 and t.get("live") is True and near(exp_(), pb - 12) and any("→ из ящика с" in x for x in lines),
      J({"changed": u.get("changed"), "till": t, "ящик": [pb, exp_()], "журнал": lines[:1]}))
s, u2 = api("PATCH", f"/admin/expenses/{e.get('id')}", {"before_closeout": co}, "boss")
t2 = (u2.get("expense") or {}).get("till") or {}
check("обратно «до сдачи» → снова объясняет её, «должно быть» как до правок", s == 200 and t2.get("closeout_id") == co and near(exp_(), pb), J(t2))
s, u3 = api("PATCH", f"/admin/expenses/{e.get('id')}", {"note": f"{MARK}: курьер, поправил заметку"}, "boss")
t3 = (u3.get("expense") or {}).get("till") or {}
s4, u4 = api("PATCH", f"/admin/expenses/{e.get('id')}", {"before_closeout": co}, "boss")
check("правка заметки без поля — место как было; тот же ответ ещё раз — «изменений нет»",
      s == 200 and t3.get("closeout_id") == co and near(exp_(), pb) and s4 == 200 and u4.get("changed") == 0, J({"till": t3, "changed": u4.get("changed")}))
done()
