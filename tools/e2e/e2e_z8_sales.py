#!/usr/bin/env python3
"""З8-и4 «Итог ленты продаж» (хвост З8, «Найдено по дороге» З8-и3): лента продаж смены (GET /admin/sales) — в
списке последние 300 строк, а итог «N zł · кошельком · в долг · отменено» и число строк суток — по всему окну
клубных суток, как в базе. Раньше итог считался по тем же трёмстам строкам: в загруженный день новая строка
вытесняла из списка старую оплаченную, и «продажи смены» занижались.
Чтобы лента была полной на любом стенде, проверка на время добавляет 300 отменённых продаж картой «z8 заглушка»
без гостя и товара — в выручку, долг и ящик они не идут; они занимают весь список, итог должен остаться прежним.
В конце заглушки удаляются."""
from _z import *

STUB = "z8 заглушка"
rh = int(sql("SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')"))
sales = lambda who="admin1", q="": api("GET", f"/admin/sales{q}", None, who)


def truth(day):
    """то же окно клубных суток, что у ленты, — суммы прямо из базы"""
    f = at_club(day, f"{rh:02d}:00")
    t = at_club(sql(f"SELECT ('{day}'::date + 1)::text"), f"{rh:02d}:00")  # сутки по стенным часам — в дни перевода 23 и 25 ч
    paid = f"voided_at IS NULL AND paid_at >= {f} AND paid_at < {t}"
    r = sql(f"SELECT COUNT(*) || '|' || COUNT(*) FILTER (WHERE voided_at IS NOT NULL) || '|' || "
            f"COUNT(*) FILTER (WHERE voided_at IS NULL AND paid_at IS NULL) || '|' || "
            f"COALESCE(SUM(total_pln) FILTER (WHERE voided_at IS NULL AND paid_at IS NULL), 0) || '|' || "
            f"COALESCE(SUM(total_pln) FILTER (WHERE {paid} AND method <> 'wallet'), 0) || '|' || "
            f"COALESCE(SUM(total_pln) FILTER (WHERE {paid} AND method = 'wallet'), 0) FROM sales "
            f"WHERE (created_at >= {f} AND created_at < {t}) OR (paid_at >= {f} AND paid_at < {t}) OR (paid_at IS NULL AND voided_at IS NULL)")
    n, voided, unpaid, unpaid_pln, total, wallet = r.split("|")
    return {"total_count": int(n), "voided": int(voided), "unpaid": int(unpaid), "unpaid_pln": float(unpaid_pln),
            "total_pln": float(total), "wallet_pln": float(wallet)}


def same(d, t):
    """итог ленты = база: деньги — до гроша, счётчики — точно"""
    return all(near(d.get(k, -1), t[k]) if k.endswith("_pln") else d.get(k) == t[k] for k in t)


shown = lambda d: {k: d.get(k) for k in ("total_count", "count", "voided", "unpaid", "unpaid_pln", "total_pln", "wallet_pln")}

sql(f"DELETE FROM sales WHERE name = '{STUB}'")  # хвост упавшего прогона
section("итог ленты — по всему окну клубных суток")
s, d0 = sales()
t0 = truth(club_day(0))
check("admin1 → 200: оплачено, кошельком, в долг, отменено и число строк — как в базе за сутки", s == 200 and same(d0, t0),
      J({"лента": shown(d0), "база": t0}, 500))
check("в списке — не больше 300 последних строк", d0.get("count") == min(t0["total_count"], 300) == len(d0.get("sales", [])),
      f"{d0.get('count')} / {t0['total_count']}")

section("лента полная: 300 отменённых заглушек сверху — итог прежний")
boss = uid("boss")
at_end(lambda: sql(f"DELETE FROM sales WHERE name = '{STUB}'"))
sql(f"INSERT INTO sales (club_id, name, qty, price_pln, total_pln, method, created_by, voided_at, voided_by, created_at) "
    f"SELECT (SELECT id FROM clubs ORDER BY created_at LIMIT 1), '{STUB}', 1, 1, 1, 'card', '{boss}', now(), '{boss}', "
    f"now() + g * interval '1 millisecond' FROM generate_series(1, 300) AS g")
s, d1 = sales()
t1 = truth(club_day(0))
check("в списке 300 строк — все заглушки", d1.get("count") == 300 and all(x.get("name") == STUB for x in d1.get("sales", [])),
      f"{d1.get('count')} · {sorted({x.get('name') for x in d1.get('sales', [])})[:3]}")
check("итог — как в базе: оплачено, кошельком и в долг — прежние, отменено +300, строк суток +300",
      same(d1, t1) and near(d1.get("total_pln", -1), d0.get("total_pln", -2)) and near(d1.get("wallet_pln", -1), d0.get("wallet_pln", -2))
      and d1.get("unpaid") == d0.get("unpaid") and d1.get("voided") == (d0.get("voided") or 0) + 300
      and d1.get("total_count") == (d0.get("total_count") or 0) + 300, J({"до": shown(d0), "после": shown(d1)}, 500))
s, d2 = sales("boss")
check("владелец видит те же итоги", s == 200 and same(d2, t1), J(shown(d2)))

section("владелец листает любой день")
s, d3 = sales("boss", f"?date={club_day(-1)}")
t3 = truth(club_day(-1))
check(f"вчера ({club_day(-1)}): итог — как в базе за те сутки", s == 200 and d3.get("date") == club_day(-1) and same(d3, t3),
      J({"лента": shown(d3), "база": t3}, 500))
done()
