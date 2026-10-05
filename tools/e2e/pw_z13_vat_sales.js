// З13-и3 «Продажи по ставкам VAT», браузер (№72 «а»; решения основателя 04.10, З-Р37). «Бухгалтерия» → «Обзор»: блок
// «Продажи по ставкам VAT» за месяц «Обзора» — «Ставка · Брутто · Нетто · VAT» и «Итого» как у сервера, как считали (по
// настройкам «Бухгалтерии») со ссылкой в «Экономику», из чего и по способам; «По дням» — строки дней с продажами, как у
// сервера. «При пополнении» «Итого брутто» — это «Доходы» − «Возвраты» из «Прибыли за период» на том же экране;
// освобождённый от VAT — одна строка «zw»; ‹ › — блок за выбранный месяц. «Экономика» → «Бухгалтерия» — «Подарки товаров
// в VAT»: «Да — по себестоимости» с пояснением, предупреждением и взводом на «Сохранить». Палитра «фискальн» → блок на
// экране; у админа — нет. Телефон 360 — строки карточками с подписями, без прокрутки вбок.
// Данные: позиции «z13t Напиток» (23 %) и «z13t Снек» (8 %) заводятся каждый прогон и в конце удаляются вместе с
// продажами (картой, отменяются «не возвращали»); настройки «Бухгалтерии» в конце — как были.
const z = require('./_z');
(async () => {
  const MARK = 'z13t', A = 'z13t Напиток', B = 'z13t Снек';
  const KEYS = "'acc_vat_payer', 'acc_vat_moment', 'acc_gift_vat', 'acc_topup_vat'";
  const accOrig = z.rows(`SELECT key || '|' || value FROM settings WHERE key IN (${KEYS})`).map(r => r.split('|'));
  const put = (k, v) => z.sql(`INSERT INTO settings (key, value) VALUES ('${k}', '${v}') ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`);
  const gone = async () => {
    for (const sid of z.rows(`SELECT id FROM sales WHERE name LIKE '${MARK} %' AND voided_at IS NULL`))
      await z.api('POST', `/admin/sales/${sid}/void`, { refund_method: 'none' }, 'boss');
    for (const id of z.rows(`SELECT id FROM goods WHERE name LIKE '${MARK} %'`)) z.dropGood(id);
    z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`);
    for (const [k, v] of accOrig) put(k, v);
  };
  await gone(); z.atEnd(gone);
  z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`); // с известного начала: плательщик VAT, при трате, подарки без VAT
  const mk = async (name, price, vat) => ((await z.api('POST', '/admin/goods', { name, category: MARK, price_pln: price, stock: 10, vat_pct: vat }, 'boss')).data.good || {}).id;
  const ga = await mk(A, 6, 23), gb = await mk(B, 4, 8);
  const sold = [(await z.api('POST', '/admin/sales', { good_id: ga, qty: 1, method: 'card' }, 'boss')).status,
    (await z.api('POST', '/admin/sales', { good_id: gb, qty: 1, method: 'card' }, 'boss')).status];
  if (!z.check('данные: «z13t Напиток» (23 %) и «z13t Снек» (8 %) проданы картой', ga && gb && sold.join() === '201,201', JSON.stringify({ ga, gb, sold })))
    return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const num = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }));
  const sgn = v => (v < 0 ? '−' : '') + num(Math.abs(v || 0)) + ' zł'; // как ovMoney: минус длинным знаком
  const G = v => Math.round((v || 0) * 100);
  const MN = ['январь', 'февраль', 'март', 'апрель', 'май', 'июнь', 'июль', 'август', 'сентябрь', 'октябрь', 'ноябрь', 'декабрь'];
  const MR = ['янв', 'фев', 'мар', 'апр', 'мая', 'июн', 'июл', 'авг', 'сен', 'окт', 'ноя', 'дек'];
  const addm = (m, k) => { const d = new Date(+m.slice(0, 4), +m.slice(5, 7) - 1 + k, 1); return d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0'); };
  const TODAY = z.clubDay(0), CUR = TODAY.slice(0, 7), PREV = addm(CUR, -1);
  const vat = async q => (await z.api('GET', '/admin/reports/vat?' + q, undefined, 'boss')).data;
  // блок на экране: строки таблицы (подписи у чисел — только на телефоне, innerText их не видит), как считали, из чего
  const block = p => p.evaluate(() => {
    const t = document.getElementById('ovVat'), txt = id => ((document.getElementById(id) || {}).textContent || '').replace(/\s+/g, ' ').trim();
    return { head: !!document.getElementById('ovVatHead'), how: txt('ovVatHow'), parts: txt('ovVatParts'), pays: txt('ovVatPays'), empty: txt('ovVatEmpty'),
      rows: t ? [...t.rows].slice(1).map(r => [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()).join(' | ')) : null };
  });
  const want = v => (v.rates || []).map(r => [r.label, sgn(r.gross_pln), sgn(r.net_pln), sgn(r.vat_pln)].join(' | '))
    .concat([['Итого', sgn((v.total || {}).gross_pln), sgn((v.total || {}).net_pln), sgn((v.total || {}).vat_pln)].join(' | ')]);
  const rerender = async p => { await p.evaluate(() => renderOv()); await z.sleep(3000); };

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'acc');
  await boss.waitForSelector('#ovVatHead', { timeout: 30000 }).catch(() => {});
  const V = await vat('preset=month'), b0 = await block(boss);
  z.check('«Обзор» → «Продажи по ставкам VAT» за этот месяц: «Ставка · Брутто · Нетто · VAT» и «Итого» — как у сервера; '
    + 'есть 23 % и 8 %', b0.head && JSON.stringify(b0.rows) === JSON.stringify(want(V))
    && (V.rates || []).some(r => r.vat_pct === 23) && (V.rates || []).some(r => r.vat_pct === 8), JSON.stringify({ rows: b0.rows, want: want(V) }));
  z.check('как считали — «VAT при трате…», «Возвраты — минусом в день отмены», ссылка «Как считать — в «Экономика» → «Бухгалтерия»»; '
    + '«Из чего:» — виды, «По способам — для сверки с терминалом:» — способы',
    /^VAT при трате: время с кошелька — по ставке зоны/.test(b0.how) && /Возвраты — минусом в день отмены/.test(b0.how)
    && /Как считать — в «Экономика» → «Бухгалтерия»/.test(b0.how) && /^Из чего: .*товары и кухня/.test(b0.parts) && /^По способам — для сверки с терминалом: .*карта/.test(b0.pays),
    JSON.stringify(b0));
  await boss.evaluate(() => { const e = document.getElementById('ovVatHead'); if (e) e.scrollIntoView({ block: 'start' }); });
  await z.sleep(300);
  await z.shot(boss, 'z13_vat_sales');

  await boss.evaluate(() => { const d = document.getElementById('ovVatDays'); if (d) d.open = true; });
  await z.sleep(300);
  const dd = await boss.evaluate(() => { const t = document.querySelector('#ovVatDays table'); return t ? { head: [...t.rows[0].cells].map(c => c.textContent.trim()),
    rows: [...t.rows].slice(1).map(r => ({ d: r.dataset.d, cells: [...r.cells].map(c => c.innerText.replace(/\s+/g, ' ').trim()) })) } : null; });
  const vdays = (V.days || []).filter(x => (x.rates || []).length);
  const dayRow = x => [+x.date.slice(8, 10) + ' ' + MR[+x.date.slice(5, 7) - 1]].concat((V.rates || []).map(r => {
    const c = (x.rates || []).find(y => y.vat_pct === r.vat_pct); return c ? sgn(c.gross_pln) : '—'; }), [sgn(x.gross_pln), sgn(x.vat_pln)]);
  z.check('«По дням — для сверки с дневными отчётами кассы»: колонки — ставки периода, строки — дни с продажами, брутто по ставкам, '
    + 'брутто и VAT дня — как у сервера', !!dd && dd.head.join('|') === ['День'].concat((V.rates || []).map(r => r.label), ['Брутто', 'VAT']).join('|')
    && JSON.stringify(dd.rows.map(r => r.cells)) === JSON.stringify(vdays.map(dayRow)) && dd.rows.some(r => r.d === TODAY),
    JSON.stringify({ dd, want: vdays.map(dayRow) }));
  await boss.evaluate(() => { const e = document.getElementById('ovVatDays'); if (e) e.scrollIntoView({ block: 'center' }); });
  await z.sleep(300);
  await z.shot(boss, 'z13_vat_sales_days');

  put('acc_vat_moment', 1);
  await rerender(boss);
  const S = await vat('preset=month'), b1 = await block(boss);
  const pnl = await boss.evaluate(() => Object.fromEntries([...document.querySelectorAll('#ovPnl tr.ovRow')].map(r =>
    [r.dataset.k, r.cells[1].innerText.replace(/\s+/g, ' ').trim()])));
  const P = (await z.api('GET', '/admin/reports/profit?preset=month', undefined, 'boss')).data;
  z.check('«VAT при пополнении»: таблица — как у сервера, «Итого брутто» = «Доходы» − «Возвраты» из «Прибыли за период» на том же экране',
    /^VAT при пополнении: пополнения — по ставке пополнения/.test(b1.how) && JSON.stringify(b1.rows) === JSON.stringify(want(S))
    && G((S.total || {}).gross_pln) === G(P.income.pln) - G(P.returns.pln) && pnl.income === num(P.income.pln) && pnl.returns === num(P.returns.pln),
    JSON.stringify({ how: b1.how, total: S.total, pnl, income: P.income && P.income.pln, returns: P.returns && P.returns.pln }));
  put('acc_vat_payer', 0);
  await rerender(boss);
  const b2 = await block(boss), Z = await vat('preset=month');
  z.check('освобождённый от VAT — «Клуб освобождён от VAT — всё «zw»», одна строка «zw», VAT 0',
    /^Клуб освобождён от VAT — всё «zw»/.test(b2.how) && (b2.rows || []).length === 2 && /^zw \|/.test(b2.rows[0]) && (Z.rates || []).length === 1
    && G((Z.total || {}).vat_pln) === 0 && JSON.stringify(b2.rows) === JSON.stringify(want(Z)), JSON.stringify(b2));
  z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`);

  await boss.evaluate(() => ovStep(-1));
  await z.sleep(3500);
  const VP = await vat('month=' + PREV), b3 = await block(boss);
  z.check(`‹ — прошлый месяц (${MN[+PREV.slice(5, 7) - 1]}): блок за него — как у сервера с month=${PREV}`,
    (VP.rates || []).length ? JSON.stringify(b3.rows) === JSON.stringify(want(VP)) : /За период продаж нет/.test(b3.empty), JSON.stringify({ b3, want: want(VP) }));
  await boss.evaluate(() => ovMonthGo(''));
  await z.sleep(3000);

  await boss.click('#ovVatHow a').catch(() => {});
  await z.sleep(2500);
  const go = await boss.evaluate(() => { const e = document.getElementById('set_vatpayer'), r = e ? e.closest('.setcard').getBoundingClientRect() : null;
    return { tab: curTab, seen: !!r && r.top >= 0 && r.top < innerHeight }; });
  z.check('ссылка «Как считать» — «Экономика», карточка «Бухгалтерия» на экране', go.tab === 'settings' && go.seen, JSON.stringify(go));
  const gs = await boss.evaluate(() => { const s = document.getElementById('set_giftvat');
    return s ? { v: s.value, o: [...s.options].map(o => o.textContent), card: (s.closest('.setcard').querySelector('h4') || {}).textContent,
      label: (document.querySelector('label[for="set_giftvat"]') || {}).textContent || '' } : null; });
  z.check('«Бухгалтерия» → «Подарки товаров в VAT (nieodpłatne przekazanie towarów)»: «Нет — подарки без VAT» (по умолчанию) / «Да — по себестоимости»',
    !!gs && gs.card === 'Бухгалтерия' && gs.v === '0' && gs.o.join('|') === 'Нет — подарки без VAT|Да — по себестоимости'
    && /nieodpłatne przekazanie towarów/.test(gs.label), JSON.stringify(gs));
  const puts = [];
  boss.on('request', r => { if (r.method() === 'PUT' && r.url().endsWith('/admin/settings')) puts.push(r.postData() || ''); });
  if (gs) await boss.selectOption('#set_giftvat', '1');
  const gn = await boss.evaluate(() => ({ note: ((document.getElementById('setVatNote') || {}).textContent || '').replace(/\s+/g, ' '),
    warn: !!document.getElementById('setVatWarn') && !document.getElementById('setVatWarn').classList.contains('hidden') }));
  await z.shot(boss, 'z13_vat_gifts');
  await boss.click('#setSave').catch(() => {});
  await z.sleep(500);
  const armed = await z.text(boss, '#setSave');
  await boss.click('#setSave').catch(() => {});
  await z.sleep(1800);
  z.check('«Да — по себестоимости»: пояснение «Подарки товаров — в VAT по себестоимости», предупреждение про прошлые месяцы; «Сохранить» — '
    + 'взвод «Ещё раз — сменить расчёт VAT», второй клик — PUT {acc_gift_vat: 1}, в базе 1',
    /Подарки товаров — в VAT по себестоимости/.test(gn.note) && gn.warn && /Ещё раз — сменить расчёт VAT/.test(armed || '')
    && puts.length === 1 && puts[0] === '{"acc_gift_vat":1}' && z.sql("SELECT value FROM settings WHERE key = 'acc_gift_vat'") === '1',
    JSON.stringify({ gn, armed, puts }));
  z.sql(`DELETE FROM settings WHERE key IN (${KEYS})`);

  await boss.evaluate(() => palOpen());
  await boss.fill('#palInp', 'фискальн');
  await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter');
  await z.sleep(4000);
  const pv = await boss.evaluate(() => { const e = document.getElementById('ovVatHead'), r = e ? e.getBoundingClientRect() : null;
    return { tab: curTab, acc: typeof accTab === 'undefined' ? null : accTab, seen: !!r && r.top >= 0 && r.top < innerHeight }; });
  z.check('палитра владельца: «фискальн» → «Продажи по ставкам VAT», Enter — «Бухгалтерия» → «Обзор», блок на экране',
    items[0] === 'Продажи по ставкам VAT' && pv.tab === 'acc' && pv.acc === 'ov' && pv.seen, JSON.stringify({ items, pv }));

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const adm = await z.openAdmin('admin1');
  await adm.evaluate(() => palOpen());
  await adm.fill('#palInp', 'фискальн');
  await z.sleep(500);
  const ai = await adm.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await adm.keyboard.press('Escape');
  const as = (await z.api('GET', '/admin/reports/vat?preset=month', undefined, 'admin1')).status;
  z.check('админ: в палитре «Продаж по ставкам VAT» нет, отчёт ему не отдаётся — 403', !ai.includes('Продажи по ставкам VAT') && as === 403,
    JSON.stringify({ ai, as }));

  await adm.goto('about:blank');
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'acc');
  await ph.waitForSelector('#ovVatHead', { timeout: 30000 }).catch(() => {});
  const pc = await ph.evaluate(() => { const t = document.getElementById('ovVat'); if (!t) return null;
    const r = t.rows[1]; r.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: [...t.rows].slice(1).every(x => x.getBoundingClientRect().right <= innerWidth),
      labels: [...r.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) }; });
  await z.sleep(300);
  await z.shot(ph, 'z13_vat_sales_phone');
  z.check('телефон 360: строки ставок — карточки с подписями «брутто · нетто · VAT», без прокрутки вбок',
    !!pc && pc.sw <= 360 && pc.fit && pc.labels.join('|') === 'брутто|нетто|VAT', JSON.stringify(pc));
  await z.done();
})().catch(z.crash);
