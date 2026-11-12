// К9-и3 «Пакет в карточке» (KIOSK.md, этап VII), браузер. Киоск PC-07 (Standard, 20 zł/ч), гость Guest46, сессия идёт
// 20 минут, в кошельке около 83 zł. В каталоге, кроме стендовых пакетов, четыре ловушки «k93 …» (в конце удаляются):
// чужая зона (час по 6), окно списания (час по 7), выключенный (час по 8), час не дешевле тарифа (по 20).
// 1. После входа строка появляется сама, вкладку не открывали: «5 часов Standard — 75 zł», «час по 15 zł вместо 20»;
//    цена часа = цена пакета / часы, посчитана здесь от базы. 2. Пороги (кошелёк и цена сессии подставлены на
//    странице одним действием, каталог настоящий): 49,99 — строки нет; 50 — «3 часа Standard»; 74,99 и 75; скидка
//    гостя (час 16 — «вместо 16», «3 часа» с часом 16,67 отпадает; час 15 — пакет с часом 15 не дешевле); зона по id
//    и по имени; уже играет по пакету; красное состояние (денег на 13 минут при дешёвом пакете; чужая бронь через
//    9 минут) — строки нет. 3. Подарили пакет — строка ушла, отменили — вернулась. 4. Имя в 64 знака и цена 1439,99
//    на трёх ширинах и трёх языках: цена видна, «Завершить» на месте, ничего не вылезает. 5. Нажатие открывает
//    вкладку «Пакеты»: подтверждения нет, сессия идёт. 6. Покупка во вкладке (в кошельке 77 zł, после покупки —
//    на 6 минут): подтверждение спрошено, строка исчезла сразу, карточка ни разу не покраснела, остаток не
//    проваливался. Купленное отменяется через админку, кошелёк гостя возвращается.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07', NOTE = 'k93 проверка';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const zs = z.sql("SELECT id FROM zones WHERE name='Standard'"), zv = z.sql("SELECT id FROM zones WHERE name='VIP'");
  const pack = name => { const [id, min, gr] = z.sql(`SELECT id || '|' || minutes || '|' || round(price_pln * 100) FROM time_packages WHERE name='${name}'`).split('|');
    return { id, name, min: +min, gr: +gr }; };
  const P3 = pack('3 часа Standard'), P5 = pack('5 часов Standard');
  const fmt = v => { const n = Math.round(v * 100) / 100; return (Number.isInteger(n) ? String(n) : n.toFixed(2)).replace('.', ','); };
  const hourOf = k => fmt(Math.round(k.gr * 60 / k.min) / 100); // цена пакета / часы, до гроша
  const line1 = k => `${k.name} — ${fmt(k.gr / 100)} zł`, line2 = (k, rate) => `час по ${hourOf(k)} zł вместо ${fmt(rate)}`;

  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const before = z.rows(`SELECT id FROM user_packages WHERE user_id='${g}'`);
  const mine = () => z.rows(`SELECT id FROM user_packages WHERE user_id='${g}' AND voided_at IS NULL`).filter(id => !before.includes(id));
  const voidMine = async () => { for (const id of mine()) await z.api('POST', `/admin/user-packages/${id}/void`, {}, 'boss'); };
  const dropTraps = () => { try { z.sql("DELETE FROM time_packages WHERE name LIKE 'k93 %'"); } catch (e) { console.log('ловушки не удалились:', e.message.split('\n')[0]); } };
  const [w0, cm0, pct0] = z.sql(`SELECT wallet_grosz || '|' || coin_minutes || '|' || payment_increase_pct FROM users WHERE id='${g}'`).split('|');
  // Кошелёк проверка выставляет мимо сервера, а стенд трека З держит правило «кошелёк гостя = сумма его журнала»
  // (e2e_z0_stand). Поэтому выставление — один запрос: кошелёк и строка журнала на ту же разницу (как посев стенда,
  // seed_stand.py); в конце проверка сама сверяет, что расхождение кошелька с журналом осталось каким было
  const setWallet = v => z.sql(`WITH o AS (SELECT id, wallet_grosz AS w FROM users WHERE id='${g}' FOR UPDATE),
      u AS (UPDATE users SET wallet_grosz = ${v} FROM o WHERE users.id = o.id RETURNING o.w)
    INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, note, created_at)
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k9_pack: проверка выставила кошелёк', now() FROM u WHERE u.w <> ${v}`);
  const gap = () => z.sql('SELECT u.wallet_grosz - COALESCE((SELECT sum(amount_grosz) FROM wallet_transactions w WHERE w.user_id = u.id), 0) '
    + `FROM users u WHERE u.id='${g}'`);
  const gap0 = gap();
  z.atEnd(async () => {
    await endTails(); await voidMine(); dropTraps();
    setWallet(+w0);
    z.sql(`UPDATE users SET coin_minutes=${cm0}, payment_increase_pct=${pct0} WHERE id='${g}'`);
    z.check('проверка убрала за собой: кошелёк гостя сходится со своим журналом так же, как до неё', gap() === gap0, `расхождение ${gap0} → ${gap()} гр`);
  });
  await endTails(); dropTraps();
  setWallet(9000);
  z.sql(`UPDATE users SET coin_minutes=0, payment_increase_pct=0 WHERE id='${g}'`);

  // Ловушки: ни один из этих пакетов в строку попасть не должен, хотя час по каждому из первых трёх дешевле, чем по стендовым
  const TRAP_ZONE = { name: 'k93 чужая зона', min: 600, gr: 6000 };
  const traps = [
    { name: TRAP_ZONE.name, zone_id: zv, minutes: 600, price_pln: 60 },
    { name: 'k93 окно', zone_id: zs, minutes: 600, price_pln: 70, spend_days_mask: 127, spend_start_min: 0, spend_end_min: 1380 },
    { name: 'k93 выключен', zone_id: zs, minutes: 600, price_pln: 80, active: false },
    { name: 'k93 час не дешевле', zone_id: zs, minutes: 60, price_pln: 20 },
  ];
  const made = [];
  for (const t of traps) {
    const r = await z.api('POST', '/admin/packages', Object.assign({ days_valid: 30, sort: 90, active: true }, t), 'boss');
    made.push(r.status); if (r.status !== 201) console.log('ловушка', t.name, r.status, J(r.data));
  }
  z.check('ловушки заведены: чужая зона, окно списания, выключенный, час не дешевле тарифа', made.every(s => s === 201), J(made));

  const BACK = '20 minutes 10 seconds';
  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  const sid = seat.data && seat.data.session_id;
  if (sid) z.sql(`UPDATE sessions SET started_at = now() - interval '${BACK}', ready_at = now() - interval '${BACK}' WHERE id='${sid}'`);
  const billed = () => (sid ? +z.sql(`SELECT billed_minutes FROM sessions WHERE id='${sid}'`) : -1);
  for (let i = 0; i < 50 && sid && billed() < 20; i++) await z.sleep(1000);
  const sessActive = () => !!sid && z.sql(`SELECT status FROM sessions WHERE id='${sid}'`) === 'active';

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const AGENT_OK = { 'Access-Control-Allow-Origin': '*' };
  const ctx = await br.newContext({ viewport: { width: 1920, height: 1080 } });
  await ctx.route('http://127.0.0.1:1448/**', rt => {
    const path = new URL(rt.request().url()).pathname;
    const body = path === '/ping' ? { computer_id: pcId } : path === '/apps-status' ? { apps: {} } : null;
    return rt.fulfill(body ? { status: 200, contentType: 'application/json', headers: AGENT_OK, body: J(body) } : { status: 404, headers: AGENT_OK, body: '' });
  });
  const p = await ctx.newPage();
  p.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
  await p.waitForFunction(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'),
    null, { timeout: 15000 }).catch(() => {});
  await p.waitForSelector('#scLeft', { timeout: 8000 }).catch(() => {});

  // Подстановка, отрисовка, чтение и возврат — одним синхронным действием на странице: опрос кошелька (раз в 10 с)
  // вклиниться не может. Каталог пакетов, если не сказано иное, — настоящий, с сервера
  await p.evaluate(() => { window.__probe = o => {
    const s = activeSession, $ = id => document.getElementById(id), vis = e => !!e && !e.classList.contains('hidden');
    const K = { w: me.wallet_grosz, r: s.effective_rate_pln, zid: s.zone_id, shop: packShop, pm: packMinutesHere, bd: bookingDeadline, str: STR, sz: sessZone, eco: economy };
    if (o.rate != null) s.effective_rate_pln = o.rate;
    const rateGr = Math.round(s.effective_rate_pln * 100), paid = s.money_minutes || 0, cost = m => Math.floor((rateGr * m + 59) / 60);
    if (o.w != null) me.wallet_grosz = o.w;
    if (o.mins != null) me.wallet_grosz = cost(paid + o.mins) - cost(paid); // денег ровно на столько минут
    if ('zid' in o) { if (o.zid) s.zone_id = o.zid; else delete s.zone_id; }
    if (o.zone) sessZone = o.zone;
    if (o.shop) packShop = o.shop.map(x => Object.assign({ zone_id: s.zone_id, zone: sessZone, spend_days_mask: 127, spend_start_min: 0, spend_end_min: 1440 }, x));
    if (o.pm != null) packMinutesHere = o.pm;
    if (o.bk) bookingDeadline = new Date(Date.now() + o.bk * 60000 - 20000).toISOString();
    if (o.lang) STR = Object.assign({}, SH_RU, o.lang === 'en' ? SH_EN : o.lang === 'pl' ? SH_PL : {});
    if (o.pause) economy = Object.assign({}, economy, { pause_limit_min: 5 });
    const redraw = o.lang || o.pause; if (redraw) renderSession();
    renderWallet();
    const el = $('scPack'), box = e => { const r = e.getBoundingClientRect(); return { l: r.left, r: r.right, t: r.top, b: r.bottom, w: Math.round(r.width), h: Math.round(r.height * 10) / 10 }; };
    const res = { shown: vis(el), name: el ? $('scPackName').textContent : null, price: el ? $('scPackPrice').textContent : null, hour: el ? $('scPackHour').textContent : null,
      red: $('sessCard').classList.contains('sc-low'), note: vis($('scNote')), wallet: me.wallet_grosz, rowH: Math.round(document.querySelector('.sc-btns').getBoundingClientRect().height * 10) / 10 };
    if (o.geo && vis(el)) { const card = $('sessCard'), hr = $('scPackHour');
      res.geo = { pack: box(el), name: box($('scPackName')), price: box($('scPackPrice')), hour: box(hr), btns: box(document.querySelector('.sc-btns')),
        end: box(document.querySelector('.sc-end')), card: box(card), over: card.scrollWidth > card.clientWidth, hourClip: hr.scrollWidth > hr.clientWidth,
        nameClip: $('scPackName').scrollWidth > $('scPackName').clientWidth }; }
    me.wallet_grosz = K.w; s.effective_rate_pln = K.r; if (K.zid) s.zone_id = K.zid; else delete s.zone_id;
    packShop = K.shop; packMinutesHere = K.pm; bookingDeadline = K.bd; STR = K.str; sessZone = K.sz; economy = K.eco;
    if (redraw) renderSession();
    renderWallet();
    return res;
  }; });
  const probe = o => p.evaluate(o => __probe(o), o || {});
  const is = (c, k, rate) => c.shown && c.name + c.price === line1(k) && c.hour === line2(k, rate);

  // 1. После входа — настоящие деньги, настоящий каталог
  await p.waitForFunction(() => { const e = document.getElementById('scPack'); return !!e && !e.classList.contains('hidden'); }, null, { timeout: 9000 }).catch(() => {});
  let c = await probe();
  const pre = await p.evaluate(() => ({ pm: packMinutesHere, zm: zoneMinutesHere, cm: me.coin_minutes, w: me.wallet_grosz, rate: activeSession.effective_rate_pln,
    tab: curTab, shop: packShop.map(x => x.name) }));
  z.check('условия: сессия идёт по 20 zł/ч, в кошельке хватает на «5 часов Standard», минут и пакетов у гостя нет',
    sessActive() && pre.rate === 20 && pre.w >= P5.gr && !pre.pm && !pre.zm && !pre.cm, J(pre));
  z.check(`после входа строка появилась сама, вкладку «Пакеты» не открывали: «${c.name}${c.price}», «${c.hour}» — цена часа = цена пакета / часы`,
    is(c, P5, 20) && !c.red && pre.tab !== 'pack', J({ c, tab: pre.tab, want: [line1(P5), line2(P5, 20)] }));
  z.check('каталог гостя: выключенной ловушки в нём нет, остальные три есть — и в строку они не попали',
    !pre.shop.includes('k93 выключен') && ['k93 чужая зона', 'k93 окно', 'k93 час не дешевле'].every(n => pre.shop.includes(n)) && is(c, P5, 20), J(pre.shop));
  await p.evaluate(async () => { if (!packShop.length) await loadPacks(); }); // до правки каталог сам не грузится — дальше проверяем остальное

  // 2. Пороги
  const T = [
    ['в кошельке 49,99 zł — строки нет: на «3 часа» не хватает гроша, а пакет за 20 zł с часом по 20 не дешевле', { w: 4999 }, null],
    ['в кошельке 50,00 zł', { w: 5000 }, P3, 20],
    ['в кошельке 74,99 zł', { w: 7499 }, P3, 20],
    ['в кошельке 75,00 zł — из двух доступных тот, где час дешевле', { w: 7500 }, P5, 20],
    ['скидка гостя, час сессии 16 zł', { w: 9000, rate: 16 }, P5, 16],
    ['час сессии 16 zł, в кошельке 74,99 — строки нет: «3 часа» с часом по 16,67 не дешевле', { w: 7499, rate: 16 }, null],
    ['час сессии 15 zł — строки нет: пакет с часом по 15 не дешевле', { w: 9000, rate: 15 }, null],
    ['час сессии 15,01 zł', { w: 9000, rate: 15.01 }, P5, 15.01],
    ['сервер не прислал id зоны — зона сверяется по имени', { w: 9000, zid: '' }, P5, 20],
    ['сессия в зоне VIP (30 zł/ч) — пакет этой зоны, не Standard', { w: 9000, zid: zv, zone: 'VIP', rate: 30 }, TRAP_ZONE, 30],
    ['гость уже играет по пакету (120 минут) — строки нет', { w: 9000, pm: 120 }, null],
  ];
  for (const [what, o, k, rate] of T) {
    c = await probe(o);
    z.check(k ? `${what}: «${line1(k)}», «${line2(k, rate)}»` : what, k ? is(c, k, rate) && !c.red : !c.shown && !c.red, J(c));
  }
  const CHEAP = [{ id: 'k93-cheap', name: '30 минут', minutes: 30, price_pln: 4, hour_pln: 8 }];
  c = await probe({ shop: CHEAP, mins: 18 });
  z.check('дешёвый пакет (4 zł), денег на 18 минут — карточка обычная, строка есть: «30 минут — 4 zł», «час по 8 zł вместо 20»',
    c.shown && !c.red && c.name + c.price === '30 минут — 4 zł' && c.hour === 'час по 8 zł вместо 20', J(c));
  c = await probe({ shop: CHEAP, mins: 13 });
  z.check('тот же пакет, денег на 13 минут — карточка красная: строки нет, на её месте «Пополнить — у стойки»', !c.shown && c.red && c.note && c.wallet >= 400, J(c));
  c = await probe({ w: 9000, bk: 9 });
  z.check('чужая бронь через 9 минут — карточка красная: строки нет', !c.shown && c.red && !c.note, J(c));

  // 3. Подарок — настоящий: гость играет по пакету, строка уходит; отменили — возвращается
  const gift = await z.api('POST', `/admin/users/${g}/packages`, { package_id: P3.id, method: 'comp', comp_reason: NOTE }, 'boss');
  await p.waitForFunction(() => (packMinutesHere || 0) > 0, null, { timeout: 14000 }).catch(() => {});
  c = await probe();
  const pm1 = await p.evaluate(() => packMinutesHere);
  z.check('гостю выдали пакет своей зоны — он играет по пакету, строка ушла', gift.status < 300 && pm1 > 0 && !c.shown, J({ s: gift.status, pm1, c }));
  await voidMine();
  await p.waitForFunction(() => !packMinutesHere, null, { timeout: 14000 }).catch(() => {});
  c = await probe();
  z.check('выдачу отменили — строка вернулась', is(c, P5, 20), J(c));

  // 4. Длинное имя и большая цена: три ширины × три языка, и самый тесный случай — с кнопкой паузы на 1440
  const LONG = [{ id: 'k93-long', name: ('Ночной марафон выходного дня для своих — ' + 'W'.repeat(64)).slice(0, 64), minutes: 6000, price_pln: 1439.99, hour_pln: 14.4 }];
  const HOUR = { ru: 'час по 14,40 zł вместо 20', en: '14,40 zł per hour instead of 20', pl: '14,40 zł za godzinę zamiast 20' };
  const plain = await probe({ w: 4999 });
  const bad = [], heights = [];
  const geoOk = (c, lang, tight) => { const q = c.geo; return c.shown && !!q && c.price === ' — 1439,99 zł' && c.hour === HOUR[lang]
    && q.price.r <= q.pack.r - 4 && q.price.w >= 60 && q.name.r <= q.price.l + 0.5 && q.name.w >= 30 && q.nameClip
    && q.pack.l >= q.btns.l - 0.5 && q.pack.r <= q.end.l - 4 && q.end.r <= q.card.r && !q.over
    && Math.abs(q.name.t - q.price.t) < 2 && q.hour.b <= q.pack.b && q.hour.r <= q.pack.r && (tight || !q.hourClip) && c.rowH - plain.rowH <= 8; };
  for (const [w, h] of [[1440, 900], [1920, 1080], [2560, 1440]]) {
    await p.setViewportSize({ width: w, height: h }); await p.waitForTimeout(250);
    for (const lang of ['ru', 'en', 'pl']) {
      c = await probe({ shop: LONG, w: 200000, lang, geo: true });
      heights.push(c.rowH); if (!geoOk(c, lang)) bad.push({ w, lang, c });
    }
    if (w === 1440) { c = await probe({ shop: LONG, w: 200000, lang: 'pl', pause: true, geo: true }); if (!geoOk(c, 'pl', true)) bad.push({ w, lang: 'pl+пауза', c }); }
  }
  await p.setViewportSize({ width: 1920, height: 1080 }); await p.waitForTimeout(250);
  z.check(`имя в 64 знака и цена 1439,99 zł на 1440, 1920 и 2560 по-русски, по-английски и по-польски (и с кнопкой паузы на 1440): цена видна целиком, `
    + `имя обрезано многоточием, «Завершить» на месте, карточку не распирает; ряд кнопок ${plain.rowH} → ${Math.max(...heights, 0)} px`, bad.length === 0 && heights.length === 9, J(bad.slice(0, 2)));

  // 5. Нажатие — только переход во вкладку
  await p.click('#scPack', { timeout: 3000 }).catch(() => {});
  await p.waitForSelector(`button[onclick*="${P5.id}"]`, { timeout: 5000 }).catch(() => {});
  const t5 = await p.evaluate(id => ({ tab: curTab, on: document.getElementById('st-pack').classList.contains('on'), cf: document.getElementById('confirmOverlay').classList.contains('show'),
    sess: !!activeSession, buy: !!document.querySelector(`button[onclick*="${id}"]`) }), P5.id);
  z.check('нажатие на строку открыло вкладку «Пакеты»: предложенный пакет там продаётся, подтверждения нет, сессия идёт',
    t5.tab === 'pack' && t5.on && !t5.cf && t5.sess && t5.buy && sessActive() && mine().length === 0, J(t5));
  if (t5.tab !== 'pack') { await p.evaluate(() => sideTab('pack')); await p.waitForSelector(`button[onclick*="${P5.id}"]`, { timeout: 5000 }).catch(() => {}); }

  // 6. Покупка во вкладке — настоящая. В кошельке 77 zł: после покупки денег на 6 минут, но у гостя 300 минут пакета
  setWallet(7700);
  await p.waitForFunction(() => me.wallet_grosz <= 7700 && me.wallet_grosz >= 7500, null, { timeout: 14000 }).catch(() => {});
  const b6 = await p.evaluate(() => { const card = document.getElementById('sessCard'), row = document.getElementById('scPack'), f = sessForecast(); window.__red = 0;
    new MutationObserver(() => { if (card.classList.contains('sc-low')) __red++; }).observe(card, { attributes: true, attributeFilter: ['class'] });
    return { left: f ? f.left : null, w: me.wallet_grosz, row: !!row && !row.classList.contains('hidden') }; });
  await p.click(`button[onclick*="${P5.id}"]`, { timeout: 5000 }).catch(e => console.log('кнопка «Купить»:', e.message.split('\n')[0]));
  await p.waitForSelector('#confirmOverlay.show', { timeout: 4000 }).catch(() => {});
  const asked = await p.evaluate(() => document.getElementById('confirmOverlay').classList.contains('show'));
  // С момента «Да» каждые 40 мс: когда уменьшился кошелёк, видна ли после этого строка, красная ли карточка, сколько осталось
  const watch = p.evaluate(() => new Promise(res => { const t0 = Date.now(), o = { bought: -1, pack: -1, n: 0, rowAfter: 0, redAfter: 0, minLeft: 1e9 };
    const iv = setInterval(() => { const t = Date.now() - t0, f = sessForecast(), row = document.getElementById('scPack');
      if (o.bought < 0 && me.wallet_grosz < 1000) o.bought = t;
      if (o.bought >= 0) { o.n++; if (row && !row.classList.contains('hidden')) o.rowAfter++;
        if (document.getElementById('sessCard').classList.contains('sc-low')) o.redAfter++;
        if (f) o.minLeft = Math.min(o.minLeft, f.left); if (o.pack < 0 && packMinutesHere > 0) o.pack = t; }
      if ((o.pack >= 0 && t > o.pack + 1500) || t > 16000) { clearInterval(iv); o.red = window.__red; o.left = f ? f.left : null; o.w = me.wallet_grosz; res(o); }
    }, 40); }));
  if (asked) await p.click('#cfYes');
  const w6 = await watch, got = mine();
  z.check('покупка во вкладке: подтверждение спрошено, пакет куплен с кошелька', asked && got.length === 1 && w6.bought >= 0 && w6.w < 400, J({ asked, got: got.length, w6, b6 }));
  z.check('после покупки строка исчезла сразу — в тот же миг, что уменьшился кошелёк', b6.row && w6.bought >= 0 && w6.n > 5 && w6.rowAfter === 0, J({ b6, w6 }));
  z.check(`после покупки карточка ни разу не покраснела и остаток не проваливался: было ${b6.left} мин, наименьшее после покупки — ${w6.minLeft}, стало ${w6.left}`,
    w6.bought >= 0 && w6.n > 5 && w6.red === 0 && w6.redAfter === 0 && w6.minLeft >= b6.left && w6.left >= 300, J({ b6, w6 }));
  await voidMine();

  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
