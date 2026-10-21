package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

const testUserID = "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d"

// recordingResponder captures the error a middleware decided to report. Rendering it is the
// responder's own job and is covered in the handler package, so these tests assert on the error.
type recordingResponder struct {
	err error
}

func (r *recordingResponder) FromError(_ http.ResponseWriter, _ *http.Request, err error) {
	r.err = err
}

// stubParser accepts one credential and reports the actor behind it.
type stubParser struct {
	credential string
	actor      usecase.Actor
	err        error

	calls int
}

func (p *stubParser) ParseAccessToken(raw string) (usecase.Actor, error) {
	p.calls++
	if p.err != nil {
		return usecase.Actor{}, p.err
	}
	if raw != p.credential {
		return usecase.Actor{}, domain.ErrTokenInvalid
	}
	return p.actor, nil
}

func TestBearerExtraction(t *testing.T) {
	cases := []struct {
		name    string
		header  string
		want    string
		wantErr error
	}{
		{name: "canonical", header: "Bearer token123", want: "token123"},
		{name: "lowercase scheme", header: "bearer token123", want: "token123"},
		{name: "uppercase scheme", header: "BEARER token123", want: "token123"},
		{name: "leading and trailing space", header: "  Bearer token123  ", want: "token123"},
		{name: "missing header", header: "", wantErr: domain.ErrUnauthorized},
		{name: "scheme only", header: "Bearer", wantErr: domain.ErrTokenInvalid},
		{name: "scheme with blank credential", header: "Bearer   ", wantErr: domain.ErrTokenInvalid},
		{name: "basic scheme", header: "Basic dXNlcjpwYXNz", wantErr: domain.ErrTokenInvalid},
		{name: "bare credential", header: "token123", wantErr: domain.ErrTokenInvalid},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := bearer(tc.header)

			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, got, "a rejected header must not yield a credential")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestAuthenticate(t *testing.T) {
	ada := usecase.Actor{UserID: testUserID, Role: domain.RoleDoctor}

	cases := []struct {
		name        string
		header      string
		parserErr   error
		wantErr     error
		wantReached bool
	}{
		{name: "valid token", header: "Bearer secret", wantReached: true},
		{name: "no header", header: "", wantErr: domain.ErrUnauthorized},
		{name: "wrong scheme", header: "Token secret", wantErr: domain.ErrTokenInvalid},
		{name: "expired token", header: "Bearer secret", parserErr: domain.ErrTokenExpired, wantErr: domain.ErrTokenExpired},
		{name: "forged signature", header: "Bearer secret", parserErr: domain.ErrTokenInvalid, wantErr: domain.ErrTokenInvalid},
		{name: "revoked session", header: "Bearer secret", parserErr: domain.ErrSessionRevoked, wantErr: domain.ErrSessionRevoked},
		{name: "issuer failure propagates unwrapped", header: "Bearer secret", parserErr: errors.New("issuer unavailable")},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			parser := &stubParser{credential: "secret", actor: ada, err: tc.parserErr}
			reporter := &recordingResponder{}
			authorized := false

			handler := Authenticate(parser, reporter)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen, ok := ActorFrom(r.Context())
				assert.Truef(t, ok, "the verified actor is missing from the context")
				assert.Equal(t, ada, seen)

				authorized = true
				_, _ = w.Write([]byte("ok"))
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			if tc.wantReached {
				assert.True(t, authorized, "the handler must run for a valid token")
				assert.Equal(t, "ok", rec.Body.String())
				assert.NoError(t, reporter.err)
				assert.Equal(t, 1, parser.calls, "the parser must run exactly once per request")
				return
			}

			assert.False(t, authorized, "the handler must not run for a rejected request")
			assert.Empty(t, rec.Body.String())
			if tc.wantErr != nil {
				assert.ErrorIs(t, reporter.err, tc.wantErr)
			}
		})
	}

	t.Run("a rejected header never reaches the parser", func(t *testing.T) {
		parser := &stubParser{credential: "secret", actor: ada}
		reporter := &recordingResponder{}

		Authenticate(parser, reporter)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Error("the handler must not run")
		})).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil))

		assert.Equal(t, 0, parser.calls, "a missing header must not become a parse attempt")
	})
}

func TestAuthenticateRejectsAnActorWithoutASubject(t *testing.T) {
	// A parser that hands back an empty user id would authorize every downstream lookup against
	// an unknown account, so RequireRoles treats it as absent.
	reporter := &recordingResponder{}
	reached := false

	RequireRoles(reporter, domain.RoleAdmin)(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { reached = true })).
		ServeHTTP(httptest.NewRecorder(), withBearer(httptest.NewRequest(http.MethodGet, "/x", nil), "secret"))

	assert.False(t, reached, "an actor with no user id must not read as authenticated")
	assert.ErrorIs(t, reporter.err, errNoActor)
}

func TestRequireRoles(t *testing.T) {
	cases := []struct {
		name        string
		roles       []domain.Role
		actor       *usecase.Actor
		wantErr     error
		wantReached bool
	}{
		{
			name: "admin among admins", roles: []domain.Role{domain.RoleAdmin},
			actor: &usecase.Actor{UserID: testUserID, Role: domain.RoleAdmin}, wantReached: true,
		},
		{
			name: "doctor among admin and doctor", roles: []domain.Role{domain.RoleAdmin, domain.RoleDoctor},
			actor: &usecase.Actor{UserID: testUserID, Role: domain.RoleDoctor}, wantReached: true,
		},
		{
			name: "patient among admins", roles: []domain.Role{domain.RoleAdmin},
			actor: &usecase.Actor{UserID: testUserID, Role: domain.RolePatient}, wantErr: domain.ErrForbidden,
		},
		{
			name: "admin outside the list", roles: []domain.Role{domain.RoleDoctor},
			actor: &usecase.Actor{UserID: testUserID, Role: domain.RoleAdmin}, wantErr: domain.ErrForbidden,
		},
		{
			// An empty allow list denies everybody, so a half-configured guard fails closed.
			name: "no role allows nobody", roles: nil,
			actor: &usecase.Actor{UserID: testUserID, Role: domain.RoleAdmin}, wantErr: domain.ErrForbidden,
		},
		{
			// A route mounted outside Authenticate is a wiring fault, not a client mistake: it
			// reports an unlabeled internal error so the responder renders an opaque 500.
			name: "missing actor", roles: []domain.Role{domain.RoleAdmin},
			actor: nil, wantErr: errNoActor,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reached := false
			reporter := &recordingResponder{}

			handler := RequireRoles(reporter, tc.roles...)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				reached = true
				_, _ = w.Write([]byte("ok"))
			}))

			req := httptest.NewRequest(http.MethodGet, "/api/v1/admin/users", nil)
			if tc.actor != nil {
				req = req.WithContext(WithActor(req.Context(), *tc.actor))
			}

			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, tc.wantReached, reached)
			if tc.wantErr != nil {
				require.Error(t, reporter.err)
				if errors.Is(tc.wantErr, errNoActor) {
					assert.NotErrorIs(t, reporter.err, domain.ErrUnauthorized,
						"a wiring fault must not be reported as a client authentication failure")
					assert.NotContains(t, reporter.err.Error(), "Bearer", "the message must not echo the credential")
					return
				}
				assert.ErrorIs(t, reporter.err, tc.wantErr)
			}
		})
	}
}

func TestActorContextRoundTrip(t *testing.T) {
	_, ok := ActorFrom(context.Background())
	assert.False(t, ok, "an unset context must not read as authenticated")

	actor := usecase.Actor{UserID: testUserID, Role: domain.RoleAdmin}
	got, ok := ActorFrom(WithActor(context.Background(), actor))
	require.True(t, ok)
	assert.Equal(t, actor, got)

	// The key is an unexported type, so a value stored under any other key cannot satisfy this.
	type otherKey struct{}
	_, ok = ActorFrom(context.WithValue(context.Background(), otherKey{}, actor))
	assert.False(t, ok)

	// A stored actor with no subject is rejected rather than forwarded downstream.
	_, ok = ActorFrom(WithActor(context.Background(), usecase.Actor{Role: domain.RoleAdmin}))
	assert.False(t, ok)
}

func withBearer(req *http.Request, credential string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+credential)
	return req
}
