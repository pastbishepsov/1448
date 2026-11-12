// З19-и12 «Вкладка рисует только свой последний ответ», браузер (из «Найдено по дороге» З19-и11). Медленный ответ —
// page.route держит запрос страницы SLOW мс. «Бухгалтерия»: медленные «Расходы», сразу «Касса» — на экране книга, а не
// «Расходы»; медленная «Касса» — пока считается, на экране заглушка, а не «Расходы», сразу «Расходы» — «Расходы»; период
// «Кассы»: медленные «7 дней», сразу «Сегодня» — книга за сегодня; шторка расхода живёт поверх вкладок: открыта на
// «Расходах», выбрана «Касса», расход записан — на экране «Касса». «Отчёты»: медленные «Деньги», сразу «Гости» — «Гости»
// без undefined; медленные «30 дней», сразу «Вчера» — подпись за вчера. На HEAD поздний ответ рисуется поверх.
// Гостей нет: расход 1 zł «Банк» с заметкой «z19l проверка …» в конце сторнируется (хвост упавшего прогона — в начале).
const z = require('./_z');
(async () => {
  const SLOW = 2500, MARK = 'z19l проверка', TAG = MARK + ' ' + Date.now().toString(36).slice(-5);
  const storno = async () => {
    for (const id of z.sql(`SELECT string_agg(id::text, ' ') FROM expenses WHERE note LIKE '${MARK}%' AND storno_at IS NULL`).split(' ').filter(Boolean))
      await z.api('POST', `/admin/expenses/${id}/storno`, { reason: 'уборка проверки' }, 'boss');
  };
  await storno();
  z.atEnd(storno);

  const boss = await z.openAdmin('boss');
  await z.idle(boss); // сеть страницы — под счёт с этой минуты
  let slowRe = null; // какие GET страницы держать: путь с запросом
  await boss.route(u => !!slowRe && slowRe.test(u.pathname + u.search), async route => {
    if (route.request().method() !== 'GET') return route.continue();
    await z.sleep(SLOW); await route.continue();
  });
  const tab = t => boss.click(`#accTabs button[data-t="${t}"]`);
  const acc = () => boss.evaluate(() => ({ on: (document.querySelector('#accTabs button.on') || {}).textContent, accTab,
    book: !!document.getElementById('accNow'), exp: !!document.getElementById('expTotal'), skel: !!document.querySelector('#accBody > .skel'),
    rows: document.querySelectorAll('#accBody tr.accDay').length, per: (accData || {}).period || null, preset: accPeriod.preset,
    presetOn: (document.querySelector('#accPresets button.on') || {}).textContent }));
  await z.nav(boss, 'acc');
  await tab('cash'); await z.idle(boss); // у книги и у «Расходов» данные уже есть — без правки на экране осталась бы чужая вкладка
  await tab('exp'); await z.idle(boss);

  // медленные «Расходы», сразу «Касса»
  slowRe = /^\/api\/v1\/admin\/expenses\?/;
  await tab('exp'); await z.sleep(300);
  await tab('cash'); await z.idle(boss); // idle — и поздний ответ «Расходов» пришёл
  const s1 = await acc();
  z.check('«Бухгалтерия»: «Расходы» отвечают 2,5 с, сразу нажата «Касса» — после ответа «Расходов» на экране книга, а не «Расходы»',
    s1.on === 'Касса' && s1.accTab === 'cash' && s1.book && !s1.exp, JSON.stringify(s1));

  // медленная «Касса»: пока считается — заглушка, а не «Расходы»; сразу «Расходы» — «Расходы»
  slowRe = null; await tab('exp'); await z.idle(boss);
  slowRe = /^\/api\/v1\/admin\/cash\/book\?/;
  await tab('cash'); await z.sleep(400);
  const s2 = await acc();
  z.check('«Касса» считается (ответ 2,5 с), на экране только что были «Расходы» — под выбранной «Кассой» заглушка, а не «Расходы»',
    s2.on === 'Касса' && s2.skel && !s2.exp && !s2.book, JSON.stringify(s2));
  await tab('exp'); await z.idle(boss);
  const s3 = await acc();
  z.check('…и сразу нажаты «Расходы» — после ответа книги на экране «Расходы», а не книга', s3.on === 'Расходы' && s3.accTab === 'exp' && s3.exp && !s3.book,
    JSON.stringify(s3));

  // период «Кассы»: медленные «7 дней», сразу «Сегодня»
  slowRe = null; await tab('cash'); await z.idle(boss);
  await boss.click('#accPresets button:has-text("Сегодня")'); await z.idle(boss);
  slowRe = /^\/api\/v1\/admin\/cash\/book\?preset=d7/;
  await boss.click('#accPresets button:has-text("7 дней")'); await z.sleep(300);
  await boss.click('#accPresets button:has-text("Сегодня")'); await z.idle(boss);
  const s4 = await acc();
  z.check('период «Кассы»: «7 дней» отвечают 2,5 с, сразу «Сегодня» — после ответа за 7 дней книга за сегодня (один день, период — сегодня)',
    s4.preset === 'today' && s4.presetOn === 'Сегодня' && s4.rows === 1 && !!s4.per && s4.per.from === s4.per.to && s4.per.from === z.clubDay(0),
    JSON.stringify(s4));

  // шторка расхода поверх вкладок: открыта на «Расходах», выбрана «Касса», расход записан — на экране «Касса»
  slowRe = null; await tab('exp'); await z.idle(boss);
  await boss.click('#expNewBtn'); await z.idle(boss);
  await boss.selectOption('#efCat', { label: 'Оборудование и ремонт' });
  await boss.fill('#efAmount', '1'); await boss.fill('#efNote', TAG + ': шторка поверх вкладок');
  await tab('cash'); await z.idle(boss);
  const t5 = await z.toastAfter(boss, () => boss.click('#efSave'), 'Расход 1,00 zł внесён');
  const s5 = await acc(), saved = z.sql(`SELECT count(*) FROM expenses WHERE note = '${TAG}: шторка поверх вкладок' AND storno_at IS NULL`);
  z.check(`шторка расхода открыта на «Расходах», выбрана «Касса», «Внести расход» — тост «${t5.slice(0, 60)}…», на экране «Касса», а не «Расходы»`,
    saved === '1' && t5.includes('Расход 1,00 zł внесён') && s5.on === 'Касса' && s5.book && !s5.exp, JSON.stringify({ saved, s5 }));

  // «Отчёты»: медленные «Деньги», сразу «Гости»
  await z.nav(boss, 'stats'); await z.idle(boss);
  const rep = () => boss.evaluate(() => ({ repTab, preset: repPeriod.preset, on: (document.querySelector('#repTabs button.on') || {}).textContent,
    pOn: (document.querySelector('#repPresets button.on') || {}).textContent, label: document.getElementById('repLabel').textContent,
    body: document.getElementById('repBody').innerText.replace(/\s+/g, ' ').slice(0, 200) }));
  slowRe = /^\/api\/v1\/admin\/reports\/money\?/;
  await boss.click('#repTabs button[data-t="money"]'); await z.sleep(300);
  await boss.click('#repTabs button[data-t="guests"]'); await z.idle(boss);
  const r1 = await rep();
  z.check('«Отчёты»: «Деньги» отвечают 2,5 с, сразу «Гости» — после ответа «Денег» на экране «Гости» с цифрами, без undefined',
    r1.repTab === 'guests' && r1.on === 'Гости' && /Уникальных гостей \d/.test(r1.body) && !/undefined/.test(r1.body), JSON.stringify(r1));

  // период «Отчётов»: медленные «30 дней», сразу «Вчера»
  slowRe = null; await boss.click('#repTabs button[data-t="money"]'); await z.idle(boss);
  slowRe = /^\/api\/v1\/admin\/reports\/money\?preset=d30/;
  await boss.click('#repPresets button[data-p="d30"]'); await z.sleep(300);
  await boss.click('#repPresets button[data-p="yesterday"]'); await z.idle(boss);
  const r2 = await rep();
  z.check('период «Отчётов»: «30 дней» отвечают 2,5 с, сразу «Вчера» — после ответа за 30 дней подпись за вчера («1 день»), а не за 30 дней',
    r2.preset === 'yesterday' && r2.pOn === 'Вчера' && / · 1 день/.test(r2.label) && !/30 дней/.test(r2.label), JSON.stringify(r2));
  slowRe = null; await boss.click('#repPresets button[data-p="d7"]'); await z.idle(boss);
  await z.done();
})().catch(z.crash);
