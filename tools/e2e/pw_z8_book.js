// З8-и3 «Бухгалтерия» (№78), браузер: у владельца в рейле — «Бухгалтерия», вкладка «Касса». Сверху — «В ящике
// сейчас» (то же «должно быть», что у админа в «Моей смене»), пришло и ушло за период, расхождения; кнопки
// «Инкассация» и «Коррекция остатка»; ниже — кассовая книга по дням, свежий день сверху, клик по дню раскрывает
// сдачи и события ящика. Инкассация, коррекция и отмена события — отсюда, с тостами. Админ раздела не видит;
// палитра: «касс» у владельца ведёт в «Бухгалтерию», «инкасс» у админа — в «Мою смену» с открытой формой. На
// телефоне (360 px) нижняя панель владельца с новой кнопкой влезает целиком, у чисел дня — подписи.
// Временный сотрудник z8bp; события владельца в конце отменяются — ящик стенда как был, z8bp уволен.
const z = require('./_z');
(async () => {
  await z.hireTemp('z8bp');
  const pv = async () => (await z.api('GET', '/admin/work/closeout', undefined, 'z8bp')).data;
  const book = async (q = 'preset=today') => (await z.api('GET', '/admin/cash/book?' + q, undefined, 'boss')).data;
  const adm = await z.openAdmin('z8bp'); // вход в админку открывает смену
  if ((await pv()).baseline) { // ящика на стенде ещё нет — заводит его z8bp сдачей на 500 zł
    await z.api('POST', '/admin/work/stop', { counted_pln: 500 }, 'z8bp');
    await z.api('POST', '/admin/work/start', {}, 'z8bp');
  }
  const events = [];
  z.atEnd(async () => { for (const id of events) await z.api('POST', `/admin/cash/events/${id}/void`, {}, 'boss'); });
  const lastEvent = kind => z.sql(`SELECT id FROM cash_events WHERE kind='${kind}' AND created_by='${z.uid('boss')}' ORDER BY created_at DESC LIMIT 1`);
  const nEvents = () => z.sql(`SELECT count(*) FROM cash_events WHERE created_by='${z.uid('boss')}'`);

  const a = await adm.evaluate(() => { const b = document.getElementById('r-acc'); nav('acc');
    return { hidden: !b || b.classList.contains('hidden') || !b.offsetParent, tab: curTab }; });
  z.check('у админа «Бухгалтерии» в рейле нет, nav("acc") его туда не пускает', a.hidden && a.tab !== 'acc', JSON.stringify(a));
  await adm.evaluate(() => loadMyShift());
  await adm.evaluate(() => palOpen()); await adm.fill('#palInp', 'инкасс'); await z.sleep(500);
  const ai = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await adm.keyboard.press('Enter'); await z.sleep(1500);
  const af = await adm.evaluate(() => ({ drawer: document.getElementById('drawer').classList.contains('open') ? document.getElementById('drawerTitle').textContent : '',
    form: !!document.getElementById('ccAmount') && !!document.getElementById('ccAmount').offsetParent }));
  z.check('палитра админа: «инкасс» → «Инкассация», Enter — «Моя смена» с открытой формой инкассации', ai[0] === 'Инкассация' && /Моя смена/.test(af.drawer) && af.form,
    JSON.stringify({ ai, af }));

  const boss = await z.openAdmin('boss');
  const F = v => boss.evaluate(x => fmtPLN(x).replace(/\s+/g, ' '), v); // деньги так, как их пишет страница
  const kpis = () => boss.evaluate(() => [...document.querySelectorAll('#accBody .kpi')].map(k => k.textContent.replace(/\s+/g, ' ').trim()));
  const toast = async () => { await z.sleep(1600); return (await z.text(boss, '#toast')) || ''; };
  if (!z.check('владелец: в рейле «Бухгалтерия»', await boss.isVisible('#r-acc'))) return z.done();
  await boss.click('#r-acc'); await z.sleep(1800);
  const d7 = await book('preset=d7'), k0 = await kpis();
  const now0 = d7.now.expected_pln;
  z.check(`«В ящике сейчас» — «должно быть» сдачи: ${await F(now0)} zł; пришло и ушло — итоги недели`,
    k0[0] === `В ящике сейчас ${await F(now0)} zł` && Math.abs((await pv()).expected_pln - now0) < 0.005
    && k0[1] === `Пришло за период ${await F(d7.totals.in_pln)} zł` && k0[2] === `Ушло за период ${await F(d7.totals.out_pln)} zł`, JSON.stringify(k0));
  const tr = await boss.evaluate(() => [...document.querySelectorAll('#accBody tr.accDay')].map(r => ({ date: r.dataset.date,
    cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) }))); // innerText — без скрытых подписей телефона
  const today = d7.days[d7.days.length - 1], yday = d7.days[d7.days.length - 2];
  z.check('книга: 7 дней, свежий сверху — сегодня', tr.length === 7 && tr[0].date === z.clubDay(0) && tr[6].date === z.clubDay(-6)
    && /сегодня/.test(tr[0].cells[0]), JSON.stringify(tr.map(r => r.date)));
  const cell = async v => v == null ? '—' : await F(v); // «—» — ящика ещё не было
  z.check('строки дней: «на начало» и «на конец» — из книги, конец вчера = начало сегодня',
    tr[0].cells[1] === await cell(today.opening_pln) && tr[0].cells.at(-1) === await cell(today.closing_pln)
    && tr[1].cells.at(-1) === await cell(yday.closing_pln) && tr[1].cells.at(-1) === tr[0].cells[1], JSON.stringify(tr.slice(0, 2))); // «на конец» — последняя колонка (З9-и3а: + «Расходы», З9-и3б: + «Владельцу»)
  await z.shot(boss, 'z8_book');
  await boss.click(`#accBody tr.accDay[data-date="${z.clubDay(0)}"]`); await z.sleep(400);
  const det = await z.text(boss, '#accBody tr.accDet');
  z.check('клик по дню — сдачи дня и инкассации с коррекциями', /Сдачи/.test(det || '') && /Инкассации и коррекции/.test(det || ''), (det || '').slice(0, 300));

  await boss.click('#accColBtn'); await z.sleep(500);
  z.check('«Инкассация» — шторка с суммой, «В банк» / «В сейф», заметкой', await z.text(boss, '#drawerTitle') === 'Инкассация'
    && await boss.isVisible('#acAmount') && await boss.isVisible('#acNote'));
  const n0 = nEvents();
  await boss.click('#acSubmit'); await z.sleep(200); await boss.click('#acSubmit');
  const t0 = await toast();
  z.check('без суммы — «сумма больше нуля», ничего не записано', /сумма больше нуля/.test(t0) && nEvents() === n0, t0);
  await boss.fill('#acAmount', '12');
  await boss.click('#drawerBody button:has-text("В сейф")'); await z.sleep(300);
  await boss.fill('#acNote', 'в сейф на ночь');
  await z.shot(boss, 'z8_book_collect');
  await boss.click('#acSubmit'); await z.sleep(200); await boss.click('#acSubmit');
  const t1 = await toast();
  const col = lastEvent('collect'); if (col) events.push(col);
  const now1 = Math.round((now0 - 12) * 100) / 100;
  z.check(`тост «✓ Инкассация 12,00 zł в сейф записана · в ящике должно быть ${await F(now1)} zł», в книге — минус 12`,
    t1.includes(`Инкассация 12,00 zł в сейф записана · в ящике должно быть ${await F(now1)} zł`)
    && (await kpis())[0] === `В ящике сейчас ${await F(now1)} zł`, t1);

  await boss.click('#accCorBtn'); await z.sleep(500);
  const cb = await z.text(boss, '#drawerBody');
  z.check(`«Коррекция остатка»: по расчёту ${await F(now1)} zł`, await z.text(boss, '#drawerTitle') === 'Коррекция остатка'
    && (cb || '').includes(`По расчёту в ящике сейчас ${await F(now1)} zł`), (cb || '').slice(0, 200));
  const stated = Math.round((now1 + 7) * 100) / 100;
  await boss.fill('#acStated', String(stated)); await z.sleep(200);
  const hint = await z.text(boss, '#acDelta');
  await boss.click('#acCorSubmit'); await z.sleep(200); await boss.click('#acCorSubmit');
  const t2 = await toast();
  z.check('поправка видна до записи (+7,00); без причины — «нужна причина»', hint === 'Поправка +7,00 zł' && /нужна причина/.test(t2), `${hint} / ${t2}`);
  await boss.fill('#acReason', 'пересчитал ящик');
  await z.shot(boss, 'z8_book_correct');
  await boss.click('#acCorSubmit'); await z.sleep(200); await boss.click('#acCorSubmit');
  const t3 = await toast();
  const cor = lastEvent('correct'); if (cor) events.push(cor);
  z.check(`тост «✓ Остаток поправлен: в ящике ${await F(stated)} zł, поправка +7,00 zł», «В ящике сейчас» — ${await F(stated)}`,
    t3.includes(`Остаток поправлен: в ящике ${await F(stated)} zł, поправка +7,00 zł`) && (await kpis())[0] === `В ящике сейчас ${await F(stated)} zł`, t3);

  const line = () => boss.evaluate(() => [...document.querySelectorAll('#accBody tr.accDet div')].filter(e => /инкассация 12,00 zł в сейф/.test(e.textContent))
    .map(e => ({ t: e.textContent.replace(/\s+/g, ' ').trim(), off: e.classList.contains('accVoid') })).pop() || null);
  const l0 = await line();
  const vb = await boss.$$('#accBody tr.accDet div:has-text("инкассация 12,00 zł в сейф") button:has-text("Отменить")');
  if (vb.length) { await vb[vb.length - 1].click(); await z.sleep(200); await vb[vb.length - 1].click(); }
  const t4 = await toast(), l1 = await line();
  const now2 = Math.round((stated + 12) * 100) / 100;
  z.check(`отмена инкассации из книги: тост «в ящике должно быть ${await F(now2)} zł», строка зачёркнута — «отменил boss»`,
    l0 && !l0.off && t4.includes(`Событие отменено · в ящике должно быть ${await F(now2)} zł`) && l1 && l1.off && /отменил boss/.test(l1.t), JSON.stringify({ l0, t4, l1 }));
  await z.shot(boss, 'z8_book_day');

  await boss.evaluate(() => nav('hall')); await z.sleep(600);
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'касс'); await z.sleep(500);
  const oi = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(1500);
  z.check('палитра владельца: «касс» → «Касса», Enter — «Бухгалтерия»', oi[0] === 'Касса' && await boss.evaluate(() => curTab) === 'acc', JSON.stringify(oi));

  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'acc');
  const pr = await ph.evaluate(() => { const r = document.getElementById('rail'), bs = [...r.querySelectorAll('button')].filter(b => b.offsetParent);
    const c = document.querySelector('#accBody tr.accDay'), lab = c ? [...c.querySelectorAll('.accL')].filter(x => x.offsetParent).map(x => x.textContent) : [];
    return { fits: r.scrollWidth <= r.clientWidth && bs.every(b => b.getBoundingClientRect().right <= innerWidth), n: bs.length, lab, page: document.documentElement.scrollWidth }; });
  z.check('телефон 360 px: нижняя панель владельца — все 12 кнопок на экране, у чисел дня подписи', pr.fits && pr.n === 12 && pr.page <= 360
    && pr.lab.join(',') === 'на начало,пришло,возвраты,инкассации,расходы,владельцу,коррекции,расхождения,на конец', JSON.stringify(pr));
  await ph.evaluate(() => document.querySelector('#accBody .tablewrap').scrollIntoView());
  await z.shot(ph, 'z8_book_phone');
  await z.done();
})().catch(z.crash);
