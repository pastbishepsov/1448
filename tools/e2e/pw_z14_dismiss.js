// З14-и1 «Увольнение закрывает смену и кассу», браузер (№24; З-Р18). «Персонал» → «Уволить»: шторка заранее говорит,
// что открыта смена с ЧЧ:ММ и касса не сдана — увольнение закроет смену пометкой «закрыта увольнением» (часы — в
// табель на проверку) и запишет кассу сданной без пересчёта, сколько должно быть в ящике; сколько будущих смен
// снимется. Давняя смена — «уход не отмечен», закроется по графику. После «Уволить» — тост «смена закрыта, касса — без
// пересчёта», в табеле — «закрыта увольнением · касса не считана», в сводке смены — «·авто» с пояснением, чип «на
// смене» в шапке — без уволенного. Телефон 360 — шторка без прокрутки вбок.
// Данные: временные z14w (входит в админку — смена открывается; на завтра — смена в графике) и z14q (вчерашняя
// открытая запись — SQL-ом), оба уволены к концу проверки; денег не трогает.
const z = require('./_z');
(async () => {
  const W = 'z14w', Q = 'z14q';
  const uw = await z.hireTemp(W), uq = await z.hireTemp(Q);
  const r = await z.login(W, undefined, { client: 'admin' }); // вход в админку открывает смену
  const entry = z.sql(`SELECT id FROM work_entries WHERE user_id = '${uw}' AND ended_at IS NULL`);
  const since = z.sql(`SELECT to_char(started_at AT TIME ZONE 'Europe/Warsaw', 'HH24:MI') FROM work_entries WHERE id = '${entry}'`);
  const TODAY = z.clubDay(0), YDAY = z.clubDay(-1), TOMORROW = z.clubDay(1);
  const s0 = z.sql('SELECT id FROM shifts WHERE active ORDER BY sort, start_min LIMIT 1');
  const sa = await z.api('POST', `/admin/shifts/${s0}/assign`, { date: TOMORROW, nickname: W }, 'boss');
  const day = z.sql('SELECT id || \'|\' || start_min || \'|\' || end_min FROM shifts WHERE active AND start_min < end_min ORDER BY start_min LIMIT 1').split('|');
  const hm = m => String(Math.floor(m / 60)).padStart(2, '0') + ':' + String(m % 60).padStart(2, '0');
  const club = z.sql(`SELECT club_id FROM work_entries WHERE id = '${entry}'`);
  const old = day.length === 3 ? z.sql(`INSERT INTO work_entries (club_id, user_id, shift_id, date, started_at, note, created_by) VALUES ('${club}', '${uq}', '${day[0]}', '${YDAY}',
    (('${YDAY}'::date + time '${hm(+day[1] + 60)}') AT TIME ZONE 'Europe/Warsaw'), 'открыта входом в админку', '${uq}') RETURNING id`) : '';
  if (!z.check('данные: у z14w открыта смена, на завтра — смена в графике; у z14q — вчерашняя открытая запись',
    r.status === 200 && entry && [200, 201].includes(sa.status) && old, JSON.stringify({ r: r.status, entry, sa: sa.status, old }))) return z.done();
  const pv = (await z.api('GET', `/admin/staff/${uw}/dismiss`, undefined, 'boss')).data;
  const pln = v => (+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).replace(/\s/g, ' ');
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();

  const boss = await z.openAdmin('boss');
  const openFor = async (p, nick) => {
    await z.nav(p, 'staff');
    await p.waitForSelector('#staffTable tr', { timeout: 15000 }).catch(() => {});
    await p.evaluate(n => { const row = [...document.querySelectorAll('#staffTable tr')].find(x => x.cells[0] && x.cells[0].innerText.split('\n')[0].trim() === n);
      if (row) row.querySelector('button.danger').click(); }, nick);
    await p.waitForSelector('#dm_date', { timeout: 15000 }).catch(() => {});
    await z.sleep(400);
    return p.evaluate(() => ({ warn: ((document.getElementById('dmShift') || {}).textContent || '').replace(/\s+/g, ' ').trim(),
      note: [...document.querySelectorAll('#drawer .uform > .muted')].map(x => x.textContent.replace(/\s+/g, ' ').trim()).pop() || '' }));
  };
  const d0 = await openFor(boss, W);
  const want0 = `⚠ Открыта смена с ${since}, касса не сдана. Увольнение закроет смену сейчас пометкой «закрыта увольнением»: `;
  z.check(`шторка «Увольнение · z14w»: «Открыта смена с ${since}, касса не сдана», закроется сейчас пометкой «закрыта увольнением», часы — в табель на `
    + `проверку; касса — без пересчёта, «в ящике должно быть ${pln(pv.cash && pv.cash.expected_pln)} zł»; «С будущих смен в графике снимется: 1»`,
    d0.warn.startsWith(want0) && /0 ч 0\d м встанут в табель на проверку — в выплату войдут, когда поправишь запись\./.test(d0.warn)
    && d0.warn.endsWith(`Кассу запишет сданной без пересчёта: в ящике должно быть ${pln(pv.cash && pv.cash.expected_pln)} zł — пересчитай его.`)
    && /С будущих смен в графике снимется: 1\.$/.test(d0.note), JSON.stringify(d0));
  await z.shot(boss, 'z14_dismiss');
  await boss.evaluate(() => closeDrawer());
  const d1 = await openFor(boss, Q);
  const yd = YDAY.slice(8, 10) + '.' + YDAY.slice(5, 7) + '.' + YDAY.slice(0, 4), mins = +day[2] - +day[1] - 60;
  z.check(`давняя смена z14q: «Открыта смена с ${yd} ${hm(+day[1] + 60)} — уход не отмечен», закроется по графику — `
    + `${Math.floor(mins / 60)} ч ${String(mins % 60).padStart(2, '0')} м в табель на проверку; будущих смен нет`,
    d1.warn.startsWith(`⚠ Открыта смена с ${yd} ${hm(+day[1] + 60)} — уход не отмечен, касса не сдана. Увольнение закроет смену по графику пометкой «закрыта увольнением»: `
      + `${Math.floor(mins / 60)} ч ${String(mins % 60).padStart(2, '0')} м встанут в табель на проверку`) && /Будущих смен в графике у него нет\.$/.test(d1.note),
    JSON.stringify(d1));
  await boss.evaluate(() => closeDrawer());

  await openFor(boss, W);
  await boss.click('#drawer button.danger:has-text("Уволить")');
  await z.sleep(2500);
  const t = norm(await z.text(boss, '#toast'));
  const ts = await boss.evaluate(id => { const tr = [...document.querySelectorAll('#tsTable tr')].find(x => x.innerHTML.includes(id)); // строка записи — по её id в кнопках
    const cash = tr ? [...tr.querySelectorAll('span')].find(s => s.textContent.includes('касса не считана')) : null;
    return tr ? { hours: tr.cells[4].innerText.replace(/\s+/g, ' ').trim(), title: cash ? cash.title : '' } : null; }, entry);
  const staff = await boss.evaluate(n => [...document.querySelectorAll('#staffTable tr')].some(x => x.cells[0] && x.cells[0].innerText.split('\n')[0].trim() === n), W);
  const chip = await z.text(boss, '#shiftTxt');
  z.check(`«Уволить» — тост «z14w уволен ${TODAY.slice(8, 10)}.${TODAY.slice(5, 7)}.${TODAY.slice(0, 4)} · смена закрыта, касса — без пересчёта · снят с 1 будущих смен»; `
    + 'в команде его нет; в табеле — «закрыта увольнением · касса не считана» с подсказкой «уволен — касса записана без пересчёта»; чип в шапке — без него',
    t === `${W} уволен ${TODAY.slice(8, 10)}.${TODAY.slice(5, 7)}.${TODAY.slice(0, 4)} · смена закрыта, касса — без пересчёта · снят с 1 будущих смен`
    && !staff && !!ts && /^0 ч 0\d м · закрыта увольнением · касса не считана$/.test(ts.hours) && ts.title === 'уволен — касса записана без пересчёта'
    && !String(chip).includes(W), JSON.stringify({ t, staff, ts, chip }));
  await z.shot(boss, 'z14_dismiss_timesheet');

  await z.nav(boss, 'stats');
  await boss.waitForSelector('#shWorked tr', { timeout: 20000 }).catch(() => {});
  await z.sleep(800);
  const sw = await boss.evaluate(n => { const row = [...document.querySelectorAll('#shWorked tr')].find(x => x.cells[0] && x.cells[0].innerText.trim() === n);
    const a = row ? [...row.querySelectorAll('span')].find(s => s.textContent.includes('авто')) : null; return row ? { left: row.cells[2].innerText.trim(), title: a ? a.title : '' } : null; }, W);
  z.check('«Отчёты» → сводка смены: у z14w — время ухода «·авто» с подсказкой «закрыта увольнением — часы проверь в табеле»',
    !!sw && /^\d\d:\d\d ·авто$/.test(sw.left) && sw.title === 'закрыта увольнением — часы проверь в табеле', JSON.stringify(sw));

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  const d2 = await openFor(ph, Q);
  const fit = await ph.evaluate(() => { const b = document.getElementById('dmShift'), r = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!r && r.left >= 0 && r.right <= innerWidth }; });
  await z.shot(ph, 'z14_dismiss_phone');
  z.check('телефон 360: шторка увольнения с предупреждением — без прокрутки вбок', d2.warn.length > 0 && fit.sw <= 360 && fit.box, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
