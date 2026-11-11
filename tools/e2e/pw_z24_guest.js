// З24-и3 «Расписание цен — киоск и PWA» (№53 «в»), браузер. Временные зоны «z24g Вечер» по 10 zł/ч с правилом «каждый
// день» с 10 минут назад на час — 25 zł/ч, и «z24g Утро» по 12 zł/ч с правилом только на завтрашний день недели 09:00–10:00 —
// 15 zł/ч (API). Киоск 1440×900, заставка: «z24g Вечер 25 zł/ч (с … — 10 zł)», «z24g Утро 12 zł/ч (завтра с 09:00 — 15
// zł)», зона без правил — как было; EN — «(from …)», «(tomorrow from 09:00 …)», PL — «(od …)», «(jutro od 09:00 …)»;
// граница прошла, а /clubs ещё старый — следующая цена уже «сейчас», без скобок. PWA 390×844 (гость z24pg): в карточке
// клуба — «z24g Вечер 25 zł/час (с … — 10 zł)» и «z24g Утро 12 zł/час (завтра с 09:00 — 15 zł)», EN и PL — так же;
// APP_VER = app_version манифеста. В конце зоны «z24g …» (с правилами) удаляются SQL-ом.
const z = require('./_z');
const { chromium } = require('playwright-core');
const { CHROME } = require('./_env');
(async () => {
  const ZE = 'z24g Вечер', ZM = 'z24g Утро', G = 'z24pg';
  const tidy = () => z.sql(`DELETE FROM zones WHERE name LIKE 'z24g %'`);
  tidy();
  z.atEnd(tidy);
  if (!z.uid(G)) await z.api('POST', '/auth/register', { nickname: G, password: z.GUEST_PASS });
  const hm = m => `${String(Math.floor(m / 60) % 24).padStart(2, '0')}:${String(m % 60).padStart(2, '0')}`;
  const day = x => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const when = (iso, today, tmrw) => { const d = new Date(iso), dd = Math.round((day(d) - day(new Date())) / 864e5);
    return (dd <= 0 ? today : dd === 1 ? tmrw : '?') + ' ' + hm(d.getHours() * 60 + d.getMinutes()); };

  const now = +z.sql("SELECT (extract(hour FROM now() AT TIME ZONE 'Europe/Warsaw') * 60 + extract(minute FROM now() AT TIME ZONE 'Europe/Warsaw'))::int");
  const dow = +z.sql("SELECT extract(isodow FROM now() AT TIME ZONE 'Europe/Warsaw')::int"); // пн = 1 … вс = 7
  const tmrwBit = 1 << (dow % 7); // завтра: пн = бит 0 … вс = бит 6
  const ze = ((await z.api('POST', '/admin/zones', { name: ZE, rate_pln: 10 }, 'boss')).data.zone || {}).id;
  const zm = ((await z.api('POST', '/admin/zones', { name: ZM, rate_pln: 12 }, 'boss')).data.zone || {}).id;
  const r1 = await z.api('POST', `/admin/zones/${ze}/rules`, { days_mask: 127, start_min: (now - 10 + 1440) % 1440, end_min: (now + 50) % 1440 || 1440, rate_pln: 25 }, 'boss');
  const r2 = await z.api('POST', `/admin/zones/${zm}/rules`, { days_mask: tmrwBit, start_min: 540, end_min: 600, rate_pln: 15 }, 'boss');
  const pub = ((await z.api('GET', '/clubs')).data.clubs || [])[0] || {};
  const pe = (pub.zones || []).find(x => x.name === ZE) || {}, pm = (pub.zones || []).find(x => x.name === ZM) || {};
  z.check(`«${ZE}» сейчас 25 → 10, «${ZM}» 12 → 15 завтра в 09:00 (API, /clubs)`, r1.status === 201 && r2.status === 201
    && pe.rate_pln === 25 && pe.next_rate_pln === 10 && pm.rate_pln === 12 && pm.next_rate_pln === 15 && when(pm.next_at || 0, 'с', 'завтра с') === 'завтра с 09:00',
    JSON.stringify([r1.status, r2.status, pe, pm]));
  const E = { ru: [`${ZE} 25 zł/ч (${when(pe.next_at, 'с', 'завтра с')} — 10 zł)`, `${ZM} 12 zł/ч (завтра с 09:00 — 15 zł)`],
    en: [`${ZE} 25 zł/h (${when(pe.next_at, 'from', 'tomorrow from')} — 10 zł)`, `${ZM} 12 zł/h (tomorrow from 09:00 — 15 zł)`],
    pl: [`${ZE} 25 zł/h (${when(pe.next_at, 'od', 'jutro od')} — 10 zł)`, `${ZM} 12 zł/h (jutro od 09:00 — 15 zł)`] };

  const br = await chromium.launch({ executablePath: CHROME, args: ['--no-sandbox'] });
  z.atEnd(() => br.close().catch(() => {}));
  // ── киоск: заставка
  const pcId = z.sql("SELECT id FROM computers WHERE name = 'PC-01'");
  const k = await (await br.newContext({ viewport: { width: 1440, height: 900 } })).newPage();
  k.on('pageerror', e => z.jsErrors.push('киоск: ' + e.message));
  await k.goto(z.BASE + '/shell?pc=' + pcId, { waitUntil: 'load' }); await z.sleep(1500);
  const has = await k.evaluate(() => typeof zoneNowNext === 'function');
  await k.evaluate(() => attractShow()); await z.sleep(2000);
  const rates = lang => k.evaluate(l => {
    STR = Object.assign({}, SH_RU, SH_LANGS[l]);
    const el = document.getElementById('attractRates');
    el.innerHTML = attractRatesText();
    return el.innerText.split(' · ').map(s => s.replace(/\s+/g, ' ').trim());
  }, lang);
  const kr = { ru: await rates('ru'), en: await rates('en'), pl: await rates('pl') };
  const kOk = l => E[l].every(w => kr[l].includes(w));
  z.check(`киоск, заставка RU: «${E.ru[0]}», «${E.ru[1]}»; зона без правил — без скобок`,
    has && kOk('ru') && kr.ru.some(t => /^Standard \d+(,\d+)? zł\/ч$/.test(t)), JSON.stringify(kr.ru));
  z.check(`киоск EN и PL: «${E.en[0]}» · «${E.pl[1]}»`, has && kOk('en') && kOk('pl'), JSON.stringify([kr.en, kr.pl]));
  const stale = await k.evaluate(n => { STR = Object.assign({}, SH_RU, SH_LANGS.ru);
    const zz = (clubs[0].zones || []).find(x => x.name === n); if (!zz) return null;
    const was = zz.next_at; zz.next_at = new Date(Date.now() - 60000).toISOString();
    const t = attractRatesText(); zz.next_at = was;
    const d = document.createElement('div'); d.innerHTML = t; return d.innerText.split(' · ').find(s => s.startsWith(n)); }, ZE);
  z.check(`киоск: граница прошла, а /clubs старый — «${ZE} 10 zł/ч», без скобок`, has && stale === `${ZE} 10 zł/ч`, stale);
  await k.evaluate(() => { STR = Object.assign({}, SH_RU, SH_LANGS.ru); document.getElementById('attractRates').innerHTML = attractRatesText(); });
  await k.screenshot({ path: `${z.OUT}/z_z24_kiosk.png` });

  // ── PWA: карточка клуба
  const a = await (await br.newContext({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true })).newPage();
  a.on('pageerror', e => z.jsErrors.push('PWA: ' + e.message));
  await a.goto(z.BASE + '/app', { waitUntil: 'load' });
  await a.fill('#nick', G); await a.fill('#pass', z.GUEST_PASS); await a.click('#authBtn');
  await a.waitForFunction(() => { const m = document.getElementById('clubMeta'); return m && m.textContent.trim() !== ''; }, null, { timeout: 15000 }).catch(() => {});
  const meta = lang => a.evaluate(l => {
    if (typeof LANGS === 'undefined' || typeof renderClub !== 'function') return [];
    STR = Object.assign({}, STR_RU, LANGS[l]); renderClub();
    return ((document.getElementById('clubMeta') || {}).textContent || '').split(' · ').map(s => s.replace(/\s+/g, ' ').trim());
  }, lang);
  const PE = { ru: [`${ZE} 25 zł/час (${when(pe.next_at, 'с', 'завтра с')} — 10 zł)`, `${ZM} 12 zł/час (завтра с 09:00 — 15 zł)`],
    en: [`${ZE} 25 zł/hour (${when(pe.next_at, 'from', 'tomorrow from')} — 10 zł)`, `${ZM} 12 zł/hour (tomorrow from 09:00 — 15 zł)`],
    pl: [`${ZE} 25 zł/godz. (${when(pe.next_at, 'od', 'jutro od')} — 10 zł)`, `${ZM} 12 zł/godz. (jutro od 09:00 — 15 zł)`] };
  const pr = { ru: await meta('ru'), en: await meta('en'), pl: await meta('pl') };
  const pOk = l => PE[l].every(w => pr[l].includes(w));
  z.check(`PWA RU: «${PE.ru[0]}», «${PE.ru[1]}»; зона без правил — без скобок`, pOk('ru') && pr.ru.some(t => /^Standard \d+(,\d+)? zł\/час$/.test(t)), JSON.stringify(pr.ru));
  z.check(`PWA EN и PL: «${PE.en[1]}» · «${PE.pl[0]}»`, pOk('en') && pOk('pl'), JSON.stringify([pr.en, pr.pl]));
  const ver = await a.evaluate(async () => ({ app: typeof APP_VER === 'undefined' ? null : APP_VER,
    mf: (await (await fetch('/app.webmanifest?v=' + Date.now(), { cache: 'no-store' })).json()).app_version }));
  await a.evaluate(() => { STR = Object.assign({}, STR_RU, LANGS.ru); renderClub(); });
  await a.screenshot({ path: `${z.OUT}/z_z24_pwa.png` });
  const fit = await a.evaluate(() => ({ sw: document.documentElement.scrollWidth, w: innerWidth }));
  z.check(`PWA: APP_VER ${ver.app} = app_version манифеста; телефон 390 — вбок не прокручивается`, !!ver.app && ver.app === ver.mf && fit.sw <= fit.w,
    JSON.stringify([ver, fit]));
  z.check('ошибок JS в киоске и PWA нет', z.jsErrors.length === 0, z.jsErrors.join(' | '));
  await z.done();
})().catch(z.crash);
