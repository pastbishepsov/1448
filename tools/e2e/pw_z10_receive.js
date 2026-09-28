// З10-и4 «Приёмка поставки админом», браузер. admin1 в «Товарах» → «Поставки · ждут цен»: «Принять поставку» — шторка со
// строками «позиция × штук» и заметкой, тост «✓ Поставка принята: …», строка в списке без единой цены. «Исправить» — шторка
// с «Принята … · кто», штуки строк; сохранили — у поставки пометка «исправлено ДД.ММ ЧЧ:ММ · admin1», у строки «(было N)».
// Ушёл товар — отменить нельзя, меньше ушедшего не даёт; вернули — «Отменить поставку» (два нажатия), строки нет.
// Владелец в «Бухгалтерии»: бейдж раздела вырос на поставку сразу (канал персонала), тост «📦 Приёмка …»; в «Расходах» —
// «Поставки ждут цен: N» со ссылкой в «Товары» → «Поставки»; в «Журнале» — «приёмка», «приёмка: правка», «приёмка: отмена».
// Палитра «приёмка» → «Поставки». Телефон 360: список и шторка без горизонтальной прокрутки.
// Данные: позиции «z10 Приёмка окно» и «z10 Приёмка ящик» (категория z10) заводятся каждый прогон и в конце удаляются
// вместе с движениями, партиями и поставками; продажа — картой, возвращается.
const z = require('./_z');
(async () => {
  const A = 'z10 Приёмка окно', B = 'z10 Приёмка ящик', MARK = 'z10 окно';
  const gone = () => { for (const n of [A, B]) for (const id of z.rows(`SELECT id FROM goods WHERE name = '${n}'`)) z.dropGood(id); };
  gone(); z.atEnd(gone);
  const mk = async (name, price) => ((await z.api('POST', '/admin/goods', { name, category: 'z10', price_pln: price }, 'boss')).data.good || {}).id;
  const ga = await mk(A, 3), gb = await mk(B, 4);
  const due0 = (await z.api('GET', '/admin/expenses/due', undefined, 'boss')).data.count || 0;
  const sup0 = (await z.api('GET', '/admin/overview', undefined, 'boss')).data.supplies_waiting || 0;
  const stock = id => +z.sql(`SELECT stock FROM goods WHERE id = '${id}'`);

  const boss = await z.openAdmin('boss');
  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  const badge = () => boss.evaluate(() => { const b = document.getElementById('accBadge'), t = document.querySelector('#accTabs button[data-t="exp"]');
    return { rail: b && !b.classList.contains('hidden') ? +b.textContent : 0, tab: t && t.dataset.n ? +t.dataset.n : 0, title: b ? b.title : '' }; });
  const b0 = await badge();

  const adm = await z.openAdmin('admin1');
  const toast = async (p, ms = 2000) => { await z.sleep(ms); return (await z.text(p, '#toast')) || ''; };
  const row = () => adm.evaluate(n => { const r = [...document.querySelectorAll('#suppliesBox tr.supRow')].find(x => x.innerText.includes(n));
    return r ? { id: r.dataset.id, cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()), mark: (r.querySelector('.supEdited') || {}).textContent || null } : null; }, MARK);
  await z.nav(adm, 'goods');
  const s0 = await adm.evaluate(() => ({ head: (document.getElementById('supHead') || {}).textContent, btn: (document.getElementById('supNewBtn') || {}).textContent }));
  if (!z.check(`«Товары» у админа: «Поставки · ждут цен ${sup0}» и «Принять поставку»`, s0.head === `Поставки · ждут цен ${sup0}` && s0.btn === 'Принять поставку',
    JSON.stringify(s0))) return z.done();

  section('приёмка');
  await adm.click('#supNewBtn'); await z.sleep(700);
  await adm.selectOption('#supLines .supLine:nth-child(1) select', ga); await adm.fill('#supLines .supLine:nth-child(1) input', '12');
  await adm.click('#supAddLine'); await z.sleep(300);
  await adm.selectOption('#supLines .supLine:nth-child(2) select', gb); await adm.fill('#supLines .supLine:nth-child(2) input', '6');
  await adm.fill('#supNote', `${MARK}: накладная 12`);
  const d0 = await adm.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, lines: document.querySelectorAll('#supLines .supLine').length,
    save: document.getElementById('supSave').textContent }));
  await z.shot(adm, 'z10_receive_new');
  await adm.click('#supSave');
  const t1 = await toast(adm), r1 = await row(), n1 = await z.text(adm, '#supWait');
  z.check('шторка «Принять поставку»: две строки и заметка → тост «✓ Поставка принята: окно ×12, ящик ×6»; в списке — кто и что, «ждут цен» +1; остатки выросли',
    d0.title === 'Принять поставку' && d0.lines === 2 && d0.save === 'Принять поставку' && t1 === `Поставка принята: ${A} ×12, ${B} ×6`
    && r1 && r1.cells[1] === 'admin1' && r1.cells[2] === `${A} ×12, ${B} ×6` && r1.cells[3] === `${MARK}: накладная 12` && r1.mark === null
    && +n1 === sup0 + 1 && stock(ga) === 12 && stock(gb) === 6, JSON.stringify({ d0, t1, r1, n1 }));
  const box = await adm.evaluate(() => document.getElementById('suppliesBox').innerText);
  z.check('в списке поставок ни одной цены (zł)', !/zł/.test(box), box.slice(0, 200));
  await adm.evaluate(() => document.getElementById('supHead').scrollIntoView()); await z.sleep(300);
  await z.shot(adm, 'z10_receive_list');

  const b1 = await badge(), bt = await toast(boss, 300);
  z.check('владелец: бейдж «Бухгалтерии» вырос на поставку сразу (вкладка «Расходы» — без неё), тост «📦 Приёмка — строка в канале персонала»',
    b1.rail === due0 + sup0 + 1 && b1.tab === b0.tab && b0.rail === due0 + sup0 && b1.title.includes(`поставки ждут цен: ${sup0 + 1}`)
    && bt === '📦 Приёмка — строка в канале персонала', JSON.stringify({ b0, b1, bt, due0, sup0 }));
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  const hint = await z.text(boss, '#expSupHint');
  await z.shot(boss, 'z10_receive_owner_hint');
  await boss.click('#expSupHint a'); await z.sleep(2000);
  const w = await boss.evaluate(n => { const h = document.getElementById('supHead'), r = h ? h.getBoundingClientRect() : null;
    return { tab: curTab, seen: !!r && r.top >= 0 && r.top < innerHeight, row: [...document.querySelectorAll('#suppliesBox tr.supRow')].some(x => x.innerText.includes(n)) }; }, MARK);
  z.check(`«Расходы»: «Поставки ждут цен: ${sup0 + 1} — «Товары» → «Поставки»», ссылка ведёт к списку поставок`,
    hint === `Поставки ждут цен: ${sup0 + 1} — «Товары» → «Поставки»` && w.tab === 'goods' && w.seen && w.row, JSON.stringify({ hint, w }));

  section('исправление');
  await adm.click(`#suppliesBox tr.supRow[data-id="${r1.id}"] button`); await z.sleep(700);
  const e0 = await adm.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, who: (document.getElementById('supWho') || {}).textContent,
    ins: [...document.querySelectorAll('#supLines input')].map(i => i.value), add: !!document.getElementById('supAddLine'), cancel: !!document.getElementById('supCancel') }));
  await adm.fill('#supLines .supLine:nth-child(1) input', '10');
  await adm.click('#supSave');
  const t2 = await toast(adm), r2 = await row(), now = z.sql("SELECT to_char(now() AT TIME ZONE 'Europe/Warsaw', 'DD.MM')");
  z.check('«Исправить»: «Принята … · admin1», штуки 12 и 6, «Отменить поставку»; окно 12 → 10 → тост «✓ Поставка исправлена — …», '
    + 'пометка «исправлено ДД.ММ ЧЧ:ММ · admin1», у строки «(было 12)», остаток 10',
    e0.title === 'Исправить поставку' && /^Принята \d\d\.\d\d \d\d:\d\d · admin1$/.test(e0.who) && JSON.stringify(e0.ins) === '["12","6"]' && !e0.add && e0.cancel
    && t2 === `Поставка исправлена — ${A} ×10, ${B} ×6` && r2 && new RegExp(`^исправлено ${now.replace('.', '\\.')} \\d\\d:\\d\\d · admin1$`).test(r2.mark)
    && r2.cells[2] === `${A} ×10 (было 12), ${B} ×6` && stock(ga) === 10, JSON.stringify({ e0, t2, r2 }));
  await adm.evaluate(() => document.getElementById('supHead').scrollIntoView()); await z.sleep(300);
  await z.shot(adm, 'z10_receive_edited');

  const sale = (await z.api('POST', '/admin/sales', { good_id: gb, qty: 3, method: 'card' }, 'admin1')).data.sale || {};
  await adm.evaluate(() => renderGoods()); await z.sleep(1500);
  await adm.click(`#suppliesBox tr.supRow[data-id="${r1.id}"] button`); await z.sleep(700);
  const e1 = await adm.evaluate(() => ({ who: (document.getElementById('supWho') || {}).textContent, no: (document.getElementById('supNoCancel') || {}).textContent || null,
    cancel: !!document.getElementById('supCancel'), names: [...document.querySelectorAll('#supLines .supName')].map(x => x.textContent.replace(/\s+/g, ' ').trim()) }));
  await z.shot(adm, 'z10_receive_sold');
  await adm.fill('#supLines .supLine:nth-child(2) input', '2');
  await adm.click('#supSave');
  const t3 = await toast(adm, 800);
  z.check('продали ящик ×3: в шторке «исправлено … · admin1», «ушло 3 — меньше нельзя», отменить нельзя; 2 штуки — тост «ушло 3 — меньше нельзя»',
    e1.who.includes('· исправлено ') && e1.who.endsWith(' · admin1') && e1.no === 'Из поставки уже ушёл товар — отменить целиком нельзя, исправь штуки.' && !e1.cancel
    && e1.names[0] === `${A} · было 12` && e1.names[1] === `${B} · ушло 3 — меньше нельзя` && t3 === `«${B}»: ушло 3 — меньше нельзя` && stock(gb) === 3,
    JSON.stringify({ e1, t3 }));
  await adm.evaluate(() => closeDrawer());

  section('отмена');
  await z.api('POST', `/admin/sales/${sale.id}/void`, {}, 'admin1');
  await adm.evaluate(() => renderGoods()); await z.sleep(1500);
  await adm.click(`#suppliesBox tr.supRow[data-id="${r1.id}"] button`); await z.sleep(700);
  await adm.click('#supCancel'); await z.sleep(400);
  const armed = await z.text(adm, '#supCancel');
  await adm.click('#supCancel');
  const t4 = await toast(adm), r4 = await row(), n4 = await z.text(adm, '#supWait');
  z.check('вернули продажу: «Отменить поставку» — «Ещё раз — отменить поставку» → тост «✓ Поставка отменена …»; строки нет, «ждут цен» — прежнее; остатки 0',
    armed === 'Ещё раз — отменить поставку' && t4 === 'Поставка отменена — остаток уменьшен на всё принятое' && r4 === null && +n4 === sup0
    && stock(ga) === 0 && stock(gb) === 0, JSON.stringify({ armed, t4, r4, n4 }));
  await z.sleep(1500);
  const b4 = await badge();
  z.check('владелец: бейдж «Бухгалтерии» вернулся (отмена — тоже строкой в канал)', b4.rail === due0 + sup0, JSON.stringify({ b4, due0, sup0 }));

  section('журнал, палитра, телефон');
  await z.nav(boss, 'grants');
  const jr = await boss.evaluate(n => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => t.includes(n)).slice(0, 6), A);
  z.check('«Журнал»: «приёмка», «приёмка: правка», «приёмка: отмена» — по-русски, с тем, что было и что стало',
    jr.some(t => t.includes('приёмка ') && t.includes(`${A} ×12, ${B} ×6`)) && jr.some(t => t.includes('приёмка: правка') && t.includes(`${A} 12 → 10`))
    && jr.some(t => t.includes('приёмка: отмена') && t.includes(`${A} ×10, ${B} ×6`)), JSON.stringify(jr));
  await z.nav(adm, 'hall');
  await adm.evaluate(() => palOpen()); await adm.fill('#palInp', 'приёмка'); await z.sleep(500);
  const items = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await adm.keyboard.press('Enter'); await z.sleep(2000);
  const pw = await adm.evaluate(() => { const h = document.getElementById('supHead'), r = h ? h.getBoundingClientRect() : null;
    return { tab: curTab, seen: !!r && r.top >= 0 && r.top < innerHeight }; });
  z.check('палитра админа: «приёмка» → «Поставки», Enter — «Товары», список поставок на экране', items[0] === 'Поставки' && pw.tab === 'goods' && pw.seen,
    JSON.stringify({ items, pw }));

  await z.api('POST', '/admin/supplies', { lines: [{ good_id: ga, qty: 5 }], note: `${MARK}: для телефона` }, 'admin1');
  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // три окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('admin1', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'goods'); await z.sleep(3000); // тосты входа сошли
  await ph.evaluate(() => document.getElementById('supHead').scrollIntoView()); await z.sleep(300);
  const p1 = await ph.evaluate(n => ({ sw: document.documentElement.scrollWidth, row: [...document.querySelectorAll('#suppliesBox tr.supRow')].some(x => x.innerText.includes(n)) }), MARK);
  await z.shot(ph, 'z10_receive_phone');
  await ph.click('#supNewBtn'); await z.sleep(700);
  await ph.click('#supAddLine'); await z.sleep(300);
  const p2 = await ph.evaluate(() => { const d = document.getElementById('drawer').getBoundingClientRect();
    return { sw: document.documentElement.scrollWidth, fit: [...document.querySelectorAll('#supLines .supLine')].every(l => [...l.children].every(c => c.getBoundingClientRect().right <= d.right + 1)) }; });
  await z.shot(ph, 'z10_receive_phone_new');
  z.check('телефон 360: список поставок и шторка «Принять поставку» — без горизонтальной прокрутки, строки в ширину шторки',
    p1.sw <= 360 && p1.row && p2.sw <= 360 && p2.fit, JSON.stringify({ p1, p2 }));
  await z.done();
})().catch(z.crash);

function section(t) { console.log(`\n── ${t}`); }
