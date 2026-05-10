
## Finance

CRUD for personal accounts, the categories that classify money flows, and the transactions that move money in, out, and between accounts.

**Auth:** full-private. All endpoints require a `full` token (see [auth.md](auth.md)).

**Total:** every account has a `total` column maintained by a Postgres trigger on `transactions`. Income increases the total, expense decreases it, and a transfer simultaneously decreases the source account and increases the destination. The total is read-only via the API; it is never accepted in a request body.

**Transaction type:** `income`, `expense`, and `transfer` are values of a Postgres `transaction_type` enum reused on both `transactions.type` and `categories.type`. The matching Go enum lives in `internal/finance/txtype` and is wired through sqlc via an override, so every layer (DB row, generated params, DTO, handler validation) shares the same `txtype.Type` value. A transaction's category must have the same type as the transaction itself; the API rejects mismatches with 400.

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
- **Description:** Single roll-up used by dashboards. Returns the sum of every account's balance, this-month income/expense/balance (computed in the server's configured timezone, starting at midnight on the 1st), the equivalent figures for the previous calendar month, and every transaction from the last 30 days joined with account and category names.
- **Notes:**
  - `accounts_total` sums `accounts.total` directly. If accounts use multiple currencies the sum is naive — clients must show the breakdown themselves if that matters.
  - `month.balance = month.income - month.expense`. Transfers are excluded because they net out across accounts.
  - `previous_month` has the same shape as `month`. It is computed by summing transactions from the start of last month to the start of this month and lets clients render savings rate / month-over-month deltas without a second request.
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
- **Description:** Replaces all editable fields. The trigger reverses the prior effect on account totals and applies the new one, so changing `type`, `amount`, `account_id`, `to_account_id`, or `category_id` keeps balances consistent. `occurred_at` is required.
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

Three read-only endpoints power the chart sheets in the web client (`/money` page). They share the same date-range conventions:

- `from` and `to` are optional `YYYY-MM-DD` (or RFC3339) strings interpreted in the server's timezone.
- `to` defaults to *now*.
- `from` defaults to **the date of the earliest transaction** (`MIN(occurred_at)`), or, when there are no transactions yet, *now − 6 months*. Clients use this to implement an "All time" range simply by omitting `from`.
- All money values are returned as JSON strings (`NUMERIC(15,2)`).

### Net-worth series

- **Method:** `GET`
- **Endpoint:** `/finance/stats/networth`
- **Query Parameters:**
  - `from`, `to` — date range (see conventions above).
  - `granularity` — one of `day` | `week` | `month`. Defaults to `day`.
- **Description:** Reconstructs net worth at the end of each period in the range. The current `SUM(accounts.total)` snapshot is the anchor; the value for each bucket is computed by walking back through transaction deltas (`+income`, `-expense`, `0` for transfers because they net within the user's portfolio). Buckets are aligned to `date_trunc(granularity, from)` and emitted by `generate_series(...)`.
- **Notes:**
  - The trigger-maintained `accounts.total` is the source of truth, so opening balances seeded directly on `accounts.total` (not as `income` rows) are correctly reflected as the starting net worth, while monthly income/expense aggregations stay clean.
  - Granularities other than `day` use the chart axis's "data points are already period-aligned" rule on the client side.
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
- **Description:** Sums `amount` and counts transactions of the given `type` in the date range, grouped by `category_id`. Returns one row per leaf category with that type. Clients render the parent/child tree client-side using `/finance/categories`; this endpoint never aggregates up the parent chain.
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
- **Description:** Returns one row per calendar month in the range with summed `income`, `expense`, and computed `balance = income - expense`. Transfers are excluded by the `type IN ('income','expense')` filter — they net out across the user's accounts.
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
