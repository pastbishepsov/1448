// З16-и1д «Кнопка паузы в PWA», браузер (из «Найдено по дороге» З16-и1г, решение основателя 01.11). Паузы нет, пока
// владелец её не включил (К4-и4): кнопка «⏸ Пауза» в карточке «Сессия идёт» PWA — только когда пауза включена и это уже
// известно, как в киоске. Раньше сессии, пришедшие раньше настроек клуба (/me/economy), рисовали карточку с кнопкой, и
// она так и оставалась. Порядок ответов задаётся задержкой /me/economy на 1,5 с. Гость Guest47: кошелёк докладывается до
// 30 zł (пополнение картой от boss), сессия за PC-03 (API), в конце завершается; лимит паузы на время проверки — 0 и 30
// минут, в конце — как был.
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
  const lim0 = z.sql("SELECT value FROM settings WHERE key='pause_limit_min'");
  z.atEnd(() => lim0 !== '' ? z.api('PUT', '/admin/settings', { pause_limit_min: +lim0 }, 'boss')
    : z.sql("DELETE FROM settings WHERE key='pause_limit_min'"));
  const limit = async n => (await z.api('PUT', '/admin/settings', { pause_limit_min: n }, 'boss')).status;
  const w = +z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  if (w < 3000) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: (3000 - w) / 100, method: 'card' }, 'boss');
  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  const sid = seat.data.session_id;
  z.atEnd(async () => {
    if (sid && z.sql(`SELECT status FROM sessions WHERE id='${sid}'`) === 'active') await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  });
  if (sid) z.sql(`UPDATE sessions SET ready_at = now() WHERE id='${sid}'`); // [Готов!] нажат — карточка «Сессия идёт»
  const l0 = await limit(0);
  z.check('Guest47 за PC-03 (API) — 201; лимит паузы 0 — пауза выключена', seat.status === 201 && !!sid && l0 === 200,
    `${seat.status} ${l0} ${JSON.stringify(seat.data).slice(0, 160)}`);

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const BTN = '#liveCard [onclick="appPause()"]', RESUME = '#liveCard [onclick="appResume()"]';
  // вход в PWA гостем; slow — ответ /me/economy (с любым ?client=) на 1,5 с позже: сессии приходят раньше настроек клуба
  const open = async slow => {
    const ctx = await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    const p = await ctx.newPage();
    p.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
    if (slow) await p.route(u => u.pathname.endsWith('/me/economy'), async r => { await new Promise(f => setTimeout(f, 1500)); await r.continue(); });
    await p.goto(z.BASE + '/app', { waitUntil: 'load' });
    await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
    await p.waitForFunction(() => { const c = document.getElementById('liveCard');
      return !!c && !c.classList.contains('hidden') && !!document.getElementById('liveTime'); }, null, { timeout: 15000 }).catch(() => {});
    const early = await p.evaluate(b => ({ econ: !!economy, card: !!document.getElementById('liveTime'), btn: !!document.querySelector(b) }), BTN);
    await p.waitForFunction(() => !!economy, null, { timeout: 15000 }).catch(() => {});
    await p.waitForTimeout(400);
    const late = await p.evaluate(b => ({ limit: economy ? economy.pause_limit_min : null, btn: !!document.querySelector(b) }), BTN);
    return { ctx, p, early, late };
  };

  let r = await open(true);
  z.check('пауза выключена, настройки клуба пришли позже сессий: карточка «Сессия идёт» до них — без «⏸ Пауза», после — тоже',
    r.early.card && !r.early.econ && !r.early.btn && r.late.limit === 0 && !r.late.btn, JSON.stringify([r.early, r.late]));
  await z.shot(r.p, 'z16_pause_off');
  await r.ctx.close();

  r = await open(false);
  z.check('пауза выключена, обычный вход — кнопки нет', r.early.card && r.late.limit === 0 && !r.late.btn, JSON.stringify([r.early, r.late]));
  const ver = await r.p.evaluate(async () => ({ app: APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  z.check('APP_VER в app.html поднят и равен app_version манифеста (иначе телефоны залипнут на старой версии)',
    ver.app === ver.mf && ver.app !== '2026-10-31.1', JSON.stringify(ver));
  await r.ctx.close();

  const l30 = await limit(30);
  r = await open(true);
  z.check('пауза включена (30 мин), настройки позже сессий: до них кнопки нет — ещё неизвестно; пришли — «⏸ Пауза» появилась '
    + 'без перезагрузки', l30 === 200 && r.early.card && !r.early.econ && !r.early.btn && r.late.limit === 30 && r.late.btn,
    JSON.stringify([l30, r.early, r.late]));
  await z.shot(r.p, 'z16_pause_on');
  if (r.late.btn) await r.p.click(BTN);
  const paused = await r.p.waitForFunction(s => !!document.querySelector(s), RESUME, { timeout: 10000 }).then(() => true).catch(() => false);
  const ps = sid ? z.sql(`SELECT paused_at IS NOT NULL FROM sessions WHERE id='${sid}'`) : '';
  if (paused) await r.p.click(RESUME);
  const back = await r.p.waitForFunction(s => !!document.querySelector(s), BTN, { timeout: 10000 }).then(() => true).catch(() => false);
  const rs = sid ? z.sql(`SELECT paused_at IS NULL FROM sessions WHERE id='${sid}'`) : '';
  z.check('«⏸ Пауза» → сессия на паузе, в карточке «Продолжить»; «Продолжить» → время снова идёт, «⏸ Пауза» на месте',
    paused && ps === 't' && back && rs === 't', JSON.stringify({ paused, ps, back, rs }));
  await r.ctx.close();
  z.check('ошибок JS в PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
