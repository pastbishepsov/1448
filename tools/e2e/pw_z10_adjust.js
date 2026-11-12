// З10-и7 «Корректировки с выбором партии и «Потери» по себестоимости», браузер (№68, №70; решение основателя 29.09).
// Владелец: «Товары» → «Склад» → «Корректировка» — список партий «откуда · осталось N из M · цена» (первым — «со старых
// партий первыми»); выбрал «нашлось», −2 → тост «Склад обновлён · из партии «нашлось …»», списано ровно оттуда. Админ: в той
// же шторке партии нет. «Бухгалтерия» → «Закупки» → «Начальный остаток и «нашлось»»: цена за штуку, «Записать» — тост «Цена
// партии: … — 1,50 zł за штуку». «Отчёты» → «Деньги»: «— склад по себестоимости» — как в отчёте сервера. Журнал «партия: цена».
// Телефон 360 — шторка и список без горизонтальной прокрутки.
// Данные: позиция «z10p Бой окно» (категория z10) заводится каждый прогон и в конце удаляется вместе с поставками; расход её
// поставки (заметка «… z10p Бой …») в конце сторнируется.
const z = require('./_z');
(async () => {
  const A = 'z10p Бой окно', MARK = 'z10p бой';
  const gone = async () => {
    for (const eid of z.rows(`SELECT id FROM expenses WHERE storno_at IS NULL AND note LIKE '%z10p Бой%'`))
      await z.api('POST', `/admin/expenses/${eid}/storno`, { reason: 'уборка проверки' }, 'boss');
    for (const id of z.rows(`SELECT id FROM goods WHERE name = '${A}'`)) z.dropGood(id);
  };
  await gone(); z.atEnd(gone);
  const ga = ((await z.api('POST', '/admin/goods', { name: A, category: 'z10', price_pln: 5 }, 'boss')).data.good || {}).id;
  const sid = ((await z.api('POST', '/admin/supplies', { lines: [{ good_id: ga, qty: 10 }], note: `${MARK}: приход` }, 'admin1')).data.supply || {}).id;
  await z.api('PUT', `/admin/supplies/${sid}/pricing`, { lines: [{ good_id: ga, prices: [{ qty: 10, price_pln: 2 }] }], method: 'bank' }, 'boss');
  await z.api('POST', `/admin/goods/${ga}/stock`, { delta: 3, reason: 'adjust', note: `${MARK}: нашлось` }, 'admin1'); // «нашлось» — партия без цены
  const found = z.sql(`SELECT id FROM stock_lots WHERE good_id = '${ga}' AND kind = 'found'`);
  const left = id => +z.sql(`SELECT qty_left FROM stock_lots WHERE id = '${id}'`);
  const after = (p, act, want) => z.toastAfter(p, act, want); // З19-и13: тост этого действия, а не #toast через паузу
  const openStock = async p => { await z.nav(p, 'goods'); await p.evaluate(i => stockDrawer(i), ga); await z.sleep(700);
    await p.click('#stk_adjust'); await z.sleep(1200); };

  const boss = await z.openAdmin('boss');
  await openStock(boss);
  const opts = await boss.evaluate(() => { const s = document.getElementById('stk_lot');
    return s ? { shown: !s.classList.contains('hidden'), texts: [...s.options].map(o => o.text) } : null; });
  if (!z.check('владелец: «Склад» → «Корректировка» — партии «откуда · осталось · цена», первым — «со старых партий первыми»',
    opts && opts.shown && opts.texts[0] === 'Со старых партий первыми (как у админа)' && /^поставка от \d\d\.\d\d \d\d:\d\d · осталось 10 из 10 · 2,00 zł\/шт$/.test(opts.texts[1])
    && /^нашлось \d\d\.\d\d \d\d:\d\d · осталось 3 из 3 · без цены$/.test(opts.texts[2]), JSON.stringify(opts))) return z.done();
  await boss.selectOption('#stk_lot', found);
  await boss.fill('#stk_delta', '-2'); await boss.fill('#stk_note', `${MARK}: разбили`);
  await z.shot(boss, 'z10_adjust_drawer');
  const t1 = await after(boss, () => boss.click('.uform button:has-text("Записать")'), 'Склад обновлён');
  z.check('−2 из «нашлось» → тост «Склад обновлён · из партии «нашлось …»»; списано ровно из неё (осталось 1), поставка не тронута',
    /^Склад обновлён · из партии «нашлось \d\d\.\d\d \d\d:\d\d»$/.test(t1) && left(found) === 1
    && z.sql(`SELECT qty_left FROM stock_lots WHERE good_id = '${ga}' AND kind = 'supply'`) === '10', JSON.stringify({ t1, found: left(found) }));

  const adm = await z.openAdmin('admin1');
  await openStock(adm);
  const aopt = await adm.evaluate(() => ({ lot: !!document.getElementById('stk_lot'), hint: (document.getElementById('stk_hint') || {}).textContent }));
  z.check('админ: в шторке «Склад» партии не выбрать — списание со старых партий первыми', !aopt.lot && !/партии/.test(aopt.hint), JSON.stringify(aopt));
  await adm.evaluate(() => closeDrawer());

  await boss.evaluate(() => accGo('pur')); await z.sleep(2200);
  const row = () => boss.evaluate(i => { const r = document.querySelector(`#purLots tr.purLot[data-id="${i}"]`);
    return r ? { cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()), price: r.querySelector('input').value } : null; }, found);
  const r0 = await row(), head = await z.text(boss, '#purLotsHead');
  await boss.evaluate(i => document.querySelector(`#purLots tr.purLot[data-id="${i}"]`).scrollIntoView({ block: 'center' }), found); await z.sleep(300);
  await z.shot(boss, 'z10_adjust_lots');
  await boss.fill(`#purLots tr.purLot[data-id="${found}"] input`, '1.5');
  const t2 = await after(boss, () => boss.click(`#purLots tr.purLot[data-id="${found}"] button`), 'Цена партии'), r1 = await row();
  z.check('«Закупки» → «Начальный остаток и «нашлось»»: партия без цены — «пришло 3 · осталось 1»; 1,50 → тост «Цена партии: … — 1,50 zł за штуку», цена записана',
    /^Начальный остаток и «нашлось» · без цены: \d+$/.test(head) && r0 && r0.cells[0] === A && /^нашлось /.test(r0.cells[1]) && r0.cells[2] === '3 · 1' && r0.price === ''
    && /^Цена партии: z10p Бой окно · нашлось \d\d\.\d\d \d\d:\d\d — 1,50 zł за штуку$/.test(t2) && r1 && r1.price === '1.5'
    && z.sql(`SELECT price_grosz FROM stock_lots WHERE id = '${found}'`) === '150', JSON.stringify({ head, r0, t2, r1 }));

  await boss.evaluate(() => palGoReport('money')); await z.sleep(2500);
  const q = await boss.evaluate(() => repQuery()); // тот же период, что на экране
  const api = (await z.api('GET', '/admin/reports/money?' + q, undefined, 'boss')).data.losses || {};
  const kpi = await boss.evaluate(() => { const k = [...document.querySelectorAll('#repBody .kpi')].find(x => x.textContent.startsWith('— склад по себестоимости'));
    return k ? { v: k.querySelector('b').textContent, title: k.title } : null; });
  const fmt = v => (+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł';
  z.check('«Отчёты» → «Деньги»: «— склад по себестоимости» — как в отчёте сервера, в подсказке штуки и по рознице',
    kpi && kpi.v === fmt(api.cost_pln) && kpi.title.includes(api.units + ' шт, по рознице ') && api.cost_pln >= 3, JSON.stringify({ kpi, api }));

  await z.nav(boss, 'grants');
  const jr = await boss.evaluate(n => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => t.includes('партия: цена') && t.includes(n)).slice(0, 2), A);
  z.check('«Журнал»: «партия: цена» — «… · 3 шт — 1,50 zł за штуку (было без цены)»', jr.some(t => t.includes('3 шт — 1,50 zł за штуку (было без цены)')), JSON.stringify(jr));

  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // два окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await openStock(ph);
  const p1 = await ph.evaluate(() => { const d = document.getElementById('drawer').getBoundingClientRect(), s = document.getElementById('stk_lot').getBoundingClientRect();
    return { sw: document.documentElement.scrollWidth, fit: s.right <= d.right + 1 && s.width > 0 }; });
  await z.shot(ph, 'z10_adjust_phone');
  await ph.evaluate(() => { closeDrawer(); accGo('pur'); }); await z.sleep(3000);
  const p2 = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, lots: !!document.getElementById('purLots') }));
  await ph.evaluate(() => document.getElementById('purLotsHead').scrollIntoView()); await z.sleep(300);
  await z.shot(ph, 'z10_adjust_phone_lots');
  z.check('телефон 360: шторка «Склад» с партией и «Начальный остаток и «нашлось»» — без горизонтальной прокрутки', p1.sw <= 360 && p1.fit && p2.sw <= 360 && p2.lots,
    JSON.stringify({ p1, p2 }));
  await z.done();
})().catch(z.crash);
