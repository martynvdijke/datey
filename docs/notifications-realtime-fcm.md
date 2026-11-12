# Real-time, Web Push and Android (FCM) notifications for new items

Status: implementation spec. Do not change scope without updating this file.

## Goal

When any user creates a **new item** (a Person or an Event), every *other* user is notified:

1. **Real-time in-app** — SSE event → toast in the web UI (browser tab open).
2. **Web notification** — existing Web Push (VAPID) delivery, browser notification.
3. **Android notification** — Firebase Cloud Messaging to the Flutter app.

Admin config gains a Google FCM section. Users gain per-user notification settings for the
new channels plus one general "new item alerts" toggle.

## Hard constraints

- Go 1.2x, ent ORM, chi router, html/template + embedded static FS.
- **Do not add new ent schemas** (avoids codegen churn). Reuse existing tables.
- Never break existing reminder delivery paths.
- Notify everyone **except the actor** (the user who created the item).
- Every delivery failure must be logged, never returned to the user (fire-and-forget
  goroutine with `context.WithoutCancel` or a short detached timeout).

## Storage decisions (already made — do not redesign)

- **FCM device tokens** live in `push_subscription` with `endpoint = "fcm:" + token`,
  `p256dh = "fcm"`, `auth = "fcm"`, `user_id` = owner. Multiple devices per user are fine
  because `endpoint` is unique. Detected by prefix `fcm:`.
- **Per-user channel toggles** live in `user_notification_channel` with the two new
  `channel_type` values:
  - `"fcm"` — Android app notifications (no target text; enabled flag only).
  - `"newitems"` — general "notify me when someone adds a new item" master switch.
  Both store `target = ""`. Missing row for `newitems` means **enabled** (opt-out default).
  Missing row for `fcm` means disabled (nothing registered yet).

## Config (internal/config/config.go)

Add fields, env parsing and validation in the existing style:

| Go field | Env | Notes |
|---|---|---|
| `FCMEnabled bool` | `FCM_ENABLED` | |
| `FCMProjectID string` | `FCM_PROJECT_ID` | Firebase project id |
| `FCMServiceAccountJSON string` | `FCM_SERVICE_ACCOUNT_JSON` | raw service-account JSON (secret) |
| `NotifyNewItems bool` | `NOTIFY_NEW_ITEMS` | instance-wide default, **true** when unset |

Validation: when `FCMEnabled`, `FCMProjectID` and `FCMServiceAccountJSON` are required
(mirror the existing `PushEnabled` VAPID validation block and its error style).

The service account JSON must never be logged or rendered in clear. Admin UI shows it as a
secret field, masked when already set (same treatment as `PUSH_VAPID_PRIVATE_KEY`).

## Notifier (internal/notifier/fcm.go)

`FCMNotifier` implementing the existing `Notifier` interface:

- `Name() "fcm"`, `IsConfigured()` = `FCMEnabled && FCMProjectID != "" && FCMServiceAccountJSON != ""`.
- Auth: parse the service-account JSON (`golang.org/x/oauth2/google` is already a direct
  dependency; `google.CredentialsFromJSON` with scope
  `https://www.googleapis.com/auth/firebase.messaging`), cache the token source.
- Send: `POST https://fcm.googleapis.com/v1/projects/{projectID}/messages:send`
  body `{"message":{"token":tok,"notification":{"title":..,"body":..},"data":{"url":..,"kind":..},"android":{"priority":"high","notification":{"channel_id":"datey_new_items"}}}}`.
- Prune the token on `UNREGISTERED` / `INVALID_ARGUMENT` responses (delete by endpoint).
- Register it in `registry.go` and wire it in `main.go` next to the webpush notifier.
- Add a `SendToUsers`-style targeted API used by the dispatcher below (webpush also needs a
  per-user send: add `SendToUser(ctx, userID, title, message)` to `WebPushNotifier`, using the
  existing `PushSubscriptionRepository.ListByUser`, keeping the 404/410 pruning logic).

## Dispatcher (internal/notifier/newitems.go or internal/notify/newitems.go)

`NotifyNewItem(ctx, actorUserID int, kind, title, message, url string)`:

1. If instance `NotifyNewItems` is false → return.
2. Load users who are eligible (active users, excluding `actorUserID`): general `newitems`
   toggle not explicitly disabled.
3. Publish to the realtime broker (`internal/web/realtime.go`).
4. For each eligible user: if their `webpush` channel is enabled → `WebPushNotifier.SendToUser`;
   if `fcm` channel enabled → FCM send to their tokens.
5. All sends in a goroutine, errors logged via `slog`.

## Real-time broker + SSE (internal/web/realtime.go)

- `Broker` with `mu sync.RWMutex`, `subs map[int][]chan Notification`, buffered channels (16),
  `Subscribe(userID) (ch, cancel)`, `Publish(userIDs []int, n Notification)` non-blocking
  (drop on full buffer), `Notification{Title, Message, URL, Kind string, At time.Time}`.
- `GET /events/stream` (authenticated, same middleware as other pages): SSE headers
  `text/event-stream`, `no-cache`, `X-Accel-Buffering: no`; send `event: ready` immediately,
  `event: notification` + JSON data per message, `: ping` comment every 25s, exit on
  `r.Context().Done()`. Disable any write timeout on this route if the router wraps one.
- Broker instance on `Handler` (new field, initialised in `NewHandler`) so tests can inject.
- Client: `static/realtime.js` — `EventSource('/events/stream')`, auto-reconnect (EventSource
  does it), on `notification` show the app toast (reuse existing toast helper if one exists,
  otherwise a small fixed-position element) and, when `Notification.permission === 'granted'`
  and the page is hidden, `new Notification(title, {body, icon})`. Included from
  `templates/base.html` for signed-in users only.

## Hook points (publish after successful create)

- `internal/web/people.go` `createPerson` — after groups are added, before the redirect:
  kind `person`, title = person display name, message = localized "New person added: %s",
  url `/people/{id}`.
- `internal/web/events.go` `createEvent` — after reminder days are set, before redirect:
  kind `event`, title = person name + event type, url `/people/{personID}`.
- `internal/web/api_v1_write.go` — the same model's create path used by the mobile app
  (find the POST create handlers there and mirror the call).
- Actor user id: `UserFromContext(r.Context())`; nil-safe (skip actor filtering if absent).

## Device registration API

- `POST /api/v1/devices` body `{"token":"...","platform":"android"}` → upsert
  `fcm:<token>` for the current user; `201`.
- `DELETE /api/v1/devices` body `{"token":"..."}` → delete `fcm:<token>`; `200`.
- Both authenticated via the existing API auth mechanism used by `api_v1_write.go`
  (accept the session cookie and the existing bearer/API-token path — reuse the helper that
  file already uses).

## User settings (internal/web/settings_notifications.go + template)

- `notificationChannelOrder` += `"fcm"`, `"newitems"`.
- Template: render `fcm` as "Android app" (checkbox only, helper text "Install the Datey app
  and allow notifications"), `newitems` as "New item alerts" master switch with helper text.
  No target input for these two; keep target inputs for the existing channels untouched.
- Save handler: always upsert `fcm`/`newitems` (even with empty target) so an unchecked box
  persists; `ValidateTarget` must accept empty target for these two channel types.
- General settings: also expose instance-wide FCM status read-only (configured / not
  configured) so users understand why the app toggle may do nothing.

## Admin settings (internal/web/settings_config.go + templates/settings_config.html)

Add group `fcmGroup` immediately after `pushGroup`, in the existing group-list literal and in
the same shape as `pushGroup`:

- `FCM_ENABLED` (checkbox)
- `FCM_PROJECT_ID` (text)
- `FCM_SERVICE_ACCOUNT_JSON` (secret textarea)
- `NOTIFY_NEW_ITEMS` (checkbox, general notification setting, default on)
- Read-only hint: FCM requires the Flutter app to be built with the same Firebase project;
  point at `docs/android-push.md`.

## Mobile (Flutter, mobile/)

- Add `firebase_core` + `firebase_messaging` to `pubspec.yaml` with current stable versions.
- `mobile/lib/services/push_service.dart`: init Firebase **only when** the config file exists
  (`android/app/google-services.json`); request permission, get token, register via
  `POST /api/v1/devices`, refresh on `onTokenRefresh`, unregister on logout, and register
  again on login on top of the existing auth providers.
- Foreground: show an in-app SnackBar/banner via a global `navigatorKey` (add one in `app.dart`).
  Background/terminated: rely on the notification payload; handle `onMessageOpenedApp` and
  `getInitialMessage` by navigating with the existing `router.dart` to the payload `url`.
- Android: document the `google-services.json` placement and the
  `com.google.gms.google-services` Gradle plugin requirement in `docs/android-push.md`;
  keep the build working **without** the plugin when the file is absent (conditional apply in
  `mobile/android/app/build.gradle.kts` / `settings.gradle.kts` is acceptable; if that is not
  reliably possible, add the plugin and document that `google-services.json` is now required).
- iOS: out of scope beyond not breaking the build.

## i18n

Add every new user-visible string to `internal/i18n/locales/en.json` and `de.json`, keys
namespaced `notifications.*`, with a sensible German translation. No hardcoded English in
templates that already use `t`.

## Tests (minimum)

- `internal/notifier/fcm_test.go`: httptest server asserting bearer token + payload shape,
  token pruning on `UNREGISTERED`, `IsConfigured` matrix.
- `internal/web/realtime_test.go`: broker fan-out, drop-on-full, unsubscribe.
- Extend the existing settings-notifications test (if present) to assert the two new cards.
- `go build ./... && go vet ./...` clean; targeted `go test` green.

## Out of scope

- Notification history UI, per-item granularity, email digests, iOS APNs, per-group targeting
  for new-item alerts (group scope stays reminder-only).
