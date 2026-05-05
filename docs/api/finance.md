
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
- **Description:** Single roll-up used by dashboards. Returns the sum of every account's balance, this-month income/expense/balance (computed in the server's configured timezone, starting at midnight on the 1st), and every transaction from the last 30 days joined with account and category names.
- **Notes:**
  - `accounts_total` sums `accounts.total` directly. If accounts use multiple currencies the sum is naive — clients must show the breakdown themselves if that matters.
  - `month.balance = month.income - month.expense`. Transfers are excluded because they net out across accounts.
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
