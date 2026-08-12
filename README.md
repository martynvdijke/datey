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
- **🔄 Recurring Rules** — Built-in recurring events (Mother's Day, Father's Day, New Year's Day, Easter-based holidays) that auto-generate each year.
- **⏰ Daily Scheduler** — Checks for upcoming events daily at a configurable hour and sends reminders.
- **📧 Email Notifications** — SMTP-based email reminders for upcoming events.
- **🔔 Gotify Notifications** — Push notifications via Gotify self-hosted server.
- **🤖 Telegram Notifications** — Reminders sent via Telegram bot.
- **🔔 ntfy Notifications** — Push notifications via ntfy.sh (or any self-hosted ntfy server), with optional bearer-token auth and priority levels.
- **🔗 Webhook Notifications** — Generic JSON-POST webhooks for any endpoint (Slack, Discord, IFTTT, custom scripts), with optional HMAC-SHA256 request signing.
- **🔧 Multi-Notification Registry** — Configure one or multiple channels; each is tested independently.
- **✅ Test Notifications** — Send test messages per channel from the settings page.
- **📊 Dashboard** — At-a-glance view of upcoming events with days remaining.
- **🗓️ Calendar View** — Full month calendar with upcoming events, theme-aware, with `<noscript>` fallback.
- **👥 Groups** — Organize people into groups and filter by group.
- **👤 User Management** — Multi-user support with admin and user roles.
- **📝 In-App Logging** — Ring-buffer log viewer filterable by level and source, with live log level changes.
- **🔍 People Search** — Quick search through people by name.
- **📇 vCard Import/Export** — Import one or more vCard files (with optional overwrite of existing people) and export contacts as vCard. `BDAY` supports full dates (`19951120`, `1995-11-20`) and year-less formats (`--0608`, `--06-08`, `-0608`, `0608`); year-less birthdays import as regular birthday events and are shown without an age.
- **🎂 Age Display** — Ages derived from birthday events, shown on the people list, person detail, and dashboard (leap-day aware).
- **🎈 Annual Notifications** — Birthdays, anniversaries, weddings and holidays fire every year on their occurrence date, even when the stored date is historical (leap-day aware). Birthday reminders are on by default per person and can be turned off with the toggle on a person's detail page. On upgrade, existing people with a parseable `BDAY` in their stored vCard data get a birthday event backfilled once.
- **💾 Database Backup** — On-demand SQLite backup with configurable retention.
- **🎨 Theme Selector** — Light, Dark, and E-Ink themes via an accessible select control.
 - **🖥️ TRMNL E-Ink Plugin** — `trmnl/` plugin folder + public `/api/trmnl/stats` feed to display upcoming dates and stats on a TRMNL e-ink display.
 - **📡 RSS Feed** — Public, key-protected RSS 2.0 feed of upcoming events (`/rss.xml`) for feed readers and aggregators.
 - **🔌 Upcoming Events API** — Public, key-protected JSON API (`/api/upcoming`) for scripts, dashboards and automations.
 - **🏠 Home Assistant Plugin** — `homeassistant/` plugin folder with a key-protected `/api/homeassistant/stats` feed and a RESTful `sensor.yaml` snippet for Home Assistant dashboards.
- **🔔 Web Push Notifications** — browser push notifications via VAPID + service worker (`/sw.js`); enabled from Settings → Configuration, requires HTTPS (or localhost).
- **♿ Accessibility** — Skip-to-content link, keyboard-operable controls, ARIA labels, focus management on HTMX swaps.
- **🔒 Security Hardening** — CSRF double-submit tokens on all state-changing requests, login rate limiting, sanitized error messages, SRI on CDN assets.
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
| `LOG_LEVEL` | `warn` | Log level (**enforced**: must be one of `debug`, `info`, `warn`, `error`) |
| `LOG_BUFFER_SIZE` | `10000` | In-memory ring buffer size for log viewer *(restart required)* |
| `BACKUP_DIR` | — | Directory for database backups |
| `BACKUP_RETENTION_DAYS` | `30` | Days to retain backups before pruning |
| `OTEL_ENDPOINT` | — | OpenTelemetry OTLP endpoint *(restart required)* |
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

> **Note:** Enforced ranges are validated both at startup and when saving from the admin UI. Invalid values cause the application to exit at startup, or re-render the admin form with an inline error in the UI.

> **Webhook receivers:** each reminder is POSTed as JSON `{"title": "...", "message": "...", "channel": "webhook", "sent_at": "<RFC3339>"}` to every configured URL. When `WEBHOOK_SECRET` is set, requests carry an `X-Datey-Signature: sha256=<hex>` header — recompute the HMAC-SHA256 of the raw body with your shared secret to verify the request came from datey. Use the "Webhook" test button in Settings → Notifications to confirm delivery before wiring up automation.

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
| `GET` | `/people` | List all people (search + group filter) |
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
| `GET` | `/logout` | Logout |
| `GET` | `/setup` | Initial setup (first run only) |
| `POST` | `/setup` | Create admin user |
| `GET` | `/ical.ics` | Public iCal feed — all dates (`?key=...` required; 404 when disabled) |
| `GET` | `/ical/{personID}.ics` | Public iCal feed — single person's dates (`?key=...` required; 404 when disabled) |
| `GET` | `/api/trmnl/stats` | Public JSON stats feed for the TRMNL e-ink plugin |
| `GET` | `/rss.xml` | Public RSS 2.0 feed of upcoming events (`?key=...` required; 404 when disabled) |
| `GET` | `/api/upcoming` | Public JSON API of upcoming events (`?key=...` required; `days` optional, max 365; 404 when disabled) |
| `GET` | `/api/homeassistant/stats` | Public JSON stats feed for the Home Assistant plugin (`?key=...` required; 404 when disabled) |
| `GET` | `/sw.js` | Service worker for Web Push notifications |
| `POST` | `/push/subscribe` | Store a Web Push subscription (authenticated) |
| `POST` | `/push/unsubscribe` | Remove a Web Push subscription (authenticated) |
| `GET` | `/push/vapid-public-key` | Public VAPID key for establishing browser subscriptions (authenticated) |
| `GET` | `/health` | Health check |
| `GET` | `/health/db` | Database health check |
| `GET` | `/contacts/*` | Legacy redirects → `/people/*` (301) |

## License

MIT
