package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/NumerR/ElitStom/internal/delivery/rest/middleware"
	"github.com/NumerR/ElitStom/internal/domain"
	"github.com/NumerR/ElitStom/internal/usecase"
)

// maxBodyBytes caps a decoded request. The limit is generous for a profile edit and tiny
// next to the ~1 kB these endpoints actually need, so it only ever stops a memory-grabbing
// payload from reaching the JSON decoder.
const maxBodyBytes = 1 << 14 // 16 KiB

// --- requests ---------------------------------------------------------------

type registerRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// sessionRequest is the body of both refresh and logout: the one credential the client keeps.
type sessionRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type profileRequest struct {
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
}

type createUserRequest struct {
	Email    string `json:"email"`
	FullName string `json:"full_name"`
	Phone    string `json:"phone"`
	Role     string `json:"role"`
	Password string `json:"password"`
}

type roleRequest struct {
	Role string `json:"role"`
}

type statusRequest struct {
	Status string `json:"status"`
}

// --- responses --------------------------------------------------------------

// userResponse is the only projection of a domain.User that reaches a client. It has no
// password field at all, so forgetting to strip the hash cannot become a leak.
type userResponse struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"full_name"`
	Phone     string    `json:"phone"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type tokenResponse struct {
	AccessToken      string    `json:"access_token"`
	TokenType        string    `json:"token_type"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

type authResponse struct {
	User   userResponse  `json:"user"`
	Tokens tokenResponse `json:"tokens"`
}

type listUsersResponse struct {
	Users  []userResponse `json:"users"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

func newUserResponse(user domain.User) userResponse {
	return userResponse{
		ID:        user.ID,
		Email:     user.Email,
		FullName:  user.FullName,
		Phone:     user.Phone,
		Role:      user.Role.String(),
		Status:    string(user.Status),
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}

func newAuthResponse(user domain.User, pair usecase.TokenPair) authResponse {
	return authResponse{
		User: newUserResponse(user),
		Tokens: tokenResponse{
			AccessToken:      pair.AccessToken,
			TokenType:        "Bearer",
			AccessExpiresAt:  pair.AccessExpiresAt,
			RefreshToken:     pair.RefreshToken,
			RefreshExpiresAt: pair.RefreshExpiresAt,
		},
	}
}

// --- decoding ---------------------------------------------------------------

// decode reads a strict JSON body into dst. Unknown fields are an error rather than a silent
// drop: an ignored "role" or "password" key is how a client believes a field took effect.
// Every failure it returns is client-caused and maps to 400.
func (r *Responder) decode(req *http.Request, dst any) error {
	contentType := req.Header.Get("Content-Type")
	if media := strings.TrimSpace(strings.Split(contentType, ";")[0]); !strings.EqualFold(media, "application/json") {
		return domain.NewValidationError("content_type", "must be application/json")
	}

	decoder := json.NewDecoder(io.LimitReader(req.Body, maxBodyBytes))
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		return wrapDecodeError(err)
	}

	// A trailing second document is a smuggled payload, not a second value to read.
	if err := decoder.Decode(new(json.RawMessage)); !errors.Is(err, io.EOF) {
		return domain.NewValidationError("body", "must contain exactly one JSON document")
	}
	return nil
}

// wrapDecodeError names the offending field wherever the decoder knows it. The text is derived
// from the client's own body, so it is safe to echo back as a validation detail.
func wrapDecodeError(err error) error {
	var syntaxErr *json.SyntaxError
	if errors.As(err, &syntaxErr) {
		return domain.NewValidationError("body", "is not valid JSON")
	}

	// A truncated read means the limit was hit or the body ended mid-value.
	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return domain.NewValidationError("body", "is missing, truncated, or larger than the limit")
	}

	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &typeErr) {
		// A top-level mismatch (an array or a bare string instead of an object) names no field,
		// and an empty key would render as a detail that tells the client nothing.
		if typeErr.Field == "" {
			return domain.NewValidationError("body", "must be an object")
		}
		return domain.NewValidationError(typeErr.Field, "must be "+typeErr.Type.String())
	}

	// DisallowUnknownFields reports a plain error naming the key; nothing else in this path can
	// be a server fault, so it stays a 400.
	return domain.NewValidationError("body", err.Error())
}

// actor reads the identity the authentication middleware verified. A missing result means a
// route was mounted without it, which is reported as a server fault rather than guessed from
// a caller-controlled id.
func (r *Responder) actor(req *http.Request) (usecase.Actor, error) {
	actor, ok := middleware.ActorFrom(req.Context())
	if !ok {
		return usecase.Actor{}, errors.New("handler: no verified identity on the request")
	}
	return actor, nil
}

// paging reads the limit and offset query parameters. 0 means "not supplied": the use case
// owns the default and the clamp, so the transport never invents a page size.
func paging(query url.Values) (limit, offset int) {
	return optionalUint(query, "limit"), optionalUint(query, "offset")
}

func optionalUint(query url.Values, name string) int {
	raw := strings.TrimSpace(query.Get(name))
	if raw == "" {
		return 0
	}

	parsed, err := strconv.Atoi(raw)
	if err != nil || parsed < 0 {
		return 0
	}
	return parsed
}
