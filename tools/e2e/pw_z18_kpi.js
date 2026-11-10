// З18-и4и «Плитка KPI на телефоне» (из «Найдено по дороге» З18-и4з; решение основателя 09.11 — перед З18-и5), браузер.
// На узком экране плитка KPI, которой не хватает ширины, переносится — подпись по словам, число и чип роста целиком — и
// не вылезает за экран; чип роста печатает процент по-русски, с запятой («+232,2%», а не «+232.2%»). Раньше на телефоне
// 360 плитка «Маржа товаров» с длинным чипом уходила правым краем на 370 px; переносились только «Сейчас» и итоги
// «Обзора» и итоги «Возвратов» — их правила теперь общие (проверяют pw_z12_profit и pw_z11_returns). Владелец,
// компьютер: чип — dlt(232.2) «+232,2%», dlt(-11.1) «-11,1%», dlt(1232.5) «+1 232,5%»; во всех пяти вкладках «Отчётов»
// в чипах нет точки; плитка «Маржа товаров» с длинным числом и чипом (собрана той же repKpi, что и отчёт) — в одну
// строку. Телефон 360 (то же окно, узкое): та же плитка переносится и остаётся в экране, число и чип — целиком; плитки
// «Зала», всех вкладок «Отчётов» со «Сменой» и «Бухгалтерии» («Обзор», «Касса», «Расходы») — в экране, прокрутки вбок
// нет. Данных не заводит: плитка-образец живёт только на странице.
const z = require('./_z');
(async () => {
  const TABS = ['money', 'guests', 'load', 'staff', 'coins'], ACC = ['ov', 'cash', 'exp'];
  const norm = s => String(s == null ? '' : s).replace(/\s+/g, ' ').trim();
  const settle = (p, box) => p.waitForFunction(b => { const e = document.getElementById(b); return !!e && !e.querySelector('.skel'); }, box, { timeout: 20000 })
    .catch(() => {}).then(() => z.sleep(700)); // отчёт или вкладка встали
  const repOpen = async (p, t) => { await p.evaluate(t => repTabSet(t, document.querySelector(`#repTabs button[data-t="${t}"]`)), t); await settle(p, 'repBody'); };
  const accOpen = async (p, t) => { await p.evaluate(t => accTabSet(t), t); await settle(p, 'accBody'); };
  // плитки на экране: сколько, какие вылезли за край (подпись → правый край), ширина документа
  const fit = (p, sel) => p.evaluate(sel => {
    const ks = [...document.querySelectorAll(sel)].filter(k => k.offsetParent !== null);
    const out = ks.filter(k => { const r = k.getBoundingClientRect(); return r.right > innerWidth + 0.5 || r.left < -0.5; })
      .map(k => k.textContent.replace(/\s+/g, ' ').trim().slice(0, 70) + ' → ' + Math.round(k.getBoundingClientRect().right));
    return { n: ks.length, out, sw: document.documentElement.scrollWidth, w: innerWidth };
  }, sel);
  // плитка-образец в «Деньгах»: длинное число маржи и длинный чип роста — как их собирает отчёт
  const probe = p => p.evaluate(() => {
    const box = document.getElementById('repMargin');
    if (!box) return null;
    const old = document.getElementById('z18kpi'); if (old) old.remove();
    box.insertAdjacentHTML('beforeend', repKpi('Маржа товаров', repMarginTxt({ margin_pln: 123456.78, margin_pct: 49.3 }), 1232.5, { was: '1 000,00 zł' }));
    const k = box.lastElementChild, b = k.querySelector('b'), c = k.querySelector('.dlt');
    k.id = 'z18kpi'; k.scrollIntoView({ block: 'center' });
    const r = k.getBoundingClientRect(), rb = b.getBoundingClientRect(), rc = c.getBoundingClientRect();
    return { right: Math.round(r.right), h: Math.round(r.height), w: innerWidth, sw: document.documentElement.scrollWidth, chip: c.textContent.replace(/\s/g, ' '),
      val: b.textContent.replace(/\s/g, ' '), vLines: b.getClientRects().length, cLines: c.getClientRects().length,
      same: rc.top < rb.bottom && rc.bottom > rb.top }; // чип в одной строке с числом
  });

  const boss = await z.openAdmin('boss', { viewport: { width: 1280, height: 900 } });
  const unit = await boss.evaluate(() => [232.2, -11.1, 1232.5, 0, 50, null].map(v => {
    const d = document.createElement('div'); d.innerHTML = dlt(v); return d.textContent.replace(/\s/g, ' '); }));
  z.check('чип роста по-русски: dlt(232.2) — «+232,2%», dlt(-11.1) — «-11,1%», dlt(1232.5) — «+1 232,5%», 0 — «0%», 50 — «+50%», пусто — «—»',
    unit.join('|') === '+232,2%|-11,1%|+1 232,5%|0%|+50%|—', JSON.stringify(unit));

  await z.nav(boss, 'stats'); await settle(boss, 'repBody');
  const chips = {};
  for (const t of TABS) {
    await repOpen(boss, t);
    chips[t] = await boss.evaluate(() => [...document.querySelectorAll('#repBody .dlt')].map(c => c.textContent.replace(/\s+/g, ' ').trim()));
  }
  const all = Object.values(chips).flat(), dotted = all.filter(c => /\d\.\d/.test(c));
  z.check(`«Отчёты», пять вкладок: в чипах роста нет точки — проценты с запятой (чипов ${all.length}, с числом — ${all.filter(c => /\d/.test(c)).length})`,
    all.some(c => /\d/.test(c)) && dotted.length === 0, JSON.stringify({ dotted, chips }).slice(0, 1500));

  await repOpen(boss, 'money');
  const wide = await probe(boss);
  await z.shot(boss, 'z18_kpi_wide');
  z.check('компьютер: плитка «Маржа товаров 123 456,78 zł · 49,3%» с чипом «+1 232,5%» — в одну строку, в экране',
    !!wide && wide.same && wide.right <= wide.w && wide.chip === '+1 232,5%' && wide.val === '123 456,78 zł · 49,3%', JSON.stringify(wide));

  await boss.setViewportSize({ width: 360, height: 780 }); await z.sleep(800);
  const ph = await probe(boss);
  await z.sleep(300);
  await z.shot(boss, 'z18_kpi_phone');
  z.check('телефон 360: та же плитка переносится (выше, чем в строку) и остаётся в экране; число и чип роста — целиком, каждый в одной строке; '
    + 'прокрутки вбок нет', !!ph && !!wide && ph.h > wide.h * 1.4 && ph.right <= 360 && ph.sw <= 360 && ph.vLines === 1 && ph.cLines === 1
    && ph.chip === '+1 232,5%' && ph.val === '123 456,78 zł · 49,3%', JSON.stringify({ ph, wide: wide && wide.h }));
  await boss.evaluate(() => { const k = document.getElementById('z18kpi'); if (k) k.remove(); });

  const bad = {}, seen = {};
  const note = (name, f) => { seen[name] = f.n; if (f.out.length || f.sw > 360) bad[name] = f; };
  await z.nav(boss, 'hall'); await z.sleep(1000);
  note('Зал', await fit(boss, '#kpis .kpi'));
  await z.nav(boss, 'stats');
  for (const t of TABS) { await repOpen(boss, t); note('Отчёты · ' + t, await fit(boss, '#repBody .kpi, #shiftKpis .kpi')); }
  await z.nav(boss, 'acc'); await settle(boss, 'accBody');
  for (const t of ACC) { await accOpen(boss, t); note('Бухгалтерия · ' + t, await fit(boss, '#accBody .kpi')); }
  z.check(`телефон 360: плитки «Зала», пяти вкладок «Отчётов» со «Сменой» и «Бухгалтерии» (${ACC.length} вкладки) — в экране, прокрутки вбок нет `
    + `(плиток: ${Object.values(seen).reduce((a, b) => a + b, 0)})`, Object.keys(bad).length === 0 && Object.values(seen).reduce((a, b) => a + b, 0) > 10,
    JSON.stringify({ bad, seen }).slice(0, 1500));
  await z.done();
})().catch(z.crash);
