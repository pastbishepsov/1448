// З19-и5 «Шапка киоска без агента» (из «Найдено по дороге» З16-и1е; решение основателя 07.11), браузер. Киоск PC-05 на
// 1440×900 и 1920×1080 — с предупреждением «⚠ агент не найден…» (агента на стенде нет) и без него (агент — подмена
// /ping в браузере), и на 1280×800 с предупреждением. Ряд шапки — на экране целиком: вбок не прокручивается, «Сенса»,
// ⓘ, «Админ» и ⏻ видны и нажимаются; числа опыта — каждое одной строкой и не наезжают друг на друга (как есть, с
// «2 230 / 2 600 XP» и по-польски); предупреждение — своей строкой под рядом; без предупреждения — один ряд, шапка не
// выше 70 px, на 1920 опыт — в одну строку. Гость Guest49 за PC-05 (API), кошелёк докладывается до 30 zł; в конце
// сессия завершается.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const G = 'Guest49', g = z.uid(G);
  const pcId = z.sql("SELECT id FROM computers WHERE name='PC-05'");
  for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`)) // хвосты
    await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
  z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
    + "(SELECT computer_id FROM sessions WHERE status='active')");
  const w0 = +z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  if (w0 < 3000) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: (3000 - w0) / 100, method: 'card' }, 'boss');
  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  const sid = seat.data.session_id;
  z.atEnd(async () => {
    if (sid && z.sql(`SELECT status FROM sessions WHERE id='${sid}'`) === 'active') await z.api('POST', `/admin/sessions/${sid}/end`, {}, 'admin1');
  });
  if (sid) z.sql(`UPDATE sessions SET ready_at = now() WHERE id='${sid}'`);
  z.check('Guest49 за PC-05 (API) — 201', seat.status === 201 && !!sid, `${seat.status} ${JSON.stringify(seat.data).slice(0, 160)}`);

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const AGENT_OK = { 'Access-Control-Allow-Origin': '*' };
  async function kiosk(wd, ht, agent) {
    const ctx = await br.newContext({ viewport: { width: wd, height: ht } });
    if (agent) await ctx.route('http://127.0.0.1:1448/**', r => { // живой агент ПК: /ping со своим номером, пустой /apps-status
      const path = new URL(r.request().url()).pathname;
      const body = path === '/ping' ? { computer_id: pcId } : path === '/apps-status' ? { apps: {} } : null;
      return r.fulfill(body ? { status: 200, contentType: 'application/json', headers: AGENT_OK, body: JSON.stringify(body) }
        : { status: 404, headers: AGENT_OK, body: '' });
    });
    const p = await ctx.newPage();
    p.on('pageerror', e => z.jsErrors.push(`киоск ${wd}: ` + e.message));
    await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
    await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
    await p.waitForFunction(() => typeof me === 'object' && !!me && !document.getElementById('shellView').classList.contains('hidden')
      && /\d/.test(document.getElementById('xpLeft').textContent), null, { timeout: 15000 }).catch(() => {});
    await p.waitForFunction(a => agentOnline === a, agent, { timeout: 6000 }).catch(() => {});
    await p.waitForTimeout(600);
    return { ctx, p };
  }
  // замер шапки; probe — подставить числа опыта и язык предупреждения: null — как есть, 'big' — «2 230 / 2 600 XP», 'pl'
  const measure = (p, probe) => p.evaluate(probe => {
    const tb = document.querySelector('.topbar'), dot = document.getElementById('agentDot');
    const xt = document.getElementById('xpText'), xl = document.getElementById('xpLeft');
    if (probe) {
      xt.textContent = '2 230 / 2 600 XP';
      const S = probe === 'pl' ? Object.assign({}, SH_RU, SH_PL) : STR; // словари киоска — SH_RU/SH_EN/SH_PL
      xl.textContent = String(S.xp_left).replace('{n}', (370).toLocaleString('ru-RU'));
      if (!agentOnline) dot.innerHTML = UI.monitor + ' ' + esc(S.tb_agent_warn);
    }
    const vis = e => getComputedStyle(e).display !== 'none' && e.getBoundingClientRect().width > 0;
    const R = e => { const q = e.getBoundingClientRect(); return { l: Math.round(q.left), t: Math.round(q.top), r: Math.round(q.right), b: Math.round(q.bottom) }; };
    const row = [...tb.children].filter(e => e !== dot && vis(e)).map(R);
    const btns = [...tb.querySelectorAll(':scope > button')];
    const reach = btns.map(b => { const q = b.getBoundingClientRect(), hit = document.elementFromPoint(q.left + q.width / 2, q.top + q.height / 2); return !!hit && b.contains(hit); });
    // строки текста — по прямоугольникам Range (спаны — flex-элементы: у самих getClientRects всегда один)
    const lines = e => { const g = document.createRange(); g.selectNodeContents(e);
      return new Set([...g.getClientRects()].filter(q => q.width > 0).map(q => Math.round(q.top))).size; };
    const a = R(xt), c = R(xl);
    return { W: innerWidth, sw: tb.scrollWidth, cw: tb.clientWidth, h: Math.round(tb.getBoundingClientRect().height),
      oneRow: Math.max(...row.map(q => q.t)) < Math.min(...row.map(q => q.b)), maxR: Math.max(...row.map(q => q.r)),
      rowB: Math.max(...row.map(q => q.b)), reach: reach.filter(Boolean).length, nBtn: btns.length,
      warn: vis(dot) && dot.classList.contains('warn'), dot: vis(dot) ? R(dot) : null,
      labels: [...tb.querySelectorAll('.res .rl')].filter(vis).length,
      xpLines: [lines(xt), lines(xl)], same: Math.abs(a.t - c.t) < 3,
      overlap: !(a.r <= c.l || c.r <= a.l || a.b <= c.t || c.b <= a.t), xp: xt.textContent + ' · ' + xl.textContent };
  }, probe);
  const rowOk = m => m.oneRow && m.sw <= m.cw && m.maxR <= m.W && m.nBtn === 4 && m.reach === 4;
  const xpOk = m => m.xpLines[0] === 1 && m.xpLines[1] === 1 && !m.overlap;
  const J = (...ms) => JSON.stringify(ms).slice(0, 900);

  for (const [wd, ht] of [[1440, 900], [1920, 1080]]) {
    console.log(`— ${wd}×${ht}, агента нет — предупреждение`);
    let { ctx, p } = await kiosk(wd, ht, false);
    const m = await measure(p, null);
    await z.shot(p, `z19_kiosk_${wd}_warn`);
    const mb = await measure(p, 'big'), mp = await measure(p, 'pl');
    z.check(`${wd}×${ht} с предупреждением: ряд шапки — один ряд и на экране целиком (вбок не прокручивается; «Сенса», ⓘ, «Админ» и ⏻ видны и нажимаются)`,
      m.warn && rowOk(m) && rowOk(mb) && rowOk(mp), J(m, mb, mp));
    z.check(`${wd}×${ht}: предупреждение — своей строкой под рядом шапки, на экране целиком`,
      m.warn && !!m.dot && m.dot.t >= m.rowB - 1 && m.dot.l >= 0 && m.dot.r <= m.W, J(m));
    z.check(`${wd}×${ht}: числа опыта — каждое одной строкой, не наезжают друг на друга («${m.xp}», «2 230 / 2 600 XP · до уровня: 370 XP», по-польски)`,
      xpOk(m) && xpOk(mb) && xpOk(mp), J(m, mb, mp));
    await ctx.close();

    console.log(`— ${wd}×${ht}, агент на месте — без предупреждения`);
    ({ ctx, p } = await kiosk(wd, ht, true));
    const n = await measure(p, null);
    await z.shot(p, `z19_kiosk_${wd}_ok`);
    const nb = await measure(p, 'big'), np = await measure(p, 'pl');
    z.check(`${wd}×${ht} без предупреждения: предупреждения нет, шапка — один ряд на экране, не выше 70 px`,
      !n.warn && !n.dot && rowOk(n) && rowOk(nb) && rowOk(np) && n.h <= 70 && nb.h <= 70 && np.h <= 70, J(n, nb, np));
    z.check(`${wd}×${ht}: числа опыта — каждое одной строкой, без наложения` + (wd === 1920 ? ', в одну строку рядом' : '')
      + ' (и с «2 230 / 2 600 XP», и по-польски)',
      xpOk(n) && xpOk(nb) && xpOk(np) && (wd !== 1920 || (n.same && nb.same && np.same)), J(n, nb, np));
    await ctx.close();
  }

  console.log('— 1280×800 (1920×1080 при масштабе 150%), агента нет');
  const { ctx, p } = await kiosk(1280, 800, false);
  const s = await measure(p, null), sb = await measure(p, 'big');
  await z.shot(p, 'z19_kiosk_1280_warn');
  z.check('1280×800 с предупреждением: ряд шапки на экране целиком, кнопки нажимаются, предупреждение — строкой под рядом, '
    + 'опыт без наложения; подписей «монеты»/«очки» нет (значок и подсказка остаются)',
    s.warn && rowOk(s) && rowOk(sb) && !!s.dot && s.dot.t >= s.rowB - 1 && xpOk(s) && xpOk(sb) && s.labels === 0, J(s, sb));
  await ctx.close();

  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
