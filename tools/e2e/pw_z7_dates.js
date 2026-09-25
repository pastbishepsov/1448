// З7-и2 (№42), браузер: даты в формах владельца — «дд.мм.гггг» в семи местах: день журнала, «с» и «по» в отчётах,
// запись табеля, наём, увольнение, карточка сотрудника. «17.09.2026» уходит на сервер как 2026-09-17, кривая дата
// подсвечена и никуда не уходит. Записи табеля, найма, увольнения и карточки перехватываются в браузере и до сервера
// не доходят — стенд не меняется.
const z = require('./_z');
const DDMM = /^\d\d\.\d\d\.\d{4}$/;
(async () => {
  const page = await z.openAdmin('boss');
  const reqs = [];
  page.on('request', r => reqs.push({ q: r.method() + ' ' + r.url(), b: r.postData() || '' }));
  await page.route(/\/admin\/(work|staff\/hire|staff\/[^/]+\/dismiss|staff\/[^/]+\/profile)$/, route =>
    route.request().method() === 'GET' ? route.continue()
      : route.fulfill({ status: 200, contentType: 'application/json',
          body: JSON.stringify({ nickname: 'z7', dismissed_at: '2026-09-17', removed_shifts: 0 }) }));
  const mark = () => reqs.length;
  const sent = (from, re) => reqs.slice(from).filter(x => re.test(x.q));
  const isText = sel => page.$eval(sel, e => e.type !== 'date').catch(() => false);
  const bad = sel => page.$eval(sel, e => e.classList.contains('bad')).catch(() => false);
  const a2 = z.uid('admin2');

  // ── журнал: день клубных суток ──
  await z.nav(page, 'grants');
  if (await isText('#audDay')) {
    let n = mark();
    await page.fill('#audDay', '17.09.2026');
    await page.press('#audDay', 'Enter');
    await z.sleep(1200);
    z.check('Журнал, день: «17.09.2026» → запрос ?day=2026-09-17', sent(n, /\/admin\/audit\?.*day=2026-09-17/).length > 0,
      reqs.slice(n).map(x => x.q).join(' | '));
    await page.fill('#audDay', '31.02.2026');
    await page.press('#audDay', 'Enter');
    await z.sleep(600);
    const st = await page.evaluate(() => audDay);
    z.check('Журнал: «31.02.2026» — поле подсвечено, день остался 2026-09-17', await bad('#audDay') && st === '2026-09-17', st);
    await page.evaluate(() => audDaySet(''));
  } else z.check('Журнал, день: поле «дд.мм.гггг»', false, 'type=date — порядок как в системе');

  // ── отчёты: «с» и «по» ──
  await z.nav(page, 'stats');
  if (await isText('#repFrom') && await isText('#repTo')) {
    const shown = [await page.inputValue('#repFrom'), await page.inputValue('#repTo')];
    z.check('Отчёты: после пресета «с» и «по» — дд.мм.гггг', shown.every(v => DDMM.test(v)), shown.join(' — '));
    let n = mark();
    await page.fill('#repFrom', '17.09.2026');
    await page.fill('#repTo', '18.09.2026');
    await page.click('#s-stats button:has-text("Показать")');
    await z.sleep(1500);
    z.check('Отчёты: 17.09.2026 — 18.09.2026 → ?from=2026-09-17&to=2026-09-18', sent(n, /\/admin\/reports\/\w+\?from=2026-09-17&to=2026-09-18$/).length > 0,
      reqs.slice(n).map(x => x.q).join(' | '));
    n = mark();
    await page.fill('#repTo', '32.09.2026');
    await page.click('#s-stats button:has-text("Показать")');
    await z.sleep(800);
    z.check('Отчёты: «по» 32.09.2026 — подсвечено, отчёт не запрошен', await bad('#repTo') && sent(n, /\/admin\/reports\//).length === 0,
      reqs.slice(n).map(x => x.q).join(' | '));
    await z.shot(page, 'z7_dates_reports');
  } else z.check('Отчёты: «с» и «по» — поля «дд.мм.гггг»', false, 'type=date — порядок как в системе');

  // ── формы персонала: запись табеля, наём, увольнение, карточка ──
  await z.nav(page, 'staff');
  const forms = [
    { name: 'Запись табеля', sel: '#ts_date', key: 'date', re: /POST .*\/admin\/work$/, today: true,
      open: () => page.evaluate(() => tsEdit('')),
      fill: async () => { await page.fill('#ts_nick', 'admin2'); await page.fill('#ts_start', '10:00'); await page.fill('#ts_end', '12:00'); },
      send: () => page.evaluate(() => tsSave('')) },
    { name: 'Наём', sel: '#hr_hired', key: 'hired_at', re: /POST .*\/admin\/staff\/hire$/, today: true,
      open: () => page.evaluate(() => hireDrawer()),
      fill: async () => { await page.fill('#hr_nick', 'z7date'); await page.fill('#hr_pass', 'Admin12345'); },
      send: () => page.evaluate(() => doHire()) },
    { name: 'Увольнение', sel: '#dm_date', key: 'date', re: /POST .*\/admin\/staff\/[^/]+\/dismiss$/, today: true,
      open: () => page.evaluate(id => dismissDrawer(id, 'admin2'), a2), fill: async () => {},
      send: () => page.evaluate(id => doDismiss(id, 'admin2'), a2) },
    { name: 'Карточка сотрудника', sel: '#sc_hired', key: 'hired_at', re: /PUT .*\/admin\/staff\/[^/]+\/profile$/, today: false,
      open: () => page.evaluate(id => staffCard(id), a2), fill: async () => {},
      send: () => page.evaluate(id => saveStaffCard(id), a2) },
  ];
  for (const f of forms) {
    await f.open();
    await z.sleep(900);
    if (!(await isText(f.sel))) { z.check(`${f.name}: поле «дд.мм.гггг»`, false, 'type=date — порядок как в системе'); continue; }
    if (f.today) z.check(`${f.name}: по умолчанию сегодня в виде дд.мм.гггг`, DDMM.test(await page.inputValue(f.sel)), await page.inputValue(f.sel));
    await f.fill();
    await page.fill(f.sel, '31.02.2026');
    let n = mark();
    await f.send();
    await z.sleep(700);
    z.check(`${f.name}: «31.02.2026» — подсвечено, запроса нет`, await bad(f.sel) && sent(n, f.re).length === 0, reqs.slice(n).map(x => x.q).join(' | '));
    if (f.name === 'Наём') await z.shot(page, 'z7_dates_hire');
    await page.fill(f.sel, '17.09.2026');
    n = mark();
    await f.send();
    await z.sleep(900);
    const r = sent(n, f.re)[0];
    let body = {}; try { body = JSON.parse((r || {}).b || '{}'); } catch (e) {}
    z.check(`${f.name}: «17.09.2026» → ${f.key} = 2026-09-17`, body[f.key] === '2026-09-17', r ? r.b.slice(0, 160) : 'запроса нет');
    await page.evaluate(() => closeDrawer());
    await z.sleep(300);
  }
  await z.done();
})().catch(z.crash);
