// З9-и3а «Расход из кассы и ящик», браузер. Владелец вносит расход «Из кассы»: в шторке — подсказка, что будет с
// ящиком (у «Банка» её нет); тост — «должно быть в ящике» меньше на N zł. Админ на смене видит в «Моей смене» —
// «минус расходы из кассы», «должно быть» меньше. В «Кассе» у сегодняшнего дня — колонка «Расходы», в раскрытии —
// «Расходы из кассы». Расход за вчера «Из кассы» — тост «объясняет сдачу …», у той сдачи в раскрытии её дня —
// «объяснено расходом, внесённым позже», у самого расхода — «внесён <когда> — объясняет сдачу» (сдача-начальный
// остаток — «уже в этом остатке»).
// Временный z9tp на смене; расходы с заметкой «z9tp проверка» в конце сторнируются, z9tp уволен.
const z = require('./_z');
(async () => {
  const MARK = 'z9tp проверка';
  const stornoMine = async () => {
    for (const id of z.sql(`SELECT string_agg(id::text, ' ') FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`).split(' ').filter(Boolean))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
  };
  await stornoMine(); // хвост упавшего прогона
  z.atEnd(stornoMine);
  await z.hireTemp('z9tp');
  const pv = async () => (await z.api('GET', '/admin/work/closeout', undefined, 'z9tp')).data;
  const book = async (q = 'preset=d7') => (await z.api('GET', '/admin/cash/book?' + q, undefined, 'boss')).data;
  const adm = await z.openAdmin('z9tp'); // вход в админку открывает смену
  if ((await pv()).baseline) { // ящика на стенде ещё нет — заводит его z9tp сдачей на 500 zł
    await z.api('POST', '/admin/work/stop', { counted_pln: 500 }, 'z9tp');
    await z.api('POST', '/admin/work/start', {}, 'z9tp');
  }
  const p0 = await pv();

  const boss = await z.openAdmin('boss');
  const F = v => boss.evaluate(x => fmtPLN(x).replace(/\s+/g, ' '), v); // деньги так, как их пишет страница
  const after = (act, want) => z.toastAfter(boss, act, want); // З19-и13: тост этого действия, а не #toast через паузу
  const newExpense = async (method, cat, amount, note, date) => {
    await boss.click('#expNewBtn'); await z.sleep(500);
    const h0 = await boss.isVisible('#efTillHint');
    if (date) await boss.fill('#efDate', date);
    await boss.click(`#efMethod button:has-text("${method}")`); await z.sleep(200);
    const h1 = await boss.isVisible('#efTillHint');
    await boss.selectOption('#efCat', { label: cat });
    await boss.fill('#efAmount', String(amount));
    await boss.fill('#efNote', MARK + ': ' + note);
    return { h0, h1 };
  };
  await boss.evaluate(() => { try{ localStorage.removeItem('1448a_acctab'); }catch(e){} });
  await z.nav(boss, 'acc');
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);

  const h = await newExpense('Из кассы', 'Прочее', 7, 'вода для зала');
  await z.shot(boss, 'z9_till_form');
  const t1 = await after(() => boss.click('#efSave'), 'Расход 7,00 zł внесён');
  const p1 = await pv();
  z.check('шторка: у «Банка» подсказки нет, у «Из кассы» — есть; тост «✓ Расход 7,00 zł внесён — Прочее · «должно быть в ящике» меньше на 7,00 zł»',
    !h.h0 && h.h1 && t1.includes('Расход 7,00 zł внесён — Прочее · «должно быть в ящике» меньше на 7,00 zł'), JSON.stringify({ h, t1 }));

  await adm.evaluate(() => loadMyShift()); await adm.evaluate(() => openMyShift()); await z.sleep(1500);
  const mine = (await z.text(adm, '#drawerBody')) || '', coExp = await z.text(adm, '#coExpected');
  z.check(`админ, «Моя смена»: «должно быть» на 7 меньше — ${await F(p1.expected_pln)} zł, в «Из чего» — «минус расходы из кассы ${await F(p1.expenses_pln)}»`,
    Math.abs(p1.expected_pln - (p0.expected_pln - 7)) < 0.005 && coExp === `${await F(p1.expected_pln)} zł` && mine.includes(`минус расходы из кассы ${await F(p1.expenses_pln)}`),
    JSON.stringify({ coExp, mine: mine.slice(0, 420) }));
  await z.shot(adm, 'z9_till_myshift');

  await boss.click('#accTabs button[data-t="cash"]'); await z.sleep(1800);
  const d7 = await book(), today = d7.days[d7.days.length - 1];
  const tr = await boss.evaluate(() => ({ head: [...document.querySelectorAll('#accBody table.accBook th')].map(t => t.textContent),
    cells: [...(document.querySelector('#accBody tr.accDay') || { cells: [] }).cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) }));
  await boss.evaluate(() => { accDay = null; });
  await boss.click(`#accBody tr.accDay[data-date="${z.clubDay(0)}"]`); await z.sleep(400);
  const det = (await z.text(boss, '#accBody tr.accDet')) || '';
  z.check(`«Касса»: колонка «Расходы» после «Инкассаций», у сегодня — −${await F(today.expenses_pln)}; в раскрытии — «Расходы из кассы: … Прочее — 7,00 zł»`,
    tr.head[5] === 'Расходы' && tr.head[4] === 'Инкассации' && tr.cells[5] === '−' + await F(today.expenses_pln)
    && /Расходы из кассы/.test(det) && det.includes(`Прочее — 7,00 zł · ${MARK}: вода для зала`), JSON.stringify({ tr, det: det.slice(-300) }));
  await z.shot(boss, 'z9_till_book');

  // задним числом: по правилу — последняя сдача вчерашних клубных суток, нет её — первая после них; сдали без пересчёта —
  // первый пересчёт после неё. Сдача-начальный остаток ничего не объясняет: расход «уже в начальном остатке»
  const rh = String(+z.sql(`SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')`)).padStart(2, '0');
  const at = d => `(('${d}'::date + time '${rh}:00') AT TIME ZONE 'Europe/Warsaw')`;
  const pts = z.rows(`SELECT id, counted_grosz IS NOT NULL, created_at < ${at(z.clubDay(0))} FROM shift_closeouts
    WHERE opening_grosz IS NOT NULL AND created_at >= ${at(z.clubDay(-1))} ORDER BY created_at`).map(r => r.split('|'));
  const inDay = pts.map((p, i) => p[2] === 't' ? i : -1).filter(i => i >= 0);
  const pick = inDay.length ? inDay[inDay.length - 1] : (pts.length ? 0 : -1);
  const want = pick < 0 ? '' : ((pts.slice(pick).find(p => p[1] === 't') || [])[0] || '');
  const coDay = d7.days.find(d => (d.closeouts || []).some(c => c.id === want)) || {};
  const co = (coDay.closeouts || []).find(c => c.id === want) || {};
  const coTxt = co.created_at ? await boss.evaluate(c => ruDate(c.created_at.slice(0, 10)) + ' ' + hhmm(c.created_at) + ' (' + c.nickname + ')', co) : '?';
  const says = co.baseline ? ['уже учтён в начальном остатке ящика — ', 'расход из кассы, внесённый позже, уже в этом остатке: 3,00 zł', '— уже в начальном остатке ']
    : ['объясняет сдачу ', 'объяснено расходом, внесённым позже: 3,00 zł', '— объясняет сдачу '];
  await boss.click('#accTabs button[data-t="exp"]'); await z.sleep(1500);
  const yday = await boss.evaluate(d => ruDate(d), z.clubDay(-1));
  await newExpense('Из кассы', 'Аренда', 3, 'задним числом', yday);
  const t2 = await after(() => boss.click('#efSave'), 'Расход 3,00 zł внесён');
  const p2 = await pv();
  z.check(`расход 3 zł за вчера «Из кассы»: тост «… · ${says[0]}${coTxt}», «должно быть в ящике» не изменилось`,
    !!want && t2.includes('Расход 3,00 zł внесён — Аренда · ' + says[0] + coTxt) && Math.abs(p2.expected_pln - p1.expected_pln) < 0.005,
    JSON.stringify({ t2, want, p1: p1.expected_pln, p2: p2.expected_pln }));

  await boss.click('#accTabs button[data-t="cash"]'); await z.sleep(1800);
  await boss.evaluate(() => { accDay = null; paintAcc(); });
  await boss.click(`#accBody tr.accDay[data-date="${coDay.date}"]`); await z.sleep(400);
  const det2 = (await z.text(boss, '#accBody tr.accDet')) || '';
  z.check(`у той сдачи в раскрытии дня — «${says[1]}», у расхода — «внесён <сегодня> … ${says[2]}…»`,
    det2.includes(says[1]) && new RegExp(`Аренда — 3,00 zł · ${MARK}: задним числом · внесён \\d\\d\\.\\d\\d\\.\\d{4} \\d\\d:\\d\\d`).test(det2)
    && det2.includes(says[2] + coTxt.split(' ').slice(1).join(' ')), det2.slice(-700));
  const focus = re => boss.evaluate(r => { const d = [...document.querySelectorAll('#accBody tr.accDet div')].find(x => new RegExp(r).test(x.textContent));
    if (d) d.scrollIntoView({ block: 'center' }); }, re);
  // обычная сдача с недостачей (на стенде вчерашняя может оказаться начальным остатком) — подписи на своих данных
  const syn = await boss.evaluate(() => {
    const r = { closeouts: [{ id: 'c1', created_at: '2026-09-26T21:40:00+02:00', nickname: 'admin2', expected_pln: 100, counted_pln: 80, diff_pln: -20, explained_pln: 20 }],
      events: [], expenses: [{ id: 'e1', category: 'Прочее', amount_pln: 20, note: 'моющее', created_at: '2026-09-27T12:05:00+02:00',
        drawer_at: '2026-09-26T21:39:59+02:00', op_date: '2026-09-26', explains_closeout: 'c1' }] };
    const d = document.createElement('div'); d.innerHTML = accDetails(r);
    return { det: d.textContent.replace(/\s+/g, ' '),
      note: expTillNote({ amount_pln: 20, drawer_at: 'x', till: { live: false, closeout_at: '2026-09-26T21:40:00+02:00', closeout_by: 'admin2' } }, null),
      same: expTillNote({ amount_pln: 20, drawer_at: 'x', till: { live: true } }, 'x'), off: expTillNote({ amount_pln: 20 }, 'x') }; });
  z.check('обычная сдача с недостачей: «расхождение −20,00 · объяснено расходом, внесённым позже: 20,00 zł», у расхода — «— объясняет сдачу ЧЧ:ММ (admin2)»; '
    + 'тост — «· объясняет сдачу 26.09.2026 ЧЧ:ММ (admin2)»; место в ящике не менялось — без приписки; ушёл из кассы — «ящик больше не трогает»',
    syn.det.includes('расхождение −20,00 · объяснено расходом, внесённым позже: 20,00 zł') && /Прочее — 20,00 zł · моющее · внесён 27\.09\.2026 \d\d:\d\d — объясняет сдачу \d\d:\d\d \(admin2\)/.test(syn.det)
    && /^ · объясняет сдачу 26\.09\.2026 \d\d:\d\d \(admin2\)$/.test(syn.note) && syn.same === '' && syn.off === ' · ящик больше не трогает', JSON.stringify(syn));
  await focus('внесённым позже'); await z.shot(boss, 'z9_till_book_day'); // сдача, объяснённая расходом
  await focus(MARK + ': задним числом'); await z.shot(boss, 'z9_till_book_exp'); // сам расход в «Расходах из кассы»
  await z.done();
})().catch(z.crash);
