// К9-и2 «Деньги кончаются» (KIOSK.md, этап VII), браузер. Киоск PC-07 (Standard), гость Guest46, сессия идёт 20 минут.
// Кошелёк в базе подогнан так, чтобы денег хватало ровно на 16 минут: карточка обычная, чип кошелька не красный,
// сервер balance_low ещё не слал (warn15_at пуст). Дальше ничего не трогаем — сервер сам списывает минуту: у админа
// 15 минут, карточка красная («Деньги кончаются — осталось 15 мин», заметка «Пополнить — у стойки · игра не
// прервётся» — текст, не кнопка), чип кошелька красный, warn15_at заполнен: порог один на всех. В perf-lite рамка
// не мигает. Красная карточка на 1440×900, 1920×1080 и 2560×1440 по-русски, по-английски и по-польски (только
// кошелёк, минуты на счету, чужая бронь) не разваливается. Пауза — не красная; чужая бронь через 9 минут — красная,
// но без «Пополнить». Пополнение у стойки (50 zł) возвращает обычный вид без перезагрузки страницы. Минуты на счету
// при пустом кошельке — «Время кончается». Всё, что проверка меняет у гостя (кошелёк, минуты, скидка, пополнение),
// в конце возвращается.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const [w0, cm0, pct0] = z.sql(`SELECT wallet_grosz || '|' || coin_minutes || '|' || payment_increase_pct FROM users WHERE id='${g}'`).split('|');
  let dep = null;
  z.atEnd(async () => { // гостя — как был: сессию закрыть, пополнение отменить, кошелёк, минуты и скидку вернуть
    await endTails();
    if (dep) {
      z.sql(`UPDATE users SET wallet_grosz = ${+w0 + 5000} WHERE id='${g}'`);
      await z.api('POST', `/admin/deposits/${dep}/void`, {}, 'boss');
    }
    z.sql(`UPDATE users SET wallet_grosz=${w0}, coin_minutes=${cm0}, payment_increase_pct=${pct0} WHERE id='${g}'`);
  });
  await endTails();
  z.sql(`UPDATE users SET wallet_grosz=9000, coin_minutes=0, payment_increase_pct=0 WHERE id='${g}'`);

  // посадка админом; «сессия идёт уже 20 минут»: начало сдвигается в базе, биллинг досчитывает минуты сам
  const BACK = '20 minutes 10 seconds';
  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  const sid = seat.data && seat.data.session_id;
  if (sid) z.sql(`UPDATE sessions SET started_at = now() - interval '${BACK}', ready_at = now() - interval '${BACK}' WHERE id='${sid}'`);
  const billed = () => (sid ? +z.sql(`SELECT billed_minutes FROM sessions WHERE id='${sid}'`) : -1);
  for (let i = 0; i < 50 && sid && billed() < 20; i++) await z.sleep(1000);
  z.check(`${G} за ${PC} (API) — 201; сессия «идёт 20 минут», биллинг их досчитал`, seat.status === 201 && billed() >= 20,
    `${seat.status} ${JSON.stringify(seat.data).slice(0, 160)} · учтено минут: ${billed()}`);

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
    const el = document.getElementById('sessCard'), n = document.getElementById('scNote'), b = n && n.querySelector('b');
    const t = id => { const e = document.getElementById(id); return e ? e.textContent : null; };
    return { red: el.classList.contains('sc-low'), lbl: t('scLbl'), left: t('scLeft'), till: t('scTill'),
      chip: document.getElementById('walletChip').classList.contains('low'),
      note: !n || !b || n.classList.contains('hidden') ? null : b.textContent + ' · ' + n.textContent.slice(b.textContent.length),
      mins: (me.coin_minutes || 0) + (zoneMinutesHere || 0) + (packMinutesHere || 0), k9: window.__k9 || 0 };
  });
  const mins = s => { const m = /^(?:(\d+) ч )?(\d+) мин$/.exec(s || ''); return m ? (+m[1] || 0) * 60 + +m[2] : NaN; };
  const hmOf = s => { const m = /(\d\d):(\d\d)/.exec(s || ''); return m ? +m[1] * 60 + +m[2] : NaN; };
  const hm = d => { const x = new Date(d); return x.getHours() * 60 + x.getMinutes(); };
  const J = o => JSON.stringify(o).slice(0, 700);
  const warned = () => z.sql(`SELECT warn15_at IS NOT NULL FROM sessions WHERE id='${sid}'`) === 't'; // сервер отправил balance_low
  // киоск опрашивает сервер раз в 10 с — дождаться, пока у него те же учтённые и оплаченные минуты, кошелёк и минуты счёта, что в базе
  const sync = async p => {
    const v = z.sql(`SELECT s.billed_minutes || '|' || u.wallet_grosz || '|' || u.coin_minutes || '|' || s.money_minutes FROM sessions s JOIN users u `
      + `ON u.id = s.user_id WHERE s.id='${sid}'`).split('|').map(Number);
    await p.waitForFunction(([b, w, cm, mm]) => !!activeSession && activeSession.billed_minutes === b && me.wallet_grosz === w
      && (me.coin_minutes || 0) === cm && (activeSession.money_minutes || 0) === mm, v, { timeout: 14000 }).catch(() => {});
  };
  // сервер списывает раз в минуту — дождаться очередного списания (учтённых минут стало больше, чем from)
  const waitCharge = async (from, sec) => { for (let i = 0; i < sec * 2 && billed() === from; i++) await z.sleep(500); return billed(); };
  const isRed = p => p.waitForFunction(() => document.getElementById('sessCard').classList.contains('sc-low'), null, { timeout: 14000 }).catch(() => {});

  const { p } = await kiosk();
  if (!z.check('в идущей сессии — карточка с прогнозом и место под заметку «Пополнить» (#scLeft, #scNote)',
    !!(await p.$('#scLeft')) && !!(await p.$('#scNote')))) { await z.done(); return; }
  const L = await p.evaluate(() => { // строки трёх словарей киоска
    const o = {};
    for (const k of ['sc_lbl', 'sc_lbl_bk', 'sc_low', 'sc_low_mix', 'sc_note', 'sc_note2']) o[k] = { ru: SH_RU[k], en: SH_EN[k], pl: SH_PL[k] };
    return o;
  });
  const NOTE = { ru: `${L.sc_note.ru} · ${L.sc_note2.ru}`, en: `${L.sc_note.en} · ${L.sc_note2.en}`, pl: `${L.sc_note.pl} · ${L.sc_note2.pl}` };
  z.check(`тексты красной карточки: «${L.sc_low.ru}», «${NOTE.ru}» — и они есть по-английски и по-польски`,
    L.sc_low.ru === 'Деньги кончаются — осталось' && NOTE.ru === 'Пополнить — у стойки · игра не прервётся'
    && Object.values(L).every(v => ['ru', 'en', 'pl'].every(l => typeof v[l] === 'string' && v[l].length > 3)), J(L));

  console.log('— денег ровно на 16 минут');
  const rate = Math.round(+z.sql(`SELECT effective_rate_pln FROM sessions WHERE id='${sid}'`) * 100);
  const cost = m => Math.floor((rate * m + 59) / 60); // цена минут — как у сервера (costForMinutes)
  const b1 = await waitCharge(billed(), 75); // сразу после списания: до следующего — около минуты
  const paid = +z.sql(`SELECT money_minutes FROM sessions WHERE id='${sid}'`);
  z.sql(`UPDATE users SET wallet_grosz = ${cost(paid + 16) - cost(paid)} WHERE id='${g}'`);
  await sync(p);
  let a = await adm(), c = await card(p);
  const btns0 = await p.evaluate(() => document.querySelectorAll('#sessCard button').length);
  z.check(`денег на 16 минут (у админа — ${a.minutes_left}): карточка обычная — «${c.lbl} ${c.left}», «Пополнить» нет, чип кошелька не красный, `
    + 'сервер balance_low не слал', a.minutes_left === 16 && !c.red && c.lbl === L.sc_lbl.ru && c.left === '16 мин' && c.note === null
    && !c.chip && !warned() && c.mins === 0, J({ c, a, warned: warned(), rate, paid }));
  await z.shot(p, 'k9_low_16');
  await p.evaluate(() => { window.__k9 = 1; }); // метка на странице: дальше она не перезагружается

  console.log('— сервер сам списывает минуту: 16 → 15');
  const b2 = await waitCharge(b1, 75);
  await sync(p); await isRed(p);
  a = await adm(); c = await card(p);
  z.check(`сервер списал минуту (учтено ${b1} → ${b2}), у админа — ${a.minutes_left} мин: карточка красная — «${c.lbl} ${c.left}», до ${c.till}; `
    + 'чип кошелька красный; сервер отправил balance_low — порог один', b2 === b1 + 1 && a.minutes_left === 15 && c.red && c.lbl === L.sc_low.ru
    && c.left === '15 мин' && hmOf(c.till) === hm(a.until) && c.chip && warned(), J({ c, a, warned: warned() }));
  const st = await p.evaluate(() => {
    const el = document.getElementById('sessCard'), n = document.getElementById('scNote'), R = e => e.getBoundingClientRect();
    const r = R(n), e = R(el.querySelector('.sc-end')), cr = R(el), body = document.body, was = body.classList.contains('perf-lite');
    const look = () => { const cs = getComputedStyle(el); return { anim: cs.animationName, iter: cs.animationIterationCount, bg: cs.backgroundImage, frame: cs.boxShadow }; };
    body.classList.remove('perf-lite'); const full = look();
    body.classList.add('perf-lite'); const lite = look();
    body.classList.toggle('perf-lite', was);
    return { tag: n.tagName, inter: n.querySelectorAll('button, a, input, [onclick]').length + (n.hasAttribute('onclick') ? 1 : 0),
      cursor: getComputedStyle(n).cursor, btns: el.querySelectorAll('button').length,
      place: r.height > 0 && r.height < 40 && r.left >= cr.left && r.right <= e.left + .5, full, lite,
      lbl: getComputedStyle(document.getElementById('scLbl')).color, dot: getComputedStyle(el.querySelector('.sc-dot')).backgroundColor };
  });
  z.check(`заметка «${c.note}» — слева от «Завершить», в две строки; это текст, не кнопка (Р7): нажимать нечего, кнопок в карточке не прибавилось`,
    c.note === NOTE.ru && st.tag === 'DIV' && st.inter === 0 && st.cursor !== 'pointer' && st.btns === btns0 && st.place, J(st));
  z.check('красный вид: красные фон и рамка, подпись и точка; рамка мигает три раза и замирает (scLow × 3)',
    /255, 39, 64/.test(st.full.bg) && st.full.anim === 'scLow' && st.full.iter === '3' && st.lbl === 'rgb(255, 154, 166)'
    && st.dot === 'rgb(255, 39, 64)', J(st));
  z.check('perf-lite (слабое железо, «уменьшить движение»): карточка красная, но не мигает',
    st.lite.anim === 'none' && /255, 39, 64/.test(st.lite.bg) && /255, 39, 64/.test(st.lite.frame), J(st.lite));
  await z.shot(p, 'k9_low_15');

  // ── три экрана × три языка: красная карточка с предельными числами — только кошелёк, минуты на счету, чужая бронь ──
  const probe = (lang, o) => p.evaluate(([lang, o]) => {
    const keep = { STR, w: me.wallet_grosz, cm: me.coin_minutes, s: Object.assign({}, activeSession), bd: bookingDeadline };
    STR = Object.assign({}, SH_RU, lang === 'en' ? SH_EN : lang === 'pl' ? SH_PL : {});
    Object.assign(activeSession, { base_rate_pln: 25, effective_rate_pln: 22.5, charged_grosz: 143900, paused_total_sec: 0, money_minutes: 0,
      billed_minutes: null, zone_name: 'Bootcamp VIP Arena Premium Lounge', started_at: new Date(Date.now() - (23 * 60 + 59) * 60000 - 30000).toISOString() });
    me.wallet_grosz = o.mix ? 0 : o.bk ? 53963 : 338; me.coin_minutes = o.mix ? 9 : 0; // 338 гр при 22,50 zł/ч — ровно 9 минут
    if (o.bk) bookingDeadline = new Date(Date.now() + 9 * 60000 - 20000).toISOString();
    renderSession(); renderWallet();
    const cardEl = document.getElementById('sessCard'), q = s => cardEl.querySelector(s), R = e => e.getBoundingClientRect(), cr = R(cardEl);
    const fits = e => e.scrollWidth <= e.clientWidth + 1, one = e => R(e).height < parseFloat(getComputedStyle(e).fontSize) * 2;
    const tx = s => q(s).textContent.replace(/\s+/g, ' ').trim(), n = q('#scNote'), nb = q('#scNote b').textContent, nr = R(n), er = R(q('.sc-end'));
    const shown = !n.classList.contains('hidden');
    const m = { w: Math.round(cr.width), red: cardEl.classList.contains('sc-low'),
      wide: ['.sc-top', '.sc-main', '.sc-row', '.sc-btns'].filter(s => !fits(q(s))).concat(fits(cardEl) ? [] : ['#sessCard']),
      wrap: ['.sc-top', '#scLbl', '#scLeft', '.sc-row'].filter(s => !one(q(s))),
      out: ['#scRate', '#scOff', '#scTillBox', '.sc-end'].filter(s => R(q(s)).right > cr.right + .5 || R(q(s)).left < cr.left - .5),
      lbl: tx('#scLbl'), left: tx('#scLeft'), note: shown ? nb + ' · ' + n.textContent.slice(nb.length) : null,
      noteBad: shown && !(nr.height > 0 && nr.height < 40 && nr.left >= cr.left - .5 && nr.right <= er.left + .5 && fits(n)),
      noteW: Math.round(nr.width), raw: /undefined|NaN|sc_|dur_/.test(cardEl.innerText) };
    STR = keep.STR; me.wallet_grosz = keep.w; me.coin_minutes = keep.cm; activeSession = keep.s; bookingDeadline = keep.bd;
    renderSession(); renderWallet();
    return m;
  }, [lang, o]);
  const LEFT = { ru: '9 мин', en: '9 min', pl: '9 min' };
  for (const [wd, ht] of [[1440, 900], [1920, 1080], [2560, 1440]]) {
    await p.setViewportSize({ width: wd, height: ht });
    await p.waitForTimeout(250);
    const bad = []; let w = 0, nw = 0, sample = '';
    for (const lang of ['ru', 'en', 'pl']) for (const o of [{}, { mix: true }, { bk: true }]) {
      const m = await probe(lang, o); w = m.w;
      if (lang === 'pl' && !o.mix && !o.bk) { sample = `${m.lbl} ${m.left} · ${m.note}`; nw = m.noteW; }
      const ok = m.red && !m.wide.length && !m.wrap.length && !m.out.length && !m.raw && !m.noteBad && m.left === LEFT[lang]
        && m.lbl === (o.bk ? L.sc_lbl_bk : o.mix ? L.sc_low_mix : L.sc_low)[lang] && m.note === (o.bk ? null : NOTE[lang]);
      if (!ok) bad.push({ lang, o, m });
    }
    z.check(`${wd}×${ht}, карточка ${w} px, под заметку ${nw} px: красная RU/EN/PL (кошелёк, минуты, бронь) с предельными числами — подпись в одну `
      + `строку, заметка в две, ничего не вылезает, сырых ключей нет (PL: «${sample}»)`, bad.length === 0, J(bad));
  }
  await p.setViewportSize({ width: 1920, height: 1080 });
  await p.waitForTimeout(250);

  console.log('— пауза и чужая бронь (подставлены на странице)');
  const pz = await p.evaluate(() => {
    const el = document.getElementById('sessCard'), keep = activeSession.paused_at, was = el.classList.contains('sc-low');
    activeSession.paused_at = new Date().toISOString(); renderSession(); renderWallet();
    const paused = el.classList.contains('sc-low');
    activeSession.paused_at = keep; renderSession(); renderWallet();
    return { was, paused, back: el.classList.contains('sc-low') };
  });
  z.check('пауза: время и деньги стоят — карточка не красная; пауза снята — снова красная', pz.was && !pz.paused && pz.back, J(pz));
  const bk = await p.evaluate(() => {
    const el = document.getElementById('sessCard'), keep = bookingDeadline, w = me.wallet_grosz, t = id => document.getElementById(id).textContent;
    me.wallet_grosz = 60000; bookingDeadline = new Date(Date.now() + 9 * 60000 - 20000).toISOString(); renderWallet();
    const r = { red: el.classList.contains('sc-low'), lbl: t('scLbl'), left: t('scLeft'), note: !document.getElementById('scNote').classList.contains('hidden') };
    bookingDeadline = keep; me.wallet_grosz = w; renderWallet();
    return r;
  });
  z.check(`чужая бронь через 9 минут при полном кошельке: карточка красная, «${bk.lbl} ${bk.left}», «Пополнить» нет — деньги бронь не отодвинут`,
    bk.red && bk.lbl === L.sc_lbl_bk.ru && bk.left === '9 мин' && !bk.note, J(bk));

  console.log('— пополнение у стойки');
  await isRed(p);
  const red0 = (await card(p)).red, t0 = Date.now();
  const d = await z.api('POST', `/admin/users/${g}/deposit`, { amount_pln: 50, method: 'card' }, 'boss');
  dep = d.data && d.data.deposit_id;
  await p.waitForFunction(() => !document.getElementById('sessCard').classList.contains('sc-low'), null, { timeout: 14000 }).catch(() => {});
  const sec = ((Date.now() - t0) / 1000).toFixed(1);
  let okUp = false;
  for (let i = 0; i < 3 && !okUp; i++) {
    await sync(p); a = await adm(); c = await card(p);
    okUp = !c.red && c.lbl === L.sc_lbl.ru && c.note === null && !c.chip && c.k9 === 1 && a.minutes_left > 15 && mins(c.left) === a.minutes_left;
  }
  z.check(`пополнили у стойки на 50 zł (${d.status}): через ${sec} с карточка снова обычная — «${c.lbl} ${c.left}» (у админа — ${a.minutes_left} мин), `
    + '«Пополнить» нет, чип не красный; страница не перезагружалась', d.status < 300 && !!dep && red0 && okUp, J({ c, a, red0, dep, d: d.data }));
  await z.shot(p, 'k9_low_topup');

  console.log('— в запасе минуты, кошелёк пуст');
  z.sql(`UPDATE users SET coin_minutes = 9, wallet_grosz = 0 WHERE id='${g}'`);
  await p.waitForFunction(() => (me.coin_minutes || 0) > 0 && !(me.wallet_grosz > 0), null, { timeout: 14000 }).catch(() => {});
  let okMix = false;
  for (let i = 0; i < 3 && !okMix; i++) {
    await sync(p); a = await adm(); c = await card(p);
    okMix = c.red && c.lbl === L.sc_low_mix.ru && a.minutes_left >= 7 && a.minutes_left <= 9 && mins(c.left) === a.minutes_left && c.note === NOTE.ru;
  }
  z.check(`минуты на счету, кошелёк пуст (у админа — ${a.minutes_left} мин): карточка красная — «${c.lbl} ${c.left}», «Пополнить» на месте`,
    okMix, J({ c, a }));

  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
