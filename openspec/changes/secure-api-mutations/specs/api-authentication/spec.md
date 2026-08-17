## ADDED Requirements

### Requirement: Public Datey reads

The service SHALL allow unauthenticated GET requests to `/api/trmnl/stats`, `/api/trmnl/birthdays`, and documented public read endpoints.

#### Scenario: TRMNL stats poll

- WHEN TRMNL requests either public feed without credentials
- THEN Datey returns the corresponding read payload

### Requirement: Mutations require user and token

Calendar, CardDAV, settings, Home Assistant/API-key management, and every create/update/delete route MUST require both an authenticated user and a valid bearer API token, in addition to existing CSRF protections for browser requests.

#### Scenario: Anonymous calendar write

- WHEN an anonymous client posts or deletes a calendar resource
- THEN Datey rejects the request and leaves the resource unchanged

### Requirement: Secure token lifecycle

An authenticated user SHALL create, list metadata for, revoke, and rotate owned tokens. Secrets MUST be returned only once and stored as hashes.

#### Scenario: Expired token

- WHEN an expired token is used on a mutation
- THEN Datey rejects it without revealing token state or writing data
