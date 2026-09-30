// З11-и2 «Способ возврата — экран», браузер (№17; решения основателя 30.09). Первое нажатие «Отменить» раскрывает выбор на
// месте кнопки: «Вернуть N zł <как платили>» (быстрый путь) и «Не возвращали»; у владельца — ещё «Другим способом…»:
// наличными, на карту, по BLIK (кошелёк — только как платили кошельком, тогда это главная кнопка). Тост говорит, что делать с
// деньгами. Продажа, чек, депозит, пакет, заказ кухни; выбор сворачивается сам через 6 с. Телефон 360 — без прокрутки вбок.
// Данные: временный админ z11rp (на смене); Guest42 (кошелёк докладывается до 70 zł); позиции «z11p Возврат A» (10 zł) и
// «z11p Возврат B» (5 zł) заводятся каждый прогон и в конце удаляются вместе с поставками; самый дешёвый пакет каталога. Всё,
// что проверка продаёт, пополняет и выдаёт, отменяется по ходу или в уборке.
const z = require('./_z');
(async () => {
  const A = 'z11p Возврат A', B = 'z11p Возврат B', GUEST = 'Guest42';
  const gone = async () => {
    for (const oid of z.rows(`SELECT id FROM kitchen_orders WHERE name IN ('${A}', '${B}') AND status IN ('new','accepted','preparing','delivering')`))
      await z.api('POST', `/admin/kitchen/orders/${oid}/cancel`, {}, 'boss');
    for (const sid of z.rows(`SELECT id FROM sales WHERE name IN ('${A}', '${B}') AND voided_at IS NULL`))
      await z.api('POST', `/admin/sales/${sid}/void`, {}, 'boss');
    for (const n of [A, B]) for (const id of z.rows(`SELECT id FROM goods WHERE name = '${n}'`)) z.dropGood(id);
  };
  await gone(); z.atEnd(gone);
  await z.hireTemp('z11rp');
  const g = z.uid(GUEST), deps = [], packs = [];
  z.atEnd(async () => {
    for (const d of deps) if (z.sql(`SELECT voided_at IS NULL FROM deposits WHERE id = '${d}'`) === 't') await z.api('POST', `/admin/deposits/${d}/void`, {}, 'boss');
    for (const p of packs) if (z.sql(`SELECT voided_at IS NULL FROM user_packages WHERE id = '${p}'`) === 't') await z.api('POST', `/admin/user-packages/${p}/void`, {}, 'boss');
  });
  const need = 70 - +z.sql(`SELECT wallet_grosz FROM users WHERE id = '${g}'`) / 100;
  if (need > 0) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: need, method: 'card' }, 'boss');
  const mk = async (name, price) => ((await z.api('POST', '/admin/goods', { name, category: 'z11', price_pln: price, stock: 20 }, 'boss')).data.good || {}).id;
  const ga = await mk(A, 10), gb = await mk(B, 5);
  const sell = async method => ((await z.api('POST', '/admin/sales', { good_id: ga, qty: 1, method }, 'z11rp')).data.sale || {}).id;
  const kitchen = async () => (await z.api('POST', '/admin/kitchen/orders', { guest_id: g, good_id: ga, qty: 1, method: 'wallet' }, 'z11rp')).data;
  const rm = (t, id) => z.sql(`SELECT COALESCE(refund_method, '-') FROM ${t} WHERE id = '${id}'`);
  const toast = async (p, ms = 1500) => { await z.sleep(ms); return (await z.text(p, '#toast')) || ''; };
  // первое нажатие «Отменить» у строки с этим id: ячейка помечается t, в ответ — надписи кнопок выбора (выбора нет — кнопки ячейки)
  const open = (p, id, t) => p.evaluate(([i, tag]) => { const b = document.querySelector(`button[onclick^="arm"][onclick*="'${i}'"]`); if (!b) return null;
    const cell = b.parentElement; cell.dataset.t = tag; b.click();
    return [...(cell.querySelector('.rfPick') || cell).querySelectorAll('button')].map(x => x.textContent); }, [id, t]);
  const btns = (p, t) => p.evaluate(tag => { const cell = document.querySelector(`[data-t="${tag}"]`);
    return cell ? [...(cell.querySelector('.rfPick') || cell).querySelectorAll('button')].map(x => x.textContent) : null; }, t);
  const choose = async (p, t, how) => { const sel = `[data-t="${t}"] .rfPick button[data-rf="${how}"]`; if (await p.$(sel)) await p.click(sel); };

  const s1 = await sell('cash'), s2 = await sell('card'), s3 = await sell('cash');
  const adm = await z.openAdmin('z11rp');
  await z.nav(adm, 'goods');
  const o1 = await open(adm, s1, 's1');
  await z.shot(adm, 'z11_refund_pick');
  await choose(adm, 's1', 'none');
  const t1 = await toast(adm);
  const o2 = await open(adm, s2, 's2');
  await choose(adm, 's2', 'card');
  const t2 = await toast(adm);
  z.check('админ, продажа налом: «Отменить» → «Вернуть 10,00 zł наличными» и «Не возвращали» (другого способа нет) → «Не возвращали» — '
    + 'тост «… · деньги не возвращаем»; картой → «Вернуть 10,00 zł на карту» → «верни гостю 10,00 zł на карту через терминал»',
    JSON.stringify(o1) === '["Вернуть 10,00 zł наличными","Не возвращали"]' && /^Продажа отменена — товар вернулся на склад · деньги не возвращаем$/.test(t1)
    && rm('sales', s1) === 'none' && JSON.stringify(o2) === '["Вернуть 10,00 zł на карту","Не возвращали"]'
    && /· верни гостю 10,00 zł на карту через терминал$/.test(t2) && rm('sales', s2) === 'card', JSON.stringify({ o1, t1, o2, t2 }));
  await open(adm, s3, 's3');
  await z.sleep(6600);
  const back = await btns(adm, 's3');
  z.check('выбор сам сворачивается через 6 с — на месте снова «Отменить», продажа цела', JSON.stringify(back) === '["Отменить"]'
    && z.sql(`SELECT voided_at IS NULL FROM sales WHERE id = '${s3}'`) === 't', JSON.stringify(back));

  const d1 = ((await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: 20, method: 'cash' }, 'z11rp')).data || {}).deposit_id; deps.push(d1);
  const pack = z.sql('SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1');
  const price = +z.sql(`SELECT price_pln FROM time_packages WHERE id = '${pack}'`);
  const p1 = (((await z.api('POST', `/admin/users/${g}/packages`, { package_id: pack, method: 'card' }, 'z11rp')).data || {}).package || {}).id; packs.push(p1);
  await adm.evaluate(i => openUserDrawer(i), g); await z.sleep(1800);
  await adm.evaluate(() => uTabSet('deposits')); await z.sleep(500);
  const od = await open(adm, d1, 'd1');
  await choose(adm, 'd1', 'cash');
  const td = await toast(adm, 1800);
  await adm.evaluate(() => uTabSet('pack')); await z.sleep(500);
  const op = await open(adm, p1, 'p1');
  await z.shot(adm, 'z11_refund_pack');
  await choose(adm, 'p1', 'card');
  const tp = await toast(adm, 1800);
  const P = price.toLocaleString('ru-RU', { minimumFractionDigits: 2 });
  z.check(`карточка гостя: депозит налом → «Вернуть 20,00 zł наличными» → «… · верни гостю 20,00 zł наличными из ящика …»; пакет картой → «Вернуть ${P} zł на карту» → «… · верни гостю ${P} zł на карту через терминал»`,
    JSON.stringify(od) === '["Вернуть 20,00 zł наличными","Не возвращали"]' && td.includes('отменён — с кошелька снято 20,00 zł')
    && td.includes('· верни гостю 20,00 zł наличными из ящика — «должно быть» уже меньше на них') && rm('deposits', d1) === 'cash'
    && JSON.stringify(op) === `["Вернуть ${P} zł на карту","Не возвращали"]` && /^Выдача отменена — забрано \d+ мин · верни гостю .* zł на карту через терминал$/.test(tp)
    && tp.includes(`${P} zł на карту`) && rm('user_packages', p1) === 'card', JSON.stringify({ od, td, op, tp }));
  await adm.evaluate(() => closeDrawer());

  const k1 = await kitchen();
  await z.nav(adm, 'kitchen');
  const ok1 = await open(adm, (k1.order || {}).id, 'k1');
  await choose(adm, 'k1', 'wallet');
  const tk = await toast(adm);
  z.check('кухня, заказ с кошелька: «Вернуть 10,00 zł на кошелёк» → «Заказ отменён — 10,00 zł вернулись на кошелёк гостя, товар — на склад»',
    JSON.stringify(ok1) === '["Вернуть 10,00 zł на кошелёк","Не возвращали"]' && tk === 'Заказ отменён — 10,00 zł вернулись на кошелёк гостя, товар — на склад'
    && rm('sales', k1.sale_id) === 'wallet', JSON.stringify({ ok1, tk }));

  const r = (await z.api('POST', '/admin/sales', { items: [{ good_id: ga, qty: 1 }, { good_id: gb, qty: 1 }], method: 'cash' }, 'z11rp')).data;
  const rid = (r.receipt || {}).id, k2 = await kitchen();
  await adm.goto('about:blank'); await z.sleep(1500); // два окна с одного адреса упираются в лимитер стенда
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'goods');
  const or1 = await open(boss, rid, 'r1');
  await choose(boss, 'r1', 'more');
  const or2 = await btns(boss, 'r1');
  await z.shot(boss, 'z11_refund_owner');
  await choose(boss, 'r1', 'blik');
  const tr = await toast(boss);
  const w0 = +z.sql(`SELECT wallet_grosz FROM users WHERE id = '${g}'`);
  await z.nav(boss, 'kitchen');
  await open(boss, (k2.order || {}).id, 'k2');
  await choose(boss, 'k2', 'more');
  const ok2 = await btns(boss, 'k2');
  await choose(boss, 'k2', 'cash');
  const tk2 = await toast(boss);
  z.check('владелец: у чека налом — ещё «Другим способом…» → «на карту», «по BLIK» (не «наличными» и не кошелёк) → «по BLIK» — тост «… · верни гостю 15,00 zł по BLIK», '
    + 'у обеих позиций «blik»', JSON.stringify(or1) === '["Вернуть 15,00 zł наличными","Не возвращали","Другим способом…"]'
    && JSON.stringify(or2) === '["Вернуть 15,00 zł наличными","Не возвращали","на карту","по BLIK"]'
    && /^Чек отменён — 2 позиций вернулись на склад · верни гостю 15,00 zł по BLIK$/.test(tr)
    && z.rows(`SELECT refund_method FROM sales WHERE receipt_id = '${rid}'`).join() === 'blik,blik', JSON.stringify({ or1, or2, tr }));
  z.check('владелец, заказ с кошелька → «Другим способом…» → наличными, на карту, по BLIK → «наличными»: «верни гостю 10,00 zł наличными из ящика …», кошелёк не пополнился',
    JSON.stringify(ok2) === '["Вернуть 10,00 zł на кошелёк","Не возвращали","наличными","на карту","по BLIK"]'
    && tk2 === 'Заказ отменён — верни гостю 10,00 zł наличными из ящика — «должно быть» уже меньше на них, товар — на склад'
    && rm('sales', k2.sale_id) === 'cash' && +z.sql(`SELECT wallet_grosz FROM users WHERE id = '${g}'`) === w0, JSON.stringify({ ok2, tk2 }));

  await boss.goto('about:blank'); await z.sleep(2500);
  const s4 = await sell('cash');
  const ph = await z.openAdmin('z11rp', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'goods');
  const o4 = await open(ph, s4, 's4');
  const fit = await ph.evaluate(() => { const b = [...document.querySelectorAll('[data-t="s4"] .rfPick button')]; if (b[0]) b[0].scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, n: b.length, in: b.every(x => { const r = x.getBoundingClientRect(); return r.left >= 0 && r.right <= innerWidth; }) }; });
  await z.sleep(300);
  await z.shot(ph, 'z11_refund_phone');
  await choose(ph, 's4', 'cash');
  const t4 = await toast(ph);
  z.check('телефон 360: выбор в строке продажи — без прокрутки вбок, кнопки в экране; «Вернуть 10,00 zł наличными» → «верни гостю 10,00 zł наличными из ящика …»',
    fit.sw <= 360 && fit.n === 2 && fit.in && JSON.stringify(o4) === '["Вернуть 10,00 zł наличными","Не возвращали"]'
    && t4.endsWith('· верни гостю 10,00 zł наличными из ящика — «должно быть» уже меньше на них') && rm('sales', s4) === 'cash', JSON.stringify({ fit, o4, t4 }));
  await z.done();
})().catch(z.crash);
