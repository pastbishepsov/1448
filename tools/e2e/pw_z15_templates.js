// З15-и4 «Шаблоны смен», браузер (№30; З-Р20). Владелец в «Персонале» правит шаблон «z15t сетка» (00:00–23:00 каждый
// день — пересекается с «Днём» и «Ночью» стенда): снимает галочку дня недели, на который завтра назначен z15t, и ставит
// порядок 0. «Сохранить» → «Всё равно сохранить» (пересечение) → не молча: «⚠ В убранные дни на «z15t сетка» уже
// назначены: … dd.mm z15t.» с пояснением и «Снять с графика и сохранить»; нажал — тост «Смена сохранена · снято с
// графика: 1», шторка закрыта, назначения нет, порядок 0. У «Удалить смену» — подпись, что имя в табеле и отчёте
// остаётся. Телефон 360 — предупреждение без прокрутки вбок. Шаблоны «z15t …» удаляются в начале и в конце; z15t —
// временный сотрудник; денег не трогает.
const z = require('./_z');
(async () => {
  const Z = 'z15t', DAYS = ['пн', 'вт', 'ср', 'чт', 'пт', 'сб', 'вс'];
  const drop = async () => { for (const id of z.rows(`SELECT id FROM shifts WHERE name LIKE 'z15t %'`)) await z.api('DELETE', `/admin/shifts/${id}`, undefined, 'boss'); };
  await drop(); // хвост упавшего прогона
  await z.hireTemp(Z);
  z.atEnd(drop);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const tmr = z.sql(`SELECT ((now() AT TIME ZONE 'Europe/Warsaw')::date + 1)::text`);
  const wd = +z.sql(`SELECT extract(isodow FROM date '${tmr}')::int - 1`);
  const ddmm = tmr.slice(8, 10) + '.' + tmr.slice(5, 7);
  const tpl = async name => {
    const c = await z.api('POST', '/admin/shifts', { name, start_min: 0, end_min: 1380, sort: 9000, force: true }, 'boss');
    const a = await z.api('POST', `/admin/shifts/${c.data.id}/assign`, { date: tmr, nickname: Z }, 'boss');
    return { id: c.data.id, ok: c.status === 201 && a.status === 201 };
  };
  const state = id => z.sql(`SELECT s.days_mask || '|' || s.sort || '|' || (SELECT count(*) FROM shift_assignments a WHERE a.shift_id = s.id AND a.date = '${tmr}') FROM shifts s WHERE s.id = '${id}'`);
  const t = await tpl('z15t сетка');
  z.check('шаблон «z15t сетка» (порядок 9000), z15t в графике на завтра', t.ok && state(t.id) === '127|9000|1', state(t.id));

  // до сужения дней — подготовить шторку: снять день, порядок 0, пройти «пересекается»
  const narrow = async (page, id) => {
    await page.evaluate(i => editShiftDrawer(i), id);
    await z.sleep(800);
    await page.uncheck(`.shDay[data-b="${wd}"]`);
    await page.fill('#shSort', '0');
    await page.click('#drawer button:has-text("Сохранить")');
    await z.sleep(1200);
    const ov = norm(await z.text(page, '#shOverlap'));
    if (ov.endsWith('Всё равно сохранить')) { await page.click('#shOverlap button:has-text("Всё равно сохранить")'); await z.sleep(1200); }
    return ov;
  };
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'staff');
  await boss.evaluate(i => editShiftDrawer(i), t.id);
  await z.sleep(800);
  const note = norm(await z.text(boss, '#drawer'));
  z.check('у «Удалить смену» — «Удаление уносит и её назначения из графика (в табеле и отчёте имя смены остаётся)»',
    note.includes('Удаление уносит и её назначения из графика (в табеле и отчёте имя смены остаётся); чтобы просто спрятать'), note.slice(-200));
  await boss.evaluate(() => closeDrawer());
  await z.sleep(500);
  const ov = await narrow(boss, t.id);
  const w = norm(await z.text(boss, '#shOverlap')), open1 = await boss.evaluate(() => document.getElementById('drawer').classList.contains('open'));
  z.check('«Всё равно сохранить» после «пересекается» — не молча: «⚠ В убранные дни на «z15t сетка» уже назначены: '
    + `${DAYS[wd]} ${ddmm} z15t.» с пояснением и «Снять с графика и сохранить»; шторка открыта, шаблон и график прежние`,
    ov.startsWith('⚠ Пересекается') && open1 && w.startsWith(`⚠ В убранные дни на «z15t сетка» уже назначены: ${DAYS[wd]} ${ddmm} z15t. В убранные дни смены в сетке не будет`)
    && w.endsWith('Снять с графика и сохранить') && state(t.id) === '127|9000|1', JSON.stringify({ ov: ov.slice(0, 40), w, open1, st: state(t.id) }));
  await z.shot(boss, 'z15_days_assigned');
  const btn = await boss.$('#shOverlap button:has-text("Снять с графика и сохранить")');
  if (btn) { await btn.click(); await z.sleep(1500); }
  const toast = norm(await z.text(boss, '#toast')), open2 = await boss.evaluate(() => document.getElementById('drawer').classList.contains('open'));
  z.check('«Снять с графика и сохранить» — тост «Смена сохранена · снято с графика: 1», шторка закрыта; день убран, порядок 0, назначения на завтра нет',
    toast === 'Смена сохранена · снято с графика: 1' && !open2 && state(t.id) === `${127 & ~(1 << wd)}|0|0`, JSON.stringify({ toast, open2, st: state(t.id) }));

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const t2 = await tpl('z15t сетка 2');
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'staff');
  await narrow(ph, t2.id);
  const w3 = norm(await z.text(ph, '#shOverlap'));
  const fit = await ph.evaluate(() => { const b = document.getElementById('shOverlap'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(ph, 'z15_days_assigned_phone');
  z.check('телефон 360: то же предупреждение о назначениях в убранные дни — без прокрутки вбок',
    t2.ok && w3.startsWith('⚠ В убранные дни на «z15t сетка 2» уже назначены:') && w3.endsWith('Снять с графика и сохранить') && fit.sw <= 360 && fit.box,
    JSON.stringify({ w3: w3.slice(0, 80), fit }));
  await ph.evaluate(() => closeDrawer());
  await z.done();
})().catch(z.crash);
