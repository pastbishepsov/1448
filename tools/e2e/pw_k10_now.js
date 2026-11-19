// К10 «Вкладка „Сейчас“» (KIOSK.md, этап VII; пункты 1–3 — К10-и1, пункт 4 — К10-и3, пункт 5 — К10-и2), браузер.
// Киоск PC-07, гость Guest46, сессия идёт.
// 1. После входа открыта «Сейчас»; вкладки 5 + 5 в порядке «Сейчас, Кейсы, Таланты, Ачивки, Топ / История, Кухня,
//    Брони, Пакеты, Монетки»; на трёх языках подписи целиком и в одну строку.
// 2. Наполнение — подстановкой на странице одним синхронным действием (подставить → нарисовать → прочитать →
//    вернуть): порядок блоков «заказ — кейсы — ачивка — стрик»; в заказах только те, что в работе, и выданный, но не
//    оплаченный; кейсов не больше трёх, ближе к сгоранию — выше, бессрочный — последним, открытые и сгоревшие не
//    попадают, остальное — кнопкой «Ещё N»; ачивка — ближайшая незакрытая дневная (закрытая, недельная и «полная, но
//    не взятая» не подходят), взяли её — показана следующая; пустое состояние со стриком и без него; три языка.
// 3. Настоящие пути, страницу никто не перезагружает: выдали кейс — счётчик вырос сам; открыли кейс прямо со «Сейчас»,
//    забрали приз — карточка ушла; заказали на «Кухне» — заказ на «Сейчас» со статусом и крестиком; админ принял —
//    статус сменился сам; вышли с «Кухни» и вошли снова — открыта «Сейчас», заказ на месте, меню при этом не
//    запрашивали; админ отменил — блок ушёл; полоса ачивки раз в минуту обновляется и на «Сейчас».
// 4. Заказы на «Сейчас» обновляются сами: заказали «с телефона» (теми же запросами, что шлёт PWA, мимо киоска; админ
//    не трогал, уведомлений гостю нет) — строка появилась сама, причём читались одни заказы, без меню; ничего не
//    изменилось — вкладка не перерисована; отменили «с телефона» — блок ушёл; вернулись на «Сейчас» с другой вкладки —
//    заказ виден сразу; «Кухня» обновляется как раньше, раз в 15 секунд.
// 5. Вкладка «Кейсы»: сверху только кейсы, которые можно открыть; открытые и сгоревшие — одной свёрнутой строкой
//    «Открытые раньше · N» и «Сгоревшие · M»; нажатие раскрывает и сворачивает список, перерисовка его не схлопывает,
//    смена вкладки сворачивает; счётчики сходятся с базой; кейс, открытый во вкладке, уходит в свёрнутые.
// Убирает за собой: сессия закрыта, свои заказы отменены (остаток возвращён), кошелёк и надбавка гостя как были.
// Выданный кейс и открытый кейс дают ноль по числу ждущих; выпавшее из кейса остаётся гостю.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const ordBefore = z.rows(`SELECT id FROM kitchen_orders WHERE user_id='${g}'`);
  const ordMine = () => z.rows(`SELECT id FROM kitchen_orders WHERE user_id='${g}' AND status NOT IN ('done','cancelled')`)
    .filter(id => !ordBefore.includes(id));
  const cancelMine = async () => { for (const id of ordMine()) await z.api('POST', `/admin/kitchen/orders/${id}/cancel`, {}, 'admin1'); };
  const waitingDb = () => +z.sql(`SELECT count(*) FROM cases WHERE user_id='${g}' AND opened_at IS NULL AND (expires_at IS NULL OR expires_at > now())`);
  const [w0, pct0] = z.sql(`SELECT wallet_grosz || '|' || payment_increase_pct FROM users WHERE id='${g}'`).split('|');
  // Кошелёк возвращаем одним запросом вместе со строкой журнала на ту же разницу: стенд трека З держит правило
  // «кошелёк гостя = сумма его журнала» (e2e_z0_stand); в конце проверка сама сверяет, что расхождение осталось каким было
  const setWallet = v => z.sql(`WITH o AS (SELECT id, wallet_grosz AS w FROM users WHERE id='${g}' FOR UPDATE),
      u AS (UPDATE users SET wallet_grosz = ${v} FROM o WHERE users.id = o.id RETURNING o.w)
    INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, note, created_at)
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k10_now: проверка вернула кошелёк', now() FROM u WHERE u.w <> ${v}`);
  const gap = () => z.sql('SELECT u.wallet_grosz - COALESCE((SELECT sum(amount_grosz) FROM wallet_transactions w WHERE w.user_id = u.id), 0) '
    + `FROM users u WHERE u.id='${g}'`);
  const gap0 = gap();
  z.atEnd(async () => {
    await endTails(); await cancelMine();
    setWallet(+w0);
    z.sql(`UPDATE users SET payment_increase_pct=${pct0} WHERE id='${g}'`); // бустер из открытого кейса — это надбавка
    z.check('проверка убрала за собой: своих открытых заказов нет, кошелёк гостя сходится со своим журналом так же, как до неё',
      ordMine().length === 0 && gap() === gap0, `заказов ${ordMine().length}, расхождение ${gap0} → ${gap()}`);
  });
  await endTails(); await cancelMine();

  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  const sid = seat.data && seat.data.session_id;
  z.check('гость посажен за PC-07, сессия идёт', !!sid, J(seat.data));

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
  const reqs = []; // что страница просит у сервера: [время, путь] — для «меню не запрашивали» и минутного обновления ачивок
  p.on('request', r => { const u = new URL(r.url()); if (u.pathname.includes('/me/')) reqs.push([Date.now(), u.pathname.replace(/^.*\/me\//, '')]); });
  const login = async () => {
    await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
    await p.waitForFunction(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'),
      null, { timeout: 15000 }).catch(() => {});
    await p.waitForSelector('#scLeft', { timeout: 8000 }).catch(() => {});
  };
  const until = (fn, arg, ms) => p.waitForFunction(fn, arg, { timeout: ms }).then(() => true).catch(() => false);
  const tabsOn = () => p.evaluate(() => curTab + ' / ' + [...document.querySelectorAll('.sidetabs button.on')].map(b => b.id).join(','));
  const heads = () => p.evaluate(() => [...document.querySelectorAll('#sideBody .now-h')].map(e => e.textContent.trim()));
  const shot = async (sel, name) => { try { await p.locator(sel).first().screenshot({ path: `${z.OUT}/k10_${name}.png` }); } catch (e) { console.log('кадр', name, e.message.split('\n')[0]); } };
  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await login();

  // ── 1. Вкладка по умолчанию и ряды вкладок
  z.check('после входа открыта «Сейчас», подсвечена только она', await tabsOn() === 'now / st-now', await tabsOn());
  const ROWS = ['now cases talents ach top', 'hist food book pack coins'];

  // ── 2. Подстановка: одно синхронное действие на странице — данные подставлены, вкладка нарисована, прочитана, всё возвращено
  const probe = st => p.evaluate(st => {
    const keep = { cases, achievements, myOrders, STR, curTab };
    const txt = e => (e ? e.textContent : '').replace(/\s+/g, ' ').trim();
    const out = {};
    try {
      if (st.lang) { STR = Object.assign({}, SH_RU, st.lang === 'en' ? SH_EN : st.lang === 'pl' ? SH_PL : {}); applyStrings(); }
      if ('cases' in st) cases = st.cases;
      if ('ach' in st) achievements = st.ach;
      if ('orders' in st) myOrders = st.orders;
      curTab = 'now';
      renderSide();
      const body = document.getElementById('sideBody');
      out.kids = [...body.children].map(e => {
        const c = e.classList;
        if (c.contains('now-h')) return 'h:' + txt(e);
        if (c.contains('foodorder')) return 'o:' + [...c].find(x => x.startsWith('st-')).slice(3) + ':' + txt(e.querySelector('b'))
          + (e.querySelector('button') ? ':x' : '') + (e.querySelector('b[style]') ? ':u' : '');
        if (c.contains('case-i')) return 'c:' + ((e.getAttribute('onclick') || '').match(/openCase\('([^']*)'/) || [0, '-'])[1];
        if (c.contains('now-more')) return 'm:' + txt(e);
        if (c.contains('ach')) return 'a:' + txt(e.querySelector('.t')) + ':' + txt(e.querySelector('.cyan-t'));
        if (c.contains('now-empty')) return 'e:' + txt(e.querySelector('.t')) + '|' + txt(e.lastChild);
        if (c.contains('streakcard')) return 's';
        return '?:' + e.className;
      });
      out.chips = [...body.querySelectorAll('.foodorder .fst')].map(txt);
      out.wide = body.scrollWidth - body.clientWidth; // вылезло ли что-то вбок
      out.left12 = T('ach_left_min', { n: 12 }); out.left50 = T('ach_left_min', { n: 50 });
      out.foodH = STR.food_my_orders; out.stNames = [STR.food_st_new, STR.food_st_done, STR.food_st_preparing];
      const btns = [...document.querySelectorAll('.sidetabs button')];
      out.rows = [...document.querySelectorAll('.sidetabs .strow')].map(r => [...r.querySelectorAll('button')].map(b => b.id.slice(3)).join(' '));
      out.labels = btns.map(b => txt(b.querySelector('[data-str]')));
      // подпись обрезана или перенесена: шире своей кнопки, в две строки, или ряд шире колонки
      out.clipped = btns.filter(b => { const s = b.querySelector('[data-str]');
        return b.scrollWidth > b.clientWidth + 1 || s.getClientRects().length !== 1 || s.getBoundingClientRect().height > 15
          || b.parentElement.scrollWidth > b.parentElement.clientWidth + 1; }).map(b => b.id.slice(3));
    } catch (e) { out.err = String((e && e.message) || e); }
    cases = keep.cases; achievements = keep.achievements; myOrders = keep.myOrders; STR = keep.STR; curTab = keep.curTab;
    if (st.lang) applyStrings();
    renderSide();
    return out;
  }, st);
  // то же, но оставить на экране (для кадра); вернуть как было — release()
  const hold = st => p.evaluate(st => {
    window.__k10 = { cases, achievements, myOrders, STR, curTab };
    if (st.lang) { STR = Object.assign({}, SH_RU, st.lang === 'en' ? SH_EN : st.lang === 'pl' ? SH_PL : {}); applyStrings(); }
    if ('cases' in st) cases = st.cases;
    if ('ach' in st) achievements = st.ach;
    if ('orders' in st) myOrders = st.orders;
    curTab = 'now'; renderSide();
  }, st);
  const release = () => p.evaluate(() => { const k = window.__k10; if (!k) return;
    cases = k.cases; achievements = k.achievements; myOrders = k.myOrders; STR = k.STR; curTab = k.curTab; window.__k10 = null;
    applyStrings(); renderSide(); });

  const DAY = 86400000, iso = d => new Date(Date.now() + d * DAY).toISOString();
  const kase = (id, tier, d, more) => Object.assign({ id, tier, source: 'admin_grant', club_id: '', opened_at: null, expires_at: d === null ? null : iso(d) }, more || {});
  const W20 = kase('w20', 'titan', 20.5), WNO = kase('wNo', 'medium', null), W5 = kase('w5', 'light', 5.5), W2 = kase('w2', 'light', 2.5), W9 = kase('w9', 'medium', 9.5);
  const OPENED = kase('op', 'light', 9.5, { opened_at: iso(-1), drop_type: 'coins', drop_amount: 30 }), BURNT = kase('ex', 'heavy', -0.5);
  const CASES = [W20, WNO, W5, OPENED, BURNT, W2, W9];
  const ach = (id, cat, v, t, more) => Object.assign({ id, category: cat, title: 'Ачивка ' + id, description: 'k10', earned: false,
    progress: { value: v, target: t, unit: 'min' }, reward_xp: 10 }, more || {});
  const STREAK = { days: 3, freeze: { enabled: false } };
  const A = ach('A', 'daily', 10, 60), B = ach('B', 'daily', 48, 60), C = ach('C', 'daily', 60, 60, { earned: true }),
    D = ach('D', 'daily', 60, 60), W = ach('W', 'weekly', 59, 60);
  const ACH = { achievements: [A, B, C, D, W], streak: STREAK };
  const ord = (id, status, more) => Object.assign({ id, status, name: 'Чай ' + id, qty: 1, total_pln: 7, can_cancel: status === 'new', unpaid: false, pay_method: null }, more || {});
  const ORD = [ord('n1', 'new'), ord('d1', 'done', { unpaid: true }), ord('d2', 'done', { pay_method: 'cash' }), ord('c1', 'cancelled'), ord('p1', 'preparing', { qty: 2 })];
  const FULL = { cases: CASES, ach: ACH, orders: ORD };
  const same = (a, b) => J(a) === J(b);

  const s1 = await probe(FULL);
  z.check('вкладки 5 + 5: Сейчас, Кейсы, Таланты, Ачивки, Топ / История, Кухня, Брони, Пакеты, Монетки', same(s1.rows, ROWS), J(s1.rows));
  const want1 = ['h:' + s1.foodH, 'o:new:Чай n1:x', 'o:done:Чай d1:u', 'o:preparing:Чай p1',
    'h:Ждут открытия · 5', 'c:w2', 'c:w5', 'c:w9', 'm:Ещё 2 — во вкладке „Кейсы“',
    'h:Ближайшая дневная ачивка', 'a:Ачивка B:' + s1.left12, 's'];
  z.check('порядок блоков сверху вниз: заказ кухни, кейсы, ближайшая дневная ачивка, стрик', same(s1.kids, want1) && !s1.err, J(s1.kids) + ' ' + (s1.err || ''));
  z.check('в заказах — те, что в работе, и выданный, но не оплаченный; оплаченного и отменённого нет; у каждого свой статус',
    s1.kids.filter(k => k.startsWith('o:')).length === 3 && same(s1.chips, [s1.stNames[0], s1.stNames[1], s1.stNames[2]]), J(s1.chips));
  z.check('кейсов на «Сейчас» не больше трёх: ближе к сгоранию — выше; открытый и сгоревший не попали; остальные — «Ещё 2»',
    same(s1.kids.filter(k => /^[cm]:/.test(k)), ['c:w2', 'c:w5', 'c:w9', 'm:Ещё 2 — во вкладке „Кейсы“']), J(s1.kids.filter(k => /^[cm]:/.test(k))));
  z.check('ачивка — ближайшая незакрытая дневная, с «ещё 12 мин»: закрытая, недельная 59/60 и «60/60, но не взята» не подошли',
    s1.kids.includes('a:Ачивка B:' + s1.left12) && s1.left12.includes('12') && s1.kids.filter(k => k.startsWith('a:')).length === 1, J(s1.kids.filter(k => k.startsWith('a:'))));
  z.check('ничего не вылезает вбок из колонки', s1.wide <= 0, 'лишних px: ' + s1.wide);

  const s2 = await probe({ cases: [WNO, W20, W2], ach: ACH, orders: [] });
  z.check('ровно три кейса: все три, бессрочный — последним, кнопки «Ещё» нет; блока заказов нет',
    same(s2.kids.slice(0, 4), ['h:Ждут открытия · 3', 'c:w2', 'c:w20', 'c:wNo']) && !s2.kids.some(k => /^[mo]:/.test(k)), J(s2.kids));
  const s2b = await probe({ cases: [WNO, W20, W2, W5], ach: ACH, orders: [] });
  z.check('четыре кейса: три карточки и «Ещё 1»', same(s2b.kids.slice(0, 5), ['h:Ждут открытия · 4', 'c:w2', 'c:w5', 'c:w20', 'm:Ещё 1 — во вкладке „Кейсы“']), J(s2b.kids));
  const s2c = await probe({ cases: CASES, orders: ORD, ach: { achievements: [A, Object.assign({}, B, { earned: true }), C, D, W], streak: STREAK } });
  z.check('ачивку взяли — на её месте следующая по близости («ещё 50 мин»)', s2c.kids.includes('a:Ачивка A:' + s2c.left50), J(s2c.kids.filter(k => k.startsWith('a:'))));
  const s2d = await probe({ cases: CASES, orders: ORD, ach: { achievements: [C, D, W], streak: STREAK } });
  z.check('незакрытых дневных нет — блока ачивки нет, остальное на месте', !s2d.kids.some(k => k.startsWith('a:') || k === 'h:Ближайшая дневная ачивка') && s2d.kids.length === want1.length - 2, J(s2d.kids));

  const EMPTY = 'e:Пока ничего не ждёт|Заказ с кухни, новый кейс и ближайшая дневная ачивка появятся здесь сами';
  const s3 = await probe({ cases: [OPENED, BURNT], orders: [ORD[2], ORD[3]], ach: { achievements: [C, W], streak: STREAK } });
  z.check('пусто (кейсы открыты или сгорели, заказы закрыты, дневные взяты): «Пока ничего не ждёт» и стрик под ним', same(s3.kids, [EMPTY, 's']), J(s3.kids));
  const s4 = await probe({ cases: [], orders: [], ach: { achievements: [] } });
  z.check('пусто и стрика нет в данных: только пустое состояние', same(s4.kids, [EMPTY]), J(s4.kids));
  const s5 = await probe({ cases: [], orders: [], ach: null });
  z.check('ачивки ещё не пришли с сервера: вкладка рисуется, а не падает', same(s5.kids, [EMPTY]) && !s5.err, J(s5.kids) + ' ' + (s5.err || ''));

  // три языка: подписи вкладок и тексты блоков
  const L = { ru: ['Сейчас', 'Ждут открытия · 5', 'Ещё 2 — во вкладке „Кейсы“', 'Ближайшая дневная ачивка', EMPTY],
    en: ['Now', 'Waiting to be opened · 5', '2 more — in the Cases tab', 'Nearest daily achievement',
      'e:Nothing is waiting right now|A kitchen order, a new case and the nearest daily achievement will show up here by themselves'],
    pl: ['Teraz', 'Czekają na otwarcie · 5', 'Jeszcze 2 — w zakładce Skrzynki', 'Najbliższe dzienne osiągnięcie',
      'e:Na razie nic nie czeka|Zamówienie z kuchni, nowa skrzynka i najbliższe dzienne osiągnięcie pojawią się tu same'] };
  for (const lang of ['ru', 'en', 'pl']) {
    const f = await probe(Object.assign({ lang }, FULL)), e = await probe({ lang, cases: [], orders: [], ach: { achievements: [] } });
    z.check(`${lang}: десять подписей вкладок целиком и в одну строку, первая — «${L[lang][0]}»`,
      f.labels.length === 10 && f.labels.every(t => t.length > 0) && f.labels[0] === L[lang][0] && f.clipped.length === 0 && same(f.rows, ROWS),
      J(f.labels) + ' обрезаны: ' + J(f.clipped));
    z.check(`${lang}: заголовки блоков, «ещё N» и пустое состояние на своём языке, вбок ничего не вылезает`,
      f.kids.includes('h:' + f.foodH) && f.kids.includes('h:' + L[lang][1]) && f.kids.includes('m:' + L[lang][2]) && f.kids.includes('h:' + L[lang][3])
      && same(e.kids, [L[lang][4]]) && f.wide <= 0 && e.wide <= 0, J(f.kids.filter(k => /^[hm]:/.test(k))) + ' ' + J(e.kids));
    await hold(Object.assign({ lang }, FULL)); await shot('.sidetabs', 'tabs_' + lang);
    if (lang === 'ru') { await shot('#sideBody', 'now_full'); await release(); await hold({ cases: [], orders: [], ach: { achievements: [], streak: STREAK } }); await shot('#sideBody', 'now_empty'); }
    await release();
  }
  z.check('после подстановок страница вернулась к своим данным и к «Сейчас»', await tabsOn() === 'now / st-now', await tabsOn());

  // ── 3. Настоящие пути: страницу не перезагружаем и сами ничего не перечитываем
  const n0 = waitingDb();
  // в заголовке блока кейсов стоит n; если n = 0 — блока кейсов нет совсем
  const cntIs = n => until(n => (n > 0 ? [...document.querySelectorAll('#sideBody .now-h')].some(e => e.textContent.trim().endsWith('· ' + n))
    : curTab === 'now' && !document.querySelector('#sideBody .case-i')), n, 25000);
  if (n0 > 0) z.check(`у гостя на стенде ждут ${n0} кейсов — столько же в заголовке; карточек ${Math.min(3, n0)}`,
    await cntIs(n0) && await p.locator('#sideBody .case-i').count() === Math.min(3, n0), J(await heads()));
  const gr = await z.api('POST', `/admin/users/${g}/grant`, { type: 'case', case_tier: 'light', reason: 'pw_k10_now: проверка' }, 'boss');
  z.check('кейс выдан — счётчик на «Сейчас» вырос сам, без перезагрузки', gr.status < 300 && await cntIs(n0 + 1), gr.status + ' ' + J(await heads()));
  if (n0 + 1 > 3) {
    await p.click('#sideBody .now-more');
    z.check('«Ещё N» ведёт во вкладку «Кейсы»', await tabsOn() === 'cases / st-cases', await tabsOn());
    await p.evaluate(() => sideTab('now'));
  }
  await shot('#sideBody', 'now_real');
  const cid = await p.evaluate(() => ((document.querySelector('#sideBody .case-i').getAttribute('onclick') || '').match(/openCase\('([^']*)'/) || [0, ''])[1]);
  await p.click('#sideBody .case-i');
  const prize = await until(() => !document.getElementById('coResult').classList.contains('hidden'), null, 20000);
  await p.evaluate(() => goBack()); // «назад» после приза = «Забрать»
  z.check('кейс открыт прямо со «Сейчас»: приз показан, после «Забрать» карточка ушла и счётчик вернулся — без перезагрузки',
    prize && await cntIs(n0) && !(await p.evaluate(id => !!document.querySelector(`#sideBody .case-i[onclick*="${id}"]`), cid))
    && z.sql(`SELECT (opened_at IS NOT NULL)::int FROM cases WHERE id='${cid}'`) === '1', `приз ${prize}, ${J(await heads())}`);

  await p.evaluate(() => sideTab('food'));
  const canOrder = await until(() => !!document.querySelector('#sideBody button[onclick^="foodOrder("]:not([disabled])'), null, 8000);
  if (canOrder) { // К11-и4: «Заказать» у блюда спрашивает — заказ создаёт кнопка «Заказать» в окне вопроса
    await p.click('#sideBody button[onclick^="foodOrder("]:not([disabled])');
    if (await until(() => document.getElementById('confirmOverlay').classList.contains('show'), null, 3000)) await p.click('#cfYes');
  }
  for (let i = 0; i < 30 && ordMine().length < 1; i++) await z.sleep(300);
  const oid = ordMine()[0];
  z.check('заказ с «Кухни» принят сервером', canOrder && !!oid, `кнопка ${canOrder}, заказ ${oid}`);
  await p.evaluate(() => sideTab('now'));
  const rowNew = await until(() => { const b = document.getElementById('sideBody'), r = b.querySelector('.foodorder.st-new');
    return !!r && !!r.querySelector('button') && b.firstElementChild.classList.contains('now-h') && b.children[1] === r; }, null, 5000);
  z.check('заказ на «Сейчас» первым блоком: статус «новый» и крестик отмены, как на «Кухне»', rowNew, J(await heads()));
  const st1 = oid ? await z.api('POST', `/admin/kitchen/orders/${oid}/status`, { status: 'accepted' }, 'admin1') : { status: 0 };
  const rowAcc = await until(() => { const r = document.querySelector('#sideBody .foodorder'); return !!r && r.classList.contains('st-accepted') && !r.querySelector('button'); }, null, 25000);
  z.check('админ принял заказ — статус на «Сейчас» сменился сам, крестик пропал', st1.status === 200 && rowAcc, 'ответ ' + st1.status);

  await p.evaluate(() => sideTab('food')); await z.sleep(800);
  await p.evaluate(() => logout());
  await p.waitForSelector('#nick', { state: 'visible', timeout: 8000 }).catch(() => {});
  const mark = Date.now();
  await login();
  const rowBack = await until(() => !!document.querySelector('#sideBody .foodorder.st-accepted'), null, 8000);
  const menuAsked = reqs.filter(r => r[0] >= mark && r[1] === 'menu').length;
  z.check('вышел с «Кухни», вошёл снова — открыта «Сейчас», подсвечена только она', await tabsOn() === 'now / st-now', await tabsOn());
  z.check('заказ после входа на месте, хотя «Кухню» не открывали (меню не запрашивалось)', rowBack && menuAsked === 0, `строка ${rowBack}, запросов меню ${menuAsked}`);
  const cn = oid ? await z.api('POST', `/admin/kitchen/orders/${oid}/cancel`, {}, 'admin1') : { status: 0 };
  const rowGone = await until(() => !document.querySelector('#sideBody .foodorder'), null, 25000);
  z.check('админ отменил заказ — блок заказа ушёл сам вместе с заголовком', cn.status === 200 && rowGone && !(await heads()).some(h => h === s1.foodH), 'ответ ' + cn.status + ' ' + J(await heads()));

  // полоса ачивки раз в минуту: одиночный запрос ачивок (не вместе с кейсами, как при полной перезагрузке данных)
  const lone = () => reqs.some(a => a[1] === 'achievements' && !reqs.some(c => c[1] === 'cases' && Math.abs(c[0] - a[0]) < 3000));
  for (let i = 0; i < 75 && !lone(); i++) await z.sleep(1000);
  z.check('пока открыта «Сейчас» и идёт сессия, ачивки обновляются раз в минуту (как на вкладке «Ачивки»)', lone() && await tabsOn() === 'now / st-now', await tabsOn());

  // ── 4. К10-и3: заказы на «Сейчас» обновляются сами. «С телефона» — теми же запросами, что шлёт PWA, мимо этого киоска;
  // админ ничего не делает, уведомлений гостю о его же действиях сервер не шлёт — строка может появиться и уйти только
  // за счёт собственного обновления вкладки
  const notifN = () => +z.sql(`SELECT count(*) FROM notifications WHERE user_id='${g}'`);
  const since = (t, what) => reqs.filter(r => r[0] >= t && r[1] === what).length;
  const gid = oid ? z.sql(`SELECT good_id FROM kitchen_orders WHERE id='${oid}'`) : '';
  // сначала киоск забирает прежние уведомления (отмена заказа админом выше): иначе данные перечитал бы их тост, а не вкладка
  const t40 = Date.now();
  for (let i = 0; i < 20 && since(t40, 'notifications') < 1; i++) await z.sleep(1000);
  await z.sleep(2500);
  const nt0 = notifN(), t4 = Date.now();
  const po = await z.api('POST', '/me/kitchen/orders', { good_id: gid, qty: 1 }, G);
  const poId = po.data && po.data.order && po.data.order.id;
  const rowPhone = await until(() => !!document.querySelector('#sideBody .foodorder.st-new'), null, 20000);
  const sec4 = ((Date.now() - t4) / 1000).toFixed(1), full4 = since(t4, 'cases');
  z.check('К10-и3: заказ «с телефона» (мимо киоска, админ не трогал) сам появился на «Сейчас» за 20 секунд — без уведомления гостю и без полной перезагрузки данных',
    !!poId && rowPhone && notifN() === nt0 && full4 === 0 && await tabsOn() === 'now / st-now',
    `ответ ${po.status}, строка ${rowPhone} за ${sec4} с, уведомлений ${notifN() - nt0}, полных перезагрузок ${full4}, ${await tabsOn()}`);
  z.check('обновление «Сейчас» читает одни заказы — меню не запрашивает', since(t4, 'kitchen/orders') >= 1 && since(t4, 'menu') === 0,
    `запросов заказов ${since(t4, 'kitchen/orders')}, меню ${since(t4, 'menu')}`);
  // ничего не изменилось — вкладку не трогаем: метка на строке заказа переживает обновление (вторая попытка — на случай,
  // если ровно в этот миг вкладку перерисовало минутное обновление ачивок)
  const keepMark = () => p.evaluate(async () => {
    const r = document.querySelector('#sideBody .foodorder');
    if (!r || typeof loadMyOrders !== 'function') return 'нет строки заказа или самого обновления';
    r.dataset.k10 = 'keep'; await loadMyOrders();
    const r2 = document.querySelector('#sideBody .foodorder');
    return r2 && r2.dataset.k10 === 'keep' ? 'та же' : 'перерисована';
  });
  let kept = await keepMark(); if (kept === 'перерисована') kept = await keepMark();
  z.check('заказы перечитаны, ничего не изменилось — вкладка не перерисована (строка заказа та же)', kept === 'та же', kept);
  const pc = poId ? await z.api('POST', `/me/kitchen/orders/${poId}/cancel`, {}, G) : { status: 0 };
  const rowPhoneGone = await until(() => !document.querySelector('#sideBody .foodorder'), null, 20000);
  z.check('отменили «с телефона» — блок заказа ушёл с «Сейчас» сам за 20 секунд, уведомлений гостю нет',
    rowPhone && pc.status === 200 && rowPhoneGone && notifN() === nt0, `строка была ${rowPhone}, ответ ${pc.status}, ушла ${rowPhoneGone}, уведомлений ${notifN() - nt0}`);
  // с другой вкладки на «Сейчас» — заказы перечитаны сразу, своей очереди не ждём
  await p.click('#st-top');
  const po2 = await z.api('POST', '/me/kitchen/orders', { good_id: gid, qty: 1 }, G);
  const po2Id = po2.data && po2.data.order && po2.data.order.id;
  const t6 = Date.now();
  await p.click('#st-now');
  const rowOpen = await until(() => !!document.querySelector('#sideBody .foodorder.st-new'), null, 2500);
  z.check('вернулись на «Сейчас» с другой вкладки — новый заказ виден сразу, а не через 15 секунд', !!po2Id && rowOpen, `ответ ${po2.status}, строка ${rowOpen} за ${Date.now() - t6} мс`);
  // «Кухня» обновляется как раньше: раз в 15 секунд меню и заказы вместе (запросы самого открытия вкладки не в счёт)
  await p.click('#st-food');
  const t7 = Date.now() + 1500;
  for (let i = 0; i < 20 && since(t7, 'menu') < 1; i++) await z.sleep(1000);
  await z.sleep(500);
  z.check('«Кухня» обновляется как раньше — раз в 15 секунд меню и заказы вместе', since(t7, 'menu') >= 1 && since(t7, 'kitchen/orders') >= 1,
    `после открытия вкладки: запросов меню ${since(t7, 'menu')}, заказов ${since(t7, 'kitchen/orders')}`);
  if (po2Id) await z.api('POST', `/me/kitchen/orders/${po2Id}/cancel`, {}, G);
  await p.click('#st-now');
  const clean4 = await until(() => !document.querySelector('#sideBody .foodorder'), null, 20000);
  z.check('проверка отменила свои заказы — на «Сейчас» блока заказа снова нет', ordMine().length === 0 && clean4, `открытых заказов в базе ${ordMine().length}, блок ушёл ${clean4}`);

  // ── 5. К10-и2: во вкладке «Кейсы» сверху кейсы, которые можно открыть; открытые и сгоревшие — одной свёрнутой строкой.
  // Подстановка, как в пункте 2: одно синхронное действие на странице — данные, вкладка, шаги, чтение, всё вернуть.
  // Чтение: top / under — карточки над строкой и под ней (o — можно открыть, x — открыт или сгорел), line — ярлыки строки
  const view = (cs, steps) => p.evaluate(([cs, steps]) => {
    const keep = { cases, curTab }, out = [], body = document.getElementById('sideBody');
    const read = () => {
      const kids = [...body.children], i = kids.findIndex(e => e.classList.contains('cases-past'));
      const rows = l => l.filter(e => e.classList.contains('case-i')).map(e => e.classList.contains('opened') ? 'x' : 'o').join('');
      return { line: i < 0 ? null : [...kids[i].querySelectorAll('span')].map(s => s.textContent.trim()).join(' | '),
        top: rows(i < 0 ? kids : kids.slice(0, i)), under: i < 0 ? '' : rows(kids.slice(i + 1)),
        empty: !!body.querySelector(':scope > .muted a') };
    };
    try {
      cases = cs; sideTab('cases'); out.push(read());
      for (const s of steps || []) {
        if (s === 'click') { const b = body.querySelector('.cases-past'); if (b) b.click(); }
        if (s === 'render') renderSide();
        if (s === 'tabs') { sideTab('top'); sideTab('cases'); }
        out.push(read());
      }
    } catch (e) { out.push({ err: String((e && e.message) || e) }); }
    cases = keep.cases; sideTab(keep.curTab);
    return out;
  }, [cs, steps]);
  const LINE11 = 'Открытые раньше · 1 | Сгоревшие · 1 | ▾';
  const v1 = await view(CASES, ['click', 'render', 'click', 'click', 'tabs']), at = n => v1[n] || {};
  z.check('К10-и2: «Кейсы» — сверху только кейсы, которые можно открыть; открытый и сгоревший убраны в одну свёрнутую строку со своими счётчиками',
    at(0).top === 'ooooo' && at(0).under === '' && at(0).line === LINE11, J(v1[0]));
  z.check('нажали на строку — под ней открытый и сгоревший, наверху всё как было; перерисовка (пришли свежие данные) список не сворачивает',
    at(1).top === 'ooooo' && at(1).under === 'xx' && /▴$/.test(at(1).line || '') && at(2).under === 'xx', J([v1[1], v1[2]]));
  z.check('нажали ещё раз — список свёрнут; раскрыли, ушли на другую вкладку и вернулись — снова свёрнут',
    at(3).under === '' && at(4).under === 'xx' && at(5).under === '' && at(5).top === 'ooooo', J([v1[3], v1[4], v1[5]]));
  const O2 = kase('op2', 'medium', 3.5, { opened_at: iso(-2), drop_type: 'buster', drop_amount: 500 });
  const v2 = await view([W20, OPENED, BURNT, O2], ['click']), v3 = await view([BURNT, W2]), v4 = await view([W20, W2]), v5 = await view([OPENED, BURNT]);
  z.check('счётчики раздельные: открытых 2 и сгоревший 1; только сгоревший — в строке один ярлык «Сгоревшие · 1»; ни тех ни других — строки нет',
    v2[0].line === 'Открытые раньше · 2 | Сгоревшие · 1 | ▾' && v2[0].top === 'o' && (v2[1] || {}).under === 'xxx'
    && v3[0].line === 'Сгоревшие · 1 | ▾' && v3[0].top === 'o' && v4[0].line === null && v4[0].top === 'oo', J([v2, v3[0], v4[0]]));
  z.check('открывать нечего, а прошлые есть — сверху «Кейсов пока нет», строка под ним на месте',
    v5[0].empty === true && v5[0].top === '' && v5[0].line === LINE11, J(v5[0]));
  const dict = await p.evaluate(() => [SH_RU, SH_EN, SH_PL].map(d => [d.cases_past, d.cases_burnt].map(s => /\{n\}/.test(s || '') ? 1 : 0).join('')).join(' '));
  z.check('ярлыки строки есть на трёх языках', dict === '11 11 11', dict);
  // кадры для глаз: те же подставленные кейсы — строка свёрнута и раскрыта (нажатие настоящее, мышью); в пункты не входят
  await p.evaluate(cs => { window.__k10c = { cases, curTab }; cases = cs; curTab = 'cases'; renderSide(); }, CASES);
  await shot('#sideBody', 'cases_past');
  if (await p.locator('#sideBody .cases-past').count()) await p.click('#sideBody .cases-past');
  await shot('#sideBody', 'cases_past_open');
  await p.evaluate(() => { const k = window.__k10c; window.__k10c = null; cases = k.cases; sideTab(k.curTab); });
  // настоящие данные: счётчики — открытые и сгоревшие кейсы гостя, как их отдаёт сервер (не больше ста, сначала неоткрытые)
  const pastDb = () => z.sql(`SELECT count(*) FILTER (WHERE opened_at IS NOT NULL) || ' ' || count(*) FILTER (WHERE opened_at IS NULL AND expires_at <= now())
    FROM (SELECT opened_at, expires_at FROM cases WHERE user_id='${g}' ORDER BY (opened_at IS NULL) DESC, created_at DESC LIMIT 100) x`);
  const tri = want => until(want => {
    const b = document.querySelector('#sideBody .cases-past'), t = b ? [...b.querySelectorAll('span')].map(s => s.textContent) : [];
    const num = k => { const pre = (STR[k] || '\n').split('{n}')[0], m = t.find(x => x.startsWith(pre)); return m ? +m.slice(pre.length).replace(/\D+/g, '') : 0; };
    return curTab === 'cases' && [num('cases_past'), num('cases_burnt'), document.querySelectorAll('#sideBody > .case-i:not(.opened)').length].join(' ') === want;
  }, want, 25000);
  const gr5 = await z.api('POST', `/admin/users/${g}/grant`, { type: 'case', case_tier: 'light', reason: 'pw_k10_now: проверка' }, 'boss');
  await p.click('#st-cases');
  const w5 = waitingDb(), [o5, b5] = pastDb().split(' ').map(Number);
  z.check('настоящие данные: наверху столько кейсов, сколько ждут в базе; в строке — столько открытых и сгоревших, сколько отдаёт сервер',
    gr5.status < 300 && w5 >= 1 && o5 >= 1 && !!(await tri(`${o5} ${b5} ${w5}`)), `в базе: открытых ${o5}, сгоревших ${b5}, ждут ${w5}`);
  await shot('#sideBody', 'cases_real');
  await p.click('#sideBody > .case-i:not(.opened)');
  const prize5 = await until(() => !document.getElementById('coResult').classList.contains('hidden'), null, 20000);
  await p.evaluate(() => goBack()); // «назад» после приза = «Забрать»
  z.check('открыли кейс во вкладке «Кейсы» — он ушёл в свёрнутые: наверху на один меньше, «Открытые раньше» на один больше',
    !!prize5 && !!(await tri(`${o5 + 1} ${b5} ${w5 - 1}`)) && pastDb() === `${o5 + 1} ${b5}`, `приз ${prize5}, в базе ${pastDb()}, ждут ${waitingDb()}`);
  await p.evaluate(() => sideTab('now'));

  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
