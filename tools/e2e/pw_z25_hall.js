// З25-и2 «Экспорт и импорт зала — экран» (№56), браузер. Зона «z25v Зона» (12 zł/ч) и ПК z25v-1 на свободной клетке с
// MAC, z25v-2 вне схемы (API). Владелец, «Зал» → «Редактор зала»: «Выгрузить зал» — файл hall-1448-ГГГГ-ММ-ДД.json со
// своей зоной и ПК; свои ПК и зону удалили (API) — «Загрузить из файла» тем же файлом: просмотр «зоны: новых 1 …, ПК:
// новых 2 …», строки «z25v Зона — новая», «z25v-1 — новый · z25v Зона · клетка …», «Применить» двумя нажатиями — зал
// как был (клетка, MAC, цена). Не JSON и чужой файл — ошибка в шторке. У админа кнопок нет. Журнал, палитра «шаблон
// зала», телефон 360 — шторка с просмотром без прокрутки вбок. В конце ПК «z25v-…» и зона «z25v …» удаляются SQL-ом.
const z = require('./_z');
const fs = require('fs');
(async () => {
  const wipe = () => { z.sql("DELETE FROM computers WHERE name LIKE 'z25v-%'"); z.sql("DELETE FROM zones WHERE name LIKE 'z25v %'"); };
  wipe();
  z.atEnd(wipe);
  const W = +z.sql('SELECT layout_w FROM clubs WHERE is_active ORDER BY name LIMIT 1'), H = +z.sql('SELECT layout_h FROM clubs WHERE is_active ORDER BY name LIMIT 1');
  const busy = new Set(z.rows("SELECT pos_x || ':' || pos_y FROM computers WHERE pos_x IS NOT NULL"));
  let cell = null;
  for (let y = H - 1; y >= 0 && !cell; y--) for (let x = W - 1; x >= 0 && !cell; x--) if (!busy.has(x + ':' + y)) cell = [x, y];
  const zid = (((await z.api('POST', '/admin/zones', { name: 'z25v Зона', rate_pln: 12 }, 'boss')).data || {}).zone || {}).id;
  const p1 = ((await z.api('POST', '/admin/computers', { name: 'z25v-1', zone_id: zid, pos_x: cell[0], pos_y: cell[1] }, 'boss')).data || {}).computer_id;
  await z.api('PATCH', `/admin/computers/${p1}`, { mac: 'AA:BB:CC:00:25:0A' }, 'boss');
  const p2 = ((await z.api('POST', '/admin/computers', { name: 'z25v-2', zone_id: zid }, 'boss')).data || {}).computer_id;
  const CELL = `клетка ${cell[0] + 1}×${cell[1] + 1}`;
  z.check(`зона «z25v Зона», ПК z25v-1 (${CELL}, MAC) и z25v-2 вне схемы (API)`, !!zid && !!p1 && !!p2, JSON.stringify([zid, p1, p2]));

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'hall'); await z.sleep(800);
  const has = await boss.evaluate(() => typeof openHallImport === 'function' && !!document.getElementById('hallImpBtn'));
  if (!has) { // зала файлом нет (до З25-и2) — FAIL по каждой проверке, а не обрыв
    for (const n of ['кнопки в редакторе', 'выгрузить', 'просмотр', 'применить', 'ошибки', 'админ', 'журнал', 'палитра', 'телефон 360'])
      z.check(n + ': зала файлом нет', false);
    return z.done();
  }
  const vis = (pg, id) => pg.evaluate(i => { const e = document.getElementById(i); return !!e && getComputedStyle(e).display !== 'none'; }, id);
  const before = [await vis(boss, 'hallExpBtn'), await vis(boss, 'hallImpBtn')];
  await boss.evaluate(() => toggleHallEdit()); await z.sleep(600);
  const after = [await vis(boss, 'hallExpBtn'), await vis(boss, 'hallImpBtn')];
  z.check('«Редактор зала»: «Выгрузить зал» и «Загрузить из файла» появились (вне редактора их нет)',
    before.join() === 'false,false' && after.join() === 'true,true', JSON.stringify([before, after]));
  const [dl] = await Promise.all([boss.waitForEvent('download', { timeout: 30000 }), boss.click('#hallExpBtn')]);
  const path = await dl.path(), file = JSON.parse(fs.readFileSync(path, 'utf8'));
  const fz = (file.zones || []).find(x => x.name === 'z25v Зона') || {}, f1 = (file.computers || []).find(x => x.name === 'z25v-1') || {};
  const today = z.sql("SELECT to_char(now() AT TIME ZONE 'Europe/Warsaw', 'YYYY-MM-DD')");
  z.check(`«Выгрузить зал» — файл hall-1448-${today}.json: зона 12 zł/ч, z25v-1 — клетка и MAC`,
    dl.suggestedFilename() === `hall-1448-${today}.json` && file.format === '1448-hall' && fz.rate_pln === 12 && f1.pos_x === cell[0]
    && f1.pos_y === cell[1] && f1.mac === 'AA:BB:CC:00:25:0A' && (file.computers || []).some(x => x.name === 'z25v-2' && x.pos_x === undefined),
    JSON.stringify([dl.suggestedFilename(), fz, f1]));

  // чистый клуб → загрузить тем же файлом
  await z.api('DELETE', `/admin/computers/${p1}`, undefined, 'boss');
  await z.api('DELETE', `/admin/computers/${p2}`, undefined, 'boss');
  await z.api('DELETE', `/admin/zones/${zid}`, undefined, 'boss');
  await boss.click('#hallImpBtn'); await z.sleep(500);
  await boss.setInputFiles('#hiFile', path); await z.idle(boss); // З19-и14: после записи — ждём страницу, а не паузу
  const pv = await boss.evaluate(() => ({ title: document.getElementById('drawerTitle').textContent, sum: (document.getElementById('hiSum') || {}).textContent || '',
    zones: [...document.querySelectorAll('#hiZones .hiRow')].map(r => r.innerText.replace(/\s+/g, ' ').trim()),
    pcs: [...document.querySelectorAll('#hiPCs .hiRow')].map(r => r.innerText.replace(/\s+/g, ' ').trim()), apply: !!document.getElementById('hiApply') }));
  await z.shot(boss, 'z25_hall_import');
  z.check(`«Загрузить из файла» тем же файлом: «зоны: новых 1 …; ПК: новых 2 …», «z25v Зона новая», «z25v-1 новый z25v Зона · ${CELL}», «z25v-2 … вне схемы»`,
    pv.title === 'Зал из файла' && /зоны: новых 1/.test(pv.sum) && /ПК: новых 2/.test(pv.sum) && pv.zones.includes('z25v Зона новая')
    && pv.pcs.includes(`z25v-1 новый z25v Зона · ${CELL}`) && pv.pcs.includes('z25v-2 новый z25v Зона · вне схемы') && pv.apply
    && z.sql("SELECT count(*) FROM computers WHERE name LIKE 'z25v-%'") === '0', JSON.stringify(pv));
  await boss.click('#hiApply'); await z.sleep(200);
  const armed = await boss.evaluate(() => (document.getElementById('hiApply') || {}).textContent || '');
  const t1 = await z.toastAfter(boss, () => boss.click('#hiApply'), 'Зал из файла'); // З19-и13: тост этого действия, а не #toast через паузу
  const back = z.sql("SELECT concat_ws('|', c.pos_x, c.pos_y, c.mac, z.name, z.rate_pln) FROM computers c JOIN zones z ON z.id = c.zone_id WHERE c.name = 'z25v-1'");
  z.check('«Применить» — первое нажатие «Применить? Ещё раз», второе — тост «Зал из файла: …», z25v-1 снова на месте с MAC, зона 12 zł/ч',
    /Применить\? Ещё раз/.test(armed) && /Зал из файла: .*ПК: новых 2/.test(t1) && back === `${cell[0]}|${cell[1]}|AA:BB:CC:00:25:0A|z25v Зона|12.00`,
    JSON.stringify([armed, t1, back]));
  await boss.evaluate(() => openHallImport()); await z.sleep(400);
  await boss.fill('#hiText', '{кривой'); await boss.click('#hiLook'); await z.sleep(500);
  const e1 = (await z.text(boss, '#hiErr')) || '';
  await boss.fill('#hiText', '{"format":"x","version":1}'); await boss.click('#hiLook'); await z.idle(boss);
  const e2 = (await z.text(boss, '#hiErr')) || '';
  z.check('не JSON — «Это не JSON …»; чужой файл — слова сервера «Это не файл зала 14:48 …»',
    /Это не JSON/.test(e1) && /Это не файл зала 14:48/.test(e2), JSON.stringify([e1, e2]));
  await boss.evaluate(() => closeDrawer());

  // админ, журнал, палитра
  await z.sleep(2500);
  const adm = await z.openAdmin('admin1');
  await z.nav(adm, 'hall');
  const ab = [await vis(adm, 'hallEditBtn'), await vis(adm, 'hallExpBtn'), await vis(adm, 'hallImpBtn')];
  z.check('у админа «Редактора зала» и кнопок файла нет', ab.join() === 'false,false,false', JSON.stringify(ab));
  const lbl = await boss.evaluate(() => [AUD_LABEL.hall_export, AUD_LABEL.hall_import, AUD_GROUPS.pc.includes('hall_export') && AUD_GROUPS.pc.includes('hall_import')]);
  const aud = z.rows("SELECT action FROM admin_actions WHERE action IN ('hall_export', 'hall_import') AND created_at > now() - interval '5 minutes'");
  z.check('журнал: «зал: выгружен файлом», «зал: загружен из файла» в «ПК и зал»; строки выгрузки и записи есть',
    JSON.stringify(lbl) === JSON.stringify(['зал: выгружен файлом', 'зал: загружен из файла', true]) && aud.includes('hall_export') && aud.includes('hall_import'),
    JSON.stringify([lbl, aud]));
  await boss.evaluate(() => { if (hallEdit) toggleHallEdit(); nav('stats'); }); await z.sleep(800);
  await boss.evaluate(() => palOpen()); await boss.fill('#palInp', 'шаблон зала'); await z.sleep(500);
  const items = await boss.evaluate(() => [...document.querySelectorAll('#palList .pi b')].map(x => x.textContent.trim()).slice(0, 3));
  await boss.keyboard.press('Enter'); await z.sleep(1800);
  const pal = await boss.evaluate(() => ({ tab: curTab, edit: hallEdit }));
  z.check(`палитра: «шаблон зала» → «${items[0] || '—'}», Enter — «Зал» в редакторе, кнопки файла на экране`,
    items[0] === 'Зал файлом' && pal.tab === 'hall' && pal.edit && await vis(boss, 'hallImpBtn'), JSON.stringify({ items, pal }));

  // телефон 360
  await z.sleep(2500);
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 740 } });
  await z.nav(ph, 'hall'); await z.sleep(600);
  await ph.evaluate(() => openHallImport()); await z.sleep(400);
  await ph.setInputFiles('#hiFile', path); await z.idle(ph);
  const fit = await ph.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth, none: !!document.getElementById('hiNone'),
    r: [...document.querySelectorAll('#drawer .uform .input, #drawer .uform .btn, #hiPrev .hiRow, #hiPrev .muted')].map(e => Math.round(e.getBoundingClientRect().right)) }));
  await z.shot(ph, 'z25_hall_phone');
  z.check('телефон 360: шторка «Зал из файла» с просмотром (тот же файл — «Менять нечего») — в экране, вбок не прокручивается',
    fit.none && fit.r.length > 3 && fit.r.every(x => x <= fit.w) && fit.sw <= fit.w, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
