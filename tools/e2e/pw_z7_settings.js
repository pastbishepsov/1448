// З7-и1 (№41), браузер: «Экономика» — семь карточек по смыслу, у каждой строка «на что влияет»; «Сохранить» без
// правок ничего не отправляет; правка одной настройки подсвечена, уходит одна и одной строкой в журнал; у часа
// границы суток — предупреждение про прошлые отчёты и взвод на «Сохранить». Таблица settings в конце — как была.
const z = require('./_z');
const CARDS = ['Монеты и XP', 'Лимиты персонала', 'Клубные сутки и сегменты', 'Сессия у ПК', 'Брони', 'Стрики и ДР', 'Мышь и коврик'];
(async () => {
  const orig = z.rows("SELECT key || '|' || value FROM settings").map(r => r.split('|'));
  z.atEnd(() => {
    z.sql(`DELETE FROM settings WHERE key NOT IN (${orig.map(([k]) => `'${k}'`).join(',') || "''"})`);
    for (const [k, v] of orig) z.sql(`INSERT INTO settings (key, value) VALUES ('${k}', '${v}') ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`);
  });
  const audit = () => +z.sql("SELECT count(*) FROM admin_actions WHERE action='settings_update'");
  const last = () => z.sql("SELECT details FROM admin_actions WHERE action='settings_update' ORDER BY created_at DESC LIMIT 1");

  const page = await z.openAdmin('boss');
  // до З7-и1 кнопка была без id — тогда жмём прежнюю, чтобы прогон на HEAD показал, что она делала
  const save = async () => (await page.$('#setSave') || await page.$('#s-settings button:has-text("Сохранить")')).click();
  const puts = [];
  page.on('request', r => { if (r.method() === 'PUT' && r.url().endsWith('/admin/settings')) puts.push(r.postData() || ''); });
  await z.nav(page, 'settings');
  const cards = await page.evaluate(() => [...document.querySelectorAll('#setGrid .setcard')].map(c => ({
    t: (c.querySelector('h4') || {}).textContent, why: ((c.querySelector('.why') || {}).textContent || '').trim(),
    ids: [...c.querySelectorAll('input')].map(i => i.id) })));
  z.check('семь карточек: ' + CARDS.join(' · '), JSON.stringify(cards.map(c => c.t)) === JSON.stringify(CARDS), JSON.stringify(cards.map(c => c.t)));
  z.check('у каждой карточки — строка «на что влияет»', cards.length === 7 && cards.every(c => c.why.length > 20), JSON.stringify(cards.map(c => c.why)));
  const where = id => (cards.find(c => c.ids.includes(id)) || {}).t;
  z.check('поля на своих местах: час суток — «Клубные сутки», кейсы админа — «Лимиты персонала», DPI — «Мышь и коврик», 31 поле',
    where('set_rhour') === 'Клубные сутки и сегменты' && where('set_casecap') === 'Лимиты персонала' && where('set_mousedpi') === 'Мышь и коврик'
    && cards.reduce((n, c) => n + c.ids.length, 0) === 31, JSON.stringify(cards.map(c => [c.t, c.ids.length])));
  await z.shot(page, 'z7_settings');

  const a0 = audit();
  await save();
  await z.sleep(800);
  const t0 = await z.text(page, '#toast');
  z.check('«Сохранить» без правок: тост «Правок нет», запроса нет, журнал молчит', puts.length === 0 && /Правок нет/.test(t0 || '') && audit() === a0,
    `PUT ${puts.length}, тост «${t0}», журнал +${audit() - a0}`);

  const v = +(await page.inputValue('#set_seglost'));
  await page.fill('#set_seglost', String(v + 1));
  const st = await page.evaluate(() => ({ on: document.getElementById('set_seglost').classList.contains('changed'),
    hint: (document.getElementById('setHint') || {}).textContent || '' }));
  z.check('правка подсвечена, у кнопки «Изменено: 1»', st.on && /Изменено: 1\b/.test(st.hint), JSON.stringify(st));
  await save();
  await z.sleep(1500);
  z.check(`ушёл один PUT и только с «пропавшими»: {"seg_lost_days":${v + 1}}`, puts.length === 1 && puts[0] === JSON.stringify({ seg_lost_days: v + 1 }), puts.join(' | '));
  z.check(`в журнале одна строка «seg_lost_days: ${v}→${v + 1}»`, audit() === a0 + 1 && last() === `seg_lost_days: ${v}→${v + 1}`, `+${audit() - a0}: ${last()}`);

  const rh = +(await page.inputValue('#set_rhour')), rh2 = (rh + 1) % 24;
  await page.fill('#set_rhour', String(rh2));
  const warn = await page.evaluate(() => { const w = document.getElementById('setRhourWarn'); return w && !w.classList.contains('hidden') ? w.textContent.replace(/\s+/g, ' ').trim() : null; });
  z.check('час границы суток изменён — предупреждение «… и в прошлых отчётах»', /прошлых отчётах/.test(warn || ''), warn);
  await save();
  await z.sleep(400);
  const armed = await z.text(page, '#setSave');
  z.check('первый клик «Сохранить» — взвод «Ещё раз — …», запроса нет', /Ещё раз/.test(armed || '') && puts.length === 1, `${armed}; PUT ${puts.length}`);
  await page.locator('#set_rhour').scrollIntoViewIfNeeded();
  await z.shot(page, 'z7_settings_rhour');
  await save();
  await z.sleep(1500);
  z.check(`второй клик — сохранено: граница суток ${rh2} в базе, ушёл только report_hour`, z.sql("SELECT value FROM settings WHERE key='report_hour'") === String(rh2)
    && puts.length === 2 && puts[1] === JSON.stringify({ report_hour: rh2 }), puts.join(' | '));
  await z.done();
})().catch(z.crash);
