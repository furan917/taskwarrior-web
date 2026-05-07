package handlers

import "context"

type csrfKey struct{}

// WithCSRFToken stores the per-request CSRF token in context. Called by the
// CSRF middleware on safe-method requests.
func WithCSRFToken(ctx context.Context, token string) context.Context {
	return context.WithValue(ctx, csrfKey{}, token)
}

// CSRFToken extracts the token rendered by the middleware. Empty string if
// the middleware wasn't applied (e.g. /healthz).
func CSRFToken(ctx context.Context) string {
	if t, ok := ctx.Value(csrfKey{}).(string); ok {
		return t
	}
	return ""
}
