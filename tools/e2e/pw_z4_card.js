// З4-и1 (№6), браузер: в карточке гостя снятая посадка остаётся в истории сессий с пометкой «посадка снята»,
// а не «0 мин», и в счётчик «сессий» не входит. Guest34, одна снятая посадка на прошлой неделе; сессия удаляется.
const z = require('./_z');
(async () => {
  const g = z.uid('Guest34'), day = z.clubDay(-5);
  const sid = z.sql(`INSERT INTO sessions (user_id, computer_id, club_id, status, started_at, ended_at, minutes_total,
    minutes_used, base_rate_pln, effective_rate_pln, ended_reason)
    SELECT '${g}', id, club_id, 'cancelled', (('${day}'::date + time '12:00') AT TIME ZONE 'Europe/Warsaw'),
    (('${day}'::date + time '12:00') AT TIME ZONE 'Europe/Warsaw'), 0, 0, 0, 0, 'z4' FROM computers WHERE name='PC-08' RETURNING id`);
  z.atEnd(() => z.sql(`DELETE FROM sessions WHERE id='${sid}'`));
  const page = await z.openAdmin('boss');
  await page.evaluate(id => openUserDrawer(id), g);
  await z.sleep(1200);
  const played = await z.text(page, '#drawerBody');
  z.check('в сводке карточки: «сессий 0»', /сессий 0/.test(played), (played.match(/Наиграно[^·]*·[^\d]*\d+/) || [played.slice(0, 120)])[0]);
  await page.evaluate(() => uTabSet('sessions'));
  await z.sleep(800);
  const hist = await z.text(page, '#drawerBody .hist');
  z.check('в истории — «посадка снята», а не «0 мин»', /посадка снята/.test(hist || '') && !/0 мин/.test(hist || ''), hist);
  await z.shot(page, 'z4_card');
  await z.done();
})().catch(z.crash);
