// З16-и1е «Суммы в уведомлениях о деньгах», браузер (из «Найдено по дороге» З16-и1д, решение основателя 01.11).
// Киоск и PWA писали сумму в уведомлениях о пополнении, его отмене и долге за кухню и бар как она пришла с сервера —
// «депозит 12.5 zł»; теперь — как ценники: «12,50», «0,34», «30»; монеты в уведомлении о пополнении в киоске — с
// пробелом тысяч, как в PWA. Гость Guest47: кошелёк докладывается до 30 zł (пополнение картой от boss), сессия за PC-03
// (API), киоск PC-03 и PWA входят им; его непрочитанные уведомления в начале помечаются прочитанными; пополнение на
// 12,50 zł картой от boss отменяется им же — кошелёк и монеты как до него; в конце сессия завершается.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const G = 'Guest47', g = z.uid(G);
  const pcId = z.sql("SELECT id FROM computers WHERE name='PC-03'");
  for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND user_id='${g}'`)) // хвосты упавшего прогона
    await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
  z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
    + "(SELECT computer_id FROM sessions WHERE status='active')");
  const w = +z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  if (w < 3000) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: (3000 - w) / 100, method: 'card' }, 'boss');
  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  const sid = seat.data.session_id;
  z.atEnd(async () => {
    if (sid && z.sql(`SELECT status FROM sessions WHERE id='${sid}'`) === 'active') await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  });
  if (sid) z.sql(`UPDATE sessions SET ready_at = now() WHERE id='${sid}'`);
  z.sql(`UPDATE notifications SET read_at = now(), read_kiosk_at = now(), read_app_at = now() WHERE user_id='${g}' AND read_at IS NULL`);
  z.check('Guest47 за PC-03 (API) — 201', seat.status === 201 && !!sid, `${seat.status} ${JSON.stringify(seat.data).slice(0, 160)}`);

  const num = v => { const n = Math.round((+v || 0) * 100) / 100; return (Number.isInteger(n) ? String(n) : n.toFixed(2)).replace('.', ','); };
  const last = type => JSON.parse(z.sql(`SELECT payload::text FROM notifications WHERE user_id='${g}' AND type='${type}' `
    + 'ORDER BY created_at DESC LIMIT 1') || '{}');
  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const toastIs = (p, want, ms) => p.waitForFunction(w => { const t = document.getElementById('toast'); return !!t && t.textContent === w; },
    want, { timeout: ms }).then(() => true).catch(() => false);

  // киоск PC-03: тосты — с ближайшим опросом уведомлений (раз в 12 с)
  const k = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await k.fill('#nick', G); await k.fill('#pass', z.GUEST_PASS); await k.click('#authBtn');
  await k.waitForFunction(() => typeof me === 'object' && !!me, null, { timeout: 15000 }).catch(() => {});
  const dep = await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: 12.5, method: 'card' }, 'boss');
  const pd = last('deposit');
  const coins = await k.evaluate(c => (c || 0).toLocaleString('ru-RU'), pd.coins);
  const WANT_DEP = `💰 +${coins} монеток — депозит 12,50 zł`; // валюта у гостя — «монетки»: киоск с К11-и1, PWA с К11-и3
  const kDep = await toastIs(k, WANT_DEP, 25000);
  z.check(`пополнение на 12,50 zł картой — киоск: «${WANT_DEP}» (было «депозит 12.5 zł»)`,
    dep.status === 201 && pd.amount_pln === 12.5 && kDep, `${dep.status} ${JSON.stringify(pd)} · ${await z.text(k, '#toast')}`);
  await z.shot(k, 'z16_amounts_kiosk');
  const vd = await z.api('POST', `/admin/deposits/${dep.data.deposit_id}/void`, {}, 'boss');
  const pv = last('deposit_void');
  const WANT_VOID = `↩ Депозит 12,50 zł отменён у стойки — с кошелька снято ${num(pv.refund_pln)} zł, в кошельке ${num(pv.wallet_pln)} zł`;
  const kVoid = await toastIs(k, WANT_VOID, 30000);
  z.check(`отмена пополнения — киоск: «${WANT_VOID}»`, vd.status === 200 && pv.amount_pln === 12.5 && kVoid,
    `${vd.status} ${JSON.stringify(pv)} · ${await z.text(k, '#toast')}`);
  const ks = await k.evaluate(() => {
    const n = (type, payload) => notifText({ type, payload });
    const out = { dep: n('deposit', { coins: 1500, amount_pln: 12.5 }), want: '💰 +' + (1500).toLocaleString('ru-RU') + ' монеток — депозит 12,50 zł',
      void: n('deposit_void', { amount_pln: 0.34, refund_pln: 0.34, wallet_pln: 30 }),
      kt: n('kitchen_unpaid', { total_pln: 7.5, bar_pln: 0 }), bar: n('kitchen_unpaid', { total_pln: 7.5, bar_pln: 3 }) };
    const save = STR;
    for (const [l, d] of [['en', SH_EN], ['pl', SH_PL]]) { STR = Object.assign({}, SH_RU, d); out[l] = n('deposit', { coins: 5, amount_pln: 0.34 }); }
    STR = save;
    return out;
  });
  z.check('киоск: «+1 500 монеток — депозит 12,50 zł», отмена «0,34 … 30 zł», долг «7,50 zł» (и с баром); EN «top-up 0,34 zł», PL «wpłata 0,34 zł»',
    ks.dep === ks.want && ks.void === '↩ Депозит 0,34 zł отменён у стойки — с кошелька снято 0,34 zł, в кошельке 30 zł'
    && ks.kt === '🧾 Не забудь рассчитаться за кухню у стойки: 7,50 zł' && ks.bar === '🧾 Не забудь рассчитаться у стойки: 7,50 zł (кухня и бар)'
    && ks.en === '💰 +5 coins — top-up 0,34 zł' && ks.pl === '💰 +5 monetek — wpłata 0,34 zł', JSON.stringify(ks));

  // PWA: оба уведомления не прочитаны «вообще» (киоск ставит свою отметку) — приходят при входе, в ленте — обе строки
  const a = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })).newPage();
  a.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await a.goto(z.BASE + '/app', { waitUntil: 'load' });
  await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
  const feedHas = await a.waitForFunction(ws => { const f = document.getElementById('ntfFeed'); return !!f && ws.every(x => f.textContent.includes(x)); },
    [WANT_DEP, WANT_VOID], { timeout: 20000 }).then(() => true).catch(() => false);
  const feed = await a.evaluate(() => { const f = document.getElementById('ntfFeed'); return f ? f.textContent : ''; });
  z.check('PWA: в ленте уведомлений — те же строки пополнения и отмены', feedHas, feed.slice(0, 240));
  await z.shot(a, 'z16_amounts_pwa');
  const as = await a.evaluate(() => {
    const n = (type, payload) => ntfText({ type, payload });
    const out = { dep: n('deposit', { coins: 1500, amount_pln: 12.5 }), want: '💰 +' + (1500).toLocaleString('ru-RU') + ' монеток — депозит 12,50 zł',
      void: n('deposit_void', { amount_pln: 0.34, refund_pln: 0.34, wallet_pln: 30 }),
      kt: n('kitchen_unpaid', { total_pln: 7.5, bar_pln: 0 }), bar: n('kitchen_unpaid', { total_pln: 7.5, bar_pln: 3 }) };
    const save = STR;
    for (const [l, d] of [['en', STR_EN], ['pl', STR_PL]]) { STR = Object.assign({}, STR_RU, d); out[l] = n('deposit', { coins: 5, amount_pln: 0.34 }); }
    STR = save;
    return out;
  });
  z.check('PWA: «+1 500 монеток — депозит 12,50 zł», отмена «0,34 … 30 zł», долг «7,50 zł» (и с баром); EN «top-up 0,34 zł», PL «wpłata 0,34 zł»',
    as.dep === as.want && as.void === '↩ Депозит 0,34 zł отменён у стойки — с кошелька снято 0,34 zł, в кошельке 30 zł'
    && as.kt === '🧾 Не забудь рассчитаться за кухню у стойки: 7,50 zł' && as.bar === '🧾 Не забудь рассчитаться у стойки: 7,50 zł (кухня и бар)'
    && as.en === '💰 +5 coins — top-up 0,34 zł' && as.pl === '💰 +5 monetek — wpłata 0,34 zł', JSON.stringify(as));
  const ver = await a.evaluate(async () => ({ app: APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  z.check('APP_VER в app.html поднят и равен app_version манифеста (иначе телефоны залипнут на старой версии)',
    ver.app === ver.mf && ver.app !== '2026-11-01.1', JSON.stringify(ver));
  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
