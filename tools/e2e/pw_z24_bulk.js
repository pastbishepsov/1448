// З24-и4 «Массовые правки цен» (№53 «в»), браузер. Позиции «z24v Cola» 5, «z24v Woda» 3, «z24v Lays» 6,20 (категория
// z24v) и зона «z24v Зал» 20 zł/ч (API). Владелец, «Товары» → «Цены разом»: категория z24v, +10 %, до 0,50 —
// «Посмотреть»: три строки «было → станет», снял Lays — «Применить к 2» двумя нажатиями: Cola 5,50, Woda 3,50, Lays —
// 6,20. Ещё раз +1 zł, а Cola между просмотром и записью стала 9 (API) — «Цены изменились …: z24v Cola — было 5,50,
// сейчас 9,00», ничего не записано, «Посмотреть ещё раз» — Cola 9 → 10. Пусто в «на сколько» — ошибка в шторке. Зоны —
// просмотр без записи (своя зона 20 → 21); пакеты — фильтр по зоне. У админа кнопки нет. Журнал — «цены разом»,
// палитра «поднять цены», телефон 360 — список без прокрутки вбок. В конце позиции — dropGood, зона — SQL-ом.
const z = require('./_z');
(async () => {
  const PRE = 'z24v ', CAT = 'z24v';
  const T0 = z.sql('SELECT now()::text'); // журнал — только строки этого запуска (стенд хранит прошлые прогоны)
  const wipe = () => { for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE '${PRE}%'`)) z.dropGood(id);
    z.sql(`DELETE FROM zones WHERE name LIKE '${PRE}%'`); };
  wipe();
  z.atEnd(wipe);
  const G = {};
  for (const [n, p] of [['Cola', 5], ['Woda', 3], ['Lays', 6.2]])
    G[n] = (((await z.api('POST', '/admin/goods', { name: PRE + n, category: CAT, price_pln: p }, 'boss')).data || {}).good || {}).id;
  const zid = (((await z.api('POST', '/admin/zones', { name: PRE + 'Зал', rate_pln: 20 }, 'boss')).data || {}).zone || {}).id;
  const price = n => +z.sql(`SELECT price_pln FROM goods WHERE id = '${G[n]}'`);
  z.check('позиции z24v Cola 5, Woda 3, Lays 6,20 и зона «z24v Зал» 20 zł/ч (API)', Object.values(G).every(Boolean) && !!zid, JSON.stringify([G, zid]));

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'goods'); await z.sleep(800);
  const has = await boss.evaluate(() => typeof bulkOpen === 'function' && !!document.getElementById('bulkGoodsBtn'));
  if (!has) { // «Цен разом» нет (до З24-и4) — FAIL по каждой проверке, а не обрыв
    for (const n of ['просмотр', 'снял строку', 'применить', 'цены изменились', 'посмотреть ещё раз', 'пусто', 'зоны', 'пакеты', 'админ', 'журнал',
      'палитра', 'телефон 360']) z.check(n + ': «Цен разом» нет', false);
    return z.done();
  }
  const prev = () => boss.evaluate(() => { const l = document.getElementById('bk_list'), e = document.getElementById('bk_err'), m = document.getElementById('bk_moved');
    return { title: document.getElementById('drawerTitle').textContent, sum: (document.getElementById('bk_sum') || {}).textContent || '',
      rows: l ? [...l.querySelectorAll('.bkRow')].map(r => ({ t: r.innerText.replace(/\s+/g, ' ').trim(), on: r.querySelector('input').checked,
        dis: r.querySelector('input').disabled })) : [],
      apply: (document.getElementById('bk_apply') || {}).textContent || '', err: e ? e.textContent : '', moved: m ? m.textContent.replace(/\s+/g, ' ') : '',
      kind: (document.querySelector('#bulkBox .segs button.on') || {}).textContent || '' }; });
  const look = async () => { await boss.click('#bk_look'); await z.idle(boss); }; // З19-и14: после записи — ждём страницу, а не паузу
  const setUp = async (val, round) => {
    await boss.selectOption('#bk_cat', CAT);
    await boss.fill('#bk_val', String(val));
    if (round) await boss.click(`#bulkBox .segs button[data-v="${round}"]`);
  };

  await boss.click('#bulkGoodsBtn'); await z.sleep(1200);
  await setUp(10, '0.5'); await look();
  const p1 = await prev();
  await z.shot(boss, 'z24_bulk_drawer');
  z.check('«Товары» → «Цены разом»: «Товары», категория z24v, +10 %, до 0,50 — «Посмотреть»: Cola 5,00 → 5,50, Lays 6,20 → 7,00, Woda 3,00 → 3,50, «Применить к 3»',
    p1.title === 'Цены разом' && p1.kind === 'Товары' && p1.sum === `товары «${CAT}»: +10 %, до 0,50 — меняется 3 из 3`
    && JSON.stringify(p1.rows.map(r => r.t)) === JSON.stringify([`${PRE}Cola · ${CAT} 5,00 → 5,50`, `${PRE}Lays · ${CAT} 6,20 → 7,00`, `${PRE}Woda · ${CAT} 3,00 → 3,50`])
    && p1.rows.every(r => r.on) && p1.apply.trim() === 'Применить к 3' && price('Cola') === 5, JSON.stringify(p1));
  await boss.evaluate(() => { const r = [...document.querySelectorAll('#bk_list .bkRow')].find(x => /Lays/.test(x.textContent)); r.querySelector('input').click(); });
  await z.sleep(300);
  const p2 = await prev();
  z.check('снял Lays — «меняется 2 из 3», «Применить к 2»', /меняется 2 из 3$/.test(p2.sum) && p2.apply.trim() === 'Применить к 2', JSON.stringify(p2));
  await boss.click('#bk_apply'); await z.sleep(200);
  const armed = await boss.evaluate(() => (document.getElementById('bk_apply') || {}).textContent || '');
  const before = price('Cola');
  await z.toastAfter(boss, () => boss.click('#bk_apply'), 'Цены обновлены'); // З19-и13: тост этого действия, а не #toast через паузу
  const t1 = await boss.evaluate(() => ({ toast: document.getElementById('toast').textContent, open: document.getElementById('drawer').classList.contains('open') }));
  z.check('«Применить к 2» — первое нажатие «Применить к 2? Ещё раз» (ничего не записано), второе — тост, шторка закрыта: Cola 5,50, Woda 3,50, Lays — 6,20',
    /Применить к 2\? Ещё раз/.test(armed) && before === 5 && /Цены обновлены: 2 — \+10 %, до 0,50/.test(t1.toast) && !t1.open
    && price('Cola') === 5.5 && price('Woda') === 3.5 && price('Lays') === 6.2, JSON.stringify([armed, before, t1, price('Cola'), price('Woda'), price('Lays')]));

  // цены изменились между просмотром и записью
  await boss.evaluate(() => bulkOpen('goods')); await z.sleep(1200);
  await boss.click('#bulkBox .segs button[data-v="add"]'); await z.sleep(200);
  await setUp(1); await look();
  await z.api('PATCH', `/admin/goods/${G.Cola}`, { price_pln: 9 }, 'boss');
  await boss.click('#bk_apply'); await z.sleep(200); await boss.click('#bk_apply'); await z.idle(boss);
  const p3 = await prev();
  z.check(`Cola стала 9 после просмотра — «Цены изменились …: ${PRE}Cola — было 5,50, сейчас 9,00», Woda — 3,50 (ничего не записано)`,
    p3.moved.includes(`${PRE}Cola — было 5,50, сейчас 9,00`) && price('Woda') === 3.5 && price('Cola') === 9, JSON.stringify(p3));
  await boss.evaluate(() => [...document.querySelectorAll('#bulkPrev button')].find(b => /Посмотреть ещё раз/.test(b.textContent)).click());
  await z.idle(boss); // просмотр — тоже запрос
  const p4 = await prev();
  z.check('«Посмотреть ещё раз» — свежий просмотр: Cola 9,00 → 10,00', p4.rows.some(r => r.t === `${PRE}Cola · ${CAT} 9,00 → 10,00`) && !p4.moved, JSON.stringify(p4));
  await boss.fill('#bk_val', ''); await look();
  const p5 = await prev();
  z.check('пусто в «на сколько» — «На сколько: злотые, не ноль», просмотра нет', /На сколько: злотые, не ноль/.test(p5.err) && !p5.rows.length, JSON.stringify(p5));

  // зоны — просмотр без записи; пакеты — фильтр по зоне
  await boss.click('#bulkBox .segs button[data-v="zones"]'); await z.sleep(300);
  await boss.click('#bulkBox .segs button[data-v="pct"]'); await z.sleep(200);
  await boss.fill('#bk_val', '5'); await look();
  const p6 = await prev();
  z.check(`«Зоны»: подсказка про расписание, +5 % — «${PRE}Зал 20,00 → 21,00» в списке; без «Применить» цена — 20`,
    p6.kind === 'Зоны' && p6.rows.some(r => r.t === `${PRE}Зал 20,00 → 21,00`) && +z.sql(`SELECT rate_pln FROM zones WHERE id = '${zid}'`) === 20
    && await boss.evaluate(() => /расписание цен/.test(document.getElementById('bulkBox').textContent)), JSON.stringify(p6));
  await boss.click('#bulkBox .segs button[data-v="packages"]'); await z.sleep(300);
  const pk = await boss.evaluate(n => ({ zone: !!document.getElementById('bk_zone') && [...document.getElementById('bk_zone').options].some(o => o.textContent === n),
    all: !!document.getElementById('bk_all') }), PRE + 'Зал');
  z.check('«Пакеты»: фильтр по зоне (есть «z24v Зал») и «и выключенные»', pk.zone && pk.all, JSON.stringify(pk));
  await boss.evaluate(() => closeDrawer());

  // журнал, палитра, админ
  const lbl = await boss.evaluate(() => [AUD_LABEL.prices_bulk, AUD_ICON.prices_bulk, ['money', 'goods', 'pc'].every(g => AUD_GROUPS[g].includes('prices_bulk'))]);
  const aud = z.rows(`SELECT details FROM admin_actions WHERE action = 'prices_bulk' AND details LIKE '%${PRE}%' AND created_at > '${T0}' ORDER BY created_at DESC LIMIT 3`);
  z.check('журнал: «цены разом», значок coins, в «Деньгах», «Товарах» и «ПК и зал»; строка записи — одна',
    JSON.stringify(lbl) === JSON.stringify(['цены разом', 'coins', true]) && aud.length === 1
    && aud[0] === `товары «${CAT}»: +10 %, до 0,50 — 2 из 2: ${PRE}Cola 5,00 → 5,50 zł; ${PRE}Woda 3,00 → 3,50 zł`, JSON.stringify([lbl, aud]));
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'поднять цены'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent.trim()).slice(0, 3));
  await boss.keyboard.press('Enter'); await z.sleep(1500);
  const pal = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, open: document.getElementById('drawer').classList.contains('open') }));
  z.check(`палитра: «поднять цены» → «${items[0] || '—'}», Enter — шторка «Цены разом»`, items[0] === 'Цены разом' && pal.open && pal.title === 'Цены разом',
    JSON.stringify({ items, pal }));
  await boss.evaluate(() => closeDrawer());
  await z.sleep(2500);
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'goods');
  const ab = await adm.evaluate(() => { const b = document.getElementById('bulkGoodsBtn'); return b ? getComputedStyle(b).display : 'нет'; });
  z.check('у админа «Цены разом» в «Товарах» нет', ab === 'none' || ab === 'нет', ab);

  // телефон 360
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await ph.evaluate(() => bulkOpen('goods')); await z.sleep(1500);
  await ph.selectOption('#bk_cat', CAT); await ph.fill('#bk_val', '10'); await ph.click('#bk_look'); await z.idle(ph);
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
    r: [...document.querySelectorAll('#bulkBox .input, #bulkBox button, #bk_list .bkRow, #bk_apply')].map(e => Math.round(e.getBoundingClientRect().right)),
    rows: document.querySelectorAll('#bk_list .bkRow').length }));
  await ph.evaluate(() => { const l = document.getElementById('bk_list'); if (l) l.scrollIntoView({ block: 'center' }); }); await z.sleep(300);
  await z.shot(ph, 'z24_bulk_phone');
  z.check('телефон 360: шторка с просмотром — поля, кнопки и строки в экране, вбок не прокручивается',
    fit.rows === 3 && fit.r.every(x => x <= fit.w) && fit.sw <= fit.w, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
