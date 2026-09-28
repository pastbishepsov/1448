// З9-и3б «Долг клуба владельцу», браузер. Владелец вносит расход «Личные деньги владельца»: в шторке — подсказка про
// долг (у «Банка» её нет), тост — «клуб должен владельцу N zł», над списком расходов — KPI «Клуб должен владельцу».
// «Вернуть владельцу» открывает шторку долга: сумма — весь долг, из «Расходов» — «С банка», из «Кассы» — «Из кассы»,
// у способа — подсказка. Возврат из кассы — тост с долгом и «должно быть в ящике»; админ на смене видит в «Моей смене»
// «минус возвраты владельцу из кассы»; в «Кассе» — колонка «Владельцу» и возврат в раскрытии дня, отмена там же —
// строка зачёркнута, кто отменил. Больше долга — отказ с остатком, шторка со свежей суммой. Долг погашен — шторка
// «ничего не должен», история с отменённым. Журнал — «долг владельцу: возврат / отмена возврата»; палитра «долг».
// Временный z9dp на смене; расходы и возвраты с заметкой «z9dp проверка …» (у прогона — своя метка) в конце сторнируются и
// отменяются — долг как был.
const z = require('./_z');
(async () => {
  const MARK = 'z9dp проверка', TAG = MARK + ' ' + Date.now().toString(36).slice(-5); // TAG — строки этого прогона
  const tidy = async () => { // сначала возвраты, потом расходы — долг как был
    for (const id of z.sql(`SELECT string_agg(id::text, ' ') FROM owner_repayments WHERE note LIKE '${MARK}%' AND voided_at IS NULL`).split(' ').filter(Boolean))
      await z.api('POST', `/admin/owner-repayments/${id}/void`, undefined, 'boss');
    for (const id of z.sql(`SELECT string_agg(id::text, ' ') FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`).split(' ').filter(Boolean))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
  };
  await tidy(); // хвост упавшего прогона
  z.atEnd(tidy);
  await z.hireTemp('z9dp');
  const pv = async () => (await z.api('GET', '/admin/work/closeout', undefined, 'z9dp')).data;
  const debt = async () => ((await z.api('GET', '/admin/owner-debt', undefined, 'boss')).data.owner_debt || {}).debt_pln;
  const adm = await z.openAdmin('z9dp'); // вход в админку открывает смену
  if ((await pv()).baseline) { // ящика на стенде ещё нет — заводит его z9dp сдачей на 500 zł
    await z.api('POST', '/admin/work/stop', { counted_pln: 500 }, 'z9dp');
    await z.api('POST', '/admin/work/start', {}, 'z9dp');
  }
  const boss = await z.openAdmin('boss');
  const F = v => boss.evaluate(x => fmtPLN(x).replace(/\s+/g, ' '), v); // деньги так, как их пишет страница
  const toast = async () => { await z.sleep(1800); return (await z.text(boss, '#toast')) || ''; };
  const arm = async sel => { await boss.click(sel); await z.sleep(250); await boss.click(sel); };
  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  const d0 = await debt();

  // расход личными: подсказка про долг, тост и KPI
  await boss.click('#expNewBtn'); await z.sleep(500);
  const h0 = await boss.isVisible('#efOwnerHint');
  await boss.click('#efMethod button:has-text("Личные деньги владельца")'); await z.sleep(200);
  const h1 = await boss.isVisible('#efOwnerHint'), ht = await boss.isVisible('#efTillHint');
  await boss.selectOption('#efCat', { label: 'Оборудование и ремонт' });
  await boss.fill('#efAmount', '120');
  await boss.fill('#efNote', TAG + ': мышки');
  await z.shot(boss, 'z9_debt_form');
  await boss.click('#efSave');
  const t1 = await toast(), d1 = await debt();
  await z.sleep(600);
  const kpi1 = await z.text(boss, '#expDebtKpi');
  z.check(`шторка расхода: у «Банка» подсказки про долг нет, у «Личных денег владельца» — есть (про кассу — нет); тост «… · клуб должен владельцу ${await F(d1)} zł»; KPI — та же сумма`,
    !h0 && h1 && !ht && Math.abs(d1 - d0 - 120) < 0.005 && t1.includes(`Расход 120,00 zł внесён — Оборудование и ремонт · клуб должен владельцу ${await F(d1)} zł`)
    && kpi1 === `Клуб должен владельцу ${await F(d1)} zł`, JSON.stringify({ h0, h1, ht, t1, kpi1, d0, d1 }));

  // «Вернуть владельцу» из «Расходов»: весь долг, «С банка»; возврат 45 из кассы
  await boss.click('#expDebtBtn'); await z.sleep(900);
  const open1 = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, now: document.getElementById('odNow').textContent,
    amount: document.getElementById('odAmount').value, on: (document.querySelector('#odMethod button.on') || {}).textContent, hint: document.getElementById('odHint').textContent }));
  await boss.click('#odMethod button:has-text("Из кассы")'); await z.sleep(200);
  const hint2 = await z.text(boss, '#odHint');
  z.check(`шторка долга: «Клуб должен владельцу ${await F(d1)} zł», сумма — весь долг, способ — «С банка» с подсказкой; «Из кассы» — подсказка про ящик`,
    open1.title === 'Долг клуба владельцу' && open1.now === `Клуб должен владельцу ${await F(d1)} zł` && Math.abs(parseFloat(open1.amount) - d1) < 0.005
    && open1.on === 'С банка' && /ящик не трогает/.test(open1.hint) && /«должно быть в ящике» станет меньше/.test(hint2 || ''), JSON.stringify({ open1, hint2 }));
  const x0 = (await pv()).expected_pln;
  await boss.fill('#odAmount', '45'); await boss.fill('#odNote', TAG + ': из ящика');
  await z.shot(boss, 'z9_debt_drawer');
  await arm('#odSubmit');
  const t2 = await toast(), d2 = await debt(), p2 = await pv();
  z.check(`возврат 45 из кассы: тост «✓ Вернули владельцу 45,00 zł из кассы · клуб должен владельцу ${await F(d2)} zł · в ящике должно быть ${await F(p2.expected_pln)} zł»; «должно быть» −45`,
    Math.abs(d2 - (d1 - 45)) < 0.005 && Math.abs(p2.expected_pln - (x0 - 45)) < 0.005
    && t2.includes(`Вернули владельцу 45,00 zł из кассы · клуб должен владельцу ${await F(d2)} zł · в ящике должно быть ${await F(p2.expected_pln)} zł`), JSON.stringify({ t2, d2, x0, p2: p2.expected_pln }));

  await adm.evaluate(() => loadMyShift()); await adm.evaluate(() => openMyShift()); await z.sleep(1500);
  const mine = (await z.text(adm, '#drawerBody')) || '', coExp = await z.text(adm, '#coExpected');
  z.check(`админ, «Моя смена»: «должно быть» ${await F(p2.expected_pln)} zł, в «Из чего» — «минус возвраты владельцу из кассы ${await F(p2.owner_pln)}»`,
    coExp === `${await F(p2.expected_pln)} zł` && mine.includes(`минус возвраты владельцу из кассы ${await F(p2.owner_pln)}`), JSON.stringify({ coExp, mine: mine.slice(0, 480) }));
  await z.shot(adm, 'z9_debt_myshift');

  // «Касса»: колонка «Владельцу», возврат в раскрытии дня, отмена там же
  await boss.click('#accTabs button[data-t="cash"]'); await z.sleep(1800);
  const today = (await z.api('GET', '/admin/cash/book?preset=today', undefined, 'boss')).data.days.pop();
  const tr = await boss.evaluate(() => ({ head: [...document.querySelectorAll('#accBody table.accBook th')].map(t => t.textContent),
    cells: [...(document.querySelector('#accBody tr.accDay') || { cells: [] }).cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) }));
  await boss.evaluate(() => { accDay = null; });
  await boss.click(`#accBody tr.accDay[data-date="${z.clubDay(0)}"]`); await z.sleep(400);
  const det = (await z.text(boss, '#accBody tr.accDet')) || '';
  z.check(`«Касса»: колонка «Владельцу» после «Расходов», у сегодня — −${await F(today.owner_pln)}; в раскрытии — «Возвраты владельцу из кассы: … boss — вернули владельцу 45,00 zł»`,
    tr.head[5] === 'Расходы' && tr.head[6] === 'Владельцу' && tr.head.length === 10 && tr.cells[6] === '−' + await F(today.owner_pln)
    && /Возвраты владельцу из кассы/.test(det) && det.includes(`boss — вернули владельцу 45,00 zł · ${TAG}: из ящика`), JSON.stringify({ tr, det: det.slice(-300) }));
  const own = () => boss.evaluate(m => [...document.querySelectorAll('#accBody tr.accDet div')].filter(e => e.textContent.includes(m + ': из ящика')).map(e => ({ cls: e.className, t: e.textContent.replace(/\s+/g, ' ') })), TAG);
  await boss.evaluate(m => { const d = [...document.querySelectorAll('#accBody tr.accDet div')].find(e => e.textContent.includes(m + ': из ящика')); if (d) d.scrollIntoView({ block: 'center' }); }, MARK);
  await z.shot(boss, 'z9_debt_book');
  const vb = await boss.$$(`#accBody tr.accDet div:has-text("${TAG}: из ящика") button:has-text("Отменить")`);
  if (vb.length) { await vb[0].click(); await z.sleep(250); await vb[0].click(); }
  const t3 = await toast(), d3 = await debt(), p3 = await pv();
  const row3 = await own();
  z.check(`отмена в книге: тост «✓ Возврат отменён · клуб должен владельцу ${await F(d3)} zł · в ящике должно быть …», долг и ящик вернулись; строка зачёркнута — «отменил boss»`,
    vb.length === 1 && Math.abs(d3 - d1) < 0.005 && Math.abs(p3.expected_pln - x0) < 0.005
    && t3.includes(`Возврат отменён · клуб должен владельцу ${await F(d3)} zł · в ящике должно быть ${await F(p3.expected_pln)} zł`)
    && row3.length === 1 && row3[0].cls === 'accVoid' && /отменил boss \d\d:\d\d/.test(row3[0].t), JSON.stringify({ t3, row3 }));

  // «Вернуть владельцу» из «Кассы» — «Из кассы»; больше долга — отказ с остатком
  await boss.click('#accOwnBtn'); await z.sleep(900);
  const on2 = await z.text(boss, '#odMethod button.on');
  await boss.fill('#odAmount', String(Math.round((d3 + 10) * 100) / 100)); await boss.fill('#odNote', TAG + ': лишнее');
  await arm('#odSubmit');
  const t4 = await toast(), a4 = await boss.evaluate(() => (document.getElementById('odAmount') || {}).value);
  z.check(`из «Кассы» способ — «Из кассы»; на 10 больше долга — «⚠ Клуб должен владельцу ${await F(d3)} zł — больше вернуть нельзя», сумма — снова весь долг`,
    on2 === 'Из кассы' && t4.includes(`Клуб должен владельцу ${await F(d3)} zł — больше вернуть нельзя`) && Math.abs(parseFloat(a4) - d3) < 0.005 && Math.abs(await debt() - d3) < 0.005,
    JSON.stringify({ on2, t4, a4 }));

  // весь долг — с банка: «ничего не должен», шторка без формы, история с отменённым
  await boss.click('#odMethod button:has-text("С банка")'); await z.sleep(200);
  await boss.fill('#odNote', TAG + ': остаток');
  await arm('#odSubmit');
  const t5 = await toast(), d5 = await debt();
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  const kpi5 = await z.text(boss, '#expDebtKpi');
  await boss.click('#expDebtKpi'); await z.sleep(900);
  const box = await boss.evaluate(m => ({ now: document.getElementById('odNow').textContent, form: !!document.getElementById('odAmount'),
    rows: [...document.querySelectorAll('#odList .odRow')].filter(r => r.textContent.includes(m)).map(r => ({ off: r.classList.contains('off'), t: r.textContent.replace(/\s+/g, ' ') })) }), TAG);
  z.check(`весь долг с банка: тост «… с банка · клуб ничего не должен владельцу», KPI — 0,00 zł; шторка — «ничего не должен», без формы, в истории — возврат с банка и отменённый из кассы`,
    Math.abs(d5) < 0.005 && t5.includes(`Вернули владельцу ${await F(d3)} zł с банка · клуб ничего не должен владельцу`) && kpi5 === 'Клуб должен владельцу 0,00 zł'
    && box.now === 'Клуб ничего не должен владельцу' && !box.form && box.rows.length === 2 && box.rows[0].t.includes(`${await F(d3)} zł с банка · ${TAG}: остаток`)
    && box.rows[1].off && /45,00 zł из кассы · .* · отменил boss \d\d\.\d\d\.\d{4} \d\d:\d\d/.test(box.rows[1].t), JSON.stringify({ t5, d5, kpi5, box }));
  await z.shot(boss, 'z9_debt_history');
  await boss.evaluate(() => closeDrawer());

  // журнал и палитра
  await z.nav(boss, 'grants');
  const jr = await boss.evaluate(() => [...document.querySelectorAll('#auditTable tr')].map(r => r.innerText.replace(/\s+/g, ' ')).filter(t => /долг владельцу: /.test(t)).slice(0, 4));
  z.check('«Журнал»: «долг владельцу: возврат», «долг владельцу: отмена возврата»', ['возврат', 'отмена возврата'].every(w => jr.some(t => t.includes('долг владельцу: ' + w))), JSON.stringify(jr));
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'долг'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(1800);
  const where = await boss.evaluate(() => ({ tab: curTab, acc: (document.querySelector('#accTabs button.on') || {}).textContent, drawer: document.getElementById('drawerTitle').textContent,
    open: document.getElementById('drawer').classList.contains('open') }));
  z.check('палитра: «долг» → «Долг клуба владельцу», Enter — «Расходы» и шторка долга', items[0] === 'Долг клуба владельцу' && where.tab === 'acc' && where.acc === 'Расходы'
    && where.open && where.drawer === 'Долг клуба владельцу', JSON.stringify({ items, where }));
  await z.done();
})().catch(z.crash);
