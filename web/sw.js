/* 14:48 — минимальный service worker гостевого PWA (спринт М0, MOBILE.md).
   Стратегия: network-first без кэширования (API и app.html всегда свежие),
   офлайн — фирменный экран «нет сети» в языке Noir. Кэш оболочки — бэклог.
   Строки — v1 русский; при EN/PL вынести в словарь как STR в app.html. */

const OFFLINE_HTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
<meta name="theme-color" content="#070708">
<title>14:48 — нет сети</title>
<style>
  :root{--bg:#070708; --txt:#f2f0f2; --dim:#8d8d97; --acc:#ff2740; --acc3:#ff5563;
        --aurora:linear-gradient(120deg,#ff2740 0%,#ff5563 50%,#7a0f1c 100%);
        --glow:0 0 24px rgba(255,39,64,.45); --smooth:cubic-bezier(.4,0,.2,1); --spring:cubic-bezier(.22,1,.36,1)}
  *{box-sizing:border-box}
  body{margin:0; height:100vh; background:var(--bg); color:var(--txt);
       font-family:system-ui,-apple-system,Segoe UI,Roboto,sans-serif;
       display:flex; align-items:center; justify-content:center; flex-direction:column; gap:14px;
       text-align:center; padding:24px}
  .logo{font-weight:800; letter-spacing:2px; font-size:44px;
        background:var(--aurora); -webkit-background-clip:text; background-clip:text;
        color:transparent; -webkit-text-fill-color:transparent}
  .logo b{-webkit-text-fill-color:var(--acc3); color:var(--acc3); animation:colonPulse 1.6s var(--smooth) infinite}
  @keyframes colonPulse{0%,100%{opacity:1; text-shadow:0 0 10px var(--acc3)}50%{opacity:.35; text-shadow:none}}
  h1{margin:10px 0 0; font-size:19px}
  p{margin:0; color:var(--dim); font-size:14px; max-width:280px; line-height:1.5}
  button{margin-top:14px; border:0; border-radius:14px; padding:14px 34px; font-size:16px; font-weight:700;
         font-family:inherit; background:var(--aurora); color:#fff; cursor:pointer; box-shadow:var(--glow);
         transition:transform .2s var(--spring)}
  button:active{transform:scale(.97)}
  @media (prefers-reduced-motion:reduce){ *{animation-duration:.001ms!important} }
</style>
</head>
<body>
  <div class="logo">14<b>:</b>48</div>
  <h1>Нет сети</h1>
  <p>Проверь Wi-Fi или мобильный интернет — и попробуй ещё раз.</p>
  <button onclick="location.reload()">Повторить</button>
</body>
</html>`;

self.addEventListener('install', () => self.skipWaiting());
self.addEventListener('activate', (e) => e.waitUntil(self.clients.claim()));

self.addEventListener('fetch', (e) => {
  if (e.request.mode !== 'navigate') return; // API и статику не перехватываем
  e.respondWith(fetch(e.request).catch(() =>
    new Response(OFFLINE_HTML, {headers: {'Content-Type': 'text/html; charset=utf-8'}})
  ));
});
