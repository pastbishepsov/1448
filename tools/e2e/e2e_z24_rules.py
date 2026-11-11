#!/usr/bin/env python3
"""З24-и1 «Расписание цен — сервер» (№53 «в»): правила цены часа зоны — дни недели, окно по стенным часам клуба (через
полночь — дню начала, 24:00 — до полуночи), цена, срок действия; правила зоны не накладываются (409 rule_overlap).
Цена берётся в момент посадки и снимком живёт в сессии: посадка за минуту до начала правила — по цене зоны, через минуту
после — по правилу, первая сессия цену не меняет (как «посадка в 17:59 и в 18:00»). Пересадка в другую зону с её
тарифом — по её цене сейчас. Наружу (GET /clubs) и на табло — цена сейчас и следующая (next_at, next_rate_pln); в
GET /admin/zones — правила, rate_now_pln и следующая. Заводит, правит и убирает правила владелец (в журнале — правило
словами), админу — 403.
Данные: временные зоны «z24 Тест» (10 zł/ч) и «z24 Второй» (12 zł/ч), ПК z24r-1…3 без агента (посадка ждёт шелл — денег
не списывает), гости z24ra и z24rb (заводятся один раз, удалить аккаунт нельзя — FK; кошелёк докладывается до 30 zł
картой). В конце сессии гостей на этих ПК, ПК и зоны (с правилами) удаляются SQL-ом; токен табло — как был."""
import time as _t

from _z import *

GA, GB = "z24ra", "z24rb"
for g in (GA, GB):
    if not uid(g):
        s, d = api("POST", "/auth/register", {"nickname": g, "password": GUEST_PASS})
        if s not in (200, 201):
            raise SystemExit(f"регистрация {g}: {s} {J(d)}")


def wipe():
    for g in (GA, GB):
        for sid in rows(f"SELECT id FROM sessions WHERE user_id = '{uid(g)}' AND status = 'active'"):
            api("POST", f"/admin/sessions/{sid}/end", {}, "boss")
    sql("DELETE FROM sessions WHERE computer_id IN (SELECT id FROM computers WHERE name LIKE 'z24r-%')")
    sql("DELETE FROM computers WHERE name LIKE 'z24r-%'")
    sql("DELETE FROM zones WHERE name LIKE 'z24 %'")


wipe()
at_end(wipe)
for g in (GA, GB):
    ensure_wallet(g, 30)

z1 = (api("POST", "/admin/zones", {"name": "z24 Тест", "rate_pln": 10}, "boss")[1].get("zone") or {}).get("id")
z2 = (api("POST", "/admin/zones", {"name": "z24 Второй", "rate_pln": 12}, "boss")[1].get("zone") or {}).get("id")
pc = {}
for n, z in (("z24r-1", z1), ("z24r-2", z1), ("z24r-3", z2)):
    pc[n] = api("POST", "/admin/computers", {"name": n, "zone_id": z}, "boss")[1].get("computer_id")
check("зоны «z24 Тест» 10 zł/ч и «z24 Второй» 12 zł/ч, ПК z24r-1…3", z1 and z2 and all(pc.values()), J([z1, z2, pc]))
seats = lambda nick, p: api("POST", f"/admin/computers/{pc[p]}/session", {"guest": nick}, "admin1")
rate_of = lambda sid: float(sql(f"SELECT base_rate_pln FROM sessions WHERE id = '{sid}'") or 0)
clock = lambda: sql("SELECT to_char(now() AT TIME ZONE 'Europe/Warsaw', 'HH24:MI:SS')")
hm = lambda m: f"{m // 60 % 24:02d}:{m % 60:02d}"
rule = lambda zone, **kw: api("POST", f"/admin/zones/{zone}/rules", kw, "boss")
zone_row = lambda zid, who="boss": next((z for z in api("GET", "/admin/zones", None, who)[1].get("zones") or [] if z.get("id") == zid), {})
pub_zone = lambda name: next((z for c in api("GET", "/clubs")[1].get("clubs") or [] for z in c.get("zones") or [] if z.get("name") == name), {})
seen = lambda: {x.get("id") for x in audit("boss", limit=80)}
A0 = seen()

section("правило с начала следующей минуты: посадка до и после")
h, m, s = map(int, clock().split(":"))
if s > 45:  # до границы — меньше 15 с: ждём следующую минуту, чтобы успеть посадить первого гостя
    _t.sleep(61 - s)
    h, m, s = map(int, clock().split(":"))
B = (h * 60 + m + 1) % 1440  # начало правила — следующая минута по часам клуба
E = (B + 120) % 1440
s1, d1 = rule(z1, days_mask=127, start_min=B, end_min=E if E else 1440, rate_pln=25, note="z24 вечер")
r1 = d1.get("rule") or {}
check(f"правило «каждый день {hm(B)}–{hm(E)} — 25 zł/ч» заведено: 201, текст правила, у зоны сейчас 10, следующая — 25 в {hm(B)}",
      s1 == 201 and r1.get("text") == f"каждый день {hm(B)}–{hm(E) if E else '24:00'} — 25,00 zł/ч" and (d1.get("zone") or {}).get("rate_pln") == 10
      and (d1.get("zone") or {}).get("next_rate_pln") == 25, J([s1, d1], 500))
za = zone_row(z1)
nx = za.get("next_at") or "1970-01-01T00:00:00Z"
check("GET /admin/zones: у зоны правила, rate_pln — цена зоны 10, rate_now_pln — 10, следующая — 25",
      za.get("rate_pln") == 10 and za.get("rate_now_pln") == 10 and za.get("next_rate_pln") == 25 and len(za.get("rules") or []) == 1
      and sql(f"SELECT to_char('{nx}'::timestamptz AT TIME ZONE 'Europe/Warsaw', 'HH24:MI')") == hm(B), J(za, 500))
pz = pub_zone("z24 Тест")
check("GET /clubs: «z24 Тест» сейчас 10 zł/ч, с границы — 25", pz.get("rate_pln") == 10 and pz.get("next_rate_pln") == 25 and pz.get("next_at"), J(pz))
sa, da = seats(GA, "z24r-1")
sid_a = da.get("session_id")
before = clock()
check(f"посадка {GA} в {before} (до {hm(B)}) — 201, цена сессии 10 zł/ч — цена зоны", sa == 201 and rate_of(sid_a) == 10, J([sa, da]))
while True:  # ждём границу правила по часам клуба
    hh, mm, ss = map(int, clock().split(":"))
    if (hh * 60 + mm) % 1440 == B:
        break
    _t.sleep(1)
sb, db_ = seats(GB, "z24r-2")
sid_b = db_.get("session_id")
after = clock()
check(f"посадка {GB} в {after} (с {hm(B)}) — 201, цена сессии 25 zł/ч — по правилу", sb == 201 and rate_of(sid_b) == 25, J([sb, db_]))
check(f"сессия {GA} — по-прежнему 10 zł/ч: цена зафиксирована на посадке", rate_of(sid_a) == 10, rate_of(sid_a))
pz2 = pub_zone("z24 Тест")
nx2 = pz2.get("next_at") or "1970-01-01T00:00:00Z"
check(f"GET /clubs теперь: 25 zł/ч, следующая — 10 в {hm(E)}", pz2.get("rate_pln") == 25 and pz2.get("next_rate_pln") == 10
      and sql(f"SELECT to_char('{nx2}'::timestamptz AT TIME ZONE 'Europe/Warsaw', 'HH24:MI')") == hm(E), J(pz2))

section("пересадка в другую зону — по её цене сейчас")
mm_now = int(clock()[:2]) * 60 + int(clock()[3:5])
s2, d2 = rule(z2, days_mask=127, start_min=(mm_now - 10) % 1440, end_min=(mm_now + 110) % 1440 or 1440, rate_pln=18)
check("у «z24 Второй» правило, которое идёт сейчас: 18 zł/ч", s2 == 201 and (d2.get("zone") or {}).get("rate_pln") == 18, J([s2, d2]))
sm, dm = api("POST", f"/admin/sessions/{sid_a}/move", {"computer_id": pc["z24r-3"], "tariff": "new"}, "admin1")
check(f"{GA} пересажен на z24r-3 с тарифом новой зоны — 18 zł/ч (цена зоны 12 — не она)", sm == 200 and rate_of(sid_a) == 18, J([sm, dm, rate_of(sid_a)]))

section("проверки и наложение")
bad = [
    ("без дней — 400 bad_days", dict(days_mask=0, start_min=60, end_min=120, rate_pln=5), 400, "bad_days"),
    ("пустое окно — 400 zero_length", dict(days_mask=1, start_min=60, end_min=60, rate_pln=5), 400, "zero_length"),
    ("конец 00:00 — 400 bad_time (до полуночи — 24:00)", dict(days_mask=1, start_min=60, end_min=0, rate_pln=5), 400, "bad_time"),
    ("цена 0 — 400 bad_rate", dict(days_mask=1, start_min=60, end_min=120, rate_pln=0), 400, "bad_rate"),
    ("срок наоборот — 400 bad_dates", dict(days_mask=1, start_min=60, end_min=120, rate_pln=5, valid_from="2026-12-31", valid_to="2026-12-01"), 400, "bad_dates"),
    ("дата не ГГГГ-ММ-ДД — 400 bad_dates", dict(days_mask=1, start_min=60, end_min=120, rate_pln=5, valid_from="01.12.2026"), 400, "bad_dates"),
    ("без цены — 400", dict(days_mask=1, start_min=60, end_min=120), 400, "invalid_request"),
]
res = [(n, rule(z1, **body)) for n, body, st, code in bad]
check("кривые правила: " + "; ".join(n for n, *_ in bad),
      all(r[0] == st and r[1].get("code") == code for (n, r), (_, _, st, code) in zip(res, bad)), J([(n, r[0], r[1].get("code")) for n, r in res]))
so, do_ = rule(z1, days_mask=127, start_min=(B + 60) % 1440, end_min=(B + 180) % 1440 or 1440, rate_pln=30)
check("правило внахлёст с первым — 409 rule_overlap, в ответе — с каким",
      so == 409 and do_.get("code") == "rule_overlap" and (do_.get("with") or {}).get("id") == r1.get("id") and "Накладывается на правило «каждый день" in (do_.get("message") or ""),
      J([so, do_]))
sx, _ = rule(z1, days_mask=127, start_min=(B + 120) % 1440, end_min=(B + 180) % 1440 or 1440, rate_pln=30)
check("правило встык (с конца первого) — можно", sx == 201, sx)
st_, _ = api("POST", f"/admin/zones/{z1}/rules", {"days_mask": 1, "start_min": 60, "end_min": 120, "rate_pln": 5}, "admin1")
sz, dz = rule("8f0e6c43-1111-4c1e-9e55-0a0b0c0d0e0f", days_mask=1, start_min=60, end_min=120, rate_pln=5)
check("админ правил не заводит — 403; чужая зона — 404 zone_not_found", st_ == 403 and sz == 404 and dz.get("code") == "zone_not_found", J([st_, sz, dz]))

section("правка и удаление")
sp, dp = api("PATCH", f"/admin/zone-rules/{r1.get('id')}", {"rate_pln": 26}, "boss")
sp2, _ = api("PATCH", f"/admin/zone-rules/{r1.get('id')}", {"rate_pln": 26}, "boss")
check("PATCH цены 25 → 26 — 200, зона сейчас 26; повтор без изменений — 200",
      sp == 200 and (dp.get("rule") or {}).get("rate_pln") == 26 and (dp.get("zone") or {}).get("rate_pln") == 26 and sp2 == 200, J([sp, dp]))
sd, _ = api("DELETE", f"/admin/zone-rules/{r1.get('id')}", None, "boss")
check("DELETE правила — 200, зона снова 10; правила больше нет", sd == 200 and zone_row(z1).get("rate_now_pln") == 10
      and sql(f"SELECT count(*) FROM zone_price_rules WHERE id = '{r1.get('id')}'") == "0", sd)
s404, _ = api("PATCH", f"/admin/zone-rules/{r1.get('id')}", {"rate_pln": 5}, "boss")
check("правка убранного — 404", s404 == 404, s404)

section("журнал и табло")
aud = [(x.get("kind"), x.get("text", "")) for x in audit("boss", limit=60) if x.get("id") not in A0 and (x.get("kind") or "").startswith("zone_rule")]
txt = f"каждый день {hm(B)}–{hm(E) if E else '24:00'}"
check("журнал: заведено, «было → стало» (одна строка — повтор без изменений не пишется), убрано",
      sum(1 for k, t in aud if k == "zone_rule_create" and t == f"z24 Тест: правило {txt} — 25,00 zł/ч · z24 вечер") == 1
      and sum(1 for k, t in aud if k == "zone_rule_update") == 1
      and (("zone_rule_update", f"z24 Тест: правило {txt} — 25,00 zł/ч · z24 вечер → {txt} — 26,00 zł/ч · z24 вечер") in aud)
      and any(k == "zone_rule_delete" and t.startswith("z24 Тест: правило убрано — ") for k, t in aud), J(aud, 900))
s_t, d_t = api("GET", "/admin/board/token", None, "boss")
tok, made = d_t.get("token") or "", False
if not tok:
    tok, made = api("POST", "/admin/board/token", None, "boss")[1].get("token") or "", True
if made:
    at_end(lambda: api("DELETE", "/admin/board/token", None, "boss"))
bs, bd = api("GET", f"/board/state?token={tok}")
bz = next((z for z in bd.get("zones") or [] if z.get("name") == "z24 Второй"), {})
check("табло: «z24 Второй» — 18 zł/ч сейчас (правило), следующая — 12 (цена зоны) в конце окна",
      bs == 200 and bz.get("rate_pln") == 18 and bz.get("next_rate_pln") == 12 and bz.get("next_at"), J([bs, bz]))

done()
