// З23-и2 «Заказать» (№48), браузер. «Товары» у владельца: блок «Заказать» — по продажам последних 7 клубных суток
// остаток (и порог), продано за 7 дней, на сколько дней хватит, сколько заказать, чтобы хватило на N дней (и остаток
// был выше порога «заканчивается»); сверху — что кончится раньше. «В приёмку» — форма поставки с этими строками. Админу
// блока нет. Телефон 360 — в экране; палитра «заказать». Данные — позиции «z23o …», их продажи от владельца картой
// сдвинуты SQL-ом на позавчера; в конце продажи отменяет владелец, позиции удаляются SQL-ом (dropGood). Поставку
// проверка не сохраняет.
const z = require('./_z');
(async () => {
  const wipe = () => { for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE 'z23o %'`)) z.dropGood(id); };
  wipe();
  z.atEnd(wipe); // в конце — после отмены продаж (уборка идёт в обратном порядке)
  const sales = [];
  z.atEnd(async () => { for (const s of sales) if (s) await z.api('POST', `/admin/sales/${s}/void`, {}, 'boss'); });
  const d2 = z.clubDay(-2), at = `(('${d2}'::date + time '12:00') AT TIME ZONE 'Europe/Warsaw')`;
  const ids = {};
  for (const [name, stock, low] of [['z23o Быстро', 24, 3], ['z23o Медленно', 27, 0], ['z23o Порог', 2, 5], ['z23o Тихо', 10, 0]])
    ids[name] = (((await z.api('POST', '/admin/goods', { name, category: 'z23o', price_pln: 5, stock, low_stock: low }, 'boss')).data || {}).good || {}).id;
  for (const [name, qty] of [['z23o Быстро', 14], ['z23o Медленно', 7]]) {
    const id = ((((await z.api('POST', '/admin/sales', { good_id: ids[name], method: 'card', qty }, 'boss')).data) || {}).sale || {}).id;
    sales.push(id);
    z.sql(`UPDATE sales SET created_at = ${at}, paid_at = ${at} WHERE id = '${id}'`);
  }

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'goods');
  const ready = page => page.waitForFunction(() => { const b = document.getElementById('ordBox'); return b && !b.querySelector('.skel'); },
    null, { timeout: 15000 }).then(() => true).catch(() => false);
  if (!await ready(boss)) { // блока нет (админка до З23-и2) — FAIL по каждой проверке, а не обрыв
    for (const n of ['строки «Заказать»', 'порядок', 'на 7 дней', 'В приёмку', 'админу нет', 'телефон 360', 'палитра'])
      z.check(n + ': блока «Заказать» нет', false);
    return z.done();
  }
  const read = page => page.evaluate(() => [...document.querySelectorAll('#ordBox tr[data-g]')].map(r => ({ low: r.classList.contains('ordLow'),
    td: [...r.querySelectorAll('td')].map(td => { const c = td.cloneNode(true); c.querySelectorAll('.repL').forEach(x => x.remove());
      return c.textContent.replace(/\s+/g, ' ').trim(); }) })));
  const mine = rows => rows.filter(r => /^z23o /.test(r.td[0]));
  const row = (rows, n) => (rows.find(r => r.td[0].startsWith(n)) || { td: [] });
  let v = await read(boss);
  const b = row(v, 'z23o Быстро').td, p = row(v, 'z23o Порог'), m = row(v, 'z23o Медленно').td, t = row(v, 'z23o Тихо').td;
  z.check(`«Быстро»: «${b.slice(1).join(' | ')}» — 14 шт за 7 дней, хватит на 5 дн, на 14 дней — заказать 18 шт`,
    b[1] === '10 шт · порог 3' && b[2] === '14 шт' && b[3] === '5 дн' && b[4] === '18 шт', JSON.stringify(b));
  z.check(`«Порог»: «${p.td.slice(1).join(' | ')}» — не продавали, но остаток 2 при пороге 5 — оранжевым, заказать 4 (до 6)`,
    p.low && p.td[1] === '2 шт · порог 5' && p.td[3] === 'не продавали' && p.td[4] === '4 шт', JSON.stringify(p));
  z.check(`«Медленно» (хватит на 20 дн) и «Тихо» (не продавали, выше порога) — заказывать не нужно, «—»`,
    m[3] === '20 дн' && m[4] === '—' && t[3] === 'не продавали' && t[4] === '—', JSON.stringify({ m, t }));
  const order = mine(v).map(r => r.td[0].replace(/ ·.*$/, ''));
  z.check(`порядок: что кончится раньше — сверху, не продававшиеся — внизу: ${order.join(', ')}`,
    JSON.stringify(order) === JSON.stringify(['z23o Быстро', 'z23o Медленно', 'z23o Порог', 'z23o Тихо']), JSON.stringify(order));
  await boss.evaluate(() => { const e = document.getElementById('ordHead'); if (e) e.scrollIntoView(); });
  await z.shot(boss, 'z23_order');

  // на 7 дней
  await boss.fill('#ordDays', '7'); await z.sleep(300);
  v = await read(boss);
  z.check('«на 7 дней»: «Быстро» — 4 шт, «Порог» — 4 шт, «Медленно» — «—»',
    row(v, 'z23o Быстро').td[4] === '4 шт' && row(v, 'z23o Порог').td[4] === '4 шт' && row(v, 'z23o Медленно').td[4] === '—',
    JSON.stringify(mine(v).map(r => [r.td[0], r.td[4]])));

  // «В приёмку» — форма поставки с этими строками (не сохраняем)
  await boss.click('#ordToSup'); await z.sleep(800);
  const f = await boss.evaluate(() => ({ title: (document.getElementById('drawerTitle') || {}).textContent || '',
    open: document.getElementById('drawer').classList.contains('open'),
    lines: (supForm && supForm.lines || []).map(l => [((goodsData.find(g => g.id === l.good) || {}).name) || l.good, l.qty]),
    note: (document.getElementById('supNote') || {}).value || '' }));
  const fl = f.lines.filter(l => /^z23o /.test(l[0]));
  z.check(`«В приёмку» — «${f.title}» со строками ${JSON.stringify(fl)} и заметкой «${f.note}»`,
    f.open && f.title === 'Принять поставку' && JSON.stringify(fl.sort()) === JSON.stringify([['z23o Быстро', '4'], ['z23o Порог', '4']])
    && f.note === 'по списку «Заказать» на 7 дней', JSON.stringify(f));
  await boss.evaluate(() => closeDrawer());

  // админу блока нет
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'goods'); await z.sleep(1500);
  const a = await adm.evaluate(() => ({ head: !document.getElementById('ordHead').classList.contains('hidden'),
    box: !document.getElementById('ordBox').classList.contains('hidden') }));
  z.check('админу «Заказать» не виден', !a.head && !a.box, JSON.stringify(a));

  // телефон
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'goods'); await ready(ph);
  await ph.evaluate(() => { const e = document.getElementById('ordBox'); if (e) e.scrollIntoView(); });
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
    r: [...document.querySelectorAll('#ordBox tr[data-g]')].map(e => Math.round(e.getBoundingClientRect().right)) }));
  await z.shot(ph, 'z23_order_phone');
  z.check('телефон 360: строки — карточки в экране, вбок не прокручивается', fit.r.length > 0 && fit.r.every(x => x <= fit.w)
    && fit.sw <= fit.w, JSON.stringify({ ...fit, r: fit.r.slice(0, 5) }));

  // палитра
  await boss.evaluate(() => nav('hall'));
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'заказать'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent.trim()).slice(0, 3));
  await boss.keyboard.press('Enter'); await z.sleep(1500);
  const w = await boss.evaluate(() => ({ tab: curTab, top: Math.round(document.getElementById('ordHead').getBoundingClientRect().top),
    h: innerHeight }));
  z.check(`палитра: «заказать» → «${items[0] || '—'}», Enter — «Товары», блок «Заказать» на экране`,
    items[0] === 'Заказать' && w.tab === 'goods' && w.top >= 0 && w.top < w.h, JSON.stringify({ items, w }));
  await z.done();
})().catch(z.crash);
