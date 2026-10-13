// З9-и4 «Справочники и способы оплаты», браузер. «Бухгалтерия» → третья вкладка «Справочники»: категории (стандартные
// и свои, закупка), контрагенты, способы оплаты. Своя категория добавляется строкой внизу (закупка — галочкой),
// переименовывается в шторке (у стандартной «закупка» заблокирована), скрытая — бледная и в форме расхода её нет,
// «Вернуть» — снова в списке. Новый контрагент: NIP с несходящейся суммой — тост с ошибкой, верный — записан. Способ
// «Карта» выключили — в форме расхода его нет, включили — снова есть. Палитра «справочник» ведёт сюда, в «Журнале» —
// «категория: …», «контрагент: …», «способы оплаты». На телефоне вкладка без горизонтальной прокрутки.
// Категория «z9rp Упаковка» и контрагент «z9rp Типография» живут между прогонами — в конце их имена и видимость как
// были; способы оплаты — все включены.
const z = require('./_z');
(async () => {
  const CAT = 'z9rp Упаковка', CAT2 = 'z9rp Упаковка и плёнка', CP = 'z9rp Типография', ALL = ['till', 'bank', 'card', 'owner'];
  const apiCats = async () => (await z.api('GET', '/admin/expense-categories', undefined, 'boss')).data.categories || [];
  const apiCps = async () => (await z.api('GET', '/admin/counterparties', undefined, 'boss')).data.counterparties || [];
  const restore = async () => {
    await z.api('PUT', '/admin/expense-methods', { on: ALL }, 'boss');
    for (const c of await apiCats()) if ([CAT, CAT2].includes(c.name)) await z.api('PATCH', `/admin/expense-categories/${c.id}`, { name: CAT, hidden: false }, 'boss');
    for (const p of await apiCps()) if (p.name === CP) await z.api('PATCH', `/admin/counterparties/${p.id}`, { hidden: false }, 'boss');
  };
  await restore(); // хвост упавшего прогона
  z.atEnd(restore);
  const catExisted = (await apiCats()).some(c => c.name === CAT), cpExisted = (await apiCps()).some(p => p.name === CP);

  const boss = await z.openAdmin('boss');
  const toast = async () => { await z.sleep(1500); return (await z.text(boss, '#toast')) || ''; };
  const catRow = name => boss.evaluate(n => { const r = [...document.querySelectorAll('#refCats tr.refRow')].find(x => x.cells[0].textContent.startsWith(n));
    return r ? { id: r.dataset.id, off: r.classList.contains('refOff'), cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) } : null; }, name);
  const cpRow = name => boss.evaluate(n => { const r = [...document.querySelectorAll('#refCps tr.refRow')].find(x => x.cells[0].textContent.startsWith(n));
    return r ? { id: r.dataset.id, off: r.classList.contains('refOff'), cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) } : null; }, name);
  const tabRefs = async () => { await boss.click('#accTabs button[data-t="refs"]'); await z.sleep(1500); };
  const expForm = async () => { await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500); await boss.click('#expNewBtn'); await z.sleep(1200);
    return boss.evaluate(() => ({ cats: [...document.querySelectorAll('#efCat option')].map(o => o.textContent), methods: [...document.querySelectorAll('#efMethod button')].map(b => b.textContent) })); };

  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  const tabs = await boss.evaluate(() => [...document.querySelectorAll('#accTabs button')].map(b => b.textContent));
  await tabRefs();
  const std = await catRow('Аренда'), ms = await boss.evaluate(() => [...document.querySelectorAll('#rmMethods button')].map(b => ({ t: b.textContent, on: b.classList.contains('on') })));
  z.check('«Бухгалтерия»: вкладки «Обзор» (З12-и4), «Касса», «Расходы», «Закупки» (З10-и6), «Справочники», «Выгрузка» (З13-и7); в справочниках — «Аренда» стандартная, способы — четыре, все включены',
    tabs.join(',') === 'Обзор,Касса,Расходы,Закупки,Справочники,Выгрузка' && std && std.cells[1] === 'стандартная' && ms.map(m => m.t).join(',') === 'Из кассы,Банк,Карта,Личные деньги владельца'
    && ms.every(m => m.on), JSON.stringify({ tabs, std, ms }));
  await z.shot(boss, 'z9_refs');

  // своя категория
  await boss.fill('#rcName', CAT); await boss.check('#rcPurchase'); await boss.click('#rcAdd');
  const t1 = await toast(), r1 = await catRow(CAT);
  z.check(`своя категория «${CAT}» с закупкой → тост «добавлена — закупка», строка «своя · закупка»`,
    (catExisted ? /уже есть/.test(t1) : t1.includes(`Категория «${CAT}» добавлена — закупка · уже в форме расхода`)) && r1 && r1.cells[1] === 'своя' && r1.cells[2] === 'закупка',
    JSON.stringify({ t1, r1 }));
  await boss.click(`#refCats tr.refRow[data-id="${std.id}"] button:has-text("Изменить")`); await z.sleep(600);
  const lock = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, dis: document.getElementById('rcEditPurchase').disabled }));
  await boss.evaluate(() => closeDrawer());
  await boss.click(`#refCats tr.refRow[data-id="${r1.id}"] button:has-text("Изменить")`); await z.sleep(600);
  await boss.fill('#rcEditName', CAT2); await boss.click('#rcSave');
  const t2 = await toast(), r2 = await catRow(CAT2);
  z.check('«Изменить»: у «Аренды» «закупка» заблокирована; своя переименована → «✓ Категория поправлена», строка с новым именем',
    lock.title === 'Категория' && lock.dis && /Категория поправлена/.test(t2) && r2 && r2.id === r1.id, JSON.stringify({ lock, t2, r2 }));
  await boss.click(`#refCats tr.refRow[data-id="${r1.id}"] button:has-text("Скрыть")`);
  const t3 = await toast(), r3 = await catRow(CAT2);
  const f1 = await expForm();
  await boss.evaluate(() => closeDrawer());
  await tabRefs();
  await boss.click(`#refCats tr.refRow[data-id="${r1.id}"] button:has-text("Вернуть")`);
  const t4 = await toast(), r4 = await catRow(CAT2);
  z.check('«Скрыть» → тост, строка бледная «· скрыта», в форме расхода её нет; «Вернуть» → «снова в списке»',
    t3.includes(`«${CAT2}» скрыта — в новом расходе её нет, старые расходы остались`) && r3 && r3.off && /· скрыта/.test(r3.cells[0])
    && !f1.cats.includes(CAT2) && f1.cats.includes('Аренда') && t4.includes(`«${CAT2}» снова в списке`) && r4 && !r4.off, JSON.stringify({ t3, r3, cats: f1.cats.length, t4 }));

  // контрагент
  if (!cpExisted) {
    await boss.click('#rpNew'); await z.sleep(600);
    await boss.fill('#rpName', CP); await boss.fill('#rpNip', '123-456-78-90'); await boss.click('#rpSave');
    const t5 = await toast();
    await boss.fill('#rpNip', '774-000-14-54'); await boss.fill('#rpContact', 'druk@example.pl'); await boss.click('#rpSave');
    const t6 = await toast(), p1 = await cpRow(CP);
    z.check('новый контрагент: NIP 123-456-78-90 → «NIP не сходится по контрольной сумме»; 774-000-14-54 → «добавлен», в списке NIP через дефисы',
      /NIP не сходится по контрольной сумме/.test(t5) && t6.includes(`Контрагент «${CP}» добавлен`) && p1 && p1.cells[1] === '774-000-14-54' && p1.cells[2] === 'druk@example.pl',
      JSON.stringify({ t5, t6, p1 }));
  } else {
    const p0 = await cpRow(CP);
    await boss.click(`#refCps tr.refRow[data-id="${p0.id}"] button:has-text("Изменить")`); await z.sleep(600);
    await boss.fill('#rpNip', '123-456-78-90'); await boss.click('#rpSave');
    const t5 = await toast();
    await boss.fill('#rpNip', '774-000-14-54'); await boss.fill('#rpContact', 'druk' + Date.now() % 1000 + '@example.pl'); await boss.click('#rpSave');
    const t6 = await toast(), p1 = await cpRow(CP);
    z.check('контрагент (с прошлого прогона): NIP 123-456-78-90 → «NIP не сходится по контрольной сумме»; верный — «Контрагент поправлен»',
      /NIP не сходится по контрольной сумме/.test(t5) && /Контрагент поправлен/.test(t6) && p1 && p1.cells[1] === '774-000-14-54', JSON.stringify({ t5, t6, p1 }));
  }
  const p2 = await cpRow(CP);
  await boss.click(`#refCps tr.refRow[data-id="${p2.id}"] button:has-text("Скрыть")`);
  const t7 = await toast(), p3 = await cpRow(CP);
  await boss.click(`#refCps tr.refRow[data-id="${p2.id}"] button:has-text("Вернуть")`);
  const t8 = await toast();
  z.check('контрагент: «Скрыть» → «скрыт — в подсказках его нет», строка бледная; «Вернуть» → «снова в подсказках»',
    t7.includes(`«${CP}» скрыт — в подсказках его нет`) && p3 && p3.off && t8.includes(`«${CP}» снова в подсказках`), JSON.stringify({ t7, p3, t8 }));

  // способы оплаты
  await boss.click('#rmMethods button[data-m="card"]');
  const t9 = await toast(), cardOn = await boss.evaluate(() => document.querySelector('#rmMethods button[data-m="card"]').classList.contains('on'));
  await z.shot(boss, 'z9_refs_methods');
  const f2 = await expForm();
  await boss.evaluate(() => closeDrawer());
  await tabRefs();
  await boss.click('#rmMethods button[data-m="card"]');
  const t10 = await toast();
  const f3 = await expForm();
  await boss.evaluate(() => closeDrawer());
  z.check('«Карта» выключена → тост, кнопка погасла, в форме расхода «Карты» нет; включили — снова есть',
    t9.includes('Способ «Карта» выключен — в новом расходе его нет') && !cardOn && !f2.methods.includes('Карта') && f2.methods.includes('Банк')
    && t10.includes('Способ «Карта» снова включён') && f3.methods.includes('Карта'), JSON.stringify({ t9, cardOn, m2: f2.methods, t10, m3: f3.methods }));

  await boss.evaluate(() => nav('hall')); await z.sleep(600);
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'справочник'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(1800);
  const where = await boss.evaluate(() => ({ tab: curTab, acc: (document.querySelector('#accTabs button.on') || {}).textContent }));
  z.check('палитра: «справочник» → «Справочники», Enter — «Бухгалтерия», вкладка «Справочники»', items[0] === 'Справочники' && where.tab === 'acc' && where.acc === 'Справочники',
    JSON.stringify({ items, where }));
  await z.nav(boss, 'grants');
  const jr = await boss.evaluate(() => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).slice(0, 40));
  const has = w => jr.some(t => t.includes(w));
  z.check('«Журнал»: «категория: правка», «контрагент: правка», «способы оплаты» — по-русски', has('категория: правка') && has('контрагент: правка') && has('способы оплаты'),
    JSON.stringify(jr.filter(t => /категория|контрагент|способы/.test(t)).slice(0, 4)));

  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'acc');
  await ph.click('#accTabs button[data-t="refs"]'); await z.sleep(1500);
  const w = await ph.evaluate(() => ({ page: document.documentElement.scrollWidth, rows: document.querySelectorAll('#refCats tr.refRow').length }));
  await z.shot(ph, 'z9_refs_phone');
  z.check('телефон 360 px: «Справочники» без горизонтальной прокрутки, категории на месте', w.page <= 360 && w.rows >= 11, JSON.stringify(w));
  await z.done();
})().catch(z.crash);
