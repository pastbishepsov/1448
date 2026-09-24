// З5-и1 (№11 «а»), браузер: число дней «новых» берётся из настройки владельца — чип у стойки «Новые за N дней»,
// в списке колонка «Впервые» (первое появление: первая сессия, без сессий — регистрация, Д2), подписи KPI «Новые»
// в отчёте «Гости» и «Новые гости» в сводке смены. seg_new_days = 10 на время проверки; Guest36 «зарегистрирован
// минуту назад» и без визитов. Настройка и регистрация возвращаются как были.
const z = require('./_z');
(async () => {
  const g36 = z.uid('Guest36');
  const nd0 = z.sql("SELECT value FROM settings WHERE key='seg_new_days'"), reg0 = z.sql(`SELECT registered_at FROM users WHERE id='${g36}'`);
  z.atEnd(() => { z.sql(`UPDATE users SET registered_at='${reg0}' WHERE id='${g36}'`);
    return nd0 ? z.api('PUT', '/admin/settings', { seg_new_days: +nd0 }, 'boss') : z.sql("DELETE FROM settings WHERE key='seg_new_days'"); });
  const r = await z.api('PUT', '/admin/settings', { seg_new_days: 10 }, 'boss');
  z.check('seg_new_days = 10 → 200', r.status === 200, String(r.status));
  z.sql(`UPDATE users SET registered_at = now() - interval '1 minute' WHERE id='${g36}'`);

  const page = await z.openAdmin('admin1'); // у стойки
  await page.evaluate(() => nav('users'));
  await z.sleep(1500);
  const chip = await z.text(page, '#uFilters button[data-f="new"]');
  z.check('чип у стойки: «Новые за 10 дней»', chip === 'Новые за 10 дней', chip);
  await page.click('#uFilters button[data-f="new"]');
  await z.sleep(1500);
  const list = await page.evaluate(() => {
    const tr = [...document.querySelectorAll('#usersTable tr')].find(t => /Guest36/.test(t.textContent));
    return { th: [...document.querySelectorAll('#usersTable th')].map(t => t.textContent.trim()),
             row: tr ? [...tr.cells].map(c => c.textContent.trim()) : null, sort: userSort, today: fmtDate(new Date().toISOString()) };
  });
  z.check('«Новые»: колонка «Впервые», сортировка по первому появлению, Guest36 в списке',
    list.th.some(t => t.startsWith('Впервые')) && list.sort === 'first_seen' && list.row, JSON.stringify({ th: list.th, sort: list.sort }));
  z.check('у Guest36 «Впервые» — сегодня (без визитов — дата регистрации)', list.row && list.row.some(c => c.startsWith(list.today + ' ')), JSON.stringify(list.row));
  await z.shot(page, 'z5_new_guests');

  const own = await z.openAdmin('boss');
  await z.nav(own, 'stats');
  await own.click('#repTabs button[data-t="guests"]');
  await z.sleep(1500);
  const kpi = await own.evaluate(() => { const k = [...document.querySelectorAll('#repBody .kpi')].find(e => /^Новые/.test(e.textContent.trim())); return k ? k.title : null; });
  z.check('отчёт «Гости», KPI «Новые»: «первый визит — не раньше чем за 10 дней до его конца»', /не раньше чем за 10 дней до его конца/.test(kpi || ''), kpi);
  const sh = await own.evaluate(() => { const k = [...document.querySelectorAll('#shiftKpis .kpi')].find(e => /Новые гости/.test(e.textContent)); return k ? k.title : null; });
  z.check('сводка смены, «Новые гости»: «… не раньше чем за 10 дней до её конца»', /не раньше чем за 10 дней до её конца/.test(sh || ''), sh);
  await own.locator('#repBody .kpis').nth(1).scrollIntoViewIfNeeded();
  await z.shot(own, 'z5_new_report');
  await z.done();
})().catch(z.crash);
