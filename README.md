# Datey — Important Date Reminder

<p align="center">
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go" alt="Go">
  <img src="https://img.shields.io/badge/SQLite3-003B57?style=flat&logo=sqlite" alt="SQLite">
  <img src="https://img.shields.io/badge/license-MIT-blue" alt="License">
  <img src="https://img.shields.io/badge/docker-ready-2496ED?style=flat&logo=docker" alt="Docker">
</p>

A self-hosted web application for tracking important dates and receiving automated reminders. Never miss a birthday, anniversary, or holiday again.

## Features

- **📅 Event Tracking** — Manage contacts and their important events (birthdays, anniversaries, etc.) with dates and descriptions.
- **🔄 Recurring Rules** — Built-in recurring events (Mother's Day, Father's Day, New Year's Day) that auto-generate each year.
- **⏰ Daily Scheduler** — Checks for upcoming events daily at a configurable hour and sends reminders.
- **📧 Email Notifications** — SMTP-based email reminders for upcoming events.
- **🔔 Gotify Notifications** — Push notifications via Gotify self-hosted server.
- **🤖 Telegram Notifications** — Reminders sent via Telegram bot.
- **🔧 Multi-Notification Registry** — Configure one or multiple channels; each is tested independently.
- **✅ Test Notifications** — Send test messages per channel from the settings page.
- **📊 Dashboard** — At-a-glance view of upcoming events with days remaining.
- **📝 In-App Logging** — Ring-buffer log viewer filterable by level and source, with live log level changes.
- **🔍 Contact Search** — Quick search through contacts.
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

# Build
CGO_ENABLED=1 go build -o datey .

# Run
./datey
```

## Configuration

All configuration is done via environment variables. See `.env.example` for a template.

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `6270` | HTTP listen port |
| `DATA_DIR` | `/data` | Data directory for SQLite database |
| `SCHEDULER_HOUR` | `8` | Hour of day to run reminder check (0-23) |
| `REMINDER_DAYS` | `7` | Days ahead to look for upcoming events |
| `LOG_LEVEL` | `warn` | Log level (debug, info, warn, error) |
| `LOG_BUFFER_SIZE` | `10000` | In-memory ring buffer size for log viewer |
| `OTEL_ENDPOINT` | — | OpenTelemetry OTLP endpoint |
| `SMTP_HOST` | — | SMTP server hostname |
| `SMTP_PORT` | `587` | SMTP server port |
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

## Project Structure

```
datey/
├── main.go                    # Application entry point
├── handlers/
│   └── health.go              # Health check endpoint
├── internal/
│   ├── config/
│   │   └── config.go          # Environment-based configuration
│   ├── db/
│   │   └── db.go              # Database initialization & seeding
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
│   │   ├── contact.go         # Contact repository
│   │   ├── event.go           # Event repository
│   │   ├── notification_log.go # Notification log repository
│   │   └── recurring_rule.go  # Recurring rule repository
│   ├── scheduler/
│   │   └── scheduler.go       # Daily reminder scheduler
│   └── web/
│       ├── handler.go         # Web routes & handlers
│       └── templates.go       # Template loading
├── Dockerfile                 # Multi-stage Docker build
├── docker-compose.yml         # Docker Compose configuration
├── .env.example               # Environment variable template
└── go.mod / go.sum            # Go module dependencies
```

## Routes

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/` | Dashboard — upcoming events |
| `GET` | `/contacts` | List all contacts |
| `GET` | `/contacts/new` | Add a new contact |
| `POST` | `/contacts/new` | Create a contact |
| `GET` | `/contacts/{id}` | View contact and their events |
| `POST` | `/contacts/{id}/delete` | Delete a contact |
| `GET` | `/contacts/{id}/events/new` | Add event for a contact |
| `POST` | `/contacts/{id}/events/new` | Create an event |
| `POST` | `/events/{id}/delete` | Delete an event |
| `GET` | `/settings` | Notification settings & test |
| `POST` | `/settings/test/{channel}` | Send test notification |
| `GET` | `/logs` | Log viewer |
| `POST` | `/logs/level` | Change log level |
| `GET` | `/health` | Health check |

## License

MIT
