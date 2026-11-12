// Real-time in-app notifications (spec: docs/notifications-realtime-fcm.md).
//
// Opens an authenticated Server-Sent Events stream and shows a notification
// for every `notification` event. Reuses the app's existing toast helper
// (the `show-toast` event handled in base.html) and, when the browser has
// granted permission and the tab is hidden, a native Notification as well.
// EventSource reconnects on its own; when it is unavailable this script is a
// no-op so the rest of the UI keeps working.
(function () {
  'use strict';

  if (typeof window.EventSource !== 'function') {
    return;
  }

  var ICON = '/static/icon-192.png';

  function showToast(message, type) {
    if (!message) return;
    document.body.dispatchEvent(new CustomEvent('show-toast', {
      detail: { message: message, type: type || 'info' }
    }));
  }

  function showBrowserNotification(title, message, url) {
    if (typeof Notification !== 'function' || Notification.permission !== 'granted') {
      return;
    }
    // Only when the page is not visible — otherwise the toast is enough.
    if (!document.hidden) {
      return;
    }
    try {
      var n = new Notification(title || 'Datey', {
        body: message || '',
        icon: ICON,
        tag: 'datey-realtime'
      });
      n.onclick = function () {
        window.focus();
        if (url) {
          window.location.href = url;
        }
        n.close();
      };
    } catch (e) {
      // Some browsers (e.g. Android Chrome) require a service worker for
      // notifications; the SSE toast still fired, so failing here is fine.
    }
  }

  function openStream() {
    var source = new EventSource('/events/stream');

    source.addEventListener('notification', function (evt) {
      var data;
      try {
        data = JSON.parse(evt.data || '{}');
      } catch (e) {
        return;
      }
      var title = data.title || 'Datey';
      var message = data.message || title;
      showToast(message, 'info');
      showBrowserNotification(title, message, data.url);
    });

    // `ready` is just a handshake; no UI change is needed.
    source.addEventListener('error', function () {
      // EventSource retries automatically; do not spam the user.
    });
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', openStream);
  } else {
    openStream();
  }
})();
