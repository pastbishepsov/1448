// З23-и1б «ABC и маржа — экран» (№48), браузер. «Отчёты» → «Товары»: плитки классов (A — основа продаж, B, C, без
// продаж) и итог «продано на» и маржа, фильтр по классу, таблица позиций — класс значком, продано (шт · zł), доля в
// продажах и штуках, в скольких днях периода продажи были, дней без продаж, склад и на сколько дней хватит, маржа; над
// отчётом — период без «сравнения с»; телефон 360 — карточки в экране; палитра «abc». Данные — как у e2e_z23_abc:
// позиции «z23 …» и их продажи от владельца картой, сдвинутые SQL-ом в пустую неделю 2024 года; в конце продажи
// отменяет владелец, позиции удаляются SQL-ом (dropGood).
const z = require('./_z');
(async () => {
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const wipe = () => { for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE 'z23 %'`)) z.dropGood(id); };
  wipe();
  z.atEnd(wipe); // в конце — после отмены продаж (уборка идёт в обратном порядке)
  const sales = [];
  z.atEnd(async () => { for (const s of sales) if (s) await z.api('POST', `/admin/sales/${s}/void`, {}, 'boss'); });
  // пустая неделя в прошлом: понедельник, от которого 7 дней без живых продаж (свои прошлые — отменены)
  const at = (d, hm) => `(('${d}'::date + time '${hm}') AT TIME ZONE 'Europe/Warsaw')`;
  let week = '';
  for (let k = 0; k < 30 && !week; k++) {
    const d0 = z.sql(`SELECT (date '2024-02-05' - ${7 * k})::text`);
    if (z.sql(`SELECT count(*) FROM sales WHERE voided_at IS NULL AND created_at >= ${at(d0, '00:00')} - interval '1 day'
      AND created_at < ${at(d0, '00:00')} + interval '8 days'`) === '0') week = d0;
  }
  const d1 = z.sql(`SELECT ('${week}'::date + 1)::text`), d6 = z.sql(`SELECT ('${week}'::date + 6)::text`);
  const ids = {};
  for (const [name, price] of [['z23 Альфа', 10], ['z23 Бета', 5], ['z23 Гамма', 10], ['z23 Дельта', 5], ['z23 Тишина', 4]])
    ids[name] = (((await z.api('POST', '/admin/goods', { name, category: 'z23', price_pln: price, stock: 50 }, 'boss')).data || {}).good || {}).id;
  const sell = async (name, qty, day, hm) => {
    const id = ((((await z.api('POST', '/admin/sales', { good_id: ids[name], method: 'card', qty }, 'boss')).data) || {}).sale || {}).id;
    sales.push(id);
    z.sql(`UPDATE sales SET created_at = ${at(day, hm)}, paid_at = ${at(day, hm)} WHERE id = '${id}'`);
  };
  await sell('z23 Альфа', 7, week, '12:00'); await sell('z23 Бета', 3, week, '12:00');
  await sell('z23 Гамма', 1, week, '12:00'); await sell('z23 Дельта', 1, week, '12:00'); await sell('z23 Альфа', 1, d1, '18:30');

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'stats');
  const settle = page => page.waitForFunction(() => { const b = document.getElementById('repBody'); return b && !b.querySelector('.skel'); },
    null, { timeout: 20000 }).catch(() => {}).then(() => z.sleep(500));
  const open = async page => {
    const has = await page.evaluate(([f, t]) => {
      const b = document.querySelector('#repTabs button[data-t="goods"]');
      if (!b) return false;
      repPeriod = { from: f, to: t };
      repTabSet('goods', b);
      return true;
    }, [week, d6]);
    if (has) await settle(page);
    return has;
  };
  if (!await open(boss)) { // вкладки нет (админка до З23-и1б) — FAIL по каждой проверке, а не обрыв
    for (const n of ['вкладка и период', 'плитки классов', 'строки позиций', 'фильтр по классу', 'телефон 360', 'палитра'])
      z.check(n + ': вкладки «Товары» нет', false);
    return z.done();
  }
  const read = page => page.evaluate(() => ({
    label: (document.getElementById('repLabel') || {}).textContent || '',
    on: (document.querySelector('#repTabs button.on') || {}).dataset ? document.querySelector('#repTabs button.on').dataset.t : '',
    kpis: [...document.querySelectorAll('#repAbc .kpi')].map(k => k.textContent.replace(/\s+/g, ' ').trim()),
    rows: [...document.querySelectorAll('#repBody .abcTable tr[data-g]')].map(r => ({ none: r.classList.contains('abcNone'),
      td: [...r.querySelectorAll('td')].map(td => { const c = td.cloneNode(true); c.querySelectorAll('.repL').forEach(x => x.remove());
        return c.textContent.replace(/\s+/g, ' ').trim(); }) })),
  }));
  const v = await read(boss);
  z.check(`вкладка «Товары» открыта, над отчётом — «${norm(v.label)}»: период недели без «сравнения с»`,
    v.on === 'goods' && /· 7 дней$/.test(norm(v.label)) && !/сравнение/.test(v.label), JSON.stringify(v.label));
  z.check(`плитки: «${v.kpis.join(' · ')}» — A 2 позиции, B 1, C 1`,
    v.kpis.length === 4 && /^A — основа продаж 2 позиции/.test(v.kpis[0]) && /^B — середина 1 позиция/.test(v.kpis[1])
    && /^C — хвост 1 позиция/.test(v.kpis[2]) && /^Без продаж \d+ позици/.test(v.kpis[3]), JSON.stringify(v.kpis));
  const mine = v.rows.filter(r => /^z23 /.test(r.td[0]));
  const a = (mine.find(r => /^z23 Альфа/.test(r.td[0])) || { td: [] }).td, t = mine.find(r => /^z23 Тишина/.test(r.td[0])) || { td: [] };
  z.check(`строка «Альфы»: «${a.join(' | ')}» — A, 8 шт · 80,00 zł, 72,7% · штук 61,5%, 2 из 7 дней, склад 42 шт · хватит на 36,8 дн`,
    a[1] === 'A' && a[2] === '8 шт · 80,00 zł' && a[3] === '72,7% · штук 61,5%' && a[4] === '2 из 7 дней'
    && a[6] === '42 шт · хватит на 36,8 дн' && /дней$|дня$|день$/.test(a[5]), JSON.stringify(a));
  z.check(`классы позиций проверки: ${mine.map(r => r.td[1]).join(', ')}; «Тишина» без продаж — бледная, «—», «продаж не было»`,
    JSON.stringify(mine.map(r => r.td[0].replace(/ ·.*$/, '') + ':' + r.td[1])) === JSON.stringify(['z23 Альфа:A', 'z23 Бета:A',
      'z23 Гамма:B', 'z23 Дельта:C', 'z23 Тишина:—']) && t.none && t.td[6] === '50 шт · продаж не было' && t.td[5] === '—',
    JSON.stringify(mine.map(r => r.td.slice(0, 2).concat(r.td.slice(5, 7)))));
  await z.shot(boss, 'z23_abc_desktop');

  // фильтр по классу — без запроса
  const by = async c => { await boss.evaluate(c => repGoodsFilter(c), c); return read(boss); };
  const fc = await by('C'), fn = await by('-'), fa = await by('');
  z.check('фильтр: «C» — одна «Дельта» (у стенда в эту неделю продаж нет), «Без продаж» — только бледные, «Все» — снова все',
    fc.rows.length === 1 && /^z23 Дельта/.test(fc.rows[0].td[0]) && fn.rows.length >= 1 && fn.rows.every(r => r.none)
    && fn.rows.some(r => /^z23 Тишина/.test(r.td[0])) && fa.rows.length === v.rows.length,
    JSON.stringify({ c: fc.rows.map(r => r.td[0]), none: fn.rows.length, all: fa.rows.length }));

  // телефон
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'stats');
  await open(ph);
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
    r: [...document.querySelectorAll('#repBody .abcTable tr[data-g]')].map(e => Math.round(e.getBoundingClientRect().right)),
    lab: [...document.querySelectorAll('#repBody .abcTable tr[data-g] .repL')].filter(e => getComputedStyle(e).display !== 'none').length }));
  await ph.evaluate(() => { const e = document.querySelector('#repBody .abcTable'); if (e) e.scrollIntoView(); });
  await z.shot(ph, 'z23_abc_phone');
  z.check('телефон 360: строки — карточки в экране, у чисел подписи, вбок не прокручивается',
    fit.r.length > 0 && fit.r.every(x => x <= fit.w) && fit.sw <= fit.w && fit.lab > 0, JSON.stringify({ ...fit, r: fit.r.slice(0, 5) }));

  // палитра: «abc» → «Товары: ABC» → «Отчёты» → «Товары»
  await boss.evaluate(() => { repTab = 'money'; nav('hall'); });
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'abc'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent.trim()).slice(0, 3));
  await boss.keyboard.press('Enter');
  await settle(boss);
  const pal = await boss.evaluate(() => ({ tab: repTab, on: (document.querySelector('#repTabs button.on') || {}).textContent || '',
    abc: !!document.getElementById('repAbc') }));
  z.check(`палитра: «abc» → «${items[0] || '—'}», Enter — «Отчёты» → «Товары»`,
    items[0] === 'Товары: ABC' && pal.tab === 'goods' && pal.on === 'Товары' && pal.abc, JSON.stringify({ items, pal }));
  await z.done();
})().catch(z.crash);
