// З11-и4 «Списание долга и дебиторка», браузер (№15; З-Р12; решения основателя 30.09). Владелец: карточка гостя → форма долга
// → «Списать долг N zł» — одной кнопкой, без причины (второе нажатие подтверждает); в карточке вместо «Должен» — «Долг списан
// … · Вернуть долг»; «Вернуть долг» — долг снова в карточке. Лента продаж — «долг списан» без кнопки отмены; кухня — «долг
// списан — ушёл, не заплатив». «Деньги»: «Дебиторка сейчас», «— списанные долги», «Списано за период» — как в отчёте сервера;
// в «Дебиторке» ушедший гость — «ушёл, не рассчитавшись». Палитра «дебиторка» → «Дебиторка» → ряд на экране. Админ: «Долг
// списан» без «Вернуть долг», в форме долга — без «Списать долг». Телефон 360 — без горизонтальной прокрутки.
// Данные: гость Guest41 (кошелёк докладывается до 10 zł; за PC-03 — продажа в долг и заказ кухни при выходе, выдан; сессия
// завершается — гость «ушёл»); позиция «z11e Долг» (7 zł) заводится каждый прогон и в конце удаляется вместе с поставкой. В
// конце долг рассчитан картой, продажа бара отменена с возвратом на карту; заказ кухни остаётся рассчитанным.
const z = require('./_z');
(async () => {
  const NAME = 'z11e Долг', GUEST = 'Guest41', G = z.uid(GUEST);
  const debtOf = async () => +(((await z.api('GET', `/admin/users/${G}/debt`, undefined, 'boss')).data.debt || {}).total_pln || 0);
  const gone = async () => {
    for (const w of z.rows(`SELECT id FROM debt_write_offs WHERE user_id = '${G}' AND cancelled_at IS NULL`))
      await z.api('POST', `/admin/debt-write-offs/${w}/cancel`, undefined, 'boss');
    for (const s of z.rows(`SELECT id FROM sessions WHERE status = 'active' AND user_id = '${G}'`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    if (await debtOf() > 0) await z.api('POST', `/admin/users/${G}/settle`, { method: 'card' }, 'boss');
    for (const s of z.rows(`SELECT id FROM sales WHERE name = '${NAME}' AND voided_at IS NULL AND paid_at IS NOT NULL
        AND id NOT IN (SELECT sale_id FROM kitchen_orders WHERE sale_id IS NOT NULL)`))
      await z.api('POST', `/admin/sales/${s}/void`, { refund_method: 'card' }, 'boss');
    for (const id of z.rows(`SELECT id FROM goods WHERE name = '${NAME}'`)) z.dropGood(id);
  };
  await gone(); z.atEnd(gone);
  const need = 10 - +z.sql(`SELECT wallet_grosz FROM users WHERE id = '${G}'`) / 100;
  if (need > 0) await z.api('POST', `/admin/users/${G}/deposit`, { amount_pln: need, method: 'card' }, 'boss');
  const good = ((await z.api('POST', '/admin/goods', { name: NAME, category: 'z11', price_pln: 7, stock: 20 }, 'boss')).data.good || {}).id;
  const pc = z.sql(`SELECT id FROM computers WHERE name = 'PC-03'`);
  const seat = await z.api('POST', `/admin/computers/${pc}/session`, { guest_id: G }, 'admin1');
  const sale = await z.api('POST', '/admin/sales', { good_id: good, qty: 1, method: 'later', guest_id: G }, 'admin1');
  const ko = ((await z.api('POST', '/admin/kitchen/orders', { guest_id: G, good_id: good, qty: 1, method: 'later' }, 'admin1')).data.order || {}).id;
  const done = await z.api('POST', `/admin/kitchen/orders/${ko}/status`, { status: 'done' }, 'admin1');
  const end = await z.api('POST', `/admin/sessions/${seat.data.session_id}/end`, {}, 'admin1');
  const d0 = await debtOf();
  if (!z.check('данные: Guest41 за PC-03 — продажа в долг 7 zł и выданный заказ кухни 7 zł, сессия завершена — должен 14,00',
    [seat.status, sale.status, done.status, end.status].join() === '201,201,200,200' && d0 === 14, JSON.stringify([seat.status, sale.status, done.status, end.status, d0]))) return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const fmt = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł');
  const boss = await z.openAdmin('boss');
  const kv = page => page.evaluate(() => Object.fromEntries([...document.querySelectorAll('#drawer table.kv tr')].map(r =>
    [r.cells[0].textContent.trim(), { text: r.cells[1].innerText.replace(/\s+/g, ' ').trim(), btn: !!r.querySelector('button') }])));
  await boss.evaluate(id => openUserDrawer(id, 'debt'), G); await z.sleep(1500);
  const wbtn = boss.locator('#woBtn');
  const b0 = { has: await wbtn.count(), text: await wbtn.count() ? norm(await wbtn.innerText()) : '' };
  if (b0.has) { await wbtn.click(); await z.sleep(300); }
  const armed = await wbtn.count() ? norm(await wbtn.innerText()) : '';
  await z.shot(boss, 'z11_debt_form');
  const toast1 = norm(await z.toastAfter(boss, async () => { if (b0.has) await wbtn.click(); }, 'списан:')), k1 = await kv(boss);
  z.check('владелец: в форме долга — «Списать долг 14,00 zł»; первое нажатие — «Ещё раз — списать 14,00 zł», второе — списано, без причины: '
    + 'тост «Долг Guest41 списан: 14,00 zł — в «Потерях → долги»…»; в карточке вместо «Должен» — «Долг списан 14,00 zł — кухня 7,00 (1) · бар 7,00 (1) · … · boss» с «Вернуть долг»',
    b0.has === 1 && b0.text === 'Списать долг 14,00 zł' && armed === 'Ещё раз — списать 14,00 zł' && toast1.startsWith('Долг Guest41 списан: 14,00 zł — в «Потерях → долги»')
    && !k1['Должен'] && k1['Долг списан'] && k1['Долг списан'].text.startsWith('14,00 zł — кухня 7,00 (1) · бар 7,00 (1) · ') && k1['Долг списан'].text.includes('· boss')
    && k1['Долг списан'].btn, JSON.stringify({ b0, armed, toast1, k1: k1['Долг списан'], owes: k1['Должен'] }));
  await z.shot(boss, 'z11_debt_card');
  z.check('сервер: долг 0, списание — одно, на 14,00', await debtOf() === 0
    && z.sql(`SELECT count(*) || '|' || max(total_grosz) FROM debt_write_offs WHERE user_id = '${G}' AND cancelled_at IS NULL`) === '1|1400');

  await boss.evaluate(() => { closeDrawer(); nav('goods'); }); await z.sleep(2000);
  const srow = await boss.evaluate(n => { const r = [...document.querySelectorAll('#salesTable tr')].find(t => t.cells[1] && t.cells[1].innerText.includes(n));
    return r ? { pay: r.cells[3].innerText.trim(), btn: !!r.cells[6].querySelector('button') } : null; }, NAME);
  await boss.evaluate(() => nav('kitchen')); await z.sleep(2000);
  const kcard = await boss.evaluate(n => { const c = [...document.querySelectorAll('#kitBoard .card')].find(e => e.innerText.includes(n));
    return c ? norm0(c.innerText) : null; function norm0(s){ return s.replace(/\s+/g, ' ').trim(); } }, NAME);
  z.check('лента продаж: у продажи — «долг списан», кнопки отмены нет; кухня: заказ закрыт — «долг списан — ушёл, не заплатив»',
    srow && srow.pay === 'долг списан' && !srow.btn && kcard && kcard.includes('долг списан — ушёл, не заплатив'), JSON.stringify({ srow, kcard }));

  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'дебиторка'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(3000);
  const rep = (await z.api('GET', '/admin/reports/money?' + await boss.evaluate(() => repQuery()), undefined, 'boss')).data;
  const rcv = (rep.debts || {}).receivable || {}, wof = (rep.debts || {}).written_off || {};
  const pg = await boss.evaluate(n => {
    const r = (document.getElementById('repDebt') || { getBoundingClientRect: () => ({ top: -1, bottom: 1e9 }) }).getBoundingClientRect();
    const kpis = Object.fromEntries([...document.querySelectorAll('#repBody .kpi')].map(k => [k.firstChild.textContent.trim(), k.querySelector('b').innerText.replace(/\s+/g, ' ').trim()]));
    const t = document.getElementById('repWriteOffs'), tb = t && t.nextElementSibling && t.nextElementSibling.querySelector('table');
    const wrow = tb ? [...tb.rows].slice(1).map(r => [...r.cells].map(c => [...c.childNodes].filter(x => !(x.classList && x.classList.contains('repL')))
      .map(x => x.textContent).join('').replace(/\s+/g, ' ').trim())).find(c => c[1] === n) : null;
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight, kpis, wrow }; }, GUEST);
  z.check('палитра «дебиторка» → «Дебиторка», Enter — «Деньги», ряд дебиторки на экране; «Дебиторка сейчас» и «— списанные долги» — как в отчёте сервера; '
    + '«Списано за период» — Guest41: 14,00 zł, кухня 7,00 (1) · бар 7,00 (1), кто списал — boss',
    items[0] === 'Дебиторка' && pg.tab === 'stats' && pg.rep === 'money' && pg.seen && pg.kpis['Дебиторка сейчас'] === fmt(rcv.total_pln)
    && pg.kpis['— списанные долги'] === fmt(wof.pln) && wof.pln >= 14 && pg.wrow && pg.wrow[2] === '14,00 zł' && pg.wrow[3] === 'кухня 7,00 (1) · бар 7,00 (1)'
    && pg.wrow[4] === 'boss', JSON.stringify({ items, pg, rcv: rcv.total_pln, wof: wof.pln }));
  await z.shot(boss, 'z11_debt_money');

  const adm = await z.openAdmin('admin1');
  await adm.evaluate(id => openUserDrawer(id), G); await z.sleep(1500);
  const ka = await kv(adm);
  await boss.evaluate(id => openUserDrawer(id), G); await z.sleep(1500);
  const rb = boss.locator('#drawer tr.woRow button'), rbn = await rb.count();
  if (rbn) { await rb.click(); await z.sleep(300); }
  const rarmed = rbn ? norm(await rb.innerText()) : '';
  const toast2 = norm(await z.toastAfter(boss, async () => { if (rbn) await rb.click(); }, 'вернули:')), k2 = await kv(boss);
  z.check('админ видит «Долг списан» без «Вернуть долг»; владелец: «Вернуть долг» → «Ещё раз — вернуть 14,00 zł в долг» → долг снова в карточке: «Должен 14,00»',
    ka['Долг списан'] && !ka['Долг списан'].btn && rarmed === 'Ещё раз — вернуть 14,00 zł в долг' && toast2.startsWith('Долг Guest41 вернули: 14,00 zł')
    && !k2['Долг списан'] && k2['Должен'] && k2['Должен'].text.startsWith('🧾 14,00 zł') && await debtOf() === 14,
    JSON.stringify({ ka: ka['Долг списан'], rarmed, toast2, k2: k2['Должен'] }));
  await adm.evaluate(id => openUserDrawer(id, 'debt'), G); await z.sleep(1500);
  const af = await adm.evaluate(() => ({ settle: [...document.querySelectorAll('#drawer .uform button')].some(b => b.innerText.includes('Рассчитать')),
    wo: !!document.getElementById('woBtn') }));
  z.check('админ: в форме долга — «Рассчитать», «Списать долг» нет (списывает владелец)', af.settle && !af.wo, JSON.stringify(af));
  const aw = await z.api('POST', `/admin/users/${G}/debt/write-off`, undefined, 'admin1');
  z.check('админ списать через API — 403', aw.status === 403, String(aw.status));

  await boss.goto('about:blank'); await adm.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => typeof palGoDebt === 'function' ? palGoDebt() : palGoReport('money')); await z.sleep(3000);
  const card = await ph.evaluate(n => { const s = document.getElementById('repDebtList'), tb = s && s.nextElementSibling && s.nextElementSibling.nextElementSibling;
    const r = tb ? [...tb.querySelectorAll('tr')].find(t => t.cells[0] && t.cells[0].innerText.trim() === n) : null;
    if (r) r.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: !!r && r.getBoundingClientRect().right <= innerWidth,
      kfit: [...document.querySelectorAll('#repDebt .kpi')].every(k => k.getBoundingClientRect().right <= innerWidth),
      gone: !!r && !!r.querySelector('.retOld') && r.querySelector('.retOld').textContent === 'ушёл, не рассчитавшись',
      labels: r ? [...r.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) : [] }; }, GUEST);
  await z.sleep(300);
  await z.shot(ph, 'z11_debt_phone');
  z.check('телефон 360: «Дебиторка сейчас» — Guest41 карточкой с подписями («должен · что · с какого · где»), «ушёл, не рассчитавшись» оранжевым; без горизонтальной прокрутки',
    card.sw <= 360 && card.fit && card.kfit && card.gone && card.labels.join('|') === 'должен|что|с какого|где', JSON.stringify(card));
  await ph.evaluate(id => openUserDrawer(id, 'debt'), G); await z.sleep(2000);
  const pf = await ph.evaluate(() => { const b = document.getElementById('woBtn'); if (b) b.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, fit: !!b && b.getBoundingClientRect().right <= innerWidth }; });
  await z.sleep(300);
  await z.shot(ph, 'z11_debt_phone_card');
  z.check('телефон 360: в карточке гостя «Списать долг» помещается, без горизонтальной прокрутки', pf.sw <= 360 && pf.fit, JSON.stringify(pf));
  await z.done();
})().catch(z.crash);
