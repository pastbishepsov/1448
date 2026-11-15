// З24-и2 «Расписание цен — админка и табло» (№53 «в»), браузер. Временная зона «z24u Вечер» по 10 zł/ч, ПК z24u-1 на
// свободной клетке зала и z24u-2 вне схемы (без агента — offline), правило «каждый день» с 10 минут назад на час —
// 25 zł/ч (API). Админ, «Зал»: на плитках свободных ПК — цена посадки сейчас «25 zł» золотом (выше цены зоны), в шторке
// ПК — «Цена часа: сейчас 25,00 zł/ч · с … — 10,00 zł/ч». Владелец, «Экономика» → «Зоны»: в ячейке цены — «сейчас 25,00
// · с … — 10,00 · 1 правило»; шторка зоны — «Расписание цен»: правило словами, заметка, «идёт сейчас»; «Правило» —
// внахлёст: в форме ответ сервера «Накладывается на правило «…»», встык — добавлено (2 правила); «Правка» — цена и
// заметка; ✕ двумя нажатиями — убрано; без дней — «Отметь хотя бы один день»; «Сохранить» зоны с набранным правилом —
// не закрывает шторку. Журнал — подписи и значки трёх видов; палитра «расписание цен» — к зонам. Телефон 360 — шторка
// с формой правила без прокрутки вбок. Табло — у зоны «сейчас 25 zł/ч · с … — 10 zł/ч». В конце ПК и зоны «z24u …» (с
// правилами) удаляются SQL-ом, токен табло — как был.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const ZN = 'z24u Вечер';
  const tidy = () => { z.sql(`DELETE FROM computers WHERE name LIKE 'z24u-%'`); z.sql(`DELETE FROM zones WHERE name LIKE 'z24u %'`); };
  tidy();
  z.atEnd(tidy);
  const hm = m => `${String(Math.floor(m / 60) % 24).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
  const hmEnd = m => (m === 1440 ? '24:00' : hm(m));
  const whenLbl = iso => { // как в админке и на табло: сегодня — «с 18:00», завтра — «завтра с 10:00»
    const d = new Date(iso), n = new Date(), day = x => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
    const dd = Math.round((day(d) - day(n)) / 864e5), t = hm(d.getHours() * 60 + d.getMinutes());
    return dd <= 0 ? 'с ' + t : dd === 1 ? 'завтра с ' + t : 'с ? ' + t;
  };
  const rules = () => z.rows(`SELECT rate_pln || '|' || start_min || '|' || end_min || '|' || note FROM zone_price_rules
    WHERE zone_id = (SELECT id FROM zones WHERE name = '${ZN}') ORDER BY created_at`);

  // ── данные: зона, ПК, правило «сейчас» (API)
  const W = +z.sql('SELECT layout_w FROM clubs WHERE is_active ORDER BY name LIMIT 1') || 12;
  const H = +z.sql('SELECT layout_h FROM clubs WHERE is_active ORDER BY name LIMIT 1') || 8;
  const busy = new Set(z.rows("SELECT pos_x || ':' || pos_y FROM computers WHERE pos_x IS NOT NULL"));
  let cell = null;
  for (let y = H - 1; y >= 0 && !cell; y--) for (let x = 0; x < W && !cell; x++) if (!busy.has(x + ':' + y)) cell = [x, y];
  const zid = ((await z.api('POST', '/admin/zones', { name: ZN, rate_pln: 10 }, 'boss')).data.zone || {}).id;
  const p1 = (await z.api('POST', '/admin/computers', { name: 'z24u-1', zone_id: zid, pos_x: cell[0], pos_y: cell[1] }, 'boss')).data.computer_id;
  const p2 = (await z.api('POST', '/admin/computers', { name: 'z24u-2', zone_id: zid }, 'boss')).data.computer_id;
  const now = +z.sql("SELECT (extract(hour FROM now() AT TIME ZONE 'Europe/Warsaw') * 60 + extract(minute FROM now() AT TIME ZONE 'Europe/Warsaw'))::int");
  const S0 = (now - 10 + 1440) % 1440, E0 = (now + 50) % 1440 || 1440;
  const R1 = `каждый день ${hm(S0)}–${hmEnd(E0)} — 25,00 zł/ч`;
  const c1 = await z.api('POST', `/admin/zones/${zid}/rules`, { days_mask: 127, start_min: S0, end_min: E0, rate_pln: 25, note: 'z24u вечер' }, 'boss');
  const zrow = async () => ((await z.api('GET', '/admin/zones', undefined, 'boss')).data.zones || []).find(x => x.id === zid) || {};
  const zr = await zrow();
  const NX = zr.next_at ? whenLbl(zr.next_at) : '?';
  z.check(`«${ZN}» 10 zł/ч, ПК z24u-1 (клетка ${cell}) и z24u-2, правило «${R1}» (API): сейчас 25, ${NX} — 10`,
    !!zid && !!p1 && !!p2 && c1.status === 201 && zr.rate_now_pln === 25 && zr.next_rate_pln === 10 && !!zr.next_at,
    JSON.stringify([c1.status, zr.rate_now_pln, zr.next_at, zr.next_rate_pln]));

  // ── зал (админ): цена на плитках и в шторке ПК
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'hall'); await z.sleep(1500);
  const has = await adm.evaluate(() => typeof zrBoxHtml === 'function' && typeof pcPriceTag === 'function');
  if (!has) { // расписания в админке нет (до З24-и2) — FAIL по каждой проверке, а не обрыв
    for (const n of ['плитка ПК', 'плитка вне схемы', 'шторка ПК', 'список зон', 'шторка зоны', 'внахлёст', 'встык', 'правка',
      'убрать', 'без дней и «Сохранить» зоны', 'журнал', 'палитра', 'телефон 360']) z.check(n + ': расписания цен в админке нет', false);
  } else {
    const tiles = await adm.evaluate(() => {
      const h = [...document.querySelectorAll('.hpc')].find(t => t.querySelector('.nm').textContent === 'z24u-1');
      const p = [...document.querySelectorAll('.pc')].find(t => t.querySelector('.nm') && t.querySelector('.nm').textContent === 'z24u-2');
      const pr = h && h.querySelector('.inf2 .pr');
      return { hpc: h ? h.innerText.replace(/\s+/g, ' ').trim() : null, pr: pr ? pr.textContent : null, cls: pr ? pr.className : null,
        pc: p ? p.querySelector('.info').innerText.replace(/\s+/g, ' ').trim() : null, pcCls: p && p.querySelector('.pr') ? p.querySelector('.pr').className : null };
    });
    z.check('«Зал»: плитка z24u-1 — «offline» и цена посадки «25 zł» золотом (выше цены зоны)',
      tiles.pr === '25 zł' && /\bup\b/.test(tiles.cls || '') && /offline/.test(tiles.hpc || ''), JSON.stringify(tiles));
    z.check('«Зал» → «Вне схемы»: плитка z24u-2 — «offline · 25 zł»', /offline · 25 zł$/.test(tiles.pc || '') && /\bup\b/.test(tiles.pcCls || ''), JSON.stringify(tiles));
    await adm.evaluate(id => openPCDrawer(id), p1); await z.sleep(700);
    const dp = await z.text(adm, '#dPrice');
    z.check(`шторка ПК z24u-1: «Цена часа» — «сейчас 25,00 zł/ч · ${NX} — 10,00 zł/ч»`, dp === `сейчас 25,00 zł/ч · ${NX} — 10,00 zł/ч`, dp);
    await adm.evaluate(() => closeDrawer());

    // ── «Экономика» → «Зоны» (владелец)
    const boss = await z.openAdmin('boss');
    await z.nav(boss, 'settings');
    await boss.waitForFunction(n => [...document.querySelectorAll('#zonesTable tr')].some(t => t.cells[0] && t.cells[0].innerText.trim() === n), ZN,
      { timeout: 10000 }).catch(() => {});
    const cellTxt = () => boss.evaluate(n => { const r = [...document.querySelectorAll('#zonesTable tr')].find(t => t.cells[0] && t.cells[0].innerText.trim() === n);
      return r ? r.cells[1].innerText.replace(/\s+/g, ' ').trim() : null; }, ZN);
    const ct = await cellTxt();
    z.check(`список зон: у «${ZN}» — «10,00 zł» и «сейчас 25,00 · ${NX} — 10,00 · 1 правило»`, ct === `10,00 zł сейчас 25,00 · ${NX} — 10,00 · 1 правило`, ct);
    await boss.evaluate(id => zoneEdit(id), zid); await z.sleep(600);
    const box = () => boss.evaluate(() => { const b = document.getElementById('zr_box'); if (!b) return null;
      const e = document.getElementById('zr_err');
      return { now: (document.getElementById('zr_now') || {}).textContent || '', rows: [...b.querySelectorAll('.zrRow')].map(r => r.querySelector('.zrTxt').innerText.replace(/\s+/g, ' ').trim()),
        on: [...b.querySelectorAll('.zrRow.zrOn')].length, form: !!document.getElementById('zr_f'), add: !!document.querySelector('#zr_add:not(.hidden)'),
        err: e && !e.classList.contains('hidden') ? e.textContent : '' }; });
    const b1 = await box();
    z.check(`шторка зоны: «Расписание цен» — «${R1}», «идёт сейчас», заметка; над списком — «сейчас 25,00 zł/ч · ${NX} — 10,00 zł/ч»`,
      !!b1 && b1.rows.length === 1 && b1.rows[0] === `${R1} идёт сейчас z24u вечер` && b1.on === 1 && b1.now === `сейчас 25,00 zł/ч · ${NX} — 10,00 zł/ч` && b1.add,
      JSON.stringify(b1));
    await z.shot(boss, 'z24_zone_drawer');

    // внахлёст — ответ сервера в форме; встык — добавлено
    const fill = async (s, e, rate, note) => {
      await boss.evaluate(() => zrDays(127));
      await boss.fill('#zr_start', hm(s)); await boss.fill('#zr_end', hm(e % 1440));
      await boss.fill('#zr_rate', String(rate)); if (note != null) await boss.fill('#zr_note', note);
    };
    await boss.click('#zr_add'); await z.sleep(300);
    const E = E0 % 1440;
    await fill((E - 30 + 1440) % 1440, (E + 30) % 1440, 30, 'z24u ночь');
    await boss.click('#zr_save'); await z.idle(boss); // З19-и14: после записи — ждём страницу, а не паузу
    const b2 = await box();
    z.check(`«Правило» внахлёст (${hm((E - 30 + 1440) % 1440)}–${hm((E + 30) % 1440)}): в форме — «Накладывается на правило «${R1}»…», правило одно`,
      !!b2 && b2.form && /Накладывается на правило «каждый день/.test(b2.err) && b2.err.includes(R1) && rules().length === 1, JSON.stringify([b2, rules()]));
    await fill(E, E + 60, 30, 'z24u ночь');
    const t3 = await z.toastAfter(boss, () => boss.click('#zr_save'), 'Правило добавлено'); // З19-и13: тост этого действия, а не #toast через паузу
    const R2 = `каждый день ${hm(E)}–${hmEnd(E + 60 > 1440 ? (E + 60) % 1440 : E + 60)} — 30,00 zł/ч`;
    const b3 = await box(), ct3 = await cellTxt();
    z.check(`встык (${hm(E)}–${hm((E + 60) % 1440)}) — добавлено: тост «✓ Правило добавлено: ${R2}», в шторке два правила, форма закрыта; в списке — «2 правила»`,
      !!b3 && b3.rows.length === 2 && b3.rows[1] === `${R2} z24u ночь` && !b3.form && b3.add && /· 2 правила$/.test(ct3 || '')
      && (t3 || '').includes('Правило добавлено: ' + R2) && rules().length === 2, JSON.stringify([b3, ct3, t3]));

    // правка: цена и заметка
    await boss.evaluate(() => { const r = document.querySelectorAll('#zr_box .zrRow')[1]; r.querySelector('button').click(); });
    await z.sleep(300);
    const pre = await boss.evaluate(() => ({ rate: document.getElementById('zr_rate').value, start: document.getElementById('zr_start').value,
      days: [...document.querySelectorAll('.zrDay')].filter(c => c.checked).length, save: document.getElementById('zr_save').textContent.trim() }));
    await boss.fill('#zr_rate', '35'); await boss.fill('#zr_note', 'z24u ночь дороже');
    await boss.click('#zr_save'); await z.idle(boss);
    const b4 = await box();
    z.check('«Правка»: форма с цифрами правила (30, начало, 7 дней, «Сохранить правило») — цена 35 и заметка сохранены',
      pre.rate === '30' && pre.start === hm(E) && pre.days === 7 && pre.save === 'Сохранить правило' && !!b4
      && b4.rows[1] === `${R2.replace('30,00', '35,00')} z24u ночь дороже` && rules()[1] === `35.00|${E}|${(E + 60) % 1440 || 1440}|z24u ночь дороже`,
      JSON.stringify([pre, b4 && b4.rows, rules()]));

    // ✕ двумя нажатиями
    const del = () => boss.evaluate(() => { const r = document.querySelectorAll('#zr_box .zrRow')[1]; const b = r && r.querySelector('button.danger'); if (b) b.click(); return b ? b.textContent : null; });
    const rowFit = await boss.evaluate(() => [...document.querySelectorAll('#zr_box .zrRow')].map(r => { const b = [...r.querySelectorAll('button')].map(x => Math.round(x.getBoundingClientRect().top));
      return b.length === 2 && b[0] === b[1]; }));
    await del(); await z.sleep(200);
    const armed = await boss.evaluate(() => { const r = document.querySelectorAll('#zr_box .zrRow')[1]; return r ? r.querySelector('button.danger').textContent : null; });
    const n1 = rules().length;
    await del(); await z.idle(boss);
    const b5 = await box();
    z.check('«Правка» и ✕ правила — в одну строку; ✕: первое нажатие — «Убрать? Ещё раз» (правило на месте), второе — убрано, осталось одно',
      rowFit.length === 2 && rowFit.every(Boolean) && /Убрать\? Ещё раз/.test(armed || '') && n1 === 2 && !!b5 && b5.rows.length === 1 && rules().length === 1, JSON.stringify([rowFit, armed, n1, b5 && b5.rows]));

    // без дней — ошибка в форме; «Сохранить» зоны с набранным правилом — шторка остаётся
    await boss.click('#zr_add'); await z.sleep(300);
    await boss.evaluate(() => zrDays(0)); await boss.fill('#zr_rate', '12');
    await boss.click('#zr_save'); await z.sleep(400);
    const b6 = await box();
    await z.toastAfter(boss, () => boss.evaluate(() => { const b = [...document.querySelectorAll('#drawer .uform button')].find(x => x.textContent.trim() === 'Сохранить'); if (b) b.click(); }),
      'Правило не добавлено'); // снимок ниже — с этим тостом
    const st = await boss.evaluate(() => ({ open: document.getElementById('drawer').classList.contains('open'), toast: document.getElementById('toast').textContent }));
    z.check('без дней — «Отметь хотя бы один день»; «Сохранить» зоны с набранным правилом — шторка открыта, тост «Правило не добавлено …»',
      !!b6 && /Отметь хотя бы один день/.test(b6.err) && st.open && /Правило не добавлено/.test(st.toast) && rules().length === 1, JSON.stringify([b6 && b6.err, st]));
    await boss.evaluate(id => zrPaint(id), zid);
    await boss.evaluate(() => closeDrawer());

    // журнал и палитра
    const lbl = await boss.evaluate(() => [AUD_LABEL.zone_rule_create, AUD_LABEL.zone_rule_update, AUD_LABEL.zone_rule_delete,
      AUD_ICON.zone_rule_create, AUD_ICON.zone_rule_update, AUD_ICON.zone_rule_delete, ['zone_rule_create', 'zone_rule_update', 'zone_rule_delete'].every(k => AUD_GROUPS.pc.includes(k))]);
    z.check('журнал: «зона: правило цены», «… — правка», «… убрано», значки plus, edit, cross, раздел «ПК и зал»',
      JSON.stringify(lbl) === JSON.stringify(['зона: правило цены', 'зона: правило цены — правка', 'зона: правило цены убрано', 'plus', 'edit', 'cross', true]), JSON.stringify(lbl));
    await boss.evaluate(() => nav('hall')); await z.sleep(800);
    await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'расписание цен'); await z.sleep(500);
    const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent.trim()).slice(0, 3));
    await boss.keyboard.press('Enter'); await z.sleep(2000);
    const w = await boss.evaluate(() => { const h = document.getElementById('zonesHead'); return { tab: curTab, top: h ? Math.round(h.getBoundingClientRect().top) : null, h: innerHeight }; });
    z.check(`палитра: «расписание цен» → «${items[0] || '—'}», Enter — «Экономика», зоны на экране`,
      items[0] === 'Расписание цен' && w.tab === 'settings' && w.top !== null && w.top >= 0 && w.top < w.h, JSON.stringify({ items, w }));

    // телефон 360: шторка зоны с формой правила
    const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
    await z.nav(ph, 'settings'); await z.sleep(800);
    await ph.evaluate(id => zoneEdit(id), zid); await z.sleep(600);
    await ph.evaluate(id => zrForm(id, ''), zid); await z.sleep(400);
    const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth,
      r: [...document.querySelectorAll('#zr_box input, #zr_box button, #zr_box .zrRow')].map(e => Math.round(e.getBoundingClientRect().right)),
      dw: Math.round(document.getElementById('drawer').getBoundingClientRect().right) }));
    await ph.evaluate(() => document.getElementById('zr_f').scrollIntoView({ block: 'start' })); await z.sleep(300);
    await z.shot(ph, 'z24_zone_phone');
    z.check('телефон 360: шторка зоны с формой правила — поля и кнопки в экране, вбок не прокручивается',
      fit.r.length > 8 && fit.r.every(x => x <= fit.w) && fit.dw <= fit.w + 1 && fit.sw <= fit.w, JSON.stringify(fit));
  }

  // ── табло: «сейчас 25 zł/ч · с … — 10 zł/ч»
  let tok = ((await z.api('GET', '/admin/board/token', undefined, 'boss')).data || {}).token || '';
  if (!tok) {
    tok = ((await z.api('POST', '/admin/board/token', undefined, 'boss')).data || {}).token || '';
    z.atEnd(() => z.api('DELETE', '/admin/board/token', undefined, 'boss'));
  }
  const browser = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => browser.close());
  const tv = await (await browser.newContext({ viewport: { width: 1920, height: 1080 } })).newPage();
  const tvErr = []; tv.on('pageerror', e => tvErr.push(e.message));
  await tv.goto(z.BASE + '/board?token=' + tok, { waitUntil: 'load' }); await z.sleep(2000);
  const bz = await tv.evaluate(n => { const zz = [...document.querySelectorAll('#zones .zone')].find(x => x.querySelector('.nm').textContent === n);
    const r = zz && zz.querySelector('.rate');
    return r ? { txt: r.innerText.replace(/\s+/g, ' ').trim(), down: !!r.querySelector('.down'), cnt: zz.querySelector('.cnt').innerText.replace(/\s+/g, ' ') } : null; }, ZN);
  const std = await tv.evaluate(() => [...document.querySelectorAll('#zones .zone .rate')].map(r => r.innerText.trim()));
  await tv.screenshot({ path: `${z.OUT}/z_z24_board.png` });
  z.check(`табло: у «${ZN}» — «сейчас 25 zł/ч · ${NX} — 10 zł/ч» (дешевле — зелёным), 2 ПК; у зон без правил — просто цена`,
    !!bz && bz.txt === `сейчас 25 zł/ч · ${NX} — 10 zł/ч` && bz.down && /\/ 2$/.test(bz.cnt) && std.some(t => /^\d+(,\d+)? zł\/ч$/.test(t)),
    JSON.stringify([bz, std]));
  z.check('табло: ошибок JS нет', tvErr.length === 0, tvErr.join(' | '));
  await z.done();
})().catch(z.crash);
