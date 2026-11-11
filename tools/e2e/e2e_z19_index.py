#!/usr/bin/env python3
"""З19-и4 «Индекс движений склада по продаже» (из «Найдено по дороге» З10-и8; решение основателя 07.11). Миграция 095 —
частичный индекс stock_moves (sale_id) WHERE sale_id IS NOT NULL: поиск движений продажи — отмена (lotsBack) и маржа
(движения продаж периода) — идёт по нему, а не полным проходом (план запроса — EXPLAIN с выключенным seq scan, чтобы
маленькая таблица стенда не прятала индекс). Отмена и маржа — как были: продажа 2 шт и её отмена возвращают остаток, у
продажи два движения (−2 и +2), маржа «Денег» за сегодня — как до продажи. Позиция «z19i Вода» (остаток 10) и её
продажи удаляются в начале и в конце."""
from _z import *

GOOD = "z19i Вода"


def wipe():
    sql(f"DELETE FROM sales WHERE name = '{GOOD}'")
    for gid in rows(f"SELECT id FROM goods WHERE name = '{GOOD}'"):
        drop_good(gid)


wipe()
at_end(wipe)
plan = lambda q: " ".join(rows("SET enable_seqscan = off; EXPLAIN " + q))

section("индекс")
idx = sql("SELECT indexdef FROM pg_indexes WHERE indexname = 'idx_stock_moves_sale'")
check("миграция 095: индекс idx_stock_moves_sale — stock_moves (sale_id) WHERE sale_id IS NOT NULL",
      "(sale_id)" in idx and "WHERE (sale_id IS NOT NULL)" in idx, idx)

section("отмена и маржа — как были, движения продажи ищутся по индексу")
m0 = api("GET", "/admin/reports/money?preset=today", None, "boss")[1].get("goods_margin") or {}
gid = (api("POST", "/admin/goods", {"name": GOOD, "category": "z19i", "price_pln": 6, "stock": 10}, "boss")[1].get("good") or {}).get("id")
s, d = api("POST", "/admin/sales", {"good_id": gid, "qty": 2, "method": "card"}, "admin1")
sale = (d.get("sale") or d).get("id") or d.get("sale_id") or sql(f"SELECT id FROM sales WHERE name = '{GOOD}' ORDER BY created_at DESC LIMIT 1")
p1 = plan(f"SELECT id FROM stock_moves WHERE sale_id = '{sale}'")
p2 = plan(f"SELECT m.id FROM sales s JOIN stock_moves m ON m.sale_id = s.id WHERE s.id = '{sale}'")
check("поиск движений продажи (как у отмены) и продажа → её движения (как у маржи) — по idx_stock_moves_sale",
      "idx_stock_moves_sale" in p1 and "idx_stock_moves_sale" in p2, J([p1[:200], p2[:200]]))
left = lambda: sql(f"SELECT stock FROM goods WHERE id = '{gid}'")
st1 = left()
sv = api("POST", f"/admin/sales/{sale}/void", {}, "boss")[0]
mv = rows(f"SELECT delta FROM stock_moves WHERE sale_id = '{sale}' ORDER BY created_at")
m1 = api("GET", "/admin/reports/money?preset=today", None, "boss")[1].get("goods_margin") or {}
check("продажа 2 шт — остаток 8, отмена — 200 и снова 10; у продажи два движения: −2 и +2; маржа «Денег» за сегодня — как до продажи",
      s in (200, 201) and st1 == "8" and sv == 200 and left() == "10" and mv == ["-2", "2"]
      and [m0.get(k) for k in ("sold_pln", "cost_pln", "margin_pln")] == [m1.get(k) for k in ("sold_pln", "cost_pln", "margin_pln")],
      J([s, st1, sv, left(), mv, m0, m1], 500))
done()
