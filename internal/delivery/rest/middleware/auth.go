package middleware

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// TokenParser verifies a bearer access token. auth.TokenIssuer satisfies it; declaring the
// port here keeps the middleware free of the JWT library.
type TokenParser interface {
	ParseAccessToken(raw string) (usecase.Actor, error)
}

// FailureReporter renders a failure. handler.Responder satisfies it.
type FailureReporter interface {
	FromError(w http.ResponseWriter, req *http.Request, err error)
}

// authorizationHeader is the only scheme the API accepts; "Basic" and anything else are
// rejected rather than silently ignored, so a misconfigured client fails loudly.
const authorizationHeader = "Authorization"

type actorKey struct{}

// errNoActor reports a route that was mounted without the Authenticate middleware. It is
// deliberately not a domain sentinel: the responder must render it as a 500, not tell the
// client to go and authenticate.
var errNoActor = errors.New("middleware: no verified identity on the request")

// WithActor places a verified identity on the context. Exported so an authorizing handler can
// be tested without minting a real token.
func WithActor(ctx context.Context, actor usecase.Actor) context.Context {
	return context.WithValue(ctx, actorKey{}, actor)
}

// ActorFrom reads the identity the Authenticate middleware installed.
func ActorFrom(ctx context.Context) (usecase.Actor, bool) {
	actor, ok := ctx.Value(actorKey{}).(usecase.Actor)
	return actor, ok && actor.UserID != ""
}

// bearer extracts the credential from an Authorization header.
func bearer(header string) (string, error) {
	if header == "" {
		return "", fmt.Errorf("%w: authorization header is required", domain.ErrUnauthorized)
	}

	scheme, credential, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found || strings.TrimSpace(credential) == "" {
		return "", fmt.Errorf("%w: authorization header must be \"Bearer <token>\"", domain.ErrTokenInvalid)
	}
	// The scheme is case-insensitive per RFC 7235, but the credential must stay byte-exact.
	if !strings.EqualFold(scheme, "bearer") {
		return "", fmt.Errorf("%w: unsupported authorization scheme %q", domain.ErrTokenInvalid, scheme)
	}
	return strings.TrimSpace(credential), nil
}

// Authenticate turns a bearer access token into a usecase.Actor on the request context.
// Every failure is a 401: the response must not reveal whether a token was unknown, expired
// or forged.
func Authenticate(parser TokenParser, report FailureReporter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			credential, err := bearer(r.Header.Get(authorizationHeader))
			if err != nil {
				report.FromError(w, r, err)
				return
			}

			actor, err := parser.ParseAccessToken(credential)
			if err != nil {
				report.FromError(w, r, err)
				return
			}

			// The parsed subject is the only trusted source of identity: a body or query
			// parameter naming a different account is never consulted.
			next.ServeHTTP(w, r.WithContext(WithActor(r.Context(), actor)))
		})
	}
}

// RequireRoles authorizes an already-authenticated request. The list is checked against the
// role carried in the verified token, so demoting an account takes effect as soon as its short
// access token expires.
func RequireRoles(report FailureReporter, roles ...domain.Role) func(http.Handler) http.Handler {
	allowed := make(map[domain.Role]bool, len(roles))
	for _, role := range roles {
		allowed[role] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			actor, ok := ActorFrom(r.Context())
			if !ok {
				// Reaching this means a route was mounted outside Authenticate, which is a wiring
				// fault rather than a client error, so it reports as a 500 and never leaks the
				// internal message.
				report.FromError(w, r, errNoActor)
				return
			}
			if !allowed[actor.Role] {
				report.FromError(w, r, domain.ErrForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
