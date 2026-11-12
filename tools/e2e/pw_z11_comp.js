// З11-и5 «Подарено», браузер (№18; З-Р12, Д6; решение основателя 30.09). Админ: «Товары» — способ «Подарено»: появляется
// поле «за что», подпись «подарок — не выручка и не касса, в пределах дневного лимита компенсаций», кнопка «Подарить»; без
// причины — тост и ничего не записано; с причиной — подарок, в ленте продаж «подарено» с причиной и строка «подарено: …
// (не выручка)»; сверх дневного лимита — тост «Дневной лимит подарков и компенсаций…»; пакет «Подарено» сверх лимита — отказ.
// Владелец: пакет «Подарено» из карточки гостя, заказ кухни «Подарено» от стойки — на кухне «подарено»; «Деньги»: палитра
// «подарено» → ряд «Подарено · — по себестоимости» — как в отчёте сервера, «Подарено за период» — что, кому, по цене, за что,
// кто. Телефон 360 — строки карточками с подписями, без горизонтальной прокрутки.
// Данные: временный админ z11cp (на смене); гость Guest41; дневной лимит компенсаций на время проверки — 20 zł (настройка
// возвращается как была); позиция «z11q Подарок» (6 zł, 10 шт) заводится каждый прогон и в конце удаляется; самый дешёвый
// пакет каталога. Все подарки в конце отменяются.
const z = require('./_z');
(async () => {
  const NAME = 'z11q Подарок', MARK = 'z11q', GUEST = 'Guest41', G = z.uid(GUEST);
  const gone = async () => {
    for (const oid of z.rows(`SELECT id FROM kitchen_orders WHERE name = '${NAME}' AND status IN ('new','accepted','preparing','delivering')`))
      await z.api('POST', `/admin/kitchen/orders/${oid}/cancel`, {}, 'boss');
    for (const sid of z.rows(`SELECT id FROM sales WHERE name = '${NAME}' AND voided_at IS NULL`))
      await z.api('POST', `/admin/sales/${sid}/void`, { refund_method: 'none' }, 'boss');
    for (const pid of z.rows(`SELECT id FROM user_packages WHERE method = 'comp' AND voided_at IS NULL AND comp_reason LIKE '${MARK}%'`))
      await z.api('POST', `/admin/user-packages/${pid}/void`, {}, 'boss');
    for (const id of z.rows(`SELECT id FROM goods WHERE name = '${NAME}'`)) z.dropGood(id);
  };
  await gone(); z.atEnd(gone);
  const cap0 = z.sql(`SELECT value FROM settings WHERE key = 'admin_day_comp_cap_pln'`) || '0';
  z.atEnd(() => z.api('PUT', '/admin/settings', { admin_day_comp_cap_pln: +cap0 }, 'boss'));
  const setCap = await z.api('PUT', '/admin/settings', { admin_day_comp_cap_pln: 20 }, 'boss');
  await z.hireTemp('z11cp');
  const good = ((await z.api('POST', '/admin/goods', { name: NAME, category: 'z11', price_pln: 6, stock: 10 }, 'boss')).data.good || {}).id;
  const pack = z.sql(`SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1`);
  const comps = () => +z.sql(`SELECT count(*) FROM sales WHERE name = '${NAME}' AND method = 'comp' AND voided_at IS NULL`);
  if (!z.check('данные: лимит компенсаций админа 20 zł, позиция 6 zł на складе', setCap.status === 200 && !!good, JSON.stringify([setCap.status, good]))) return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const tap = (p, sel) => p.click(sel, { timeout: 4000 }).catch(() => {}); // нового на HEAD нет — проверка упадёт, прогон нет
  const put = (p, sel, v) => p.fill(sel, v, { timeout: 4000 }).catch(() => {});
  const fmt = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł');
  const adm = await z.openAdmin('z11cp');
  await z.nav(adm, 'goods');
  await adm.fill('#saleNick', GUEST); await adm.dispatchEvent('#saleNick', 'input');
  await tap(adm, '#s-goods .segs button[data-m="comp"]'); await z.sleep(200);
  await adm.click(`.gtile:has-text("${NAME}")`); await z.sleep(400);
  const f0 = await adm.evaluate(() => ({ why: !!document.getElementById('saleWhy') && !document.getElementById('saleWhy').classList.contains('hidden'),
    hint: document.getElementById('cartHint').textContent, btn: document.getElementById('cartSellBtn').innerText.trim() }));
  const t0 = norm(await z.toastAfter(adm, () => tap(adm, '#cartSellBtn'), 'У подарка нужна причина')); // З19-и13: тост этого действия, а не #toast через паузу
  z.check('админ: «Подарено» — поле «за что», подпись «подарок — не выручка и не касса, в пределах дневного лимита компенсаций», кнопка «Подарить · 6,00 zł»; '
    + 'без причины — тост «У подарка нужна причина…», ничего не записано',
    f0.why && /^1 шт · подарок — не выручка и не касса, в пределах дневного лимита компенсаций$/.test(norm(f0.hint)) && norm(f0.btn) === 'Подарить · 6,00 zł'
    && t0.startsWith('У подарка нужна причина') && comps() === 0, JSON.stringify({ f0, t0 }));
  await put(adm, '#saleWhy', `${MARK}: победа в турнире`);
  const t1 = norm(await z.toastAfter(adm, () => tap(adm, '#cartSellBtn'), 'Подарено:'));
  const feed = await adm.evaluate(n => ({ total: document.getElementById('salesTotal').textContent.replace(/\s+/g, ' '),
    row: (r => r ? { name: r.cells[1].innerText.replace(/\s+/g, ' ').trim(), pay: r.cells[3].innerText.trim() } : null)(
      [...document.querySelectorAll('#salesTable tr')].find(t => t.cells[1] && t.cells[1].innerText.includes(n))) }), NAME);
  z.check('с причиной — тост «Подарено: z11q Подарок — 6,00 zł · Guest41 · z11q: победа в турнире»; в ленте — «подарено», причина у позиции, '
    + 'в итоге смены «подарено: … (не выручка)»',
    t1.startsWith(`Подарено: ${NAME} — 6,00 zł · Guest41 · ${MARK}: победа в турнире`) && comps() === 1 && feed.row && feed.row.pay === 'подарено'
    && feed.row.name === `${NAME} · ${MARK}: победа в турнире` && /подарено: [\d\s,]+ zł \(\d+, не выручка\)/.test(feed.total), JSON.stringify({ t1, feed }));
  await z.shot(adm, 'z11_comp_goods');
  for (let i = 0; i < 3; i++) { await adm.click(`.gtile:has-text("${NAME}")`); await z.sleep(250); }
  await put(adm, '#saleWhy', `${MARK}: день рождения`);
  const t2 = norm(await z.toastAfter(adm, () => tap(adm, '#cartSellBtn'), 'Дневной лимит подарков'));
  z.check('ещё 3 × 6 = 18 zł при лимите 20 и уже подаренных 6 — тост «Дневной лимит подарков и компенсаций админа: 20 zł (уже выдано 6 zł)…», не записано',
    t2.startsWith('Дневной лимит подарков и компенсаций админа: 20 zł (уже выдано 6 zł)') && comps() === 1, t2);
  await adm.evaluate(() => cartClear());
  await adm.evaluate(id => openUserDrawer(id, 'pack'), G); await z.sleep(2000);
  await tap(adm, '#drawer .uform .segs button:has-text("Подарено")'); await z.sleep(400);
  await put(adm, '#packWhy', `${MARK}: постоянный гость`);
  const t3 = norm(await z.toastAfter(adm, () => tap(adm, '#drawer .uform button:has-text("Подарить пакет")'), 'Дневной лимит подарков'));
  z.check('админ: пакет «Подарено» сверх лимита — отказ «Дневной лимит подарков и компенсаций…»',
    t3.startsWith('Дневной лимит подарков и компенсаций админа') && !z.sql(`SELECT id FROM user_packages WHERE comp_reason = '${MARK}: постоянный гость' AND voided_at IS NULL`), t3);

  const boss = await z.openAdmin('boss');
  await boss.evaluate(id => openUserDrawer(id, 'pack'), G); await z.sleep(2000);
  await tap(boss, '#drawer .uform .segs button:has-text("Подарено")'); await z.sleep(400);
  const pf = await boss.evaluate(() => ({ why: !!document.getElementById('packWhy'), btn: [...document.querySelectorAll('#drawer .uform button')].map(b => b.innerText.trim()).find(t => /пакет/i.test(t)),
    hint: [...document.querySelectorAll('#drawer .uform .muted')].map(e => e.textContent).join(' | ') }));
  await put(boss, '#packWhy', `${MARK}: постоянный гость`);
  const t4 = norm(await z.toastAfter(boss, () => tap(boss, '#drawer .uform button:has-text("Подарить пакет")'), 'Подарен пакет'));
  z.check('владелец: пакет «Подарено» — поле «за что», «Подарить пакет», подпись без лимита; подарен — тост «Подарен пакет … · z11q: постоянный гость»',
    pf.why && pf.btn === 'Подарить пакет' && /Подарок — не выручка и не касса: минуты гостю, причина — в журнал и владельцу\./.test(pf.hint)
    && /^Подарен пакет .+ мин на счету · z11q: постоянный гость$/.test(t4), JSON.stringify({ pf, t4 }));
  await z.shot(boss, 'z11_comp_pack');
  await boss.evaluate(() => { closeDrawer(); nav('kitchen'); }); await z.sleep(1500);
  await boss.click('#s-kitchen button:has-text("Новый заказ")'); await z.sleep(800);
  await boss.fill('#kitGuest', GUEST); await boss.dispatchEvent('#kitGuest', 'input');
  await boss.selectOption('#kitNew select', { label: `${NAME} · 6,00 zł` });  await z.sleep(300);
  await tap(boss, '#kitNew .segs button:has-text("Подарено")'); await z.sleep(300);
  await put(boss, '#kitWhy', `${MARK}: извинение за PC-05`);
  const t5 = norm(await z.toastAfter(boss, () => boss.click('#kitNew button:has-text("Принять заказ")'), '· подарено:'));
  const kcard = await boss.evaluate(n => { const c = [...document.querySelectorAll('#kitBoard .card')].find(e => e.innerText.includes(n)); return c ? c.innerText.replace(/\s+/g, ' ') : null; }, NAME);
  z.check('владелец: заказ кухни «Подарено» с причиной — тост «… · подарено: z11q: извинение за PC-05», на кухне — «подарено»',
    t5.endsWith(`· подарено: ${MARK}: извинение за PC-05`) && kcard && kcard.includes('6,00 zł · подарено'), JSON.stringify({ t5, kcard }));

  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'подарено'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(3000);
  const rep = (await z.api('GET', '/admin/reports/money?' + await boss.evaluate(() => repQuery()), undefined, 'boss')).data, gf = rep.gifts || {};
  const pg = await boss.evaluate(() => {
    const r = (document.getElementById('repGifts') || { getBoundingClientRect: () => ({ top: -1, bottom: 1e9 }) }).getBoundingClientRect();
    const kpis = Object.fromEntries([...document.querySelectorAll('#repGifts .kpi')].map(k => [k.firstChild.textContent.trim(), k.querySelector('b').innerText.replace(/\s+/g, ' ').trim()]));
    const s = document.getElementById('repGiftList'), tb = s && s.nextElementSibling && s.nextElementSibling.querySelector('table');
    const rows = tb ? [...tb.rows].slice(1).map(r => [...r.cells].map(c => [...c.childNodes].filter(x => !(x.classList && x.classList.contains('repL')))
      .map(x => x.textContent).join('').replace(/\s+/g, ' ').trim())) : [];
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight, kpis, head: tb ? [...tb.rows[0].cells].map(c => c.textContent) : [], rows };
  });
  const has = (what, why, by) => pg.rows.some(r => r[1] === what && r[2] === GUEST && r[4] === why && r[5] === by);
  z.check('палитра «подарено» → «Подарено», Enter — «Деньги», ряд на экране; «Подарено» и «— по себестоимости» — как в отчёте сервера; «Подарено за период» — '
    + '«Когда · Что · Гость · По цене · За что · Кто подарил»: товар от z11cp, пакет и заказ кухни от boss — с причинами',
    items[0] === 'Подарено' && pg.tab === 'stats' && pg.rep === 'money' && pg.seen && pg.kpis['Подарено'] === fmt(gf.pln)
    && pg.kpis['— по себестоимости'] === (gf.cost_pln == null ? '—' : fmt(gf.cost_pln)) && pg.head.join('|') === 'Когда|Что|Гость|По цене|За что|Кто подарил'
    && has(NAME, `${MARK}: победа в турнире`, 'z11cp') && has(NAME, `${MARK}: извинение за PC-05`, 'boss')
    && pg.rows.some(r => /^Пакет «.+»$/.test(r[1]) && r[4] === `${MARK}: постоянный гость` && r[5] === 'boss'),
    JSON.stringify({ items, pg: { ...pg, rows: pg.rows.slice(0, 4) }, gf: { pln: gf.pln, cost: gf.cost_pln } }));
  await z.shot(boss, 'z11_comp_money');

  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => typeof palGoGifts === 'function' ? palGoGifts() : palGoReport('money')); await z.sleep(3000);
  const card = await ph.evaluate(why => { const s = document.getElementById('repGiftList'), tb = s && s.nextElementSibling;
    const r = tb ? [...tb.querySelectorAll('tr')].find(t => t.innerText.includes(why)) : null;
    if (r) r.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: !!r && r.getBoundingClientRect().right <= innerWidth,
      kfit: [...document.querySelectorAll('#repGifts .kpi')].every(k => k.getBoundingClientRect().right <= innerWidth),
      labels: r ? [...r.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) : [] }; }, `${MARK}: победа в турнире`);
  await z.sleep(300);
  await z.shot(ph, 'z11_comp_phone');
  z.check('телефон 360: «Подарено за период» — карточка с подписями («когда · гость · по цене · за что · кто подарил»), без горизонтальной прокрутки',
    card.sw <= 360 && card.fit && card.kfit && card.labels.join('|') === 'когда|гость|по цене|за что|кто подарил', JSON.stringify(card));
  await z.nav(ph, 'goods');
  await tap(ph, '#s-goods .segs button[data-m="comp"]'); await z.sleep(200);
  await ph.click(`.gtile:has-text("${NAME}")`); await z.sleep(400);
  const pgd = await ph.evaluate(() => { const w = document.getElementById('saleWhy'); if (w) w.scrollIntoView({ block: 'center' });
    const b = document.getElementById('cartSellBtn');
    return { sw: document.documentElement.scrollWidth, fit: !!w && !w.classList.contains('hidden') && w.getBoundingClientRect().right <= innerWidth,
      btn: !!b && b.getBoundingClientRect().right <= innerWidth }; });
  await z.sleep(300);
  await z.shot(ph, 'z11_comp_phone_goods');
  await ph.evaluate(() => cartClear());
  z.check('телефон 360: «Товары» — способ «Подарено», поле «за что» и кнопка «Подарить» помещаются (подпись корзины — своей строкой), без горизонтальной прокрутки',
    pgd.sw <= 360 && pgd.fit && pgd.btn, JSON.stringify(pgd));
  await z.done();
})().catch(z.crash);
