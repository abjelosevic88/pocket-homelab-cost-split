# API

Base path `/api/v1`. JSON in, JSON out. Authenticate with the session cookie (browser) or a personal token:

```
Authorization: Bearer ps_xxxxxxxx…
```

Create tokens under **Settings → API tokens**. A token acts as the user who created it and sees their groups. With Pocket Core on the box, a token Core issued to a paired phone or a person (`pcd_`, `pcu_`, `pco_`) works the same way.

**Pocket Core app links.** Another Pocket app may act for a person with a five-minute `pcx_` token from Core (see `docs/links.md` in pocket-homelab-core). It sees exactly what that person sees, and only these routes; everything else answers `403`:

| Scope | Routes |
|---|---|
| any | `GET /auth/me` |
| `groups:read` | `GET /groups`, `GET /groups/{id}`, `GET /groups/{id}/expenses`, `GET /groups/{id}/expenses/{eid}`, `GET /categories` |
| `expenses:write` | `POST /groups/{id}/expenses`; `PUT` · `DELETE /groups/{id}/expenses/{eid}` only for expenses that app added (others answer `404`) |

An expense added over a link has `"source": "<app slug>"` (empty for people), and its activity line ends in “· via Pocket <App>”. Dates are `YYYY-MM-DD`. **Money is in integer minor units** of the given currency: `1250` = 12.50 EUR, `1500` = ¥1500, `1250` = 1.250 BHD. `GET /currencies` tells you the decimals.

## Groups

| Method | Path | Notes |
|---|---|---|
| `GET` | `/groups` | Your groups: the ones you created or where you are a linked member (admins too, no exception). `archived=1` for archived ones, `q=` to search. A group you may not see is `404` everywhere under `/groups/{id}`. |
| `POST` | `/groups` | `{"name", "description", "currency", "members": [{"name"}, {"user_id": 2}, {"name": "Me", "me": true}]}`. You are always added as a member. Currency defaults to the one in Settings. |
| `GET` | `/groups/{id}` | The group with `members`, `expense_count`, `total` (spent, group minor units), `my_member_id`, `my_balance`. |
| `PUT` | `/groups/{id}` | `{"name", "description", "currency", "archived"}`. Currency only while the group has no expenses. |
| `DELETE` | `/groups/{id}` | Creator only. Everything in it goes. Unlinking or removing someone else's account (`PUT`/`DELETE /members/{mid}`) is also the creator's call (`403` otherwise). |
| `GET` | `/groups/{id}/balances` | `{"currency", "balances": [{member_id, paid, owed, balance}], "settlements": [{from, to, amount}]}` |
| `GET` | `/groups/{id}/stats` | Totals by month, category, member and currency (see below). |
| `GET` | `/groups/{id}/activity` | Newest first, `limit=` (default 50). |
| `GET` | `/groups/{id}/export.json` · `export.csv` | Full export; CSV has one column per person with their part in the group currency. |
| `GET` | `/groups/{id}/report` | Filtered summary: who owes whom (`settlements`, the whole group as of now), totals, per person, by category, by month, the `expenses` list, currencies. Settle-up payments are never part of a report (only the debts section counts them). Filters: `from=`, `to=` (YYYY-MM-DD), `categories=a,b`, `member=` (paid or shared), `debts=1` to add the who-owes-whom section. |
| `GET` | `/groups/{id}/report.pdf` | The same report as an A4 PDF (same filters). |
| `POST` | `/import` | Body = an `export.json`. Creates a new group "(imported)" with its people and expenses at the stored rates. |

### Members

| Method | Path | Notes |
|---|---|---|
| `POST` | `/groups/{id}/members` | `{"name": "Marko"}` or `{"user_id": 2}` (name taken from the account). `{"name": "PSUP project", "kind": "cost_center"}` adds a cost center (see below). |
| `PUT` | `/groups/{id}/members/{mid}` | `{"name"}` and/or `{"user_id": 2}` to link, `{"user_id": 0}` to unlink. `{"kind": "cost_center"}` / `{"kind": "person"}` turns a member into a cost center or back (refused for an account, a member who paid for expenses, or a cost center that payments bought into). |
| `DELETE` | `/groups/{id}/members/{mid}` | Only if they are in no expense. |

## Expenses

| Method | Path | Notes |
|---|---|---|
| `GET` | `/groups/{id}/expenses` | `{"expenses": […], "total"}`. Filters: `q=`, `member=` (paid or shared), `category=`, `from=`, `to=`, `limit=`, `offset=`. Newest date first, shares included. |
| `POST` | `/groups/{id}/expenses` | Body below. |
| `GET` · `PUT` · `DELETE` | `/groups/{id}/expenses/{eid}` | Read / replace (full body) / delete. |

### Expense body

```json
{
  "title": "Dinner at Ramiro",
  "amount": 12600,                 // minor units of "currency"
  "currency": "EUR",               // default: the group currency
  "fx_rate": 1,                    // 1 unit of currency in group currency; omit to look it up for "date"
  "paid_by": 2,                    // member id
  "date": "2026-10-04",            // default: today
  "category": "restaurant",        // a category id, see GET /categories; default the fallback (General). Always "" on a reimbursement
  "notes": "Marko had the lobster",
  "split_mode": "amount",          // even (default) | shares | percent | amount
  "reimbursement": false,          // true = a payment from paid_by to the single sharer
  "shares": [                      // who it was for; "share" depends on split_mode:
    {"member_id": 1, "share": 3800},   //   even: ignored · shares: weight · percent: hundredths of a percent (10000 = 100 %) · amount: minor units
    {"member_id": 2, "share": 4800},
    {"member_id": 3, "share": 4000}
  ]
}
```

The answer carries the computed parts: each share's `amount` (expense currency) and `amount_group` (group currency), which always add up to the expense exactly. Validation errors come back as `400 {"error": "percentages must add up to 100"}`.

**A reimbursement** ("Jelena paid Marko 100 €"): `{"amount": 10000, "paid_by": <Jelena>, "reimbursement": true, "shares": [{"member_id": <Marko>}]}`. Title defaults to "Reimbursement"; it is left out of spending totals.

### Cost centers (projects)

A member has a `kind`: `person` (default) or `cost_center`, a project the people of the group fund up front and that pays them back later. A group can have several. A cost center:

- never carries a personal share and never appears in `balances` / `settlements` (who owes whom);
- takes a cost as a whole: an expense whose only share is the cost center (`"shares": [{"member_id": <project>}]`) becomes the payer's claim on it. It cannot be split together with people, and it never pays for expenses itself;
- pays people back with a payment: `{"reimbursement": true, "paid_by": <project>, "shares": [{"member_id": <Ana>}], "amount": …}`. A payment from a person to the cost center is money put into it (a claim too).

**Buying into a cost center's debt.** A payment from B to A with `"cost_center_id": <project>` is B taking over part of what the project owes A: afterwards the project owes B that amount instead of A, and nobody owes anybody personally. A must hold at least that much of a claim on the project. Without `cost_center_id` it stays an ordinary personal payment.

`GET /groups/{id}/balances` answers both books:

```json
{
  "currency": "EUR",
  "balances": [{"member_id": 1, "paid": 0, "owed": 0, "balance": 0}, …],   // people only
  "settlements": [],                                                        // people only
  "cost_centers": [{"member_id": 3, "charged": 200000, "repaid": 0, "owes": 200000,
                    "claims": [{"member_id": 1, "amount": 100000}, {"member_id": 2, "amount": 100000}]}]
}
```

A group's `my_balance` is the personal balance; `my_claims` is what its cost centers owe the viewer. The dashboard's `positions[].claims` / `claims_text` and the group lines' `claims` carry the same, and their `settlements` include the cost centers' debts marked `"cost_center": true`. Reports list them under `cost_center_debts`.

## Currencies & rates

| Method | Path | Notes |
|---|---|---|
| `GET` | `/currencies` | `[{"code": "EUR", "name": "Euro", "decimals": 2}, …]` — the ISO 4217 list. |
| `GET` | `/categories` | `[{"id": "restaurant", "label": "Restaurants & cafés", "icon": "🍽️", "builtin": true, "sort_order": 3, "group_id": 0, "count": 12, "fallback": false}, …]`: the built-in ones (offered in every group, `group_id` 0) and the own categories of every group you may see (`group_id` set, `"mine": true` = you may change it). `?group=ID` narrows it to what that group offers. `fallback` marks where expenses without a category land (General). |
| `GET` · `POST` | `/groups/{id}/categories` | What the group offers (built-in + its own) / add one to the group: `{"label": "Ski passes", "icon": "🎿"}` → the category (id is a slug of the label). A label or slug the group already offers returns the existing one. |
| `POST` | `/categories` | Same, with `"group_id"` in the body (required). |
| `PUT` · `DELETE` | `/categories/{id}` | A category of a group you may see (anyone in the group), or a built-in one if you are an admin. `{"label", "icon", "sort_order"}` / remove (its expenses move to the fallback category). The built-in General can be removed once no expense uses it, and then the first built-in category left becomes the fallback; the last built-in category cannot be removed. An expense may use a built-in category or one of its own group. |
| `GET` | `/fx?from=USD&to=EUR&date=2026-10-01` | `{"rate": 0.9218, "source": "frankfurter", "date", "example": "92.18 EUR"}`. Cached per pair and day. `502` when no provider knows the pair (or lookups are off). |

## Home & activity

| Method | Path | Notes |
|---|---|---|
| `GET` | `/dashboard` | See below. |
| `GET` | `/activity?limit=20` | Newest lines across your groups. |

### Dashboard

```json
{
  "groups": 2, "archived_groups": 0,
  "positions": [{"currency": "EUR", "you_owe": 5324, "you_are_owed": 0, "net": -5324}, {"currency": "BAM", "you_owe": 0, "you_are_owed": 47382, "net": 47382}],
  "you_owe_text": "53.24 EUR", "you_are_owed_text": "473.82 BAM", "net_text": "473.82 BAM · -53.24 EUR",
  "spent_this_month": [{"currency": "EUR", "total": 18169, "count": 10}], "spent_text": "181.69 EUR",
  "group_lines": [{"id": 1, "name": "Lisbon week", "currency": "EUR", "balance": -5324, "members": 3, "expenses": 14, "total": 177500, "last_date": "2026-10-11"}]
}
```

A [Homepage](https://gethomepage.dev) `customapi` widget:

```yaml
- Pocket Split:
    href: https://split.example.com/
    icon: mdi-account-cash
    widget:
      type: customapi
      url: http://pocket-split:8080/api/v1/dashboard
      headers:
        Authorization: Bearer {{HOMEPAGE_VAR_POCKET_SPLIT_TOKEN}}
      mappings:
        - { field: you_owe_text, label: You owe, format: text }
        - { field: you_are_owed_text, label: Owed to you, format: text }
        - { field: groups, label: Groups, format: number }
```

### Stats

```json
{
  "currency": "EUR", "total": 177500, "reimbursed": 10000, "count": 13,
  "months": [{"month": "2026-09", "total": 122700, "count": 2}],
  "categories": [{"category": "accommodation", "total": 84000, "count": 1}],
  "members": [{"member_id": 1, "paid": 53745, "share": 59069, "count": 5}],
  "currencies": [{"currency": "USD", "total": 8250, "count": 1}],
  "first_date": "2026-09-02", "last_date": "2026-10-11"
}
```

## Users, settings, tokens

| Method | Path | Notes |
|---|---|---|
| `GET` | `/auth/status` | `needs_setup`, `auth_mode`, `authenticated`, `user`, `version`, `title`. No auth needed. |
| `POST` | `/auth/setup` · `/auth/login` · `/auth/logout` | `{"email", "password", "name"}` / `{"email", "password"}` / – |
| `GET` · `PUT` | `/auth/me` | Profile; `PUT {"name", "email", "current_password", "new_password"}`. |
| `GET` · `POST` · `PUT` · `DELETE` | `/users`, `/users/{id}` | Admin. `{"email", "name", "password", "role": "admin|member"}`. |
| `GET` · `PUT` | `/settings` | `{"title", "default_currency", "fx_auto", "date_format", "time_format"}`. `default_currency`, `date_format` and `time_format` are your own; `title` and `fx_auto` are box-wide and only an admin's save changes them. Date formats: `auto`, `dmy-dot` (31.12.2026), `dmy-slash`, `mdy-slash`, `ymd`, `dmmmy` (31 Dec 2026), `mmmdy`; time: `auto`, `24h`, `12h`. |
| `GET` | `/settings/system` | Version, runtime, paths, row counts. |
| `GET` · `POST` · `DELETE` | `/tokens`, `/tokens/{id}` | `POST {"name"}` → `{"token": "ps_…"}` (shown once). |
| `GET` | `/healthz` · `/readyz` · `/metrics` | Liveness, readiness (DB ping), Prometheus. No auth. |
