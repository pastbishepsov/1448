// З3-и2 (№20 «а»), браузер: «Отчёты → Персонал → Люди», ячейка «К выплате» — сумма по подтверждённым часам
// и строкой ниже «+ 12 ч не подтверждены (автозакрытие)». Временный z3payw со ставкой 200 zł за смену: одна
// подтверждённая смена 8 ч и одна автозакрытая «День 08–20». Записи табеля удаляются, z3payw уволен.
const z = require('./_z');
(async () => {
  const zid = await z.hireTemp('z3payw');
  z.atEnd(() => z.sql(`DELETE FROM work_entries WHERE user_id='${zid}'`));
  z.sql(`DELETE FROM work_entries WHERE user_id='${zid}'`);
  z.sql(`UPDATE staff_profiles SET rate_type='shift', rate_amount=200 WHERE user_id='${zid}'`);
  const d1 = z.clubDay(-3), d2 = z.clubDay(-2);
  const r = await z.api('POST', '/admin/work', { nickname: 'z3payw', date: d1, start: '10:00', end: '18:00' }, 'boss');
  z.check('подтверждённая смена 10–18 → 201', r.status === 201, String(r.status));
  await z.tok('z3payw'); // вход в админку открывает запись — делаем её забытой «День 08–20»
  const e2 = z.sql(`SELECT id FROM work_entries WHERE user_id='${zid}' AND ended_at IS NULL`);
  z.sql(`UPDATE work_entries SET date='${d2}', started_at=(('${d2}'::date + time '08:00') AT TIME ZONE 'Europe/Warsaw'),
    shift_id=(SELECT id FROM shifts WHERE name='День') WHERE id='${e2}'`);
  const s = await z.api('POST', '/admin/work/start', {}, 'z3payw');
  z.check('забытая закрыта автоматически', s.status === 201 && z.sql(`SELECT auto_closed FROM work_entries WHERE id='${e2}'`) === 't', String(s.status));

  const page = await z.openAdmin('boss');
  await z.nav(page, 'stats');
  await page.click('#repTabs button[data-t="staff"], button[data-t="staff"]');
  await z.sleep(1500);
  const cell = await page.evaluate(() => {
    const tr = [...document.querySelectorAll('#repBody tr')].find(t => /z3payw/.test(t.textContent));
    return tr ? { pay: tr.lastElementChild.textContent.replace(/\s+/g, ' ').trim(), hours: tr.children[2].textContent.replace(/\s+/g, ' ').trim() } : null;
  });
  z.check('«К выплате»: 200 zł и ниже «+ 12 ч не подтверждены (автозакрытие)»', cell && /200,00 zł/.test(cell.pay)
    && /\+ 12 ч не подтверждены \(автозакрытие\)/.test(cell.pay), JSON.stringify(cell));
  z.check('«Часов факт» — только подтверждённые 8 ч', cell && /· 8 ч$/.test(cell.hours), JSON.stringify(cell));
  const row = page.locator('#repBody tr', { hasText: 'z3payw' });
  await row.scrollIntoViewIfNeeded();
  await z.shot(page, 'z3_payout');
  await z.done();
})().catch(z.crash);
