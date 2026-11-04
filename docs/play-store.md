# Google Play submission

What the Play Console asks for, and the answers that match what Datey actually
does. Update this file whenever a permission, SDK or server feature changes.
The Data safety form is a legal declaration, not a formality.

## Before the first upload

| Requirement | Where | Datey answer |
| --- | --- | --- |
| App must exist | Play Console → Create app | Name, default language, "App" + "Free" |
| Privacy policy URL | Store listing → Privacy policy | `https://<your-host>/privacy` — must load without signing in |
| Account deletion URL | App content → Data deletion | `https://<your-host>/account/delete` |
| Data deletion in app | App content → Data deletion | Yes: Settings → Delete account, plus `DELETE /api/v1/me` |
| Sign-in details | App content → App access | Test account credentials, or "all functionality is restricted" if the instance is private |

Play requires the deletion URL to be reachable **and** to explain what happens,
not just to exist. `/account/delete` does both. The default hosted instance is
`https://datey.vandijke.xyz`; self-hosters must substitute their own HTTPS
origin.

## Data safety answers

The Android app talks to the Datey instance's HTTP API. It has no crash
reporting, no advertising SDK, no Google/Firebase SDK, no location permission
and no device-contacts permission. Push notifications are a web feature in
Datey; the Android app does not register a push token.

**Does your app collect or share any of the required user data types?** Yes.

| Data type | Collected | Shared | Purpose | Optional? | Ephemeral? |
| --- | --- | --- | --- | --- | --- |
| Personal info → Email address | Yes, if the account has one (OIDC or admin-entered) | No | Account management, sign-in | Yes | No |
| Personal info → Name | Yes (username; people names in the address book) | No | Account management, app functionality | No | No |
| Personal info → User IDs | Yes (account id, usernames) | No | Account management | No | No |
| Personal info → Address | Only if the operator types an address into a person's notes | No | App functionality | Yes | No |
| Photos and videos → Photos | No upload from the Android app; person photos are displayed, and uploads happen on the web | No | App functionality | Yes | No |
| App activity → Other user-generated content | Yes (gift ideas, notes and dates entered in the app) | No | App functionality | Yes | No |
| App activity → Other actions | Yes (linking a profile, creating/updating gift ideas) | No | App functionality | No | No |
| Contacts | No device contacts access. The server may import Google Contacts when the operator connects Google on the web | No | App functionality (address book import) | Yes | No |
| Device or other IDs | No; the Android app registers no push token and no advertising ID | No | — | — | — |
| Location | No | No | — | — | — |

Notes that matter during review:

- **"Shared" means transferred to a third party.** Datey sends no app data to a
  third party. Notification delivery (SMTP, Gotify, ntfy, Telegram, Discord,
  Slack, Matrix, webhook, web push) is configured by the operator on the server
  and is not initiated by the Android app.
- **Nothing is sold**, so "sharing for advertising" is No everywhere.
- **Data is encrypted in transit.** The release manifest sets
  `android:usesCleartextTraffic="false"`; HTTP is allowed only in debug builds
  for self-hosted LAN development.
- **Deletion.** In-app deletion removes the account and all credentials. Shared
  address-book records are deliberately retained for other members; the
  deletion page explains this before the user confirms. This is a documented
  product decision, not an omission in the form.
- **Backups.** `android:allowBackup="false"`,
  `android:fullBackupContent="false"` and
  `res/xml/data_extraction_rules.xml` keep the session token out of cloud
  backups and device transfers.
- **Google API Limited Use.** The server's Google Contacts import is covered by
  the explicit Limited Use section in `/privacy`. The Android app itself does
  not call Google APIs.

## App access

If the instance is invite-only, choose "All or some functionality is
restricted" and provide a working test account with enough access to open the
People list, a person, and Settings → Delete account. Never put real member
data in the test account. If the instance is open for registration, provide the
public sign-up/sign-in route instead.

## Permissions

| Permission | Why it is declared | Asked at runtime? |
| --- | --- | --- |
| `android.permission.INTERNET` | The app is a client for a remote Datey instance | No (normal permission) |

There is no camera, location, contacts, notification or storage permission in
the release manifest. If a future feature adds one, update the manifest, this
table and the Data safety form together.

## Content rating

Answer the questionnaire from the app's actual content, not its category. Datey
is a birthday and gift-list app: no violence, no sexual content, no profanity,
no gambling, no controlled substances, no user-to-user chat. User-generated
notes and gift descriptions are possible, so answer the UGC question honestly
and confirm that the instance has moderation through the administrator. Expect
a low rating (Everyone / PEGI 3), but let the questionnaire decide.

## Ads and target audience

- Contains ads: No.
- Target age group: 16+. The privacy policy commits to 16, matching the GDPR
  default for consent in most member states.
- Not designed for children; do not opt into the Families programme.

## Target API level

`android/app/build.gradle.kts` sets `targetSdk = flutter.targetSdkVersion`.
With the Flutter stable used for this release that resolves to **API 36**.
Verify with `flutter build appbundle --release` and the generated manifest
before uploading; if an older Flutter toolchain resolves lower, pin
`targetSdk = 36` explicitly rather than shipping a lower target.

## Uploading

The release bundle is built with:

```bash
cd mobile
flutter build appbundle --release
```

Signing is wired through `android/key.properties` with a debug fallback for
local builds. Play App Signing should be enabled; the upload key in
`key.properties` is only the upload key. The version code comes from
`pubspec.yaml` (`1.56.2+1056002`); bump it for every upload or Play rejects the
bundle as a downgrade.

Personal developer accounts created after November 2023 must run a closed test
with at least 12 testers for 14 continuous days before production access is
granted. Internal testing does not count towards it.

## Listing links

- Privacy policy: `https://<your-host>/privacy`
- Account deletion: `https://<your-host>/account/delete`
- Terms: `https://<your-host>/terms` (optional in the listing, useful in the
  app's Settings screen)
