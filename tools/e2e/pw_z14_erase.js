// З14-и4 «Личные данные уволенного», браузер (№26; З-Р19). «Персонал» → «Уволенные» → «Карточка»: правка сохраняется
// (раньше — «Карточка заводится сотруднику»), внизу — «Стереть личные данные» с пояснением, что сотрётся и что останется;
// «Точно стереть?» → тост «Личные данные z14g стёрты: ФИО, телефон, заметка, ставка · причина увольнения в журнале — «—»»,
// карточка открывается заново с пустыми полями, в «Уволенных» под ником больше нет ФИО; журнал «Персонал» — «персонал:
// данные стёрты». У работающего кнопки нет. Телефон 360 — карточка без прокрутки вбок. Данные: временный z14g (карточка
// и увольнение с причиной — через API), уходит проверкой; денег не трогает.
const z = require('./_z');
(async () => {
  const G = 'z14g';
  const ug = await z.hireTemp(G);
  const card = { full_name: 'Ян Браузерный', phone: '+48 500 000 003', position: 'кассир', hired_at: z.clubDay(-30), rate_type: 'hour', rate_amount: 30, note: 'z14 заметка' };
  const r1 = await z.api('PUT', `/admin/staff/${ug}/profile`, card, 'boss');
  const r2 = await z.api('POST', `/admin/staff/${ug}/dismiss`, { reason: 'z14 причина из браузера' }, 'boss');
  if (!z.check('данные: у z14g карточка с ФИО, телефоном, ставкой и заметкой; уволен с причиной', r1.status === 200 && r2.status === 200,
    JSON.stringify({ r1: r1.status, r2: r2.status }))) return z.done();
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const val = (p, id) => p.evaluate(i => (document.getElementById(i) || {}).value, id);

  const boss = await z.openAdmin('boss');
  const archive = async p => { await z.nav(p, 'staff'); await p.click('#s-staff button:has-text("Уволенные")'); await z.sleep(1500); };
  const row = (p, nick) => p.evaluate(n => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    return tr ? tr.cells[0].innerText.replace(/\s+/g, ' ').trim() : null; }, nick);
  const openCard = async (p, nick) => { await p.evaluate(n => { const tr = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && (x.cells[0].querySelector('b') || {}).textContent === n);
    const b = tr && [...tr.querySelectorAll('button')].find(x => x.textContent.trim() === 'Карточка'); if (b) b.click(); }, nick); await z.sleep(1200); };
  await archive(boss);
  const a0 = await row(boss, G);
  await openCard(boss, G);
  const hint = norm(await z.text(boss, '#scErase'));
  z.check('«Уволенные»: под ником — ФИО; в карточке — «Стереть личные данные» и что сотрётся, что останется',
    a0 === `${G} Ян Браузерный` && hint === 'Стереть личные данные — ФИО, телефон, заметка и ставка станут пустыми, причина увольнения в журнале — «—». '
      + 'Ник, игровой аккаунт, даты, должность, табель и выплаты останутся.', JSON.stringify({ a0, hint }));
  await boss.fill('#sc_phone', '+48 500 000 004');
  await boss.click('#drawer button:has-text("Сохранить")');
  await z.sleep(1500);
  const t1 = norm(await z.text(boss, '#toast'));
  const ph = z.sql(`SELECT phone || '|' || COALESCE(dismissed_at::text, '') FROM staff_profiles WHERE user_id = '${ug}'`);
  z.check('карточка уволенного сохраняется: тост «Карточка z14g сохранена», телефон новый, он по-прежнему уволен',
    t1 === `Карточка ${G} сохранена` && ph === `+48 500 000 004|${z.clubDay(0)}`, JSON.stringify({ t1, ph }));
  await archive(boss);
  await openCard(boss, G);
  await z.shot(boss, 'z14_erase_card');
  await boss.click('#scErase button:has-text("Стереть личные данные")');
  await z.sleep(300);
  const armed = norm(await z.text(boss, '#scErase button'));
  await boss.click('#scErase button:has-text("Точно стереть?")');
  await z.sleep(2500);
  const t2 = norm(await z.text(boss, '#toast'));
  const after = { name: await val(boss, 'sc_name'), phone: await val(boss, 'sc_phone'), note: await val(boss, 'sc_note'),
    rate: await val(boss, 'sc_ratetype'), pos: await val(boss, 'sc_pos') };
  z.check('«Стереть личные данные» → «Точно стереть?» → тост «Личные данные z14g стёрты: ФИО, телефон, заметка, ставка · причина увольнения в '
    + 'журнале — «—»»; карточка заново: пусто, ставки нет, должность на месте', armed === 'Точно стереть?'
    && t2 === `Личные данные ${G} стёрты: ФИО, телефон, заметка, ставка · причина увольнения в журнале — «—»`
    && after.name === '' && after.phone === '' && after.note === '' && after.rate === 'none' && after.pos === 'кассир', JSON.stringify({ armed, t2, after }));
  await z.shot(boss, 'z14_erase_done');
  await boss.evaluate(() => closeDrawer());
  await archive(boss);
  const a1 = await row(boss, G);
  z.check('«Уволенные»: z14g на месте, ФИО под ником больше нет', a1 === G, a1);

  await z.nav(boss, 'staff');
  await boss.click('#s-staff button:has-text("Работают")');
  await z.sleep(1500);
  await openCard(boss, 'admin1');
  const noBtn = await boss.evaluate(() => !document.getElementById('scErase'));
  await boss.evaluate(() => closeDrawer());
  z.check('у работающего (admin1) кнопки «Стереть личные данные» нет', noBtn);

  await z.nav(boss, 'grants');
  await boss.click('#s-grants .segs button[data-k="staff"]');
  await z.sleep(1800);
  const au = await boss.evaluate(n => [...document.querySelectorAll('#auditTable tr')].filter(x => x.cells[2] && x.cells[2].innerText.trim() === n)
    .map(x => x.cells[1].innerText.trim() + ' | ' + x.cells[3].innerText.trim()), G);
  z.check('журнал «Персонал»: «персонал: данные стёрты — z14g · личные данные стёрты: …»; у увольнения причина — «—»',
    /^персонал: данные стёрты \| z14g · личные данные стёрты: ФИО, телефон, заметка, ставка; причина увольнения в журнале — «—» \(строк: \d+\)$/.test(au[0] || '')
    && au.some(x => x.startsWith(`персонал: уволен | ${G} · уволен `) && x.endsWith(' · —')) && !au.some(x => x.includes('z14 причина')), JSON.stringify(au.slice(0, 4)));

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const phn = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await archive(phn);
  await openCard(phn, G);
  const fit = await phn.evaluate(() => { const b = document.getElementById('scErase'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(phn, 'z14_erase_phone');
  z.check('телефон 360: карточка уволенного с «Стереть личные данные» — без прокрутки вбок', fit.sw <= 360 && fit.box, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
