// З17-и3 «Тепловая карта» (№13 «б», З-Р6), браузер: «Отчёты → Загрузка» — карта «час × день недели» по heat сервера
// (З17-и2). Неделя клубных суток в прошлом без сессий; в неё — сессии Guest47 SQL-ом (без денег, ended_reason z17t): в
// пятницу 18:00–23:00 на PC-07, 20:00–21:00 на PC-09 и ночь 23:00–02:00 на VIP-01, в понедельник 10:00–12:00 на PC-08 с
// часом паузы; на время — шаблон смены «z17 Рассвет» 06:00–10:00 в один день недели (SQL; не сегодня и не вчера). Карта:
// 7 строк по 24 часа от границы суток, ночь — в строке пятницы; клетки — занятость сервера, ярче всего — самый плотный
// час, его число — в легенде; скобки смен — День и Ночь в каждой строке, «z17 Рассвет» — куском в конце строки накануне и
// в начале своей; подсказка — занятость, часы, сессии и смена, с клавиатуры — тоже; «Числа таблицей» — занятость
// матрицей; у «по часам суток» в подсказке — занятость часа. За одни сутки — у остальных дней «в периоде нет …».
// Телефон 360 — без горизонтальной прокрутки. В конце сессии и шаблон удаляются (и при падении тоже).
const z = require('./_z');
const DAYS = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'];
const pct = v => v == null ? '—' : String(v).replace('.', ',') + '%';
(async () => {
  const G = 'Guest47', g = z.uid(G), TPL = 'z17 Рассвет';
  const drop = () => z.sql(`DELETE FROM sessions WHERE ended_reason = 'z17t' AND user_id = '${g}'`);
  const dropTpl = () => z.sql(`DELETE FROM shifts WHERE name = '${TPL}'`);
  const wake = () => z.sql("UPDATE shifts SET active = true WHERE name IN ('День', 'Ночь') AND NOT active RETURNING id");
  drop(); dropTpl(); wake(); // и после оборванного прогона: шаблоны стенда — снова включены
  z.atEnd(drop); z.atEnd(dropTpl); z.atEnd(wake);
  // З18-и4б: стенд — с полуночи, а карта проверяется с ночью в строке пятницы — на время свой час границы 8 (вернётся как был)
  const rh0 = z.rows("SELECT value FROM settings WHERE key = 'report_hour'")[0];
  z.atEnd(() => { z.sql("DELETE FROM settings WHERE key = 'report_hour'");
    if (rh0 !== undefined) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${rh0}', now())`); });
  z.sql("INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '8', now()) ON CONFLICT (key) DO UPDATE SET value = '8'");
  const rh = 8;
  // понедельник недели клубных суток без единой начатой сессии
  const MON = z.sql(`SELECT g::date::text FROM generate_series(((now() AT TIME ZONE 'Europe/Warsaw')::date - 400)::timestamp,
    ((now() AT TIME ZONE 'Europe/Warsaw')::date - 10)::timestamp, interval '1 day') AS g WHERE extract(isodow FROM g) = 1
    AND NOT EXISTS (SELECT 1 FROM sessions WHERE started_at >= ((g + interval '${rh} hours') AT TIME ZONE 'Europe/Warsaw')
      AND started_at < ((g + interval '7 days ${rh} hours') AT TIME ZONE 'Europe/Warsaw')) ORDER BY g DESC LIMIT 1`);
  const day = n => z.sql(`SELECT ('${MON}'::date + ${n})::text`);
  const FRI = day(4), SAT = day(5), SUN = day(6);
  const at = (d, hm) => `(('${d}'::date + time '${hm}') AT TIME ZONE 'Europe/Warsaw')`;
  const ins = (pc, d, frm, d2, to, mins) => z.sql(`INSERT INTO sessions (user_id, computer_id, club_id, status, started_at,
    ready_at, ended_at, minutes_total, minutes_used, billed_minutes, base_rate_pln, effective_rate_pln, ended_reason, zone_id,
    zone_name, xp_per_min, coins_per_min) SELECT '${g}', c.id, c.club_id, 'completed', ${at(d, frm)}, ${at(d, frm)},
    ${at(d2, to)}, ${mins}, ${mins}, ${mins}, 0, 0, 'z17t', c.zone_id, COALESCE(c.zone, ''), 10, 2 FROM computers c
    WHERE c.name = '${pc}' RETURNING id`);
  ins('PC-07', FRI, '18:00', FRI, '23:00', 300);
  ins('PC-09', FRI, '20:00', FRI, '21:00', 60);
  ins('VIP-01', FRI, '23:00', SAT, '02:00', 180);
  ins('PC-08', MON, '10:00', MON, '12:00', 60); // час паузы: отрезок — 2 часа, игры — 60 минут
  // временный шаблон: день X — не сегодня и не вчера (по календарю), чтобы «сейчас на смене» его не видело
  const today = (new Date().getDay() + 6) % 7, X = (today + 3) % 7, XP = (X + 6) % 7;
  const tpl = z.sql(`INSERT INTO shifts (club_id, name, start_min, end_min, days_mask, sort, active) SELECT club_id, '${TPL}',
    360, 600, ${1 << X}, 99, true FROM shifts ORDER BY sort LIMIT 1 RETURNING id`);
  const shifts = ((await z.api('GET', '/admin/shifts', undefined, 'boss')).data.shifts || []).filter(s => s.active);
  const std = JSON.stringify(shifts.filter(s => s.name !== TPL).map(s => [s.name, s.start_min, s.end_min, s.days_mask]));
  const load = (await z.api('GET', `/admin/reports/load?from=${MON}&to=${SUN}`, undefined, 'boss')).data;
  const heat = load.heat || [], hc = (w, h) => (heat[w] || [])[h] || {};
  const occ = [].concat(...heat.map(r => r.map(c => c.occupancy_pct))).filter(v => v != null);
  const max = Math.max(0, ...occ), inService = (load.capacity || {}).computers;
  z.check(`неделя ${MON} — ${SUN}: 4 сессии SQL-ом, шаблон «${TPL}» на ${DAYS[X]}; на стенде День 8:00–20:00 и Ночь 20:00–8:00 `
    + `ежедневно, граница суток — ${rh}:00; пик — пт 20:00, 2 ч на ${inService} ПК = ${pct(max)}`,
    !!MON && !!tpl && rh === 8 && std === JSON.stringify([['День', 480, 1200, 127], ['Ночь', 1200, 480, 127]])
    && hc(4, 20).hours === 2 && hc(4, 20).occupancy_pct === max, `${MON} · ${std} · пт 20: ${JSON.stringify(hc(4, 20))}`);

  const page = await z.openAdmin('boss');
  await z.nav(page, 'stats');
  await page.click('#repTabs button[data-t="load"]');
  await z.sleep(800);
  const period = async (from, to) => {
    await page.evaluate(([f, t]) => { repPeriod = { from: f, to: t }; renderReports(); }, [from, to]);
    await page.waitForFunction(() => /Компьютеры/i.test((document.getElementById('repBody') || {}).textContent || ''), null,
      { timeout: 10000 }).catch(() => {});
    await z.sleep(800);
  };
  await period(MON, SUN);
  const map = () => page.evaluate(() => {
    const f = document.querySelector('#repBody .chart.heat');
    if (!f) return null;
    const clean = s => s.replace(/\s+/g, ' ').trim();
    return {
      rows: [...f.querySelectorAll('.hmrow')].map(r => {
        const lane = r.querySelector('.hmlane'), lr = lane ? lane.getBoundingClientRect() : null;
        return { day: clean(r.querySelector('.hmday').textContent), none: clean((r.querySelector('.hmnone') || {}).textContent || ''),
          lane: lr ? Math.round(lr.height) : 0,
          cells: [...r.querySelectorAll('.bars .bar')].map(b => ({ v: b.dataset.v, k: (b.querySelector('i') || {}).className || '',
            off: b.classList.contains('off'), tip: b.dataset.tip || '' })),
          sh: [...r.querySelectorAll('.hmsh')].map(s => { const sr = s.getBoundingClientRect();
            return { name: clean(s.textContent), cl: s.classList.contains('cl'), cr: s.classList.contains('cr'),
              left: +((sr.left - lr.left) / lr.width * 100).toFixed(1), width: +(sr.width / lr.width * 100).toFixed(1),
              top: Math.round(sr.top - lr.top) }; }) };
      }),
      axis: [...f.querySelectorAll('.hmx span')].map(s => clean(s.textContent)).filter(Boolean),
      leg: clean(f.querySelector('.hmleg').textContent),
      table: [...f.querySelectorAll('details.chtable tr')].map(tr => [...tr.children].map(td => clean([...td.childNodes]
        .filter(n => !(n.classList && n.classList.contains('repL'))).map(n => n.textContent).join('')))), // без подписи часа телефона
      sect: [...document.querySelectorAll('#repBody .sect')].map(s => clean(s.textContent)).find(s => /^Карта загрузки/i.test(s)) || '',
      hourTips: [...document.querySelectorAll('#repBody .chart:not(.heat)')].map(c => [...c.querySelectorAll('.bars .bar')]
        .map(b => b.dataset.tip || ''))[1] || [] };
  });
  const m = await map();
  const R = (w) => (m && m.rows[w]) || { cells: [], sh: [] };
  const col = h => (h - rh + 24) % 24; // столбец часа h в строке
  z.check('карта — 7 строк пн…вс по 24 часа, ось — от границы суток через 3 часа: 8:00, 11:00 … 5:00; под заголовком — '
    + 'что строка, цвет и скобки', m && JSON.stringify(m.rows.map(r => r.day)) === JSON.stringify(DAYS)
    && m.rows.every(r => r.cells.length === 24)
    && JSON.stringify(m.axis) === JSON.stringify(['8:00', '11:00', '14:00', '17:00', '20:00', '23:00', '2:00', '5:00'])
    && /клубные сутки с 8:00: ночь после пятницы — в пятнице; цвет — занятость часа, ярче — плотнее; скобки — смены по нынешним шаблонам/.test(m.sect),
    m ? `${JSON.stringify(m.axis)} · ${m.sect}` : 'карты нет');
  const same = m && m.rows.every((r, w) => r.cells.every((c, i) => c.v === String(hc(w, (rh + i) % 24).occupancy_pct ?? '')));
  z.check('клетка = занятость сервера в этом часе (все 168)', same, m ? JSON.stringify(R(4).cells.map(c => c.v)) : '');
  const v = (w, h) => R(w).cells[col(h)] || {};
  const lit = h => +(v(4, h).v || 0) > 0;
  z.check('пятница 18:00–23:00 и ночь 23:00–02:00 — в строке пятницы (часы 18 … 23, 0 и 1); в субботе ночью пусто',
    m && [18, 19, 20, 21, 22, 23, 0, 1].every(lit) && !lit(17) && !lit(2) && v(5, 0).v === '0' && v(5, 1).v === '0',
    m ? JSON.stringify([17, 18, 23, 0, 1, 2].map(h => v(4, h).v)) + ' · сб 0–1: ' + v(5, 0).v + ' ' + v(5, 1).v : '');
  const h5 = m ? [].concat(...m.rows.map((r, w) => r.cells.map((c, i) => c.k === 'h5' ? DAYS[w] + ' ' + (rh + i) % 24 : ''))).filter(Boolean) : [];
  z.check(`ярче всего — только пт 20:00 (${pct(max)}), в легенде — «0% … ${pct(max)}»; час игры — третья ступень, полчаса `
    + '(пн 10:00 и 11:00) — вторая, ноль — без ступени', m && JSON.stringify(h5) === JSON.stringify(['пт 20'])
    && m.leg.startsWith('0%') && m.leg.includes(pct(max)) && v(4, 18).k === 'h3' && v(0, 10).k === 'h2' && v(0, 11).k === 'h2'
    && v(0, 9).k === '' && v(0, 12).k === '', m ? `${JSON.stringify(h5)} · ${m.leg} · ${[v(4, 18).k, v(0, 10).k, v(0, 9).k]}` : '');
  const near = (a, b) => Math.abs(a - b) <= 0.5;
  const pair = w => { const s = R(w).sh, d = s.find(x => x.name === 'День'), n = s.find(x => x.name === 'Ночь');
    return d && n && near(d.left, 0) && near(d.width, 50) && near(n.left, 50) && near(n.width, 50); };
  const own = (w, name) => R(w).sh.filter(x => x.name === name);
  const tail = own(XP, TPL), head = own(X, TPL);
  z.check(`скобки смен: в каждой строке — День (левая половина) и Ночь (правая); «${TPL}» 6:00–10:00 — в ${DAYS[XP]} в конце `
    + `(6:00–8:00, без правого бортика) и в ${DAYS[X]} в начале (8:00–10:00, без левого), в других строках её нет`,
    m && m.rows.every((r, w) => pair(w)) && tail.length === 1 && near(tail[0].left, 91.7) && near(tail[0].width, 8.3) && tail[0].cr
    && !tail[0].cl && head.length === 1 && near(head[0].left, 0) && near(head[0].width, 8.3) && head[0].cl && !head[0].cr
    && m.rows.every((r, w) => w === X || w === XP || !own(w, TPL).length), m ? JSON.stringify([R(XP).sh, R(X).sh]) : '');
  const top = (w, name) => (R(w).sh.find(x => x.name === name) || {}).top;
  z.check(`пересекается с Днём и Ночью — «${TPL}» вторым ярусом, ниже их; ярусов в строках поровну (два)`,
    m && top(X, TPL) > top(X, 'День') && top(XP, TPL) > top(XP, 'Ночь') && top(X, 'День') === top(0, 'День')
    && new Set(m.rows.map(r => r.lane)).size === 1 && m.rows[0].lane === 26,
    m ? JSON.stringify({ lanes: m.rows.map(r => r.lane), x: R(X).sh.map(x => [x.name, x.top]), xp: R(XP).sh.map(x => [x.name, x.top]) }) : '');

  const tip = () => page.evaluate(() => { const t = document.getElementById('chtip');
    return t && !t.classList.contains('hidden') ? { v: (t.querySelector('b') || {}).textContent, text: t.textContent } : null; });
  const cellSel = (w, h) => `#repBody .chart.heat .hmrow:nth-child(${w + 1}) .bars .bar:nth-child(${col(h) + 1})`;
  let t1 = null, t2 = null, t3 = null, k1 = null, k2 = null;
  if (m) {
    await page.locator('#repBody .chart.heat').scrollIntoViewIfNeeded(); await z.sleep(300);
    await page.hover(cellSel(4, 20)); await z.sleep(250); t1 = await tip();
    await page.hover(cellSel(4, 0)); await z.sleep(250); t2 = await tip();
    await page.hover(cellSel(X, 9)); await z.sleep(250); t3 = await tip();
    await page.hover(cellSel(4, 20)); await z.sleep(250);
    await z.shot(page, 'z17_heat');
    await page.focus('#repBody .chart.heat .hmrow:nth-child(5) .bars');
    await page.keyboard.press('Home'); await z.sleep(200); k1 = await tip();
    await page.keyboard.press('ArrowRight'); await z.sleep(200); k2 = await tip();
  }
  z.check(`подсказка пт 20:00: «${pct(max)}» крупно, «пт · 20:00–21:00 · наиграно 2 ч · 2 сессии · смена Ночь»`,
    t1 && t1.v === pct(max) && t1.text.includes('пт · 20:00–21:00') && t1.text.includes('наиграно 2 ч · 2 сессии · смена Ночь'),
    JSON.stringify(t1));
  z.check('подсказка ночного часа: «ночь с пт на сб · 0:00–1:00 · наиграно 1 ч · 1 сессия · смена Ночь»; в 9:00 дня '
    + `«${TPL}» — «смены День и ${TPL}»`, t2 && t2.v === pct(hc(4, 0).occupancy_pct) && t2.text.includes('ночь с пт на сб · 0:00–1:00')
    && t2.text.includes('наиграно 1 ч · 1 сессия · смена Ночь') && t3 && t3.text.includes(`смены День и ${TPL}`),
    JSON.stringify([t2, t3 && t3.text]));
  z.check('с клавиатуры: фокус на строке пятницы, Home — 8:00–9:00, → — 9:00–10:00',
    k1 && k1.text.includes('пт · 8:00–9:00') && k2 && k2.text.includes('пт · 9:00–10:00'), JSON.stringify([k1, k2]));
  const tb = m ? m.table : [];
  z.check('«Числа таблицей» — занятость матрицей: шапка «День · 8:00 … 7:00», строка на день, пт 20:00 — как на карте',
    tb.length === 8 && tb[0].length === 25 && tb[0][0] === 'День' && tb[0][1] === '8:00' && tb[0][24] === '7:00' && tb[5][0] === 'пт'
    && tb[5][1 + col(20)] === pct(max) && tb[1][1 + col(9)] === '0%', JSON.stringify(tb[5] || null));
  const ht = (m && m.hourTips[20]) || '';
  z.check(`«Загрузка по часам суток»: в подсказке часа — занятость (20:00 — ${pct((load.hours || [])[20] && load.hours[20].occupancy_pct)})`,
    m && m.hourTips.length === 24 && ht.endsWith('2 сессии · занятость ' + pct(load.hours[20].occupancy_pct)), ht);

  await period(FRI, FRI);
  const one = await map();
  const absent = one ? one.rows.filter((r, w) => w !== 4).map(r => r.none) : [];
  z.check('за одни сутки (пятница): у остальных дней — «в периоде нет понедельников … воскресений», в пятнице — 24 часа',
    one && JSON.stringify(absent) === JSON.stringify(['в периоде нет понедельников', 'в периоде нет вторников',
      'в периоде нет сред', 'в периоде нет четвергов', 'в периоде нет суббот', 'в периоде нет воскресений'])
    && one.rows[4].cells.length === 24 && one.rows[4].cells.filter(c => c.k === 'h5').length === 1,
    one ? JSON.stringify(absent) : 'карты нет');

  // закрыто по сменам: на время выключить шаблон, который сейчас не идёт (SQL), — его часы на карте «закрыты»
  dropTpl();
  const nowMin = +z.sql("SELECT (extract(hour FROM now() AT TIME ZONE 'Europe/Warsaw') * 60 + extract(minute FROM now() AT TIME ZONE 'Europe/Warsaw'))::int");
  const OFF = nowMin >= 480 && nowMin < 1200 ? 'Ночь' : 'День', ON = OFF === 'Ночь' ? 'День' : 'Ночь';
  z.sql(`UPDATE shifts SET active = false WHERE name = '${OFF}' RETURNING id`);
  const fri = ((await z.api('GET', `/admin/reports/load?from=${FRI}&to=${FRI}`, undefined, 'boss')).data.heat || [])[4] || [];
  await period(FRI, FRI);
  const cl = await map();
  if (cl) { await page.locator('#repBody .chart.heat').scrollIntoViewIfNeeded(); await z.sleep(300); await z.shot(page, 'z17_heat_closed'); }
  wake();
  const closedH = OFF === 'Ночь' ? [20, 21, 22, 23, 0, 1, 2, 3, 4, 5, 6, 7] : [8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19];
  const playH = OFF === 'Ночь' ? [20, 21, 22, 23, 0, 1] : [18, 19];
  const fc = h => ((cl && cl.rows[4]) || { cells: [] }).cells[col(h)] || {};
  const offs = cl ? cl.rows[4].cells.map((c, i) => c.off ? (rh + i) % 24 : -1).filter(h => h >= 0) : [];
  const plays = cl ? [...Array(24).keys()].filter(h => fc(h).off && fc(h).tip.includes('наиграно')) : [];
  z.check(`«${OFF}» выключен на время: его часы в пятнице — «закрыто» пунктиром (у сервера занятость null), там, где играли, `
    + `— точка (${playH.map(h => h + ':00').join(', ')}); в легенде — оба знака; скобок «${OFF}» нет, «${ON}» — на месте`,
    cl && JSON.stringify(offs.sort((a, b) => a - b)) === JSON.stringify([...closedH].sort((a, b) => a - b))
    && closedH.every(h => fri[h] && fri[h].occupancy_pct === null)
    && JSON.stringify(plays.sort((a, b) => a - b)) === JSON.stringify([...playH].sort((a, b) => a - b))
    && cl.leg.includes('клуб закрыт по сменам') && cl.leg.includes('играли и в закрытый час')
    && cl.rows.every(r => !r.sh.some(x => x.name === OFF) && r.sh.some(x => x.name === ON)),
    cl ? `закрыто: ${JSON.stringify(offs)} · с игрой: ${JSON.stringify(plays)} · ${cl.leg}` : 'карты нет');
  const tipH = OFF === 'Ночь' ? 23 : 18, pt = fc(tipH).tip || ''; // в этом часе — одна сессия
  z.check(`подсказка закрытого часа с игрой (пт ${tipH}:00): «клуб закрыт по сменам · наиграно 1 ч · 1 сессия · смены нет»`,
    pt.includes(`пт · ${tipH}:00–${(tipH + 1) % 24}:00 · клуб закрыт по сменам · наиграно 1 ч · 1 сессия · смены нет`), pt);

  await z.sleep(2500); // лимитер входа с одного адреса
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 800 } });
  await z.nav(ph, 'stats');
  await ph.evaluate(() => { const b = document.querySelector('#repTabs button[data-t="load"]'); if (b) b.click(); });
  await z.sleep(800);
  await ph.evaluate(([f, t]) => { repPeriod = { from: f, to: t }; renderReports(); }, [MON, SUN]);
  await z.sleep(2500);
  const fit = await ph.evaluate(() => {
    const f = document.querySelector('#repBody .chart.heat');
    if (!f) return null;
    f.querySelector('.hmplot').scrollIntoView({ block: 'center' });
    const r = f.getBoundingClientRect(), c = f.querySelector('.bars .bar').getBoundingClientRect();
    return { over: document.documentElement.scrollWidth - document.documentElement.clientWidth, right: Math.round(r.right),
      vw: innerWidth, cell: +c.width.toFixed(1), rows: f.querySelectorAll('.hmrow').length };
  });
  await z.sleep(300);
  await z.shot(ph, 'z17_heat_phone');
  z.check('телефон 360: карта во всю ширину без горизонтальной прокрутки, клетка часа — не уже 9 px',
    fit && fit.over <= 0 && fit.right <= fit.vw && fit.cell >= 9 && fit.rows === 7, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
