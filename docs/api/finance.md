## Finance

Accounts, categories, transactions, budgets and stats.

**Auth:** full-private. All endpoints require a `full` token (see [auth.md](auth.md)).

**Total:** each account's `total` is maintained by a trigger on `transactions` (income adds, expense subtracts, a transfer moves it between accounts). It is read-only.

**Transaction type:** `income`, `expense` or `transfer` (Postgres `transaction_type`, Go `txtype.Type`). A transaction's category must have the same type, or the API answers 400.

### Account fields

| Field        | Type     | Notes                                                  |
|--------------|----------|--------------------------------------------------------|
| `id`         | integer  | Server-assigned.                                       |
| `name`       | string   | Required, 1–40 chars.                                  |
| `total`      | string   | Read-only. NUMERIC(15,2) serialized as a JSON string.  |
| `created_at` | string   | RFC3339 timestamp.                                     |

### Category fields

| Field        | Type             | Notes                                                                  |
|--------------|------------------|------------------------------------------------------------------------|
| `id`         | integer          | Server-assigned.                                                       |
| `name`       | string           | Required, 1–40 chars.                                                  |
| `parent_id`  | integer \| null  | Optional self-FK to another category. Must not equal `id`.             |
| `type`       | string           | One of `income`, `expense`, `transfer`. Conventionally matches parent. |
| `created_at` | string           | RFC3339 timestamp.                                                     |

### Transaction fields

| Field            | Type             | Notes                                                                          |
|------------------|------------------|--------------------------------------------------------------------------------|
| `id`             | integer          | Server-assigned.                                                               |
| `type`           | string           | One of `income`, `expense`, `transfer`.                                        |
| `amount`         | string           | NUMERIC(15,2) > 0, serialized as a JSON string.                                |
| `account_id`     | integer          | Required. For `transfer` this is the source account.                           |
| `to_account_id`  | integer \| null  | Required for `transfer` and must differ from `account_id`. Must be `null` for `income` / `expense`. |
| `category_id`    | integer          | Required. Must reference a category whose `type` matches the transaction type. |
| `description`    | string \| null   | Optional free-form note.                                                       |
| `occurred_at`    | string           | RFC3339 timestamp. On Create, defaults to `now()` if omitted.                  |
| `created_at`     | string           | RFC3339 timestamp.                                                             |

---

## Overview

- **Method:** `GET`
- **Endpoint:** `/finance/overview`
- **Description:** Sum of all account totals, this and last month's income/expense/balance (server timezone), and the last 30 days of transactions with account and category names.
- **Notes:**
  - `accounts_total` sums `accounts.total` directly. If accounts use multiple currencies the sum is naive — clients must show the breakdown themselves if that matters.
  - `month.balance = month.income - month.expense`. Transfers are excluded because they net out across accounts.
  - `previous_month` has the same shape as `month`.
  - `to_account_name` and `category_name` are nullable: the former is `null` for `income` / `expense`, the latter is `null` for legacy rows where the schema column is unset.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "accounts_total": "8879.15",
      "month": {
        "income": "45.00",
        "expense": "34.00",
        "balance": "11.00"
      },
      "previous_month": {
        "income": "3650.00",
        "expense": "1920.00",
        "balance": "1730.00"
      },
      "recent_transactions": [
        {
          "id": 30,
          "type": "expense",
          "amount": "11.30",
          "account_name": "Wallet",
          "to_account_name": null,
          "category_name": "Snacks",
          "description": "Snacks",
          "occurred_at": "2026-05-05T10:38:33Z"
        }
      ]
    }
    ```
- **Error Response:** `500 Internal Server Error` — `Failed to get overview`

---

## List Accounts

- **Method:** `GET`
- **Endpoint:** `/finance/accounts`
- **Description:** Returns every account, ordered by `name ASC`.
- **Success Response:** `200 OK` with an array of account objects.
- **Error Response:** `500 Internal Server Error` — `Failed to list accounts`

## Get Account

- **Method:** `GET`
- **Endpoint:** `/finance/accounts/{id}`
- **Success Response:** `200 OK` with a single account object.
- **Error Responses:** `400` `invalid account id` · `404` `account not found` · `500` `Failed to get account`

## Create Account

- **Method:** `POST`
- **Endpoint:** `/finance/accounts`
- **Request Body:** `{ "name": "Wallet" }`
- **Success Response:** `201 Created` with the new account (`total` will be `"0.00"`).
- **Error Responses:** `400` (`Invalid Body`, `name is required`, `name must be at most 40 characters`) · `500` `Failed to create account`

## Update Account

- **Method:** `PUT`
- **Endpoint:** `/finance/accounts/{id}`
- **Description:** Replaces `name`. `total` is unaffected — only transactions move money.
- **Request Body:** same as Create.
- **Success Response:** `200 OK` with the updated account.
- **Error Responses:** same validation as Create, plus `400` `invalid account id` and `404` `account not found`.

## Delete Account

- **Method:** `DELETE`
- **Endpoint:** `/finance/accounts/{id}`
- **Description:** Hard delete. Fails if any transaction (as source or destination) still references the account.
- **Success Response:** `204 No Content`
- **Error Responses:** `400` `invalid account id` · `409 Conflict` `account has transactions; delete them first` · `500` `Failed to delete account`

---

## List Categories

- **Method:** `GET`
- **Endpoint:** `/finance/categories`
- **Description:** Returns every category, ordered by `type ASC, name ASC`.
- **Success Response:** `200 OK` with an array of category objects.
- **Error Response:** `500 Internal Server Error` — `Failed to list categories`

## Get Category

- **Method:** `GET`
- **Endpoint:** `/finance/categories/{id}`
- **Success Response:** `200 OK` with a single category object.
- **Error Responses:** `400` `invalid category id` · `404` `category not found` · `500` `Failed to get category`

## Create Category

- **Method:** `POST`
- **Endpoint:** `/finance/categories`
- **Request Body:**
  ```json
  { "name": "Groceries", "parent_id": 2, "type": "expense" }
  ```
- **Success Response:** `201 Created` with the new category.
- **Error Responses:** `400` (`Invalid Body`, `name is required`, `name must be at most 40 characters`, `type must be income, expense, or transfer`, `parent_id is invalid` if the referenced parent doesn't exist) · `500` `Failed to create category`

## Update Category

- **Method:** `PUT`
- **Endpoint:** `/finance/categories/{id}`
- **Description:** Replaces all editable fields. `parent_id` may be set to `null` or to any other existing category, but never to the category's own id.
- **Request Body:** same shape as Create.
- **Success Response:** `200 OK` with the updated category.
- **Error Responses:** same validation as Create, plus `400` `invalid category id` / `parent_id must not equal id`, `404` `category not found`.

## Delete Category

- **Method:** `DELETE`
- **Endpoint:** `/finance/categories/{id}`
- **Description:** Hard delete. Fails if the category is referenced by any transaction or by another category as `parent_id`.
- **Success Response:** `204 No Content`
- **Error Responses:** `400` `invalid category id` · `409 Conflict` `category is referenced by transactions or other categories` · `500` `Failed to delete category`

---

## List Transactions

- **Method:** `GET`
- **Endpoint:** `/finance/transactions`
- **Query Parameters:**
  - `account_id` (optional): filter to transactions where the account appears as either source or destination.
- **Description:** Ordered by `occurred_at DESC, id DESC`.
- **Success Response:** `200 OK` with an array of transaction objects.
- **Error Responses:** `400` `invalid account_id` · `500` `Failed to list transactions`

## Get Transaction

- **Method:** `GET`
- **Endpoint:** `/finance/transactions/{id}`
- **Success Response:** `200 OK` with a single transaction object.
- **Error Responses:** `400` `invalid transaction id` · `404` `transaction not found` · `500` `Failed to get transaction`

## Create Transaction

- **Method:** `POST`
- **Endpoint:** `/finance/transactions`
- **Request Body (income / expense):**
  ```json
  { "type": "income", "amount": "100.00", "account_id": 1, "category_id": 7, "description": "salary" }
  ```
- **Request Body (transfer):**
  ```json
  { "type": "transfer", "amount": "20.00", "account_id": 1, "to_account_id": 2, "category_id": 22 }
  ```
- **Notes:**
  - `category_id` is required, and the category's `type` must match the transaction's `type`.
  - `occurred_at` may be supplied (RFC3339); if omitted, the server uses `now()`.
  - The trigger updates `accounts.total` atomically with the insert.
- **Success Response:** `201 Created` with the new transaction.
- **Error Responses:** `400` (`Invalid Body`, `type must be income, expense, or transfer`, `to_account_id is required for transfer`, `to_account_id must differ from account_id`, `to_account_id must be omitted for income`/`expense`, `amount must be greater than 0`, `account_id is required`, `category_id is required`, `category type does not match transaction type`, `referenced account or category does not exist`) · `500` `Failed to create transaction`

## Update Transaction

- **Method:** `PUT`
- **Endpoint:** `/finance/transactions/{id}`
- **Description:** Replaces all editable fields; the trigger keeps account totals consistent. `occurred_at` is required.
- **Request Body:** same shape as Create, plus an explicit `occurred_at`.
- **Success Response:** `200 OK` with the updated transaction.
- **Error Responses:** same validation as Create, plus `400` `invalid transaction id` / `occurred_at is required`, `404` `transaction not found`.

## Delete Transaction

- **Method:** `DELETE`
- **Endpoint:** `/finance/transactions/{id}`
- **Description:** Hard delete. The trigger reverses the row's effect on account totals.
- **Success Response:** `204 No Content`
- **Error Responses:** `400` `invalid transaction id` · `500` `Failed to delete transaction`

---

## Stats

Four read-only endpoints. The first three share these date-range conventions:

- `from` and `to` are optional `YYYY-MM-DD` (or RFC3339) strings interpreted in the server's timezone.
- `to` defaults to *now*.
- `from` defaults to the earliest transaction (or now − 6 months if none), so omitting it means "all time".
- All money values are returned as JSON strings (`NUMERIC(15,2)`).

### Net-worth series

- **Method:** `GET`
- **Endpoint:** `/finance/stats/networth`
- **Query Parameters:**
  - `from`, `to` — date range (see conventions above).
  - `granularity` — one of `day` | `week` | `month`. Defaults to `day`.
- **Description:** Net worth at the end of each period, walking back from the current `SUM(accounts.total)` through income and expense (transfers net out). Buckets align to `date_trunc(granularity, from)`.
- **Notes:**
  - Opening balances seeded directly on `accounts.total` show up as the starting net worth.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      { "date": "2024-05-01", "total": "5450.00" },
      { "date": "2024-06-01", "total": "5876.32" }
    ]
    ```
- **Error Responses:** `400` (`invalid from`, `invalid to`, `granularity must be day, week, or month`) · `500` `Failed to compute net worth`

### Stats by category

- **Method:** `GET`
- **Endpoint:** `/finance/stats/by-category`
- **Query Parameters:**
  - `type` (**required**) — one of `income` | `expense` | `transfer`.
  - `from`, `to` — date range (see conventions above).
  - `account_id` (optional) — filter to transactions where this account is the source *or* destination.
- **Description:** Sums and counts transactions of `type` per category in the range. Never aggregates up the parent chain; clients build the tree from `/finance/categories`.
- **Notes:**
  - `share` is each row's amount divided by the sum across all rows in the response (range-relative, not all-time). It is `0` when the range total is `0`.
  - `category_id` is `null` for transactions whose category was deleted before the schema required it; `name` falls back to `"Sin categoría"` in that case.
  - Sorting: `SUM(amount) DESC, name ASC`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      { "category_id": 20, "name": "Rent",    "amount": "1750.00", "tx_count": 2,  "share": 0.469 },
      { "category_id": 19, "name": "Dinner",  "amount": "485.50",  "tx_count": 11, "share": 0.130 }
    ]
    ```
- **Error Responses:** `400` (`type must be income, expense, or transfer`, `invalid from`, `invalid to`, `invalid account_id`) · `500` `Failed to compute category stats`

### Monthly stats

- **Method:** `GET`
- **Endpoint:** `/finance/stats/monthly`
- **Query Parameters:**
  - `from`, `to` — date range.
  - `account_id` (optional) — filter to transactions touching this account.
  - `category_id` (optional) — filter to transactions tagged with this exact category id.
- **Description:** One row per calendar month with `income`, `expense` and `balance = income - expense`. Transfers are excluded.
- **Notes:**
  - The `month` field is the `YYYY-MM` form of `date_trunc('month', occurred_at)`.
  - Sorting: `month ASC`.
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    [
      { "month": "2024-05", "income": "0",       "expense": "716.86",  "balance": "-716.86" },
      { "month": "2024-06", "income": "2459.44", "expense": "2016.33", "balance": "443.11" }
    ]
    ```
- **Error Responses:** `400` (`invalid from`, `invalid to`, `invalid account_id`, `invalid category_id`) · `500` `Failed to compute monthly stats`

### Estimation

- **Method:** `GET`
- **Endpoint:** `/finance/stats/estimation`
- **Query Parameters:**
  - `start_month` (**required**) — `YYYY-MM` (or `YYYY-MM-DD`). Clamped to the earliest transaction's month, so an earlier value means "all history".
  - `end_month` (**required**) — `YYYY-MM` (or `YYYY-MM-DD`). The last month included in the projected series. Must be on or after `start_month`.
  - `mode` (**required**) — one of `rate` | `saving`. Selects how the projection factor is derived and applied.
- **Description:** Monthly series from `start_month` to `end_month`: actual points (the `networth` reconstruction, up to last month) then projected ones from the current month. The projection factor is derived from the actuals:
  - `mode=rate`: compound monthly rate `r` with `last = first × (1 + r)^n`, as a percentage (`1.25` = 1.25%/month). Projection: `prev × (1 + r/100)`.
  - `mode=saving`: average monthly delta `(last − first) / n`. Projection: `prev + saving`.
  - With fewer than two actual points, `rate` and `saving` are `0` and the projection stays flat.
- **Notes:**
  - `rate` and `saving` are always present; the one not matching `mode` is `0`.
  - Each point's `date` is the first day of the bucket month (same convention as `/finance/stats/networth` with `granularity=month`).
  - `estimated` marks projected points. The segments are contiguous (last actual = previous month).
- **Success Response:**
  - **Code:** `200 OK`
  - **Content:**
    ```json
    {
      "points": [
        { "date": "2025-12-01", "total": "8000.00", "estimated": false },
        { "date": "2026-01-01", "total": "8200.00", "estimated": false },
        { "date": "2026-02-01", "total": "8350.00", "estimated": false },
        { "date": "2026-03-01", "total": "8500.00", "estimated": false },
        { "date": "2026-04-01", "total": "8700.00", "estimated": false },
        { "date": "2026-05-01", "total": "8852.46", "estimated": true },
        { "date": "2026-06-01", "total": "9007.55", "estimated": true }
      ],
      "rate": "1.7521",
      "saving": "0"
    }
    ```
- **Error Responses:** `400` (`start_month is required (YYYY-MM)`, `end_month is required (YYYY-MM)`, `end_month must be on or after start_month`, `mode must be rate or saving`) · `500` `Failed to compute estimation`

---

## Budgets

Budgets per income or expense category, compared with actuals. A budget is **monthly** or **yearly** (for annual or irregular expenses).

**Model:** a budget applies to its period and every later one until changed. Each (category, period) pair is its own series. Every change has a `scope`:

- `forward` (default) — this period and every later one take the new value; later changes are discarded.
- `once` — only this period changes; the next one goes back to whatever was in effect before (unless it already had its own value). `month` is still accepted as an alias.

Removing a budget takes the same scopes (`forward` ends it, `once` skips one period). `0` is a real budget ("spend nothing"), distinct from having none. Transfer categories cannot be budgeted.

**Roll-up:** a budget covers its category and same-type descendants, except subtrees budgeted with the other period. Nested budgets of the same period are reported with `depth` and not counted twice.

Months and years are calendar periods in the server's configured timezone (same as `/finance/overview`).

### Get a month

- **Method:** `GET`
- **Endpoint:** `/finance/budgets?month=YYYY-MM`
- **Query:** `month` — optional, defaults to the current month. Yearly budgets are those of the year containing it.
- **Success Response:** `200 OK`
  ```json
  {
    "month": "2026-09",
    "month_progress": 0.95,
    "expense": { "budgeted": "2140.00", "actual": "1973.67", "unbudgeted": "0", "overspent": "74.60" },
    "income":  { "budgeted": "3200.00", "actual": "3067.06", "unbudgeted": "86.53", "overspent": "0" },
    "items": [
      {
        "category_id": 12, "period": "monthly", "name": "Eating out", "parent_id": 6, "type": "expense",
        "depth": 1, "budget": "350.00", "actual": "424.60", "remaining": "-74.60",
        "progress": 1.213, "status": "over", "since": "2026-03"
      }
    ],
    "planned_balance": "980.83",
    "yearly": {
      "year": "2026",
      "year_progress": 0.74,
      "expense": { "budgeted": "950.00", "actual": "877.80", "unbudgeted": "0", "overspent": "2.30" },
      "income":  { "budgeted": "0", "actual": "0", "unbudgeted": "0", "overspent": "0" },
      "items": [
        {
          "category_id": 30, "period": "yearly", "name": "Property tax (IBI)", "parent_id": null,
          "type": "expense", "depth": 0, "budget": "450.00", "actual": "452.30", "remaining": "-2.30",
          "progress": 1.005, "status": "over", "since": "2025"
        }
      ]
    },
    "averages": [ { "category_id": 6, "amount": "739.12" } ],
    "previous_year": [ { "category_id": 30, "amount": "438.10" } ]
  }
  ```
- **Fields:**
  - `month_progress` / `yearly.year_progress` — elapsed share of the period (`1` past, `0` future).
  - `budgeted` — sum of the outermost budgets only (a budgeted child under a budgeted parent is not added again).
  - `actual` — monthly: every income / expense transaction of the month, budgeted or not. Yearly: what the yearly budgets' categories took in the year.
  - `unbudgeted` — monthly only: the part of `actual` no budget of either period covers, uncategorized included.
  - `overspent` — expenses beyond the period's budgets, counting each euro once across nested budgets. `0` for income.
  - `items` — every budget in effect, depth-first in category order. `depth` is the number of budgeted ancestors of the same type and period; `remaining = budget − actual` (negative when over); `progress = actual / budget` (a `0` budget reports `1` when anything was spent); `since` is when the value in effect started (`YYYY-MM` monthly, `YYYY` yearly).
  - `status` — expenses: `ok` (< 80%), `warning` (≥ 80%), `over` (> 100%). Income: `pending` / `met` (actual ≥ budget).
  - `planned_balance` — monthly budgeted income − expenses, plus a twelfth of the yearly budgeted net: what the plan expects to save in an average month.
  - `averages` — per income / expense category with activity, the average of the last 3 complete months before `month` (rolled up like `actual`), to suggest a monthly amount.
  - `previous_year` — per category, the previous calendar year's total (rolled up), to suggest a yearly amount.
- **Error Responses:** `400` `month must be YYYY-MM` · `500` `Failed to get budgets`

### Budget transactions

- **Method:** `GET`
- **Endpoint:** `/finance/budgets/{category_id}/transactions?month=YYYY-MM&period=monthly|yearly`
- **Description:** The transactions behind a budget's `actual` in the month (or year, for `yearly`), newest first, shaped like `recent_transactions` in `/finance/overview`. Defaults: current month, `monthly`. Works for any income/expense category.
- **Success Response:** `200 OK` with an array of `{ id, type, amount, account_name, to_account_name, category_name, description, occurred_at }`.
- **Error Responses:** `400` (`month must be YYYY-MM`, `period must be monthly or yearly`, `transfer categories cannot be budgeted`) · `404` `category not found` · `500` `Failed to list budget transactions`

### Set a budget

- **Method:** `PUT`
- **Endpoint:** `/finance/budgets/{category_id}`
- **Request Body:** `{ "month": "2026-09", "amount": "400.00", "scope": "forward", "period": "monthly" }` — `scope` defaults to `forward`, `period` to `monthly`. For `yearly`, `month` may also be `YYYY`; only its year is used. `amount` is 0–9999999999999.99, rounded to cents.
- **Success Response:** `204 No Content`
- **Error Responses:** `400` (`Invalid Body`, `month is required (YYYY-MM)`, `month must be YYYY-MM`, `scope must be forward or once`, `period must be monthly or yearly`, amount out of range, `transfer categories cannot be budgeted`) · `404` `category not found` · `500` `Failed to save budget`

### Remove a budget

- **Method:** `DELETE`
- **Endpoint:** `/finance/budgets/{category_id}?month=YYYY-MM&scope=forward|once&period=monthly|yearly`
- **Success Response:** `204 No Content` (also when there was nothing to remove).
- **Error Responses:** same as Set.
