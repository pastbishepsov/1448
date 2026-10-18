// З15-и1 «Пересечения шаблонов», браузер (№22; З-Р16). «Персонал» → «Новая смена» 15:00–23:00 при «День 08–20» и «Ночь
// 20–08» стенда: «Создать» не создаёт молча — в шторке «⚠ Пересекается со сменами «День» 08:00–20:00, «Ночь»
// 20:00–08:00.» с пояснением и «Всё равно создать»; нажал — смена создана, тост, шторка закрыта. Правка той же смены в
// пересечение — «Всё равно сохранить». Телефон 360 — предупреждение без прокрутки вбок. Шаблоны проверки удаляются в
// конце; денег и людей не трогает.
const z = require('./_z');
(async () => {
  const NAME = 'z15 Вечер ui';
  const drop = async () => { for (const id of z.rows(`SELECT id FROM shifts WHERE name LIKE 'z15 %'`)) await z.api('DELETE', `/admin/shifts/${id}`, undefined, 'boss'); };
  await drop(); // хвост упавшего прогона
  z.atEnd(drop);
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const made = () => z.sql(`SELECT count(*) FROM shifts WHERE name = '${NAME}'`);
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'staff');
  await boss.click('button:has-text("Новая смена")');
  await z.sleep(800);
  await boss.fill('#shName', NAME); await boss.fill('#shStart', '15:00'); await boss.fill('#shEnd', '23:00');
  await boss.click('#drawer button:has-text("Создать")');
  await z.sleep(1200);
  const w = norm(await z.text(boss, '#shOverlap')), vis = await boss.evaluate(() => !document.getElementById('shOverlap').classList.contains('hidden'));
  z.check('«Создать» 15:00–23:00 — не молча: «⚠ Пересекается со сменами «День» 08:00–20:00, «Ночь» 20:00–08:00.» с пояснением и «Всё равно создать»; смены нет',
    vis && w.startsWith('⚠ Пересекается со сменами «День» 08:00–20:00, «Ночь» 20:00–08:00. Назначенным смену при входе берём из графика')
    && w.endsWith('Всё равно создать') && made() === '0', JSON.stringify({ vis, w }));
  await z.shot(boss, 'z15_overlap');
  await boss.click('#shOverlap button:has-text("Всё равно создать")');
  await z.sleep(1500);
  const t1 = norm(await z.text(boss, '#toast')), open1 = await boss.evaluate(() => document.getElementById('drawer').classList.contains('open'));
  z.check('«Всё равно создать» — смена создана: тост «Смена сохранена», шторка закрыта', t1 === 'Смена сохранена' && !open1 && made() === '1',
    JSON.stringify({ t1, open1, n: made() }));

  const id = z.sql(`SELECT id FROM shifts WHERE name = '${NAME}'`);
  await boss.evaluate(i => editShiftDrawer(i), id);
  await z.sleep(800);
  await boss.fill('#shStart', '10:00'); await boss.fill('#shEnd', '12:00');
  await boss.click('#drawer button:has-text("Сохранить")');
  await z.sleep(1200);
  const w2 = norm(await z.text(boss, '#shOverlap'));
  const time2 = z.sql(`SELECT start_min || '-' || end_min FROM shifts WHERE id = '${id}'`);
  z.check('правка в 10:00–12:00 — тоже предупреждение, кнопка «Всё равно сохранить»; время пока прежнее',
    w2.startsWith('⚠ Пересекается со сменой «День» 08:00–20:00.') && w2.endsWith('Всё равно сохранить') && time2 === '900-1380', JSON.stringify({ w2, time2 }));
  await boss.evaluate(() => closeDrawer());

  await boss.goto('about:blank');
  await z.sleep(2500); // окна с одного адреса упираются в лимитер стенда
  const ph = await z.openAdmin('boss', { viewport: { width: 360, height: 780 } });
  await z.nav(ph, 'staff');
  await ph.evaluate(i => editShiftDrawer(i), id);
  await z.sleep(800);
  await ph.fill('#shStart', '10:00'); await ph.fill('#shEnd', '12:00');
  await ph.click('#drawer button:has-text("Сохранить")');
  await z.sleep(1200);
  const w3 = norm(await z.text(ph, '#shOverlap'));
  const fit = await ph.evaluate(() => { const b = document.getElementById('shOverlap'), q = b ? b.getBoundingClientRect() : null;
    return { sw: document.documentElement.scrollWidth, box: !!q && q.width > 0 && q.left >= 0 && q.right <= innerWidth }; });
  await z.shot(ph, 'z15_overlap_phone');
  z.check('телефон 360: то же предупреждение — «Пересекается со сменой «День» 08:00–20:00.» и «Всё равно сохранить»',
    w3.startsWith('⚠ Пересекается со сменой «День» 08:00–20:00.') && w3.endsWith('Всё равно сохранить'), w3);
  z.check('телефон 360: предупреждение о пересечении — без прокрутки вбок', fit.sw <= 360 && fit.box, JSON.stringify(fit));
  await z.done();
})().catch(z.crash);
