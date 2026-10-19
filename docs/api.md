# API

Base path `/api/v1`. JSON in, JSON out. Authenticate with the session cookie (browser) or a personal token:

```
Authorization: Bearer ps_xxxxxxxx…
```

Create tokens under **Settings → API tokens**. A token acts as the user who created it and sees their groups. Dates are `YYYY-MM-DD`. **Money is in integer minor units** of the given currency: `1250` = 12.50 EUR, `1500` = ¥1500, `1250` = 1.250 BHD. `GET /currencies` tells you the decimals.

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
| `GET` | `/groups/{id}/report` | Filtered summary: who owes whom (`settlements`, the whole group as of now), totals, per person, by category, by month, `expenses` and `payments` lists, currencies. Filters: `from=`, `to=` (YYYY-MM-DD), `categories=a,b`, `member=` (paid or shared), `reimbursements=include` (default: excluded), `debts=1` to add the who-owes-whom section. |
| `GET` | `/groups/{id}/report.pdf` | The same report as an A4 PDF (same filters). |
| `POST` | `/import` | Body = an `export.json`. Creates a new group "(imported)" with its people and expenses at the stored rates. |

### Members

| Method | Path | Notes |
|---|---|---|
| `POST` | `/groups/{id}/members` | `{"name": "Marko"}` or `{"user_id": 2}` (name taken from the account). |
| `PUT` | `/groups/{id}/members/{mid}` | `{"name"}` and/or `{"user_id": 2}` to link, `{"user_id": 0}` to unlink. |
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
  "category": "restaurant",        // a category id, see GET /categories; default "general"
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

## Currencies & rates

| Method | Path | Notes |
|---|---|---|
| `GET` | `/currencies` | `[{"code": "EUR", "name": "Euro", "decimals": 2}, …]` — the ISO 4217 list. |
| `GET` | `/categories` | `[{"id": "restaurant", "label": "Restaurants & cafés", "icon": "🍽️", "builtin": true, "sort_order": 3, "count": 12}, …]` |
| `POST` | `/categories` | `{"label": "Ski passes", "icon": "🎿"}` → the category (id is a slug of the label). A label or slug that already exists returns the existing one. Any user. |
| `PUT` · `DELETE` | `/categories/{id}` | Your own category, or a built-in one if you are an admin. `{"label", "icon", "sort_order"}` / remove (its expenses move to `general`, which cannot be removed). `GET /categories` lists the built-in ones, yours (`"mine": true`) and any used in a group you share. |
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
