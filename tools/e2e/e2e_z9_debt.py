#!/usr/bin/env python3
"""З9-и3б «Долг клуба владельцу» (№62): расход способом «личные деньги владельца» — клуб потратил деньги владельца и
должен их вернуть. Долг = оплаченные расходы личными без сторно минус возвраты без отменённых, за всё время. Возврат
с банка ящик не трогает; из кассы — «должно быть в ящике» меньше с минуты записи: в предпросмотре сдачи (его видит и
админ) — «владельцу», у сдачи — своей строкой в «ушло», в кассовой книге — сумма в строке дня и сам возврат в её
раскрытии, строка сходится. Больше долга — 409 over_debt с остатком долга; кривые сумма, способ, заметка — 400.
Ошибочный возврат отменяется: строка остаётся с пометкой, долг и ящик возвращаются; повторно — 409. Всё — только
владельцу: у admin три ручки → 403, строк журнала о возвратах он не видит.
Временный z9d на смене; он сдаёт кассу без расхождения. Возвраты и расходы с заметкой «z9d проверка …» (у прогона —
своя метка) в конце отменяются и сторнируются — долг как был; возврат из кассы 70 zł, который учла сдача z9d, после
отмены «из ящика не уходил» — z9d пересчитывает кассу с излишком +70: ящик как до проверки, расхождение дня в кассовой
книге — 0. z9d уволен."""
import time
from _z import *

MARK = "z9d проверка"
TAG = f"{MARK} {int(time.time()) % 100000:05d}"  # строки этого прогона; уборка — по MARK, всех прогонов
hire_temp("z9d")
tok("z9d")  # вход в админку открывает смену
drawer_ready("z9d")
cats = {c.get("code"): c.get("id") for c in api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", [])}
preview = lambda: api("GET", "/admin/work/closeout", None, "z9d")[1]
exp_ = lambda: preview().get("expected_pln", 0)
debt = lambda: api("GET", "/admin/owner-debt", None, "boss")[1]
D = lambda: debt().get("owner_debt", {}).get("debt_pln", 0)
book = lambda: api("GET", "/admin/cash/book?preset=today", None, "boss")[1]["days"][-1]


covered = [0]  # возврат из кассы, который учла сдача z9d (zł)


def cleanup():
    for i in rows(f"SELECT id FROM owner_repayments WHERE note LIKE '{MARK}%' AND voided_at IS NULL"):
        api("POST", f"/admin/owner-repayments/{i}/void", None, "boss")
    if covered[0]:  # отменённый возврат, учтённый сдачей, «из ящика не уходил»: пересчёт с излишком возвращает ящику
        x = preview().get("expected_pln", 0)  # прежний уровень, расхождение дня в кассовой книге — 0
        api("POST", "/admin/work/stop", {"counted_pln": round(x + covered[0], 2)}, "z9d")
        api("POST", "/admin/work/start", {}, "z9d")
        covered[0] = 0
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
at_end(cleanup)
cleanup()


def expense(amount, method="owner", **kw):
    body = {"op_date": club_day(0), "category_id": cats.get("equipment") or cats.get("other"), "amount_pln": amount,
            "method": method, "note": f"{TAG}: {amount}", **kw}
    s, d = api("POST", "/admin/expenses", body, "boss")
    return s, d


def repay(amount, method, note=None, who="boss"):
    return api("POST", "/admin/owner-repayments", {"amount_pln": amount, "method": method,
               "note": note if note is not None else f"{TAG}: {amount} {method}"}, who)


def adds_up(r):
    if r.get("opening_pln") is None:
        return True
    return near(r["opening_pln"] + r["in_pln"] - r["out_pln"] + r["adjust_pln"] + r["diff_pln"], r["closing_pln"])


section("только владельцу")
some = sql("SELECT id FROM owner_repayments LIMIT 1") or "00000000-0000-0000-0000-000000000000"
codes = [api("GET", "/admin/owner-debt", None, "z9d")[0], repay(1, "bank", who="z9d")[0],
         api("POST", f"/admin/owner-repayments/{some}/void", None, "z9d")[0]]
check("админ z9d: долг, вернуть, отменить возврат — 403", codes == [403] * 3, J(codes))

section("долг растёт с расходом «личными»")
d0 = D()
s, d = expense(300)
e1 = d.get("expense", {})
check("расход 300 zł «личные деньги владельца» → 201, в ответе — клуб должен владельцу на 300 больше",
      s == 201 and near(d.get("owner_debt", {}).get("debt_pln", 0), d0 + 300) and near(D(), d0 + 300), f"{s} {d0} → {J(d.get('owner_debt'))}")
s, d = expense(50, paid=False)
e2 = d.get("expense", {})
unpaid = D()
s2, _ = api("PATCH", f"/admin/expenses/{e2.get('id')}", {"paid": True}, "boss")
paid = D()
s3, d3 = api("POST", f"/admin/expenses/{e2.get('id')}/storno", {"reason": "проверка сторно"}, "boss")
check("не оплачен — долг не растёт; отметили оплату — +50; сторно — −50 (в ответе сторно — долг)",
      s == 201 and near(unpaid, d0 + 300) and s2 == 200 and near(paid, d0 + 350) and s3 == 200
      and near(d3.get("owner_debt", {}).get("debt_pln", 0), d0 + 300), f"{unpaid} → {paid} → {J(d3.get('owner_debt'))}")
s, _ = expense(40, method="bank")
lst = api("GET", "/admin/expenses?preset=today", None, "boss")[1]
check("расход банком — долг не меняется; список расходов отдаёт тот же долг",
      s == 201 and near(D(), d0 + 300) and near(lst.get("owner_debt", {}).get("debt_pln", 0), d0 + 300), J(lst.get("owner_debt")))

section("возврат с банка — долг меньше, ящик не трогает")
x0 = exp_()
s, d = repay(100, "bank")
r1 = d.get("repayment", {})
check("вернули 100 zł с банка → 201: долг −100, «должно быть в ящике» как было",
      s == 201 and r1.get("method") == "bank" and near(r1.get("amount_pln", 0), 100) and r1.get("nickname") == "boss"
      and near(d.get("owner_debt", {}).get("debt_pln", 0), d0 + 200) and near(d.get("expected_pln", -1), x0) and near(exp_(), x0),
      f"{s} {J(d, 400)}")
h = debt()
check("«Долг владельцу»: итог — потрачено и возвращено, свежий возврат сверху",
      near(h.get("owner_debt", {}).get("debt_pln", 0), d0 + 200) and (h.get("repayments") or [{}])[0].get("id") == r1.get("id")
      and near(h["owner_debt"]["spent_pln"] - h["owner_debt"]["repaid_pln"], h["owner_debt"]["debt_pln"]), J(h.get("owner_debt")))

section("возврат из кассы — «должно быть в ящике» меньше")
p0, b0 = preview(), book()
s, d = repay(70, "till")
r2 = d.get("repayment", {})
p1, b1 = preview(), book()
check("вернули 70 zł из кассы → 201: долг −70, «должно быть» −70 (в ответе и в предпросмотре сдачи)",
      s == 201 and near(d.get("owner_debt", {}).get("debt_pln", 0), d0 + 130) and near(d.get("expected_pln", 0), p0.get("expected_pln", 0) - 70)
      and near(p1.get("expected_pln", 0), p0.get("expected_pln", 0) - 70), f"{p0.get('expected_pln')} → {p1.get('expected_pln')}")
check("предпросмотр сдачи (админ): «владельцу» +70, «ушло» +70, инкассации и расходы из кассы — как были",
      near(p1.get("owner_pln", 0) - p0.get("owner_pln", 0), 70) and near(p1.get("out_pln", 0) - p0.get("out_pln", 0), 70)
      and near(p1.get("collections_pln", 0), p0.get("collections_pln", 0)) and near(p1.get("expenses_pln", 0), p0.get("expenses_pln", 0)),
      J({k: p1.get(k) for k in ("owner_pln", "out_pln", "collections_pln", "expenses_pln")}))
mine = [x for x in b1.get("owner", []) if x.get("id") == r2.get("id")]
check("кассовая книга, сегодня: «владельцу» +70, возврат — в раскрытии дня (кто, сумма), банковского там нет, строка сходится",
      near(b1.get("owner_pln", 0) - b0.get("owner_pln", 0), 70) and mine and mine[0].get("nickname") == "boss" and near(mine[0].get("amount_pln", 0), 70)
      and not any(x.get("id") == r1.get("id") for x in b1.get("owner", [])) and adds_up(b1) and near(b1.get("closing_pln", -1), p1.get("expected_pln", -2)),
      J({k: b1.get(k) for k in ("owner_pln", "out_pln", "closing_pln")}))

section("отмена возврата — долг и ящик возвращаются")
s, d = repay(30, "till")
r3 = d.get("repayment", {})
mid, xm = D(), exp_()
s1, v = api("POST", f"/admin/owner-repayments/{r3.get('id')}/void", None, "boss")
row = next((x for x in debt().get("repayments", []) if x.get("id") == r3.get("id")), {})
check("вернули 30 из кассы и отменили: долг и «должно быть» — снова как до возврата; строка осталась — кто и когда отменил",
      s == 201 and near(mid, d0 + 100) and near(xm, p1.get("expected_pln", 0) - 30) and s1 == 200
      and near(v.get("owner_debt", {}).get("debt_pln", 0), d0 + 130) and near(v.get("expected_pln", 0), p1.get("expected_pln", 0))
      and row.get("voided_at") and row.get("voided_by_nickname") == "boss", f"{mid} → {J(v.get('owner_debt'))} · {J(row)}")
b2 = book()
gone = next((x for x in b2.get("owner", []) if x.get("id") == r3.get("id")), {})
check("кассовая книга: отменённый — в раскрытии с пометкой, в сумму дня не входит",
      gone.get("voided_at") and near(b2.get("owner_pln", 0), b1.get("owner_pln", 0)) and adds_up(b2), J(gone))
s2, d2 = api("POST", f"/admin/owner-repayments/{r3.get('id')}/void", None, "boss")
s3, d3 = api("POST", "/admin/owner-repayments/00000000-0000-0000-0000-000000000000/void", None, "boss")
check("повторная отмена — 409 already_void; чужой id — 404 repayment_not_found",
      (s2, d2.get("code"), s3, d3.get("code")) == (409, "already_void", 404, "repayment_not_found"), J([s2, d2, s3, d3]))

section("больше долга — нельзя; кривое — 400")
left = D()
n0 = sql(f"SELECT count(*) FROM owner_repayments WHERE note LIKE '{MARK}%'")
s, d = repay(round(left + 0.01, 2), "bank")
check(f"вернуть на 0,01 больше долга ({left:.2f}) → 409 over_debt, в ответе — остаток долга и сумма словами; ничего не записано",
      s == 409 and d.get("code") == "over_debt" and near(d.get("owner_debt", {}).get("debt_pln", 0), left)
      and f"{left:.2f} zł" in d.get("message", "") and sql(f"SELECT count(*) FROM owner_repayments WHERE note LIKE '{MARK}%'") == n0,
      f"{s} {J(d)}")
bad = [repay(0, "bank"), repay(1.005, "bank"), repay(-5, "till"), repay(10, "card"), repay(10, ""), repay(10, "till", "я" * 201),
       api("POST", "/admin/owner-repayments", {"method": "till"}, "boss")]
got = [(x[0], x[1].get("code")) for x in bad]
want = [(400, "bad_amount")] * 3 + [(400, "bad_method")] * 2 + [(400, "bad_note"), (400, "bad_amount")]
check("0, три знака, минус → bad_amount; карта и пусто → bad_method; заметка 201 символ → bad_note; без суммы → bad_amount",
      got == want, J(got))

section("сдача — «владельцу» своей строкой в «ушло»")
pre = preview()
s, st = api("POST", "/admin/work/stop", {"counted_pln": pre.get("expected_pln", 0)}, "z9d")
api("POST", "/admin/work/start", {}, "z9d")
co = (st or {}).get("closeout") or {}
if s == 200 and r2.get("id") and co.get("owner_pln", 0) >= 70:  # сдача учла возврат 70 из кассы — уборке его вернуть
    covered[0] = 70
check("z9d сдал без расхождения: у сдачи «владельцу» — как в предпросмотре, «ушло» его включает, инкассации — без него",
      s == 200 and co.get("diff_pln") == 0 and near(co.get("owner_pln", -1), pre.get("owner_pln", -2))
      and near(co.get("out_pln", 0), pre.get("out_pln", 0)) and near(co.get("collections_pln", -1), pre.get("collections_pln", -2)) and pre.get("owner_pln", 0) >= 70,
      J({k: co.get(k) for k in ("owner_pln", "out_pln", "collections_pln", "expenses_pln", "diff_pln")}))
og = sql(f"SELECT owner_grosz FROM shift_closeouts WHERE id='{co.get('id')}'") if co.get("id") else ""
check("в базе у сдачи — owner_grosz, в грошах", pre.get("owner_pln", 0) >= 70 and og == str(round(pre["owner_pln"] * 100)), og)

section("журнал — только владельцу")
kinds = "owner_repay,owner_repay_void"
own = audit(kind=kinds, limit=10)
adm = audit("z9d", kind=kinds, limit=10)
texts = [i.get("text", "") for i in own]
check("владелец видит возвраты и отмену: «100.00 zł с банка · …», «30.00 zł из кассы · … (внесён ДД.ММ ЧЧ:ММ)»; админ — ни строки",
      any(t.startswith(f"100.00 zł с банка · {TAG}") for t in texts) and any(i.get("kind") == "owner_repay_void"
      and i.get("text", "").startswith(f"30.00 zł из кассы · {TAG}") and "(внесён " in i.get("text", "") for i in own) and adm == [],
      J({"владелец": texts[:4], "админ": len(adm)}, 500))
done()
