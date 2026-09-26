// З8-и2 «Инкассация» (№64 «б», Д11), браузер: в шторке «Моя смена» у админа — кнопка «Инкассация». Форма:
// сумма, «В сейф» / «В банк», «по чьему указанию» — без заметки не уходит; сумма больше, чем в ящике по
// расчёту, — второе нажатие это называет. Записал — тост с новым «должно быть», в «Из чего» — «минус
// инкассации». В ленте зала строку инкассации видят и админ, и владелец. Временный сотрудник z8cw; его
// инкассацию в конце отменяет владелец — ящик стенда как был, z8cw уволен.
const z = require('./_z');
(async () => {
  await z.hireTemp('z8cw');
  const pv = async () => (await z.api('GET', '/admin/work/closeout', undefined, 'z8cw')).data;
  const page = await z.openAdmin('z8cw'); // вход в админку открывает смену
  if ((await pv()).baseline) { // ящика на стенде ещё нет — заводит его z8cw сдачей на 500 zł
    await z.api('POST', '/admin/work/stop', { counted_pln: 500 }, 'z8cw');
    await z.api('POST', '/admin/work/start', {}, 'z8cw');
  }
  const p0 = await pv();
  const events = [];
  z.atEnd(async () => { for (const id of events) await z.api('POST', `/admin/cash/events/${id}/void`, {}, 'boss'); });
  const body = () => z.text(page, '#drawerBody');
  await page.evaluate(() => loadMyShift());
  await page.evaluate(() => openMyShift());
  await z.sleep(1200);
  if (!z.check('«Моя смена»: кнопка «Инкассация» под ящиком', await page.isVisible('#ccOpen'), (await body() || '').slice(0, 200))) return z.done();
  await page.click('#ccOpen');
  await z.sleep(400);
  z.check('форма: сумма, «В сейф» / «В банк», «по чьему указанию»', await page.isVisible('#ccAmount') && await page.isVisible('#ccNote')
    && /В сейф/.test(await body()) && /В банк/.test(await body()));
  await page.fill('#ccAmount', '50');
  await page.click('#drawerBody button:has-text("В банк")');
  await z.sleep(300);
  await page.click('#ccSubmit');
  await z.sleep(200);
  await page.click('#ccSubmit');
  await z.sleep(800);
  const t0 = await z.text(page, '#toast');
  z.check('без заметки — «Напиши, по чьему указанию», ничего не записано', /по чьему указанию/.test(t0 || '')
    && Math.abs((await pv()).collections_pln - p0.collections_pln) < 0.005, t0);
  await page.fill('#ccAmount', String(Math.ceil(p0.expected_pln + 1000)));
  await page.click('#ccSubmit');
  await z.sleep(200);
  const armed = await page.evaluate(() => document.getElementById('ccSubmit').textContent.trim());
  z.check('больше, чем в ящике по расчёту, — второе нажатие это называет', /Больше, чем в ящике по расчёту/.test(armed), armed);
  await z.sleep(4300); // взвод снимается сам
  await page.fill('#ccAmount', '50');
  await page.fill('#ccNote', 'по указанию владельца');
  await z.shot(page, 'z8_collect_form');
  await page.click('#ccSubmit');
  await z.sleep(200);
  await page.click('#ccSubmit');
  await z.sleep(1800);
  const t1 = await z.text(page, '#toast');
  const p1 = await pv();
  const ev = z.sql(`SELECT id FROM cash_events WHERE created_by='${z.uid('z8cw')}' AND voided_at IS NULL ORDER BY created_at DESC LIMIT 1`);
  if (ev) events.push(ev);
  const want = Number(p0.expected_pln - 50).toFixed(2).replace('.', ',');
  z.check(`тост «✓ Инкассация 50,00 zł в банк записана · в ящике должно быть ${want} zł»`, /Инкассация 50,00 zł в банк записана/.test(t1 || '')
    && (t1 || '').includes('должно быть ' + want + ' zł') && Math.abs(p1.expected_pln - (p0.expected_pln - 50)) < 0.005, t1);
  const b1 = await body();
  z.check('в «Из чего» — «минус инкассации 50,00», форма свернулась', /минус инкассации 50,00/.test(b1 || '') && await page.isVisible('#ccOpen'), (b1 || '').slice(0, 420));
  await z.shot(page, 'z8_collect_done');

  const boss = await z.openAdmin('boss'), adm = await z.openAdmin('admin2');
  const line = async p => (await p.evaluate(() => [...document.querySelectorAll('#feedList .fi')].map(e => e.textContent.replace(/\s+/g, ' ').trim())))
    .find(l => /касса: инкассация/.test(l) && /z8cw/.test(l));
  const lb = await line(boss), la = await line(adm);
  z.check('лента зала владельца: «касса: инкассация — z8cw · 50.00 zł в банк · по указанию владельца»',
    lb && /касса: инкассация — z8cw · 50\.00 zł в банк · по указанию владельца/.test(lb), lb);
  z.check('лента зала admin2: ту же строку видит и админ', !!la, la);
  await z.done();
})().catch(z.crash);
