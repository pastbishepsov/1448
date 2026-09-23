// З0-и0 «Стенд трека З», браузерная часть: _z.js открывает админку владельцем и админом (вход через
// форму, тур пропущен), переключает экраны, делает скриншоты, ходит в API с кэшем токенов; в конце
// done() сам проверяет, что ошибок JS не было. Ничего в стенде не меняет.
const z = require('./_z');
(async () => {
  const boss = await z.openAdmin('boss');
  const railOf = page => page.evaluate(() => [...document.querySelectorAll('#rail button')].filter(b => b.offsetParent).map(b => b.id));
  const rb = await railOf(boss);
  z.check('владелец: в рейке «Отчёты», «Персонал», «Экономика»', ['r-stats', 'r-staff', 'r-settings'].every(x => rb.includes(x)), rb.join(' '));
  await z.nav(boss, 'stats');
  const shown = await boss.evaluate(() => ['hall', 'stats'].map(s => !document.getElementById('s-' + s).classList.contains('hidden')));
  z.check('nav(stats): экран «Отчёты» открыт, «Зал» скрыт', !shown[0] && shown[1], JSON.stringify(shown));
  await z.shot(boss, 'z0_boss_stats');

  const adm = await z.openAdmin('admin1');
  const ra = await railOf(adm);
  z.check('админ: «Отчётов» и «Экономики» в рейке нет, «Зал» есть', !ra.includes('r-stats') && !ra.includes('r-settings') && ra.includes('r-hall'), ra.join(' '));
  await z.shot(adm, 'z0_admin_hall');

  const own = await z.api('GET', '/admin/settings', undefined, 'boss');
  const deny = await z.api('GET', '/admin/settings', undefined, 'admin1');
  z.check('api(): настройки владельцу — 200, админу — 403', own.status === 200 && deny.status === 403, `${own.status} · ${deny.status}`);
  z.check('sql(): 20 гостей трека З в базе', z.sql("SELECT count(*) FROM users WHERE nickname ~ '^Guest(3[1-9]|4[0-9]|50)$' AND role='player'") === '20');
  await z.done();
})().catch(z.crash);
