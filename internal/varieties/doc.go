// Package varieties manages the weed variety catalog with per-judge scores.
// Mounted under the semiprivate auth group (semi or full token).
//
// Endpoints:
//
//	GET    /varieties        - list varieties
//	GET    /varieties/{id}   - get variety
//	POST   /varieties        - create variety
//	PUT    /varieties/{id}   - update variety
//	DELETE /varieties/{id}   - delete variety
package varieties
