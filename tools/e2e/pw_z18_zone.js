// З18-и1 «Удаление зоны с живыми минутами» (№8, З-Р7), браузер: «Экономика → Зоны». Временная зона «z18 Окно» по 10 zł/ч,
// у Guest48 — 30 мин за монеты, у Guest49 — 15 (API). В списке — «У гостей: 45 мин · 2 гостя», ✕ открывает шторку зоны
// на «Удалить зону»: «У 2 гостей 45 мин этой зоны …», выбор зоны «минута в минуту», «Перенести 45 мин» и «Сжечь».
// Перенос в Standard — тост, мастер сменяется на «Зону ничто не держит» и «Удалить зону» (два нажатия) — зоны нет.
// Снова «z18 Окно» с 20 мин Guest49 — «Сжечь 20 мин» (два нажатия) — остаток 0, удаление. У Standard в шторке — «В зоне
// 9 ПК» и «продавали пакеты … только переименовать», кнопки удаления нет. PWA гостей — уведомления «⏱ 30 мин зоны
// «z18 Окно» теперь в зоне «Standard»» и «🔥 Зону «z18 Окно» закрыли — сгорело 20 мин этой зоны»; киоск и PWA — тексты
// RU/EN/PL; APP_VER поднят парой с манифестом. З18-и1а: в «Отчёты → Деньги» плитка «— минуты» «Сгорело за период» и строка
// «Минуты зоны «z18 Окно»» Guest49 — 20 мин, 3,33 zł. Телефон 360 — шторка с мастером без прокрутки вбок. В конце зоны
// «z18 …», выдачи и погашения Guest48 и Guest49 удаляются (и при падении тоже).
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const G1 = 'Guest48', G2 = 'Guest49', U1 = z.uid(G1), U2 = z.uid(G2), ZN = 'z18 Окно';
  const tidy = () => {
    z.sql(`DELETE FROM minute_grants WHERE user_id IN ('${U1}', '${U2}')`);
    z.sql(`DELETE FROM coin_redemptions WHERE user_id IN ('${U1}', '${U2}')`);
    z.sql(`DELETE FROM zones WHERE name = '${ZN}'`);
  };
  tidy();
  z.atEnd(tidy);
  z.sql(`UPDATE users SET coins_balance = 1000, coin_minutes = 0 WHERE id IN ('${U1}', '${U2}')`);
  z.sql(`UPDATE notifications SET read_at = now(), read_kiosk_at = now(), read_app_at = now() WHERE user_id IN ('${U1}', '${U2}') AND read_at IS NULL`);
  const STD = z.sql("SELECT id FROM zones WHERE name = 'Standard'");
  const grants = u => z.rows(`SELECT zone_name || ':' || minutes_left FROM minute_grants WHERE user_id = '${u}' ORDER BY created_at`).join(',');
  const mkZone = async (mins) => {
    const zid = ((await z.api('POST', '/admin/zones', { name: ZN, rate_pln: 10 }, 'boss')).data.zone || {}).id;
    const r = [];
    for (const [u, m] of mins) r.push((await z.api('POST', `/admin/users/${u}/redeem`, { zone_id: zid, minutes: m }, 'admin1')).status);
    return { zid, r };
  };
  const a = await mkZone([[U1, 30], [U2, 15]]);
  z.check(`«${ZN}» по 10 zł/ч: у Guest48 — 30 мин, у Guest49 — 15 (API)`, !!a.zid && a.r.join() === '200,200'
    && grants(U1) === `${ZN}:30` && grants(U2) === `${ZN}:15`, `${a.zid} ${a.r} · ${grants(U1)} · ${grants(U2)}`);

  const page = await z.openAdmin('boss');
  await z.nav(page, 'settings');
  await page.waitForFunction(n => [...document.querySelectorAll('#zonesTable tr')].some(t => t.cells[0] && t.cells[0].innerText.trim() === n), ZN,
    { timeout: 10000 }).catch(() => {});
  const row = n => page.evaluate(n => { const t = document.querySelector('#zonesTable table'); if (!t) return null;
    const head = [...t.rows[0].cells].map(c => c.textContent.trim()), r = [...t.rows].find(r => r.cells[0] && r.cells[0].innerText.trim() === n);
    if (!r) return null;
    const x = r.querySelector('button.danger'), i = head.indexOf('У гостей');
    return { head, live: i >= 0 && r.cells[i] ? r.cells[i].innerText.replace(/\s+/g, ' ').trim() : null, x: x ? x.title || 'arm' : null }; }, n);
  const click = async sel => { if (await page.$(sel)) await page.click(sel); }; // нет кнопки (HEAD) — проверка ниже скажет FAIL
  const r1 = await row(ZN), rs = await row('Standard');
  z.check('список зон: колонка «У гостей» — «45 мин · 2 гостя»; у «z18 Окно» ✕ ведёт к мастеру, у Standard (ПК, продажи) ✕ нет',
    !!r1 && r1.head.join('|') === 'Зона|Цена часа|VAT|Час в монетах|Компьютеров|У гостей|' && r1.live === '45 мин · 2 гостя'
    && /сначала перенеси или сожги/.test(r1.x || '') && !!rs && rs.live === '—' && rs.x === null, JSON.stringify([r1, rs]));
  await page.evaluate(n => { const r = [...document.querySelectorAll('#zonesTable tr')].find(t => t.cells[0] && t.cells[0].innerText.trim() === n);
    const x = r && r.querySelector('button.danger'); if (x) x.click(); }, ZN);
  await z.sleep(600);
  const del = () => page.evaluate(() => { const b = document.getElementById('zn_del'); if (!b) return null;
    const to = document.getElementById('zn_to');
    return { title: document.getElementById('drawerTitle').textContent, text: b.innerText.replace(/\s+/g, ' ').trim(),
      opts: to ? [...to.options].map(o => o.textContent) : [], move: (document.getElementById('zn_move') || {}).textContent || '',
      burn: (document.getElementById('zn_burn') || {}).textContent || '', drop: !!document.getElementById('zn_drop'),
      seen: (() => { const h = document.getElementById('zn_delh'), r = h && h.getBoundingClientRect(); return !!r && r.top >= 0 && r.top < innerHeight; })() }; });
  const d1 = await del();
  z.check('✕ — шторка «Зона · z18 Окно» сразу на «Удалить зону»: «У 2 гостей 45 мин этой зоны …», зоны «минута в минуту», '
    + '«Перенести 45 мин» и «Сжечь 45 мин», «Удалить зону» пока нет',
    !!d1 && d1.title === 'Зона · ' + ZN && d1.seen && d1.text.startsWith('У 2 гостей 45 мин этой зоны — выданы за монеты или в компенсацию')
    && /минута в минуту \(час здесь — 10,00 zł\)/.test(d1.text) && JSON.stringify(d1.opts) === JSON.stringify(['Standard · 20,00 zł/ч', 'VIP · 30,00 zł/ч'])
    && d1.move.trim() === 'Перенести 45 мин' && d1.burn.trim() === 'Сжечь 45 мин' && !d1.drop, JSON.stringify(d1));
  await z.shot(page, 'z18_zone_wizard');
  if (await page.$('#zn_to')) await page.selectOption('#zn_to', STD);
  await click('#zn_move');
  await z.sleep(1500);
  const t1 = await z.text(page, '#toast'), d2 = await del(), r2 = await row(ZN);
  z.check('«Перенести» в Standard: тост «45 мин перенесены в «Standard» — гостям ушло уведомление»; в шторке — «Зону ничто '
    + 'не держит» и «Удалить зону»; в списке — «—»; у гостей — Standard:30 и Standard:15',
    /45 мин перенесены в «Standard» — гостям ушло уведомление/.test(t1 || '') && !!d2 && d2.text.startsWith('Зону ничто не держит')
    && d2.drop && !!r2 && r2.live === '—' && grants(U1) === 'Standard:30' && grants(U2) === 'Standard:15',
    JSON.stringify({ t1, d2: d2 && d2.text, r2, g: [grants(U1), grants(U2)] }));
  await click('#zn_drop'); await z.sleep(300);
  const armed = await page.evaluate(() => (document.getElementById('zn_drop') || {}).textContent || '');
  await click('#zn_drop'); await z.sleep(1500);
  const t2 = await z.text(page, '#toast'), open = await page.evaluate(() => document.getElementById('drawer').classList.contains('open'));
  z.check('«Удалить зону» — с подтверждением («Удалить? Ещё раз»), второе нажатие — «Зона удалена», шторка закрыта, в списке зоны нет',
    /Удалить\? Ещё раз/.test(armed) && /Зона удалена/.test(t2 || '') && !open && (await row(ZN)) === null
    && z.sql(`SELECT count(*) FROM zones WHERE name = '${ZN}'`) === '0', JSON.stringify({ armed, t2, open }));

  const b = await mkZone([[U2, 20]]);
  await page.evaluate(async id => { await renderZones(); zoneEdit(id, true); }, b.zid);
  await z.sleep(600);
  const d3 = await del();
  await click('#zn_burn'); await z.sleep(300);
  const armB = await page.evaluate(() => (document.getElementById('zn_burn') || {}).textContent || '');
  await click('#zn_burn'); await z.sleep(1500);
  const t3 = await z.text(page, '#toast'), d4 = await del();
  z.check('снова «z18 Окно» с 20 мин Guest49: «Сжечь 20 мин» — с подтверждением, тост «Сожжено 20 мин у 1 гостя — гостям ушло '
    + 'уведомление», остаток 0, в шторке — «Удалить зону»',
    !!d3 && d3.text.startsWith('У 1 гостя 20 мин этой зоны') && /Сжечь 20 мин\? Ещё раз/.test(armB)
    && /Сожжено 20 мин у 1 гостя — гостям ушло уведомление/.test(t3 || '') && grants(U2) === `Standard:15,${ZN}:0` && !!d4 && d4.drop,
    JSON.stringify({ d3: d3 && d3.text.slice(0, 60), armB, t3, g: grants(U2) }));
  await click('#zn_drop'); await z.sleep(300); await click('#zn_drop'); await z.sleep(1500);
  await page.evaluate(id => zoneEdit(id), STD);
  await z.sleep(500);
  const d5 = await del();
  z.check('шторка Standard: «В зоне 9 ПК — сначала переведи их …» и «По зоне продавали пакеты (…) — … можно только переименовать», '
    + 'мастера и «Удалить зону» нет; «z18 Окно» удалена',
    !!d5 && /^В зоне 9 ПК — сначала переведи их в другую зону/.test(d5.text) && /По зоне продавали пакеты \(\d+\) — в истории продаж она нужна: удалить нельзя, можно только переименовать/.test(d5.text)
    && !d5.drop && !d5.move && !d5.burn && z.sql(`SELECT count(*) FROM zones WHERE name = '${ZN}'`) === '0', JSON.stringify(d5));
  const aud = await page.evaluate(() => [AUD_LABEL.zone_minutes_move, AUD_LABEL.zone_minutes_burn, AUD_GROUPS.money.includes('zone_minutes_burn'),
    AUD_GROUPS.pc.includes('zone_minutes_move')]);
  z.check('журнал: «зона: минуты перенесены», «зона: минуты сожжены» — в «Деньгах» и «ПК и зонах»',
    JSON.stringify(aud) === JSON.stringify(['зона: минуты перенесены', 'зона: минуты сожжены', true, true]), JSON.stringify(aud));
  await page.evaluate(() => closeDrawer());

  // З18-и1а: сожжённое — в «Сгорело за период» «Денег»: плитка «— минуты» и строка в списке
  await z.nav(page, 'stats');
  await page.waitForFunction(() => !!document.getElementById('repLiab'), null, { timeout: 15000 }).catch(() => {});
  await z.sleep(800);
  const rep = (await z.api('GET', '/admin/reports/money?' + await page.evaluate(() => repQuery()), undefined, 'boss')).data;
  const BM = (rep.burned || {}).minutes || {};
  const fmt = v => (Math.round((+v || 0) * 100) / 100).toLocaleString('ru-RU', { minimumFractionDigits: 2, maximumFractionDigits: 2 }) + ' zł';
  const sb = await page.evaluate(g => {
    const tile = [...document.querySelectorAll('#repLiab .kpi')].find(k => k.firstChild.textContent.trim() === '— минуты');
    const s = document.getElementById('repBurnList'), tb = s && s.nextElementSibling && s.nextElementSibling.nextElementSibling
      && s.nextElementSibling.nextElementSibling.querySelector('table');
    const rows = tb ? [...tb.rows].slice(1).map(r => [...r.cells].map(c => [...c.childNodes]
      .filter(x => !(x.classList && x.classList.contains('repL'))).map(x => x.textContent).join('').replace(/\s+/g, ' ').trim())) : [];
    if (s) s.scrollIntoView({ block: 'start' });
    return { tile: tile ? tile.querySelector('b').innerText.replace(/\s+/g, ' ').trim() : null, title: tile ? tile.title : '',
      row: rows.find(r => r[1] === g && /^Минуты зоны/.test(r[2])) || null }; }, G2);
  await z.sleep(300);
  await z.shot(page, 'z18_zone_burned');
  z.check(`«Деньги»: в ряду «Сгорело за период» — «— минуты» ${fmt(BM.pln)} («сожжены при удалении зоны, по тарифу выдачи»); в списке `
    + `сгоревшего — Guest49 «Минуты зоны «${ZN}»», 20 мин, 3,33 zł`,
    BM.count > 0 && sb.tile === fmt(BM.pln) && /сожжены при удалении зоны, по тарифу выдачи/.test(sb.title)
    && !!sb.row && sb.row[2] === `Минуты зоны «${ZN}»` && sb.row[3] === '20 мин' && sb.row[4] === '3,33 zł', JSON.stringify({ BM, sb }));

  // гость: уведомления в PWA — настоящие (move и burn выше), тексты киоска и PWA — RU/EN/PL
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const pwa = async nick => {
    const p = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })).newPage();
    p.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
    await p.goto(z.BASE + '/app', { waitUntil: 'load' });
    await p.fill('#nick', nick); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
    return p;
  };
  const MOV1 = `⏱ 30 мин зоны «${ZN}» теперь в зоне «Standard»`, MOV2 = `⏱ 15 мин зоны «${ZN}» теперь в зоне «Standard»`;
  const BURN = `🔥 Зону «${ZN}» закрыли — сгорело 20 мин этой зоны`;
  const feed = (p, ws) => p.waitForFunction(ws => { const f = document.getElementById('ntfFeed'); return !!f && ws.every(x => f.textContent.includes(x)); },
    ws, { timeout: 20000 }).then(() => true).catch(() => false);
  const a1 = await pwa(G1), f1 = await feed(a1, [MOV1]);
  await z.sleep(2500);
  const a2 = await pwa(G2), f2 = await feed(a2, [MOV2, BURN]);
  z.check(`PWA Guest48 — «${MOV1}»; Guest49 — «${MOV2}» и «${BURN}»`, f1 && f2,
    JSON.stringify(await Promise.all([a1, a2].map(p => p.evaluate(() => (document.getElementById('ntfFeed') || {}).textContent || '')))).slice(0, 400));
  await z.shot(a2, 'z18_zone_pwa');
  // словари — const страницы, не свойства window: код подсказки — строкой, имена разрешаются в её области видимости
  const texts = (p, fn, [RU, EN, PL]) => p.evaluate(`(() => {
    const n = (type, payload) => ${fn}({ type, payload });
    const one = () => [n('zone_minutes_moved', { minutes: 30, from: 'Tmp', to: 'Standard' }), n('zone_minutes_burned', { minutes: 20, zone: 'Tmp' })];
    const save = STR, out = {};
    STR = Object.assign({}, ${RU}); out.ru = one();
    STR = Object.assign({}, ${RU}, ${EN}); out.en = one();
    STR = Object.assign({}, ${RU}, ${PL}); out.pl = one();
    STR = save;
    return out;
  })()`);
  const WANT = { ru: ['⏱ 30 мин зоны «Tmp» теперь в зоне «Standard»', '🔥 Зону «Tmp» закрыли — сгорело 20 мин этой зоны'],
    en: ['⏱ 30 min of zone “Tmp” are now in zone “Standard”', '🔥 Zone “Tmp” was closed — 20 min of that zone are gone'],
    pl: ['⏱ 30 min ze strefy „Tmp” jest teraz w strefie „Standard”', '🔥 Strefę „Tmp” zamknięto — przepadło 20 min z tej strefy'] };
  const tp = await texts(a1, 'ntfText', ['STR_RU', 'STR_EN', 'STR_PL']);
  const k = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell?pc=' + z.sql("SELECT id FROM computers WHERE name = 'PC-03'"), { waitUntil: 'load' });
  await z.sleep(800);
  const tk = await texts(k, 'notifText', ['SH_RU', 'SH_EN', 'SH_PL']);
  z.check('тексты RU/EN/PL — в PWA и в киоске одинаковые: перенос и сгорание', JSON.stringify(tp) === JSON.stringify(WANT)
    && JSON.stringify(tk) === JSON.stringify(WANT), JSON.stringify({ tp, tk }));
  const ver = await a1.evaluate(async () => ({ app: APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  z.check('APP_VER в app.html поднят и равен app_version манифеста (иначе телефоны залипнут на старой версии)',
    ver.app === ver.mf && ver.app !== '2026-11-01.2', JSON.stringify(ver));

  // телефон: шторка с мастером в ширину экрана
  const c = await mkZone([[U1, 10]]);
  await z.sleep(2500); // лимитер входа с одного адреса
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 800 } });
  await z.nav(ph, 'settings');
  await ph.evaluate(async id => { await renderZones(); zoneEdit(id, true); }, c.zid);
  await z.sleep(800);
  const fit = await ph.evaluate(() => { const ids = ['zn_to', 'zn_move', 'zn_burn'].map(i => document.getElementById(i));
    return { ok: ids.every(e => !!e && e.getBoundingClientRect().right <= innerWidth + 0.5 && e.getBoundingClientRect().left >= 0),
      over: document.getElementById('drawerBody').scrollWidth - document.getElementById('drawerBody').clientWidth }; });
  await z.shot(ph, 'z18_zone_phone');
  z.check('телефон 360: в шторке выбор зоны, «Перенести» и «Сжечь» — в ширину экрана, вбок не прокручивается', fit.ok && fit.over <= 0, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
