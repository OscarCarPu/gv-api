// Package assistant implements the "Voz" natural-language assistant: it turns a
// user's text request into either a read-only SQL query or a structured write
// action (via an LLM), lets the user confirm it, and — for reads — summarizes
// the results in plain language. It also meters LLM token usage and cost.
//
// The suggest/execute flow is stateless: /suggest returns an opaque signed
// token carrying the proposed query/action; the client echoes that token back
// to /execute. Reads run in a rolled-back read-only transaction; writes go
// through the existing domain services so their validation and business logic
// are reused.
package assistant
