// Datey service worker — web push + PWA app-shell caching.
// Serves at /sw.js (origin root) so its scope covers the whole app.

// __DATEY_VERSION__ is substituted with the real version by the server when
// /sw.js is served, so every deploy gets a fresh cache namespace.
const CACHE_VERSION = '__DATEY_VERSION__';
const CACHE_NAME = 'datey-pwa-v' + CACHE_VERSION;
const OFFLINE_URL = '/static/offline.html';
const PRECACHE_URLS = [
  OFFLINE_URL,
  '/static/style.css',
  '/static/eink.css',
  '/static/manifest.json',
  '/static/icon-192.png',
  '/static/icon-512.png',
];

self.addEventListener('install', (event) => {
  event.waitUntil(
    caches.open(CACHE_NAME).then((cache) => cache.addAll(PRECACHE_URLS)).then(() => self.skipWaiting())
  );
});

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches.keys().then((keys) =>
      Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k)))
    ).then(() => self.clients.claim())
  );
});

self.addEventListener('fetch', (event) => {
  const req = event.request;
  if (req.method !== 'GET') return;
  const url = new URL(req.url);

  // Navigations are never cached (they are authenticated and user-specific);
  // on network failure we fall back to the precached offline page. Static
  // assets are safe to cache regardless of credentials.

  if (url.pathname.startsWith('/static/')) {
    event.respondWith(
      caches.match(req).then((cached) => {
        if (cached) return cached;
        return fetch(req).then((resp) => {
          if (resp && resp.ok) {
            const clone = resp.clone();
            caches.open(CACHE_NAME).then((cache) => cache.put(req, clone));
          }
          return resp;
        });
      })
    );
    return;
  }

  if (req.mode === 'navigate' || (req.headers.get('Accept') || '').includes('text/html')) {
    event.respondWith(
      fetch(req).catch(() =>
        caches.match(req).then((cached) => cached || caches.match(OFFLINE_URL)).then((fallback) => fallback || Response.error())
      )
    );
  }
});

self.addEventListener('push', (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch (_) {
    data = { message: event.data ? event.data.text() : '' };
  }

  const title = data.title || 'Datey';
  const options = {
    body: data.message || '',
    icon: '/static/icon-192.png',
    badge: '/static/icon-192.png',
    data: { url: data.url || '/' },
  };

  event.waitUntil(self.registration.showNotification(title, options));
});

self.addEventListener('notificationclick', (event) => {
  event.notification.close();
  const url = (event.notification.data && event.notification.data.url) || '/';
  event.waitUntil(
    self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((clients) => {
      for (const client of clients) {
        if ('focus' in client) {
          client.navigate(url);
          return client.focus();
        }
      }
      return self.clients.openWindow(url);
    })
  );
});
