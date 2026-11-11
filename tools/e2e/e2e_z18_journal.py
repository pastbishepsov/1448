#!/usr/bin/env python3
"""З18-и4г «Журнал админа, потолки и отмена — с начала смены» (З-Р38 п. 3; решение основателя 07.11): «сегодня» роли
admin — клубные сутки, а если его смена («Я пришёл») началась раньше — с её начала, но не дальше суток назад. Раньше с
сутками с полуночи ночная смена резалась пополам: в 00:00 из журнала админа уходило всё с 20:00, дневной потолок
начинался заново, своё пополнение в 23:50 админ уже не мог отменить (403 too_old), «Продажи смены» и карточка гостя
его не показывали. Здесь — временный админ z18j на смене, начатой до начала текущих суток (запись табеля сдвинута
SQL-ом): его пополнение и продажа «до полуночи» — в журнале, в «Продажах смены» и карточке гостя, идут в дневной потолок
и отменяются им самим; пополнение до начала смены — нет; смена старше суток — окно не дальше суток назад; не на смене —
клубные сутки, как раньше. Гость z18jg — свой у проверок журнала (с `pw_z18_journal`; заводится один раз, удалить
аккаунт нельзя — FK): карточка гостя показывает его 20 последних пополнений, и у общего Guest49 пополнение «его смены»
уходило из неё после нескольких прогонов за день (З21-и3). Пополнения и продажа — картой (уезжают в прошлое), в конце
отменяются; отменённые пополнения z18jg удаляются в начале и в конце; позиция «z18j Сок» заводится и удаляется; записи
табеля z18j удаляются в начале и в конце."""
from _z import *

NICK, G, GOOD = "z18j", "z18jg", "z18j Сок"
if not uid(G):
    s, d = api("POST", "/auth/register", {"nickname": G, "password": GUEST_PASS})
    if s not in (200, 201):
        raise SystemExit(f"регистрация {G}: {s} {J(d)}")
# З21-и3: отменённые пополнения своего гостя — хвосты прошлых прогонов (ссылок на них нет); в конце — уже после отмены
MINE = f"DELETE FROM deposits WHERE user_id = (SELECT id FROM users WHERE nickname = '{G}') AND voided_at IS NOT NULL"
sql(MINE)
at_end(lambda: sql(MINE))
ENTRIES = f"DELETE FROM work_entries WHERE user_id = (SELECT id FROM users WHERE nickname = '{NICK}')"
sql(ENTRIES)  # хвост прошлого прогона
at_end(lambda: sql(ENTRIES))  # в конце — все его записи, уже после увольнения (уборка идёт в обратном порядке)
for gid in rows(f"SELECT id FROM goods WHERE name = '{GOOD}'"):
    drop_good(gid)
at_end(lambda: [drop_good(gid) for gid in rows(f"SELECT id FROM goods WHERE name = '{GOOD}'")])  # после отмены продажи
deps, sales = [], []
live = lambda table, i: sql(f"SELECT voided_at IS NULL FROM {table} WHERE id = '{i}'") == "t"
at_end(lambda: [api("POST", f"/admin/deposits/{d}/void", {}, "boss") for d in deps if live("deposits", d)])
at_end(lambda: [api("POST", f"/admin/sales/{s}/void", {}, "boss") for s in sales if s and live("sales", s)])
good = (api("POST", "/admin/goods", {"name": GOOD, "category": "z18", "price_pln": 4, "stock": 5}, "boss")[1].get("good") or {}).get("id")
zid = hire_temp(NICK)
tok(NICK)  # вход в админку открывает смену
loc = lambda expr: sql(f"SELECT to_char(({expr}) AT TIME ZONE '{TZ}', 'YYYY-MM-DD HH24:MI')")  # момент по часам клуба
same_at = lambda iso, expr: bool(iso) and sql(f"SELECT '{iso}'::timestamptz = {expr}") == "t"  # момент из ответа = момент SQL
DF = f"'{api('GET', '/admin/overview', None, NICK)[1].get('day_from', '')}'::timestamptz"  # начало текущих суток
# смена — до начала суток (по возможности с 20:00 накануне) и меньше суток назад
s_at = sql(f"SELECT greatest({DF} - interval '4 hours', now() - interval '23 hours 55 minutes')")
S = f"'{s_at}'::timestamptz"
m_at = sql(f"SELECT {S} + ({DF} - {S}) / 2")  # между началом смены и началом суток
M = f"'{m_at}'::timestamptz"
rh = settings().get("report_hour", 0)
sql(f"UPDATE work_entries SET started_at = {S}, date = (({S} AT TIME ZONE '{TZ}') - interval '{rh} hours')::date "
    f"WHERE user_id = '{zid}' AND ended_at IS NULL")


def dep(pln, at):
    d = topup(G, pln, method="card", who=NICK)
    deps.append(d)
    sql(f"UPDATE deposits SET created_at = {at} WHERE id = '{d}'")
    return d


def seen(d, who=NICK):
    """журнал who страницей сразу после пополнения d: виден ли он; ответ целиком"""
    cur = sql(f"SELECT to_char((created_at + interval '1 second') AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS\"Z\"') FROM deposits WHERE id = '{d}'")
    s, au = api("GET", f"/admin/audit?kind=deposit&limit=200&before={cur}", None, who)
    return f"deposit:{d}" in [x.get("id") for x in au.get("items", [])], au


section(f"смена z18j — с {loc(S)}, раньше текущих суток (с {loc(DF)})")
check("смена началась раньше текущих суток и меньше суток назад", sql(f"SELECT {S} < {DF} AND {S} > now() - interval '1 day'") == "t",
      f"{loc(S)} · сутки с {loc(DF)}")
d_in = dep(7, M)
d_out = dep(3, f"{S} - interval '10 minutes'")
s, sd = api("POST", "/admin/sales", {"good_id": good, "method": "card"}, NICK)
sale = (sd.get("sale") or {}).get("id")
sales.append(sale)
sql(f"UPDATE sales SET created_at = {M}, paid_at = {M} WHERE id = '{sale}'")
ok_in, au = seen(d_in)
check(f"журнал z18j: пополнение 7 zł в {loc(M)} — до начала суток, но в его смене — видно; окно — с начала смены (shift: true), "
      "сутки — те же", ok_in and au.get("shift") is True and same_at(au.get("from"), S) and au.get("day") == club_day(),
      f"{ok_in} {J({k: au.get(k) for k in ('day', 'from', 'to', 'shift')})}")
ok_out, _ = seen(d_out)
check("пополнение за 10 минут до начала смены — не видно", not ok_out, str(ok_out))
# З18-и4ж: лента отдаёт 300 последних продаж окна, а стенд за сутки копит их больше — продажа смены ищется по итогам окна
# (total_count): за 10 минут до начала смены её в окне нет — на одну меньше; в списке — если он не обрезан
s, fs = api("GET", "/admin/sales", None, NICK)
sql(f"UPDATE sales SET created_at = {S} - interval '10 minutes', paid_at = {S} - interval '10 minutes' WHERE id = '{sale}'")
n_out = api("GET", "/admin/sales", None, NICK)[1].get("total_count")
sql(f"UPDATE sales SET created_at = {M}, paid_at = {M} WHERE id = '{sale}'")
listed = sale in [x.get("id") for x in fs.get("sales", [])]
capped = (fs.get("total_count") or 0) > len(fs.get("sales", []))
check("«Продажи смены»: продажа его смены до начала суток — в итогах окна (за 10 минут до начала смены — на одну меньше) и в "
      "списке, если он не обрезан; окно — с начала смены (shift, from)",
      bool(sale) and fs.get("total_count") is not None and n_out == fs.get("total_count") - 1 and (listed or capped)
      and fs.get("shift") is True and same_at(fs.get("from"), S),
      J({**{k: fs.get(k) for k in ("date", "from", "shift", "count", "total_count")}, "без неё": n_out, "в списке": listed}))
s, card = api("GET", f"/admin/users/{uid(G)}", None, NICK)
ids = [x.get("id") for x in card.get("deposits", [])]
check("карточка гостя у z18j: пополнение его смены — есть, до смены — нет", d_in in ids and d_out not in ids, f"{len(ids)} строк")

section("дневной потолок — с начала смены")
set_settings(admin_day_deposit_cap_pln=10)
s5, r5 = api("POST", f"/admin/users/{uid(G)}/deposit", {"amount_pln": 5, "method": "card"}, NICK)
if s5 == 201:
    deps.append(r5.get("deposit_id"))
check("потолок 10 zł: 7 zł до начала суток + 5 → 403 day_cap (смена одна — потолок не начинается заново в полночь)",
      s5 == 403 and r5.get("code") == "day_cap", f"{s5} {r5.get('code')} {r5.get('message')}")
s3, r3 = api("POST", f"/admin/users/{uid(G)}/deposit", {"amount_pln": 3, "method": "card"}, NICK)
if s3 == 201:
    deps.append(r3.get("deposit_id"))
check("7 + 3 = 10 — в пределах потолка → 201", s3 == 201, f"{s3} {r3.get('code')}")
restore_settings()

section("отмена своего — с начала смены")
v_in = api("POST", f"/admin/deposits/{d_in}/void", {}, NICK)
v_sale = api("POST", f"/admin/sales/{sale}/void", {}, NICK)
v_out = api("POST", f"/admin/deposits/{d_out}/void", {}, NICK)
check("своё пополнение и продажу до начала суток отменяет сам (200), пополнение до начала смены — 403 too_old",
      v_in[0] == 200 and v_sale[0] == 200 and v_out[0] == 403 and v_out[1].get("code") == "too_old",
      J([v_in[0], v_sale[0], v_out[0], v_out[1].get("code")]))

section("смена старше суток — окно не дальше суток назад")
sql(f"UPDATE work_entries SET started_at = now() - interval '30 hours' WHERE user_id = '{zid}' AND ended_at IS NULL")
d_old, d_23 = dep(2, "now() - interval '25 hours'"), dep(2, "now() - interval '23 hours'")
o_old, au = seen(d_old)
o_23, _ = seen(d_23)
reach = bool(au.get("from")) and sql(f"SELECT '{au.get('from')}'::timestamptz BETWEEN now() - interval '1 day 1 minute' AND {DF}") == "t"
check("смена с «30 часов назад»: окно — с «сутки назад»; пополнение 25 ч назад — не видно, 23 ч назад — видно",
      not o_old and o_23 and reach, f"{o_old} {o_23} · from {au.get('from')}")

section("не на смене — клубные сутки, как раньше")
sql(f"UPDATE work_entries SET ended_at = now() WHERE user_id = '{zid}' AND ended_at IS NULL")
s, au = api("GET", "/admin/audit?limit=5", None, NICK)
check("смена закрыта: окно журнала — с начала суток, shift: false", au.get("shift") is False and same_at(au.get("from"), DF),
      J({k: au.get(k) for k in ("day", "from", "shift")}))
done()
