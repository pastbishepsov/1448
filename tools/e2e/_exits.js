// Общие помощники проверок «выходов» (pw_exits_*.js, docs/ВЫХОДЫ-НАЗАД.md): Esc, кнопка «назад»
// мыши, клавиша BrowserBack и системный «назад» в киоске, PWA и админке. Гости у проверок свои —
// ExitKiosk, ExitPwa, ExitAdmA, ExitAdmB: регистрируются сами при первом прогоне (Guest31…Guest50
// закреплены за треком З, см. README). Стенд и браузер — из _env.js (E2E_API, E2E_CHROME, E2E_OUT).
const { chromium } = require('playwright-core');
const { API, BASE, CHROME, OUT } = require('./_env');
async function api(method, path, body, tok){
  const r = await fetch(API + path, { method, headers: { 'Content-Type': 'application/json', ...(tok ? { Authorization: 'Bearer ' + tok } : {}) },
    body: body === undefined ? undefined : JSON.stringify(body) });
  let d = {}; try { d = await r.json(); } catch (e) {}
  return { status: r.status, d };
}
const toks = {};
const PASS = { boss: 'Boss12345', admin1: 'Admin12345', admin2: 'Admin12345' };
const passOf = nick => PASS[nick] || 'Guest12345';
async function login(nick){
  const r = await api('POST', '/auth/login', { login: nick, password: passOf(nick) });
  if (r.status !== 200) throw new Error('login ' + nick + ' ' + r.status + ' ' + JSON.stringify(r.d));
  return (toks[nick] = r.d);
}
async function tok(nick){ return (toks[nick] || await login(nick)).access_token; }
async function userId(nick){ return (toks[nick] || await login(nick)).user.id; }
const PHONES = { ExitKiosk: '+48500199001', ExitPwa: '+48500199002', ExitAdmA: '+48500199003', ExitAdmB: '+48500199004' };
async function ensureGuest(nick){
  const r = await api('POST', '/auth/login', { login: nick, password: 'Guest12345' });
  if (r.status === 200){ toks[nick] = r.d; return r.d.user.id; }
  const reg = await api('POST', '/auth/register', { nickname: nick, password: 'Guest12345', phone: PHONES[nick] });
  if (reg.status >= 300) throw new Error('register ' + nick + ' ' + reg.status + ' ' + JSON.stringify(reg.d));
  return userId(nick);
}
async function ensureMoney(nick, minPln){
  await ensureGuest(nick);
  const t = await login(nick);
  const w = (t.user.wallet_grosz || 0) / 100;
  if (w < minPln){
    const r = await api('POST', '/admin/users/' + t.user.id + '/deposit', { amount_pln: Math.ceil(minPln - w + 5), method: 'cash' }, await tok('boss'));
    if (r.status >= 300) throw new Error('deposit ' + r.status + JSON.stringify(r.d));
  }
}
async function endSessions(nick){ await api('POST', '/me/sessions/end', {}, (await login(nick)).access_token); }
async function browser(){ return chromium.launch({ executablePath: CHROME, args: ['--no-sandbox', '--disable-gpu'] }); }
// настоящая боковая кнопка мыши: CDP шлёт pointer/mouse-события с button 3 (как Chromium/WebView2 у гостя)
async function mouseBack(page, x = 700, y = 450){
  const c = page._cdp || (page._cdp = await page.context().newCDPSession(page));
  await c.send('Input.dispatchMouseEvent', { type: 'mousePressed', x, y, button: 'back', buttons: 8, clickCount: 1 });
  await c.send('Input.dispatchMouseEvent', { type: 'mouseReleased', x, y, button: 'back', buttons: 0, clickCount: 1 });
  await page.waitForTimeout(350);
}
async function browserBackKey(page){ // клавиша BrowserBack (мультимедийная клавиатура, драйвер мыши)
  const c = page._cdp || (page._cdp = await page.context().newCDPSession(page));
  const k = { key: 'BrowserBack', code: 'BrowserBack', windowsVirtualKeyCode: 166, nativeVirtualKeyCode: 166 };
  await c.send('Input.dispatchKeyEvent', { type: 'rawKeyDown', ...k });
  await c.send('Input.dispatchKeyEvent', { type: 'keyUp', ...k });
  await page.waitForTimeout(300);
}
async function esc(page){ await page.keyboard.press('Escape'); await page.waitForTimeout(300); }
let ok = 0, fail = 0; const fails = [];
function check(name, cond, extra){
  if (cond){ ok++; console.log('  ok  ' + name); }
  else { fail++; fails.push(name); console.log('FAIL ' + name + (extra !== undefined ? ' — ' + JSON.stringify(extra) : '')); }
  return !!cond;
}
function summary(){ console.log(`итого: ${ok} ok, ${fail} fail` + (fails.length ? '\n  провалы: ' + fails.join(' | ') : '')); process.exitCode = fail ? 1 : 0; }
const quietConsole = t => /Failed to load resource|ERR_CONNECTION_REFUSED|127\.0\.0\.1:1448/.test(t); // нет агента ПК на стенде — это не ошибка страницы
module.exports = { api, login, tok, userId, ensureGuest, ensureMoney, endSessions, browser, mouseBack, browserBackKey, esc, check, summary, quietConsole, BASE, API, OUT };
