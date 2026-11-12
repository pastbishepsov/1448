// З7-и1 (№41), браузер: «Экономика» — карточки по смыслу (с З13-и2 — и «Бухгалтерия», с З18-и4 — и «Сгорание пакетов»,
// с З22-и1 — и «Алерты»: десять), у каждой строка «на что
// влияет»; «Сохранить» без правок ничего не отправляет; правка одной настройки подсвечена, уходит одна и одной строкой в журнал; у часа
// границы суток — предупреждение про прошлые отчёты и взвод на «Сохранить» (с З18-и4б поле часа — после «Свой час границы» в
// «Сутках клуба»; сам выбор — в pw_z18_day). Таблица settings в конце — как была.
const z = require('./_z');
const CARDS = ['Монеты и XP', 'Сгорание пакетов', 'Лимиты персонала', 'Клубные сутки и сегменты', 'Сессия у ПК', 'Брони', 'Стрики и ДР', 'Мышь и коврик', 'Алерты', 'Бухгалтерия'];
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
    ids: [...c.querySelectorAll('input, select')].map(i => i.id) })));
  z.check('десять карточек: ' + CARDS.join(' · '), JSON.stringify(cards.map(c => c.t)) === JSON.stringify(CARDS), JSON.stringify(cards.map(c => c.t)));
  z.check('у каждой карточки — строка «на что влияет»', cards.length === 10 && cards.every(c => c.why.length > 20), JSON.stringify(cards.map(c => c.why)));
  const where = id => (cards.find(c => c.ids.includes(id)) || {}).t;
  z.check('поля на своих местах: час суток — «Клубные сутки», кейсы админа — «Лимиты персонала», DPI — «Мышь и коврик», форма учёта (З13-и7), '
    + 'плательщик VAT, подарки в VAT и ставка ryczałtu времени — «Бухгалтерия»; окно [Готов!] (З18-и4) — «Сессия у ПК»; «Сутки клуба» '
    + '(З18-и4б) — «Клубные сутки»; «Алерты в канал» (З22-и1), пороги (З22-и3), смены (З22-и4) и события (З22-и2) — «Алерты», '
    + '49 полей',
    where('set_rhour') === 'Клубные сутки и сегменты' && where('set_rmode') === 'Клубные сутки и сегменты' && where('set_casecap') === 'Лимиты персонала' && where('set_mousedpi') === 'Мышь и коврик'
    && where('set_accform') === 'Бухгалтерия' && where('set_vatpayer') === 'Бухгалтерия' && where('set_giftvat') === 'Бухгалтерия'
    && where('set_rytime') === 'Бухгалтерия' && where('set_readywait') === 'Сессия у ПК' && where('set_packwarn') === 'Сгорание пакетов'
    && where('set_alerts') === 'Алерты' && where('set_alertdebt') === 'Алерты' && where('set_shiftnobody') === 'Алерты'
    && where('set_alertlow') === 'Алерты' && cards.reduce((n, c) => n + c.ids.length, 0) === 49,
    JSON.stringify(cards.map(c => [c.t, c.ids.length])));
  await z.shot(page, 'z7_settings');

  const a0 = audit();
  const t0 = await z.toastAfter(page, () => save(), 'Правок нет'); // З19-и13: тост этого действия, а не #toast через паузу
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

  // З18-и4б: при сутках с полуночи поле часа спрятано — сначала «Свой час границы» (на HEAD выбора нет — сразу поле)
  if (await page.$('#set_rmode') && await page.inputValue('#set_rmode') === '0') await page.selectOption('#set_rmode', '1');
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
