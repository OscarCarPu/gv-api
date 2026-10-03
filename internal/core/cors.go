package core

import "github.com/go-chi/cors"

// BrowserHeaders are the request headers a browser client may send. All must be listed: a
// preflight asking for an unlisted one gets no Access-Control-Allow-* headers and the browser
// blocks the whole request.
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
