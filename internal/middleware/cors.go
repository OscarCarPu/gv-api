package middleware

import "github.com/go-chi/cors"

// BrowserHeaders are the request headers a browser client may send. Every one of
// them has to be listed: go-chi/cors answers a preflight that asks for an
// unlisted header without any Access-Control-Allow-* header at all, so the
// browser reports it as a missing Access-Control-Allow-Origin and blocks the
// whole request rather than just stripping the header.
var BrowserHeaders = []string{
	"Content-Type",
	"Authorization",
	"X-Request-ID",
	"X-Device-ID", // stable per-browser UUID from gv-web; unread so far, but sent on every call
}

// CORSOptions is the CORS configuration the API serves browser clients with.
func CORSOptions(allowedOrigins []string) cors.Options {
	return cors.Options{
		AllowedOrigins:   allowedOrigins,
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "HEAD"},
		AllowedHeaders:   BrowserHeaders,
		ExposedHeaders:   []string{"Content-Length", "X-Request-ID"},
		AllowCredentials: false,
		MaxAge:           300,
	}
}
