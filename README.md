# Datey — Important Date Reminder

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/SQLite3-003B57?style=flat&logo=sqlite" alt="SQLite">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
  <img src="https://img.shields.io/badge/docker-ready-2496ED?style=flat&logo=docker" alt="Docker">
</p>

A self-hosted web application for tracking important dates and receiving automated reminders. Never miss a birthday, anniversary, or holiday again.

## Features

- **📅 Event Tracking** — Manage people and their important events (birthdays, anniversaries, weddings, holidays, meetings, custom) with dates and descriptions.
- **📥 ICS Import** — Preview and import events from `.ics` calendar files (all-day and timed events, yearly recurrences; duplicates skipped).
- **🔄 Recurring Rules** — Built-in recurring events (Mother's Day 2nd Sunday of May, Father's Day 3rd Sunday of June, New Year's Day, Easter-based holidays) plus custom nth-weekday-of-month rules (e.g. "Last Monday of August"); ordinal 5 means "Last" and nonexistent ordinals skip the year.
- **⏰ Daily Scheduler** — Checks for upcoming events daily at a configurable hour and sends reminders. If the server was offline when a reminder was due, it catches up on the next startup: dates that fell inside the reminder window during the downtime are re-checked and sent with "was N days ago" phrasing (no duplicate sends).
- **📧 Email Notifications** — SMTP-based email reminders for upcoming events.
- **🔔 Gotify Notifications** — Push notifications via Gotify self-hosted server.
- **🤖 Telegram Notifications** — Reminders sent via Telegram bot.
- **🔔 ntfy Notifications** — Push notifications via ntfy.sh (or any self-hosted ntfy server), with optional bearer-token auth and priority levels.
- **🔗 Webhook Notifications** — Generic JSON-POST webhooks for any endpoint (Slack, Discord, IFTTT, custom scripts), with optional HMAC-SHA256 request signing.
- **🔧 Multi-Notification Registry** — Configure one or multiple channels; each is tested independently.
- **✅ Test Notifications** — Send test messages per channel from the settings page.
- **📊 Dashboard** — At-a-glance view of upcoming events with days remaining.
- **📈 Stats Dashboard** — Read-only `/stats` overview with age distribution, busiest birthday months, events-per-month histogram, upcoming milestones (next 60 days), and recently-missed events (last 30 days); CSS-only bar charts.
- **🗓️ Calendar View** — Full month calendar with upcoming events, theme-aware, with `<noscript>` fallback.
- **📅 Date Variant** — User-facing dates render day-first by default (`25 Dec`); switch to month-first (`Dec 25`) from Settings → Configuration (`DATE_VARIANT`). Existing installs render day-first after upgrade — change it back in Settings if you prefer US style. Machine feeds (iCal, RSS, APIs) always stay ISO 8601.
- **👥 Groups** — Organize people into one or more groups as searchable categories. Group detail pages hold members, their own events, and a shared notes timeline; filter the people list with the group dropdown or a `group:Family` search prefix.
- **🏷️ Tags** — Lightweight free-form labels (e.g. `vip`, `summer-camp`) distinct from groups. Add/remove tags on a person's detail page with autocomplete (`GET /api/tags?q=`) and filter the people list via `?tag=vip` (comma-separated `?tag=a,b` is AND). Normalization is trim + lowercase, `^[a-z0-9_-]{1,30}$`, with deduplication. Tags render as `bg-info-subtle` chips, groups as `bg-secondary` badges.
- **🎁 Gift Ideas** — Track per-person gift ideas with title, optional notes/price/URL and status lifecycle (`idea`→`purchased`→`given`, any→`archived`). Add ideas from the person detail page and hide purchased/given/archived behind a spoiler-safe toggle (`?show_purchased=1`); deleting the person cascades to its ideas.
- **🖼️ Immich Profile Pictures** — One-click "Sync with Immich" bulk import from Settings matches every person by name (or per-person override) and imports their thumbnail locally. Uploaded photos always win over imported ones, removing a photo reverts to the live Immich proxy, and nothing is imported automatically when a person is created.
- **👤 User Management** — Multi-user support with admin and user roles.
- **📝 In-App Logging** — Ring-buffer log viewer filterable by level and source, with live log level changes.
- **🔍 People Search** — Quick search through people by name.
- **📇 vCard Import/Export** — Import one or more vCard files (with optional overwrite of existing people) and export contacts as vCard. `BDAY` supports full dates (`19951120`, `1995-11-20`) and year-less formats (`--0608`, `--06-08`, `-0608`, `0608`); year-less birthdays import as regular birthday events and are shown without an age.
- **🔁 CardDAV Sync** — Two-way, authenticated sync of people and birthdays with a CardDAV address book (Nextcloud, Baikal, iCloud, ...). Remote changes are pulled once a day (plus a manual "Sync Now" from Settings → Notifications); local edits are tracked and pushed back. `BDAY` becomes a birthday event and `NOTE` maps to notes. Conflicts resolve last-write-wins by `REV`/modification time; a server-side deletion either unlinks the local person (default, `keep`) or removes them (`delete`). Disabled until you enable it.
- **🎂 Age Display** — Ages derived from birthday events, shown on the people list, person detail, and dashboard (leap-day aware).
- **🎖️ Milestone Badges** — Birthdays at 10, 18, 20, 21, 30–100 (decade) and anniversaries at 10, 25 (Silver), 50 (Golden), 60 (Diamond) show a milestone badge in the dashboard, calendar, and person detail views, plus a "milestones this year" summary on the dashboard. Yearless events show no badge. Optional 10 000-day badges are available flag-gated.
- **🎈 Annual Notifications** — Birthdays, anniversaries, weddings and holidays fire every year on their occurrence date, even when the stored date is historical (leap-day aware). Birthday reminders are on by default per person and can be turned off with the toggle on a person's detail page. On upgrade, existing people with a parseable `BDAY` in their stored vCard data get a birthday event backfilled once.
- **🌙 Lunar Calendar Birthdays** — Per-event calendar system (`gregorian` default | `lunar`) with per-year Chinese lunisolar conversion (via [6tail/lunar-go](https://github.com/6tail/lunar-go), MIT). Lunar month/day maps to a different Gregorian date each year; leap-month-marked entries resolve only in years containing that leap month (else no occurrence), regular entries always use the regular month. UI shows converted Gregorian date primary with lunar notation secondary (e.g. `Mar 4 · lunar 1/15`); feeds (iCal/RSS/upcoming JSON) emit Gregorian only. Supported year range ~1900–2100. Age for lunar birthdays counts completed lunar years (one per anniversary of the stored lunar month/day).
- **💾 Database Backup** — Automatic nightly SQLite backups plus a weekly backup on a configurable weekday, with on-demand backup and configurable retention.
- **🎨 Theme Selector** — Light, Dark, and E-Ink themes via an accessible select control.
 - **🖥️ TRMNL E-Ink Plugin** — `trmnl/` plugin folder + public `/api/trmnl/stats` feed to display upcoming dates and stats on a TRMNL e-ink display.
 - **📡 RSS Feed** — Public, key-protected RSS 2.0 feed of upcoming events (`/rss.xml`) for feed readers and aggregators.
 - **🔌 Upcoming Events API** — Public, key-protected JSON API (`/api/upcoming`) for scripts, dashboards and automations.
  - **🏠 Home Assistant Plugin** — `homeassistant/` plugin folder with key-protected `/api/homeassistant/stats` (RESTful sensor) and `/api/homeassistant/calendar` (calendar entity, all-day `start.date`/`end.date`) feeds plus `sensor.yaml`/`calendar.yaml` snippets for Home Assistant dashboards.
- **🔔 Web Push Notifications** — browser push notifications via VAPID + service worker (`/sw.js`); enabled from Settings → Configuration, requires HTTPS (or localhost).
- **♿ Accessibility** — Skip-to-content link, keyboard-operable controls, ARIA labels, focus management on HTMX swaps.
- **🔒 Security Hardening** — CSRF double-submit tokens on all state-changing requests, login rate limiting, sanitized error messages, SRI on CDN assets. One-time password-reset links are emailed via the configured email channel (see `APP_URL`); reset requests are rate-limited per IP and responses never reveal whether a username exists.
- **📈 Umami Analytics** — Optional analytics integration via Umami.
- **🔭 OpenTelemetry Support** — Export logs to OTLP-compatible backends.
- **🐳 Docker Ready** — Multi-stage Docker build with health check and docker-compose support.
- **⚡ HTMX-Powered UI** — Fast, dynamic interface without heavy JavaScript frameworks.

## Quick Start

### Docker (Recommended)

```bash
docker compose up -d
```

Open **[http://localhost:6270](http://localhost:6270)** in your browser.

### Manual Setup

```bash
# Install dependencies
go mod download

# Build (FTS5 tag required for SQLite full-text search)
CGO_ENABLED=1 go build -tags fts5 -o datey .

# Run
./datey
```

## Configuration

Configuration is read from environment variables at startup. Every setting (except `DATA_DIR`) can additionally be overridden from the database by an administrator through the **Settings → Configuration** UI; database values take precedence over environment variables.

- **Database override** — Admin-saved values are persisted in the `app_config` SQLite table (singleton row, `NULL` = fall back to the env value).
- **Hot-reload** — Most fields (notifications, `REMINDER_DAYS`, `LOG_LEVEL`, `UMAMI_*`, `BACKUP_*`, etc.) take effect immediately after saving.
- **Restart required** — `PORT`, `SCHEDULER_HOUR`, `LOG_BUFFER_SIZE`, and `OTEL_ENDPOINT` are persisted but only applied on the next boot.
- **Data directory** — `DATA_DIR` is env-only and shown read-only in the admin UI, because the SQLite database is already open at that path before overrides load.

See `.env.example` for a template.

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `6270` | HTTP listen port *(restart required)* |
| `DATA_DIR` | `/data` | Data directory for SQLite database *(env-only, not DB-overridable)* |
| `SCHEDULER_HOUR` | `8` | Hour of day to run reminder check (**enforced**: 0–23) *(restart required)* |
| `REMINDER_DAYS` | `7` | Days ahead to look for upcoming events (**enforced**: 1–365) |
| `SCHEDULER_CATCHUP` | `true` | On startup, catch up reminders missed while the server was offline (**enforced**: boolean) |
| `DATE_VARIANT` | `european` | Date display variant for user-facing pages (**enforced**: `european` = day-first "25 Dec", `us` = month-first "Dec 25") |
| `LOG_LEVEL` | `warn` | Log level (**enforced**: must be one of `debug`, `info`, `warn`, `error`) |
| `LOG_BUFFER_SIZE` | `10000` | In-memory ring buffer size for log viewer *(restart required)* |
| `BACKUP_DIR` | — | Directory for database backups |
| `BACKUP_RETENTION_DAYS` | `30` | Days to retain daily backups before pruning |
| `WEEKLY_BACKUP_DAY` | `0` | Weekday for weekly backup (0=Sunday, 6=Saturday) |
| `WEEKLY_BACKUP_RETENTION_WEEKS` | `52` | Weeks to retain weekly backups |
| `OTEL_ENDPOINT` | — | OpenTelemetry OTLP endpoint *(restart required)* |
| `APP_URL` | — | Public base URL of this instance (e.g. `https://datey.example.com`); used to build password-reset links sent by email. When unset, reset links are derived from the incoming request instead (**enforced**: absolute URL) |
| `SMTP_HOST` | — | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port (**enforced**: 1–65535) |
| `SMTP_USER` | — | SMTP authentication username |
| `SMTP_PASS` | — | SMTP authentication password |
| `SMTP_TLS` | `true` | Enable TLS for SMTP |
| `SMTP_TIMEOUT` | `10` | SMTP timeout in seconds |
| `NOTIFICATION_EMAIL` | — | Email address to receive notifications |
| `GOTIFY_URL` | — | Gotify server URL |
| `GOTIFY_TOKEN` | — | Gotify application token |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token |
| `TELEGRAM_CHAT_ID` | — | Telegram chat ID |
| `WEBHOOK_URL` | — | Comma-separated list of URLs that receive a JSON POST per reminder (required to enable webhook) |
| `WEBHOOK_SECRET` | — | Optional secret used to sign webhook requests (`X-Datey-Signature: sha256=<hmac>`) |
| `UMAMI_URL` | — | Umami analytics endpoint |
| `UMAMI_WEBSITE_ID` | — | Umami website ID |
| `EINK_MODE` | `false` | Force high-contrast E-Ink theme for all users |
| `ICAL_FEED_ENABLED` | `false` | Enable the public iCal feed (all dates / per person) for external calendar apps |
| `ICAL_EVENT_START` | — | Feed event start time in `HH:MM` (24h); empty = all-day events (**enforced**: 0–23 / 0–59) |
| `ICAL_EVENT_DURATION` | `60` | Feed event duration in minutes, used when a start time is set (**enforced**: 1–1440) |
| `ICAL_FEED_KEY` | — | Secret key required in feed URLs (`?key=...`); auto-generated on first enable via the UI |
| `RSS_FEED_ENABLED` | `false` | Enable the public RSS feed of upcoming events for feed readers |
| `RSS_FEED_KEY` | — | Secret key required in the feed URL (`?key=...`); auto-generated on first enable via the UI |
| `UPCOMING_API_ENABLED` | `false` | Enable the public JSON API of upcoming events |
| `UPCOMING_API_KEY` | — | Secret key required in the API URL (`?key=...`); auto-generated on first enable via the UI |
| `HOMEASSISTANT_ENABLED` | `false` | Enable the Home Assistant stats feed |
| `HOMEASSISTANT_KEY` | — | Secret key required in the feed URL (`?key=...`); auto-generated on first enable via the UI |
| `PUSH_ENABLED` | `false` | Enable Web Push browser notifications (requires HTTPS or localhost) |
| `PUSH_VAPID_PUBLIC_KEY` | — | Public VAPID key; auto-generated on first enable via the UI (optional env override) |
| `PUSH_VAPID_PRIVATE_KEY` | — | Private VAPID signing key (masked in the admin UI; optional env override) |
| `CARDDAV_ENABLED` | `false` | Enable two-way CardDAV contact sync (requires a configured server URL) |
| `CARDDAV_URL` | — | CardDAV server origin or address book URL, e.g. `https://cloud.example.com` or `.../remote.php/dav/addressbooks/user/contacts/` (**enforced**: absolute URL) |
| `CARDDAV_USERNAME` | — | CardDAV username (Basic auth) |
| `CARDDAV_PASSWORD` | — | CardDAV password / app password (never logged; masked in the admin UI) |
| `CARDDAV_DELETE_POLICY` | `keep` | What happens when a contact is deleted on the server: `keep` = unlink and keep locally, `delete` = remove locally (**enforced**: `keep` or `delete`) |

> **Note:** Enforced ranges are validated both at startup and when saving from the admin UI. Invalid values cause the application to exit at startup, or re-render the admin form with an inline error in the UI.

> **Webhook receivers:** each reminder is POSTed as JSON `{"title": "...", "message": "...", "channel": "webhook", "sent_at": "<RFC3339>"}` to every configured URL. When `WEBHOOK_SECRET` is set, requests carry an `X-Datey-Signature: sha256=<hex>` header — recompute the HMAC-SHA256 of the raw body with your shared secret to verify the request came from datey. Use the "Webhook" test button in Settings → Notifications to confirm delivery before wiring up automation.

### CardDAV Sync Setup

CardDAV sync keeps your Datey people and birthday events in two-way sync with an external address book. Enable it in **Settings → Notifications** (or via the env vars above) and press **Sync Now** to run the first sync immediately; afterwards it runs automatically once a day.

- **Nextcloud / Baikal / iCloud** — use Basic auth: set `CARDDAV_USERNAME` and `CARDDAV_PASSWORD` (for Nextcloud and iCloud, generate an app password in the account settings). Set `CARDDAV_URL` to your server origin (e.g. `https://cloud.example.com`); the address book is auto-discovered via `/.well-known/carddav`.
- **Google Contacts** — currently behind the OAuth2 seam in the client (`OAuth2Transport` stub); not yet wired up in v1.
- **Deletion policy** — with the default `keep`, deleting a contact on the server only unlinks the local person (they stay in Datey); with `delete`, the local person and their events are removed too. Deleting a person in Datey removes the remote vCard when the policy is `delete`.
- **Credentials** are stored like other secrets (e.g. `SMTP_PASS`) and never rendered in the admin UI — the password field shows a masked placeholder and is left blank to keep the stored value.
- **Notes** map to the vCard `NOTE` field and a vCard `BDAY` becomes a birthday event (including year-less dates such as `--03-15`). Provider properties (`UID`, `REV`, `ETag`) are kept as sync state and never appear in notes.

## Project Structure

```
datey/
├── main.go                    # Application entry point
├── handlers/
│   └── health.go              # Health check endpoints
├── internal/
│   ├── config/
│   │   └── config.go          # Environment-based configuration + validation
│   ├── db/
│   │   └── db.go              # Database init, seeding, legacy migration
│   ├── logstore/
│   │   ├── store.go           # Ring-buffer log store
│   │   ├── handler.go         # Custom slog handler
│   │   └── otel.go            # OpenTelemetry log export
│   ├── notifier/
│   │   ├── notifier.go        # Notifier interface
│   │   ├── registry.go        # Multi-channel notification registry
│   │   ├── email.go           # Email (SMTP) notifier
│   │   ├── gotify.go          # Gotify push notifier
│   │   ├── telegram.go        # Telegram bot notifier
│   │   ├── ntfy.go            # ntfy.sh push notifier
│   │   └── webhook.go         # Generic JSON-POST webhook notifier
│   ├── repository/
│   │   ├── person.go          # Person repository
│   │   ├── event.go           # Event repository
│   │   ├── group.go           # Group repository
│   │   ├── notification_log.go # Notification log repository
│   │   └── recurring_rule.go  # Recurring rule repository
│   ├── scheduler/
│   │   └── scheduler.go       # Daily reminder scheduler
│   ├── session/
│   │   └── store.go           # Cookie session store
│   ├── vcard/
│   │   └── vcard.go           # vCard import/export
│   └── web/
│       ├── handler.go         # Handler struct, routes, dashboard, users
│       ├── auth.go            # Auth middleware, login, setup
│       ├── csrf.go            # CSRF double-submit middleware
│       ├── ratelimit.go       # Login rate limiter
│       ├── apperror.go        # Safe error rendering
│       ├── people.go          # People CRUD + legacy redirects
│       ├── events.go          # Event CRUD
│       ├── groups.go          # Group CRUD
│       ├── settings.go        # Settings, backup, test notifications
│       ├── calendar.go        # Calendar view + API
│       ├── vcard.go           # vCard import/export handlers
│       ├── templates.go       # Template loading + funcMap
│       ├── static/            # CSS assets
│       └── templates/         # Server-rendered HTML templates
├── ent/schema/                # ent ORM schema definitions
├── tests/                     # Playwright E2E tests (separate module)
├── Dockerfile                 # Multi-stage Docker build
├── docker-compose.yml         # Docker Compose configuration
├── .env.example               # Environment variable template
├── .golangci.yml              # golangci-lint configuration
└── go.mod / go.sum            # Go module dependencies
```

## Routes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Dashboard — upcoming events |
| `GET` | `/people` | List all people (search + group + `?tag=` filter; `?tag=a,b` is AND) |
| `POST` | `/people/{id}/tags` | Add tag to a person |
| `POST` | `/people/{id}/tags/remove` | Remove tag from a person |
| `GET` | `/api/tags` | Tag autocomplete (`?q=prefix`, 10 results) |
| `GET` | `/people/new` | Add a new person |
| `POST` | `/people/new` | Create a person |
| `GET` | `/people/{id}` | View person and their events |
| `POST` | `/people/{id}/delete` | Delete a person |
| `POST` | `/people/import` | Import vCard file |
| `GET` | `/people/export` | Export all people as vCard |
| `GET` | `/people/{id}/vcard` | Export single person as vCard |
| `GET` | `/people/{id}/events/new` | Add event for a person |
| `POST` | `/people/{id}/events/new` | Create an event |
| `POST` | `/events/{id}/delete` | Delete an event |
| `GET` | `/groups` | List groups |
| `POST` | `/groups/create` | Create a group |
| `POST` | `/groups/{id}/delete` | Delete a group |
| `GET` | `/calendar` | Calendar view (with `<noscript>` fallback) |
| `POST` | `/calendar/import` | Preview events from an uploaded `.ics` file |
| `POST` | `/calendar/import/confirm` | Confirm ICS import (skips duplicate events) |
| `GET` | `/api/calendar-events` | Calendar events JSON API |
| `GET` | `/settings` | Notification settings & test |
| `GET` | `/settings/config` | Configuration view |
| `POST` | `/settings/config` | Save configuration (admin only) |
| `GET` | `/settings/logs` | Log viewer |
| `GET` | `/settings/backup` | Backup view |
| `POST` | `/settings/backup` | Run a backup |
| `POST` | `/settings/test/{channel}` | Send test notification |
| `POST` | `/settings/logs/level` | Change log level |
| `POST` | `/settings/eink-toggle` | Toggle E-Ink theme |
| `GET` | `/users` | List users (admin only) |
| `POST` | `/users/create` | Create a user (admin only) |
| `POST` | `/users/{id}/delete` | Delete a user (admin only) |
| `GET` | `/login` | Login page |
| `POST` | `/login` | Login |
| `GET` | `/forgot-password` | Request a password reset link (emailed when the email channel is configured) |
| `POST` | `/forgot-password` | Submit username to request a reset link |
| `GET` | `/reset-password` | Reset-password page (`?token=...` from the emailed link) |
| `POST` | `/reset-password` | Set a new password using a reset token |
| `GET` | `/logout` | Logout |
| `GET` | `/setup` | Initial setup (first run only) |
| `POST` | `/setup` | Create admin user |
| `GET` | `/ical.ics` | Public iCal feed — all dates (`?key=...` required; 404 when disabled) |
| `GET` | `/ical/{personID}.ics` | Public iCal feed — single person's dates (`?key=...` required; 404 when disabled) |

> **iCal feed setup:** how to enable the feed, understand the secret key, subscribe from Google Calendar, and bypass an Authelia/SSO reverse proxy — see [docs/ical-feed.md](docs/ical-feed.md).
| `GET` | `/api/trmnl/stats` | Public JSON stats feed for the TRMNL e-ink plugin |
| `GET` | `/rss.xml` | Public RSS 2.0 feed of upcoming events (`?key=...` required; 404 when disabled) |
| `GET` | `/api/upcoming` | Public JSON API of upcoming events (`?key=...` required; `days` optional, max 365; 404 when disabled) |
| `GET` | `/api/homeassistant/stats` | Public JSON stats feed for the Home Assistant plugin (`?key=...` required; 404 when disabled) |
| `GET` | `/api/homeassistant/calendar` | Public HA calendar feed (`?key=...&start=YYYY-MM-DD&end=YYYY-MM-DD`, max 365 days; all-day `start.date`/`end.date`; 404 when disabled) |
| `GET` | `/sw.js` | Service worker for Web Push notifications |
| `POST` | `/push/subscribe` | Store a Web Push subscription (authenticated) |
| `POST` | `/push/unsubscribe` | Remove a Web Push subscription (authenticated) |
| `GET` | `/push/vapid-public-key` | Public VAPID key for establishing browser subscriptions (authenticated) |
| `GET` | `/health` | Health check |
| `GET` | `/health/db` | Database health check |
| `GET` | `/contacts/*` | Legacy redirects → `/people/*` (301) |

## License

MIT
