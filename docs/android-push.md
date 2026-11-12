# Android push notifications (FCM)

The Flutter app in `mobile/` can receive the "new item" notifications that the
server sends through Firebase Cloud Messaging. This document is the operator
guide: creating the Firebase project, dropping the config file, configuring the
server and testing delivery.

The web app uses Web Push (VAPID) instead; both channels are independent. The
architecture is described in [`notifications-realtime-fcm.md`](notifications-realtime-fcm.md).

## 1. Create the Firebase project

1. Open the [Firebase console](https://console.firebase.google.com/) and create
   (or reuse) a project. The project ID is needed on the server as
   `FCM_PROJECT_ID`.
2. Add an **Android app** to the project with the package name
   `xyz.vandijke.datey`. This must match `applicationId` in
   `mobile/android/app/build.gradle.kts`; a mismatch makes FCM tokens fail with
   `INVALID_ARGUMENT` or the app never receives messages.
3. Download `google-services.json`.
4. In **Project settings → Service accounts**, click *Generate new private key*
   and download the JSON file. This is the service-account credential the Go
   server uses; keep it secret.

## 2. Add `google-services.json` to the app

Place the file at:

```
mobile/android/app/google-services.json
```

The file is git-ignored. The Gradle build applies the
`com.google.gms.google-services` plugin **only when this file exists**
(`mobile/android/app/build.gradle.kts`), so a clone or CI job without Firebase
still builds `flutter build apk --debug` / `--release`. Without the file the app
runs normally, `PushService.start()` fails its guarded Firebase initialisation
and simply disables push; nothing crashes.

Rebuild after adding or replacing the file (the values are compiled into the APK
as Android resources, so a hot reload is not enough).

## 3. Configure the server

Set these environment variables (or the equivalent admin settings, which write
the same config) and restart the server:

| Env | Required | Meaning |
|---|---|---|
| `FCM_ENABLED` | yes | Turns FCM delivery on. Requires the two values below; startup fails validation otherwise. |
| `FCM_PROJECT_ID` | yes | Firebase project ID, e.g. `datey-prod`. |
| `FCM_SERVICE_ACCOUNT_JSON` | yes | Raw contents of the service-account JSON (one line or a JSON string). Treat as a secret; it is never rendered in clear. |
| `NOTIFY_NEW_ITEMS` | no | Instance-wide default for new-item alerts. **Defaults to on** when unset. |

Per-user delivery additionally depends on the user's notification settings:

- **Android app** (`fcm` channel) must be checked — it starts unchecked because
  no device existed yet.
- **New item alerts** (`newitems` master switch) must not be unchecked.

The server stores device tokens in `push_subscription` with
`endpoint = "fcm:<token>"`, so one user can have several devices and a token is
only ever registered once.

## 4. Endpoints the app calls

Both use the app's normal authenticated HTTP client (session cookie + CSRF, the
same client as every other call):

```
POST   /api/v1/devices   {"token":"<fcm-token>","platform":"android"}   -> 201
DELETE /api/v1/devices   {"token":"<fcm-token>"}                        -> 200
```

Registration happens on start-up after login and on every token refresh;
unregistration happens on logout (before the session cookie is cleared).

## 5. Test delivery

1. Build and install the app with `google-services.json` in place
   (`flutter run` or `flutter build apk --debug`).
2. Sign in and accept the Android 13+ notification permission prompt.
3. Confirm the device registration: the server log shows the app hitting
   `POST /api/v1/devices`, and the account's notification settings show
   "Android app" available.
4. Enable **Android app** in *Settings → Notifications* if it is off.
5. From a **second account**, create a person or an event. The first device
   should show a notification within a few seconds.
6. Check all three app states:
   - foreground → in-app SnackBar with an *Open* action,
   - background → system notification; tapping it opens `/people/{id}`,
   - terminated → tapping the notification cold-starts the app and lands on the
     person detail screen.
7. To test without a second account, send a test message from the Firebase
   console (*Messaging → Send test message*) to the device token, using a
   notification title/body and the data key `url` = `/people/1`.

## 6. Known limitations

- **Android only.** iOS/APNs is out of scope; the iOS build is unaffected but
  receives no notifications.
- **Token pruning is reactive.** The server deletes a token when FCM answers
  `UNREGISTERED` / `INVALID_ARGUMENT`. A token for an uninstalled app can
  therefore linger until the next failed send (logged, never surfaced).
- **No per-item granularity.** New-item alerts are a single global switch;
  event reminders keep their existing per-event schedule.
- **Notification appearance is fixed** (channel `datey_new_items`, default
  importance, launcher icon). Custom sounds or per-kind channels would need
  additional payload fields and native channel creation.
- **Server must be reachable over HTTPS** for the device registration call; a
  self-hosted HTTP server only works in debug builds (see the debug manifest).
- The foreground banner is a SnackBar and is not shown while the app is in the
  background — Android displays the FCM notification block there instead.
