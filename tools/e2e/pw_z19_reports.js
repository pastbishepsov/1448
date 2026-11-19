// З19-и1 «Отчёты» (№40), браузер: подпись сравнения говорит, как сравниваем. «Отчёты» → «Сегодня» — «сравнение с
// <вчера> до ЧЧ:ММ» (как prev_period.until сервера), «Прошлый месяц» к позапрошлому разной длины — «· рост на
// среднедневных», «7 дней» — до того же часа (З19-и8), без поправки. «Гости»: «Пропали» — за период (подсказка плитки и
// заголовок списка). «Бухгалтерия» → «Обзор»: этот месяц — без поправки, ‹ прошлый месяц (к позапрошлому разной длины)
// — «рост на среднедневных», подпись по-прежнему кончается «клубные сутки». Данных не заводит.
// З19-и16: подпись сверяется с ответом, который нарисовала страница, а не с запросом проверки после экрана — иначе на смене
// минуты «до ЧЧ:ММ» расходились (полный прогон З19-и15: «до 12:37» на экране, «до 12:38» в ответе).
const z = require('./_z');
(async () => {
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const boss = await z.openAdmin('boss');
  await z.nav(boss, 'stats');
  const settle = () => boss.waitForFunction(() => { const b = document.getElementById('repBody'); return b && !b.querySelector('.skel'); }, null,
    { timeout: 20000 }).catch(() => {}).then(() => z.sleep(600));
  // период нажат — подпись экрана и ответ, по которому страница её нарисовала (её запрос «Денег» с этим периодом)
  const preset = async p => {
    const got = boss.waitForResponse(r => /\/admin\/reports\/money\?/.test(r.url()) && r.url().includes('preset=' + p), { timeout: 20000 })
      .then(r => r.json()).catch(() => ({}));
    await boss.evaluate(p => repPreset(p, document.querySelector(`#repPresets button[data-p="${p}"]`)), p);
    const data = await got; await settle();
    return [norm(await z.text(boss, '#repLabel')), data || {}];
  };

  const [lt, st] = await preset('today');
  const until = (st.prev_period || {}).until;
  z.check(`«Сегодня»: «… сравнение с … до ${until}» — вчера до того же часа`, !!until && lt.endsWith(' до ' + until), lt);
  const [lm, sm] = await preset('prev_month');
  const diff = (sm.period || {}).days !== (sm.prev_period || {}).days;
  z.check(`«Прошлый месяц» (${(sm.period || {}).days} дн к ${(sm.prev_period || {}).days}): ${diff ? '«· рост на среднедневных»' : 'длины равны — без поправки'}`,
    diff ? lm.endsWith(' · рост на среднедневных') : !/среднедневных/.test(lm), lm);
  const [lw, sw] = await preset('d7'), wu = (sw.prev_period || {}).until; // З19-и8: и «7 дней» — до того же часа
  z.check(`«7 дней» — «… до ${wu}» (прошлые 7 дней до того же часа их последнего дня), без «на среднедневных»`,
    !!wu && lw.endsWith(' до ' + wu) && !/среднедневных/.test(lw), lw);

  await boss.evaluate(() => repTabSet('guests', document.querySelector('#repTabs button[data-t="guests"]'))); await settle();
  const g = await boss.evaluate(() => { const k = [...document.querySelectorAll('#repBody .kpi')].find(x => /^Пропали/.test(x.textContent.trim()));
    const h = [...document.querySelectorAll('#repBody .sect')].map(x => x.textContent).find(t => /^Пропал/.test(t));
    return { title: k ? k.title : null, head: h || null }; });
  z.check('«Гости»: «Пропали» — за период: подсказка «пропали за этот период: к его концу не были …», заголовок «Пропали за период · …»',
    /^пропали за этот период: к его концу не были \d+ дн и дольше, а за \d+ дн до его начала ещё ходили/.test(g.title || '')
    && /^Пропали за период · к его концу нет визитов \d+ дн и дольше$/.test(norm(g.head)), JSON.stringify(g));
  await boss.evaluate(() => repTabSet('money', document.querySelector('#repTabs button[data-t="money"]')));

  await z.nav(boss, 'acc'); await z.sleep(2500);
  const ov = () => boss.evaluate(() => (document.getElementById('ovLabel') || {}).textContent || '');
  const o0 = norm(await ov());
  await boss.click('#ovMonth .ovArr', { timeout: 4000 }).catch(() => {}); await z.sleep(2500);
  const o1 = norm(await ov());
  const pm = (await z.api('GET', '/admin/reports/profit?preset=prev_month', undefined, 'boss')).data || {};
  const pdiff = (pm.period || {}).days !== (pm.prev_period || {}).days;
  z.check('«Обзор»: этот месяц — без поправки; ‹ прошлый месяц — ' + (pdiff ? '«рост на среднедневных»' : 'длины равны — без поправки')
    + '; подпись кончается «клубные сутки»', !/среднедневных/.test(o0) && (pdiff ? /· рост на среднедневных · клубные сутки$/.test(o1) : !/среднедневных/.test(o1))
    && o0.endsWith('клубные сутки') && o1.endsWith('клубные сутки'), JSON.stringify({ o0, o1 }));
  await z.done();
})().catch(z.crash);
