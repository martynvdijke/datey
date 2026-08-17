## 1. Audit and persistence

- [ ] 1.1 Inventory calendar, CardDAV, settings, and integration mutation handlers.
- [ ] 1.2 Add reversible hashed-token migration and indexes.

## 2. Enforcement

- [ ] 2.1 Implement bearer validation, ownership, expiry, and revocation checks.
- [ ] 2.2 Apply user-plus-token checks while retaining CSRF and existing integration authorization.
- [ ] 2.3 Implement owner-only create/list/revoke/rotate token operations.

## 3. Verification

- [ ] 3.1 Test public TRMNL reads and all mutation authorization failures.
- [ ] 3.2 Test token ownership, malformed/expired/revoked tokens, CSRF, and one-time secrets.
- [ ] 3.3 Document migration and run Datey tests.
