## Why

Datey must keep its public TRMNL stats and birthdays feeds readable while ensuring calendar and integration writes cannot be performed without a user and API token.

## What Changes

- Keep `GET /api/trmnl/stats` and `GET /api/trmnl/birthdays` public.
- Require an authenticated user plus bearer API token for calendar, CardDAV, settings, and all create/update/delete operations.
- Add owner-scoped token creation and lifecycle management with hash-only storage.

## Capabilities

### New Capabilities

- `api-authentication`

### Modified Capabilities

- `public-api-reads`

## Impact

Datey handlers, existing CSRF/API-key checks, persistence migrations, token management, tests, and client documentation are affected. Public TRMNL polling remains compatible.
