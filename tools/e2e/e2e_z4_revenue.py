#!/usr/bin/env python3
"""З4-и2 «Пакеты во всех разрезах» (№5, З-Р4 «одна выручка»): пакет времени, проданный за живые деньги, — выручка
везде, где есть выручка: в шапке, сводке смены (и в разбивке по способам), итоге «Денег», ряду по дням, «Кто
оформлял», «Персонал → по сменам» и «по людям». Поэтому сходятся суммы: Σ дней = итог; Σ «Кто оформлял» =
нал + карта + BLIK; Σ смен = KPI «Персонала»; сводка = шапка. Пакет с кошелька — не выручка ни в одном разрезе
(те деньги пришли пополнением), отменённый — тоже. Временный z4pack продаёт Guest35 «3 часа Standard» за 50 zł.
Идемпотентен: пакеты и пополнение отменяются, z4pack уволен."""
from _z import *

TODAY = club_day()
PACK = sql("SELECT id FROM time_packages WHERE active AND price_pln = 50 ORDER BY name LIMIT 1")
check("в каталоге есть активный пакет за 50 zł", PACK, PACK)
hire_temp("z4pack")
g = uid("Guest35")

def snap():
    """Все места, где владелец видит выручку за сегодня, — одним словарём чисел."""
    ov = api("GET", "/admin/overview", None, "boss")[1]
    sh = api("GET", "/admin/stats/shift", None, "boss")[1]
    mo = api("GET", "/admin/reports/money?preset=today", None, "boss")[1]
    st = api("GET", "/admin/reports/staff?preset=today", None, "boss")[1]
    t, rv = mo.get("totals", {}), sh.get("revenue", {})
    bm = rv.get("by_method", {})
    me = lambda rows: next((x for x in rows if x.get("nickname") == "z4pack"), {})
    day = next((x for x in mo.get("days", []) if x.get("date") == TODAY), {})
    return {
        "шапка": ov.get("revenue_today_pln", 0),
        "сводка": rv.get("total_pln", 0), "сводка: Σ способов": sum(bm.values()),
        "сводка: нал": bm.get("cash", 0), "сводка: пакеты": rv.get("packs_pln", 0),
        "итог": t.get("revenue_pln", 0), "нал+карта+BLIK": t.get("cash_pln", 0) + t.get("card_pln", 0) + t.get("blik_pln", 0),
        "Σ дней": sum(x.get("value", 0) for x in mo.get("days", [])),
        "сегодня": day.get("value", 0), "сегодня: пакеты": day.get("packs_pln", 0),
        "Σ оформлял": sum(x.get("revenue_pln", 0) for x in mo.get("by_admin", [])),
        "оформлял z4pack": me(mo.get("by_admin", [])).get("revenue_pln", 0),
        "оформлял z4pack: пакеты": me(mo.get("by_admin", [])).get("packs_pln", 0),
        "KPI персонала": st.get("totals", {}).get("revenue_pln", 0),
        "Σ смен": sum(x.get("revenue_pln", 0) for x in st.get("by_shift", [])),
        "смены: пакеты": sum(x.get("packs_pln", 0) for x in st.get("by_shift", [])),
        "по людям z4pack": me(st.get("by_person", [])).get("revenue_pln", 0),
    }

def diff(a, b):
    return {k: round(a[k] - b[k], 2) for k in a if not near(a[k], b[k])}

def issue(method):
    s, d = api("POST", f"/admin/users/{g}/packages", {"package_id": PACK, "method": method}, "z4pack")
    pid = d.get("package", {}).get("id")
    if pid:
        at_end(lambda: api("POST", f"/admin/user-packages/{pid}/void", {}, "boss"))
    return s, pid

section("пакет 50 zł налом: +50 везде, где есть выручка")
b = snap()
s, cash_pack = issue("cash")
check("z4pack продал пакет налом → 201", s == 201 and cash_pack, s)
a = snap()
d = diff(a, b)
check("шапка и итог «Денег»: +50", near(d.get("шапка", 0), 50) and near(d.get("итог", 0), 50), J(d))
check("сводка смены: выручка +50, наличные +50, пакеты 50", near(d.get("сводка", 0), 50)
      and near(d.get("сводка: нал", 0), 50) and near(d.get("сводка: пакеты", 0), 50), J(d))
check("ряд по дням: сегодня +50, из них пакеты 50", near(d.get("сегодня", 0), 50) and near(d.get("сегодня: пакеты", 0), 50), J(d))
check("«Кто оформлял»: z4pack 50 zł, из них пакеты 50", near(a["оформлял z4pack"], 50) and near(a["оформлял z4pack: пакеты"], 50),
      J({k: a[k] for k in ("оформлял z4pack", "оформлял z4pack: пакеты")}))
check("«Персонал → по сменам»: пакет в одной из смен", near(d.get("смены: пакеты", 0), 50), J(d))
check("«Персонал → по людям»: z4pack 50 zł", near(a["по людям z4pack"], 50), a["по людям z4pack"])

section("суммы сходятся (сегодня)")
check("сводка = шапка", near(a["сводка"], a["шапка"]), f"{a['сводка']} / {a['шапка']}")
check("в сводке Σ по способам = выручка", near(a["сводка: Σ способов"], a["сводка"]), f"{a['сводка: Σ способов']} / {a['сводка']}")
check("Σ дней = итог", near(a["Σ дней"], a["итог"]), f"{a['Σ дней']} / {a['итог']}")
check("Σ «Кто оформлял» = нал + карта + BLIK = итог", near(a["Σ оформлял"], a["нал+карта+BLIK"])
      and near(a["нал+карта+BLIK"], a["итог"]), f"{a['Σ оформлял']} / {a['нал+карта+BLIK']} / {a['итог']}")
check("Σ смен = KPI «Выручка» в «Персонале»", near(a["Σ смен"], a["KPI персонала"]), f"{a['Σ смен']} / {a['KPI персонала']}")

section("за неделю: те же равенства")
mo = api("GET", "/admin/reports/money?preset=d7", None, "boss")[1]
t = mo.get("totals", {})
paid = t.get("cash_pln", 0) + t.get("card_pln", 0) + t.get("blik_pln", 0)
check("Σ дней = итог", near(sum(x.get("value", 0) for x in mo.get("days", [])), t.get("revenue_pln", -1)),
      f"{sum(x.get('value', 0) for x in mo.get('days', []))} / {t.get('revenue_pln')}")
check("Σ «Кто оформлял» = нал + карта + BLIK = итог", near(sum(x.get("revenue_pln", 0) for x in mo.get("by_admin", [])), paid)
      and near(paid, t.get("revenue_pln", -1)), f"{sum(x.get('revenue_pln', 0) for x in mo.get('by_admin', []))} / {paid} / {t.get('revenue_pln')}")
st = api("GET", "/admin/reports/staff?preset=d7", None, "boss")[1]
check("Σ смен = KPI «Выручка» в «Персонале»", near(sum(x.get("revenue_pln", 0) for x in st.get("by_shift", [])),
      st.get("totals", {}).get("revenue_pln", -1)), J(st.get("totals")))

section("отменённый пакет уходит из всех разрезов")
s, _ = api("POST", f"/admin/user-packages/{cash_pack}/void", {}, "z4pack")
c = snap()
check("z4pack отменил свой пакет → 200; все числа — как до продажи", s == 200 and not diff(c, b), f"{s} {J(diff(c, b))}")

section("пакет с кошелька — не выручка ни в одном разрезе")
dep = topup("Guest35", 50, method="card", who="boss")  # чем платить; само пополнение — выручка, мерим после него
at_end(lambda: api("POST", f"/admin/deposits/{dep}/void", {}, "boss"))
b = snap()
s, _ = issue("wallet")
c = snap()
check("z4pack продал пакет с кошелька → 201; ни одно число не сдвинулось", s == 201 and not diff(c, b), f"{s} {J(diff(c, b))}")
done()
