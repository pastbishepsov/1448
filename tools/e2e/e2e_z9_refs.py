#!/usr/bin/env python3
"""З9-и4 «Справочники и способы оплаты» (№61, №62, №66). Категории: своя — после стандартных, то же имя в другом
регистре — 409 с id, пустое или длиннее 64 — 400; переименовать (в имя другой — 409); закупка — у своей меняется, у
стандартной — 400; скрытая — новому расходу 400 category_hidden, у старого расхода правка без смены категории проходит;
вернули в список — снова можно. Контрагенты: NIP с несходящейся контрольной суммой — 400 bad_nip_sum (и при записи, и
при правке); правка NIP, контактов; имя другого — 409; скрыть и вернуть. Способы оплаты: все четыре включены, выключили
«карту» — новый расход картой 400 method_off, старый картой правится, сменить способ на карту — 400; выключить всё —
400 need_method, неизвестный код — 400. Всё — только владельцу: админу 403, строки журнала не видит.
Категория «z9r Хозтовары» и контрагент «z9r Поставщик» живут между прогонами (удаления нет) — в конце их имена,
NIP и видимость как в начале; расходы с заметкой «z9r проверка» сторнируются; способы оплаты — все включены."""
from _z import *

CAT, CAT2, CP, MARK = "z9r Хозтовары", "z9r Хозтовары и уборка", "z9r Поставщик", "z9r проверка"
cats = lambda: api("GET", "/admin/expense-categories", None, "boss")[1].get("categories", [])
cps = lambda: api("GET", "/admin/counterparties", None, "boss")[1].get("counterparties", [])
std = {c.get("code"): c.get("id") for c in cats() if c.get("code")}
ALL = ["till", "bank", "card", "owner"]


def expense(method="bank", cat=None, **kw):
    body = {"op_date": club_day(0), "category_id": cat or std.get("other"), "amount_pln": 5, "method": method,
            "note": f"{MARK}: {method}", **kw}
    s, d = api("POST", "/admin/expenses", body, "boss")
    return s, (d.get("expense") if s == 201 else d) or {}


def cleanup():
    api("PUT", "/admin/expense-methods", {"on": ALL}, "boss")
    for i in rows(f"SELECT id FROM expenses WHERE note LIKE '{MARK}%' AND storno_at IS NULL"):
        api("POST", f"/admin/expenses/{i}/storno", {"reason": "уборка проверки"}, "boss")
    for c in cats():
        if c.get("name") in (CAT, CAT2):
            api("PATCH", f"/admin/expense-categories/{c['id']}", {"name": CAT, "hidden": False, "purchase": False}, "boss")
    for c in cps():
        if c.get("name") == CP:
            api("PATCH", f"/admin/counterparties/{c['id']}", {"nip": "526-025-09-95", "contact": "", "hidden": False}, "boss")
at_end(cleanup)
cleanup()

section("категории")
s, d = api("POST", "/admin/expense-categories", {"name": CAT}, "boss")
cid = (d.get("category") or {}).get("id") or d.get("id")
mine = next((c for c in cats() if c.get("id") == cid), {})
top = max(c.get("sort", 0) for c in cats() if c.get("code"))
check("своя категория «z9r Хозтовары» → 201 (с прошлого прогона — 409 с id): без кода, после стандартных, не закупка, в списке",
      (s == 201 or (s == 409 and d.get("code") == "category_exists")) and mine and not mine.get("code") and mine.get("sort", 0) > top
      and mine.get("purchase") is False and mine.get("hidden") is False, f"{s} {J(mine)}")
s1, d1 = api("POST", "/admin/expense-categories", {"name": "  z9R хозТОВАРЫ "}, "boss")
s2, d2 = api("POST", "/admin/expense-categories", {"name": " "}, "boss")
s3, d3 = api("POST", "/admin/expense-categories", {"name": "я" * 65}, "boss")
check("то же имя в другом регистре → 409 с id той же; пустое и длиннее 64 → 400 bad_category_name",
      s1 == 409 and d1.get("id") == cid and s2 == 400 and d2.get("code") == "bad_category_name" and s3 == 400 and d3.get("code") == "bad_category_name",
      J([s1, d1.get("id"), s2, s3]))
s, d = api("PATCH", f"/admin/expense-categories/{cid}", {"name": CAT2}, "boss")
s2, d2 = api("PATCH", f"/admin/expense-categories/{cid}", {"name": "аренда"}, "boss")
check("переименовать → 200, имя новое; в имя стандартной «Аренда» → 409 с её id",
      s == 200 and d.get("changed") == 1 and (d.get("category") or {}).get("name") == CAT2 and s2 == 409 and d2.get("id") == std.get("rent"),
      J([s, d.get("category", {}).get("name"), s2, d2.get("id")]))
s, d = api("PATCH", f"/admin/expense-categories/{cid}", {"purchase": True}, "boss")
s2, d2 = api("PATCH", f"/admin/expense-categories/{std.get('rent')}", {"purchase": True}, "boss")
s3, d3 = api("PATCH", f"/admin/expense-categories/{std.get('goods')}", {"purchase": True, "name": "Закупка товаров"}, "boss")
check("закупка: у своей → 200; у стандартной «Аренда» → 400 purchase_fixed; то же значение у стандартной — «изменений нет»",
      s == 200 and (d.get("category") or {}).get("purchase") is True and s2 == 400 and d2.get("code") == "purchase_fixed"
      and s3 == 200 and d3.get("changed") == 0, J([s, s2, d2.get("code"), s3, d3.get("changed")]))
s, old = expense(cat=cid)  # расход в категории до того, как её скрыли
s, d = api("PATCH", f"/admin/expense-categories/{cid}", {"hidden": True}, "boss")
hidden = next((c.get("hidden") for c in cats() if c.get("id") == cid), None)
s2, d2 = expense(cat=cid)
s3, d3 = api("PATCH", f"/admin/expenses/{old.get('id')}", {"note": f"{MARK}: правка после скрытия"}, "boss")
s4, e4 = expense()
s5, d5 = api("PATCH", f"/admin/expenses/{e4.get('id')}", {"category_id": cid}, "boss")
check("скрыть → 200, в списке hidden; новый расход в ней → 400 category_hidden; старый в ней правится; перенести в неё другой → 400",
      s == 200 and hidden is True and s2 == 400 and d2.get("code") == "category_hidden" and s3 == 200 and d3.get("changed") == 1
      and s5 == 400 and d5.get("code") == "category_hidden", J([s, hidden, s2, d2.get("code"), s3, s5, d5.get("code")]))
s, d = api("PATCH", f"/admin/expense-categories/{cid}", {"hidden": False}, "boss")
s2, _ = expense(cat=cid)
check("вернуть в список → новый расход в ней снова 201", s == 200 and s2 == 201, J([s, s2]))

section("контрагенты")
s, d = api("POST", "/admin/counterparties", {"name": "z9r Кривой NIP", "nip": "123-456-78-90"}, "boss")
check("NIP 123-456-78-90 (сумма не сходится) → 400 bad_nip_sum", s == 400 and d.get("code") == "bad_nip_sum", J(d))
s, d = api("POST", "/admin/counterparties", {"name": CP, "nip": "526-025-09-95", "contact": "zamowienia@example.pl"}, "boss")
pid = (d.get("counterparty") or {}).get("id") or d.get("id")
check("«z9r Поставщик» с NIP 526-025-09-95 → 201 (с прошлого прогона — 409 с id)", (s == 201 and d["counterparty"].get("nip") == "5260250995")
      or (s == 409 and d.get("code") == "counterparty_exists" and pid), f"{s} {J(d)}")
s, d = api("PATCH", f"/admin/counterparties/{pid}", {"nip": "1234567890"}, "boss")
s2, d2 = api("PATCH", f"/admin/counterparties/{pid}", {"nip": "521-301-72-28", "contact": "+48 22 000 00 00"}, "boss")
s3, d3 = api("PATCH", f"/admin/counterparties/{pid}", {"name": "Z9 АРЕНДОДАТЕЛЬ"}, "boss")
check("правка: NIP с несходящейся суммой → 400; NIP и контакты → 200 (2 изменения); имя другого контрагента → 409",
      s == 400 and d.get("code") == "bad_nip_sum" and s2 == 200 and d2.get("changed") == 2 and (d2.get("counterparty") or {}).get("nip") == "5213017228"
      and s3 == 409 and d3.get("code") == "counterparty_exists", J([s, d.get("code"), s2, d2.get("changed"), s3]))
s, d = api("PATCH", f"/admin/counterparties/{pid}", {"hidden": True}, "boss")
h = next((c.get("hidden") for c in cps() if c.get("id") == pid), None)
s2, _ = api("PATCH", f"/admin/counterparties/{pid}", {"hidden": False}, "boss")
check("скрыть → hidden в списке; вернуть → 200", s == 200 and h is True and s2 == 200, J([s, h, s2]))
jr = [x.get("text", "") for x in audit("boss", limit=100) if CP in (x.get("text") or "")]
check("журнал: правка контрагента — «NIP 5260250995 → 5213017228; контакты изменены»",
      any("NIP 5260250995 → 5213017228; контакты изменены" in t for t in jr), J(jr[:3]))

section("способы оплаты")
m = api("GET", "/admin/expense-methods", None, "boss")[1].get("methods", [])
check("все четыре включены: из кассы, банк, карта, личные деньги владельца", [x.get("code") for x in m] == ALL and all(x.get("on") for x in m),
      J(m))
s, cardx = expense("card")  # картой — до того, как выключили
s, d = api("PUT", "/admin/expense-methods", {"on": ["till", "bank", "owner"]}, "boss")
off = [x.get("code") for x in d.get("methods", []) if not x.get("on")]
s2, d2 = expense("card")
s3, d3 = api("PATCH", f"/admin/expenses/{cardx.get('id')}", {"note": f"{MARK}: карта после выключения"}, "boss")
s4, e4 = expense("bank")
s5, d5 = api("PATCH", f"/admin/expenses/{e4.get('id')}", {"method": "card"}, "boss")
check("выключили «карту» → 200; новый расход картой → 400 method_off; старый картой правится; сменить способ на карту → 400",
      s == 200 and off == ["card"] and s2 == 400 and d2.get("code") == "method_off" and s3 == 200 and s5 == 400 and d5.get("code") == "method_off",
      J([s, off, s2, d2.get("code"), s3, s5]))
bad = [api("PUT", "/admin/expense-methods", b, "boss") for b in ({"on": []}, {"on": ["cash"]}, {})]
check("выключить всё → 400 need_method; неизвестный код → 400 bad_method; без списка → 400",
      [(x[0], x[1].get("code")) for x in bad] == [(400, "need_method"), (400, "bad_method"), (400, "invalid_request")], J([(x[0], x[1].get("code")) for x in bad]))
s, d = api("PUT", "/admin/expense-methods", {"on": ALL}, "boss")
check("включили всё обратно → «карта» снова годится", s == 200 and expense("card")[0] == 201, str(s))

section("только владельцу")
st = [api("POST", "/admin/expense-categories", {"name": "z9r админ"}, "admin1")[0],
      api("PATCH", f"/admin/expense-categories/{cid}", {"hidden": True}, "admin1")[0],
      api("PATCH", f"/admin/counterparties/{pid}", {"hidden": True}, "admin1")[0],
      api("GET", "/admin/expense-methods", None, "admin1")[0], api("PUT", "/admin/expense-methods", {"on": ["bank"]}, "admin1")[0]]
seen = [x.get("text", "") for x in audit("admin1") if "z9r" in (x.get("text") or "") or "способы оплаты" in (x.get("text") or "")]
check("админу — 403 на все ручки справочников; строки журнала о них не видит", st == [403] * 5 and not seen, J({"статусы": st, "видит": seen[:2]}))
done()
