package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// testMoment anchors every timestamp in this file so the encoded output is reproducible.
var testMoment = time.Date(2026, time.October, 21, 9, 0, 0, 0, time.UTC)

func decodeRegisterBody(t *testing.T, contentType, body string) (registerRequest, error) {
	t.Helper()

	log, _ := newTestLogger()
	r := NewResponder(log)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	var in registerRequest
	return in, r.decode(req, &in)
}

func TestDecodeValidBody(t *testing.T) {
	in, err := decodeRegisterBody(t, "application/json",
		`{"email":"ada@example.com","full_name":"Ada Lovelace","phone":"+15550100","password":"correct horse battery staple"}`)
	require.NoError(t, err)

	assert.Equal(t, "ada@example.com", in.Email)
	assert.Equal(t, "Ada Lovelace", in.FullName)
	assert.Equal(t, "+15550100", in.Phone)
	assert.Equal(t, "correct horse battery staple", in.Password)
}

// The charset suffix and casing both appear in real clients, and a strict comparison would
// reject them for no reason.
func TestDecodeAcceptsContentTypeVariants(t *testing.T) {
	cases := []string{
		"application/json",
		"application/json; charset=utf-8",
		"APPLICATION/JSON",
		"application/json;charset=UTF-8",
	}

	for _, contentType := range cases {
		t.Run(contentType, func(t *testing.T) {
			_, err := decodeRegisterBody(t, contentType, `{"email":"a@b.co","full_name":"A B","phone":"+15550100","password":"secret"}`)
			assert.NoError(t, err)
		})
	}
}

func TestDecodeRejectsMalformedInput(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		wantField   string
	}{
		{name: "missing content type", contentType: "", body: `{}`, wantField: "content_type"},
		{name: "form encoding", contentType: "application/x-www-form-urlencoded", body: "email=a@b.co", wantField: "content_type"},
		{name: "text plain", contentType: "text/plain", body: `{}`, wantField: "content_type"},
		{name: "empty body", contentType: "application/json", body: "", wantField: "body"},
		{name: "truncated object", contentType: "application/json", body: `{"email":"a`, wantField: "body"},
		{name: "syntax error", contentType: "application/json", body: `{"email": }`, wantField: "body"},
		{name: "bare string", contentType: "application/json", body: `"just a string"`, wantField: "body"},
		{name: "array body", contentType: "application/json", body: `[]`, wantField: "body"},
		{name: "wrong field type", contentType: "application/json", body: `{"email":123}`, wantField: "email"},
		{name: "unknown field", contentType: "application/json", body: `{"email":"a@b.co","role":"admin"}`, wantField: "body"},
		{name: "trailing document", contentType: "application/json", body: `{"email":"a@b.co"} {"email":"c@d.co"}`, wantField: "body"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := decodeRegisterBody(t, tc.contentType, tc.body)
			require.Error(t, err, "a malformed body must not decode")
			require.ErrorIs(t, err, domain.ErrInvalidInput, "every decode failure is the client's, so it is a 400")

			var ve *domain.ValidationError
			require.ErrorAs(t, err, &ve, "the failure must carry field details for the error envelope")
			require.Len(t, ve.Fields, 1)
			assert.Equal(t, tc.wantField, ve.Fields[0].Field)
		})
	}
}

// A body over the cap has to fail closed. Without the limit a single request could make the
// decoder allocate arbitrarily much before validation ever runs.
func TestDecodeEnforcesTheBodyLimit(t *testing.T) {
	huge := `{"email":"` + strings.Repeat("a", maxBodyBytes) + `","full_name":"A B","phone":"+15550100","password":"secret"}`

	_, err := decodeRegisterBody(t, "application/json", huge)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "larger than the limit")

	// A body that fits is read in full; the limit is a ceiling, not a truncating proxy.
	in, err := decodeRegisterBody(t, "application/json", `{"email":"`+strings.Repeat("a", 64)+`@b.co","full_name":"A B","phone":"+15550100","password":"secret"}`)
	require.NoError(t, err)
	assert.Len(t, in.Email, 69)
}

// Two documents in one body is a smuggling attempt, not a batch: the second must never be applied.
func TestDecodeRejectsATrailingDocument(t *testing.T) {
	_, err := decodeRegisterBody(t, "application/json",
		`{"email":"a@b.co","full_name":"A B","phone":"+15550100","password":"secret"} {"email":"c@d.co"}`)
	require.ErrorIs(t, err, domain.ErrInvalidInput)
	assert.Contains(t, err.Error(), "exactly one JSON document")
}

func TestUserResponseNeverCarriesACredential(t *testing.T) {
	// The projection is the single place a user becomes JSON, so this is the assertion that
	// makes adding a hash field to the response impossible to miss.
	user := domain.User{
		ID:           "0b8f5c1a-2d3e-4a5b-8c9d-0e1f2a3b4c5d",
		Email:        "ada@example.com",
		FullName:     "Ada Lovelace",
		Phone:        "+15550100",
		Role:         domain.RoleDoctor,
		Status:       domain.StatusActive,
		PasswordHash: "$2b$10$aVerySecretHash",
		CreatedAt:    testMoment,
		UpdatedAt:    testMoment,
	}

	body := encode(t, newUserResponse(user))
	assert.NotContains(t, body, "password")
	assert.NotContains(t, body, "$2b$10$aVerySecretHash")
	assert.Contains(t, body, `"role":"doctor"`)
	assert.Contains(t, body, `"status":"active"`)

	// Field names are a contract with the client; a rename has to be a deliberate change.
	for _, field := range []string{"id", "email", "full_name", "phone", "role", "status", "created_at", "updated_at"} {
		assert.Contains(t, body, `"`+field+`":`)
	}
}

func TestNewAuthResponseShape(t *testing.T) {
	pair := usecase.TokenPair{
		AccessToken:      "access-token",
		AccessExpiresAt:  testMoment.Add(15 * time.Minute),
		RefreshToken:     "refresh-secret",
		RefreshExpiresAt: testMoment.Add(72 * time.Hour),
	}

	body := encode(t, newAuthResponse(
		domain.User{ID: "u1", Email: "ada@example.com", Role: domain.RolePatient, Status: domain.StatusActive},
		pair,
	))

	assert.Contains(t, body, `"token_type":"Bearer"`)
	assert.Contains(t, body, `"access_token":"access-token"`)
	assert.Contains(t, body, `"refresh_token":"refresh-secret"`)
	assert.NotContains(t, body, "password")

	// The plaintext secret appears exactly once: this is the one response allowed to hand it over.
	assert.Equal(t, 1, strings.Count(body, "refresh-secret"))
}

// The transport never invents a page size: anything absent, unparsable or negative reaches
// the use case as 0, which means "use your own default and clamp".
func TestPagingReadsQueryParameters(t *testing.T) {
	cases := []struct {
		name       string
		query      string
		wantLimit  int
		wantOffset int
	}{
		{name: "nothing supplied", query: "", wantLimit: 0, wantOffset: 0},
		{name: "both supplied", query: "limit=25&offset=50", wantLimit: 25, wantOffset: 50},
		{name: "surrounding space", query: "limit=%205%20", wantLimit: 5, wantOffset: 0},
		{name: "negative offset", query: "offset=-1", wantLimit: 0, wantOffset: 0},
		{name: "not a number", query: "limit=abc", wantLimit: 0, wantOffset: 0},
		{name: "overflowing value", query: "limit=99999999999999999999", wantLimit: 0, wantOffset: 0},
		{name: "repeated parameter", query: "limit=5&limit=7", wantLimit: 5, wantOffset: 0},
		{name: "unrelated parameter", query: "role=admin", wantLimit: 0, wantOffset: 0},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, offset := paging(parseQuery(t, tc.query))
			assert.Equal(t, tc.wantLimit, limit)
			assert.Equal(t, tc.wantOffset, offset)
		})
	}
}

func parseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	parsed, err := url.ParseQuery(raw)
	require.NoError(t, err)
	return parsed
}

func encode(t *testing.T, v any) string {
	t.Helper()
	encoded, err := json.Marshal(v)
	require.NoError(t, err)
	return string(encoded)
}
