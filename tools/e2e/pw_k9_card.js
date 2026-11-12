// К9-и1 «Сколько осталось» (KIOSK.md, этап VII), браузер. Киоск PC-07 (Standard), гость Guest46; сессия идёт 47 минут —
// начало сдвинуто в базе, биллинг её досчитал. Карточка идущей сессии: «Денег хватит на / Хватит на» = minutes_left
// карточки гостя у админа, минута в минуту, в четырёх случаях (только кошелёк; минуты на счету; пакет; скидка гостя —
// вторая сессия), «до» карточки = «до» в чипе кошелька = until админа; верхняя строка — ПК, зона, тариф, скидка;
// «потрачено» = списано по сессии; «≈ XP» = формула начисления и число карточки PWA; числа обновляются сами раз в
// секунду; чужая бронь раньше денег (дедлайн подставлен на странице) — своя подпись и минуты до брони; на 1440×900,
// 1920×1080 и 2560×1440 по-русски, по-английски и по-польски с предельными числами («23 ч 59 мин», «1439,00 zł»,
// скидка, длинное название зоны) строки карточки — в одну строку и не вылезают; «Завершить» — серая, первое нажатие
// не завершает, второе завершает. Всё, что проверка меняет у гостя (минуты, скидка, пакет), в конце возвращается.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07', NOTE = 'e2e К9-и1';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const zone = z.sql(`SELECT z.name FROM zones z JOIN computers c ON c.zone_id = z.id WHERE c.id='${pcId}'`);
  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const voidPacks = async () => {
    for (const id of z.rows(`SELECT id FROM user_packages WHERE user_id='${g}' AND voided_at IS NULL AND comp_reason='${NOTE}'`))
      await z.api('POST', `/admin/user-packages/${id}/void`, {}, 'boss');
  };
  const [cm0, pct0] = z.sql(`SELECT coin_minutes || '|' || payment_increase_pct FROM users WHERE id='${g}'`).split('|');
  z.atEnd(async () => { // гостя — как был
    await endTails(); await voidPacks();
    z.sql(`UPDATE users SET coin_minutes=${cm0}, payment_increase_pct=${pct0} WHERE id='${g}'`);
  });
  await endTails(); await voidPacks();
  z.sql(`UPDATE users SET coin_minutes=0, payment_increase_pct=0 WHERE id='${g}'`);
  const w0 = +z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  if (w0 < 9000) await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: (9000 - w0) / 100, method: 'card' }, 'boss');

  // посадка админом; back — «сессия идёт уже столько»: начало сдвигается в базе, биллинг досчитывает минуты сам
  const seat = async back => {
    const r = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
    const sid = r.data && r.data.session_id;
    if (sid) z.sql(`UPDATE sessions SET started_at = now() - interval '${back}', ready_at = now() - interval '${back}' WHERE id='${sid}'`);
    return { r, sid };
  };
  const { r: seat1, sid } = await seat('47 minutes 10 seconds');
  for (let i = 0; i < 50 && sid && +z.sql(`SELECT billed_minutes FROM sessions WHERE id='${sid}'`) < 47; i++) await z.sleep(1000);
  const billed0 = sid ? +z.sql(`SELECT billed_minutes FROM sessions WHERE id='${sid}'`) : -1;
  z.check(`${G} за ${PC} (API) — 201; сессия «идёт 47 минут», биллинг их досчитал`, seat1.status === 201 && billed0 >= 47,
    `${seat1.status} ${JSON.stringify(seat1.data).slice(0, 160)} · учтено минут: ${billed0}`);

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const AGENT_OK = { 'Access-Control-Allow-Origin': '*' };
  async function kiosk() { // киоск с живым агентом ПК (подмена /ping в браузере), вход гостем
    const ctx = await br.newContext({ viewport: { width: 1920, height: 1080 } });
    await ctx.route('http://127.0.0.1:1448/**', r => {
      const path = new URL(r.request().url()).pathname;
      const body = path === '/ping' ? { computer_id: pcId } : path === '/apps-status' ? { apps: {} } : null;
      return r.fulfill(body ? { status: 200, contentType: 'application/json', headers: AGENT_OK, body: JSON.stringify(body) }
        : { status: 404, headers: AGENT_OK, body: '' });
    });
    const p = await ctx.newPage();
    p.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
    await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
    await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
    await p.waitForFunction(() => typeof me === 'object' && !!me && !!activeSession
      && !document.getElementById('shellView').classList.contains('hidden'), null, { timeout: 15000 }).catch(() => {});
    await p.waitForSelector('#scLeft', { timeout: 8000 }).catch(() => {});
    await p.waitForTimeout(400);
    return { ctx, p };
  }

  // ── помощники ──
  const find = (o, k) => { // объект с полем k где угодно в ответе
    if (!o || typeof o !== 'object') return null;
    if (k in o) return o;
    for (const v of Object.values(o)) { const f = find(v, k); if (f) return f; }
    return null;
  };
  const adm = async () => find((await z.api('GET', `/admin/users/${g}`, undefined, 'admin1')).data, 'minutes_left') || {}; // число админа
  const card = p => p.evaluate(() => {
    const t = id => { const e = document.getElementById(id); return e ? e.textContent : null; };
    const hid = id => { const e = document.getElementById(id); return !e || e.classList.contains('hidden'); };
    return { lbl: t('scLbl'), left: t('scLeft'), till: hid('scTillBox') ? null : t('scTill'), chip: t('walletEta'),
      where: t('scWhere'), rate: t('scRate'), off: hid('scOff') ? null : t('scOff'),
      spent: hid('scSpentBox') ? null : t('scSpent'), xp: hid('scXpBox') ? null : t('scXp'), play: t('sessTime'),
      mins: (me.coin_minutes || 0) + (zoneMinutesHere || 0) + (packMinutesHere || 0), pack: packMinutesHere || 0,
      base: activeSession.base_rate_pln, eff: activeSession.effective_rate_pln };
  });
  const mins = s => { const m = /^(?:(\d+) ч )?(\d+) мин$/.exec(s || ''); return m ? (+m[1] || 0) * 60 + +m[2] : NaN; };
  const hmOf = s => { const m = /(\d\d):(\d\d)/.exec(s || ''); return m ? +m[1] * 60 + +m[2] : NaN; };
  const hm = d => { const x = new Date(d); return x.getHours() * 60 + x.getMinutes(); };
  const clock = d => (d ? new Date(d).toTimeString().slice(0, 5) : '—');
  const num = v => { const n = Math.round(v * 100) / 100; return (Number.isInteger(n) ? String(n) : n.toFixed(2)).replace('.', ','); };
  const J = o => JSON.stringify(o).slice(0, 600);
  // киоск опрашивает сервер раз в 10 с — дождаться, пока у него те же учтённые и оплаченные минуты, кошелёк и минуты счёта, что в базе
  const sync = async (p, id) => {
    const v = z.sql(`SELECT s.billed_minutes || '|' || u.wallet_grosz || '|' || u.coin_minutes || '|' || s.money_minutes FROM sessions s JOIN users u `
      + `ON u.id = s.user_id WHERE s.id='${id}'`).split('|').map(Number);
    await p.waitForFunction(([b, w, cm, mm]) => !!activeSession && activeSession.billed_minutes === b && me.wallet_grosz === w
      && (me.coin_minutes || 0) === cm && (activeSession.money_minutes || 0) === mm, v, { timeout: 14000 }).catch(() => {});
  };
  // «хватит на» и «до» карточки против числа админа; до трёх попыток — между опросом киоска и ответом админа сервер
  // может списать очередную минуту
  async function same(p, id, name, wantLbl) {
    let c, a, ok = false;
    for (let i = 0; i < 3 && !ok; i++) {
      await sync(p, id);
      a = await adm(); c = await card(p);
      ok = c.lbl === wantLbl && mins(c.left) === a.minutes_left && hmOf(c.till) === hm(a.until) && hmOf(c.chip) === hmOf(c.till);
    }
    z.check(`${name}: «${c.lbl} ${c.left}», до ${c.till}; чип кошелька «${c.chip}»; у админа — ${a.minutes_left} мин, до ${clock(a.until)}`,
      ok, ok ? '' : J({ c, a }));
    return { c, a };
  }

  const { ctx, p } = await kiosk();
  if (!z.check('в идущей сессии — карточка «на сколько хватит» (#scLeft)', !!(await p.$('#scLeft')))) { await z.done(); return; }
  const L = await p.evaluate(() => { // строки трёх словарей киоска
    const o = {};
    for (const k of ['sc_lbl', 'sc_lbl_mix', 'sc_lbl_bk', 'sc_play', 'sc_spent', 'btn_finish', 'btn_finish_sure', 'eta_booking'])
      o[k] = { ru: SH_RU[k], en: SH_EN[k], pl: SH_PL[k] };
    return o;
  });

  console.log('— только кошелёк');
  let { c, a } = await same(p, sid, 'только кошелёк', L.sc_lbl.ru);
  z.check('у гостя в запасе только деньги — ни минут, ни пакета (иначе случай «только кошелёк» не проверен)', c.mins === 0, J(c));
  z.check(`верхняя строка: «${c.where} · ${c.rate}» — ПК, зона, тариф сессии; скидки нет — значка «−%» нет`,
    c.where === `${PC} · ${zone}` && c.rate === num(a.rate_pln) + ' zł/ч' && c.eff === a.rate_pln && c.base === c.eff && c.off === null, J({ c, a }));
  await z.shot(p, 'k9_card_1920');

  console.log('— минуты на счету');
  z.sql(`UPDATE users SET coin_minutes = 95 WHERE id='${g}'`);
  await p.waitForFunction(() => (me.coin_minutes || 0) > 0, null, { timeout: 14000 }).catch(() => {});
  await same(p, sid, 'минуты на счету (coin_minutes) + кошелёк', L.sc_lbl_mix.ru);

  console.log('— пакет');
  z.sql(`UPDATE users SET coin_minutes = 0 WHERE id='${g}'`);
  const pk = z.sql(`SELECT id FROM time_packages WHERE active AND spend_days_mask = 127 AND spend_start_min = 0 AND spend_end_min = 1440 `
    + `AND zone_id = (SELECT zone_id FROM computers WHERE id='${pcId}') ORDER BY minutes LIMIT 1`);
  const iss = await z.api('POST', `/admin/users/${g}/packages`, { package_id: pk, method: 'comp', comp_reason: NOTE }, 'boss');
  await p.waitForFunction(() => (packMinutesHere || 0) > 0 && !(me.coin_minutes > 0), null, { timeout: 14000 }).catch(() => {});
  ({ c } = await same(p, sid, 'пакет минут + кошелёк', L.sc_lbl_mix.ru));
  z.check('пакет выдан и киоск его видит', iss.status < 300 && c.pack > 0, `${iss.status} ${J(iss.data)} · минут пакета на киоске: ${c.pack}`);
  await voidPacks();
  await p.waitForFunction(() => !(packMinutesHere > 0), null, { timeout: 14000 }).catch(() => {});

  console.log('— потрачено, опыт, живые числа');
  await sync(p, sid);
  c = await card(p);
  const dbCh = +z.sql(`SELECT charged_grosz FROM sessions WHERE id='${sid}'`);
  z.check(`«потрачено ${c.spent}» = списано по сессии (в базе ${dbCh} гр); «Играешь ${c.play}»`,
    dbCh > 0 && c.spent === (dbCh / 100).toFixed(2).replace('.', ',') + ' zł' && /^4[7-9]:\d\d$|^5\d:\d\d$/.test(c.play), J(c));
  const x = await p.evaluate(() => { // тот же расчёт, что у сервера (sessionRewards) — здесь заново, не через calcCompute
    paintSessCard();
    const m = Math.floor(effectiveSec() / 60), e = economy, boost = e.talents.xp_boost || 0;
    return { m, boost, shown: document.getElementById('scXp').textContent,
      want: Math.round(m * e.rates.xp_per_min * (1 + boost) * e.rank.xp_mult / 5) * 5 };
  });
  z.check(`«≈ ${x.shown}» = формула начисления за ${x.m} мин (${x.want} XP)`, x.want > 0 && x.shown.replace(/\D/g, '') === String(x.want), J(x));
  const pctx = await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true }); // PWA того же гостя
  const pw = await pctx.newPage();
  pw.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await pw.goto(z.BASE + '/app', { waitUntil: 'load' });
  await pw.fill('#nick', G); await pw.fill('#pass', z.GUEST_PASS); await pw.click('#authBtn');
  await pw.waitForFunction(() => { const e = document.getElementById('liveXP'); return !!e && Number(e.dataset.v || 0) > 0; },
    null, { timeout: 15000 }).catch(() => {});
  await pw.waitForTimeout(900);
  const kx = () => p.evaluate(() => { paintSessCard(); return document.getElementById('scXp').textContent.replace(/\D/g, ''); });
  // Минута может смениться между чтениями: киоск пересчитывает сразу, PWA — на своём тике (раз в секунду) и дописывает
  // цифру с анимацией. Поэтому не одно чтение, а до шести секунд ожидания, пока цифры сойдутся
  const pxOf = () => pw.evaluate(() => { const e = document.getElementById('liveXP'); return e ? String(Number(e.dataset.v || 0)) : null; });
  let k1, px, k2, tries = 0;
  for (; tries < 12; tries++) { k1 = await kx(); px = await pxOf(); k2 = await kx(); if (px === k1 || px === k2) break; await z.sleep(500); }
  await pctx.close();
  z.check(`«≈ XP» киоска = «≈ XP» карточки сессии в PWA (${px}); у гостя нет таланта на опыт`,
    x.boost === 0 && (px === k1 || px === k2), J({ k1, px, k2, boost: x.boost, tries }));
  let ticked = false; // числа обновляются сами: меняем кошелёк в памяти страницы и ничего не перерисовываем
  for (let i = 0; i < 2 && !ticked; i++) {
    const before = (await card(p)).left;
    await p.evaluate(() => { me.wallet_grosz += 6000; });
    ticked = await p.waitForFunction(b => document.getElementById('scLeft').textContent !== b, before, { timeout: 2500 }).then(() => true, () => false);
    await p.evaluate(() => { me.wallet_grosz -= 6000; renderWallet(); });
  }
  z.check('числа карточки обновляются сами — раз в секунду, вместе с таймером', ticked);

  console.log('— чужая бронь раньше денег (дедлайн подставлен на странице)');
  const bk = await p.evaluate(() => {
    const keep = bookingDeadline, d = new Date(Date.now() + 25 * 60000 - 20000), t = id => document.getElementById(id).textContent;
    bookingDeadline = d.toISOString(); renderWallet();
    const r = { lbl: t('scLbl'), left: t('scLeft'), till: t('scTill'), chip: t('walletEta'), want: fmtClock(d) };
    bookingDeadline = keep; renderWallet();
    return r;
  });
  z.check(`бронь через 25 минут: «${bk.lbl} ${bk.left}», до ${bk.till}; чип кошелька «${bk.chip}»`,
    bk.lbl === L.sc_lbl_bk.ru && bk.left === '25 мин' && bk.till === bk.want && bk.chip.includes(bk.want)
    && bk.chip.includes(L.eta_booking.ru.trim()), J(bk));

  // ── три экрана × три языка, предельные числа: 23 ч 59 мин, скидка, 1439,00 zł, опыт за сутки ──
  const probe = (lang, o) => p.evaluate(([lang, o]) => {
    const keep = { STR, w: me.wallet_grosz, s: Object.assign({}, activeSession), bd: bookingDeadline };
    STR = Object.assign({}, SH_RU, lang === 'en' ? SH_EN : lang === 'pl' ? SH_PL : {});
    Object.assign(activeSession, { base_rate_pln: 25, effective_rate_pln: 22.5, charged_grosz: 143900, paused_total_sec: 0,
      billed_minutes: null, started_at: new Date(Date.now() - (23 * 60 + 59) * 60000 - 30000).toISOString() });
    if (o.long) activeSession.zone_name = 'Bootcamp VIP Arena Premium Lounge';
    me.wallet_grosz = 53963; // 1439 минут при 22,50 zł/ч
    if (o.bk) bookingDeadline = new Date(Date.now() + 1439 * 60000 - 20000).toISOString();
    renderSession(); renderWallet();
    const cardEl = document.getElementById('sessCard'), q = s => cardEl.querySelector(s), R = e => e.getBoundingClientRect(), cr = R(cardEl);
    const fits = e => e.scrollWidth <= e.clientWidth + 1, one = e => R(e).height < parseFloat(getComputedStyle(e).fontSize) * 2;
    const tx = s => q(s).textContent.replace(/\s+/g, ' ').trim();
    const m = { w: Math.round(cr.width),
      wide: ['.sc-top', '.sc-main', '.sc-row', '.sc-btns'].filter(s => !fits(q(s))).concat(fits(cardEl) ? [] : ['#sessCard']),
      wrap: ['.sc-top', '#scLbl', '#scLeft', '.sc-row'].filter(s => !one(q(s))),
      out: ['#scRate', '#scOff', '#scTillBox', '.sc-end'].filter(s => R(q(s)).right > cr.right + .5 || R(q(s)).left < cr.left - .5),
      cut: q('#scWhere').scrollWidth > q('#scWhere').clientWidth, lbl: tx('#scLbl'), left: tx('#scLeft'), rate: tx('#scRate'),
      off: tx('#scOff'), row: tx('.sc-row'), raw: /undefined|NaN|sc_|dur_/.test(cardEl.innerText) };
    STR = keep.STR; me.wallet_grosz = keep.w; activeSession = keep.s; bookingDeadline = keep.bd;
    renderSession(); renderWallet();
    return m;
  }, [lang, o]);
  const LEFT = { ru: '23 ч 59 мин', en: '23 h 59 min', pl: '23 h 59 min' }, RATE = { ru: '22,50 zł/ч', en: '22,50 zł/h', pl: '22,50 zł/h' };
  for (const [wd, ht] of [[1440, 900], [1920, 1080], [2560, 1440]]) {
    await p.setViewportSize({ width: wd, height: ht });
    await p.waitForTimeout(250);
    const bad = []; let w = 0, sample = '';
    for (const lang of ['ru', 'en', 'pl']) for (const o of [{}, { bk: true }, { long: true }]) {
      const m = await probe(lang, o); w = m.w;
      if (lang === 'pl' && !o.bk && !o.long) sample = m.row;
      const ok = !m.wide.length && !m.wrap.length && !m.out.length && !m.raw && m.cut === !!o.long && m.left === LEFT[lang] && m.rate === RATE[lang]
        && m.off === '−10%' && m.lbl === (o.bk ? L.sc_lbl_bk : L.sc_lbl)[lang] && m.row.startsWith(L.sc_play[lang] + ' 23:59:')
        && m.row.includes(L.sc_spent[lang] + ' 1439,00 zł') && /≈ \+\d[\d\s\u00a0\u202f]* XP$/.test(m.row);
      if (!ok) bad.push({ lang, o, m });
    }
    z.check(`${wd}×${ht}, карточка ${w} px: RU/EN/PL с предельными числами, с бронью и с длинной зоной — строки в одну строку, `
      + `ничего не вылезает, сырых ключей нет (PL: «${sample}»)`, bad.length === 0, J(bad));
  }
  await p.setViewportSize({ width: 1920, height: 1080 });
  await p.waitForTimeout(250);

  console.log('— «Завершить»');
  const END = '#sessCard .sc-end';
  const b1 = await p.evaluate(s => { const b = document.querySelector(s);
    return { cls: b.className, txt: b.textContent, red: document.querySelectorAll('#sessCard .btn.danger').length }; }, END);
  await p.click(END, { timeout: 5000 }).catch(e => z.jsErrors.push('нажатие «Завершить»: ' + e.message.split('\n')[0]));
  await p.waitForTimeout(700);
  const b2 = await p.evaluate(s => { const b = document.querySelector(s); return b ? { txt: b.textContent, armed: b.classList.contains('armed') } : {}; }, END);
  const st1 = z.sql(`SELECT status FROM sessions WHERE id='${sid}'`);
  z.check(`«${b1.txt}» — серая, красных кнопок в карточке нет; первое нажатие не завершает: кнопка — «${b2.txt}», сессия ${st1}`,
    /\bghost\b/.test(b1.cls) && !/\bdanger\b/.test(b1.cls) && b1.red === 0 && b1.txt === L.btn_finish.ru
    && b2.txt === L.btn_finish_sure.ru && b2.armed === true && st1 === 'active', J({ b1, b2 }));
  await p.click(END, { timeout: 5000 }).catch(e => z.jsErrors.push('второе нажатие «Завершить»: ' + e.message.split('\n')[0]));
  let st2 = 'active';
  for (let i = 0; i < 16 && st2 === 'active'; i++) { await z.sleep(500); st2 = z.sql(`SELECT status FROM sessions WHERE id='${sid}'`); }
  z.check(`второе нажатие завершает сессию (в базе — ${st2})`, st2 !== 'active');
  await ctx.close();

  console.log('— скидка гостя 10%: новая сессия');
  await endTails();
  z.sql(`UPDATE users SET payment_increase_pct = 10 WHERE id='${g}'`);
  const s2 = await seat('5 seconds');
  const k = await kiosk();
  ({ c, a } = await same(k.p, s2.sid, 'скидка гостя 10%, только кошелёк', L.sc_lbl.ru));
  const pct = Math.round((1 - c.eff / c.base) * 100);
  z.check(`скидка в верхней строке: «${c.where} · ${c.rate} ${c.off}» (цена зоны ${c.base}, тариф сессии ${c.eff}); у админа тариф ${a.rate_pln}`,
    s2.r.status === 201 && c.eff < c.base && pct >= 10 && c.off === `−${pct}%` && c.rate === num(c.eff) + ' zł/ч' && c.eff === a.rate_pln, J({ c, a }));
  z.check('сессия только началась: «потрачено» и «≈ XP» не показываются, пока нечего показывать', c.spent === null && c.xp === null, J(c));
  await k.ctx.close();

  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
