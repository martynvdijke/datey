# Home Assistant plugin

Show your next important date on a Home Assistant dashboard as a RESTful
sensor — "Dana's birthday in 3 days" as the sensor value, with the full
upcoming list exposed as JSON attributes.

## How it works

Datey exposes a key-protected JSON stats endpoint:

```
GET /api/homeassistant/stats?key=YOUR_KEY
```

Home Assistant polls it with a [RESTful sensor](https://www.home-assistant.io/integrations/sensor.rest/),
maps the `next_event_days` field to the sensor value, and exposes the rest of
the payload as attributes.

## Setup

1. **Enable the feed** — log in to Datey as admin and open
   **Settings → Configuration → Home Assistant**.
2. **Enable the toggle** — check *Enable Home Assistant Feed*. A secret key is
   generated automatically (you can rotate it any time by editing the field).
3. **Copy the feed URL** — the *Feed URL* field shows the full path
   (`/api/homeassistant/stats?key=...`). Prepend your server origin, e.g.
   `http://datey.local:6270` or `https://datey.example.com`.
4. **Install the sensor** — copy the snippet from `sensor.yaml` into your
   `configuration.yaml` under the `sensor:` key, replacing the placeholders.
5. **Reload** — in Home Assistant go to **Settings → Devices & Services →
   Reload → Reload sensors**, or restart Home Assistant.

The feed is disabled by default — no data is exposed until you enable it. Keep
`configuration.yaml` readable only by users you trust: the key grants read
access to all tracked dates.

## sensor.yaml

A ready-to-paste RESTful sensor configuration. Replace `YOUR_INSTANCE` and
`YOUR_KEY` before using it.

```yaml
sensor:
  - platform: rest
    name: "Next Datey Event"
    resource: "http://YOUR_INSTANCE:6270/api/homeassistant/stats?key=YOUR_KEY"
    value_template: "{{ value_json.next_event_days }}"
    json_attributes_path: "$.upcoming"
    json_attributes: ["name", "type", "date", "days"]
    scan_interval: 3600
```

- `value_template` — number of days until the next event (0 = today).
- `json_attributes` — the upcoming list; combine with
  `value_json.next_event` / `value_json.next_event_date` for more detail.
- `scan_interval: 3600` — poll once per hour; the endpoint is a cheap
  SQLite query, so shorter intervals are fine on a LAN.

## Example dashboard card

```yaml
type: markdown
content: |
  **Next: {{ states('sensor.next_datey_event') }} days until
  {{ state_attr('sensor.next_datey_event', 'next_event') }}**
```

## Notes

- Dates are personal data: unlike the TRMNL stats feed, this endpoint is
  **always key-protected** — a missing or unknown `key` returns 404.
- The `upcoming` array only contains events inside the reminder window
  (Settings → General → Reminder Days); the 7/30-day counts always cover
  those exact horizons.
- Looking for the TRMNL e-ink display instead? See the `trmnl/` folder.
