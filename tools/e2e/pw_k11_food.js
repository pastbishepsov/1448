// К11-и4 «Заказ кухни: подтверждение» — проверка в браузере (Chrome без окна): Guest46 за PC-07, сессия идёт,
// раздел «Кухня» на киоске. «Заказать» у блюда сначала спрашивает — что, сколько и за сколько, куда принесут и
// как платить — и только кнопка «Заказать» в окне создаёт заказ. «Отмена», Esc и двойной клик по «Заказать»
// заказа не создают; двойной клик по кнопке в окне создаёт один заказ. Деньги при заказе не списываются
// (сервер создаёт заказ с расчётом у стойки после игры) — кошелёк сверяем до и после; со склада товар уходит.
// Агент ПК подставной (на связи, программ не запускает). Сервер стенда настоящий.
// Замечает вслух (строка «i», в счёт не идёт): второй клик двойного приходит, когда окно уже закрылось по первому,
// и попадает в плитку под кнопкой окна. Это свойство общего окна вопроса (askConfirm), не кухни: второй заказ так
// не родится, но плитка под окном нажимается — записано в «Найдено по дороге» (KIOSK.md)
// За собой убирает: свои заказы отменяет (товар на склад возвращает сервер), сессию закрывает, кошелёк сверяет.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
const J = JSON.stringify;

const SUB = 'Принесём к твоему ПК. Расчёт у стойки после игры';
const WANT = { // вопрос на трёх языках: заголовок (n — блюдо, p — сумма), пояснение, кнопка
  ru: [(n, p) => `Заказать «${n}» за ${p}?`, SUB, 'Заказать'],
  en: [(n, p) => `Order “${n}” for ${p}?`, 'We’ll bring it to your PC. Pay at the desk after your game', 'Order'],
  pl: [(n, p) => `Zamówić „${n}” za ${p}?`, 'Przyniesiemy do twojego PC. Płatność przy ladzie po grze', 'Zamów'],
};
const zl = v => v.toFixed(2).replace('.', ',') + ' zł'; // как fmtZl киоска
const OPEN = "('new','accepted','preparing','delivering')";

(async () => {
  const G = 'Guest46', g = z.uid(G), PC = 'PC-07';
  const pcId = z.sql(`SELECT id FROM computers WHERE name='${PC}'`);
  const endTails = async () => {
    for (const s of z.rows(`SELECT id FROM sessions WHERE status='active' AND (user_id='${g}' OR computer_id='${pcId}')`))
      await z.api('POST', `/admin/sessions/${s}/end`, {}, 'admin1');
    z.sql("UPDATE computers SET status='available' WHERE status IN ('in_session','reserved') AND id NOT IN "
      + "(SELECT computer_id FROM sessions WHERE status='active')");
  };
  const wallet = () => z.sql(`SELECT wallet_grosz FROM users WHERE id='${g}'`);
  const w0 = wallet();
  // кошелёк возвращаем одним запросом вместе со строкой журнала на ту же разницу (правило стенда трека З:
  // «кошелёк гостя = сумма его журнала», e2e_z0_stand); в конце сверяем, что расхождение осталось каким было
  const setWallet = v => z.sql(`WITH o AS (SELECT id, wallet_grosz AS w FROM users WHERE id='${g}' FOR UPDATE),
      u AS (UPDATE users SET wallet_grosz = ${v} FROM o WHERE users.id = o.id RETURNING o.w)
    INSERT INTO wallet_transactions (user_id, kind, amount_grosz, balance_after, ref_type, note, created_at)
    SELECT '${g}', 'adjust', ${v} - u.w, ${v}, 'e2e', 'pw_k11_food: проверка вернула кошелёк', now() FROM u WHERE u.w <> ${v}`);
  const gap = () => z.sql('SELECT u.wallet_grosz - COALESCE((SELECT sum(amount_grosz) FROM wallet_transactions w WHERE w.user_id = u.id), 0) '
    + `FROM users u WHERE u.id='${g}'`);
  const gap0 = gap();

  // заказы гостя: что было до проверки — не трогаем; свои считаем по шагам и сразу отменяем
  const old = new Set(z.rows(`SELECT id FROM kitchen_orders WHERE user_id='${g}'`)), seen = new Set();
  let page = null, item = null, stock0 = null;
  const stock = () => (item ? z.sql(`SELECT stock FROM goods WHERE id='${item.id}'`) : null);
  const cancel = async id => { // от имени гостя с его же киоска (уведомлений себе сервер не шлёт); не вышло — от админа
    let ok = false;
    if (page) ok = await page.evaluate(i => api('/me/kitchen/orders/' + i + '/cancel', { method: 'POST' }).then(() => true, () => false), id).catch(() => false);
    if (!ok) await z.api('POST', `/admin/kitchen/orders/${id}/cancel`, {}, 'admin1');
  };
  const born = async () => { // заказы, появившиеся с прошлого вызова; открытые тут же отменяет
    const list = z.rows(`SELECT id||'|'||qty||'|'||status||'|'||name FROM kitchen_orders WHERE user_id='${g}' ORDER BY created_at`)
      .map(r => r.split('|')).filter(r => !old.has(r[0]) && !seen.has(r[0])).map(([id, qty, status, name]) => ({ id, qty: +qty, status, name }));
    for (const o of list) { seen.add(o.id); if (o.status !== 'cancelled' && o.status !== 'done') await cancel(o.id); }
    if (list.length && page) await page.evaluate(() => loadKitchen()).catch(() => {});
    return list.map(o => `${o.name} ×${o.qty}`);
  };
  z.atEnd(async () => {
    page = null; // страницы может уже не быть — отменяем от админа
    await born();
    await endTails();
    setWallet(+w0);
    const left = z.sql(`SELECT count(*) FROM kitchen_orders WHERE user_id='${g}' AND status IN ${OPEN}`);
    z.check('проверка убрала за собой: свои заказы отменены, товар вернулся на склад, сессия закрыта, кошелёк гостя сходится со своим журналом так же, как до неё',
      left === '0' && stock() === stock0 && gap() === gap0, `открытых заказов ${left}, склад ${stock0} → ${stock()}, расхождение ${gap0} → ${gap()}`);
  });
  await endTails();

  const seat = await z.api('POST', `/admin/computers/${pcId}/session`, { guest: G }, 'admin1');
  z.check('гость посажен за PC-07, сессия идёт', !!(seat.data && seat.data.session_id), J(seat.data));

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close());
  const CORS = { 'Access-Control-Allow-Origin': '*', 'Access-Control-Allow-Headers': 'Content-Type',
    'Access-Control-Allow-Methods': 'GET, POST, OPTIONS' };
  const ctx = await br.newContext({ viewport: { width: 1920, height: 1080 } });
  await ctx.route('http://127.0.0.1:1448/**', rt => { // подставной агент ПК: на связи, ничего не запускает
    const rq = rt.request(), path = new URL(rq.url()).pathname;
    const json = body => rt.fulfill({ status: 200, contentType: 'application/json', headers: CORS, body: J(body) });
    if (rq.method() === 'OPTIONS') return rt.fulfill({ status: 204, headers: CORS, body: '' });
    if (path === '/ping') return json({ computer_id: pcId });
    if (path === '/apps-status') return json({ apps: {}, running: null });
    return rt.fulfill({ status: 404, headers: CORS, body: '' });
  });
  const p = await ctx.newPage();
  p.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  const until = (fn, arg, ms) => p.waitForFunction(fn, arg, { timeout: ms }).then(() => true).catch(() => false);
  // что на экране: окно вопроса (видно ли, заголовок, пояснение, кнопки) и всплывающая строка
  const scr = () => p.evaluate(() => {
    const o = document.getElementById('confirmOverlay'), t = document.getElementById('toast'), no = o.querySelector('.btn.ghost');
    return { show: o.classList.contains('show'), title: document.getElementById('cfTitle').textContent, sub: document.getElementById('cfSub').textContent,
      yes: document.getElementById('cfYes').textContent, no: no ? no.textContent : '', toast: t.classList.contains('show') ? t.textContent : '' };
  });
  const shot = async name => { try { await p.screenshot({ path: `${z.OUT}/k11f_${name}.png` }); } catch (e) { console.log('кадр', name, e.message.split('\n')[0]); } };

  await p.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' });
  await p.fill('#nick', G); await p.fill('#pass', z.GUEST_PASS); await p.click('#authBtn');
  await until(() => typeof me === 'object' && !!me && !!activeSession && !document.getElementById('shellView').classList.contains('hidden'), null, 15000);
  page = p;
  await p.evaluate(() => sideTab('food'));
  await until(() => !!document.querySelector('.fooditem:not(.out) .fbtns button'), null, 10000);
  item = await p.evaluate(() => { // первое блюдо, которое можно заказать
    const b = document.querySelector('.fooditem:not(.out) .fbtns button');
    if (!b) return null;
    const id = ((b.getAttribute('onclick') || '').match(/'([^']+)'/) || [])[1];
    const m = ((menuData && menuData.menu) || []).find(x => x.id === id);
    return m ? { id, name: m.name, price: m.price_pln, lang: LANG, tab: curTab, btn: b.textContent } : null;
  });
  stock0 = stock();
  z.check('киоск открыт по-русски, раздел «Кухня»: в меню есть блюдо с кнопкой «🍽 Заказать», на складе его не меньше двух',
    !!item && item.lang === 'ru' && item.tab === 'food' && item.btn === '🍽 Заказать' && +stock0 >= 2, J([item, stock0]));
  if (!item) { await z.done(); return; }
  const ORDER = `button[onclick="foodOrder('${item.id}')"]`, PLUS = `button[onclick="foodQ('${item.id}',1)"]`;
  const one = WANT.ru[0](item.name, zl(item.price)), two = WANT.ru[0](item.name + ' ×2', zl(item.price * 2));

  console.log('— «Заказать» сначала спрашивает');
  await p.click(ORDER); await p.waitForTimeout(900);
  await shot('ask');
  const s1 = await scr(); let b = await born();
  z.check(`«Заказать» у блюда открывает вопрос «${one}» — заказа ещё нет`,
    s1.show && s1.title === one && b.length === 0, J([s1.show, s1.title, s1.toast, b]));
  z.check(`в окне сказано, куда принесут и как платить: «${SUB}»; кнопки — «Отмена» и «Заказать»`,
    s1.show && s1.sub === SUB && s1.yes === 'Заказать' && s1.no === 'Отмена', J(s1));

  if (s1.show) await p.click('#confirmOverlay .btn.ghost');
  await p.waitForTimeout(700);
  const s2 = await scr(); b = await born();
  z.check('«Отмена»: окно закрылось, заказа нет, кошелёк и склад те же',
    s1.show && !s2.show && b.length === 0 && wallet() === w0 && stock() === stock0 && !s2.toast.startsWith('🍔'),
    J([s1.show, s2.show, s2.toast, b, w0, wallet(), stock0, stock()]));

  await p.click(ORDER); await p.waitForTimeout(700);
  const e1 = await scr();
  if (e1.show) await p.keyboard.press('Escape'); // без окна Esc не жмём: на киоске это шаг назад в другом слое
  await p.waitForTimeout(700);
  const e2 = await scr(); b = await born();
  z.check('Esc — то же, что «Отмена»: окно закрылось, заказа нет', e1.show && !e2.show && b.length === 0, J([e1.show, e2.show, e2.toast, b]));

  console.log('— Двойные клики');
  await p.dblclick(ORDER); await p.waitForTimeout(900);
  const d1 = await scr(); b = await born();
  z.check('двойной клик по «Заказать» у блюда заказа не создаёт: на экране вопрос, второй клик в кнопку окна не попал',
    d1.show && d1.title === one && b.length === 0, J([d1.show, d1.title, d1.toast, b]));
  if (d1.show) await p.click('#confirmOverlay .btn.ghost');
  await p.waitForTimeout(500);

  await p.click(PLUS); await p.waitForTimeout(400);
  await p.click(ORDER); await p.waitForTimeout(700);
  await shot('two');
  const q1 = await scr(); b = await born();
  z.check(`две штуки: вопрос «${two}» — заказа ещё нет`, q1.show && q1.title === two && b.length === 0, J([q1.show, q1.title, q1.toast, b]));
  const okText = `🍔 Заказ принят: ${item.name} ×2 — принесём к ${PC}. Расчёт у стойки после игры`;
  const folder = () => p.evaluate(() => { const f = document.querySelector('.folder-top h2'); return f ? f.textContent : ''; });
  let got = '', fold0 = '', fold = '';
  if (q1.show) {
    fold0 = await folder();
    await p.dblclick('#cfYes');
    await until(() => { const t = document.getElementById('toast'); return t.classList.contains('show') && t.textContent.startsWith('🍔'); }, null, 8000);
    got = (await scr()).toast;
    await p.waitForTimeout(700);
    await shot('done');
    fold = await folder();
  }
  const q2 = await scr(), stockMid = stock(), walletMid = wallet(); b = await born();
  z.check(`двойной клик по «Заказать» в окне: заказ один — «${item.name} ×2», окно закрылось, строка «${okText}»`,
    q1.show && !q2.show && b.length === 1 && b[0] === `${item.name} ×2` && got === okText, J([q1.show, q2.show, got, b]));
  if (fold !== fold0) { // киоск возвращаем настоящей кнопкой «Назад» — дальше проверка идёт с того же экрана
    console.log(`i    замечено: второй клик двойного прошёл сквозь закрывшееся окно в плитку под ним — открылась папка «${fold}»`);
    await p.click('.folder-top .plat-btn'); await p.waitForTimeout(300);
  }
  z.check('деньги при заказе не списаны — расчёт у стойки: кошелёк тот же; со склада ушло две штуки',
    b.length === 1 && walletMid === w0 && +stockMid === +stock0 - 2, J([b, w0, walletMid, stock0, stockMid]));

  console.log('— Три языка');
  const L = await p.evaluate(([n, gr]) => {
    const keep = { STR, LANG }, out = {};
    for (const l of ['ru', 'en', 'pl']) {
      LANG = l; STR = Object.assign({}, SH_RU, SH_LANGS[l]);
      out[l] = [T('food_confirm_title', { n, p: fmtZl(gr) }), String(STR.food_confirm_sub || ''), String(STR.food_confirm_yes || '')];
    }
    LANG = keep.LANG; STR = keep.STR;
    return out;
  }, [item.name + ' ×2', Math.round(item.price * 200)]);
  for (const l of ['ru', 'en', 'pl']) {
    const W = WANT[l], t = W[0](item.name + ' ×2', zl(item.price * 2));
    z.check(`${l.toUpperCase()}: «${t}» · «${W[1]}» · «${W[2]}»`, L[l][0] === t && L[l][1] === W[1] && L[l][2] === W[2], J(L[l]));
  }

  const fin = await p.evaluate(() => { sideTab('now'); return { lang: LANG, tab: curTab, cf: document.getElementById('confirmOverlay').classList.contains('show'),
    fold: !!document.querySelector('.folder-top') }; });
  z.check('после проверки киоск как был: по-русски, раздел «Сейчас», окно вопроса закрыто, папка не открыта',
    fin.lang === 'ru' && fin.tab === 'now' && !fin.cf && !fin.fold, J(fin));
  z.check('ошибок JS в киоске нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
