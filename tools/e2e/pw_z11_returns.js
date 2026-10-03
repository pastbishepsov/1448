// З11-и3 «Возвраты за прошлый месяц», браузер (№17; З-Р12; решение основателя 30.09). Владелец: «Отчёты» → «Деньги» — последний
// блок «Возвраты за <прошлый месяц>»: итоги — как в GET /admin/reports/returns, в таблице «Когда · Что · Гость · Оплачено ·
// Вернули · Кто отменил» — продажа, оплаченная и отменённая в прошлом месяце; период отчёта блок не меняет. «Бухгалтерия» →
// «Касса»: у сегодняшнего дня — «Возвраты наличными»: старая продажа картой, которую владелец вернул наличными, — с датой
// оплаты. Палитра «возвраты» → «Возвраты» → блок на экране; у админа такого пункта нет. Телефон 360 — строки карточками с
// подписями, без горизонтальной прокрутки.
// Данные: позиция «z11w Возврат» (9 zł) заводится каждый прогон и в конце удаляется вместе с поставкой; продажи — владельцем:
// X картой — отменена картой и сдвинута целиком в прошлый месяц (оплачена 5-го, отменена в последнюю минуту месяца — в списке она
// свежая, сколько бы отмен в месяце ни было); Y картой — оплата сдвинута на
// 12-е прошлого месяца, владелец вернул наличными (−9 в ящике); Z налом — «не возвращали» (+9 остались): ящик в конце как в начале.
const z = require('./_z');
(async () => {
  const NAME = 'z11w Возврат';
  const gone = async () => {
    for (const sid of z.rows(`SELECT id FROM sales WHERE name = '${NAME}' AND voided_at IS NULL`))
      await z.api('POST', `/admin/sales/${sid}/void`, { refund_method: 'none' }, 'boss');
    for (const id of z.rows(`SELECT id FROM goods WHERE name = '${NAME}'`)) z.dropGood(id);
  };
  await gone(); z.atEnd(gone);
  const TZ = 'Europe/Warsaw', rh = +z.sql(`SELECT COALESCE((SELECT value FROM settings WHERE key='report_hour'), '8')`);
  const month = z.sql(`SELECT to_char(((now() AT TIME ZONE '${TZ}') - interval '${rh} hours')::date, 'YYYY-MM')`);
  const prev = z.sql(`SELECT to_char('${month}-01'::date - interval '1 month', 'YYYY-MM')`);
  const at = (day, hhmm) => `(('${day}'::date + time '${hhmm}') AT TIME ZONE '${TZ}')`;
  const expected = async () => ((await z.api('GET', '/admin/cash/book?preset=today', undefined, 'boss')).data.now || {}).expected_pln;
  const E0 = await expected();
  const good = ((await z.api('POST', '/admin/goods', { name: NAME, category: 'z11', price_pln: 9, stock: 10 }, 'boss')).data.good || {}).id;
  const sell = async method => ((await z.api('POST', '/admin/sales', { good_id: good, qty: 1, method }, 'boss')).data.sale || {}).id;
  const voidS = async (id, how) => { const r = await z.api('POST', `/admin/sales/${id}/void`, { refund_method: how }, 'boss'); return r.status + ':' + (r.data.refund_method || r.data.code); };
  const x = await sell('card'), xv = await voidS(x, 'card');
  z.sql(`UPDATE sales SET created_at = ${at(prev + '-05', '12:00')}, paid_at = ${at(prev + '-05', '12:00')}, `
    + `voided_at = ${at(month + '-01', String(rh).padStart(2, '0') + ':00')} - interval '1 minute' WHERE id = '${x}'`);
  const y = await sell('card');
  z.sql(`UPDATE sales SET created_at = ${at(prev + '-12', '12:00')}, paid_at = ${at(prev + '-12', '12:00')} WHERE id = '${y}'`);
  const yv = await voidS(y, 'cash'), zz = await sell('cash'), zv = await voidS(zz, 'none');
  if (!z.check('данные: X картой — вернули на карту и сдвинули в прошлый месяц; Y картой из прошлого месяца — вернули наличными; Z налом — «не возвращали»',
    [xv, yv, zv].join() === '200:card,200:cash,200:none', [xv, yv, zv].join())) return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const fmt = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł');
  const MONTHS = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'];
  const title = 'Возвраты за ' + MONTHS[+prev.slice(5) - 1] + ' ' + prev.slice(0, 4);
  const dm = (iso, y) => iso.slice(8, 10) + '.' + iso.slice(5, 7) + (y ? '.' + iso.slice(0, 4) : '');
  const ret = (await z.api('GET', '/admin/reports/returns', undefined, 'boss')).data, t = ret.totals || {};
  const xv0 = ((ret.items || []).find(i => i.id === x) || {}).voided_at || '', xWhen = xv0 ? dm(xv0) + ' ' + xv0.slice(11, 16) : '?';
  const boss = await z.openAdmin('boss');
  const block = () => boss.evaluate(async () => {
    for (let i = 0; i < 60 && !(document.getElementById('repRetBody') || {}).dataset?.ready; i++) await new Promise(r => setTimeout(r, 100));
    const b = document.getElementById('repRetBody');
    return { title: (document.getElementById('repReturns') || {}).textContent, ready: !!(b && b.dataset.ready),
      kpis: Object.fromEntries([...(b ? b.querySelectorAll('.kpi') : [])].map(k => [k.firstChild.textContent.trim(), k.querySelector('b').innerText.replace(/\s+/g, ' ').trim()])),
      head: b && b.querySelector('table') ? [...b.querySelector('table').rows[0].cells].map(c => c.textContent) : [],
      rows: b && b.querySelector('table') ? [...b.querySelector('table').rows].slice(1).map(r => [...r.cells].map(c =>
        [...c.childNodes].filter(n => !(n.classList && n.classList.contains('repL'))).map(n => n.textContent).join('').replace(/\s+/g, ' ').trim())) : [],
      old: b ? [...b.querySelectorAll('.retOld')].map(e => e.textContent) : [] };
  });
  await boss.evaluate(() => palGoReport('money')); await z.sleep(2500);
  const b1 = await block();
  const xRow = [xWhen, NAME, '—', '9,00 zł картой · ' + dm(prev + '-05'), '9,00 zł на карту', 'boss'];
  z.check(`«Деньги»: последний блок — «${title}»; «Вернули» (живыми деньгами) и «— на карту» — как в отчёте сервера; колонки «Когда · Что · Гость · `
    + 'Оплачено · Вернули · Кто отменил»; строка X — отменена в последнюю минуту месяца, оплачена 5-го картой, вернули 9,00 на карту, отменил boss',
    b1.ready && b1.title === title && b1.kpis['Вернули'] === fmt(t.live_pln) && b1.kpis['— на карту'] === fmt(t.card_pln)
    && b1.head.join('|') === 'Когда|Что|Гость|Оплачено|Вернули|Кто отменил' && b1.rows.length === (ret.items || []).length
    && b1.rows.some(r => r.join('|') === xRow.join('|')), JSON.stringify({ b1: { ...b1, rows: b1.rows.slice(0, 3) }, want: xRow, t }));
  await boss.evaluate(() => { const e = document.getElementById('repReturns'); if (e) e.scrollIntoView({ block: 'start' }); }); await z.sleep(300);
  await z.shot(boss, 'z11_returns_money');
  await boss.click('#repPresets button[data-p="today"]'); await z.sleep(2500);
  const b2 = await block();
  z.check('период отчёта «Сегодня» — блок тот же: прошлый месяц, те же итоги и строки',
    b2.title === title && JSON.stringify(b2.kpis) === JSON.stringify(b1.kpis) && b2.rows.length === b1.rows.length, JSON.stringify({ t1: b1.title, t2: b2.title, k1: b1.kpis, k2: b2.kpis }));
  await boss.click('#repPresets button[data-p="d7"]'); await z.sleep(1500);

  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'возвраты'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(3500);
  const pg = await boss.evaluate(() => { const r = (document.getElementById('repReturns') || { getBoundingClientRect: () => ({ top: -1, bottom: 1e9 }) }).getBoundingClientRect();
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight }; });
  z.check('палитра владельца: «возвраты» → «Возвраты», Enter — «Отчёты» → «Деньги», блок возвратов на экране',
    items[0] === 'Возвраты' && pg.tab === 'stats' && pg.rep === 'money' && pg.seen, JSON.stringify({ items, pg }));

  const today = z.sql(`SELECT (((now() AT TIME ZONE '${TZ}') - interval '${rh} hours')::date)::text`);
  await boss.evaluate(() => accGo('cash')); await z.sleep(2500);
  await boss.click(`tr.accDay[data-date="${today}"]`, { timeout: 5000 }).catch(() => {}); await z.sleep(800);
  const det = await boss.evaluate(n => { const d = document.querySelector('tr.accDet td'); if (!d) return null;
    const subs = [...d.querySelectorAll('.dsub')].map(s => s.textContent), i = subs.indexOf('Возвраты наличными');
    const lines = []; let e = i < 0 ? null : d.querySelectorAll('.dsub')[i].nextElementSibling;
    while (e && !e.classList.contains('dsub')) { lines.push(e.innerText.replace(/\s+/g, ' ').trim()); e = e.nextElementSibling; }
    return { subs, lines: lines.filter(l => l.includes(n)), all: lines.length }; }, NAME);
  const book = (await z.api('GET', '/admin/cash/book?preset=today', undefined, 'boss')).data;
  const day = (book.days || []).slice(-1)[0] || {}, yb = (day.refunds || []).find(r => r.id === y) || {};
  const want = `${yb.voided_at ? norm(yb.voided_at).slice(11, 16) : '?'} · boss — 9,00 zł: ${NAME} · оплачено ${dm(prev + '-12', true)} картой`;
  z.check('«Касса», сегодня: раскрытие дня — «Возвраты наличными» после «Сдач»; строка Y: кто отдал, сколько, за что, оплачено 12-го прошлого месяца '
    + 'картой; строк столько же, сколько в книге сервера; Z («не возвращали») там нет',
    !!det && det.subs[0] === 'Сдачи' && det.subs[1] === 'Возвраты наличными' && det.lines.includes(want) && det.lines.length === (day.refunds || []).filter(r => r.name === NAME).length
    && det.all === (day.refunds || []).length && !(day.refunds || []).some(r => r.id === zz), JSON.stringify({ det: det && { ...det, lines: det.lines.slice(0, 3) }, want }));
  await boss.evaluate(n => { const l = [...document.querySelectorAll('tr.accDet td div')].find(e => e.innerText.includes(n)); if (l) l.scrollIntoView({ block: 'center' }); }, NAME);
  await z.sleep(300);
  await z.shot(boss, 'z11_returns_book');

  const adm = await z.openAdmin('admin1');
  await adm.evaluate(() => palOpen()); await adm.fill('#palInp', 'возвраты'); await z.sleep(500);
  const ai = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await adm.keyboard.press('Escape');
  z.check('админ: «Возвратов» в палитре нет (отчёт — владельцу)', !ai.includes('Возвраты'), JSON.stringify(ai));

  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => palGoReport('money')); await z.sleep(3000);
  const card = await ph.evaluate(async w => {
    for (let i = 0; i < 60 && !(document.getElementById('repRetBody') || {}).dataset?.ready; i++) await new Promise(r => setTimeout(r, 100));
    const r = [...document.querySelectorAll('#repRetBody tr')].find(t => t.cells[1] && t.cells[1].innerText.trim() === w.name && t.cells[0].innerText.includes(w.when));
    if (r) r.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: !!r && r.getBoundingClientRect().right <= innerWidth,
      kfit: [...document.querySelectorAll('#repRetBody .kpi')].every(k => k.getBoundingClientRect().right <= innerWidth),
      labels: r ? [...r.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) : [] };
  }, { name: NAME, when: xWhen.slice(0, 5) });
  await z.sleep(300);
  await z.shot(ph, 'z11_returns_phone');
  z.check('телефон 360: строка возврата — карточка с подписями («когда · гость · оплачено · вернули · кто отменил»), без горизонтальной прокрутки',
    card.sw <= 360 && card.fit && card.kfit && card.labels.join('|') === 'когда|гость|оплачено|вернули|кто отменил', JSON.stringify(card));
  await ph.evaluate(() => accGo('cash')); await z.sleep(2500);
  await ph.click(`tr.accDay[data-date="${today}"]`, { timeout: 5000 }).catch(() => {}); await z.sleep(800);
  const pb = await ph.evaluate(n => { const l = [...document.querySelectorAll('tr.accDet td div')].find(e => e.innerText.includes(n)); if (l) l.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: !!l && l.getBoundingClientRect().right <= innerWidth }; }, NAME);
  await z.sleep(300);
  await z.shot(ph, 'z11_returns_phone_book');
  z.check('телефон 360: «Касса» — строка возврата наличными переносится, без горизонтальной прокрутки', pb.sw <= 360 && pb.fit, JSON.stringify(pb));
  const E1 = await expected();
  z.check('наличные туда и обратно сошлись: Y −9 из ящика, Z +9 остались — «должно быть в ящике» как в начале', Math.abs(E1 - E0) < 0.005, JSON.stringify([E0, E1]));
  await z.done();
})().catch(z.crash);
