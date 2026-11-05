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

The privacy policy carries an effective date and version (`1.0`, 5 November
2026). Bump both when the processing described in the policy changes.

## Demo account

The hosted instance can expose a public demo account for store review and
first-run trials. It is provisioned at boot by `internal/demo` and is off by
default except on `datey.vandijke.xyz`.

| Variable | Default | Meaning |
| --- | --- | --- |
| `DEMO_USER_ENABLED` | on only when `APP_URL` host is `datey.vandijke.xyz` | Provision and advertise the demo account |
| `DEMO_USER_HANDLE` | `demo` | Username; must match `^[a-z0-9_]{3,24}$` |
| `DEMO_USER_PASSWORD` | `demo` | Password; shown publicly, so use only on a disposable deployment |
| `DEMO_USER_EMAIL` | `demo@datey.local` | Optional account email; a conflicting address is left unset rather than stolen |

`demo.Ensure` runs on every boot and is idempotent. It creates a normal,
non-admin, read-write user with a bcrypt hash, repairs the password and locale
on restart, refuses to take over an account whose email differs, and refuses to
touch an existing admin account. It seeds a fictional address book (one group,
two tags, three people, relationships, upcoming events, gift ideas and person
notes) **only when the shared people table is empty**. People are shared across
the instance, so seeding never runs on an instance that already has an address
book.

The web login page shows a **Try the demo account** form with the advertised
credentials when the feature is on. `GET /api/auth/config` exposes the same
values for native clients:

```json
{"demo":{"enabled":true,"handle":"demo","password":"demo"}}
```

When the account is disabled, or the handle/password are empty, the endpoint
returns `{"demo":{"enabled":false}}` and Android hides the demo card. Enabling
demo mode on a real instance publishes working read-write credentials on the
login page; do not do it.

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

The self-service handlers then write a single `account.delete_self` entry with
actor `deleted-user` and target `user:<id>`; they do not call the normal
request-derived audit helper, which would otherwise re-introduce the deleted
username from the request context. Administrator deletion writes `user.delete`
with the administrator as actor and `user:<id>` as target, so no username of
the deleted account survives in the audit log. Tests assert that neither the
username nor the email appears in any audit row after deletion.

Shared address-book content is deliberately kept. Datey has no per-user
ownership of people, events, groups, notes or gift ideas: they belong to the
instance and are visible to every account. Deleting an account must not remove
another member's birthday or gift list. A member who wants a person record
removed deletes that person from the person detail page; administrators can do
the same from the admin UI.

The OIDC link (`oidc_sub`) disappears with the user row, so the next sign-in
creates a fresh account if the identity provider still allows it. The operator
can revoke Datey's access at the provider to prevent that.

The public `/account/delete` page also works without signing in, which is what
Play reviewers and former members need. When `LEGAL_CONTACT_EMAIL` is set it
shows a prefilled `mailto:` request (subject and body included) and the address
as text; otherwise it names the configured controller and asks the user to
contact the operator. Both variants explain what is removed and what remains,
and link to the privacy policy for the supervisory-authority route. The
signed-in view adds the exact-confirmation form and the data-export link.

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

Export responses are sent with `Cache-Control: no-store` and
`Pragma: no-cache` so a shared browser or proxy does not retain account data.
The web pages link the endpoint from Settings → Account and data and from the
signed-in deletion page. The Android app exposes the same call as Settings →
Download my data, shows the payload size, and offers a copy-to-clipboard action
for the JSON.

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
- [ ] For a store review, enable the demo account only on the demo host, verify
      `GET /api/auth/config` returns the advertised handle/password, and confirm
      both the web login page and the Android onboarding show **Try the demo
      account**. Never enable it on an instance with real member data.
- [ ] Sign in as the demo account and test Settings → Download my data and
      Settings → Delete account, then re-create the demo content by wiping the
      disposable demo database.
- [ ] Confirm the signed-out deletion page shows either the prefilled mailto
      (with `LEGAL_CONTACT_EMAIL`) or the named-operator fallback.
- [ ] Confirm self-service deletion leaves no username or email in the audit
      log; `internal/web/handlers_legal_test.go` covers this.
