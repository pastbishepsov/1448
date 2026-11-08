// Помощники браузерных проверок трека З (OWNER.md): на них pw_z<N>_*.js пишется в 30–60 строк.
//
//   const z = require('./_z');
//   (async () => {
//     const page = await z.openAdmin('boss');            // Chromium 1440×900, вход через форму, тур пропущен
//     await z.nav(page, 'stats');
//     z.check('…', cond, detail);
//     await z.shot(page, 'z0_stats');                    // скриншот в E2E_OUT
//     await z.done();                                    // браузеры закрыть, уборка, «ошибок JS нет», итог
//   })().catch(z.crash);
//
// Уборка — z.atEnd(fn) (в обратном порядке, и при падении тоже). Стенд задаётся окружением (_env.js).
const { chromium } = require('playwright-core');
const { execFileSync } = require('child_process');
const { API, BASE, DB, CHROME, OUT, WEB_DIR } = require('./_env');

const GUEST_PASS = 'Guest12345', STAFF_PASS = 'Admin12345';
const PASSWORDS = { boss: 'Boss12345', admin1: STAFF_PASS, admin2: STAFF_PASS };
// пароль стенда: персонал сида и временные z<N>… — STAFF_PASS (кроме boss), гости — GUEST_PASS
const passwordOf = nick => PASSWORDS[nick] || (/^z\d/.test(nick) ? STAFF_PASS : GUEST_PASS);
const sleep = ms => new Promise(r => setTimeout(r, ms));
const out = [], cleanups = [], browsers = [], jsErrors = [];

// ── база ──
const rows = q => execFileSync('psql', [DB, '-tAc', q]).toString().split('\n').filter(Boolean); // без шелла — $ и кавычки как есть
const sql = q => rows(q)[0] || '';
const uid = nick => sql(`SELECT id FROM users WHERE nickname='${nick}'`);
// временная позиция стенда — SQL-ом вместе с движениями, партиями и её поставками (как drop_good в _z.py, З10-и4)
const dropGood = gid => gid && sql(`CREATE TEMP TABLE _dg AS SELECT DISTINCT supply_id FROM stock_lots WHERE good_id = '${gid}' AND supply_id IS NOT NULL;
  DELETE FROM lot_moves WHERE lot_id IN (SELECT id FROM stock_lots WHERE good_id = '${gid}');
  DELETE FROM stock_moves WHERE good_id = '${gid}';
  DELETE FROM stock_lots WHERE good_id = '${gid}';
  DELETE FROM supplies s WHERE s.id IN (SELECT supply_id FROM _dg) AND NOT EXISTS (SELECT 1 FROM stock_lots l WHERE l.supply_id = s.id);
  DELETE FROM goods WHERE id = '${gid}'`);

// ── API ──
async function call(method, path, body, token) {
  const headers = { 'Content-Type': 'application/json' };
  if (token) headers.Authorization = 'Bearer ' + token;
  const r = await fetch(API + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
  const text = await r.text();
  await sleep(120); // лимитер сервера — 10 запросов в секунду с IP
  let data; try { data = JSON.parse(text || '{}'); } catch (e) { data = { _text: text.slice(0, 300) }; }
  return { status: r.status, data };
}
// вход как есть; extra — поля тела (client: 'admin' — вход админки, он открывает смену, З3-и1)
const login = (nick, password, extra = {}) => call('POST', '/auth/login', { login: nick, password: password || passwordOf(nick), ...extra });
const cache = {};
async function tok(nick, fresh) {
  if (fresh || !cache[nick]) {
    const r = await login(nick, undefined, { client: 'admin' }); // как админка: вход открывает админу смену (З3-и1)
    if (r.status !== 200) throw new Error(`вход ${nick}: ${r.status} ${JSON.stringify(r.data)}`);
    cache[nick] = r.data.access_token;
  }
  return cache[nick];
}
// { status, data }; who — ник (токен из кэша, на 401 — один перевход), без who — без авторизации
async function api(method, path, body, who) {
  if (!who) return call(method, path, body);
  let r = await call(method, path, body, await tok(who));
  if (r.status === 401) r = await call(method, path, body, await tok(who, true));
  return r;
}

// ── клубные сутки и персонал ──
// клубные сутки 'ГГГГ-ММ-ДД' (от report_hour по часам клуба), как их считает сервер; offset — ±дни; строки нет — с полуночи (З18-и4б)
const clubDay = (offset = 0) => sql(`SELECT (((now() AT TIME ZONE 'Europe/Warsaw') - make_interval(hours => COALESCE((SELECT value FROM settings WHERE key='report_hour'), '0')::int))::date + ${offset})::text`);
// временный сотрудник z<N>… (пароль STAFF_PASS, нанят 30 дней назад); повторный прогон возвращает того же
// человека (аккаунт не удалить — FK); в конце увольняется сам, незакрытая запись табеля снимается. Наём с З14-и3
// требует сменить пароль при первом входе — проверкам это не нужно (это проверяет e2e_z14_password): флаг снят SQL-ом
async function hireTemp(nick, { keep = false } = {}) {
  if (!uid(nick)) {
    const r = await api('POST', '/admin/staff/hire', { nickname: nick, password: STAFF_PASS, hired_at: clubDay(-30) }, 'boss');
    if (r.status !== 201) throw new Error(`наём ${nick}: ${r.status} ${JSON.stringify(r.data)}`);
  } else {
    sql(`UPDATE users SET role='admin', status='active', password_hash=(SELECT password_hash FROM users WHERE nickname='admin1') WHERE nickname='${nick}'`);
    sql(`UPDATE staff_profiles SET dismissed_at=NULL, hired_at='${clubDay(-30)}' WHERE user_id='${uid(nick)}'`);
  }
  sql(`UPDATE users SET must_change_password=false WHERE nickname='${nick}'`);
  delete cache[nick];
  if (!keep) atEnd(async () => { await dismiss(nick); sql(`DELETE FROM work_entries WHERE ended_at IS NULL AND user_id='${uid(nick)}'`); });
  return uid(nick);
}
// З18-и4г: открытая смена nick — «с сейчас» на время проверки, в конце — как была: «сегодня» админа раздвигает его смена,
// начатая раньше суток, а проверке нужны сами клубные сутки. Не на смене — вход (tok) откроет смену с «сейчас»
async function pinShift(nick) {
  await tok(nick);
  for (const r of rows(`SELECT id || '|' || started_at FROM work_entries WHERE user_id = '${uid(nick)}' AND ended_at IS NULL`)) {
    const [id, start] = [r.slice(0, r.indexOf('|')), r.slice(r.indexOf('|') + 1)];
    sql(`UPDATE work_entries SET started_at = now() WHERE id = '${id}'`);
    atEnd(() => sql(`UPDATE work_entries SET started_at = '${start}' WHERE id = '${id}' AND ended_at IS NULL`));
  }
}
async function dismiss(nick, body = {}) { // уволить от владельца; уже player — пропуск
  if (['', 'player'].includes(sql(`SELECT role FROM users WHERE nickname='${nick}'`))) return { status: 200, data: {} };
  delete cache[nick];
  return api('POST', `/admin/staff/${uid(nick)}/dismiss`, body, 'boss');
}

// ── итог ──
function check(name, cond, detail) {
  out.push((cond ? 'ok   ' : 'FAIL ') + name + (detail ? ' — ' + detail : ''));
  console.log(out[out.length - 1]);
  return !!cond;
}
const atEnd = fn => { cleanups.push(fn); return fn; };
async function cleanup() {
  for (const b of browsers.splice(0)) { try { await b.close(); } catch (e) {} }
  while (cleanups.length) { const fn = cleanups.pop(); try { await fn(); } catch (e) { console.log('уборка:', e.message); } }
}
async function done() {
  if (browsers.length) check('ошибок JS на страницах нет', jsErrors.length === 0, jsErrors.join(' | '));
  await cleanup();
  const okN = out.filter(l => l.startsWith('ok')).length, failN = out.length - okN;
  console.log(`\n${okN} ok, ${failN} FAIL`);
  process.exit(failN ? 1 : 0);
}
async function crash(e) { console.error('CRASH', e); await cleanup(); process.exit(2); }

// ── браузер ──
async function openAdmin(nick = 'boss', { password, viewport = { width: 1440, height: 900 } } = {}) {
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  browsers.push(browser);
  const page = await (await browser.newContext({ viewport })).newPage();
  page.on('pageerror', e => jsErrors.push(`${nick}: ${e.message}`));
  await page.goto(BASE + '/admin', { waitUntil: 'load' });
  await page.fill('#nick', nick); await page.fill('#pass', password || passwordOf(nick));
  await page.keyboard.press('Enter');
  await page.waitForSelector('#rail', { state: 'visible', timeout: 10000 });
  await sleep(1500); // тур стартует через 0,9 с после входа — пропускаем, если показан
  await page.evaluate(() => { const t = document.getElementById('tour'); if (t && !t.classList.contains('hidden')) tourSkip(); });
  return page;
}
async function nav(page, screen) { await page.evaluate(s => nav(s), screen); await sleep(1500); }
const shot = (page, name, fullPage = false) => page.screenshot({ path: `${OUT}/z_${name}.png`, fullPage });
const text = (page, sel) => page.evaluate(s => { const e = document.querySelector(s); return e ? e.textContent.replace(/\s+/g, ' ').trim() : null; }, sel);

module.exports = { API, BASE, DB, OUT, WEB_DIR, GUEST_PASS, STAFF_PASS, passwordOf, sleep, rows, sql, uid, dropGood, call, login, tok, api,
  clubDay, hireTemp, dismiss, pinShift, check, atEnd, done, crash, jsErrors, openAdmin, nav, shot, text };
