// З0-и4 «Потолок кейсов для админа» (№9 «а»), браузер: в «Экономике» владельца есть поле «Кейсов в день
// на админа», значение сохраняется в базу и переживает перезагрузку. В конце настройка — как была.
const z = require('./_z');
(async () => {
  const before = z.sql("SELECT value FROM settings WHERE key='admin_day_case_cap'");
  z.atEnd(() => before ? z.sql(`UPDATE settings SET value='${before}' WHERE key='admin_day_case_cap'`)
    : z.sql("DELETE FROM settings WHERE key='admin_day_case_cap'"));
  const page = await z.openAdmin('boss');
  await z.nav(page, 'settings');
  const v0 = await page.inputValue('#set_casecap');
  z.check('поле «Кейсов в день на админа» есть и показывает текущее значение', v0 === (before || '0'), v0);
  await page.fill('#set_casecap', '3');
  await page.click('#s-settings button:has-text("Сохранить")');
  await z.sleep(1200);
  z.check('после «Сохранить» в базе 3', z.sql("SELECT value FROM settings WHERE key='admin_day_case_cap'") === '3');
  await page.reload({ waitUntil: 'load' });
  await page.waitForSelector('#rail', { state: 'visible' });
  await z.nav(page, 'settings');
  z.check('после перезагрузки поле показывает 3', await page.inputValue('#set_casecap') === '3');
  await page.locator('#set_casecap').scrollIntoViewIfNeeded();
  await z.shot(page, 'z0_cases_settings');
  await z.done();
})().catch(z.crash);
