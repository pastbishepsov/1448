// З22-и1 «Движок алертов» (№59, З-Р28), браузер. «Экономика» → карточка «Алерты»: выбор «Алерты в канал персонала» —
// «Присылать» / «Молчать» (как alerts_on сервера), кнопка «Прислать проверочный»: первый раз — «ушёл», сообщение в
// канале персонала (служебное, только владельцу), второй — «сегодня уже уходил»; при «Молчать» (сохранено) —
// «выключены». Уборка: проверочные алерты (журнал и сообщения канала) — в начале и в конце, alerts_on — как был.
const z = require('./_z');
const TEXT = 'Проверка алертов: так алерт выглядит в канале персонала. Этот видит только владелец.';
const wipe = () => { // журнала нет (база до миграции 097) — чистить в нём нечего
  if (z.sql("SELECT to_regclass('alerts_sent') IS NOT NULL") === 't') z.sql("DELETE FROM alerts_sent WHERE kind = 'test'");
  z.sql(`DELETE FROM staff_messages WHERE kind = 'system' AND text = '${TEXT}'`);
};
const was = z.sql("SELECT value FROM settings WHERE key = 'alerts_on'");
(async () => {
  wipe();
  z.atEnd(() => { wipe(); was ? z.sql(`UPDATE settings SET value = '${was}' WHERE key = 'alerts_on'`) : z.sql("DELETE FROM settings WHERE key = 'alerts_on'"); });
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'settings');
  await boss.waitForSelector('#set_alerts', { timeout: 15000 }).catch(() => {});
  const card = await boss.evaluate(() => {
    const sel = document.getElementById('set_alerts');
    if (!sel) return null;
    const c = sel.closest('.setcard');
    return { h: c.querySelector('h4').textContent, opts: [...sel.options].map(o => [o.value, o.textContent]), v: sel.value,
      btn: [...c.querySelectorAll('button')].map(b => b.textContent.trim()) };
  });
  z.check('«Экономика» → «Алерты»: «Присылать» / «Молчать», сейчас — «Присылать», кнопка «Прислать проверочный»',
    !!card && card.h === 'Алерты' && JSON.stringify(card.opts) === JSON.stringify([['1', 'Присылать'], ['0', 'Молчать']]) && card.v === '1'
    && card.btn.includes('Прислать проверочный'), JSON.stringify(card));
  if (!card) { // карточки нет (админка до З22-и1) — FAIL по каждой проверке, а не обрыв
    for (const n of ['проверочный: ушёл, второй — «уже уходил»', '«Молчать» — не уходит']) z.check(n + ': карточки «Алерты» нет', false);
    return z.done();
  }
  const toastAfter = async () => {
    await boss.evaluate(() => { const t = document.getElementById('toast'); if (t) t.textContent = ''; });
    await boss.click('.setcard:has(#set_alerts) button:has-text("Прислать проверочный")');
    await boss.waitForFunction(() => { const t = document.getElementById('toast'); return t && t.textContent.trim(); }, null, { timeout: 8000 }).catch(() => {});
    return boss.evaluate(() => (document.getElementById('toast') || {}).textContent || '');
  };
  const t1 = await toastAfter(), n1 = +z.sql(`SELECT COUNT(*) FROM staff_messages WHERE kind = 'system' AND visible_to = 'owner' AND text = '${TEXT}'`);
  await z.sleep(800);
  const t2 = await toastAfter(), n2 = +z.sql(`SELECT COUNT(*) FROM staff_messages WHERE kind = 'system' AND text = '${TEXT}'`);
  z.check(`проверочный: «${t1.trim()}», второй — «${t2.trim()}»; в канале одно сообщение, только владельцу`,
    /Проверочный алерт ушёл/.test(t1) && /уже уходил/.test(t2) && n1 === 1 && n2 === 1, JSON.stringify({ t1, t2, n1, n2 }));
  // «Молчать» и «Сохранить» — проверочный не уходит
  wipe();
  await boss.selectOption('#set_alerts', '0');
  await boss.evaluate(() => saveSettings());
  await z.sleep(1500);
  const saved = z.sql("SELECT value FROM settings WHERE key = 'alerts_on'");
  const t3 = await toastAfter(), n3 = +z.sql(`SELECT COUNT(*) FROM staff_messages WHERE kind = 'system' AND text = '${TEXT}'`);
  z.check(`«Молчать» сохранено (alerts_on ${saved}) — «${t3.trim()}», сообщения нет`,
    saved === '0' && /Алерты выключены/.test(t3) && n3 === 0, JSON.stringify({ saved, t3, n3 }));
  await z.shot(boss, 'z22_alerts');
  await z.done();
})().catch(z.crash);
