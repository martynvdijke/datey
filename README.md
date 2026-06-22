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
- **🔄 Recurring Rules** — Built-in recurring events (Mother's Day, Father's Day, New Year's Day) that auto-generate each year.
- **⏰ Daily Scheduler** — Checks for upcoming events daily at a configurable hour and sends reminders.
- **📧 Email Notifications** — SMTP-based email reminders for upcoming events.
- **🔔 Gotify Notifications** — Push notifications via Gotify self-hosted server.
- **🤖 Telegram Notifications** — Reminders sent via Telegram bot.
- **🔧 Multi-Notification Registry** — Configure one or multiple channels; each is tested independently.
- **✅ Test Notifications** — Send test messages per channel from the settings page.
- **🔔 One-Time Notifications** — Schedule ad-hoc reminders independent of recurring events.
- **📊 Dashboard** — At-a-glance view of upcoming events with days remaining.
- **🗓️ Calendar View** — Full month calendar with upcoming events, theme-aware, with `<noscript>` fallback.
- **👥 Groups** — Organize people into groups and filter by group.
- **👤 User Management** — Multi-user support with admin and user roles.
- **📝 In-App Logging** — Ring-buffer log viewer filterable by level and source, with live log level changes.
- **🔍 People Search** — Quick search through people by name.
- **📇 vCard Import/Export** — Import and export contacts via vCard files.
- **💾 Database Backup** — On-demand SQLite backup with configurable retention.
- **🎨 Theme Selector** — Light, Dark, and E-Ink themes via an accessible select control.
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

All configuration is done via environment variables. See `.env.example` for a template.

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `6270` | HTTP listen port |
| `DATA_DIR` | `/data` | Data directory for SQLite database |
| `SCHEDULER_HOUR` | `8` | Hour of day to run reminder check (**enforced**: 0–23) |
| `REMINDER_DAYS` | `7` | Days ahead to look for upcoming events (**enforced**: 1–365) |
| `LOG_LEVEL` | `warn` | Log level (**enforced**: must be one of `debug`, `info`, `warn`, `error`) |
| `LOG_BUFFER_SIZE` | `10000` | In-memory ring buffer size for log viewer |
| `BACKUP_DIR` | — | Directory for database backups |
| `BACKUP_RETENTION_DAYS` | `30` | Days to retain backups before pruning |
| `OTEL_ENDPOINT` | — | OpenTelemetry OTLP endpoint |
| `SMTP_HOST` | — | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port (**enforced**: 1–65535) |
| `SMTP_USER` | — | SMTP authentication username |
| `SMTP_PASS` | — | SMTP authentication password |
| `SMTP_TLS` | `true` | Enable TLS for SMTP |
| `NOTIFICATION_EMAIL` | — | Email address to receive notifications |
| `GOTIFY_URL` | — | Gotify server URL |
| `GOTIFY_TOKEN` | — | Gotify application token |
| `TELEGRAM_BOT_TOKEN` | — | Telegram bot token |
| `TELEGRAM_CHAT_ID` | — | Telegram chat ID |
| `UMAMI_URL` | — | Umami analytics endpoint |
| `UMAMI_WEBSITE_ID` | — | Umami website ID |

> **Note:** Enforced ranges are validated at startup. Invalid values cause the application to exit with an error.

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
│   │   └── telegram.go        # Telegram bot notifier
│   ├── repository/
│   │   ├── person.go          # Person repository
│   │   ├── event.go           # Event repository
│   │   ├── group.go           # Group repository
│   │   ├── notification_log.go # Notification log repository
│   │   ├── notificationdelivery.go # One-time notification delivery
│   │   ├── onetimenotification.go  # One-time notifications
│   │   └── recurring_rule.go  # Recurring rule repository
│   ├── scheduler/
│   │   ├── scheduler.go       # Daily reminder scheduler
│   │   └── one_time_scheduler.go # One-time notification scheduler
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
│       ├── notifications.go   # One-time notifications
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
| `GET` | `/api/calendar-events` | Calendar events JSON API |
| `GET` | `/notifications` | List one-time notifications |
| `GET` | `/notifications/new` | New notification form |
| `POST` | `/notifications/new` | Create a notification |
| `POST` | `/notifications/{id}/delete` | Delete a notification |
| `POST` | `/notifications/test` | Send a notification now |
| `GET` | `/api/notifications` | Notifications JSON API |
| `GET` | `/settings` | Notification settings & test |
| `GET` | `/settings/config` | Configuration view |
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
| `GET` | `/health` | Health check |
| `GET` | `/health/db` | Database health check |
| `GET` | `/contacts/*` | Legacy redirects → `/people/*` (301) |

## License

MIT
