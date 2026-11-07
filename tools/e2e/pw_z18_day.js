// З18-и4б «Сутки с полуночи» (решение основателя 07.11, З-Р38), браузер: «Экономика» → «Клубные сутки и сегменты». Над
// полем часа — выбор «Сутки клуба»: «С полуночи — 00:00–24:00» и «Свой час границы»; при полуночи поле
// часа спрятано. «Свой час» — поле открыто, пустое, в фокусе; «Сохранить» без часа — тост, запроса нет; 8 — предупреждение
// про прошлые отчёты, взвод, PUT {report_hour: 8}, в журнале «report_hour: 0→8», после сохранения — «Свой час», 8. Обратно
// «С полуночи» — выбор подсвечен, поле спрятано, PUT {report_hour: 0}, «report_hour: 8→0». «Вернуть как было» — как на
// сервере. При полуночи подписи: «Смена · ДАТА · с 00:00», подсказка «Выручки сегодня» — «с 00:00», «Выгрузка» — «клубные
// сутки с 00:00 до 24:00, как календарь», карта загрузки — «ночь после пятницы — уже в субботе» (раньше — «с 0:00», «с 00:00
// до 00:00 следующего дня» и «ночь после пятницы — в пятнице»). Телефон 360 — без прокрутки вбок. В конце граница — как была.
const z = require('./_z');
(async () => {
  const orig = z.rows("SELECT value FROM settings WHERE key = 'report_hour'")[0];
  z.atEnd(() => {
    z.sql("DELETE FROM settings WHERE key = 'report_hour'");
    if (orig !== undefined) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${orig}', now())`);
  });
  // с полуночи — строкой 0, как после миграции 093 (на HEAD без неё по умолчанию 8)
  z.sql("INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '0', now()) ON CONFLICT (key) DO UPDATE SET value = '0'");
  const audit = () => z.sql("SELECT details FROM admin_actions WHERE action = 'settings_update' ORDER BY created_at DESC LIMIT 1");
  const base = () => z.sql("SELECT value FROM settings WHERE key = 'report_hour'");

  const page = await z.openAdmin('boss');
  const puts = [];
  page.on('request', r => { if (r.method() === 'PUT' && r.url().endsWith('/admin/settings')) puts.push(r.postData() || ''); });
  await z.nav(page, 'settings');
  const st = () => page.evaluate(() => {
    const m = document.getElementById('set_rmode'), box = document.getElementById('setRhourBox'), h = document.getElementById('set_rhour');
    const w = document.getElementById('setRhourWarn'), lab = id => ((document.querySelector(`label[for="${id}"]`) || {}).textContent || '');
    return { mode: m ? m.value : null, opts: m ? [...m.options].map(o => o.textContent) : [], modeLabel: lab('set_rmode'),
      changed: !!m && m.classList.contains('changed'), box: !!box && !box.classList.contains('hidden'),
      hour: h ? h.value : null, ph: h ? h.placeholder : null, hourLabel: lab('set_rhour'), focus: !!h && document.activeElement === h,
      card: h ? h.closest('.setcard').querySelector('h4').textContent : null,
      warn: !!w && !w.classList.contains('hidden'), hint: (document.getElementById('setHint') || {}).textContent || '' };
  });
  const mode = async v => { if (await page.$('#set_rmode')) await page.selectOption('#set_rmode', v); }; // на HEAD выбора нет
  const save = async (wait = 1500) => { await page.click('#setSave'); await z.sleep(wait); };

  const s0 = await st();
  z.check('«Клубные сутки и сегменты»: «Сутки клуба» — «С полуночи — 00:00–24:00» и «Свой час границы»; '
    + 'выбрано «С полуночи», поле часа спрятано, правок нет',
    s0.card === 'Клубные сутки и сегменты' && s0.modeLabel === 'Сутки клуба' && s0.mode === '0'
    && JSON.stringify(s0.opts) === JSON.stringify(['С полуночи — 00:00–24:00', 'Свой час границы'])
    && !s0.box && !s0.changed && /Правок нет/.test(s0.hint), JSON.stringify(s0));
  await page.locator('#set_seglost').scrollIntoViewIfNeeded();
  await z.shot(page, 'z18_day_midnight');

  await mode('1');
  const s1 = await st();
  z.check('«Свой час границы» — поле «Час начала суток (1–23)» открыто, пустое («например, 8»), в фокусе; выбор подсвечен, «Изменено: 1»',
    s1.box && s1.hour === '' && s1.ph === 'например, 8' && s1.focus && s1.hourLabel === 'Час начала суток (1–23)' && s1.changed
    && /Изменено: 1\b/.test(s1.hint), JSON.stringify(s1));
  await save(600);
  const t1 = await z.text(page, '#toast');
  z.check('«Сохранить» без часа — тост «⚠ Час начала суток (1–23): целое от 0 до 23», запроса нет',
    puts.length === 0 && /Час начала суток \(1–23\): целое от 0 до 23/.test(t1 || ''), `${t1}; PUT ${puts.length}`);

  await page.fill('#set_rhour', '8');
  const s2 = await st();
  z.check('час 8 — предупреждение «… и в прошлых отчётах»', s2.warn && s2.hour === '8', JSON.stringify(s2));
  await save(400);
  const armed = await z.text(page, '#setSave');
  await z.shot(page, 'z18_day_own');
  await save();
  const s3 = await st();
  z.check(`взвод «${armed}», второй клик — PUT {"report_hour":8}, в базе 8, в журнале «report_hour: 0→8»; после сохранения — `
    + '«Свой час границы», 8, правок нет',
    /Ещё раз — сдвинуть границу суток/.test(armed || '') && puts.length === 1 && puts[0] === '{"report_hour":8}' && base() === '8'
    && audit() === 'report_hour: 0→8' && s3.mode === '1' && s3.box && s3.hour === '8' && !s3.changed && /Правок нет/.test(s3.hint),
    JSON.stringify({ puts, s3, a: audit() }));

  await mode('0');
  const s4 = await st();
  z.check('обратно «С полуночи» — поле спрятано, в нём 0, выбор подсвечен, «Изменено: 1» и предупреждение',
    !s4.box && s4.hour === '0' && s4.changed && s4.warn && /Изменено: 1\b/.test(s4.hint), JSON.stringify(s4));
  await save(400);
  await save();
  const s5 = await st();
  z.check('со взводом — PUT {"report_hour":0}, в базе 0, в журнале «report_hour: 8→0»; выбор «С полуночи», правок нет',
    puts.length === 2 && puts[1] === '{"report_hour":0}' && base() === '0' && audit() === 'report_hour: 8→0'
    && s5.mode === '0' && !s5.box && !s5.changed, JSON.stringify({ puts, s5, a: audit() }));

  await mode('1');
  await page.fill('#set_rhour', '5');
  await page.click('#setUndo');
  await z.sleep(300);
  const s6 = await st();
  z.check('«Свой час» и 5, потом «Вернуть как было» — снова «С полуночи», поле спрятано, правок нет, запросов не было',
    s6.mode === '0' && !s6.box && !s6.changed && /Правок нет/.test(s6.hint) && puts.length === 2, JSON.stringify(s6));

  // ── подписи при полуночи ── граница 0 и SQL-ом: на HEAD выбора нет, и «до» показывает сами подписи
  z.sql("UPDATE settings SET value = '0' WHERE key = 'report_hour'");
  const TODAY = z.sql("SELECT (now() AT TIME ZONE 'Europe/Warsaw')::date::text");
  await page.evaluate(() => refresh()); // сводка — с границей 0 (сама она обновляется раз в 7 с)
  await z.sleep(800);
  const kt = await page.evaluate(() => (document.getElementById('kRevBox') || {}).title || '');
  await z.nav(page, 'stats');
  await page.waitForFunction(() => /·/.test((document.getElementById('shDate') || {}).textContent || ''), null, { timeout: 8000 }).catch(() => {});
  const sd = await z.text(page, '#shDate');
  z.check(`«Смена · ${TODAY} · с 00:00» (было «с 0:00»); подсказка «Выручки сегодня» — «за клубные сутки с 00:00 — …»`,
    sd === `${TODAY} · с 00:00` && /^за клубные сутки с 00:00 — /.test(kt), `${sd} · ${kt.slice(0, 60)}`);
  await page.click('#repTabs button[data-t="load"]');
  await page.waitForFunction(() => /Карта загрузки/.test((document.getElementById('repBody') || {}).textContent || ''), null, { timeout: 10000 }).catch(() => {});
  const heat = await page.evaluate(() => { const s = [...document.querySelectorAll('#repBody .sect')].find(e => /Карта загрузки/.test(e.textContent));
    return s ? s.textContent.replace(/\s+/g, ' ').trim() : ''; });
  z.check('карта загрузки — «строка — сутки с 0:00, как календарь: ночь после пятницы — уже в субботе» (было «ночь после пятницы — в пятнице»)',
    heat.includes('строка — сутки с 0:00, как календарь: ночь после пятницы — уже в субботе;'), heat.slice(0, 160));
  await z.nav(page, 'acc');
  await page.click('#accTabs button[data-t="out"]');
  await page.waitForFunction(() => /клубные сутки/.test((document.getElementById('outLabel') || {}).textContent || ''), null, { timeout: 10000 }).catch(() => {});
  const ol = await z.text(page, '#outLabel');
  z.check('«Выгрузка» — «· клубные сутки с 00:00 до 24:00, как календарь» (было «с 00:00 до 00:00 следующего дня»)',
    /· клубные сутки с 00:00 до 24:00, как календарь/.test(ol || ''), ol);

  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 800 } });
  await z.nav(ph, 'settings');
  await ph.locator('#set_seglost').scrollIntoViewIfNeeded();
  const pw = await ph.evaluate(() => { const m = document.getElementById('set_rmode'), r = m ? m.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, w: innerWidth, sel: r ? Math.round(r.right) : null }; });
  await z.shot(ph, 'z18_day_phone');
  z.check('телефон 360: выбор «Сутки клуба» в ширину карточки, вбок не прокручивается', pw.sel !== null && pw.sel <= pw.w && pw.sw <= pw.w,
    JSON.stringify(pw));
  await z.done();
})().catch(z.crash);
