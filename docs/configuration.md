# Configuration

All settings are environment variables. Nothing is required.

## Server

| Variable | Default | Description |
|---|---|---|
| `HOST` | `0.0.0.0` | Listen address. |
| `PORT` | `8080` | Listen port. |
| `BASE_URL` | – | Public URL (e.g. `https://split.example.com`). Also the base of the OIDC callback and the redirect URI the app asks Pocket Core for. |
| `DATA_DIR` | `./data` (`/data` in the container) | Database and backups. |
| `DB_PATH` | `$DATA_DIR/pocket-split.db` | Override the database file. |
| `TZ` | `UTC` | Timezone for "today" (the default expense date, the month in stats). The container ships `tzdata`. |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error`. |
| `LOG_FORMAT` | `text` | `text` or `json`. |
| `TRUST_PROXY` | `true` | Honour `X-Forwarded-For` / `X-Real-IP` for logging. |
| `METRICS_ENABLED` | `true` | Serve Prometheus metrics on `/metrics` (unauthenticated — restrict at the proxy if exposed). |
| `CORE_URL` / `CORE_REGISTRATION_KEY` | – | [Pocket Core](https://git.bjelke.org/abjelosevic88/pocket-homelab-core): register on start (the key is needed once; the minted app token lives in `$DATA_DIR/core.token`), publish `split.expense_created` / `split.expense_deleted` / `split.settled`, and accept the tokens Core hands to paired phones. |
| `CORE_APP_URL` | `BASE_URL` | How Core reaches this app for health checks (e.g. `http://host.docker.internal:8172`). |
| `OIDC_ISSUER` / `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | – | Sign in with an OpenID Connect provider (PocketID, Authelia, Authentik, Keycloak…). Needs `BASE_URL`; the callback is `BASE_URL/api/v1/auth/oidc/callback`. On a box with Pocket Core none of these are needed — the app receives its client when it registers. |
| `OIDC_NAME` | `single sign-on` | Button label: “Sign in with …” (a Core-issued client is labelled “Pocket Core”). |
| `OIDC_AUTO_CREATE` | `true` | Give an unknown (verified) email a member account instead of refusing it. Always on for a Core-issued client. |
| `DEMO_DATA` | `false` | Seed a sample trip and a flat share into an *empty* database. |

## Money

| Variable | Default | Description |
|---|---|---|
| `DEFAULT_CURRENCY` | `EUR` | Offered when creating a group. Settings → General overrides it once saved. |
| `FX_PROVIDER` | `auto` | Where exchange rates come from: `auto` tries [Frankfurter](https://frankfurter.app) (ECB reference rates, ~30 currencies, historical by date) and falls back to the [ExchangeRate-API open endpoint](https://www.exchangerate-api.com/docs/free) (~160 currencies, today's rates). `frankfurter` or `erapi` use only that one. `none` never calls out: the rate field must be filled by hand. Settings → General has an on/off switch too. |
| `FX_TIMEOUT` | `8s` | Timeout for a rate lookup. |

Rates are cached in the database, one per currency pair and day (today's is refreshed after six hours). A rate is copied onto the expense when it is saved, so later changes never touch old balances.

## Authentication & users

| Variable | Default | Description |
|---|---|---|
| `AUTH_MODE` | `local` | `local`: built-in email + password, first-run setup screen, users managed under Settings → Users. `proxy`: trust the user name from a header set by your SSO proxy; users are auto-created on first visit (first one becomes admin). `none`: no login at all, one implicit user — only for trusted networks. |
| `AUTH_PROXY_HEADER` | `Remote-User` | Header carrying the user name in `proxy` mode. |
| `AUTH_PROXY_EMAIL_HEADER` | `Remote-Email` | Header carrying the email in `proxy` mode. |
| `ADMIN_EMAIL`, `ADMIN_PASSWORD`, `ADMIN_NAME` | – | In `local` mode, create this admin on first start instead of showing the setup screen. Password must be 8+ characters. |
| `SESSION_TTL` | `720h` | Browser session lifetime. |
| `SECURE_COOKIES` | `false` | Mark the session cookie `Secure`. Turn on behind HTTPS. |

**Who sees what.** A group is visible to the users linked to one of its members, and to admins. Anyone in a group can add or edit expenses and manage its people; only the creator or an admin can delete it. Members do not need an account: add them by name and link an account later if they get one (their expenses stay). Roles: **admin** manages users, the shared settings and sees every group; **member** sees their groups. API tokens (`Authorization: Bearer …` or `X-API-Key`) work in every mode and act as the user who created them.

## In-app settings

- **Settings → General:** app title, default currency for new groups, automatic exchange-rate lookup on/off, date and time format (admins only).
- **Settings → Categories:** the expense categories. Anyone can add one (also by typing a new name in the expense form); admins rename, reorder or remove them.
- **Group → Settings:** name, description, main currency (locked once the group has expenses), people, archive, delete, export.

## Forgot your password?

```bash
docker exec pocket-split pocket-split reset-password            # first user, random password printed
docker exec pocket-split pocket-split reset-password -email me@example.com -password NewPass123
```
