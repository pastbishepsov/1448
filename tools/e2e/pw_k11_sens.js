// К11-и7 «Сенса: «уже работает» без рода и числа» — проверка в браузере (Chrome без окна): Guest46 за PC-07,
// сессия идёт. Агент ПК подставной: на /ping отвечает, что умеет писать сенсу в CS2 и Apex Legends.
// Сервер стенда настоящий; подменён один его ответ — очередной опрос уведомлений киоском (раз в 12 секунд):
// в нём два уведомления «сенса подставлена» — одна игра уже работала и две. Дальше всё делает сам киоск:
// строит текст и показывает всплывающую строку. По-русски строка не зависит ни от рода, ни от числа игр:
// «… · уже работает: CS2, Apex Legends — сенса подхватится при перезапуске (CS2: exec 1448 в консоли)».
// Подсказка под кнопкой «Сохранить» в настройке сенсы: «CS2 при запуске (уже работает — в консоли exec 1448)».
// По-английски и по-польски с К12-и3 строка тоже начинается со сказуемого — «already running: CS2, Apex Legends — …»,
// «już działa: …» — и не зависит от числа игр; проверяем, что строки на месте.
// В базе ничего не меняет: гостя сажает и сессию закрывает сама, кошелёк возвращает.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

const WANT = {
  ru: { head: n => `🎮 Сенса на этом ПК: ${n}`,
    tail: n => ` · уже работает: ${n} — сенса подхватится при перезапуске (CS2: exec 1448 в консоли)`,
    hint: n => `Сохранишь — и ${n} на этом ПК подхватят твою сенсу сразу: CS2 при запуске (уже работает — в консоли `
      + 'exec 1448), Apex при следующем старте. Valorant хранит настройки в облаке Riot — там введи значение руками.' },
  en: { tail: n => ` · already running: ${n} — sens applies after a restart (CS2: exec 1448 in the console)`,
    hintPart: '(already running — type exec 1448 in the console)' },
  pl: { tail: n => ` · już działa: ${n} — czułość wczyta się po restarcie (CS2: exec 1448 w konsoli)`,
    hintPart: '(już działa — wpisz exec 1448 w konsoli)' },
};
const ONE = { applied: ['cs2', 'apex'], running: ['cs2'] }, TWO = { applied: ['cs2', 'apex'], running: ['cs2', 'apex'] };

(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const w0 = z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  // кошелёк возвращаем одним запросом вместе со строкой журнала на ту же разницу (правило стенда трека З:
  // «кошелёк гостя = сумма его журнала», e2e_z0_stand); в конце сверяем, что расхождение осталось каким было
  const setWallet = v => z.sql(`WITH o AS (SELECT id, wallet_grosz AS w FROM users WHERE id='${g}' FOR UPDATE),
      u AS (UPDATE users SET wallet_grosz = ${v} FROM o WHERE users.id = o.id RETURNING o.w)
    INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, note, created_at)
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k11_sens: проверка вернула кошелёк', now() FROM u WHERE u.w <> ${v}`);
  const gap = () => z.sql('SELECT u.wallet_grosz - COALESCE((SELECT sum(amount_grosz) FROM wallet_transactions w WHERE w.user_id = u.id), 0) '
    + `FROM users u WHERE u.id='${g}'`);
  const gap0 = gap();
  z.atEnd(async () => {
    await endTails();
    setWallet(+w0);
    z.check('проверка убрала за собой: сессия закрыта, кошелёк гостя сходится со своим журналом так же, как до неё',
      gap() === gap0, `расхождение ${gap0} → ${gap()}`);
  });
  await endTails();

  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  z.check('гость посажен за PC-07, сессия идёт', !!(seat.data && seat.data.session_id), J(seat.data));

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const CORS = { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Headers': 'Content-Type',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS' };
  const ctx = await br.newContext({ viewport: { width: 1920, height: 1080 } });
  await ctx.route('http://127.0.0.1:1448/**', rt => { // подставной агент ПК: на связи, сенсу писать умеет
    const rq = rt.request(), path = new URL(rq.url()).pathname;
    const json = body => rt.fulfill({ status: 200, contentType: 'application/json', headers: CORS, body: J(body) });
    if (rq.method() === 'OPTIONS') return rt.fulfill({ status: 204, headers: CORS, body: '' });
    if (path === '/ping') return json({ computer_id: pcId, sens: true, sens_games: ['cs2', 'apex'] });
    if (path === '/apps-status') return json({ apps: {}, running: null });
    return rt.fulfill({ status: 404, headers: CORS, body: '' });
  });
  const nq = []; // уведомления, которые киоск получит в ближайшем своём опросе вместо ответа сервера
  await ctx.route(u => u.pathname.endsWith('/me/notifications'), rt => (nq.length
    ? rt.fulfill({ status: 200, contentType: 'application/json', body: J({ items: nq.splice(0) }) }) : rt.continue()));
  const p = await ctx.newPage();
  p.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  const until = (fn, arg, ms) => p.waitForFunction(fn, arg, { timeout: ms }).then(() => true).catch(() => false);
  const toastNow = () => p.evaluate(() => { const t = document.getElementById('toast'); return t.classList.contains('show') ? t.textContent : ''; });
  // дождаться всплывающей строки про сенсу (не той, что уже видели) и прочитать её с экрана
  const sensToast = async prev => {
    const ok = await until(q => { const t = document.getElementById('toast'); return t.classList.contains('show') && t.textContent.startsWith('🎮') && t.textContent !== q; }, prev, 30000);
    await p.waitForTimeout(400);
    return ok ? toastNow() : '';
  };
  const shot = async (name, c) => { // кадр: один блок с полями
    try {
      const x = Math.max(0, c.x - 14), y = Math.max(0, c.y - 14);
      await p.screenshot({ path: `${z.OUT}/k11s_${name}.png`, clip: { x, y, width: Math.min(1920 - x, c.w + 28), height: Math.min(1080 - y, c.h + 28) } });
    } catch (e) { console.log('кадр', name, e.message.split('\n')[0]); }
  };
  const boxOf = sel => p.evaluate(s => { const b = document.querySelector(s).getBoundingClientRect(); return { x: b.left, y: b.top, w: b.width, h: b.height }; }, sel);

  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
  await until(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'), null, 15000);
  await until(() => agentOnline === true && agentSens === true, null, 10000);
  const st = await p.evaluate(() => ({ online: agentOnline, sens: agentSens, lang: LANG, tab: curTab,
    n1: sensGameNames(['cs2']), n2: sensGameNames(['cs2', 'apex']) }));
  const N1 = st.n1, N2 = st.n2;
  z.check('киоск открыт по-русски, агент на связи и умеет писать сенсу в CS2 и Apex Legends',
    st.online && st.sens && st.lang === 'ru' && N1 === 'CS2' && N2 === 'CS2, Apex Legends', J(st));

  console.log('— Всплывающая строка: уведомление «сенса подставлена» пришло киоску в его опросе');
  nq.push({ type: 'sens_applied', payload: ONE }, { type: 'sens_applied', payload: TWO });
  const t1 = await sensToast('');
  await shot('one', await boxOf('#toast'));
  z.check(`одна игра уже работала: «${WANT.ru.head(N2) + WANT.ru.tail(N1)}»`, t1 === WANT.ru.head(N2) + WANT.ru.tail(N1), J(t1));
  const t2 = await sensToast(t1);
  const fit = await p.evaluate(() => { const t = document.getElementById('toast'), b = t.getBoundingClientRect();
    return { l: Math.round(b.left), r: Math.round(b.right), w: innerWidth, sw: t.scrollWidth, cw: t.clientWidth, h: Math.round(b.height) }; });
  await shot('two', await boxOf('#toast'));
  z.check(`две игры уже работали: строка та же, имена через запятую — «${WANT.ru.tail(N2).slice(3)}»`,
    t2 === WANT.ru.head(N2) + WANT.ru.tail(N2), J(t2));
  z.check('длинная строка (две игры) целиком на экране 1920×1080, не обрезана',
    !!t2 && fit.l >= 0 && fit.r <= fit.w && fit.sw <= fit.cw + 1, J(fit));

  console.log('— Подсказка в настройке сенсы (кнопка «Сенса» в шапке)');
  await p.click('#tbSens');
  const hint = await p.evaluate(async () => {
    const find = () => { const want = sensApplyHint(); return [...document.querySelectorAll('.card .muted')].find(x => x.textContent.trim() === want); };
    for (let k = 0; k < 40; k++) {
      let e = find();
      if (e) {
        e.scrollIntoView({ block: 'center' });
        await new Promise(r => setTimeout(r, 200));
        e = find(); // панель за это время могла перерисоваться — берём блок заново, нужен тот, что на экране
        const b = e ? e.getBoundingClientRect() : null;
        if (b && b.height > 0 && b.top >= 0 && b.bottom <= innerHeight)
          return { tab: curTab, text: e.textContent.trim(), box: { x: b.left, y: b.top, w: b.width, h: b.height } };
      }
      await new Promise(r => setTimeout(r, 100));
    }
    return { tab: curTab, text: '', box: null };
  });
  if (hint.box) await shot('hint', hint.box);
  z.check('под кнопкой «Сохранить», на экране: «… CS2 при запуске (уже работает — в консоли exec 1448), Apex при следующем старте …»',
    hint.tab === 'sens' && !!hint.box && hint.text === WANT.ru.hint(N2), J([hint.tab, hint.text, hint.box && Math.round(hint.box.h)]));

  console.log('— Три языка: что киоск строит из того же уведомления');
  const L = await p.evaluate(([one, two]) => {
    const keep = { STR, LANG }, out = {};
    for (const l of ['ru', 'en', 'pl']) {
      LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]);
      out[l] = { one: notifText({ type: 'sens_applied', payload: one }), two: notifText({ type: 'sens_applied', payload: two }),
        none: notifText({ type: 'sens_applied', payload: { applied: one.applied } }), hint: sensApplyHint(),
        keys: [String(STR.toast_sens_running || ''), String(STR.sens_apply_hint || '')] };
    }
    LANG = keep.LANG; STR = keep.STR;
    return out;
  }, [ONE, TWO]);
  z.check('RU: в двух строках сенсы нет слова «запущен»',
    L.ru.keys.every(v => v && !v.includes('запущен')), J(L.ru.keys.filter(v => !v || v.includes('запущен'))));
  z.check(`RU: игры и так не работали — хвоста нет: «${WANT.ru.head(N2)}»`, L.ru.none === WANT.ru.head(N2), J(L.ru.none));
  for (const l of ['en', 'pl']) {
    const W = WANT[l], o = L[l];
    z.check(`${l.toUpperCase()}: строки на месте — «${W.tail('…').slice(3)}», «${W.hintPart}»`,
      o.one.endsWith(W.tail(N1)) && o.two.endsWith(W.tail(N2)) && o.none.endsWith(N2) && o.hint.includes(W.hintPart),
      J([o.one, o.two, o.none, o.hint.slice(0, 150)]));
  }

  const fin = await p.evaluate(() => { sideTab('now'); return { lang: LANG, tab: curTab, ru: STR === SH_RU || STR.toast_sens_applied === SH_RU.toast_sens_applied }; });
  z.check('после проверки киоск как был: по-русски, раздел «Сейчас»', fin.lang === 'ru' && fin.tab === 'now' && fin.ru, J(fin));
  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})();
