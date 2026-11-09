# Spec Delta

## Purpose

Controls who can call the single-user API: password login, a TOTP second factor for full access, and the token kinds that gate each group of routes.

## ADDED Requirements

### Requirement: Password login
`POST /login` with `{"password": ...}` SHALL return 200 `{"token", "kind": "tmp"}` for `PASSWORD` and 200 `{"token", "kind": "semi"}` for `SEMIPRIVATE_PASSWORD`. Any other password SHALL return 401 `{"error": "invalid password"}`, and a malformed body 400 `{"error": "Invalid Request"}`.

#### Scenario: Private password
- **WHEN** a client posts the private password
- **THEN** the response is 200 with `kind` `tmp` and a token that expires 5 minutes after issue

#### Scenario: Semiprivate password
- **WHEN** a client posts the semiprivate password
- **THEN** the response is 200 with `kind` `semi` and a token that expires 30 days after issue

#### Scenario: Wrong password
- **WHEN** a client posts any other password
- **THEN** the response is 401 with `{"error": "invalid password"}`

### Requirement: Second factor
`POST /login/2fa` with `{"token", "code"}` SHALL return 200 `{"token"}` holding a `full` token valid for 30 days when the token is a valid `tmp` token and the code is the current 6-digit TOTP (SHA-1, 30 s step, one step of skew either way) for `TOTP_SECRET`.

#### Scenario: Valid code
- **WHEN** a client posts a live `tmp` token and the current code
- **THEN** the response is 200 with a `full` token

#### Scenario: Wrong code
- **WHEN** a client posts a live `tmp` token and a wrong code
- **THEN** the response is 401 with `{"error": "invalid 2fa code"}`

#### Scenario: Token of the wrong kind
- **WHEN** a client posts a `semi` or `full` token, or an expired `tmp` token
- **THEN** the response is 401 with `{"error": "invalid token"}`

### Requirement: Token format compatibility
Tokens SHALL be HS256 JWTs signed with `JWT_SECRET` that carry `exp`, `iat` and `kind` claims. Tokens issued by the Go service SHALL be accepted, and tokens issued by the new service SHALL be accepted by the Go service, for as long as they are unexpired.

#### Scenario: Token issued before the switch
- **WHEN** a `full` token issued by the Go service before cutover is presented to the new service
- **THEN** it is accepted on full-private routes until its `exp`

#### Scenario: Non-HMAC algorithm
- **WHEN** a token signed with an algorithm other than HMAC, or with `alg: none`, is presented
- **THEN** it is rejected with 401

### Requirement: Route tiers
Protected routes SHALL require `Authorization: Bearer <token>`. Semiprivate routes (rutas, printers, uptime) SHALL accept `semi` or `full` tokens. All other protected routes SHALL accept only `full`. A `tmp` token SHALL be accepted by no protected route. A missing, malformed, expired, invalid or wrong-kind token SHALL return 401 `{"error": "Unauthorized"}`.

#### Scenario: Semi token on a full route
- **WHEN** a client calls `GET /habits` with a `semi` token
- **THEN** the response is 401 with `{"error": "Unauthorized"}`

#### Scenario: Semi token on a semiprivate route
- **WHEN** a client calls `GET /rutas/marks` with a `semi` token
- **THEN** the request is served

#### Scenario: Missing Bearer prefix
- **WHEN** a client sends `Authorization: <token>` without `Bearer `
- **THEN** the response is 401 with `{"error": "Unauthorized"}`
