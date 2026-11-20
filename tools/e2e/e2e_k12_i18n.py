#!/usr/bin/env python3
"""К12-и1 «Сервер: переводы и отдача по языку гостя» (KIOSK.md, этап VII; миграция 103).
У ачивок и талантов польские и английские поля рядом с русскими. Гость получает текст на своём языке
(users.language) в GET /me/achievements, GET /me/talents и в уведомлении «ачивка получена»; пустой перевод
или одни пробелы — русский. Русские поля не меняются: их по-прежнему видят админка и владелец.
На время проверки пишет: переводы ачивки hour_10 и таланта xp_boost, язык Guest46, две временные ачивки
e2e_k12_n1 (с переводами) и e2e_k12_n2 (без) — их выдаёт правка профиля; на миг выдачи прочие ачивки
выключены, чтобы не выдалась настоящая. В конце всё возвращает, в том числе при падении.
На HEAD (без 103) падают пункты 1, 1а, 3а, 3б, 4а, 4б; уведомления не проверяются — полей нет."""
import json
import time
from _z import *

G = "Guest46"
U = uid(G)
A, T = "hour_10", "xp_boost"        # на них пробуем переводы
N1, N2 = "e2e_k12_n1", "e2e_k12_n2"  # временные ачивки для уведомлений
AF = ("title_pl", "title_en", "description_pl", "description_en")
TF = ("name_pl", "name_en", "description_pl", "description_en")
CV = '{"min": 1}'                    # login_count ≥ 1 — правка профиля считает вход за один


def q(s):
    return "'" + str(s).replace("'", "''") + "'"


def inl(ids):
    return ", ".join(map(q, ids))


def table(t, where="TRUE"):
    """Строки таблицы словарями — как их видит база (с полями 103, если они есть)."""
    return json.loads(sql(f"SELECT coalesce(json_agg(row_to_json(x)), '[]')::text FROM (SELECT * FROM {t} WHERE {where}) x"))


def pick(lang, r, ru, pl, en):
    """Что должен отдать сервер: перевод на языке гостя; пустой или из пробелов — русский."""
    v = {"pl": r.get(pl), "en": r.get(en)}.get(lang) or ""
    return v if v.strip() else (r.get(ru) or "")


def view(n, lang):
    """Язык гостя — его же правкой профиля, как в киоске и PWA; ответ: ачивки и таланты по id."""
    s, _ = api("PATCH", "/me", {"language": lang}, who=G)
    s1, d1 = api("GET", "/me/achievements", who=G)
    s2, d2 = api("GET", "/me/talents", who=G)
    check(f"{n}. {lang}: язык сменился, /me/achievements и /me/talents отвечают 200",
          s == 200 and s1 == 200 and s2 == 200, f"{s} {s1} {s2}")
    return ({a["id"]: a for a in d1.get("achievements", [])} if s1 == 200 else {},
            {t["id"]: t for t in d2.get("talents", [])} if s2 == 200 else {})


def same_as_db(lang, ach, tal):
    """Все включённые ачивки и таланты у гостя — как выходит из базы по правилу pick."""
    bad = []
    for r in table("achievements", "is_active"):
        a = ach.get(r["id"]) or {}
        if a.get("title") != pick(lang, r, "title", "title_pl", "title_en") or \
                (a.get("description") or "") != pick(lang, r, "description", "description_pl", "description_en"):
            bad.append(r["id"])
    for r in table("talent_definitions", "is_active"):
        t = tal.get(r["id"]) or {}
        if t.get("name") != pick(lang, r, "name", "name_pl", "name_en") or \
                (t.get("description") or "") != pick(lang, r, "description", "description_pl", "description_en"):
            bad.append(r["id"])
    return bad


section("схема")


def cols(t, names):
    got = {}
    for r in rows(f"SELECT column_name || '|' || is_nullable || '|' || coalesce(column_default, '') "
                  f"FROM information_schema.columns WHERE table_name = '{t}'"):
        n, nul, d = r.split("|", 2)
        got[n] = nul == "NO" and d.startswith("''")
    return {n: got.get(n, False) for n in names}


ca, ct = cols("achievements", AF), cols("talent_definitions", TF)
check("1. у ачивок title_pl, title_en, description_pl, description_en: NOT NULL, по умолчанию пусто", all(ca.values()), J(ca))
check("1а. у талантов name_pl, name_en, description_pl, description_en: NOT NULL, по умолчанию пусто", all(ct.values()), J(ct))
has103 = all(ca.values()) and all(ct.values())

lang0 = sql(f"SELECT language FROM users WHERE id = '{U}'")
at_end(lambda: sql(f"UPDATE users SET language = {q(lang0)} WHERE id = '{U}'"))
if has103:
    sa, st = table("achievements", f"id = '{A}'")[0], table("talent_definitions", f"id = '{T}'")[0]
    at_end(lambda: sql(f"UPDATE achievements SET {', '.join(f'{k} = {q(sa[k])}' for k in AF)} WHERE id = '{A}'"))
    at_end(lambda: sql(f"UPDATE talent_definitions SET {', '.join(f'{k} = {q(st[k])}' for k in TF)} WHERE id = '{T}'"))
    sql(f"UPDATE achievements SET title_pl = 'e2e PL tytuł', title_en = 'e2e EN title', description_pl = '', "
        f"description_en = 'e2e EN description' WHERE id = '{A}'")
    sql(f"UPDATE talent_definitions SET name_pl = 'e2e PL nazwa', name_en = '   ', description_pl = 'e2e PL opis', "
        f"description_en = 'e2e EN description' WHERE id = '{T}'")
ra, rt = table("achievements", f"id = '{A}'")[0], table("talent_definitions", f"id = '{T}'")[0]
ra_d = ra["description"] or ""

section("гость по-русски")
ach, tal = view(2, "ru")
a, t = ach.get(A, {}), tal.get(T, {})
check("2а. ru: hour_10 и xp_boost по-русски, хотя переводы есть",
      a.get("title") == ra["title"] and (a.get("description") or "") == ra_d and
      t.get("name") == rt["name"] and t.get("description") == rt["description"], J([a.get("title"), t.get("name")]))
bad = same_as_db("ru", ach, tal)
check("2б. ru: все включённые ачивки и таланты — как в базе", not bad and bool(ach) and bool(tal), J(bad))

section("гость по-польски")
ach, tal = view(3, "pl")
a, t = ach.get(A, {}), tal.get(T, {})
check("3а. pl: у hour_10 название польское, описание русское (польского нет)",
      a.get("title") == "e2e PL tytuł" and (a.get("description") or "") == ra_d, J([a.get("title"), a.get("description")]))
check("3б. pl: у xp_boost название и описание польские",
      t.get("name") == "e2e PL nazwa" and t.get("description") == "e2e PL opis", J([t.get("name"), t.get("description")]))
bad = same_as_db("pl", ach, tal)
check("3в. pl: все включённые ачивки и таланты — как в базе", not bad and bool(ach) and bool(tal), J(bad))

section("гость по-английски")
ach, tal = view(4, "en")
a, t = ach.get(A, {}), tal.get(T, {})
check("4а. en: у hour_10 название и описание английские",
      a.get("title") == "e2e EN title" and a.get("description") == "e2e EN description", J([a.get("title"), a.get("description")]))
check("4б. en: у xp_boost название русское (английское — одни пробелы), описание английское",
      t.get("name") == rt["name"] and t.get("description") == "e2e EN description", J([t.get("name"), t.get("description")]))
bad = same_as_db("en", ach, tal)
check("4в. en: все включённые ачивки и таланты — как в базе", not bad and bool(ach) and bool(tal), J(bad))
leak = sorted({k for x in list(ach.values()) + list(tal.values()) for k in x if k.endswith(("_pl", "_en"))})
check("5. в ответах гостю нет полей переводов — форма ответа прежняя", not leak, J(leak))

section("уведомление «ачивка получена»")
if not has103:
    print("нет полей 103 — уведомления не проверяем")
    done()
first = sql(f"SELECT coalesce(first_name, '') FROM users WHERE id = '{U}'")


def drop_grants():
    sql(f"DELETE FROM notifications WHERE type = 'achievement_earned' AND payload->>'id' IN ({inl([N1, N2])})")
    sql(f"DELETE FROM user_achievements WHERE achievement_id IN ({inl([N1, N2])})")


def drop_tmp():
    drop_grants()
    sql(f"DELETE FROM achievements WHERE id IN ({inl([N1, N2])})")


at_end(drop_tmp)
sql("INSERT INTO achievements (id, category, title, description, condition_type, condition_value, reward_skillpoints, "
    "reward_xp, is_active, title_pl, title_en) VALUES "
    f"({q(N1)}, 'lifetime', 'e2e RU n1', 'e2e', 'login_count', {q(CV)}, 0, 0, TRUE, 'e2e PL n1', 'e2e EN n1'), "
    f"({q(N2)}, 'lifetime', 'e2e RU n2', 'e2e', 'login_count', {q(CV)}, 0, 0, TRUE, '', '')")


def grant(lang):
    """Правка профиля (имя то же) — Г8: сервер сразу проверяет ачивки за данные. На миг выдачи выключены все
    прочие ачивки: у Guest46 есть имя и фамилия, а «Имя» не выдана — иначе выдалась бы настоящая, с наградой."""
    drop_grants()
    off = rows(f"SELECT id FROM achievements WHERE is_active AND id NOT IN ({inl([N1, N2])})")
    back = (lambda: sql(f"UPDATE achievements SET is_active = TRUE WHERE id IN ({inl(off)})")) if off else (lambda: None)
    at_end(back)
    before = int(sql(f"SELECT count(*) FROM user_achievements WHERE user_id = '{U}'"))
    if off:
        sql(f"UPDATE achievements SET is_active = FALSE WHERE id IN ({inl(off)})")
    try:
        s, _ = api("PATCH", "/me", {"language": lang, "first_name": first}, who=G)
    finally:
        back()
    got = {}
    for _ in range(20):
        got = dict(r.split("|", 1) for r in rows(
            f"SELECT payload->>'id' || '|' || coalesce(payload->>'title', '') FROM notifications WHERE user_id = '{U}' "
            f"AND type = 'achievement_earned' AND payload->>'id' IN ({inl([N1, N2])})"))
        if len(got) == 2:
            break
        time.sleep(0.15)
    return s, got, int(sql(f"SELECT count(*) FROM user_achievements WHERE user_id = '{U}'")) - before


s, got, new = grant("pl")
check("6. pl: у n1 название польское, у n2 (перевода нет) — русское; выданы только эти две",
      s == 200 and got == {N1: "e2e PL n1", N2: "e2e RU n2"} and new == 2, J([s, got, new]))
s, got, new = grant("en")
check("6а. en: у n1 название английское, у n2 — русское",
      s == 200 and got == {N1: "e2e EN n1", N2: "e2e RU n2"} and new == 2, J([s, got, new]))
s, got, new = grant("ru")
check("6б. ru: у n1 и n2 названия русские, хотя у n1 переводы есть",
      s == 200 and got == {N1: "e2e RU n1", N2: "e2e RU n2"} and new == 2, J([s, got, new]))
done()
