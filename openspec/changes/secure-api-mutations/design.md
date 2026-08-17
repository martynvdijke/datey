## Context

Datey already has CSRF protections and API keys for selected integrations. The change adds a common user-scoped bearer token for API mutations while preserving public TRMNL feeds and existing browser safeguards.

## Goals / Non-Goals

### Goals

- Preserve public stats and birthdays reads.
- Protect all mutation families with user identity plus bearer token.
- Keep CSRF checks for cookie-authenticated browser writes.

### Non-Goals

- Removing existing integration keys or CSRF checks.
- Protecting public GET feeds.
- Committing a real token.

## Decisions

- Store only a cryptographic token hash with owner, timestamps, expiry, and revocation metadata.
- Compose bearer validation with session authentication and existing CSRF checks.
- Protect calendar, CardDAV, settings, and integration mutation routes; retain any stronger existing authorization.
- Provide owner-only lifecycle operations with one-time secret display.

## Risks / Trade-offs

Automation must be migrated to user sessions and tokens. Keeping the existing integration keys and CSRF checks creates layered controls but requires careful route ordering.

## Migration Plan

1. Add token migration and lifecycle endpoints.
2. Audit handler methods and enforce the middleware.
3. Add authorization/CSRF regression tests.
4. Provision tokens to clients and revoke legacy credentials as appropriate.

## Open Questions

- Should integration API keys eventually be represented as scoped user tokens?
