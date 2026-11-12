// З8-и1 «Модель ящика», браузер: шторка «Моя смена» считает ящик клуба. Стенд без истории ящика (у прошлых
// сдач на время проверки спрятан остаток на начало): «первая сдача после обновления — пересчитай весь ящик»,
// вбитая сумма — «начальный остаток, расхождения нет», кнопка не красная, тост — «начальный остаток ящика».
// Дальше «В ящике должно быть X zł — весь ящик клуба», «На начало … — начальный остаток, пересчитал z8w»,
// «Из чего: … · из них провёл ты: +…» — пополнение владельца в ожидаемом. Отмена пакета за наличные — тост
// «верни гостю … zł наличными». Владелец: в сводке смены у нулевой точки — «начальный остаток», в табеле —
// «касса: начальный остаток». Временный сотрудник z8w, гость Guest39: пополнения отменяются, z8w уволен.
const z = require('./_z');
(async () => {
  await z.hireTemp('z8w');
  const g39 = z.uid('Guest39');
  const deps = [];
  z.atEnd(async () => { for (const d of deps.reverse()) await z.api('POST', `/admin/deposits/${d}/void`, {}, 'boss'); });
  // стенд без истории ящика: у прошлых сдач прячется остаток на начало, в конце — возвращается
  z.hideOpenings(); // З21-и3: копия — в базе (e2e_hidden_openings), вернётся в конце
  const page = await z.openAdmin('z8w'); // вход в админку открывает смену
  const body = () => z.text(page, '#drawerBody');
  const shift = async () => { await page.evaluate(() => loadMyShift()); await page.evaluate(() => openMyShift()); await z.sleep(1200); return body(); };

  let t = await shift();
  z.check('ящика ещё нет: «первая сдача после обновления — пересчитай весь ящик», «должно быть» нет',
    /первая сдача после обновления/.test(t) && /пересчитай весь ящик/.test(t) && !/В ящике должно быть/.test(t), (t || '').slice(0, 260));
  await page.fill('#coCounted', '350');
  await page.dispatchEvent('#coCounted', 'input');
  await z.sleep(200);
  const d0 = await page.evaluate(() => ({ diff: document.getElementById('coDiff').textContent, danger: document.getElementById('coSubmit').classList.contains('danger') }));
  z.check('вбил 350 → «Начальный остаток ящика: 350,00 zł — расхождения нет», кнопка не красная',
    /Начальный остаток ящика: 350,00 zł — расхождения нет/.test(d0.diff) && !d0.danger, JSON.stringify(d0));
  await z.shot(page, 'z8_drawer_start');
  await page.click('#coSubmit');
  await z.sleep(200);
  const toast = await z.toastAfter(page, () => page.click('#drawerBody button:has-text("Ещё раз — сдать смену")'), 'Смена закрыта'); // З19-и13: тост этого действия, а не #toast через паузу
  z.check('сдача: тост «Смена закрыта · … · начальный остаток ящика 350,00 zł»', /Смена закрыта/.test(toast) && /начальный остаток ящика 350,00 zł/.test(toast), toast);

  await z.api('POST', '/admin/work/start', {}, 'z8w');
  deps.push((await z.api('POST', `/admin/users/${g39}/deposit`, { amount_pln: 30, method: 'cash' }, 'z8w')).data.deposit_id);
  deps.push((await z.api('POST', `/admin/users/${g39}/deposit`, { amount_pln: 70, method: 'cash' }, 'boss')).data.deposit_id);
  t = await shift();
  z.check('«В ящике должно быть 450,00 zł — весь ящик клуба»; на начало 350,00 — начальный остаток, пересчитал z8w',
    /В ящике должно быть\s*450,00 zł — весь ящик клуба/.test(t) && /350,00 zł — начальный остаток, пересчитал z8w/.test(t), (t || '').slice(0, 320));
  z.check('из чего: пополнения 100,00 — своё и владельца; «из них провёл ты: +30,00»', /пополнения 100,00/.test(t) && /из них провёл ты: \+30,00/.test(t),
    (t || '').slice(0, 420));
  await z.shot(page, 'z8_drawer_shift');

  const pack = z.sql('SELECT id FROM time_packages WHERE active ORDER BY price_pln, name LIMIT 1');
  const issued = await z.api('POST', `/admin/users/${g39}/packages`, { package_id: pack, method: 'cash' }, 'z8w');
  const pid = (issued.data.package || {}).id, price = (issued.data.package || {}).price_pln;
  await page.evaluate(id => openUserDrawer(id), g39);
  await z.sleep(1500);
  const vt = await z.toastAfter(page, () => page.evaluate(id => voidPack(id), pid), 'верни гостю');
  const priceTxt = Number(price || 0).toFixed(2).replace('.', ',');
  z.check(`отмена пакета за наличные: тост «… · верни гостю ${priceTxt} zł наличными»`, new RegExp('верни гостю ' + priceTxt + ' zł наличными').test(vt || ''), vt);

  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'stats');
  // З19-и7: сдача — в сводке дня своей записи табеля (ночью по графику — день начала ночной смены): листаем к нему
  const coDay = z.sql("SELECT date FROM shift_closeouts WHERE user_id = (SELECT id FROM users WHERE nickname = 'z8w') ORDER BY created_at DESC LIMIT 1");
  await boss.evaluate(d => { if (d) shiftDate = d; renderShift(); }, coDay);
  await z.sleep(1500);
  const co = await z.text(boss, '#shCloseouts');
  z.check('сводка смены владельца: у сдачи z8w — «начальный остаток»', /z8w/.test(co || '') && /начальный остаток/.test(co || ''), (co || '').slice(0, 260));
  await boss.locator('#shCloseouts tr', { hasText: 'z8w' }).last().scrollIntoViewIfNeeded();
  await z.shot(boss, 'z8_drawer_owner');
  await z.nav(boss, 'staff');
  await boss.evaluate(() => renderTimesheet());
  await z.sleep(1500);
  const ts = await z.text(boss, '#tsTable');
  z.check('табель: у записи z8w — «касса: начальный остаток»', /z8w/.test(ts || '') && /касса: начальный остаток/.test(ts || ''), (ts || '').slice(0, 200));
  await z.done();
})().catch(z.crash);
