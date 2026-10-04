// З12-и1 «Обязательства клуба и сгоревшее», браузер (№49; З-Р9; решение основателя 03.10 — сгоревшие монеты тоже).
// Владелец: «Деньги» — ряд «Обязательства клуба · Сгорело за период · — пакеты · — монеты» над кошельками и строка
// состава — как в отчёте сервера; раздел «Обязательства клуба сейчас» — семь строк «Что · Сколько · Как считаем»;
// «Сгорело за период» — просроченный пакет Guest44 и его сгоревшие монеты, строка открывает карточку гостя; палитра
// «обязательства» → ряд на экране. «Бухгалтерия → Касса» — тот же ряд под ящиком (за период книги) и ссылка «состав и
// что сгорело — в «Деньгах»». Телефон 360 — ряды и строки карточками с подписями, без горизонтальной прокрутки.
// Данные: гость Guest44 — пакет (самый дешёвый, картой) с вышедшим сроком и 120 сгоревших монет (строка coin_burns
// SQL-ом, как её пишет прогон таяния; метка idle_days = 0, pct = 0); в конце пакет отменяется, строка сгорания
// удаляется, монеты возвращаются.
const z = require('./_z');
(async () => {
  const GUEST = 'Guest44', G = z.uid(GUEST), BURN = 120;
  const gone = async () => {
    for (const pid of z.rows(`SELECT id FROM user_packages WHERE user_id = '${G}' AND voided_at IS NULL`))
      await z.api('POST', `/admin/user-packages/${pid}/void`, { refund_method: 'none' }, 'boss');
    for (const r of z.rows(`SELECT id, coins FROM coin_burns WHERE user_id = '${G}' AND idle_days = 0 AND pct = 0`)) {
      const [bid, n] = r.split('|');
      z.sql(`DELETE FROM coin_burns WHERE id = '${bid}'; UPDATE users SET coins_balance = coins_balance + ${n} WHERE id = '${G}'`);
    }
  };
  await gone(); z.atEnd(gone);
  z.sql(`UPDATE users SET coins_balance = GREATEST(coins_balance, ${BURN}) WHERE id = '${G}'`);
  const pack = z.sql(`SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1`);
  const price = +z.sql(`SELECT price_pln FROM time_packages WHERE id = '${pack}'`), pname = z.sql(`SELECT name FROM time_packages WHERE id = '${pack}'`);
  const up = ((await z.api('POST', `/admin/users/${G}/packages`, { package_id: pack, method: 'card' }, 'boss')).data.package || {}).id;
  z.sql(`UPDATE user_packages SET expires_at = now() - interval '1 second' WHERE id = '${up}'`);
  const left = +z.sql(`SELECT coins_balance FROM users WHERE id = '${G}'`);
  z.sql(`UPDATE users SET coins_balance = coins_balance - ${BURN} WHERE id = '${G}';
    INSERT INTO coin_burns (user_id, coins, balance_after, idle_days, pct, manual) VALUES ('${G}', ${BURN}, ${left - BURN}, 0, 0, true)`);
  if (!z.check(`данные: у Guest44 вышел срок пакета «${pname}» (${price} zł) и сгорело ${BURN} монет`, !!up, up)) return z.done();

  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const num = v => norm((+v || 0).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }));
  const fmt = v => num(v) + ' zł';
  const tap = (p, sel) => p.click(sel, { timeout: 4000 }).catch(() => {}); // нового на HEAD нет — проверка упадёт, прогон нет
  const kpis = (p, id) => p.evaluate(id => Object.fromEntries([...document.querySelectorAll('#' + id + ' .kpi')].map(k =>
    [k.firstChild.textContent.trim(), k.querySelector('b').innerText.replace(/\s+/g, ' ').trim()])), id);
  const parts = l => `кошельки ${num((l.wallets || {}).pln)} + пакеты ${num((l.packages || {}).pln)} + монеты ${num((l.coins || {}).pln)} + минуты ${
    num((l.minutes || {}).pln)} − дебиторка ${num((l.receivable || {}).pln)} zł`;

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'hall');
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'обязательства'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent));
  await boss.keyboard.press('Enter'); await z.sleep(3000);
  const rep = (await z.api('GET', '/admin/reports/money?' + await boss.evaluate(() => repQuery()), undefined, 'boss')).data;
  const L = rep.liabilities || {}, B = rep.burned || {};
  const row = await boss.evaluate(() => { const e = document.getElementById('repLiab'), r = e ? e.getBoundingClientRect() : { top: -1, bottom: 1e9 };
    const ln = e && e.nextElementSibling;
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight, line: ln ? ln.textContent.replace(/\s+/g, ' ').trim() : '' }; });
  const k1 = await kpis(boss, 'repLiab');
  z.check('палитра «обязательства» → «Обязательства клуба», Enter — «Деньги», ряд на экране; «Обязательства клуба», «Сгорело за период», '
    + '«— пакеты», «— монеты» и строка состава — как в отчёте сервера',
    items[0] === 'Обязательства клуба' && row.tab === 'stats' && row.rep === 'money' && row.seen && !!rep.liabilities
    && k1['Обязательства клуба'] === fmt(L.net_pln) && k1['Сгорело за период'] === fmt(B.pln)
    && k1['— пакеты'] === fmt((B.packages || {}).pln) && k1['— монеты'] === fmt((B.coins || {}).pln) && row.line === parts(L),
    JSON.stringify({ items: items.slice(0, 2), row, k1, net: L.net_pln, burned: B.pln }));
  await z.shot(boss, 'z12_liab_row');

  const sect = await boss.evaluate(() => { const s = document.getElementById('repLiabList'), tb = s && s.nextElementSibling && s.nextElementSibling.nextElementSibling
    && s.nextElementSibling.nextElementSibling.querySelector('table');
    if (s) s.scrollIntoView({ block: 'start' });
    return tb ? [...tb.rows].slice(1).map(r => [...r.cells].slice(0, 2).map(c => [...c.childNodes].filter(x => !(x.classList && x.classList.contains('repL')))
      .map(x => x.textContent).join('').replace(/\s+/g, ' ').trim())) : []; });
  const rc = (L.receivable || {}).pln || 0;
  const want = [['Кошельки гостей', fmt((L.wallets || {}).pln)], ['Пакеты времени', fmt((L.packages || {}).pln)], ['Монеты на руках', fmt((L.coins || {}).pln)],
    ['Минуты за монеты и компенсации', fmt((L.minutes || {}).pln)], ['Клуб должен гостям', fmt(L.owed_pln)], ['Минус дебиторка', (rc ? '−' : '') + fmt(rc)],
    ['Обязательства клуба', fmt(L.net_pln)]];
  await z.sleep(300);
  z.check('«Обязательства клуба сейчас» — семь строк: кошельки, пакеты, монеты, минуты, «Клуб должен гостям», «Минус дебиторка», '
    + '«Обязательства клуба» — суммы как в отчёте сервера', JSON.stringify(sect) === JSON.stringify(want), JSON.stringify({ sect, want }));
  await z.shot(boss, 'z12_liab_sect');

  const burn = await boss.evaluate(() => { const s = document.getElementById('repBurnList'), tb = s && s.nextElementSibling && s.nextElementSibling.nextElementSibling
    && s.nextElementSibling.nextElementSibling.querySelector('table');
    if (s) s.scrollIntoView({ block: 'start' });
    return { head: tb ? [...tb.rows[0].cells].map(c => c.textContent) : [], rows: tb ? [...tb.rows].slice(1).map(r => [...r.cells].map(c => [...c.childNodes]
      .filter(x => !(x.classList && x.classList.contains('repL'))).map(x => x.textContent).join('').replace(/\s+/g, ' ').trim())) : [] }; });
  const bp = burn.rows.find(r => r[1] === GUEST && r[2] === `Пакет «${pname}»`), bc = burn.rows.find(r => r[1] === GUEST && r[2] === 'Монеты');
  z.check(`«Сгорело за период» — «Когда · Гость · Что · Сколько · Сгорело на»: пакет Guest44 — 3 ч, ${fmt(price)}; монеты Guest44 — ${BURN} монет `
    + `по курсу погашения`,
    burn.head.join('|') === 'Когда|Гость|Что|Сколько|Сгорело на' && bp && bp[3] === '3 ч' && bp[4] === fmt(price)
    && bc && bc[3] === `${BURN} монет` && bc[4] === fmt(BURN / ((B.coins || {}).rate || 20)), JSON.stringify({ head: burn.head, bp, bc }));
  await z.sleep(300);
  await z.shot(boss, 'z12_liab_burned');
  await tap(boss, `#repBurnList + .muted + .tablewrap tr:has-text("Пакет «${pname}»"):has-text("${GUEST}")`); await z.sleep(1500);
  const dt = norm(await z.text(boss, '#drawerTitle'));
  z.check('строка сгоревшего открывает карточку гостя', dt.includes(GUEST), dt);
  await boss.evaluate(() => closeDrawer());

  await boss.evaluate(() => accGo('cash')); await z.sleep(2500); // З12-и4: вход в «Бухгалтерию» — «Обзор», ряд — в «Кассе»
  const book = (await z.api('GET', '/admin/cash/book?' + await boss.evaluate(() => accQuery()), undefined, 'boss')).data;
  const BL = book.liabilities || {}, BB = book.burned || {};
  const k2 = await kpis(boss, 'accLiab');
  const acc = await boss.evaluate(() => { const e = document.getElementById('accLiab'), ln = e && e.nextElementSibling;
    return { line: ln ? ln.textContent.replace(/\s+/g, ' ').trim() : '', link: !!(ln && ln.querySelector('a.lnk')) }; });
  z.check('«Бухгалтерия → Касса»: под ящиком тот же ряд — «Обязательства клуба» и «Сгорело за период» за период книги — как сервер; '
    + 'строка состава и ссылка «состав и что сгорело — в «Деньгах»»',
    !!book.liabilities && k2['Обязательства клуба'] === fmt(BL.net_pln) && k2['Сгорело за период'] === fmt(BB.pln)
    && acc.line === parts(BL) + ' · состав и что сгорело — в «Деньгах»' && acc.link, JSON.stringify({ k2, acc, net: BL.net_pln, burned: BB.pln }));
  await z.shot(boss, 'z12_liab_acc');
  await tap(boss, '#accLiab + .muted a.lnk'); await z.sleep(3000);
  const back = await boss.evaluate(() => { const e = document.getElementById('repLiab'), r = e ? e.getBoundingClientRect() : { top: -1, bottom: 1e9 };
    return { tab: curTab, rep: repTab, seen: r.top >= 0 && r.bottom <= innerHeight }; });
  z.check('ссылка из «Кассы» — «Деньги», ряд «Обязательства клуба» на экране', back.tab === 'stats' && back.rep === 'money' && back.seen, JSON.stringify(back));

  await boss.goto('about:blank'); await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await ph.evaluate(() => typeof palGoLiab === 'function' ? palGoLiab() : palGoReport('money')); await z.sleep(3500);
  const pr = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth,
    kfit: [...document.querySelectorAll('#repLiab .kpi')].length > 0 && [...document.querySelectorAll('#repLiab .kpi')].every(k => k.getBoundingClientRect().right <= innerWidth) }));
  await z.shot(ph, 'z12_liab_phone_row');
  const pc = await ph.evaluate(name => { const s = document.getElementById('repLiabList'), wrap = s && s.nextElementSibling && s.nextElementSibling.nextElementSibling;
    const r = wrap ? [...wrap.querySelectorAll('tr')].find(t => t.innerText.includes('Монеты на руках')) : null;
    const b = document.getElementById('repBurnList'), bw = b && b.nextElementSibling && b.nextElementSibling.nextElementSibling;
    const br = bw ? [...bw.querySelectorAll('tr')].find(t => t.innerText.includes(name)) : null;
    if (r) r.scrollIntoView({ block: 'center' });
    const lab = x => x ? [...x.querySelectorAll('.repL')].filter(l => getComputedStyle(l).display !== 'none').map(l => l.textContent) : [];
    return { sw: document.documentElement.scrollWidth, fit: !!r && r.getBoundingClientRect().right <= innerWidth, labels: lab(r), blabels: lab(br),
      bfit: !!br && br.getBoundingClientRect().right <= innerWidth }; }, `Пакет «${pname}»`);
  await z.sleep(300);
  await z.shot(ph, 'z12_liab_phone_sect');
  z.check('телефон 360: ряд «Обязательства клуба» помещается; «Обязательства клуба сейчас» и «Сгорело за период» — карточки с подписями '
    + '(«сколько · как считаем», «когда · что · сколько · сгорело на»), без горизонтальной прокрутки',
    pr.sw <= 360 && pr.kfit && pc.sw <= 360 && pc.fit && pc.bfit && pc.labels.join('|') === 'сколько|как считаем'
    && pc.blabels.join('|') === 'когда|что|сколько|сгорело на', JSON.stringify({ pr, pc }));
  await ph.evaluate(() => accGo('cash')); await z.sleep(2500);
  const pa = await ph.evaluate(() => { const e = document.getElementById('accLiab'); if (e) e.scrollIntoView({ block: 'center' });
    return { sw: document.documentElement.scrollWidth, kfit: !!e && [...e.querySelectorAll('.kpi')].every(k => k.getBoundingClientRect().right <= innerWidth) }; });
  await z.sleep(300);
  await z.shot(ph, 'z12_liab_phone_acc');
  z.check('телефон 360: «Бухгалтерия → Касса» — ряд обязательств помещается, без горизонтальной прокрутки', pa.sw <= 360 && pa.kfit, JSON.stringify(pa));
  await z.done();
})().catch(z.crash);
