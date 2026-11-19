#!/usr/bin/env python3
"""З19-и17 «Страницы журнала без потерь» (решение основателя 17.11, «Найдено по дороге» З19-и16). GET /admin/audit
страницами по 5 строк (limit=5, следующая — before = next_before), пока «есть ещё»: каждая своя строка — ровно один раз
и по порядку, «есть ещё» честное. Строки проверки — SQL-ом, с пометкой «z19au», за последнюю минуту:
- «удержания» A1–A8 (admin_actions, pc_unhold) раз в 10 с и две смены настроек H1, H2 между ними (settings_update —
  видит только владелец);
- начисления G1–G5 гостю Guest48 (admin_grants) — старше A4;
- чеки R1–R4 по три продажи картой (sales с одним receipt_id) — 10, 20, 30, 40 с назад;
- «придержания» B1–B7 (pc_hold): B4–B6 — одним моментом (одной вставкой), как оплата долга за несколько товаров или
  отмена чека.
Один источник (только «удержания»): на HEAD «есть ещё» нет — видно 5 из 8. Админ, «удержания» и настройки: на HEAD
страница — 3 строки, остальных не видно. Админ, «удержания», настройки и начисления: на HEAD A4 нет ни на одной
странице. Чеки и начисления: на HEAD чек R2 — неполный, R3 и R4 не видно. Один момент на границе страницы
(«придержания» и начисления): на HEAD из B4–B6 на первой странице две, третьей нет нигде. Владелец видит и H1, H2. В
начале и в конце
строки «z19au» удаляются — журнал, касса и отчёты стенда как были."""
from _z import *

fresh_day(2)  # строки проверки — за последнюю минуту; сутки, которые идут меньше, — граница на время раньше
DAY = club_day()
ADM, G48 = uid("admin1"), uid("Guest48")
CLUB = sql("SELECT id FROM clubs ORDER BY created_at LIMIT 1")


def wipe():
    sql("DELETE FROM admin_actions WHERE details LIKE 'z19au %'")
    sql("DELETE FROM admin_grants WHERE reason LIKE 'z19au %'")
    sql("DELETE FROM sales WHERE name LIKE 'z19au %'")


wipe()
at_end(wipe)

sql(f"INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '{ADM}', 'pc_unhold', 'z19au A' || i, "
    f"now() - (10 * i || ' seconds')::interval FROM generate_series(1, 8) i")
sql(f"INSERT INTO admin_actions(admin_id, action, details, created_at) VALUES "
    f"('{ADM}', 'settings_update', 'z19au H1', now() - interval '5 seconds'), "
    f"('{ADM}', 'settings_update', 'z19au H2', now() - interval '15 seconds')")
sql(f"INSERT INTO admin_grants(id, user_id, admin_id, grant_type, amount, reason, created_at) SELECT gen_random_uuid(), "
    f"'{G48}', '{ADM}', 'case', 1, 'z19au G' || i, now() - ((40 + i) || ' seconds')::interval FROM generate_series(1, 5) i")
for r in range(1, 5):
    rid = sql("SELECT gen_random_uuid()")
    for k, ch in enumerate("abc"):
        sql(f"INSERT INTO sales(club_id, name, qty, price_pln, total_pln, method, created_by, receipt_id, created_at) VALUES "
            f"('{CLUB}', 'z19au R{r}{ch}', 1, 1, 1, 'card', '{ADM}', '{rid}', "
            f"now() - interval '{10 * r} seconds' - interval '{k} milliseconds')")

sql(f"INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '{ADM}', 'pc_hold', 'z19au B' || i, "
    f"now() - (i || ' seconds')::interval FROM generate_series(1, 3) i")
sql(f"INSERT INTO admin_actions(admin_id, action, details, created_at) SELECT '{ADM}', 'pc_hold', 'z19au B' || i, "
    f"now() - interval '4 seconds' FROM generate_series(4, 6) i")  # одна вставка — один момент
sql(f"INSERT INTO admin_actions(admin_id, action, details, created_at) VALUES "
    f"('{ADM}', 'pc_hold', 'z19au B7', now() - interval '5 seconds')")

A = [f"A{i}" for i in range(1, 9)]
B = [f"B{i}" for i in range(1, 8)]
G = [f"G{i}" for i in range(1, 6)]
R = [f"R{r}" for r in range(1, 5)]
H = ["H1", "H2"]


def mine(it):
    """Свои строки строки журнала: «A3», «G2», «H1» или чек «R2» со своими продажами ['R2a', 'R2b', 'R2c']."""
    t = it.get("text") or ""
    if "z19au " not in t:
        return None, None
    parts = [p.split(" ")[0].split(",")[0] for p in t.split("z19au ")[1:]]
    if it.get("kind") == "sale":
        return parts[0][:2], sorted(parts)
    return parts[0], None


def pages(who, kinds, owner_day=False):
    """Все страницы по 5 строк: [(count, has_more, [свои строки страницы])], чеки — {R: продажи}."""
    q = f"/admin/audit?kind={kinds}&limit=5" + (f"&day={DAY}" if owner_day else "")
    out, before, lines = [], None, {}
    for _ in range(12):
        s, d = api("GET", q + (f"&before={before}" if before else ""), None, who)
        if s != 200:
            out.append((s, False, [J(d)]))
            break
        got = []
        for it in d.get("items") or []:
            key, sold = mine(it)
            if key:
                got.append(key)
                if sold:
                    lines.setdefault(key, []).extend(sold)
        out.append((d.get("count"), d.get("has_more"), got))
        if not d.get("has_more"):
            break
        before = d.get("next_before")
        if not before:
            break
        before = before.replace("+", "%2B")
    return out, lines


def seen(ps):
    return [k for _, _, got in ps for k in got]


section("один источник: строк больше страницы только у «удержаний»")
for who, own in (("boss", True), ("admin1", False)):
    ps, _ = pages(who, "pc_unhold", own)
    check(f"{who}: «удержания», страница — 5: первая — 5 строк и «есть ещё»", ps[0][0] == 5 and ps[0][1] is True, J(ps))
    check(f"{who}: листая, все восемь «удержаний» — по разу и по порядку (на HEAD — 5 из 8, «есть ещё» нет)",
          seen(ps) == A, J(ps))

section("админ: владельческие строки отсеиваются до лимита")
ps, _ = pages("admin1", "pc_unhold,settings_update")
check("админ, «удержания» и настройки: первая страница — 5 строк и «есть ещё» (на HEAD — 3 строки, «есть ещё» нет)",
      ps[0][0] == 5 and ps[0][1] is True, J(ps))
check("админ: все восемь «удержаний» по разу и по порядку, смен настроек нет", seen(ps) == A, J(ps))
ps, _ = pages("admin1", "pc_unhold,settings_update,grant")
got = seen(ps)
check("админ, «удержания», настройки и начисления: каждая своя строка — по разу, A4 на месте (на HEAD A4 нет ни на одной "
      "странице), смен настроек нет", sorted(got) == sorted(A + G) and "A4" in got and not set(got) & set(H), J(ps))
check("админ: строки по времени — новее раньше", got == ["A1", "A2", "A3", "A4", "G1", "G2", "G3", "G4", "G5", "A5", "A6",
      "A7", "A8"], J(got))
check("админ: все страницы, кроме последней, — полные (5 строк)", all(c == 5 for c, _, _ in ps[:-1]), J(ps))
ps, _ = pages("boss", "pc_unhold,settings_update,grant", True)
got = seen(ps)
check("владелец: все 15 своих строк по разу, с обеими сменами настроек", sorted(got) == sorted(A + G + H), J(ps))

section("чеки: лимит — на строки журнала")
ps, lines = pages("boss", "sale,grant", True)
got = seen(ps)
check("чеки и начисления, страница — 5: каждый чек и начисление — по разу (на HEAD R3 и R4 не видно)",
      sorted(got) == sorted(R + G), J(ps))
check("каждый чек — целиком, три продажи (на HEAD R2 — неполный)",
      all(lines.get(r) == [f"{r}a", f"{r}b", f"{r}c"] for r in R), J(lines))
check("порядок: R1–R4, потом начисления", got == R + G, J(got))
ps, _ = pages("admin1", "sale")
check("админ, только чеки: четыре чека по разу", seen(ps) == R, J(ps))

section("один момент на границе страницы")
ps, _ = pages("boss", "pc_hold,grant", True)
check("B4–B6 одним моментом на границе страницы: все семь «придержаний» и пять начислений — по разу (на HEAD одной из "
      "B4–B6 нет нигде)", sorted(seen(ps)) == sorted(B + G), J(ps))
check("страница перед этим моментом — короче (B1–B3) и «есть ещё»; следующая начинается с B4–B6 целиком",
      ps[0][2] == B[:3] and ps[0][1] is True and len(ps) > 1 and sorted(ps[1][2][:3]) == ["B4", "B5", "B6"], J(ps))

done()
