package abcmiddleware

import (
	"net/http"

	"go.uber.org/zap"
)

// MiddlewareFunc is the function signature for Chi's Use() middleware
type MiddlewareFunc func(http.Handler) http.Handler

// Middleware exposes useful variables to every abcmiddleware handler
type Middleware struct {
	// Log is used for logging in your middleware and to
	// create a derived logger that includes the request ID.
	Log *zap.Logger
}

// MW is an interface defining middleware wrapping
type MW interface {
	Wrap(http.Handler) http.Handler
}
