# iCal Feed — Google Calendar & External Calendar Apps

Datey can expose all tracked dates as a public iCal feed that you can subscribe
to from Google Calendar, Apple Calendar, Thunderbird, or any app that supports
remote `.ics` subscriptions.

## Feed URLs

| Endpoint | Contents |
| --- | --- |
| `GET /ical.ics?key=<key>` | All dates across all people, plus enabled global recurring rules (e.g. Mother's Day) materialized over the next 5 years |
| `GET /ical/{personID}.ics?key=<key>` | Only one person's dates — replace `{personID}` with the ID (the number in their page URL) |

Both require `?key=<key>`. When the feed is disabled or the key is wrong, the
endpoint returns **404** — the response is deliberately indistinguishable so
the endpoint's existence is not revealed to unauthenticated callers.

For Google Calendar you only need the first URL; the per-person feed is an
optional extra if you want a specific person's dates in a separate calendar.

## Enabling

Two ways, both equivalent:

1. **Via the admin UI (recommended)** — Settings → Configuration, toggle
   **Enable Public iCal Feed**. The settings page then shows the ready-to-use
   feed URLs, including the key.
2. **Via environment variables** — set `ICAL_FEED_ENABLED=true` in `.env`.
   In this case you **must** also set `ICAL_FEED_KEY` yourself — startup fails
   with `ICAL_FEED_KEY must be set when ICAL_FEED_ENABLED is true` otherwise.

## How the feed key works

The key is a **bearer secret**: anyone who has the URL (with the key) can read
all your dates. Treat it like a password.

- **Auto-generated:** when you enable the feed via the UI, a random 128-bit key
  (32 hex characters, generated from `crypto/rand`) is created for you and
  filled into the settings form. **You do not need to set one yourself** when
  enabling from the UI.
- **Manual:** when enabling via `ICAL_FEED_ENABLED=true` in `.env`, you must
  provide `ICAL_FEED_KEY` yourself (e.g. `openssl rand -hex 32`).
- **Rotation:** to rotate the key, type a new value into the *Feed Secret Key*
  field in Settings → Configuration and save. Leaving the field empty keeps the
  current key. After rotating, update the URL in every external calendar —
  Google Calendar will show "couldn't fetch" until you do.
- **Revocation:** clearing the key and disabling the feed makes all feed URLs
  return 404 immediately.

### Optional event timing

By default events are all-day. To make them timed (e.g. so Google Calendar
renders them in a specific time slot):

| Variable | Default | Meaning |
| --- | --- | --- |
| `ICAL_EVENT_START` | — | Start time as `HH:MM` (24h); empty = all-day events (enforced 0–23 / 0–59) |
| `ICAL_EVENT_DURATION` | `60` | Duration in minutes, used when a start time is set (enforced 1–1440) |

## Subscribing in Google Calendar

1. Copy the *iCal Feed URL (all dates)* from Settings → Configuration and
   prepend your server origin, e.g. `https://datey.example.com/ical.ics?key=<key>`.
2. On the left sidebar of Google Calendar, click the **+** next to
   *Other calendars* → **From URL**.
3. Paste the full URL and click **Add calendar**.

Google refreshes subscribed iCal calendars automatically, so new dates appear
without re-subscribing.

## Behind a reverse proxy with Authelia (or similar SSO)

Google Calendar cannot perform a browser login, so the feed paths must bypass
Authelia. This is safe because the feed is already protected by the secret key.
Add a `location` block for `/ical*` **without** the `auth_request` snippet:

```nginx
include /snippets/authelia-location.conf;

# Public iCal feeds — bypass Authelia (protected by the ?key= secret instead)
location ^~ /ical {
    include /snippets/proxy.conf;
    proxy_pass $forward_scheme://$server:$port;
}

location / {
    include /snippets/proxy.conf;
    include /snippets/authelia-authrequest.conf;
    include /snippets/websocket.conf;
    proxy_pass $forward_scheme://$server:$port;
}
```

Notes:

- `location ^~ /ical` matches both `/ical.ics` and `/ical/{personID}.ics` and,
  thanks to the `^~` flag, takes precedence over `location /`.
- Omitting `authelia-authrequest.conf` means no `auth_request` fires, so the
  request is never redirected to the login page.
- The `?key=...` query string is forwarded automatically by `proxy_pass`, so
  datey's own key check still applies.
- The same pattern applies to the other public feeds (`/rss.xml`,
  `/api/upcoming`, `/api/homeassistant/stats`) if you want them reachable by
  external clients.

## Testing

Verify the feed works end to end:

```bash
# 1. Directly on datey (adjust port/host). Expect HTTP 200, Content-Type:
#    text/calendar; charset=utf-8 and a VCALENDAR body.
curl -i "http://localhost:8080/ical.ics?key=<key>"

# 2. Wrong or missing key must be a 404 (identical to disabled feed).
curl -i "http://localhost:8080/ical.ics?key=wrong"
curl -i "http://localhost:8080/ical.ics"

# 3. Through the reverse proxy. Expect 200 — NOT a 302 redirect to the
#    Authelia login page. If you see a redirect, the bypass location is
#    not being hit (check the nginx config reloaded).
curl -i "https://datey.example.com/ical.ics?key=<key>"

# 4. Per-person feed (person ID is the number in their page URL).
curl -i "http://localhost:8080/ical/1.ics?key=<key>"
```

Then add the URL in Google Calendar and confirm events appear (may take a few
minutes on Google's refresh cycle).
