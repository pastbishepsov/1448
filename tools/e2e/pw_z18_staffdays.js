// З18-и4е «Даты «Персонала» по клубным суткам» (из «Найдено по дороге» З18-и4д; решение основателя 08.11), браузер.
// Своя граница суток — на час-два позже «сейчас» (клубные сутки ещё вчерашние), браузер — в поясе UTC+14 (его календарь —
// не клубный). «Персонал» → «Нанять»: «Работает с» — дата клубных суток; нанятого той же ночью «Уволить» с последним днём
// по умолчанию — тост «… уволен ДД.ММ.ГГГГ», а не «Дата увольнения раньше даты найма». Табель → «Запись»: дата смены —
// клубные сутки. График: текущая неделя — та, где клубные сутки, подсвечены они. В 23:xx своей границей сутки не
// сдвинуть — расходится только календарь браузера; в последние полторы минуты 22:xx проверка ждёт 23:00.
// Временный z18hp нанимается с экрана каждый прогон (в админку не входит) и в конце удаляется SQL вместе со строками
// журнала о нём (карточка и периоды работы — каскадом); граница суток — как была.
const z = require('./_z');
(async () => {
  const NICK = 'z18hp', TZB = 'Pacific/Kiritimati';
  let [H, LEFT] = z.sql("SELECT extract(hour FROM now() AT TIME ZONE 'Europe/Warsaw')::int || ' ' || "
    + "(3600 - extract(epoch FROM now() - date_trunc('hour', now())))::int").split(' ').map(Number);
  if (H === 22 && LEFT < 90) { await z.sleep((LEFT + 2) * 1000); H = 23; } // граница 23 вот-вот наступит — ждём её
  const RH = H <= 21 ? H + 2 : H === 22 ? 23 : null; // граница позже «сейчас» — сутки ещё вчерашние; в 23:xx — не сдвинуть
  const drop = () => { // z18hp — SQL-ом: строки журнала о нём, потом аккаунт (карточка и периоды работы — каскадом)
    const u = z.uid(NICK);
    if (u) { z.sql(`DELETE FROM admin_actions WHERE target_user_id = '${u}'`); z.sql(`DELETE FROM users WHERE id = '${u}'`); }
  };
  drop(); // хвост упавшего прогона
  z.atEnd(drop);
  if (RH !== null) {
    const rh0 = z.rows("SELECT value FROM settings WHERE key = 'report_hour'")[0];
    z.atEnd(() => { z.sql("DELETE FROM settings WHERE key = 'report_hour'");
      if (rh0 !== undefined) z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${rh0}', now())`); });
    z.sql(`INSERT INTO settings (key, value, updated_at) VALUES ('report_hour', '${RH}', now()) ON CONFLICT (key) DO UPDATE SET value = '${RH}'`);
  }
  const CD = z.clubDay(0), CAL = z.sql("SELECT (now() AT TIME ZONE 'Europe/Warsaw')::date::text");
  const ru = d => d.slice(8) + '.' + d.slice(5, 7) + '.' + d.slice(0, 4);

  const boss = await z.openAdmin('boss', { timezoneId: TZB });
  const after = (act, want) => z.toastAfter(boss, act, want); // З19-и13: тост этого действия, а не #toast через паузу
  const BD = await boss.evaluate(() => isoDate(new Date()));
  z.check(RH !== null ? `своя граница ${RH}:00, сейчас ${H}:xx — клубные сутки ${CD}, календарь клуба ${CAL}; браузер в ${TZB} — ${BD}`
    : `${H}:xx — своей границей сутки не сдвинуть (${CD} = календарь клуба); браузер в ${TZB} — ${BD}`,
    BD !== CD && (RH === null || CD !== CAL), JSON.stringify({ H, RH, CD, CAL, BD }));
  await z.nav(boss, 'staff');

  // «Нанять» → «Уволить» той же ночью
  await boss.click('#s-staff button[onclick="hireDrawer()"]'); await z.sleep(700);
  const hired = await boss.inputValue('#hr_hired');
  z.check(`«Нанять»: «Работает с» — клубные сутки ${ru(CD)}, а не календарь браузера ${ru(BD)}`, hired === ru(CD), hired);
  await boss.fill('#hr_nick', NICK); await boss.fill('#hr_pass', z.STAFF_PASS);
  await z.shot(boss, 'z18_staffdays_hire');
  const t1 = await after(() => boss.click('button[onclick="doHire()"]'), 'нанят'), u = z.uid(NICK), prof = u ? z.sql(`SELECT hired_at FROM staff_profiles WHERE user_id = '${u}'`) : '';
  z.check(`«Нанять» → тост «${NICK} нанят …»; работает с ${CD}`, t1.includes(`${NICK} нанят`) && prof === CD, JSON.stringify({ t1, prof }));
  let last = null, t2 = '';
  if (u) {
    await boss.click(`#staffTable button[onclick^="dismissDrawer('${u}'"]`); await z.sleep(1500);
    last = await boss.inputValue('#dm_date');
    t2 = await after(() => boss.click('button[onclick^="doDismiss("]'), 'уволен');
  }
  z.check(`«Уволить» той же ночью: последний день по умолчанию — ${ru(CD)}; тост «${NICK} уволен ${ru(CD)}», а не «Дата увольнения раньше даты найма»`,
    last === ru(CD) && t2.includes(`${NICK} уволен ${ru(CD)}`), JSON.stringify({ last, t2 }));

  // табель → «Запись»
  await boss.click('#s-staff button[onclick="tsEdit(\'\')"]'); await z.sleep(700);
  const ts = await boss.inputValue('#ts_date');
  z.check(`табель → «Запись»: дата смены — клубные сутки ${ru(CD)}`, ts === ru(CD), ts);
  await boss.evaluate(() => closeDrawer()); await z.sleep(300);

  // график: текущая неделя и «сегодня» — клубные сутки
  const want = await boss.evaluate(cd => { const dt = new Date(cd + 'T12:00:00');
    return DAYS_RU[(dt.getDay() + 6) % 7] + ' ' + dt.getDate() + '.' + String(dt.getMonth() + 1).padStart(2, '0'); }, CD);
  const g = await boss.evaluate(() => ({ label: (document.getElementById('schLabel') || {}).textContent || '',
    on: [...document.querySelectorAll('#schTable th')].filter(th => th.style.color).map(th => th.textContent) }));
  z.check(`график: текущая неделя — с клубными сутками, подсвечены они — «${want}»`, g.label.endsWith('· текущая') && g.on.length === 1 && g.on[0] === want,
    JSON.stringify(g));
  await boss.locator('#schTable').scrollIntoViewIfNeeded();
  await z.shot(boss, 'z18_staffdays_schedule');
  await z.done();
})().catch(z.crash);
