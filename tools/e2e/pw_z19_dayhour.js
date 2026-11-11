// З19-и6 «Предупреждение: свой час границы позже начала смены» (из «Найдено по дороге» З18-и4з; решение основателя
// 09.11), браузер: «Экономика» → «Клубные сутки и сегменты». Свой час границы позже начала активного шаблона смены —
// под полем часа предупреждение с именами смен и их началом: запись ложится в табель следующими клубными сутками и до
// границы там не видна. Шаблоны «z19h Утро» 06:00–14:00 и выключенный «z19h Выкл» 05:00–07:00 — на время. Граница 8 —
// «z19h Утро» (06:00), выключенный не в счёт; 6 — без предупреждения; 21 — все активные с началом раньше 21:00 (как
// отдаёт /admin/shifts); «С полуночи» — без; сохранённая граница 8 — предупреждение сразу при открытии. «Сохранить» не
// нажимается. Гостей нет; шаблоны «z19h …» удаляются в начале и в конце, граница суток — SQL-ом 0, потом 8, в конце —
// как была.
const z = require('./_z');
(async () => {
  const orig = z.rows("SELECT value FROM settings WHERE key = 'report_hour'")[0];
  const setHour = h => z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${h}', now()) `
    + `ON CONFLICT (key) DO UPDATE SET value = '${h}'`);
  z.atEnd(() => {
    z.sql("DELETE FROM settings WHERE key = 'report_hour'");
    if (orig !== undefined) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${orig}', now())`);
  });
  const wipe = () => z.sql("DELETE FROM shift_assignments WHERE shift_id IN (SELECT id FROM shifts WHERE name LIKE 'z19h %'); "
    + "DELETE FROM shifts WHERE name LIKE 'z19h %'");
  const tplSnap = () => z.rows("SELECT name || ' ' || start_min || '-' || end_min || ' ' || active FROM shifts ORDER BY id").join('; ');
  wipe();
  z.atEnd(wipe);
  const before = tplSnap();
  setHour(0);
  const mk = (name, s, e, active) => z.api('POST', '/admin/shifts', { name, start_min: s, end_min: e, days_mask: 127, active, force: true }, 'boss');
  const t1 = await mk('z19h Утро', 360, 840, true), t2 = await mk('z19h Выкл', 300, 420, false);
  const off = t2.data && t2.data.id ? await z.api('PATCH', `/admin/shifts/${t2.data.id}`, { active: false }, 'boss') : { status: 0 }; // создание active не берёт
  z.check('шаблоны «z19h Утро» 06:00–14:00 и выключенный «z19h Выкл» 05:00–07:00 заведены, сутки — с полуночи',
    t1.status === 201 && t2.status === 201 && off.status === 200 && z.sql("SELECT active FROM shifts WHERE name = 'z19h Выкл'") === 'f'
    && z.sql("SELECT value FROM settings WHERE key = 'report_hour'") === '0', `${t1.status} ${t2.status} ${off.status} ${JSON.stringify(off.data).slice(0, 160)}`);
  const tpls = ((await z.api('GET', '/admin/shifts', undefined, 'boss')).data.shifts || []);
  const hh = m => String(Math.floor(m / 60)).padStart(2, '0') + ':' + String(m % 60).padStart(2, '0');
  const want = h => { // ожидаемый текст: активные шаблоны с началом раньше границы, в порядке /admin/shifts
    const early = tpls.filter(t => t.active && t.start_min < h * 60), one = early.length === 1, b = hh(h * 60);
    return early.length ? `⚠ Граница суток ${b} позже начала ${one ? 'смены' : 'смен'} `
      + early.map(t => `«${t.name}» (${hh(t.start_min)})`).join(', ') + `: ${one ? 'её запись ложится' : 'их записи ложатся'} в табель `
      + `следующими клубными сутками и до ${b} там ${one ? 'не видна' : 'не видны'}. Этого не будет при границе не позже начала смен `
      + 'или сутках с полуночи.' : '';
  };

  const page = await z.openAdmin('boss');
  const puts = [];
  page.on('request', r => { if (r.method() === 'PUT' && r.url().endsWith('/admin/settings')) puts.push(r.postData() || ''); });
  await z.nav(page, 'settings');
  const st = () => page.evaluate(() => {
    const w = document.getElementById('setRhourShift'), on = !!w && !w.classList.contains('hidden') && getComputedStyle(w).display !== 'none';
    return { on, text: on ? w.textContent : '', hint: (document.getElementById('setHint') || {}).textContent || '',
      mode: (document.getElementById('set_rmode') || {}).value, hour: (document.getElementById('set_rhour') || {}).value };
  });
  const hour = async h => { await page.fill('#set_rhour', String(h)); await z.sleep(200); return st(); };

  const s0 = await st();
  z.check('«С полуночи» — предупреждения о сменах нет', !s0.on && s0.mode === '0', JSON.stringify(s0));
  await page.selectOption('#set_rmode', '1');
  const s8 = await hour(8);
  z.check(`свой час 8 — «${want(8)}»; выключенного «z19h Выкл» нет`,
    s8.on && s8.text === want(8) && /«z19h Утро» \(06:00\)/.test(want(8)) && !/z19h Выкл/.test(s8.text), JSON.stringify(s8));
  await page.locator('#set_rhour').scrollIntoViewIfNeeded();
  await z.shot(page, 'z19_dayhour_8');
  const s6 = await hour(6);
  z.check('час 6 — граница не позже начала смен: предупреждения нет' + (want(6) ? ` (на стенде есть раньше 06:00: ${want(6)})` : ''),
    want(6) ? s6.on && s6.text === want(6) : !s6.on, JSON.stringify(s6));
  const s21 = await hour(21);
  z.check(`час 21 — все активные с началом раньше 21:00: «${want(21)}»`, s21.on && s21.text === want(21)
    && (want(21).match(/«/g) || []).length >= 2, JSON.stringify(s21));
  await page.selectOption('#set_rmode', '0'); await z.sleep(200);
  const sm = await st();
  z.check('снова «С полуночи» — предупреждения нет, правок нет', !sm.on && /Правок нет/.test(sm.hint), JSON.stringify(sm));

  setHour(8);
  await z.nav(page, 'hall'); await z.nav(page, 'settings');
  const so = await st();
  z.check(`сохранённая граница 8 — предупреждение сразу при открытии, правок нет: «${want(8)}»`,
    so.on && so.text === want(8) && so.mode === '1' && so.hour === '8' && /Правок нет/.test(so.hint), JSON.stringify(so));
  await z.shot(page, 'z19_dayhour_saved');

  wipe();
  const after = tplSnap();
  z.check('«Сохранить» не нажимался: запросов PUT /admin/settings нет, граница — как поставлена (8); шаблоны без «z19h …» — как были',
    puts.length === 0 && z.sql("SELECT value FROM settings WHERE key = 'report_hour'") === '8' && after === before,
    JSON.stringify({ puts, after: after.slice(0, 200), before: before.slice(0, 200) }));
  await z.done();
})().catch(z.crash);
