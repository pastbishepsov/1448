// З23-и4 «Инвентаризация — экран» (№54 «в»), браузер. «Товары» → «Инвентаризация»: «Пересчитать склад» — шторка с
// позициями, «в системе» и полем «насчитали»; разница — на лету, итог черновика внизу; пустое поле — позицию не
// считали. «Провести акт» (дважды): если остаток с начала пересчёта изменился — строки подсвечены «было → стало»,
// «в системе» обновилось, насчитанное осталось — пересчитать и провести ещё раз. Проведённый акт — шторкой и в списке
// актов. Админ: партии не выбирает, себестоимости и «Скачать» нет. Владелец: при недостаче — выбор партии, в акте —
// себестоимость, партии и «Скачать XLSX». «Деньги» — плитка «— по инвентаризациям»; журнал — «склад: инвентаризация»;
// палитра «инвентаризация»; телефон 360 — шторка в экране. Данные — позиции «z23v …» (категория z23v): остатки —
// корректировками «нашлось», цена партии — PUT /admin/lots/:id/price; акты проверки — с заметкой «z23v …»; в конце
// позиции удаляются SQL-ом (dropGood), акты — SQL-ом.
const z = require('./_z');
(async () => {
  const wipe = () => {
    for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE 'z23v %'`)) z.dropGood(id);
    z.sql(`DELETE FROM inventories WHERE note LIKE 'z23v%' AND NOT EXISTS (SELECT 1 FROM stock_moves m WHERE m.inventory_id = inventories.id)`);
  };
  wipe();
  z.atEnd(wipe);
  const ids = {};
  for (const [name, price] of [['z23v Кола', 5], ['z23v Чипсы', 8], ['z23v Вода', 3]])
    ids[name] = (((await z.api('POST', '/admin/goods', { name, category: 'z23v', price_pln: price }, 'boss')).data || {}).good || {}).id;
  const add = (id, n) => z.api('POST', `/admin/goods/${id}/stock`, { delta: n, reason: 'adjust', note: 'z23v: нашлось для проверки' }, 'boss');
  await add(ids['z23v Кола'], 6); await add(ids['z23v Кола'], 6); await add(ids['z23v Чипсы'], 4); await add(ids['z23v Вода'], 5);
  const lots = z.rows(`SELECT id FROM stock_lots WHERE good_id = '${ids['z23v Кола']}' ORDER BY received_at, part, id`);
  await z.api('PUT', `/admin/lots/${lots[0]}/price`, { price_pln: 2 }, 'boss');
  await z.api('PUT', `/admin/lots/${lots[1]}/price`, { price_pln: 2.5 }, 'boss');
  const stock = n => +z.sql(`SELECT stock FROM goods WHERE id = '${ids[n]}'`);

  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'goods');
  const ready = page => page.waitForFunction(() => { const b = document.getElementById('invBox'); return b && !b.querySelector('.skel'); },
    null, { timeout: 15000 }).then(() => true).catch(() => false);
  if (!await ready(adm)) { // раздела нет (админка до З23-и4) — FAIL по каждой проверке, а не обрыв
    for (const n of ['шторка пересчёта', 'разница на лету', 'остаток изменился', 'акт проведён', 'акт админу', 'список актов',
      'партия у владельца', 'акт владельцу', 'файл акта', 'Деньги', 'журнал', 'телефон 360', 'палитра'])
      z.check(n + ': раздела «Инвентаризация» нет', false);
    return z.done();
  }
  // шторка пересчёта
  await adm.click('#invNewBtn'); await z.sleep(1200);
  const line = (page, n) => page.evaluate(n => { const r = [...document.querySelectorAll('#invLines .invLine[data-g]')]
    .find(e => e.querySelector('.invName').textContent.startsWith(n));
    return r ? { exp: r.querySelector('.invExp').textContent.trim(), diff: r.querySelector('.invDiff').textContent.trim(),
      moved: r.classList.contains('invMovedRow'), note: (r.querySelector('.invMoved') || {}).textContent || '',
      lot: !!r.querySelector('.invLot select') } : null; }, n);
  const fill = async (page, n, v) => {
    await page.evaluate(([n, v]) => { const r = [...document.querySelectorAll('#invLines .invLine[data-g]')]
      .find(e => e.querySelector('.invName').textContent.startsWith(n));
      const i = r && r.querySelector('input'); if (i){ i.value = v; i.dispatchEvent(new Event('input', { bubbles: true })); } }, [n, v]);
    await z.sleep(200);
  };
  const t0 = await adm.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent,
    n: document.querySelectorAll('#invLines .invLine[data-g]').length, sum: document.getElementById('invSum').textContent }));
  const k0 = await line(adm, 'z23v Кола');
  z.check(`шторка «${t0.title}»: позиции с «в системе» — Кола ${k0 && k0.exp}; «${t0.sum.trim()}»`,
    t0.title === 'Инвентаризация' && t0.n >= 3 && k0 && k0.exp === '12' && k0.diff === '—' && /^Посчитано 0 из \d+/.test(t0.sum.trim()),
    JSON.stringify({ t0, k0 }));
  // категория z23v — только позиции проверки
  await adm.evaluate(() => { const b = [...document.querySelectorAll('#invCats button')].find(x => x.textContent === 'z23v'); if (b) b.click(); });
  await z.sleep(400);
  await fill(adm, 'z23v Кола', '9'); await fill(adm, 'z23v Чипсы', '6'); await fill(adm, 'z23v Вода', '5');
  const [kd, cd, wd] = [await line(adm, 'z23v Кола'), await line(adm, 'z23v Чипсы'), await line(adm, 'z23v Вода')];
  const s1 = await z.text(adm, '#invSum');
  z.check(`разница на лету: Кола «${kd && kd.diff}», Чипсы «${cd && cd.diff}», Вода «${wd && wd.diff}»; админу партии не выбрать; итог «${s1}»`,
    kd && kd.diff === '−3' && !kd.lot && cd && cd.diff === '+2' && wd && wd.diff === 'сошлось'
    && /^Посчитано 3 из \d+ · сошлось 1 · недостача — 1 позиция, 3 шт \(−15,00 zł\) · излишек — 1 позиция, 2 шт \(\+16,00 zł\)$/.test(s1),
    JSON.stringify({ kd, cd, wd, s1 }));
  await adm.fill('#invNote', 'z23v: пересчёт бара');
  await z.shot(adm, 'z23_inv_form');
  // пока считали — продали воду
  await z.api('POST', `/admin/goods/${ids['z23v Вода']}/stock`, { delta: -1, reason: 'adjust', note: 'z23v: продали, пока считали' }, 'boss');
  await adm.click('#invSave'); await z.sleep(300); await adm.click('#invSave'); await z.idle(adm); // З19-и14: после записи — ждём страницу, а не паузу
  const wm = await line(adm, 'z23v Вода'), km = await line(adm, 'z23v Кола');
  const mv = await adm.evaluate(() => ({ open: document.getElementById('drawer').classList.contains('open'),
    title: document.getElementById('drawerTitle').textContent, note: (document.getElementById('invMovedNote') || {}).textContent || '',
    acts: (invActs || []).length }));
  z.check(`остаток изменился: шторка осталась, Вода подсвечена «${wm && wm.note}», «в системе» ${wm && wm.exp}, разница ${wm && wm.diff}; Кола — как насчитали`,
    mv.open && mv.title === 'Инвентаризация' && wm && wm.moved && wm.note === 'пока считали: было 5, стало 4 — пересчитай' && wm.exp === '4'
    && wm.diff === '+1' && km && km.diff === '−3' && /изменился у 1 позиции/.test(mv.note) && stock('z23v Кола') === 12,
    JSON.stringify({ wm, km, mv, kola: stock('z23v Кола') }));
  await z.shot(adm, 'z23_inv_moved');
  await fill(adm, 'z23v Вода', '4');
  await adm.click('#invSave'); await z.sleep(300); await adm.click('#invSave'); await z.idle(adm);
  // строки акта: [имя, в системе, насчитали, разница, себестоимость, партии]
  const actRows = page => page.evaluate(() => [...document.querySelectorAll('#invActLines .invActLine')].map(r => {
    const q = c => { const e = r.querySelector(c); return e ? e.textContent.replace(/\s+/g, ' ').trim() : null; };
    return [q('.iaName b'), q('.iaExp'), q('.iaCnt'), q('.iaDiff'), q('.iaCost'), q('.iaLots')]; }));
  const act = await adm.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent,
    who: (document.getElementById('invActWho') || {}).textContent || '', file: !!document.getElementById('invXlsx') }));
  act.rows = await actRows(adm);
  const no = +((act.title.match(/№(\d+)/) || [])[1] || 0);
  const ar = n => (act.rows.find(r => (r[0] || '') === n) || []);
  z.check(`акт проведён: «${act.title}» — ${act.who}; Кола ${ar('z23v Кола').slice(1, 4).join(' | ')}; склад: Кола 9, Чипсы 6, Вода 4`,
    no > 0 && /провёл admin1 · z23v: пересчёт бара$/.test(act.who) && JSON.stringify(ar('z23v Кола').slice(1, 4)) === JSON.stringify(['12', '9', '−3 шт · −15,00 zł'])
    && JSON.stringify(ar('z23v Чипсы').slice(1, 4)) === JSON.stringify(['4', '6', '+2 шт · +16,00 zł'])
    && JSON.stringify(ar('z23v Вода').slice(1, 4)) === JSON.stringify(['4', '4', 'сошлось'])
    && stock('z23v Кола') === 9 && stock('z23v Чипсы') === 6 && stock('z23v Вода') === 4, JSON.stringify(act));
  z.check('акт админу — без себестоимости, партий и «Скачать»', act.rows.length === 3 && act.rows.every(r => r[4] === null && r[5] === null) && !act.file,
    JSON.stringify(act.rows));
  await z.shot(adm, 'z23_inv_act');
  await adm.evaluate(() => closeDrawer()); await z.sleep(1500);
  const list = await adm.evaluate(() => [...document.querySelectorAll('#invActs tr[data-inv]')].map(r => [...r.querySelectorAll('td')]
    .map(t => { const c = t.cloneNode(true); c.querySelectorAll('.repL').forEach(x => x.remove());
      const n = c.querySelector('.invNote'); if (n) n.textContent = ' · ' + n.textContent; return c.textContent.replace(/\s+/g, ' ').trim(); })));
  const top = list[0] || [];
  z.check(`список актов: сверху «${top.slice(0, 2).join(' · ')}», недостача «${top[4]}», излишек «${top[5]}»`,
    top[0] === `№${no} · z23v: пересчёт бара` && top[2] === 'admin1' && top[3] === '3 · сошлось 1' && top[4] === '3 шт · −15,00 zł'
    && top[5] === '2 шт · +16,00 zł', JSON.stringify(list.slice(0, 2)));

  // владелец: недостача из выбранной партии, акт с себестоимостью, файл
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'goods'); await ready(boss);
  await boss.click('#invNewBtn'); await z.sleep(1200);
  await boss.evaluate(() => { const b = [...document.querySelectorAll('#invCats button')].find(x => x.textContent === 'z23v'); if (b) b.click(); });
  await z.sleep(400);
  await fill(boss, 'z23v Кола', '7'); await z.sleep(1500);
  const opts = await boss.evaluate(() => { const r = [...document.querySelectorAll('#invLines .invLine[data-g]')]
    .find(e => e.querySelector('.invName').textContent.startsWith('z23v Кола')); const s = r && r.querySelector('.invLot select');
    return s ? [...s.options].map(o => o.textContent) : []; });
  z.check(`владелец: при недостаче — партия: ${JSON.stringify(opts)}`, opts[0] === 'со старых партий первыми' && opts.length === 3
    && /· осталось 3 · 2,00 zł$/.test(opts[1]) && /· осталось 6 · 2,50 zł$/.test(opts[2]), JSON.stringify(opts));
  await boss.evaluate(() => { const r = [...document.querySelectorAll('#invLines .invLine[data-g]')]
    .find(e => e.querySelector('.invName').textContent.startsWith('z23v Кола')); const s = r.querySelector('.invLot select');
    s.selectedIndex = 2; s.dispatchEvent(new Event('change', { bubbles: true })); });
  await boss.fill('#invNote', 'z23v: из второй партии');
  await boss.click('#invSave'); await z.sleep(300); await boss.click('#invSave'); await z.idle(boss);
  const oa = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent,
    kpi: [...document.querySelectorAll('#invAct .kpi')].map(k => k.textContent.replace(/\s+/g, ' ').trim()),
    file: !!document.getElementById('invXlsx') }));
  oa.row = (await actRows(boss)).find(r => r[0] === 'z23v Кола') || [];
  const left2 = +z.sql(`SELECT qty_left FROM stock_lots WHERE id = '${lots[1]}'`), left1 = +z.sql(`SELECT qty_left FROM stock_lots WHERE id = '${lots[0]}'`);
  z.check(`акт владельцу «${oa.title}»: Кола ${oa.row.slice(1, 6).join(' | ')}; списано из второй партии (осталось ${left1} и ${left2})`,
    oa.title === `Акт №${no + 1}` && JSON.stringify(oa.row.slice(1, 5)) === JSON.stringify(['9', '7', '−2 шт · −10,00 zł', '5,00 zł'])
    && /^нашлось \d\d\.\d\d \d\d:\d\d: 2 × 2,50$/.test(oa.row[5] || '')
    && oa.kpi.some(k => k.startsWith('По себестоимости') && k.includes('5,00 zł')) && left1 === 3 && left2 === 4, JSON.stringify({ oa, left1, left2 }));
  await z.shot(boss, 'z23_inv_owner_act');
  const [dl] = await Promise.all([boss.waitForEvent('download', { timeout: 15000 }).catch(() => null), boss.click('#invXlsx')]);
  const fname = dl ? dl.suggestedFilename() : '';
  z.check(`файл акта: «Скачать XLSX» — «${fname}»`, oa.file && new RegExp(`^1448-arkusz-spisu-${no + 1}-\\d{4}-\\d{2}-\\d{2}\\.xlsx$`).test(fname), fname);
  await boss.evaluate(() => closeDrawer());

  // «Деньги» — плитка «— по инвентаризациям»
  await boss.evaluate(() => palGoReport('money'));
  await boss.waitForFunction(() => [...document.querySelectorAll('#repBody .kpi')].some(k => k.textContent.includes('по инвентаризациям')),
    null, { timeout: 15000 }).catch(() => null);
  const tile = await boss.evaluate(() => { const k = [...document.querySelectorAll('#repBody .kpi')].find(x => x.textContent.includes('по инвентаризациям'));
    return k ? { text: k.textContent.replace(/\s+/g, ' ').trim(), title: k.getAttribute('title') || (k.querySelector('[title]') || {}).title || '' } : null; });
  z.check(`«Деньги»: плитка «${tile && tile.text}» — акты, недостача по цене продажи и по себестоимости`,
    tile && /по инвентаризациям/.test(tile.text) && /акты пересчёта за период: \d+ акт/.test(tile.title) && /по себестоимости/.test(tile.title),
    JSON.stringify(tile));

  // журнал — подпись вида
  const lbl = await boss.evaluate(() => [AUD_LABEL.inventory, AUD_LABEL.inventory_export, AUD_GROUPS.goods.includes('inventory'),
    AUD_GROUPS.money.includes('inventory_export')]);
  z.check('журнал: «склад: инвентаризация» — в «Товарах», «акт инвентаризации: файл» — в «Деньгах»',
    JSON.stringify(lbl) === JSON.stringify(['склад: инвентаризация', 'акт инвентаризации: файл', true, true]), JSON.stringify(lbl));

  // телефон 360: шторка пересчёта в экране
  const ph = await z.openAdmin('admin1', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'goods'); await ready(ph);
  await ph.click('#invNewBtn'); await z.sleep(1200);
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
    r: [...document.querySelectorAll('#invLines .invLine[data-g] input')].slice(0, 8).map(e => Math.round(e.getBoundingClientRect().right)),
    dw: Math.round(document.getElementById('drawer').getBoundingClientRect().right) }));
  await z.shot(ph, 'z23_inv_phone');
  z.check('телефон 360: шторка пересчёта в экране, поля «насчитали» не вылезают, вбок не прокручивается',
    fit.r.length > 0 && fit.r.every(x => x <= fit.w) && fit.dw <= fit.w + 1 && fit.sw <= fit.w, JSON.stringify(fit));
  await ph.evaluate(() => closeDrawer());

  // палитра
  await adm.evaluate(() => nav('hall'));
  await adm.evaluate(() => palOpen()); await adm.fill('#palInp', 'инвентаризация'); await z.sleep(500);
  const items = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent.trim()).slice(0, 3));
  await adm.keyboard.press('Enter'); await z.sleep(1500);
  const w = await adm.evaluate(() => ({ tab: curTab, top: Math.round(document.getElementById('invHead').getBoundingClientRect().top), h: innerHeight }));
  z.check(`палитра: «инвентаризация» → «${items[0] || '—'}», Enter — «Товары», раздел на экране`,
    items[0] === 'Инвентаризация' && w.tab === 'goods' && w.top >= 0 && w.top < w.h, JSON.stringify({ items, w }));
  await z.done();
})().catch(z.crash);
