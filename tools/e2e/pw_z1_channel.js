// З1-и2 «Служебные сообщения и „только владельцу“» (№59, Д4), браузер: шторка канала. Владелец видит
// служебную строку сдачи кассы с меткой «видит только владелец» и отдельно заметку сдавшего; admin2 — только
// заметку, без денег и без метки. Тост о служебном — только владельцу; в ленте зала событие канала читается
// словами, а не «staff_chat». Временный сотрудник z1cw, гость Guest32: пополнение отменяется, z1cw уволен.
const z = require('./_z');
(async () => {
  await z.hireTemp('z1cw');
  const dep = await z.api('POST', `/admin/users/${z.uid('Guest32')}/deposit`, { amount_pln: 60, method: 'cash' }, 'z1cw');
  z.atEnd(() => z.api('POST', `/admin/deposits/${dep.data.deposit_id}/void`, {}, 'boss'));
  const boss = await z.openAdmin('boss'), adm = await z.openAdmin('admin2');
  const ids = async who => new Set(((await z.api('GET', '/admin/staff-chat', undefined, who)).data.messages || []).map(m => m.id));
  const before = { boss: await ids('boss'), admin2: await ids('admin2') };
  for (const who of ['boss', 'admin2']) await z.api('POST', '/admin/staff-chat/read', {}, who); // счётчики с нуля: канал общий для всех проверок
  for (const p of [boss, adm]) await p.evaluate(() => { // тосты страницы — в список, как их увидел бы человек
    window.__toasts = []; const t = toast; toast = (msg, ...a) => { __toasts.push(String(msg)); return t(msg, ...a); };
  });
  const r = await z.api('POST', '/admin/work/stop', { counted_pln: 50, note: 'сдал смену Ане, касса под замком' }, 'z1cw');
  z.check('z1cw сдал 50 из 60 → 200', r.status === 200, String(r.status));
  await z.sleep(2000);
  const tb = await boss.evaluate(() => __toasts), ta = await adm.evaluate(() => __toasts);
  z.check('владельцу — тост о служебном сообщении', tb.some(t => /Служебное/.test(t)), JSON.stringify(tb));
  z.check('admin2 — тост только о заметке, о служебном ни слова', ta.length === 1 && !ta.some(t => /Служебное/.test(t)), JSON.stringify(ta));
  const fb = await z.text(boss, '#feedList'), fa = await z.text(adm, '#feedList');
  z.check('лента зала: событие канала словами, без «staff_chat»', !/staff_chat/.test(fb + fa) && /канал персонала/.test(fb) && /канал персонала/.test(fa),
    (fb || '').slice(0, 160));

  const drawer = (page, seen) => page.evaluate(seen => [...document.querySelectorAll('#scWrap .scmsg')]
    .filter(e => !seen.includes(e.dataset.id)).map(e => ({ cls: e.className, txt: e.textContent.replace(/\s+/g, ' ').trim() })), [...seen]);
  await boss.evaluate(() => openStaffChat()); await z.sleep(1200);
  const vb = await drawer(boss, before.boss);
  const sys = vb.find(m => /сдал смену/.test(m.txt) && /zł/.test(m.txt));
  z.check('владелец: служебная строка — «Система · видит только владелец», z1cw, 50 из 60, расхождение −10',
    sys && /\bsys\b/.test(sys.cls) && /^Система · видит только владелец/.test(sys.txt) && /z1cw/.test(sys.txt) && /−10\.00 zł/.test(sys.txt), JSON.stringify(vb));
  z.check('владелец: заметка z1cw — отдельным сообщением, без денег', vb.some(m => /^z1cw · админ/.test(m.txt) && /Передал смену: сдал смену Ане/.test(m.txt) && !/zł/.test(m.txt)),
    JSON.stringify(vb));
  z.check('шторка открыта — счётчик владельца обнулён', await boss.evaluate(() => staffUnread === 0 && document.getElementById('staffBadge').classList.contains('hidden')));
  await boss.evaluate(() => { const w = document.getElementById('scWrap'); if (w) w.scrollTop = w.scrollHeight; });
  await z.shot(boss, 'z1_channel_owner');

  const badge = await adm.evaluate(() => document.getElementById('staffBadge').textContent);
  z.check('бейдж канала у admin2 до открытия — 1 (только заметка)', badge === '1', badge);
  await adm.evaluate(() => openStaffChat()); await z.sleep(1200);
  const va = await drawer(adm, before.admin2);
  z.check('admin2: видит заметку z1cw', va.some(m => /^z1cw · админ/.test(m.txt) && /Передал смену: сдал смену Ане/.test(m.txt)), JSON.stringify(va));
  z.check('admin2: ни денег сдачи, ни служебных строк, ни метки «видит только владелец»',
    !va.some(m => /zł|\bsys\b|видит только владелец/.test(m.txt + ' ' + m.cls)), JSON.stringify(va));
  const hint = await z.text(adm, '#drawerBody');
  z.check('подпись admin2 — без пояснения про строки владельца', !/только владел/.test(hint), (hint || '').slice(-160));
  await z.shot(adm, 'z1_channel_admin');
  await z.done();
})().catch(z.crash);
