// З18-и4 «Настройки без интерфейса» (№39; readyWaitMin из №40), браузер: «Экономика». В «Сессии у ПК» — «Окно [Готов!]
// после посадки у стойки, мин» (как в /admin/settings), новая карточка «Сгорание пакетов» — «Предупредить гостя о
// сгорании пакета за столько дней». Под пакетами — «Сгорают скоро: 3 пакета на 465 мин» (пакеты Guest48 и Guest49 —
// SQL-ом, подарком, задним числом): ник, пакет, остаток, срок, «отправлено» или «ждёт»; «Предупредить сейчас — 2
// пакета» — тост, у всех «отправлено», кнопки нет, гостям — уведомления. 0 в окне [Готов!] — оранжевое «Окна нет: деньги
// идут с посадки…», «Сохранить» — один PUT {ready_wait_min: 0}; посадка на ПК без связи (шторка ПК, карточка гостя) —
// «…окно [Готов!] выключено — деньги пойдут сразу с посадки», а не «подожду его 7 мин»; снова 7 — «подожду его 7 мин».
// 0 в «Сгорании пакетов» — «Предупреждения выключены». Телефон 360 — карточки и блок без прокрутки вбок. В конце пакеты
// z18s, их уведомления и обе настройки — как были.
const z = require('./_z');
(async () => {
  const G1 = 'Guest48', G2 = 'Guest49', U1 = z.uid(G1), U2 = z.uid(G2), KEYS = ['ready_wait_min', 'pack_warn_days'];
  const [CLUB, ZONE] = z.sql("SELECT club_id || '|' || id FROM zones WHERE name = 'Standard'").split('|');
  const orig = z.rows(`SELECT key || '|' || value FROM settings WHERE key IN (${KEYS.map(k => `'${k}'`).join(', ')})`).map(r => r.split('|'));
  const tidy = () => {
    z.sql(`DELETE FROM notifications WHERE user_id IN ('${U1}', '${U2}') AND type = 'package_expiring'`);
    z.sql("DELETE FROM user_packages WHERE name LIKE 'z18s %'");
  };
  tidy();
  z.atEnd(tidy);
  z.atEnd(() => {
    z.sql(`DELETE FROM settings WHERE key IN (${KEYS.map(k => `'${k}'`).join(', ')})`);
    for (const [k, v] of orig) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('${k}', '${v}', now())`);
  });
  for (const [k, v] of [['ready_wait_min', 7], ['pack_warn_days', 3]])
    z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('${k}', '${v}', now()) ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value`);
  const mk = (u, name, left, exp, warned) => z.sql(`INSERT INTO user_packages (club_id, user_id, zone_id, name, zone_name,
    minutes_total, minutes_left, price_pln, method, comp_reason, expires_at, warned_at, created_at) VALUES ('${CLUB}', '${u}',
    '${ZONE}', '${name}', 'Standard', 600, ${left}, 45, 'comp', 'z18 проверка', now() + interval '${exp}',
    ${warned ? 'now()' : 'NULL'}, '2020-01-01')`);
  mk(U1, 'z18s 3 часа', 120, '2 days 3 hours', false);
  mk(U1, 'z18s Ночь', 300, '20 hours', true);
  mk(U2, 'z18s Вечер', 45, '1 day 2 hours', false);
  mk(U2, 'z18s Месяц', 600, '20 days', false);
  const notes = () => z.rows(`SELECT u.nickname || ':' || (n.payload->>'name') FROM notifications n JOIN users u ON u.id = n.user_id
    WHERE n.user_id IN ('${U1}', '${U2}') AND n.type = 'package_expiring' ORDER BY 1`).join(', ');

  const page = await z.openAdmin('boss');
  const puts = [];
  page.on('request', r => { if (r.method() === 'PUT' && r.url().endsWith('/admin/settings')) puts.push(r.postData() || ''); });
  await z.nav(page, 'settings');
  await page.waitForFunction(() => !document.querySelector('#packSoon .skel'), null, { timeout: 8000 }).catch(() => {});
  const field = id => page.evaluate(id => { const e = document.getElementById(id); if (!e) return null;
    const c = e.closest('.setcard'), l = document.querySelector(`label[for="${id}"]`);
    return { v: e.value, card: c ? c.querySelector('h4').textContent : '', label: l ? l.textContent : '' }; }, id);
  const f1 = await field('set_readywait'), f2 = await field('set_packwarn');
  z.check('«Сессия у ПК» — «Окно [Готов!] после посадки у стойки, мин (0 — окна нет, деньги идут сразу)» = 7; новая карточка '
    + '«Сгорание пакетов» — «Предупредить гостя о сгорании пакета за столько дней (0 — молчать)» = 3',
    !!f1 && f1.card === 'Сессия у ПК' && f1.v === '7' && f1.label === 'Окно [Готов!] после посадки у стойки, мин (0 — окна нет, деньги идут сразу)'
    && !!f2 && f2.card === 'Сгорание пакетов' && f2.v === '3' && f2.label === 'Предупредить гостя о сгорании пакета за столько дней (0 — молчать)',
    JSON.stringify([f1, f2]));

  const soon = () => page.evaluate(() => { const b = document.getElementById('packSoon'); if (!b) return null;
    const t = e => e ? e.innerText.replace(/\s+/g, ' ').trim() : '';
    return { h: t(b.querySelector('h4')), note: t(b.querySelector('.muted')), orange: !b.classList.contains('calm'),
      rows: [...b.querySelectorAll('tr')].slice(1).map(r => [...r.cells].map(c => t(c))),
      btn: t(document.getElementById('packWarnBtn')) }; });
  const s1 = await soon();
  const row = (s, nick, pack) => s && s.rows.find(r => r[0] === nick && r[1].startsWith(pack));
  z.check('«Сгорают скоро: 3 пакета на 465 мин» — оранжевым; «Уже предупреждены: 1 из 3»; месячного (20 дней) нет',
    !!s1 && s1.h === 'Сгорают скоро: 3 пакета на 465 мин' && s1.orange && /Уже предупреждены: 1 из 3\./.test(s1.note)
    && /которым до срока 3 дня или меньше/.test(s1.note) && !s1.rows.some(r => r[1].startsWith('z18s Месяц')), JSON.stringify(s1));
  const r1 = row(s1, G1, 'z18s Ночь'), r2 = row(s1, G2, 'z18s Вечер'), r3 = row(s1, G1, 'z18s 3 часа');
  z.check('строки: Guest48 «Ночь» 5 ч — «отправлено», Guest49 «Вечер» 45 мин — «ждёт», Guest48 «3 часа» 2 ч — «ждёт»; срок — через 1, 2 и 3 дня',
    !!r1 && r1[2] === '5 ч' && r1[4] === 'отправлено' && / · через 1 день$/.test(r1[3]) && !!r2 && r2[2] === '45 мин' && r2[4] === 'ждёт'
    && / · через 2 дня$/.test(r2[3]) && !!r3 && r3[2] === '2 ч' && r3[4] === 'ждёт' && / · через 3 дня$/.test(r3[3]), JSON.stringify([r1, r2, r3]));
  await page.evaluate(() => { const b = document.getElementById('packSoon'); if (b) b.scrollIntoView({ block: 'center' }); });
  await z.shot(page, 'z18_settings_soon');
  const toast = await z.toastAfter(page, async () => { if (await page.$('#packWarnBtn')) await page.click('#packWarnBtn'); }, 'Предупредили'); // З19-и13: тост этого действия, а не #toast через паузу
  const s2 = await soon();
  z.check('«Предупредить сейчас — 2 пакета» → тост «Предупредили: 2 пакета — гостям ушло уведомление»; у всех «отправлено», кнопки нет; '
    + 'уведомления — Guest48 «3 часа», Guest49 «Вечер»', !!s1 && s1.btn === 'Предупредить сейчас — 2 пакета'
    && toast === 'Предупредили: 2 пакета — гостям ушло уведомление' && !!s2 && s2.rows.length === 3 && s2.rows.every(r => r[4] === 'отправлено')
    && s2.btn === '' && /Уже предупреждены: 3 из 3\./.test(s2.note) && notes() === `${G1}:z18s 3 часа, ${G2}:z18s Вечер`,
    JSON.stringify({ btn: s1 && s1.btn, toast, s2, notes: notes() }));

  // окно [Готов!] = 0: предупреждение у поля, один PUT, подсказки посадки
  const setField = async (id, v) => { if (await page.$('#' + id)) { await page.fill('#' + id, String(v)); await page.dispatchEvent('#' + id, 'input'); } };
  const save = async () => { if (await page.$('#setSave')) await page.click('#setSave'); await z.sleep(1000); };
  await setField('set_readywait', 0);
  const warn = await page.evaluate(() => { const w = document.getElementById('setReadyWarn');
    return w && !w.classList.contains('hidden') ? w.textContent : null; });
  await page.evaluate(() => { const e = document.getElementById('set_readywait'); if (e) e.closest('.setcard').scrollIntoView({ block: 'start' }); });
  await z.shot(page, 'z18_settings_ready0');
  await save();
  z.check('0 в окне [Готов!] — «⚠ Окна нет: деньги идут с посадки у стойки, …»; «Сохранить» — один PUT {"ready_wait_min":0}, в базе 0',
    /^⚠ Окна нет: деньги идут с посадки у стойки/.test(warn || '') && puts.length === 1 && puts[0] === '{"ready_wait_min":0}'
    && z.sql("SELECT value FROM settings WHERE key = 'ready_wait_min'") === '0', JSON.stringify({ warn, puts }));
  // поля нет (HEAD) — 0 через API: подсказки посадки ниже проверяются при окне, выключенном в любом случае
  const ensure = async v => { if (z.sql("SELECT value FROM settings WHERE key = 'ready_wait_min'") !== String(v))
    await z.api('PUT', '/admin/settings', { ready_wait_min: v }, 'boss'); };
  await ensure(0);
  const PC = z.sql("SELECT id FROM computers WHERE name = 'PC-05'");
  const pcHint = async () => { await z.nav(page, 'hall');
    await page.evaluate(id => openPCDrawer(id), PC); await z.sleep(700);
    return page.evaluate(() => { const b = document.getElementById('drawerBody'); const d = b && [...b.querySelectorAll('div.muted')].find(x => /^⏻/.test(x.textContent));
      return d ? d.textContent.trim() : null; }); };
  const h0 = await pcHint();
  await page.evaluate(u => openUserDrawer(u, 'seat'), U1);
  await z.sleep(1200);
  const card0 = await page.evaluate(() => { const b = document.getElementById('drawerBody'); if (!b) return null;
    const ds = [...b.querySelectorAll('.uform div.muted')]; const h = ds.find(x => /^⏻/.test(x.textContent));
    return { hint: h ? h.textContent.trim() : null, note: (ds.find(x => /^Без пароля/.test(x.textContent)) || {}).textContent || null }; });
  z.check('окно выключено: шторка ПК без связи — «⏻ ПК не на связи, а окно [Готов!] выключено («Экономика» → «Сессия у ПК») — деньги пойдут '
    + 'сразу с посадки, по сети не бужу» (не «подожду его 7 мин»); в карточке гостя — то же и «окна [Готов!] нет — деньги идут с посадки»',
    h0 === '⏻ ПК не на связи, а окно [Готов!] выключено («Экономика» → «Сессия у ПК») — деньги пойдут сразу с посадки, по сети не бужу'
    && !!card0 && card0.hint === h0 && /· окна \[Готов!\] нет — деньги идут с посадки$/.test(card0.note || ''), JSON.stringify({ h0, card0 }));
  await page.evaluate(() => closeDrawer());
  await z.nav(page, 'settings');
  await setField('set_readywait', 7);
  await save();
  await ensure(7);
  const h7 = await pcHint();
  z.check('снова 7 — «⏻ ПК не на связи — включу по сети и подожду его 7 мин; деньги пойдут только после [Готов!]»',
    h7 === '⏻ ПК не на связи — включу по сети и подожду его 7 мин; деньги пойдут только после [Готов!]', JSON.stringify(h7));
  await page.evaluate(() => closeDrawer());
  await z.nav(page, 'settings');
  await setField('set_packwarn', 0);
  await save();
  await page.waitForFunction(() => /выключены/.test((document.getElementById('packSoon') || {}).textContent || ''), null, { timeout: 8000 }).catch(() => {});
  const s0 = await soon();
  z.check('0 в «Сгорании пакетов» — «Сгорают скоро» без оранжевого: «Предупреждения выключены: … Пакет со сроком сгорит молча.»',
    !!s0 && s0.h === 'Сгорают скоро' && !s0.orange && /^Предупреждения выключены: в карточке «Сгорание пакетов» выше — 0 дней\. Пакет со сроком сгорит молча\.$/.test(s0.note)
    && z.sql("SELECT value FROM settings WHERE key = 'pack_warn_days'") === '0', JSON.stringify(s0));

  // телефон: карточки и блок в ширину экрана
  z.sql("UPDATE settings SET value = '3' WHERE key = 'pack_warn_days'");
  await z.sleep(2500); // лимитер входа с одного адреса
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 800 } });
  await z.nav(ph, 'settings');
  await ph.waitForFunction(() => !document.querySelector('#packSoon .skel'), null, { timeout: 8000 }).catch(() => {});
  const fit = await ph.evaluate(() => { const els = ['set_readywait', 'set_packwarn', 'packSoon'].map(i => document.getElementById(i));
    return { ok: els.every(e => !!e && e.getBoundingClientRect().right <= innerWidth + 0.5 && e.getBoundingClientRect().left >= 0),
      over: document.documentElement.scrollWidth - innerWidth }; });
  await ph.evaluate(() => { const b = document.getElementById('packSoon'); if (b) b.scrollIntoView({ block: 'start' }); });
  await z.shot(ph, 'z18_settings_phone');
  z.check('телефон 360: поля окна [Готов!] и сгорания пакетов, «Сгорают скоро» — в ширину экрана, вбок не прокручивается',
    fit.ok && fit.over <= 0, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
