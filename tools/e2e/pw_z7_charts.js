// З7-и4 (№44), браузер: столбики «Отчётов» — с осью и числами. «Деньги → Выручка по дням», «Загрузка → Часы игры по
// дням» и «по часам суток»: ось с круглыми делениями от 0, подписи под столбиками (последний день — всегда), число у
// самого высокого столбика и у последнего — не у каждого, и оно совпадает с отчётом сервера; при наведении и с клавиатуры
// — подсказка: число крупно, под ним день и из чего сумма; «Числа таблицей» — все значения без наведения. Проверка
// ничего не пишет — стенд не меняется.
const z = require('./_z');
const num = s => parseFloat(String(s).replace(/[^\d,.-]/g, '').replace(',', '.'));
(async () => {
  const money = (await z.api('GET', '/admin/reports/money?preset=d7', undefined, 'boss')).data.days || [];
  const load = (await z.api('GET', '/admin/reports/load?preset=d7', undefined, 'boss')).data;
  const max = Math.max(0, ...money.map(x => x.value)), iMax = money.findIndex(x => x.value === max);
  const hMax = Math.max(0, ...(load.hours || []).map(x => x.hours));
  z.check('на стенде есть выручка и игра за неделю (иначе графикам нечего показать)', max > 0 && hMax > 0, `${max} zł, пик ${hMax} ч`);

  const page = await z.openAdmin('boss');
  await z.nav(page, 'stats'); // «Деньги» за 7 дней
  const charts = () => page.evaluate(() => [...document.querySelectorAll('#repBody .chart')].map(c => ({
    ticks: [...c.querySelectorAll('.yt span')].map(s => s.textContent.trim()),
    tips: [...c.querySelectorAll('.bars .bar')].map(b => b.dataset.tip || ''),
    labels: [...c.querySelectorAll('.vl')].map(s => s.textContent.trim()),
    xs: [...c.querySelectorAll('.xax span')].map(s => s.textContent.trim()).filter(Boolean),
    rows: c.querySelectorAll('details.chtable tbody tr').length })));
  const tip = () => page.evaluate(() => { const t = document.getElementById('chtip');
    return t && !t.classList.contains('hidden') ? { v: (t.querySelector('b') || {}).textContent, text: t.textContent } : null; });

  const [m] = await charts();
  z.check('«Деньги»: у «Выручки по дням» есть ось — от 0, делений не меньше трёх, по возрастанию',
    m && m.ticks.length >= 3 && num(m.ticks[0]) === 0 && m.ticks.every((t, i) => !i || num(t) > num(m.ticks[i - 1])), JSON.stringify(m && m.ticks));
  z.check(`числа — выборочно: у самого высокого столбика (${max} zł — как в отчёте сервера) и у последнего дня, не у каждого`,
    m && m.labels.length >= 1 && m.labels.length <= 2 && m.labels.some(l => Math.abs(num(l) - max) < 1), JSON.stringify(m && m.labels));
  const lastDay = m && m.tips.length ? m.tips[m.tips.length - 1].split(' · ')[0] : '';
  z.check(`подписи дней под столбиками, последняя — последний день периода (${lastDay || '—'})`,
    m && m.xs.length >= 2 && m.xs[m.xs.length - 1] === lastDay, JSON.stringify(m && m.xs));
  z.check('в подсказке каждого дня — дата, сумма и из чего она: пополнения · товары · пакеты',
    m && m.tips.length === money.length && m.tips.every(t => /zł/.test(t) && /пополнения .* · товары .* · пакеты /.test(t)), JSON.stringify(m && m.tips.slice(-2)));
  z.check('«Числа таблицей»: строка на каждый день', m && m.rows === money.length, `${m ? m.rows : '—'} строк на ${money.length} дней`);
  if (m) {
    await page.hover(`#repBody .chart .bars .bar:nth-child(${iMax + 1})`);
    await z.sleep(250);
    const t = await tip();
    z.check('наведение на самый высокий столбик: подсказка — число крупно и из чего сумма', t && Math.abs(num(t.v) - max) < 0.01
      && /пополнения .* · товары .* · пакеты /.test(t.text), JSON.stringify(t));
    await page.locator('#repBody .chart').first().scrollIntoViewIfNeeded();
    await z.shot(page, 'z7_charts_money');
    await page.focus('#repBody .chart .bars');
    await page.keyboard.press('ArrowLeft');
    await z.sleep(250);
    const k = await tip();
    const prevDay = (m.tips[m.tips.length - 2] || '').split(' · ')[0];
    z.check(`с клавиатуры: фокус на графике, ← — подсказка предыдущего дня (${prevDay})`, k && k.text.includes(prevDay), JSON.stringify(k));
  } else z.check('наведение и клавиатура: подсказка у столбика', false, 'графика с осью нет');

  await page.click('#repTabs button[data-t="load"]');
  await z.sleep(1500);
  const [days, hours] = await charts();
  z.check('«Загрузка»: два графика с осью от 0 — часы по дням и по часам суток', days && hours && [days, hours].every(c => c.ticks.length >= 3
    && num(c.ticks[0]) === 0), JSON.stringify([days && days.ticks, hours && hours.ticks]));
  z.check('«по часам суток»: 24 столбика, подписи 0:00, 3:00 … 21:00', hours && hours.tips.length === 24
    && JSON.stringify(hours.xs) === JSON.stringify(['0:00', '3:00', '6:00', '9:00', '12:00', '15:00', '18:00', '21:00']), JSON.stringify(hours && hours.xs));
  z.check(`число у пикового часа (${hMax} ч — как в отчёте), в подсказках — часы и сессии`, hours && hours.labels.some(l => Math.abs(num(l) - hMax) < 0.05)
    && hours.tips.every(t => /ч · \d+ сесси/.test(t)), JSON.stringify(hours && { labels: hours.labels, tip: hours.tips[0] }));
  if (days) await page.locator('#repBody .chart').first().scrollIntoViewIfNeeded();
  await z.shot(page, 'z7_charts_load');
  await z.done();
})().catch(z.crash);
