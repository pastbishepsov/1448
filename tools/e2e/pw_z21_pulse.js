// З21-и1б «Пульс дня — экран» (№46, З-Р28), браузер. Сверху «Отчётов» — «Сегодня до ЧЧ:ММ · день недели», плитки
// «Выручка», «Сессии», «Новые гости»: число, отклонение от нормы словами («выше нормы на 25%», «ниже нормы на 30,5%»,
// «как обычно», «обычно 0», «нормы пока нет») и сама норма; строка — на чём норма и «ожидаем к концу месяца ≈ X zł (от L
// до H)». Плитка открывает отчёт за сегодня; раз в 5 минут блок обновляется. Телефон 360 — плитки в экране. Числа — из
// ответа, которым страница рисовала (pulseData), он же — как у сервера. Данных не заводит.
const z = require('./_z');
(async () => {
  const norm = s => String(s == null ? '' : s).replace(/[\s\u202f]+/g, ' ').trim();
  const boss = await z.openAdmin('boss');
  let calls = 0;
  boss.on('request', r => { if (r.url().includes('/admin/reports/pulse')) calls++; });
  await z.nav(boss, 'stats');
  await boss.waitForFunction(() => typeof pulseData !== 'undefined' && pulseData && !document.querySelector('#pulseKpis .skel'), null,
    { timeout: 20000 }).catch(() => {});
  const view = await boss.evaluate(() => {
    const d = typeof pulseData === 'undefined' ? null : pulseData; // админка до З21-и1б — переменной нет
    if (!d) return null;
    const ru = v => (+v || 0).toLocaleString('ru-RU'), pln = v => (+v || 0).toLocaleString('ru-RU',
      { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł';
    const chip = (p, n) => !n ? ['нормы пока нет', ''] : p == null ? ['обычно 0', 'up'] : p === 0 ? ['как обычно', '']
      : [(p > 0 ? 'выше' : 'ниже') + ' нормы на ' + Math.abs(p).toLocaleString('ru-RU') + '%', p > 0 ? 'up' : 'dn'];
    const tiles = [...document.querySelectorAll('#pulseKpis .kpi')].map(t => ({ k: t.dataset.k, label: t.firstChild.textContent.trim(),
      b: t.querySelector('b').textContent, i: (t.querySelector('i.dlt') || {}).textContent, cls: (t.querySelector('i.dlt') || {}).className,
      n: (t.querySelector('.pN') || {}).textContent || null, title: t.title }));
    const want = [['money', 'Выручка', d.revenue, pln], ['load', 'Сессии', d.sessions, ru], ['guests', 'Новые гости', d.new, ru]]
      .map(([k, label, m, f]) => { const [i, c] = chip(m.pct, d.weeks);
        return { k, label, b: f(m.today), i, cls: ('dlt ' + c).trim(), n: d.weeks ? 'норма ' + f(m.norm) : null }; });
    return { d, tiles, want, head: document.getElementById('pulseHead').textContent, note: document.getElementById('pulseFc').textContent };
  });
  if (!view) { // блока нет (админка до З21-и1б) — FAIL по каждой проверке, а не обрыв
    for (const n of ['шапка', 'три плитки', 'подсказка плитки', 'строка нормы и прогноза', 'отклонение словами', 'строка во всех случаях',
      'плитка → отчёт за сегодня', 'пульс и отчёт — одно число', 'раз в 5 минут', 'телефон 360']) z.check(n + ': блока пульса нет', false);
    return z.done();
  }
  const { d } = view, srv = (await z.api('GET', '/admin/reports/pulse', undefined, 'boss')).data || {};
  const WD = ['воскресенье', 'понедельник', 'вторник', 'среда', 'четверг', 'пятница', 'суббота'];
  z.check(`шапка «${norm(view.head)}» — «Сегодня до ${d.at} · ${WD[d.weekday]}», сутки и недели — как у сервера`,
    norm(view.head) === `Сегодня до ${d.at} · ${WD[d.weekday]}` && d.date === srv.date && d.weeks === srv.weeks
    && JSON.stringify(d.week_dates) === JSON.stringify(srv.week_dates), JSON.stringify({ head: view.head, d: [d.date, d.weeks], srv: [srv.date, srv.weeks] }));
  const strip = t => ({ k: t.k, label: t.label, b: t.b, i: t.i, cls: t.cls, n: t.n });
  z.check('три плитки: выручка, сессии, новые — число, отклонение словами и норма, как в ответе',
    JSON.stringify(view.tiles.map(strip)) === JSON.stringify(view.want), JSON.stringify({ got: view.tiles.map(strip), want: view.want }).slice(0, 900));
  const t0 = view.tiles[0] || {};
  z.check('подсказка плитки — недели нормы по датам и «нажми — отчёт за сегодня»',
    !d.weeks || (t0.title.startsWith('норма — медиана того же дня недели до того же часа: ') && t0.title.endsWith(' · нажми — отчёт за сегодня')
      && t0.title.split(' · ').length === d.weeks + 1), t0.title);

  // строка нормы и прогноза — по ответу
  const GEN = ['воскресенья', 'понедельника', 'вторника', 'среды', 'четверга', 'пятницы', 'субботы'];
  const GEN_PL = ['воскресений', 'понедельников', 'вторников', 'сред', 'четвергов', 'пятниц', 'суббот'];
  const MON = ['января', 'февраля', 'марта', 'апреля', 'мая', 'июня', 'июля', 'августа', 'сентября', 'октября', 'ноября', 'декабря'];
  const f = d.forecast, zl = v => Math.round(+v || 0).toLocaleString('ru-RU');
  const fc = f ? ` Ожидаем к концу ${MON[+f.month.slice(5, 7) - 1]} ≈ ${zl(f.pln)} zł (от ${zl(f.low_pln)} до ${zl(f.high_pln)}) — уже есть `
    + `${zl(f.so_far_pln)} zł, ` : '';
  const n = d.weeks, base = !n ? `Нормы пока нет: прошлых ${GEN_PL[d.weekday]} в истории клуба ещё нет — прогноз месяца появится вместе с ней.`
    : `Норма — медиана ${n === 1 ? '1 прошлого ' + GEN[d.weekday] : n + ' прошлых ' + GEN_PL[d.weekday]} до того же часа.${fc}`;
  z.check(`строка: «${norm(view.note).slice(0, 140)}…»`, norm(view.note).startsWith(norm(base))
    && (!f || /впереди \d+ (день|дня|дней)\.$|сегодня последний день месяца\.$/.test(view.note)), view.note);

  // чип и строка — во всех случаях (те же функции страницы на подставленных ответах)
  const cases = await boss.evaluate(() => {
    const c = (p, n) => { const e = document.createElement('div'); e.innerHTML = pulseChip(p, n); const i = e.firstChild; return [i.textContent, i.className]; };
    const fc = (left) => ({ month: '2026-02', pln: 12345.6, low_pln: 10000.4, high_pln: 15000.5, so_far_pln: 999.5, days_left: left });
    return { chips: [c(25, 8), c(-30.5, 8), c(0, 8), c(null, 8), c(12, 0)],
      notes: [pulseNote({ weeks: 0, weekday: 3 }), pulseNote({ weeks: 1, weekday: 2, forecast: null }),
        pulseNote({ weeks: 8, weekday: 6, forecast: fc(0) }), pulseNote({ weeks: 3, weekday: 0, forecast: fc(21) })] };
  });
  z.check('отклонение: «выше нормы на 25%», «ниже нормы на 30,5%» (красным), «как обычно», «обычно 0», «нормы пока нет»',
    JSON.stringify(cases.chips) === JSON.stringify([['выше нормы на 25%', 'dlt up'], ['ниже нормы на 30,5%', 'dlt dn'], ['как обычно', 'dlt'],
      ['обычно 0', 'dlt up'], ['нормы пока нет', 'dlt']]), JSON.stringify(cases.chips));
  const nb = s => s.replace(/[\u00a0\u202f]/g, ' ');
  z.check('строка: нормы нет — «прошлых сред … ещё нет»; одна неделя — «1 прошлого вторника»; конец месяца; «впереди 21 день»',
    JSON.stringify(cases.notes.map(nb)) === JSON.stringify([
      'Нормы пока нет: прошлых сред в истории клуба ещё нет — прогноз месяца появится вместе с ней.',
      'Норма — медиана 1 прошлого вторника до того же часа.',
      'Норма — медиана 8 прошлых суббот до того же часа. Ожидаем к концу февраля ≈ 12 346 zł (от 10 000 до 15 001) — уже есть 1 000 zł, '
        + 'сегодня последний день месяца.',
      'Норма — медиана 3 прошлых воскресений до того же часа. Ожидаем к концу февраля ≈ 12 346 zł (от 10 000 до 15 001) — уже есть 1 000 zł, '
        + 'впереди 21 день.']), JSON.stringify(cases.notes));

  // плитка → отчёт за сегодня
  const opened = async (k, label) => { await boss.click(`#pulseKpis .kpi[data-k="${k}"]`); await z.sleep(300);
    await boss.waitForFunction(() => !document.querySelector('#repBody .skel'), null, { timeout: 20000 }).catch(() => {});
    return boss.evaluate(l => { const kp = [...document.querySelectorAll('#repBody .kpi')].find(x => x.firstChild.textContent.trim() === l);
      return { tab: repTab, preset: repPeriod.preset, on: [...document.querySelectorAll('#repPresets button.on')].map(b => b.dataset.p),
        t: [...document.querySelectorAll('#repTabs button.on')].map(b => b.dataset.t), v: kp ? kp.querySelector('b').textContent : null }; }, label); };
  const ol = await opened('load', 'Сессий'), om = await opened('money', 'Выручка');
  z.check('«Сессии» → «Загрузка» за сегодня, «Выручка» → «Деньги» за сегодня (кнопки «Сегодня» и вкладки подсвечены)',
    ol.tab === 'load' && ol.preset === 'today' && ol.on.join() === 'today' && ol.t.join() === 'load'
    && om.tab === 'money' && om.preset === 'today' && om.on.join() === 'today' && om.t.join() === 'money', JSON.stringify({ ol, om }));
  const num = x => +String(x || '').replace(/[^\d,-]/g, '').replace(',', '.');
  z.check(`за сегодня пульс и отчёт — одно число: выручка ${view.tiles[0].b} = «Деньги» ${om.v}, сессии ${view.tiles[1].b} = «Загрузка» ${ol.v}`,
    om.v !== null && num(om.v) === num(view.tiles[0].b) && ol.v !== null && num(ol.v) === num(view.tiles[1].b), JSON.stringify({ ol, om }));
  await z.shot(boss, 'z21_pulse_desktop');

  // раз в 5 минут — сам (счётчик секундного таймера подвинут к 300)
  // З19-и14: секундный таймер админки стоит, пока последний обмен не удался («нет связи» ставит и один 429 лимитера стенда
  // в залпе входа — «Найдено по дороге», З19-и14): сначала — связь есть, потом счётчик к 300
  await boss.waitForFunction(() => lastSync > 0, null, { timeout: 40000 }).catch(() => {});
  const before = calls;
  await boss.evaluate(() => { pulseTick = 297; });
  await z.sleep(4500);
  z.check('раз в 5 минут, пока открыты «Отчёты», пульс перезапрашивается', calls > before, `запросов: ${before} → ${calls}`);

  // телефон
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'stats');
  await ph.waitForFunction(() => pulseData && !document.querySelector('#pulseKpis .skel'), null, { timeout: 20000 }).catch(() => {});
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth,
    r: [...document.querySelectorAll('#pulseKpis .kpi, #pulseFc')].map(e => Math.round(e.getBoundingClientRect().right)) }));
  z.check('телефон 360: плитки и строка прогноза в экране, страница не шире экрана', fit.r.length === 4 && fit.r.every(x => x <= 360)
    && fit.sw <= 360, JSON.stringify(fit));
  await z.shot(ph, 'z21_pulse_phone');
  await z.done();
})().catch(z.crash);
