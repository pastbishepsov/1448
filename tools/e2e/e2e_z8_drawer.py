#!/usr/bin/env python3
"""З8-и1 «Модель ящика» (З-Р1; ревью 23.09 №1, №16): касса считает ящик клуба, а не человека. «Должно быть в
ящике» = остаток прошлой сдачи (пересчитанный, если пересчитывали) + все наличные операции клуба с неё −
наличные возвраты (− инкассации ± коррекции владельца — их ручки в З8-и2). Первая сдача после обновления —
нулевая точка (решение основателя 26.09): пересчитанное становится остатком на начало без расхождения,
владельцу — строка «Ящик: начальный остаток». Сценарии A ревью — ложный излишек (депозит владельца в смену
админа) и ложная недостача (вторая сдача за сутки) — не воспроизводятся; два админа подряд — второй начинает с
пересчитанного первым; отмена наличного пакета уменьшает ожидаемое.
Стенд без истории ящика изображается на время проверки: у прошлых сдач прячется остаток на начало, в конце —
возвращается. Временные сотрудники z8box и z8two, гость Guest39. Идемпотентен: пополнения и пакет отменяет,
z8box и z8two уволены."""
from _z import *

g39 = uid("Guest39")
hire_temp("z8box")
hire_temp("z8two")
deps = []
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in reversed(deps)])

def cash(pln, who):
    deps.append(topup("Guest39", pln, method="cash", who=who))

def stop(who, counted=None):
    return api("POST", "/admin/work/stop", {} if counted is None else {"counted_pln": counted}, who)

preview = lambda who: api("GET", "/admin/work/closeout", None, who)[1]
start = lambda who: api("POST", "/admin/work/start", {}, who)
chat = lambda: api("GET", "/admin/staff-chat", None, "boss")[1].get("messages", [])
fresh = lambda seen: [m for m in chat() if m["id"] not in seen]
owner_lines = lambda seen: [m["text"] for m in fresh(seen) if m.get("kind") == "system" and m.get("visible_to") == "owner"]

# стенд без истории ящика: у прошлых сдач прячется остаток на начало — в конце возвращается
has = sql("SELECT count(*) FROM information_schema.columns WHERE table_name='shift_closeouts' "
          "AND column_name='opening_grosz'") == "1"
check("у сдачи есть «остаток на начало» (миграция 070)", has)
if has:
    hidden = [r for r in rows("WITH old AS (SELECT id, opening_grosz FROM shift_closeouts WHERE opening_grosz IS NOT NULL) "
                              "UPDATE shift_closeouts s SET opening_grosz = NULL FROM old WHERE s.id = old.id "
                              "RETURNING s.id || '|' || old.opening_grosz") if "|" in r]

    def unhide():
        if hidden:
            vals = ",".join(f"('{i}'::uuid, {v}::bigint)" for i, v in (h.split("|") for h in hidden))
            sql(f"UPDATE shift_closeouts s SET opening_grosz = v.o FROM (VALUES {vals}) AS v(id, o) WHERE s.id = v.id")
    at_end(unhide)

section("нулевая точка: первая сдача после обновления")
tok("z8box")  # вход в админку открывает смену
p = preview("z8box")
check("истории ящика нет: предпросмотр — «первая сдача», ожидаемого нет", p.get("baseline") is True, J(p, 300))
seen = {m["id"] for m in chat()}
s, d = stop("z8box", 350)
co = d.get("closeout", {})
check("z8box пересчитал весь ящик — 350: это начальный остаток, ожидалось = пересчитано, расхождения нет",
      s == 200 and co.get("baseline") is True and near(co.get("opening_pln", -1), 350)
      and near(co.get("expected_pln", -1), 350) and near(co.get("diff_pln", -1), 0), f"{s} {J(co, 300)}")
lines = owner_lines(seen)
check("владельцу — строка «Ящик: начальный остаток 350.00 zł, пересчитал z8box»",
      len(lines) == 1 and "Ящик: начальный остаток 350.00 zł" in lines[0] and "z8box" in lines[0], J(lines))

section("депозит владельца в смену админа — в «должно быть» (ложный излишек ревью)")
start("z8box")
p0 = preview("z8box")
check("на начало — 350, пересчитал z8box; должно быть 350", p0.get("baseline") is False and near(p0.get("opening_pln", -1), 350)
      and p0.get("opening_by") == "z8box" and p0.get("opening_counted") is True and near(p0.get("expected_pln", -1), 350), J(p0, 300))
cash(30, "z8box")
cash(70, "boss")
p1 = preview("z8box")
check("пополнения 30 от z8box и 70 от владельца: должно быть 450 = 350 + 100, пришло 100",
      near(p1.get("expected_pln", -1), 450) and near(p1.get("in_pln", -1), 100) and near(p1.get("deposits_pln", -1), 100), J(p1, 300))
check("«из них провёл ты» — 30: справка, в «должно быть» — весь ящик", near((p1.get("mine") or {}).get("in_pln", -1), 30), J(p1.get("mine")))
seen = {m["id"] for m in chat()}
s, d = stop("z8box", 450)
check("сдал 450 → касса сошлась", s == 200 and near(d.get("closeout", {}).get("diff_pln", -1), 0), J(d.get("closeout"), 200))
check("владельцу — «касса сошлась — в ящике 450.00 zł»", any("касса сошлась — в ящике 450.00 zł" in t for t in owner_lines(seen)))

section("вторая сдача за сутки — от пересчитанного первой (ложная недостача ревью)")
start("z8box")
cash(30, "z8box")
p2 = preview("z8box")
check("на начало — 450 (пересчитано прошлой сдачей), плюс 30: должно быть 480", near(p2.get("opening_pln", -1), 450)
      and near(p2.get("expected_pln", -1), 480), J(p2, 300))
s, d = stop("z8box", 480)
check("сдал 480 → сошлось", s == 200 and near(d.get("closeout", {}).get("diff_pln", -1), 0), J(d.get("closeout"), 200))

section("два админа подряд: второй начинает с пересчитанного первым")
tok("z8two")
p3 = preview("z8two")
check("z8two: на начало 480 — пересчитал z8box; должно быть 480", near(p3.get("opening_pln", -1), 480)
      and p3.get("opening_by") == "z8box" and near(p3.get("expected_pln", -1), 480), J(p3, 300))
seen = {m["id"] for m in chat()}
s, d = stop("z8two", 470)
check("z8two насчитал 470 из 480 → недостача −10", s == 200 and near(d.get("closeout", {}).get("diff_pln", 0), -10), J(d.get("closeout"), 200))
check("владельцу — «расхождение −10.00 zł»", any("расхождение −10.00 zł" in t for t in owner_lines(seen)))
start("z8box")
p4 = preview("z8box")
check("после недостачи на начало — пересчитанное (470), а не ожидавшееся (480)", near(p4.get("opening_pln", -1), 470)
      and p4.get("opening_by") == "z8two" and near(p4.get("expected_pln", -1), 470), J(p4, 300))

section("отмена наличного пакета уменьшает ожидаемое")
pack = sql("SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1")
s, d = api("POST", f"/admin/users/{g39}/packages", {"package_id": pack, "method": "cash"}, "z8box")
pid, price = d.get("package", {}).get("id"), d.get("package", {}).get("price_pln", 0)
if pid:
    at_end(lambda: api("POST", f"/admin/user-packages/{pid}/void", {}, "boss"))
p5 = preview("z8box")
check(f"z8box продал пакет налом ({price} zł) → должно быть +{price}", s == 201 and near(p5.get("expected_pln", -1) - p4.get("expected_pln", 0), price)
      and near(p5.get("packs_pln", -1) - p4.get("packs_pln", 0), price), J(p5, 300))
s, v = api("POST", f"/admin/user-packages/{pid}/void", {}, "z8box")
check("отмена пакета, время не тронуто → вернуть гостю наличными всю цену", s == 200 and near(v.get("refund_pln", -1), price)
      and v.get("refund_method") == "cash", f"{s} {J(v, 200)}")
p6 = preview("z8box")
check("должно быть — как до продажи, возвраты налом — +цена пакета", near(p6.get("expected_pln", -1), p4.get("expected_pln", -2))
      and near(p6.get("refunds_pln", -1) - p5.get("refunds_pln", 0), price), J(p6, 300))

section("уход без пересчёта: следующий начинает с ожидаемого")
s, d = stop("z8box")
co = d.get("closeout", {})
check("z8box ушёл без пересчёта → skipped, должно быть 470", s == 200 and co.get("skipped") is True
      and near(co.get("expected_pln", -1), 470), J(co, 200))
start("z8two")
p7 = preview("z8two")
check("z8two: на начало 470 — по расчёту (без пересчёта), пересчитано не было", near(p7.get("opening_pln", -1), 470)
      and p7.get("opening_by") == "z8box" and p7.get("opening_counted") is False, J(p7, 300))
done()
