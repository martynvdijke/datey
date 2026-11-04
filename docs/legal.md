# Legal pages and account deletion

Datey ships the three pages a hosted, account-based service needs: a privacy
policy, terms of use, and an account-deletion page. They are public routes,
reachable without signing in, because a store reviewer and a prospective user
both need to read them before they have an account.

| Route | Template | Purpose |
| --- | --- | --- |
| `GET /privacy` | `internal/web/templates/privacy.html` | What is stored, why, who processes it, retention, rights, Google Limited Use |
| `GET /terms` | `internal/web/templates/terms.html` | Account rules, shared address book, availability, liability |
| `GET /account/delete` | `internal/web/templates/account_delete.html` | How to delete an account or a person without an account, and what remains |

The pages are linked from the footer of every page.

## Configuration

The pages render sensible text with no configuration at all, but an instance
that collects real data should name its operator. All four values are env-only
(restart to apply), never database-backed:

```sh
LEGAL_CONTROLLER_NAME="Example Family Cooperative"
LEGAL_CONTACT_EMAIL=privacy@example.com
LEGAL_CONTACT_ADDRESS="1 Example Street, 1000 AA Amsterdam"
LEGAL_HOSTING_REGION="the Netherlands"
```

- `LEGAL_CONTROLLER_NAME` is the GDPR "controller". Empty falls back to
  "the operator of this Datey instance".
- `LEGAL_CONTACT_EMAIL` adds a `mailto:` contact and enables the email fallback
  on the deletion page. Empty removes both rather than inventing an address.
- `LEGAL_CONTACT_ADDRESS` is optional; a postal address is good practice but not
  mandatory in the text.
- `LEGAL_HOSTING_REGION` describes where the server runs, for the transfer
  paragraph. Empty falls back to "the European Union".

The policy also adapts to the instance. The analytics paragraph appears only
when `UMAMI_URL` is set, tracing only when `OTEL_ENDPOINT` is set, the email
processor only when SMTP is configured, push only when the web-push notifier is
configured, the Google section only when `GOOGLE_CONTACTS_ENABLED` is true, and
the notification-processor list is read from the live notifier registry. An
instance cannot accidentally claim a processor it is not running.

Datey's settings overlay runs at boot, so `UMAMI_URL`, `OTEL_ENDPOINT`,
`SMTP_*`, `GOOGLE_*` and the notification settings saved in the admin UI are
reflected on the next restart. Values set only in the database after boot are
not re-read by the legal handler until then.

## What deletion actually does

`POST /settings/account/delete` (web, form fields `confirm` + CSRF token) and
`DELETE /api/v1/me` (JSON API, bearer token or session cookie, body
`{"confirm":"<username>"}` or `{"confirm":"DELETE"}`) both call
`internal/account.Service.Delete` in one transaction:

1. Refuse if the account is the last remaining admin (`409 last_admin` on the
   API, an error redirect on the web).
2. Delete every session, API token, password-reset token, push subscription and
   per-user notification channel belonging to the account.
3. Delete notification-history rows whose `user_id` points at the account.
4. Relabel audit entries that named the username to `deleted-user`, keeping the
   action, target, time and source IP for security review.
5. Delete the user row itself. Its username becomes available again.

Shared address-book content is deliberately kept. Datey has no per-user
ownership of people, events, groups, notes or gift ideas: they belong to the
instance and are visible to every account. Deleting an account must not remove
another member's birthday or gift list. A member who wants a person record
removed deletes that person from the person detail page; administrators can do
the same from the admin UI.

The OIDC link (`oidc_sub`) disappears with the user row, so the next sign-in
creates a fresh account if the identity provider still allows it. The operator
can revoke Datey's access at the provider to prevent that.

### Person records without accounts

Most people in Datey never have an account. `POST /people/{id}/delete` removes
the person and its events, notes, gift ideas, photo, tags and relationships.
The deletion page explains this separately so a data subject can ask for their
own record to be removed without closing an operator account.

## Export

`GET /api/v1/me/export` (and the `/api/me/export` alias) returns JSON for the
authenticated requester:

- account profile and preferences;
- API-token metadata (name, timestamps — never the hash or secret);
- notification channels (type, target, enabled);
- audit entries whose actor username matches;
- the linked person record, with that person's events, timeline, gift ideas,
  tags, groups and relationships;
- the push-subscription count.

Secrets are omitted on purpose: token hashes, push endpoint/keys, password
hashes and the OIDC subject cannot be handed back safely. The shared address
book is not attributable to one account and is not included; the response says
so in its `notes` field. This is an honest partial export, not a fabricated
complete one.

## Checklist for an EU launch

- [ ] Set all four `LEGAL_*` values, then open `/privacy`, `/terms` and
      `/account/delete` while signed out and confirm they render.
- [ ] Confirm `APP_URL` (or the deployment's public origin) is the HTTPS URL
      used in store listings.
- [ ] Sign a data-processing agreement with the hosting provider.
- [ ] Check the hosting region matches `LEGAL_HOSTING_REGION` and that any
      transfers outside the EEA rely on SCCs or an adequacy decision.
- [ ] Decide a backup retention period and write it down; the policy says
      backups rotate out without promising a number.
- [ ] Keep a record of processing activities. For a single instance this can be
      a page in your own documentation.
- [ ] Confirm the Google Cloud OAuth consent screen lists the
      `contacts.readonly` scope and links this privacy policy.
- [ ] Publish the deletion URL in the Play listing (see `docs/play-store.md`).
- [ ] Review the Data safety form whenever a permission, SDK or server feature
      changes.
