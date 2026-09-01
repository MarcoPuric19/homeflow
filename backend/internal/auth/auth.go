package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
)

type Principal struct {
	UserID   string
	Username string
	Role     string
}

type Authenticator interface {
	Authenticate(ctx context.Context, bearerToken string) (Principal, error)
}

var ErrUnauthenticated = errors.New("unauthenticated")

type DevelopmentAuthenticator struct {
	tokenHash [32]byte
	principal Principal
}

func NewDevelopmentAuthenticator(token string, principal Principal) (*DevelopmentAuthenticator, error) {
	if len(token) < 32 {
		return nil, errors.New("development token must contain at least 32 characters")
	}
	return &DevelopmentAuthenticator{tokenHash: sha256.Sum256([]byte(token)), principal: principal}, nil
}

func (a *DevelopmentAuthenticator) Authenticate(_ context.Context, token string) (Principal, error) {
	got := sha256.Sum256([]byte(token))
	if subtle.ConstantTimeCompare(got[:], a.tokenHash[:]) != 1 {
		return Principal{}, ErrUnauthenticated
	}
	return a.principal, nil
}

type contextKey struct{}
type browserSessionKey struct{}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func BrowserSessionFromContext(ctx context.Context) bool {
	value, _ := ctx.Value(browserSessionKey{}).(bool)
	return value
}

func Middleware(authenticator Authenticator, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeUnauthorized(w)
			return
		}
		principal, err := authenticator.Authenticate(r.Context(), strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			writeUnauthorized(w)
			return
		}
		authenticated := r.WithContext(context.WithValue(r.Context(), contextKey{}, principal))
		next.ServeHTTP(w, authenticated)
		r.Pattern = authenticated.Pattern
	})
}

func BrowserMiddleware(authenticator Authenticator, accessCookieName string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
			principal, err := authenticator.Authenticate(r.Context(), strings.TrimPrefix(header, "Bearer "))
			if err != nil {
				writeUnauthorized(w)
				return
			}
			authenticated := r.WithContext(context.WithValue(r.Context(), contextKey{}, principal))
			next.ServeHTTP(w, authenticated)
			r.Pattern = authenticated.Pattern
			return
		}

		cookie, err := r.Cookie(accessCookieName)
		if err != nil || cookie.Value == "" {
			writeUnauthorized(w)
			return
		}
		principal, err := authenticator.Authenticate(r.Context(), cookie.Value)
		if err != nil {
			writeUnauthorized(w)
			return
		}
		ctx := context.WithValue(r.Context(), contextKey{}, principal)
		ctx = context.WithValue(ctx, browserSessionKey{}, true)
		authenticated := r.WithContext(ctx)
		next.ServeHTTP(w, authenticated)
		r.Pattern = authenticated.Pattern
	})
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("WWW-Authenticate", "Bearer")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte(`{"type":"about:blank","title":"Unauthorized","status":401}` + "\n"))
}
