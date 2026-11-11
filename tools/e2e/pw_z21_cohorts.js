// З21-и2б «Когорты — экран» (№47, З-Р28), браузер. «Отчёты» → «Гости»: внизу блок «Когорты · гости по месяцу первого
// визита» — итоги за 12 месяцев (новых, вернулись через месяц, LTV гостя) и таблица: 12 месяцев, старый сверху, новых,
// через 1/2/3 мес — доля и число («…» — месяц ещё идёт, «—» — не начался), LTV на гостя; пустые месяцы в начале — одной
// строкой «… — …: новых гостей не было». Строки — как в ответе сервера; от периода не зависит; телефон 360 — карточки в
// экране. Данных не заводит.
const z = require('./_z');
(async () => {
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'stats');
  const settle = () => boss.waitForFunction(() => { const b = document.getElementById('repBody'); return b && !b.querySelector('.skel'); }, null,
    { timeout: 20000 }).catch(() => {}).then(() => z.sleep(500));
  await boss.evaluate(() => repTabSet('guests', document.querySelector('#repTabs button[data-t="guests"]'))); await settle();
  const co = (await z.api('GET', '/admin/reports/cohorts', undefined, 'boss')).data || {};
  const read = page => page.evaluate(() => {
    const t = document.querySelector('#repBody .cohTable');
    if (!t) return null;
    return { head: (document.getElementById('repCohorts') || {}).textContent || '',
      rows: [...t.querySelectorAll('tr[data-m]')].map(r => ({ m: r.dataset.m, empty: r.classList.contains('cohEmpty'),
        td: [...r.querySelectorAll('td')].map(td => { const c = td.cloneNode(true); c.querySelectorAll('.repL').forEach(x => x.remove());
          return c.textContent.replace(/\s+/g, ' ').trim(); }) })),
      kpis: [...document.querySelectorAll('#repCohorts ~ .kpis .kpi')].slice(0, 3).map(k => k.textContent.replace(/\s+/g, ' ').trim()) };
  });
  const v = await read(boss);
  if (!v) { // блока нет (админка до З21-и2б) — FAIL по каждой проверке, а не обрыв
    for (const n of ['шапка и строки по месяцам', 'строки как в ответе', 'итоги за 12 месяцев', 'ячейка и итоги во всех случаях',
      'от периода не зависит', 'телефон 360']) z.check(n + ': блока «Когорты» нет', false);
    return z.done();
  }
  const cs = co.cohorts || [];
  // ожидаемое — по ответу сервера, своими правилами (не функциями страницы)
  const MON = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'];
  const ru = x => (+x).toLocaleString('ru-RU'), pln = x => (+x).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
  const cell = (b, size) => !b || b.state === 'future' || b.n == null ? '—' : !size ? '—'
    : norm(`${ru(b.pct)}% · ${b.n}${b.state === 'going' ? ' …' : ''}`);
  const mon = m => MON[+m.slice(5, 7) - 1] + ' ' + m.slice(0, 4);
  const lead = cs.findIndex(c => c.size > 0), skip = lead === -1 ? cs.length : lead, fold = skip >= 2;
  const want = (fold ? [{ m: `${cs[0].month}…${cs[skip - 1].month}`, empty: true, td: [`${mon(cs[0].month)} — ${mon(cs[skip - 1].month)}: новых гостей не было`] }] : [])
    .concat((fold ? cs.slice(skip) : cs).map(c => ({ m: c.month, empty: !c.size, td: [mon(c.month), String(c.size),
      ...(c.back || []).map(b => cell(b, c.size)), c.ltv_pln == null ? '—' : norm(pln(c.ltv_pln) + ' zł')] })));
  z.check(`шапка блока — «${norm(v.head)}»; ${cs.length} месяцев, старый сверху, этот (${co.month}) — последним`
    + (fold ? `; пустые в начале (${skip}) — одной строкой` : ''),
    norm(v.head) === 'Когорты · гости по месяцу первого визита' && cs.length === 12 && v.rows.length === want.length
    && (v.rows[v.rows.length - 1] || {}).m === co.month && v.rows.every((r, i) => !i || r.m > v.rows[i - 1].m), JSON.stringify(v.rows.map(r => r.m)));
  const diff = v.rows.map((r, i) => [r, want[i]]).filter(([a, b]) => JSON.stringify({ ...a, td: a.td.map(norm) }) !== JSON.stringify(b));
  z.check('строки — как в ответе: месяц, новых, через 1/2/3 мес (доля · число, «…» — идёт, «—» — не начался), LTV; пустая — бледная',
    v.rows.length === want.length && diff.length === 0, JSON.stringify(diff.slice(0, 2)));
  let size = 0, rev = 0, n1 = 0, s1 = 0;
  for (const c of cs) { size += c.size; rev += c.revenue_pln || 0; const b = (c.back || [])[0]; if (b && b.state === 'done' && c.size) { n1 += b.n; s1 += c.size; } }
  const k = [`Новых за 12 месяцев ${size}`, `Вернулись через месяц ${s1 ? ru(Math.round(n1 / s1 * 1000) / 10) + '%' : '—'}`,
    `LTV гостя ${size ? pln(Math.round(rev / size * 100) / 100) + ' zł' : '—'}`];
  z.check(`итоги: «${v.kpis.join(' · ')}» — сумма когорт, через месяц — по когортам с прошедшим следующим месяцем`,
    JSON.stringify(v.kpis.map(norm)) === JSON.stringify(k.map(norm)), JSON.stringify({ got: v.kpis, want: k }));

  // ячейка и итоги — во всех случаях (функции страницы на подставленных когортах)
  const f = await boss.evaluate(() => {
    const txt = h => { const e = document.createElement('div'); e.innerHTML = h; return e.textContent.replace(/\s+/g, ' ').trim(); };
    return { cells: [txt(cohBack({ state: 'future', n: null, pct: null }, 5)), txt(cohBack({ state: 'going', n: 1, pct: 33.3 }, 3)),
      txt(cohBack({ state: 'done', n: 0, pct: 0 }, 4)), txt(cohBack({ state: 'done', n: 0, pct: null }, 0))],
      tot: cohTotals([{ size: 3, revenue_pln: 40.1, back: [{ state: 'done', n: 1 }] }, { size: 1, revenue_pln: 19.5, back: [{ state: 'going', n: 1 }] },
        { size: 0, revenue_pln: 0, back: [{ state: 'done', n: 0 }] }]) };
  });
  z.check('ячейка: «—» — месяц не начался, «33,3% · 1 …» — идёт, «0% · 0» — никто, у пустой когорты — «—»; итоги: идущий месяц не в «через месяц»',
    JSON.stringify(f.cells) === JSON.stringify(['—', '33,3% · 1 …', '0% · 0', '—'])
    && JSON.stringify(f.tot) === JSON.stringify({ size: 4, ltv: 14.9, back1: 33.3, s1: 3 }), JSON.stringify(f));

  // от периода не зависит
  await boss.evaluate(() => repPreset('yesterday', document.querySelector('#repPresets button[data-p="yesterday"]'))); await settle();
  const v2 = await read(boss);
  z.check('«Вчера» вместо «7 дней» — когорты те же', !!v2 && JSON.stringify(v2.rows) === JSON.stringify(v.rows), JSON.stringify((v2 || {}).rows || null).slice(0, 300));
  await boss.evaluate(() => { const e = document.getElementById('repCohorts'); if (e) e.scrollIntoView(); });
  await z.shot(boss, 'z21_cohorts_desktop');

  // телефон
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'stats');
  await ph.evaluate(() => repTabSet('guests', document.querySelector('#repTabs button[data-t="guests"]')));
  await ph.waitForSelector('#repBody .cohTable', { timeout: 20000 }).catch(() => {});
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth,
    r: [...document.querySelectorAll('#repBody .cohTable tr[data-m]')].map(e => Math.round(e.getBoundingClientRect().right)),
    lab: [...document.querySelectorAll('#repBody .cohTable tr[data-m]:last-child .repL')].map(e => getComputedStyle(e).display) }));
  z.check(`телефон 360: карточки когорт (${want.length}) в экране, у чисел подписи, страница не шире экрана`,
    fit.r.length === want.length && fit.r.every(x => x <= 360) && fit.sw <= 360 && fit.lab.length === 5 && fit.lab.every(d => d !== 'none'),
    JSON.stringify(fit));
  await ph.evaluate(() => { const e = document.getElementById('repCohorts'); if (e) e.scrollIntoView(); });
  await z.shot(ph, 'z21_cohorts_phone');
  await z.done();
})().catch(z.crash);
