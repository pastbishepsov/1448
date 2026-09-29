// З10-и8 «Маржа и стоимость склада», браузер (№70; решение основателя 29.09). Владелец: «Отчёты» → «Деньги» — ряды «Продано
// товаров на · Себестоимость проданного · Маржа товаров» и «Стоимость склада сейчас» — как в отчёте сервера за тот же период;
// «Что покупают» — «Выручка · Продано на · Штук · Себестоимость · Маржа»: позицию брали и с кошелька — маржа от цены продажи
// всех штук; продали дешевле закупки — маржа в минусе, красным; «По категориям» — то же по категории. Палитра «маржа» →
// «Маржа и склад» → «Деньги», ряд маржи на экране. Админ: в «Отчётах» ни себестоимости, ни маржи, в палитре их нет. Телефон
// 360 — строки карточками с подписями у чисел, без горизонтальной прокрутки.
// Данные: позиции «z10p Маржа окно» (300 zł; поставка 4 шт — 2 × 100 + 2 × 150) и «z10p Маржа минус» (900 zł; поставка 1 шт
// по 1 000) в категории «z10p Маржа» заводятся каждый прогон; продажи — картой и кухня с кошелька Guest40 (кошелёк
// докладывается до 300 zł). В конце заказ кухни отменяется (деньги — на кошелёк), продажи отменяются, позиции удаляются
// вместе с поставками, расходы поставок сторнируются.
const z = require('./_z');
(async () => {
  const A = 'z10p Маржа окно', B = 'z10p Маржа минус', CAT = 'z10p Маржа', MARK = 'z10p маржа', GUEST = 'Guest40';
  const gone = async () => {
    for (const oid of z.rows(`SELECT id FROM kitchen_orders WHERE name IN ('${A}', '${B}') AND status IN ('new','accepted','preparing','delivering')`))
      await z.api('POST', `/admin/kitchen/orders/${oid}/cancel`, {}, 'boss');
    for (const sid of z.rows(`SELECT id FROM sales WHERE name IN ('${A}', '${B}') AND voided_at IS NULL`))
      await z.api('POST', `/admin/sales/${sid}/void`, {}, 'boss');
    for (const eid of z.rows(`SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%z10p Маржа%'`))
      await z.api('POST', `/admin/expenses/${eid}/storno`, { reason: 'уборка проверки' }, 'boss');
    for (const n of [A, B]) for (const id of z.rows(`SELECT id FROM goods WHERE name = '${n}'`)) z.dropGood(id);
  };
  await gone(); z.atEnd(gone);
  const guest = z.uid(GUEST), need = 300 - +z.sql(`SELECT wallet_grosz FROM users WHERE id = '${guest}'`) / 100;
  if (need > 0) await z.api('POST', `/admin/users/${guest}/deposit`, { amount_pln: need, method: 'card' }, 'boss');
  const mk = async (name, price) => ((await z.api('POST', '/admin/goods', { name, category: CAT, price_pln: price }, 'boss')).data.good || {}).id;
  const ga = await mk(A, 300), gb = await mk(B, 900);
  const receive = async (gid, n) => ((await z.api('POST', '/admin/supplies', { lines: [{ good_id: gid, qty: n }], note: `${MARK}: приход` }, 'admin1')).data.supply || {}).id;
  const sa = await receive(ga, 4), sb = await receive(gb, 1);
  const priced = [(await z.api('PUT', `/admin/supplies/${sa}/pricing`, { lines: [{ good_id: ga, prices: [{ qty: 2, price_pln: 100 }, { qty: 2, price_pln: 150 }] }],
    method: 'bank' }, 'boss')).status, (await z.api('PUT', `/admin/supplies/${sb}/pricing`, { lines: [{ good_id: gb, prices: [{ qty: 1, price_pln: 1000 }] }],
    method: 'bank' }, 'boss')).status];
  const sale = async (gid, method) => (await z.api('POST', '/admin/sales', { good_id: gid, qty: 1, method }, 'admin1')).status;
  const sold = [await sale(ga, 'card'), (await z.api('POST', '/admin/kitchen/orders', { guest_id: guest, good_id: ga, qty: 1, method: 'wallet' }, 'admin1')).status,
    await sale(ga, 'blik'), await sale(gb, 'card')];
  if (!z.check('данные: поставки оценены; «окно» — карта, кухня с кошелька, BLIK; «минус» — карта', [...priced, ...sold].join() === '200,200,201,201,201,201',
    [...priced, ...sold].join())) return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const fmt = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł');
  const mtxt = x => x.margin_pln == null ? '—' : fmt(x.margin_pln) + (x.margin_pct == null ? '' : ' · ' + x.margin_pct.toLocaleString('ru-RU') + '%');
  const boss = await z.openAdmin('boss');
  await boss.evaluate(() => palGoReport('money')); await z.sleep(2500);
  const rep = (await z.api('GET', '/admin/reports/money?' + await boss.evaluate(() => repQuery()), undefined, 'boss')).data; // тот же период, что на экране
  const gm = rep.goods_margin || {}, sv = rep.stock_value || {};
  const kpis = await boss.evaluate(() => Object.fromEntries([...document.querySelectorAll('#repMargin .kpi, #repStock .kpi')].map(k =>
    [k.firstChild.textContent.trim(), k.querySelector('b').innerText.replace(/\s+/g, ' ').trim()])));
  z.check('«Деньги»: «Продано товаров на», «Себестоимость проданного», «Маржа товаров», «Стоимость склада сейчас» — как в отчёте сервера',
    kpis['Продано товаров на'] === fmt(gm.sold_pln) && kpis['Себестоимость проданного'] === fmt(gm.cost_pln) && kpis['Маржа товаров'] === mtxt(gm)
    && kpis['Стоимость склада сейчас'] === fmt(sv.cost_pln) && gm.cost_pln >= 1350 && sv.cost_pln >= 150, JSON.stringify({ kpis, gm, sv }));

  const table = sect => boss.evaluate(s => { // первая таблица после заголовка раздела
    let e = [...document.querySelectorAll('#repBody .sect')].find(x => x.textContent === s), t = null;
    while (e && !t && (e = e.nextElementSibling) && !e.classList.contains('sect')) t = e.querySelector('table');
    return t ? [...t.rows].map(r => ({ cells: [...r.cells].map(c => (c.tagName === 'TH' ? c.textContent : c.innerText).replace(/\s+/g, ' ').trim()), // шапка — прописными стилем
      neg: !!r.querySelector('.repNeg') })) : null; }, sect);
  const buy = await table('Что покупают'), cats = await table('По категориям');
  const rowOf = (t, n) => (t || []).find(r => r.cells[0] === n) || {};
  z.check('«Что покупают»: колонки «Позиция · Выручка · Продано на · Штук · Себестоимость · Маржа»; «окно» — выручка 600,00 (карта и BLIK), продано на '
    + '900,00 (и кухня с кошелька), 3 шт, себестоимость 350,00 (100 + 100 + 150), маржа 550,00 · 61,1%',
    buy && buy[0].cells.join('|') === 'Позиция|Выручка|Продано на|Штук|Себестоимость|Маржа'
    && norm((rowOf(buy, A).cells || []).join('|')) === norm(`${A}|600,00 zł|900,00 zł|3|350,00 zł|550,00 zł · 61,1%`) && !rowOf(buy, A).neg,
    JSON.stringify([buy && buy[0], rowOf(buy, A)]));
  z.check('«минус» продан дешевле закупки — маржа -100,00 · -11,1%, красным; «По категориям» — «z10p Маржа»: 1 500,00 · 1 800,00 · 4 · 1 350,00 · 450,00 · 25%',
    norm(rowOf(buy, B).cells && rowOf(buy, B).cells.join('|')) === norm(`${B}|900,00 zł|900,00 zł|1|1 000,00 zł|-100,00 zł · -11,1%`) && rowOf(buy, B).neg
    && norm(rowOf(cats, CAT).cells && rowOf(cats, CAT).cells.join('|')) === norm(`${CAT}|1 500,00 zł|1 800,00 zł|4|1 350,00 zł|450,00 zł · 25%`) && !rowOf(cats, CAT).neg,
    JSON.stringify([rowOf(buy, B), rowOf(cats, CAT)]));
  await boss.locator('#repBody .sect', { hasText: 'Что покупают' }).scrollIntoViewIfNeeded();
  await boss.evaluate(() => { const e = document.getElementById('repMargin'); if (e) e.scrollIntoView({ block: 'center' }); }); await z.sleep(300);
  await z.shot(boss, 'z10_margin_money');

  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'маржа'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(3000);
  const pg = await boss.evaluate(() => { const r = (document.getElementById('repMargin') || { getBoundingClientRect: () => ({ top: -1, bottom: 1e9 }) }).getBoundingClientRect();
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight }; });
  z.check('палитра владельца: «маржа» → «Маржа и склад», Enter — «Отчёты» → «Деньги», ряд маржи на экране',
    items[0] === 'Маржа и склад' && pg.tab === 'stats' && pg.rep === 'money' && pg.seen, JSON.stringify({ items, pg }));

  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'stats');
  const abody = await adm.evaluate(() => !!document.getElementById('repMargin') // до палитры: в ней — слово запроса; в ленте — имена позиций проверки
    || /Себестоимость проданного|Маржа товаров|Стоимость склада сейчас|По категориям|Продано на/.test(document.body.innerText));
  await adm.evaluate(() => palOpen()); await adm.fill('#palInp', 'маржа'); await z.sleep(500);
  const ai = { body: abody, pal: await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent)) };
  await adm.keyboard.press('Escape');
  z.check('админ: в «Отчётах» ни себестоимости, ни маржи; «Маржи и склада» в палитре нет', !ai.body && !ai.pal.includes('Маржа и склад'), JSON.stringify(ai));

  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => palGoReport('money')); await z.sleep(3000);
  const pk = await ph.evaluate(() => { const r = document.getElementById('repStock'); if (r) r.scrollIntoView({ block: 'center' });
    return !!r && [...document.querySelectorAll('#repMargin .kpi, #repStock .kpi')].every(k => k.getBoundingClientRect().right <= innerWidth); });
  await z.sleep(300);
  await z.shot(ph, 'z10_margin_phone_kpi');
  const card = await ph.evaluate(n => { const r = [...document.querySelectorAll('#repBody tr')].find(t => t.cells[0] && t.cells[0].innerText.trim() === n);
    if (r) r.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: !!r && r.getBoundingClientRect().right <= innerWidth,
      labels: r ? [...r.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) : [],
      text: r ? [...r.cells].map(c => [...c.childNodes].map(x => x.textContent.trim()).filter(Boolean).join(' ')).join(' | ') : null }; }, A);
  await z.sleep(300);
  await z.shot(ph, 'z10_margin_phone');
  z.check('телефон 360: «Что покупают» — карточка с подписями у чисел («выручка · продано на · штук · себестоимость · маржа»), без горизонтальной прокрутки',
    card.sw <= 360 && card.fit && pk && card.labels.join('|') === 'выручка|продано на|штук|себестоимость|маржа'
    && norm(card.text) === norm(`${A} | выручка 600,00 zł | продано на 900,00 zł | штук 3 | себестоимость 350,00 zł | маржа 550,00 zł · 61,1%`),
    JSON.stringify({ card, pk }));
  await z.done();
})().catch(z.crash);
