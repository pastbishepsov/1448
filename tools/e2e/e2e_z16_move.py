#!/usr/bin/env python3
"""З16-и1а «Пересадка в другую зону — тариф этой зоны» (вопрос 1 отчёта З16-и1, решение основателя 28.10). Пересадка не
трогала тариф никогда: гостя, пересаженного со Standard на VIP, досчитывали по Standard. Теперь в той же зоне тариф и
зона снимка не меняются; за ПК другой зоны — сперва 409 zone_change с тарифом сейчас и после, затем выбор админа: «new»
— тариф и зона новой зоны, как при посадке за этот ПК сейчас (цена зоны минус скидки гостя), сыгранные минуты досчитаны
по прежнему тарифу, предупреждения о балансе — заново; «keep» — прежние тариф и зона, только с причиной, она уходит в
журнал. Ставки XP и монет пересадка не трогает. Гости: Guest46 — скидка 10 % на время проверки (payment_increase_pct
SQL-ом, в конце — как была), сессия PC-07 → PC-06 → VIP-03; Guest47 — PC-05 → VIP-02 с прежним тарифом → PC-05;
кошельки докладываются до 30 zł, сессии списывают свои минуты как обычно."""
import json

from _z import *

G, H = "Guest46", "Guest47"
STD, VIP = sql("SELECT id FROM zones WHERE name = 'Standard'"), sql("SELECT id FROM zones WHERE name = 'VIP'")
R_STD, R_VIP = (float(sql(f"SELECT rate_pln FROM zones WHERE id = '{z}'")) for z in (STD, VIP))
end_all(G, H)
at_end(lambda: end_all(G, H))
for n in (G, H):
    ensure_wallet(n, 30)
PCT0 = sql(f"SELECT payment_increase_pct FROM users WHERE nickname = '{G}'")
at_end(lambda: sql(f"UPDATE users SET payment_increase_pct = {PCT0} WHERE nickname = '{G}'"))
sql(f"UPDATE users SET payment_increase_pct = 10 WHERE nickname = '{G}'")
# минуты гостей — только деньгами: ни запаса монетами, ни пакетов, ни зонных минут
CLEAN = sql(f"SELECT count(*) FROM users u WHERE u.nickname IN ('{G}', '{H}') AND (u.coin_minutes > 0 OR EXISTS "
            f"(SELECT 1 FROM user_packages p WHERE p.user_id = u.id AND p.minutes_left > 0) OR EXISTS "
            f"(SELECT 1 FROM minute_grants g WHERE g.user_id = u.id AND g.minutes_left > 0))") == "0"

ru = lambda v: f"{v:.2f}".replace(".", ",")
cost = lambda rate_pln, m: (round(rate_pln * 100) * m + 59) // 60  # costForMinutes: вверх до гроша
mv = lambda sid, pc, **kw: api("POST", f"/admin/sessions/{sid}/move", {"computer_id": pcs()[pc]["id"], **kw}, "admin1")
last_move = lambda nick: sql(f"SELECT details FROM admin_actions WHERE action = 'session_move' AND target_user_id = "
                             f"'{uid(nick)}' ORDER BY created_at DESC LIMIT 1")


def row(sid):
    """«ПК|зона снимка|цена зоны|тариф гостя|XP|монеты» сессии."""
    return sql(f"SELECT c.name || '|' || s.zone_name || '|' || s.base_rate_pln || '|' || s.effective_rate_pln || '|' || "
               f"COALESCE(s.xp_per_min::text, '') || '|' || COALESCE(s.coins_per_min::text, '') FROM sessions s "
               f"JOIN computers c ON c.id = s.computer_id WHERE s.id = '{sid}'")


def tariff_zone(pc):
    """Пометка «тариф …» у сессии ПК в зале (GET /admin/computers): None — нет."""
    rows_ = api("GET", "/admin/computers", None, "admin1")[1].get("computers") or []
    p = next((x for x in rows_ if x["name"] == pc), {})
    return (p.get("session") or {}).get("tariff_zone")


XP, COINS = sql("SELECT COALESCE((SELECT value FROM settings WHERE key = 'xp_per_min'), '10')"), \
    sql("SELECT COALESCE((SELECT value FROM settings WHERE key = 'coins_per_min'), '2')")
R18, R27 = round(R_STD * 0.9, 2), round(R_VIP * 0.9, 2)

section("в той же зоне — без вопроса, тариф прежний")
sid = seat(G, "PC-07", minutes_ago=10)
r0 = row(sid)
check(f"посадка Guest46 за PC-07 со скидкой 10 %: Standard, {ru(R_STD)} → {ru(R18)} zł/ч, ставки XP и монет {XP} и {COINS}",
      CLEAN and r0 == f"PC-07|Standard|{R_STD:.2f}|{R18:.2f}|{XP}|{COINS}", f"{CLEAN} {r0}")
s1, d1 = mv(sid, "PC-06")
check("PC-07 → PC-06 в той же зоне — 200 без вопроса (tariff same): тариф и зона прежние; в журнале «PC-07 → PC-06»",
      s1 == 200 and d1.get("tariff") == "same" and row(sid) == f"PC-06|Standard|{R_STD:.2f}|{R18:.2f}|{XP}|{COINS}"
      and last_move(G) == "PC-07 → PC-06", f"{s1} {J(d1)} · {row(sid)} · {last_move(G)}")

section("в другую зону — сперва вопрос")
s2, d2 = mv(sid, "VIP-03")
msg = f"VIP-03 — зона VIP: тариф гостя станет {ru(R27)} zł/ч (сейчас Standard, {ru(R18)} zł/ч)"
check(f"PC-06 → VIP-03 без выбора тарифа — 409 zone_change: «{msg}»; гость остался за PC-06",
      s2 == 409 and d2.get("code") == "zone_change" and d2.get("message") == msg
      and (d2.get("zone_from"), d2.get("zone_to"), d2.get("rate_pln"), d2.get("new_rate_pln")) == ("Standard", "VIP", R18, R27)
      and row(sid).startswith("PC-06|"), f"{s2} {J(d2)} · {row(sid)}")
s3, d3 = mv(sid, "VIP-03", tariff="keep")
s4, d4 = mv(sid, "VIP-03", tariff="keep", reason="   ")
s5, d5 = mv(sid, "VIP-03", tariff="keep", reason="я" * 201)
s6, d6 = mv(sid, "VIP-03", tariff="vip")
check("«оставить прежний» без причины и с причиной из пробелов — 400 reason_required, длиннее 200 символов — 400 "
      "reason_long, тариф «vip» — 400 bad_tariff; гость всё там же",
      [(s, d.get("code")) for s, d in ((s3, d3), (s4, d4), (s5, d5), (s6, d6))]
      == [(400, "reason_required"), (400, "reason_required"), (400, "reason_long"), (400, "bad_tariff")]
      and row(sid).startswith("PC-06|"), f"{s3} {d3.get('code')} {s4} {d4.get('code')} {s5} {d5.get('code')} {s6} "
                                          f"{d6.get('code')} · {row(sid)}")

section("по тарифу новой зоны")
sql(f"UPDATE sessions SET warn15_at = now() WHERE id = '{sid}'")  # «осталось ~15 минут» уже уходило
s7, d7 = mv(sid, "VIP-03", tariff="new")
money = sql(f"SELECT billed_minutes || '|' || money_minutes || '|' || charged_grosz || '|' || (warn15_at IS NULL) "
            f"FROM sessions WHERE id = '{sid}'")
check(f"PC-06 → VIP-03 по тарифу новой зоны (200, tariff new): VIP, {ru(R_VIP)} → {ru(R27)} zł/ч со скидкой гостя, "
      f"ставки XP и монет прежние; 10 сыгранных минут досчитаны по {ru(R18)} — {cost(R18, 10)} гр.; предупреждение о "
      f"балансе — заново", s7 == 200 and (d7.get("tariff"), d7.get("zone"), d7.get("rate_pln")) == ("new", "VIP", R27)
      and row(sid) == f"VIP-03|VIP|{R_VIP:.2f}|{R27:.2f}|{XP}|{COINS}" and money == f"10|10|{cost(R18, 10)}|true",
      f"{s7} {J(d7)} · {row(sid)} · {money}")
note = sql(f"SELECT payload FROM notifications WHERE user_id = '{uid(G)}' AND type = 'session_moved' "
           f"ORDER BY created_at DESC LIMIT 1")
note = json.loads(note) if note else {}
want_log = f"PC-06 → VIP-03 · тариф VIP {ru(R27)} zł/ч (был Standard {ru(R18)} zł/ч)"
check(f"в журнале «{want_log}»; гостю — уведомление session_moved с ПК, зоной и тарифом",
      last_move(G) == want_log and note == {"computer": "VIP-03", "zone": "VIP", "rate_pln": R27, "tariff": "new"},
      f"{last_move(G)} · {J(note)}")
sql(f"UPDATE sessions SET started_at = started_at - interval '6 minutes', ready_at = ready_at - interval '6 minutes' "
    f"WHERE id = '{sid}'")  # ещё 6 минут — уже за VIP-03
se, de = end(sid)
m = de.get("minutes") or 0
want = cost(R18, 10) + cost(R27, m) - cost(R27, 10)
got = sql(f"SELECT charged_grosz FROM sessions WHERE id = '{sid}'")
check(f"завершение ещё через 6 минут (200): {m} мин — 10 по {ru(R18)} и {m - 10} по {ru(R27)}, всего {want} гр. (по "
      f"прежнему тарифу было бы {cost(R18, m)})", se == 200 and got == str(want) and want != cost(R18, m), f"{se} {got}")

section("оставить прежний тариф — с причиной")
WHY = "z16 PC-05 сломался — VIP-02 по цене Standard"
sid2 = seat(H, "PC-05", minutes_ago=3)
r8 = row(sid2)
s8, d8 = mv(sid2, "VIP-02", tariff="keep", reason="  " + WHY + " ")
want_log = f"PC-05 → VIP-02 · тариф Standard {ru(R_STD)} zł/ч оставлен: {WHY}"
check(f"Guest47: PC-05 → VIP-02 с прежним тарифом и причиной (200, tariff keep) — тариф и зона прежние: Standard, "
      f"{ru(R_STD)} zł/ч; в журнале «{want_log}»", s8 == 200 and d8.get("tariff") == "keep"
      and row(sid2) == r8.replace("PC-05|", "VIP-02|", 1) and r8.startswith(f"PC-05|Standard|{R_STD:.2f}|{R_STD:.2f}|")
      and last_move(H) == want_log, f"{s8} {J(d8)} · {r8} → {row(sid2)} · {last_move(H)}")
tz = tariff_zone("VIP-02")
s9, d9 = mv(sid2, "PC-05")
check("в зале у VIP-02 — «тариф Standard»; обратно VIP-02 → PC-05 — зона тарифа сессии: 200 без вопроса (tariff same), "
      "пометки нет", tz == "Standard" and s9 == 200 and d9.get("tariff") == "same" and tariff_zone("PC-05") is None
      and row(sid2) == r8, f"{tz} {s9} {J(d9)} · {tariff_zone('PC-05')} · {row(sid2)}")
end(sid2)
done()
