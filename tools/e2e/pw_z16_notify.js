// З16-и1в «Тариф в уведомлении о пересадке», браузер (вопрос отчёта З16-и1а, решение основателя 28.10). Пересадка за ПК
// другой зоны по её тарифу (З16-и1а) — гостю «Тебя пересадили за VIP-01 — сессия продолжается · тариф VIP 30 zł/ч» в
// киоске нового ПК и в PWA; пересадка в той же зоне, с прежним тарифом и от прежней версии сервера — без тарифа, как
// раньше; без зоны — «· тариф 23 zł/ч»; тексты — RU, EN и PL; APP_VER в app.html поднят и равен app_version манифеста.
// Гость Guest47: кошелёк докладывается до 30 zł (пополнение картой от boss), сессия PC-03 → VIP-01 по тарифу VIP (API),
// в конце завершается; его непрочитанные уведомления перед пересадкой помечаются прочитанными — в тостах только она.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const G = 'Guest47', g = z.uid(G);
  const pc = n => z.sql(`SELECT id FROM computers WHERE name='${n}'`);
  for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND user_id='${g}'`)) // хвосты упавшего прогона
    await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
  z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
    + "(SELECT computer_id FROM sessions WHERE status='active')");
  const w = +z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  if (w < 3000) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: (3000 - w) / 100, method: 'card' }, 'boss');
  const seat = await z.api('POST', `/admin/computers/${pc('PC-03')}/session`, { guest: G }, 'admin1');
  const sid = seat.data.session_id;
  z.atEnd(async () => {
    if (sid && z.sql(`SELECT status FROM sessions WHERE id='${sid}'`) === 'active') await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  });
  z.sql(`UPDATE sessions SET started_at = now() - interval '3 minutes', ready_at = now() - interval '3 minutes' WHERE id='${sid}'`);
  z.sql(`UPDATE notifications SET read_at = now(), read_kiosk_at = now(), read_app_at = now() WHERE user_id='${g}' AND read_at IS NULL`);
  const mv = await z.api('POST', `/admin/sessions/${sid}/move`, { computer_id: pc('VIP-01'), tariff: 'new' }, 'admin1');
  const num = v => { const n = Math.round(v * 100) / 100; return (Number.isInteger(n) ? String(n) : n.toFixed(2)).replace('.', ','); };
  const WANT = `🖥 Тебя пересадили за VIP-01 — сессия продолжается · тариф VIP ${num(mv.data.rate_pln || 0)} zł/ч`;
  z.check(`PC-03 → VIP-01 по тарифу VIP (API) — 200, тариф ${num(mv.data.rate_pln || 0)} zł/ч`,
    mv.status === 200 && mv.data.tariff === 'new' && mv.data.zone === 'VIP' && mv.data.rate_pln > 0, JSON.stringify(mv.data));

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  // З18-и4ж: тосты страница складывает списком (обёртка над toast, как в pw_zh5): тост лимитера «Слишком много запросов»
  // при входе — всплеск запросов с одного IP стенда — перекрывал нужный, и проверка его не видела
  const toastLog = p => p.evaluate(() => { window.__toasts = []; const t = toast; toast = (msg, ...r) => { __toasts.push(String(msg)); return t(msg, ...r); }; });
  const toastIs = (p, want) => p.waitForFunction(w => (window.__toasts || []).includes(w), want, { timeout: 20000 }).then(() => true).catch(() => false);
  const toasts = p => p.evaluate(() => (window.__toasts || []).join(' | '));

  // киоск нового ПК: тост — с ближайшим опросом уведомлений (раз в 12 с)
  const k = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell?pc=' + pc('VIP-01'), { waitUntil: 'load' });
  await toastLog(k);
  await k.fill('#nick', G); await k.fill('#pass', z.GUEST_PASS); await k.click('#authBtn');
  const kOk = await toastIs(k, WANT);
  z.check(`киоск VIP-01: тост «${WANT}»`, kOk, await toasts(k));
  await z.shot(k, 'z16_notify_kiosk');
  const ks = await k.evaluate(() => {
    const n = (c, zone, r, tariff) => notifText({ type: 'session_moved', payload: tariff === undefined ? { computer: c }
      : { computer: c, zone, rate_pln: r, tariff } });
    const out = { keep: n('VIP-02', 'Standard', 20, 'keep'), same: n('PC-05', 'Standard', 20, 'same'), old: n('PC-05'),
      nozone: n('PC-01', '', 23, 'new') };
    const save = STR;
    for (const [l, d] of [['en', SH_EN], ['pl', SH_PL]]) { STR = Object.assign({}, SH_RU, d); out[l] = n('VIP-01', 'VIP', 27.5, 'new'); }
    STR = save;
    return out;
  });
  z.check('киоск: с прежним тарифом, в той же зоне и от прежней версии сервера — без тарифа; без зоны — «· тариф 23 zł/ч»',
    ks.keep === '🖥 Тебя пересадили за VIP-02 — сессия продолжается' && ks.same === '🖥 Тебя пересадили за PC-05 — сессия продолжается'
    && ks.old === ks.same && ks.nozone === '🖥 Тебя пересадили за PC-01 — сессия продолжается · тариф 23 zł/ч', JSON.stringify(ks));
  z.check('киоск, EN и PL: «… · VIP rate 27.50 zł/h», «… · stawka VIP 27,50 zł/h»',
    ks.en === '🖥 You were moved to VIP-01 — the session continues · VIP rate 27.50 zł/h'
    && ks.pl === '🖥 Przesadzono cię na VIP-01 — sesja trwa dalej · stawka VIP 27,50 zł/h', `${ks.en} | ${ks.pl}`);

  // PWA: уведомление ещё не прочитано «вообще» (киоск ставит свою отметку) — приходит при входе
  const a = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })).newPage();
  a.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await a.goto(z.BASE + '/app', { waitUntil: 'load' });
  await toastLog(a);
  await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
  const aOk = await toastIs(a, WANT);
  const feed = await a.evaluate(() => { const f = document.getElementById('ntfFeed'); return f ? f.textContent : ''; });
  z.check(`PWA: тост и строка ленты уведомлений «${WANT}»`, aOk && feed.includes(WANT), `${await toasts(a)} · ${feed.slice(0, 160)}`);
  await z.shot(a, 'z16_notify_pwa');
  const as = await a.evaluate(() => {
    const n = (c, zone, r, tariff) => ntfText({ type: 'session_moved', payload: { computer: c, zone, rate_pln: r, tariff } });
    const out = { keep: n('VIP-02', 'Standard', 20, 'keep'), nozone: n('PC-01', '', 23, 'new') };
    const save = STR;
    for (const [l, d] of [['en', STR_EN], ['pl', STR_PL]]) { STR = Object.assign({}, STR_RU, d); out[l] = n('VIP-01', 'VIP', 27.5, 'new'); }
    STR = save;
    return out;
  });
  z.check('PWA: с прежним тарифом — без тарифа; без зоны — «· тариф 23 zł/ч»; EN и PL — «· VIP rate 27.50 zł/h», «· stawka VIP '
    + '27,50 zł/h»', as.keep === '🖥 Тебя пересадили за VIP-02 — сессия продолжается'
    && as.nozone === '🖥 Тебя пересадили за PC-01 — сессия продолжается · тариф 23 zł/ч'
    && as.en === '🖥 You were moved to VIP-01 — the session continues · VIP rate 27.50 zł/h'
    && as.pl === '🖥 Przesadzono cię na VIP-01 — sesja trwa dalej · stawka VIP 27,50 zł/h', JSON.stringify(as));
  const ver = await a.evaluate(async () => ({ app: APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  z.check('APP_VER в app.html поднят и равен app_version манифеста (иначе телефоны залипнут на старой версии)',
    ver.app === ver.mf && ver.app !== '2026-09-27.1', JSON.stringify(ver));
  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
