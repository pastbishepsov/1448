// З12-и4 «Экран «Прибыль»», браузер (№74; решение основателя 04.10 — «общий дайджест, когда входишь в бухгалтерию, где
// будет график общих данных, а потом уже возможность нажать на каждую вкладку»). Владелец: вход в «Бухгалтерию» — всегда
// «Обзор» (первая вкладка), палитра и ссылки ведут сразу в нужную вкладку. «Сейчас» — плитки «В ящике сейчас»,
// «Ждут внесения», «Поставки ждут цен», «Обязательства клуба» — числа как у сервера, плитка ведёт во вкладку.
// З12-и4а (решение основателя 04.10 — «давай лучше сделаем по месяцам, но всегда можно отфильтровать от даты до даты»):
// период — календарный месяц, по умолчанию этот; ‹ › листают месяцы, дальше текущего — нет; свой период — с какой даты по
// какую. «Доходы», «Затраты», «Результат» с ростом — как сервер; прошлый месяц: график «Доходы и затраты по дням» — две
// серии с легендой, столбики = ряд сервера, подсказка, «Числа таблицей»; «Прибыль за период» — семь строк «Строка · За
// период · Прошлый период · Изменение» как у сервера, клик — состав строки, из «Расходов» — ссылка во вкладку за тот же
// месяц. «Отработано за период» — отработанная выручка, отдано временем и состав. Свой период в 90 дней — по неделям с
// понедельника (Σ недель = итог), в один день — без графика. Палитра «прибыль» → таблица на экране. Телефон 360 — длиннее
// двух недель — недели, карточки с подписями, без горизонтальной прокрутки.
// Данные: гость Guest44 — пополнение 10 zł картой (чтобы у сегодня были доход и затраты), в конце отменяется с возвратом
// на карту: доход и возврат остаются в сегодняшней строке, как у любой отмены.
const z = require('./_z');
(async () => {
  const dep = (await z.api('POST', `/admin/users/${z.uid('Guest44')}/deposit`, { amount_pln: 10, method: 'card' }, 'boss')).data.deposit_id;
  z.atEnd(async () => { if (dep) await z.api('POST', `/admin/deposits/${dep}/void`, { refund_method: 'card' }, 'boss'); });
  if (!z.check('данные: пополнение Guest44 на 10 zł картой', !!dep, dep)) return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const num = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }));
  const fmt = v => num(v) + ' zł';
  const sgn = v => (v < 0 ? '−' : '') + num(Math.abs(v || 0)); // как ovMoney: минус длинным знаком
  const G = v => Math.round((v || 0) * 100);
  const hm = m => { m = Math.max(0, Math.round(m || 0)); const h = Math.floor(m / 60), r = m % 60; return !m ? '0 ч' : (h ? h + ' ч' : '') + (r ? (h ? ' ' : '') + r + ' мин' : ''); };
  const pct = v => v === null || v === undefined ? '—' : (v > 0 ? '+' : '') + (+v).toLocaleString('ru-RU').replace(/\s/g, ' ') + '%'; // З18-и4и: с запятой
  const diff = (a, b) => { const v = Math.round(((a || 0) - (b || 0)) * 100) / 100; return (v > 0 ? '+' : v < 0 ? '−' : '') + num(Math.abs(v)) + ' zł'; };
  const MN = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'];
  const mname = m => { const n = MN[+m.slice(5, 7) - 1]; return n[0].toUpperCase() + n.slice(1) + ' ' + m.slice(0, 4); };
  const addm = (m, k) => { const d = new Date(+m.slice(0, 4), +m.slice(5, 7) - 1 + k, 1); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0'); };
  const lastd = m => m + '-' + String(new Date(+m.slice(0, 4), +m.slice(5, 7), 0).getDate()).padStart(2, '0');
  const ru = d => d.slice(8, 10) + '.' + d.slice(5, 7) + '.' + d.slice(0, 4);
  const tap = (p, sel) => p.click(sel, { timeout: 4000 }).catch(() => {}); // нового на HEAD нет — проверка упадёт, прогон нет
  const kpis = (p, id) => p.evaluate(id => Object.fromEntries([...document.querySelectorAll('#' + id + ' .kpi')].map(k =>
    [k.firstChild.textContent.trim(), { v: k.querySelector('b') ? k.querySelector('b').innerText.replace(/\s+/g, ' ').trim() : '',
      d: k.querySelector('.dlt') ? k.querySelector('.dlt').textContent.replace(/\s+/g, ' ').trim() : '', to: (k.querySelector(':scope > i:not(.dlt)') || {}).textContent || ''}])), id);
  const profit = async p => (await z.api('GET', '/admin/reports/profit?' + await p.evaluate(() => typeof ovQuery === 'function' ? ovQuery() : 'preset=month'), undefined, 'boss')).data;
  const state = p => p.evaluate(() => ({ tab: curTab, acc: typeof accTab === 'undefined' ? null : accTab,
    on: (document.querySelector('#accTabs button.on') || {}).textContent || '' }));
  const sw = p => p.evaluate(() => { const n = document.getElementById('ovMonthName'), b = document.querySelectorAll('#ovMonth .ovArr');
    return { name: n ? n.textContent : '', on: !!n && n.classList.contains('on'), prev: b.length === 2 && !b[0].disabled, next: b.length === 2 && !b[1].disabled,
      label: (document.getElementById('ovLabel') || {}).textContent || '' }; });
  // недели с понедельника — как их складывает страница
  const weeks = days => { const out = [];
    for (const x of days){ const dow = (new Date(x.date + 'T12:00:00').getDay() + 6) % 7;
      if (!out.length || dow === 0) out.push({ from: x.date, inc: 0, out: 0 });
      const b = out[out.length - 1]; b.inc += G(x.income_pln); b.out += G(x.returns_pln) + G(x.cogs_pln) + G(x.losses_pln) + G(x.expenses_pln); }
    return out; };
  const TODAY = z.clubDay(0), CUR = TODAY.slice(0, 7), PREV = addm(CUR, -1);

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'hall');
  await boss.click('#r-acc'); await z.sleep(2500);
  const s0 = await state(boss);
  const tabs = await boss.evaluate(() => [...document.querySelectorAll('#accTabs button')].map(b => b.textContent.replace(/\d+$/, '').trim()));
  z.check('вход в «Бухгалтерию» из рейла — «Обзор»: первая вкладка «Обзор · Касса · Расходы · Закупки · Справочники · Выгрузка»',
    s0.tab === 'acc' && s0.acc === 'ov' && s0.on === 'Обзор' && tabs.join(',') === 'Обзор,Касса,Расходы,Закупки,Справочники,Выгрузка'
    && await boss.evaluate(() => !!document.getElementById('ovKpis')), JSON.stringify({ s0, tabs }));
  await z.shot(boss, 'z12_profit_top');

  const P = await profit(boss), W = P.prev || {}, K = P.delta || {};
  const ov = (await z.api('GET', '/admin/overview', undefined, 'boss')).data;
  const kp = await kpis(boss, 'ovKpis'), now = await kpis(boss, 'ovNow');
  const m0 = await sw(boss), per = P.period || {};
  z.check(`по умолчанию — этот месяц: «‹ ${mname(CUR)} ›» (дальше — нельзя), с 1-го по сегодня, сравнение — тот же отрезок прошлого месяца; `
    + '«Доходы», «Затраты», «Результат» и их рост — как у сервера, у результата рост в злотых',
    per.preset === 'month' && per.from === CUR + '-01' && per.to === TODAY && (P.prev_period || {}).from === PREV + '-01'
    && m0.name === mname(CUR) && m0.on && m0.prev && !m0.next && norm(m0.label).endsWith('клубные сутки')
    && kp['Доходы'] && kp['Доходы'].v === fmt(P.income.pln) && kp['Доходы'].d === pct(K.income)
    && kp['Затраты'] && kp['Затраты'].v === fmt(P.costs_pln) && kp['Затраты'].d === pct(K.costs)
    && kp['Результат'] && kp['Результат'].v === sgn(P.result_pln) + ' zł' && kp['Результат'].d === diff(P.result_pln, W.result_pln),
    JSON.stringify({ per, m0, kp, inc: P.income && P.income.pln, costs: P.costs_pln, res: P.result_pln, K }));
  const due = ov.expenses_due || 0, sup = ov.supplies_waiting || 0, nw = P.now || {};
  z.check(`«Сейчас»: в ящике — как кассовая книга, обязательства клуба — как сервер; «Ждут внесения» ${due ? '— ' + due : 'нет'}, `
    + `«Поставки ждут цен» ${sup ? '— ' + sup : 'нет'}; у плиток — куда ведут`,
    now['В ящике сейчас'] && now['В ящике сейчас'].v === (nw.baseline ? '—' : fmt(nw.drawer_pln)) && now['В ящике сейчас'].to === 'Касса ›'
    && now['Обязательства клуба'] && now['Обязательства клуба'].v === fmt((P.liabilities || {}).net_pln) && now['Обязательства клуба'].to === 'Деньги ›'
    && (due ? now['Ждут внесения'] && now['Ждут внесения'].v === String(due) && now['Ждут внесения'].to === 'Расходы ›' : !now['Ждут внесения'])
    && (sup ? now['Поставки ждут цен'] && now['Поставки ждут цен'].v === String(sup) && now['Поставки ждут цен'].to === 'Закупки ›' : !now['Поставки ждут цен']),
    JSON.stringify({ now, nw, due, sup }));

  // прошлый месяц: ‹
  await tap(boss, '#ovMonth .ovArr:first-child'); await z.sleep(3000);
  const PM = await profit(boss), WM = PM.prev || {}, KM = PM.delta || {}, pm = PM.period || {}, m1 = await sw(boss);
  z.check(`‹ — ${mname(PREV)}: месяц целиком (${ru(PREV + '-01')} — ${ru(lastd(PREV))}), сравнение — ${mname(addm(PREV, -1))} целиком; `
    + '› снова можно',
    pm.from === PREV + '-01' && pm.to === lastd(PREV) && (PM.prev_period || {}).from === addm(PREV, -1) + '-01'
    && (PM.prev_period || {}).to === lastd(addm(PREV, -1)) && m1.name === mname(PREV) && m1.on && m1.prev && m1.next,
    JSON.stringify({ pm, prev: PM.prev_period, m1 }));
  const ch = await boss.evaluate(() => { const f = document.getElementById('ovChart');
    return f ? { head: (document.getElementById('ovChartHead') || {}).textContent, leg: [...f.querySelectorAll('.chleg span')].map(s => ({ t: s.textContent.trim(),
      c: getComputedStyle(s.querySelector('i')).backgroundColor })), bars: [...f.querySelectorAll('.bars .bar')].map(b => ({ from: b.dataset.from, inc: +b.dataset.inc, out: +b.dataset.out,
      cols: [...b.querySelectorAll('i')].map(i => i.className + ':' + getComputedStyle(i).backgroundColor) })),
      rows: f.querySelectorAll('.chtable tr').length } : null; });
  const days = PM.days || [];
  const same = ch && ch.bars.length === days.length && ch.bars.every((b, i) => b.from === days[i].date && b.inc === G(days[i].income_pln)
    && b.out === G(days[i].costs_pln));
  z.check(`график «Доходы и затраты по дням»: легенда «Доходы» синим и «Затраты» акцентом, ${days.length} пар столбиков = ряд сервера `
    + `(доход и затраты дня), «Числа таблицей» — ${days.length} строк`,
    !!ch && days.length === pm.days && /по дням$/i.test(norm(ch.head)) && ch.leg.map(l => l.t).join(',') === 'Доходы,Затраты' && ch.leg[0].c === 'rgb(57, 135, 229)'
    && ch.leg[1].c === 'rgb(255, 39, 64)' && same && ch.bars.every(b => b.cols.join(',') === 'inc:rgb(57, 135, 229),out:rgb(255, 39, 64)')
    && ch.rows === days.length + 1, JSON.stringify({ head: ch && ch.head, leg: ch && ch.leg, n: ch && ch.bars.length, rows: ch && ch.rows, days: days.length }));
  await boss.hover('#ovChart .bars .bar:last-child', { timeout: 4000 }).catch(() => {}); await z.sleep(400);
  const tip = norm(await z.text(boss, '#chtip')), last = days[days.length - 1] || {};
  z.check(`подсказка у последнего дня: «доходы ${fmt(last.income_pln)} · затраты ${fmt(last.costs_pln)}» и результат дня`,
    tip.startsWith(`доходы ${fmt(last.income_pln)} · затраты ${fmt(last.costs_pln)}`) && tip.includes('результат '), tip);
  await z.shot(boss, 'z12_profit_chart');
  await boss.mouse.move(5, 5);

  // строки «Прибыли за период» — прошлого месяца
  const pnl = await boss.evaluate(() => [...document.querySelectorAll('#ovPnl tr.ovRow')].map(r => [...r.cells].map(c => [...c.childNodes]
    .filter(x => !(x.classList && x.classList.contains('repL'))).map(x => x.textContent).join('').replace(/\s+/g, ' ').trim())));
  const head = await boss.evaluate(() => [...document.querySelectorAll('#ovPnl tr:first-child th')].map(t => t.textContent));
  const want = [['Доходы', num(PM.income.pln), sgn(WM.income_pln), pct(KM.income)], ['− Возвраты', num(PM.returns.pln), sgn(WM.returns_pln), pct(KM.returns)],
    ['− Себестоимость проданного и подаренного', num(PM.cogs.pln), sgn(WM.cogs_pln), pct(KM.cogs)], ['− Потери склада', num(PM.losses.pln), sgn(WM.losses_pln), pct(KM.losses)],
    ['= Валовая прибыль', sgn(PM.gross_pln), sgn(WM.gross_pln), diff(PM.gross_pln, WM.gross_pln)], ['− Расходы', num(PM.expenses.pln), sgn(WM.expenses_pln), pct(KM.expenses)],
    ['= Результат', sgn(PM.result_pln), sgn(WM.result_pln), diff(PM.result_pln, WM.result_pln)]];
  z.check('«Прибыль за период»: «Строка · За период · Прошлый период · Изменение», семь строк — доходы, возвраты, себестоимость, потери, '
    + 'валовая, расходы, результат — как у сервера', head.join('|') === 'Строка|За период|Прошлый период|Изменение'
    && JSON.stringify(pnl) === JSON.stringify(want), JSON.stringify({ head, pnl, want }));
  await boss.evaluate(() => { const e = document.getElementById('ovPnlHead'); if (e) e.scrollIntoView({ block: 'start' }); });
  await tap(boss, '#ovPnl tr.ovRow[data-k="income"]'); await z.sleep(400);
  const det = await boss.evaluate(() => { const d = document.querySelector('#ovPnl tr.ovDet'); return d ? [...d.querySelectorAll('div:not(.muted)')].map(x => x.textContent.replace(/\s+/g, ' ').trim()) : null; });
  const I = PM.income, plw = (n, a, b, c) => { const x = Math.abs(n) % 100, y = x % 10; return n + ' ' + (x > 10 && x < 20 ? c : y === 1 ? a : y >= 2 && y <= 4 ? b : c); };
  z.check('клик по «Доходам» — состав: пополнения, товары и кухня, пакеты — суммы и сколько операций, как у сервера',
    JSON.stringify(det) === JSON.stringify([`Пополнения — ${fmt(I.deposits_pln)} · ${plw(I.deposits, 'пополнение', 'пополнения', 'пополнений')}`,
      `Товары и кухня — ${fmt(I.goods_pln)} · ${plw(I.sales, 'продажа', 'продажи', 'продаж')}`, `Пакеты времени — ${fmt(I.packs_pln)} · ${plw(I.packs, 'пакет', 'пакета', 'пакетов')}`]),
    JSON.stringify({ det, I }));
  await z.shot(boss, 'z12_profit_pnl');
  await tap(boss, '#ovPnl tr.ovRow[data-k="income"]'); await z.sleep(300);
  await tap(boss, '#ovPnl tr.ovRow[data-k="expenses"]'); await z.sleep(400);
  const ex = await boss.evaluate(() => { const d = document.querySelector('#ovPnl tr.ovDet'); return { n: document.querySelectorAll('#ovPnl tr.ovDet').length,
    t: d ? d.textContent.replace(/\s+/g, ' ').trim() : '', link: !!(d && d.querySelector('a.lnk')) }; });
  const cats = (PM.expenses || {}).by_category || [];
  z.check('клик по «Доходам» ещё раз — состав закрылся; «Расходы» — по категориям как у сервера, закупки — не расход, ссылка во вкладку',
    ex.n === 1 && ex.link && (cats.length ? cats.every(c => ex.t.includes(`${c.category} — ${fmt(c.pln)}`)) : ex.t.includes('За период расходов не было'))
    && (!(PM.expenses || {}).purchases_pln || ex.t.includes(`Закупки товара за период — ${fmt(PM.expenses.purchases_pln)}: не расход`)), JSON.stringify({ ex, cats }));
  await tap(boss, '#ovPnl tr.ovDet a.lnk'); await z.sleep(2000);
  const ep = await boss.evaluate(() => ({ acc: accTab, exp: typeof expPeriod === 'undefined' ? null : expPeriod, on: (document.querySelector('#accTabs button.on') || {}).textContent }));
  z.check(`ссылка «Расходы» — вкладка «Расходы» за тот же месяц: ${ru(PREV + '-01')} — ${ru(lastd(PREV))}`,
    ep.acc === 'exp' && ep.exp && ep.exp.from === PREV + '-01' && ep.exp.to === lastd(PREV) && /^Расходы/.test(ep.on || ''), JSON.stringify(ep));

  // отработанное — тот же месяц: «Обзор» помнит выбранный месяц, пока страница открыта
  await boss.evaluate(() => accTabSet('ov')); await z.sleep(2500);
  const P2 = await profit(boss), E = P2.earned || {}, GV = E.given || {};
  const ek = await kpis(boss, 'ovEarned'), eline = norm(await z.text(boss, '#ovEarnParts'));
  z.check(`«Отработано за период» (${mname(PREV)}): «Отработанная выручка» и «Отдано временем» (злотые и часы) — как у сервера; состав — кошелёк, пакеты, товары, кухня`,
    (P2.period || {}).from === PREV + '-01' && ek['Отработанная выручка'] && ek['Отработанная выручка'].v === fmt(E.pln)
    && ek['Отдано временем'] && ek['Отдано временем'].v === `${fmt(GV.pln)} · ${hm(GV.minutes)}`
    && eline.startsWith(`время с кошелька ${num(E.wallet_pln)} + пакеты ${num(Math.round(((E.packs_pln || 0) + (E.estimate_pln || 0)) * 100) / 100)} + товары ${num(E.goods_pln)} + кухня ${num(E.kitchen_pln)} zł`),
    JSON.stringify({ per: P2.period, ek, eline, E }));
  await tap(boss, '#ovMonth .ovArr:last-child'); await z.sleep(2500);
  const m2 = await sw(boss), Pc = await profit(boss);
  z.check(`› — обратно ${mname(CUR)}: этот месяц, дальше листать нельзя`, (Pc.period || {}).preset === 'month' && m2.name === mname(CUR) && m2.on && !m2.next,
    JSON.stringify({ m2, per: Pc.period }));

  // плитки ведут во вкладки
  await tap(boss, '#ovNow button.kpi:first-child'); await z.sleep(2000);
  const cs = await state(boss);
  z.check('плитка «В ящике сейчас» — вкладка «Касса» с книгой', cs.acc === 'cash' && cs.on === 'Касса' && await boss.evaluate(() => !!document.querySelector('#accBody table.accBook')), JSON.stringify(cs));
  await z.nav(boss, 'hall'); await boss.click('#r-acc'); await z.sleep(2500);
  const s1 = await state(boss);
  await tap(boss, '#ovNow button.kpi:last-child'); await z.sleep(3000);
  const lb = await boss.evaluate(() => { const e = document.getElementById('repLiab'), r = e ? e.getBoundingClientRect() : { top: -1, bottom: 1e9 };
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight }; });
  z.check('ушли из «Кассы» и вошли снова — опять «Обзор»; плитка «Обязательства клуба» — «Отчёты → Деньги», ряд на экране',
    s1.acc === 'ov' && s1.on === 'Обзор' && lb.tab === 'stats' && lb.rep === 'money' && lb.seen, JSON.stringify({ s1, lb }));

  // палитра
  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'прибыль'); await z.sleep(500);
  const pi = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(3500);
  const pg = await boss.evaluate(() => { const e = document.getElementById('ovPnl'), r = e ? e.getBoundingClientRect() : { top: -1, bottom: 1e9 };
    return { tab: curTab, acc: typeof accTab === 'undefined' ? null : accTab, seen: r.top >= 0 && r.top < innerHeight }; });
  z.check('палитра «прибыль» → «Прибыль», Enter — «Бухгалтерия → Обзор», таблица «Прибыль за период» на экране',
    pi[0] === 'Прибыль' && pg.tab === 'acc' && pg.acc === 'ov' && pg.seen, JSON.stringify({ pi: pi.slice(0, 3), pg }));
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'касс'); await z.sleep(500);
  await boss.keyboard.press('Enter'); await z.sleep(2500);
  const pk = await state(boss);
  z.check('палитра «касса» ведёт сразу во вкладку «Касса», не в «Обзор»', pk.tab === 'acc' && pk.acc === 'cash' && pk.on === 'Касса', JSON.stringify(pk));

  // свой период: 90 дней — по неделям; один день — без графика; имя месяца — назад к месяцу
  await boss.evaluate(() => accTabSet('ov')); await z.sleep(2500);
  const custom = async (f, t) => { await boss.fill('#ovFrom', ru(f)).catch(() => {}); await boss.fill('#ovTo', ru(t)).catch(() => {});
    await tap(boss, '#accBody .toolrow button:has-text("Показать")'); await z.sleep(3500); };
  await custom(z.clubDay(-89), TODAY);
  const P90 = await profit(boss), w90 = weeks(P90.days || []), m3 = await sw(boss);
  const c90 = await boss.evaluate(() => ({ head: (document.getElementById('ovChartHead') || {}).textContent,
    bars: [...document.querySelectorAll('#ovChart .bars .bar')].map(b => ({ from: b.dataset.from, inc: +b.dataset.inc, out: +b.dataset.out })) }));
  z.check(`свой период — 90 дней (${ru(z.clubDay(-89))} — ${ru(TODAY)}): по неделям с понедельника, ${w90.length} пар столбиков, Σ недель = доходы `
    + 'и затраты периода; месяц в переключателе не выделен',
    (P90.period || {}).days === 90 && (P90.period || {}).from === z.clubDay(-89) && /по неделям$/i.test(norm(c90.head)) && JSON.stringify(c90.bars) === JSON.stringify(w90)
    && w90.reduce((a, b) => a + b.inc, 0) === G(P90.income.pln) && w90.reduce((a, b) => a + b.out, 0) === G(P90.costs_pln) && !m3.on,
    JSON.stringify({ head: c90.head, n: c90.bars.length, want: w90.length, first: c90.bars[0], wfirst: w90[0], m3 }));
  await z.shot(boss, 'z12_profit_custom');
  await custom(TODAY, TODAY);
  const c1 = await boss.evaluate(() => ({ chart: !!document.getElementById('ovChart'), note: (document.getElementById('ovChartNote') || {}).textContent || '' }));
  z.check('свой период в один день: графика нет, вместо него строка «За один день графика нет…»', !c1.chart && c1.note.startsWith('За один день графика нет'), JSON.stringify(c1));
  await tap(boss, '#ovMonthName'); await z.sleep(2500);
  const m4 = await sw(boss), P4 = await profit(boss);
  z.check(`из своего периода — нажать на месяц: снова «${mname(CUR)}» целиком, поля дат пустые`,
    m4.on && m4.name === mname(CUR) && (P4.period || {}).preset === 'month' && await boss.evaluate(() => !(document.getElementById('ovFrom') || {}).value),
    JSON.stringify({ m4, per: P4.period }));

  // телефон
  await boss.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'acc'); await z.sleep(1500);
  const look = () => ph.evaluate(() => { const f = document.getElementById('ovChart'); if (f) f.scrollIntoView({ block: 'center' });
    const r = document.querySelector('#ovPnl tr.ovRow[data-k="result"]');
    return { acc: accTab, sw: document.documentElement.scrollWidth, head: (document.getElementById('ovChartHead') || {}).textContent || '',
      bars: f ? f.querySelectorAll('.bars .bar').length : 0,
      kfit: [...document.querySelectorAll('#accBody .kpi')].length > 0 && [...document.querySelectorAll('#accBody .kpi')].every(k => k.getBoundingClientRect().right <= innerWidth),
      mfit: !!document.getElementById('ovMonth') && document.getElementById('ovMonth').getBoundingClientRect().right <= innerWidth,
      labels: r ? [...r.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) : [] }; });
  const PP = await profit(ph), nd = (PP.days || []).length, pr = await look();
  const wantNow = nd > 14 ? weeks(PP.days).length : nd >= 2 ? nd : 0;
  await z.sleep(300);
  await z.shot(ph, 'z12_profit_phone');
  z.check(`телефон 360: «Обзор» при входе, этот месяц (${nd} дн.) — ${nd > 14 ? 'по неделям' : nd >= 2 ? 'по дням' : 'без графика'}; переключатель месяцев, `
    + 'плитки и итоги помещаются; строки «Прибыли» — карточки с подписями «за период · прошлый · изменение»; без горизонтальной прокрутки',
    pr.acc === 'ov' && pr.bars === wantNow && (nd > 14 ? /по неделям$/i.test(norm(pr.head)) : true) && pr.kfit && pr.mfit && pr.sw <= 360
    && pr.labels.join('|') === 'за период|прошлый|изменение', JSON.stringify({ pr, nd, wantNow }));
  await ph.click('#ovMonth .ovArr:first-child', { timeout: 4000 }).catch(() => {}); await z.sleep(3000);
  const PP2 = await profit(ph), wp = weeks(PP2.days || []), pr2 = await look();
  await z.sleep(300);
  await z.shot(ph, 'z12_profit_phone_prev');
  z.check(`телефон 360: ‹ — ${mname(PREV)}, длиннее двух недель — по неделям (${wp.length} пар столбиков), без горизонтальной прокрутки`,
    (PP2.period || {}).from === PREV + '-01' && /по неделям$/i.test(norm(pr2.head)) && pr2.bars === wp.length && pr2.sw <= 360 && pr2.mfit,
    JSON.stringify({ pr2, per: PP2.period, want: wp.length }));
  await ph.evaluate(() => { const e = document.getElementById('ovPnlHead'); if (e) e.scrollIntoView({ block: 'start' }); }); await z.sleep(300);
  await z.shot(ph, 'z12_profit_phone_pnl');
  await z.done();
})().catch(z.crash);
