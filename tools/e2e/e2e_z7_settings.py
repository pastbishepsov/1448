#!/usr/bin/env python3
"""З7-и1 «Экономика» группами (№41; из №40 — транзакция), сервер: PUT /admin/settings пишет только то, что
изменилось; «было» — действующее значение (у настройки без строки в базе — значение по умолчанию); все строки —
одной транзакцией. Раньше «Сохранить» без правок писал в журнал «изменёнными» настройки без строки в settings (на
свежем клубе — пять: лимиты компенсаций и депозитов, «пропавшие», граница суток) и без стрелки «было → стало», а
сбой базы на середине оставлял часть настроек сохранённой. Таблица settings в конце — как была: те же строки и
значения; временный триггер сбоя снимается сразу после своей проверки."""
from _z import *

# значения по умолчанию из settings.go: строку с таким значением можно убрать — поведение клуба не меняется
DEFAULTS = {"report_hour": "8", "seg_lost_days": "21", "admin_day_comp_cap_min": "0", "admin_day_comp_cap_pln": "0",
            "admin_day_deposit_cap_pln": "0"}

orig = dict(r.split("|", 1) for r in rows("SELECT key, value FROM settings"))

def restore():
    sql("DROP TRIGGER IF EXISTS z7_fail ON settings; DROP FUNCTION IF EXISTS z7_fail()")
    keep = ",".join(f"'{k}'" for k in orig) or "''"
    sql(f"DELETE FROM settings WHERE key NOT IN ({keep})")
    for k, v in orig.items():
        sql(f"INSERT INTO settings (key, value) VALUES ('{k}', '{v}') ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value")
at_end(restore)

def fresh_club():
    """Настройки, равные значению по умолчанию, — без строки в базе, как на свежем клубе."""
    for k, v in DEFAULTS.items():
        sql(f"DELETE FROM settings WHERE key='{k}' AND value='{v}'")
    return [k for k in DEFAULTS if not sql(f"SELECT 1 FROM settings WHERE key='{k}'")]

audit_n = lambda: int(sql("SELECT count(*) FROM admin_actions WHERE action='settings_update'"))
audit_last = lambda: sql("SELECT details FROM admin_actions WHERE action='settings_update' ORDER BY created_at DESC LIMIT 1")
rows_n = lambda: int(sql("SELECT count(*) FROM settings"))

section("«Сохранить» без правок")
missing = fresh_club()
cur = settings()
a0, r0 = audit_n(), rows_n()
s, d = api("PUT", "/admin/settings", cur, "boss")
check(f"все {len(cur)} настроек с теми же значениями → 200", s == 200, f"{s} {J(d)}")
check(f"журнал не пишется (без строки в базе — {len(missing)} настроек по умолчанию)", audit_n() == a0,
      f"+{audit_n() - a0}: {audit_last()}")
check("строк в settings не прибавилось", rows_n() == r0, f"{r0} → {rows_n()}")
check("значения те же", settings() == cur)

section("одна правка — одна строка «было → стало»")
fresh_club()
v = int(settings()["seg_lost_days"])
a0 = audit_n()
s, _ = api("PUT", "/admin/settings", {"seg_lost_days": v + 1}, "boss")
check(f"«пропавшие» {v} → {v + 1}: одна строка журнала «seg_lost_days: {v}→{v + 1}»", s == 200 and audit_n() == a0 + 1
      and audit_last() == f"seg_lost_days: {v}→{v + 1}", f"{s} +{audit_n() - a0}: {audit_last()}")
w = int(settings()["coin_burn_warn_days"])
a0 = audit_n()
s, _ = api("PUT", "/admin/settings", {"seg_lost_days": v + 1, "report_hour": int(settings()["report_hour"]),
                                       "coin_burn_warn_days": w + 1}, "boss")
check("в запросе три настройки, изменилась одна → в журнале только она", s == 200 and audit_n() == a0 + 1
      and audit_last() == f"coin_burn_warn_days: {w}→{w + 1}", f"{s} +{audit_n() - a0}: {audit_last()}")

section("кривое значение — не сохраняется ничего")
a0 = audit_n()
s, d = api("PUT", "/admin/settings", {"seg_lost_days": v + 5, "report_hour": 99}, "boss")
check("report_hour 99 → 400 bad_value, «пропавшие» не тронуты, журнал молчит", s == 400 and d.get("code") == "bad_value"
      and int(settings()["seg_lost_days"]) == v + 1 and audit_n() == a0, f"{s} {J(d)}")

section("всё или ничего: сбой базы на середине")
keys = ["booking_buffer_min", "booking_lock_min", "coin_burn_warn_days", "coin_idle_days", "no_show_min",
        "seg_new_days", "streak_freeze_max", "zero_grace_min"]
before = settings()
body = {k: int(before[k]) + 1 for k in keys}
body["mousepad_cm"] = int(before["mousepad_cm"]) + 1
sql("CREATE OR REPLACE FUNCTION z7_fail() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN "
    "IF NEW.key = 'mousepad_cm' THEN RAISE EXCEPTION 'z7: сбой на середине'; END IF; RETURN NEW; END $$")
sql("CREATE TRIGGER z7_fail BEFORE INSERT OR UPDATE ON settings FOR EACH ROW EXECUTE FUNCTION z7_fail()")
a0 = audit_n()
s, d = api("PUT", "/admin/settings", body, "boss")
sql("DROP TRIGGER IF EXISTS z7_fail ON settings; DROP FUNCTION IF EXISTS z7_fail()")
after = settings()
moved = [k for k in keys if after[k] != before[k]]
check("запрос на 9 настроек, база падает на mousepad_cm → ошибка сервера", s == 500, f"{s} {J(d)}")
check("ни одна из восьми остальных не сохранилась, журнал молчит", not moved and audit_n() == a0,
      f"сохранились: {', '.join(moved) or '—'}; журнал +{audit_n() - a0}")
done()
