// З0-и1 «Роль из базы» (№3), браузер: открытая админка уволенного теряет live-канал, не долбится
// за новым тикетом и выходит на экран входа с причиной; у владельца в «Персонале» нет подписи
// «Роль вступает в силу после перелогина», тост назначения не просит перелогиниться.
// Временный сотрудник z0live — в конце уволен.
const z = require('./_z');
(async () => {
  await z.hireTemp('z0live');
  const page = await z.openAdmin('z0live');
  await z.sleep(1500);
  z.check('админка z0live открыта, live-канал подключён', await page.evaluate(() => wsOpen === true));
  const tickets = [];
  page.on('request', r => { if (/\/admin\/ws-ticket$/.test(r.url())) tickets.push(r.url()); });
  const r = await z.dismiss('z0live');
  z.check('увольнение от владельца → 200', r.status === 200, r.status);
  await z.sleep(7000); // дольше шага переподключения (5 с)
  const st = await page.evaluate(() => ({ ws: wsOpen, me: !!me, login: !document.getElementById('loginView').classList.contains('hidden'),
    err: document.getElementById('loginErr').textContent }));
  z.check('live-канал закрыт, админка вышла на экран входа', !st.ws && !st.me && st.login, JSON.stringify(st));
  z.check('на экране входа — причина «Доступ к админке закрыт»', /Доступ к админке закрыт/.test(st.err), st.err);
  z.check('за 7 с после увольнения — ни одной попытки взять тикет', tickets.length === 0, String(tickets.length));
  await z.shot(page, 'z0_role_revoked');

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'staff');
  const txt = await z.text(boss, '#s-staff');
  z.check('«Персонал» владельца: подписи про перелогин нет', !/перелогин/.test(txt), (txt.match(/[^.]*перелогин[^.]*/) || [''])[0]);
  await boss.fill('#stNick', 'z0live');
  const toast = await z.toastAfter(boss, () => boss.click('button:has-text("Назначить")'), 'может входить в админку'); // З19-и13: тост этого действия, а не #toast через паузу
  z.check('тост назначения: «теперь админ» (бывшему сотруднику — «снова в команде», З15-и5), «может входить в админку», без «пусть перелогинится»',
    /z0live (теперь админ|снова в команде) — может входить в админку/.test(toast) && !/перелогин/.test(toast), toast);
  await z.shot(boss, 'z0_role_staff');
  await z.done();
})().catch(z.crash);
