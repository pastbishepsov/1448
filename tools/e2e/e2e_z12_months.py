#!/usr/bin/env python3
"""З12-и4а «Обзор» по месяцам (решение основателя 04.10: «давай лучше сделаем по месяцам, но всегда можно отфильтровать
от даты до даты»). Отчёты понимают `month=ГГГГ-ММ` — календарный месяц: текущий — по сегодняшние клубные сутки, как пресет
«Этот месяц» (сравнение — тот же отрезок прошлого месяца); прошлый — целиком, сравнение — с позапрошлым целиком; будущий
и не «ГГГГ-ММ» — 400 bad_month; `month` главнее пресета. Разбор общий: «Прибыль» и «Деньги» за месяц — те же, что за его
даты; Σ дней ряда «Обзора» = итог. Данных не заводит — только читает отчёты."""
from _z import *
import calendar
from urllib.parse import quote

CUR = club_day()[:7]
y, m = int(CUR[:4]), int(CUR[5:7])


def shift(k):
    """Месяц со сдвигом k от текущего — 'ГГГГ-ММ'."""
    t = y * 12 + (m - 1) + k
    return f"{t // 12:04d}-{t % 12 + 1:02d}"


def last_day(mm):
    return f"{mm}-{calendar.monthrange(int(mm[:4]), int(mm[5:7]))[1]:02d}"


def get(path, who="boss"):
    return api("GET", path, None, who)


g = lambda v: round((v or 0) * 100)


def nums(p):
    return [g((p.get(k) or {}).get("pln")) for k in ("income", "returns", "cogs", "losses", "expenses")] + \
           [g(p.get(k)) for k in ("gross_pln", "costs_pln", "result_pln")]


def span(p, key):
    x = p.get(key) or {}
    return x.get("from"), x.get("to"), x.get("days")


section("календарный месяц: текущий, прошлый, позапрошлый")
s1, cur = get(f"/admin/reports/profit?month={CUR}")
s2, pm = get("/admin/reports/profit?preset=month")
check(f"month={CUR} (текущий) — то же, что «Этот месяц»: с 1-го по сегодня, сравнение — тот же отрезок прошлого месяца, суммы те же",
      s1 == 200 and s2 == 200 and span(cur, "period")[:2] == (f"{CUR}-01", club_day()) and span(cur, "period") == span(pm, "period")
      and span(cur, "prev_period") == span(pm, "prev_period") and nums(cur) == nums(pm),
      J({"cur": [span(cur, "period"), span(cur, "prev_period")], "month": [span(pm, "period"), span(pm, "prev_period")]}))
P1 = shift(-1)
s1, a = get(f"/admin/reports/profit?month={P1}")
s2, b = get("/admin/reports/profit?preset=prev_month")
check(f"month={P1} (прошлый) — то же, что «Прошлый месяц»: месяц целиком, сравнение — позапрошлый целиком, суммы те же",
      s1 == 200 and s2 == 200 and span(a, "period") == (f"{P1}-01", last_day(P1), int(last_day(P1)[8:]))
      and span(a, "prev_period")[:2] == (f"{shift(-2)}-01", last_day(shift(-2))) and span(a, "prev_period") == span(b, "prev_period")
      and nums(a) == nums(b), J({"month": [span(a, "period"), span(a, "prev_period")], "prev_month": [span(b, "period"), span(b, "prev_period")]}))
P2, P3 = shift(-2), shift(-3)
s1, c = get(f"/admin/reports/profit?month={P2}")
s2, cd = get(f"/admin/reports/profit?from={P2}-01&to={last_day(P2)}")
s3, pr = get(f"/admin/reports/profit?from={P3}-01&to={last_day(P3)}")
days = c.get("days") or []
check(f"month={P2} (позапрошлый): {P2}-01…{last_day(P2)}, сравнение — {P3} целиком; суммы — как за те же даты, прошлый период — "
      f"как за даты {P3}; ряд — все дни месяца, Σ дней = доходы и затраты",
      s1 == s2 == s3 == 200 and span(c, "period") == span(cd, "period")[:2] + (int(last_day(P2)[8:]),)
      and span(c, "prev_period")[:2] == (f"{P3}-01", last_day(P3)) and nums(c) == nums(cd)
      and [g((c.get("prev") or {}).get(k + "_pln")) for k in ("income", "result")] == [g((pr.get("income") or {}).get("pln")), g(pr.get("result_pln"))]
      and len(days) == int(last_day(P2)[8:]) and sum(g(x.get("income_pln")) for x in days) == g((c.get("income") or {}).get("pln"))
      and sum(g(x.get("costs_pln")) for x in days) == g(c.get("costs_pln")),
      J({"period": span(c, "period"), "prev": span(c, "prev_period"), "days": len(days)}))

section("ошибки и старшинство")
bad = [get(f"/admin/reports/profit?month={quote(q)}") for q in (shift(1), "2026-1", "2026-13", "октябрь")]
check(f"будущий месяц ({shift(1)}), «2026-1», «2026-13», «октябрь» — 400 bad_month с понятным текстом",
      all(st == 400 and d.get("code") == "bad_month" and "ГГГГ-ММ" in (d.get("message") or "") for st, d in bad), J(bad))
st, d = get(f"/admin/reports/profit?month={P1}&preset=d7")
check("month главнее пресета: month + preset=d7 — всё равно месяц", st == 200 and span(d, "period")[:2] == (f"{P1}-01", last_day(P1)), J(span(d, "period")))
st, mo = get(f"/admin/reports/money?month={P1}")
st2, mp = get("/admin/reports/money?preset=prev_month")
check("разбор общий: «Деньги» за month — тот же период, сравнение и выручка, что у «Прошлого месяца»",
      st == st2 == 200 and span(mo, "period") == span(mp, "period") and span(mo, "prev_period") == span(mp, "prev_period")
      and g((mo.get("totals") or {}).get("revenue_pln")) == g((mp.get("totals") or {}).get("revenue_pln")),
      J({"month": span(mo, "period"), "preset": span(mp, "period")}))
st = get(f"/admin/reports/profit?month={P1}", "admin1")[0]
check("админу «Прибыль» за месяц — 403, как и за любой период", st == 403, st)
done()
