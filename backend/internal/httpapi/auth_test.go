package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/shopping"
)

type fakeSessions struct {
	loginError error
	logins     int
}

func (f *fakeSessions) Login(_ context.Context, _ auth.Credentials, _ string) (auth.TokenPair, error) {
	f.logins++
	if f.loginError != nil {
		return auth.TokenPair{}, f.loginError
	}
	return auth.TokenPair{AccessToken: "access", RefreshToken: "refresh", TokenType: "Bearer",
		AccessExpiresAt: time.Now().Add(time.Minute), RefreshExpiresAt: time.Now().Add(time.Hour)}, nil
}
func (f *fakeSessions) Refresh(context.Context, string, string) (auth.TokenPair, error) {
	return auth.TokenPair{AccessToken: "rotated", RefreshToken: "rotated-refresh", TokenType: "Bearer"}, nil
}
func (f *fakeSessions) Logout(context.Context, string, string) error { return nil }

func authServer(t *testing.T, sessions SessionService) http.Handler {
	t.Helper()
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	return NewWithSessions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, sessions,
		shopping.NewService(shopping.NewMemoryRepository()))
}

func TestLoginAndRefreshEndpoints(t *testing.T) {
	sessions := &fakeSessions{}
	handler := authServer(t, sessions)
	login := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	login.RemoteAddr = "192.0.2.1:1234"
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, login)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "accessToken") {
		t.Fatalf("login status=%d body=%s", response.Code, response.Body.String())
	}
	refresh := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", strings.NewReader(`{"refreshToken":"refresh"}`))
	refreshResponse := httptest.NewRecorder()
	handler.ServeHTTP(refreshResponse, refresh)
	if refreshResponse.Code != http.StatusOK || !strings.Contains(refreshResponse.Body.String(), "rotated") {
		t.Fatalf("refresh status=%d body=%s", refreshResponse.Code, refreshResponse.Body.String())
	}
}

func TestLoginRateLimit(t *testing.T) {
	sessions := &fakeSessions{loginError: auth.ErrUnauthenticated}
	handler := authServer(t, sessions)
	for i := 0; i < 6; i++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"username":"alice","password":"wrong"}`))
		request.RemoteAddr = "192.0.2.5:1234"
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if i < 5 && response.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d status=%d", i+1, response.Code)
		}
		if i == 5 && response.Code != http.StatusTooManyRequests {
			t.Fatalf("rate-limited status=%d", response.Code)
		}
	}
	if sessions.logins != 5 {
		t.Fatalf("backend login calls=%d, want 5", sessions.logins)
	}
}

func TestBrowserLoginSetsProtectedCookies(t *testing.T) {
	handler := authServer(t, &fakeSessions{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/browser/login", strings.NewReader(`{"username":"alice","password":"secret"}`))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 3 {
		t.Fatalf("cookies=%d, want 3", len(cookies))
	}
	for _, cookie := range cookies {
		if cookie.SameSite != http.SameSiteStrictMode {
			t.Fatalf("cookie %s is not SameSite=Strict", cookie.Name)
		}
		if cookie.Name != csrfCookie && !cookie.HttpOnly {
			t.Fatalf("cookie %s must be HttpOnly", cookie.Name)
		}
	}
}

func TestBrowserMutationRequiresMatchingCSRFToken(t *testing.T) {
	handler := authServer(t, &fakeSessions{})
	body := `{"name":"Milk","quantity":"1","priority":"normal"}`
	request := httptest.NewRequest(http.MethodPost, "/api/v1/shopping/items", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: accessCookie, Value: testToken})
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: "expected"})
	denied := httptest.NewRecorder()
	handler.ServeHTTP(denied, request)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("without header status=%d, want %d", denied.Code, http.StatusForbidden)
	}

	request = httptest.NewRequest(http.MethodPost, "/api/v1/shopping/items", strings.NewReader(body))
	request.AddCookie(&http.Cookie{Name: accessCookie, Value: testToken})
	request.AddCookie(&http.Cookie{Name: csrfCookie, Value: "expected"})
	request.Header.Set("X-CSRF-Token", "expected")
	allowed := httptest.NewRecorder()
	handler.ServeHTTP(allowed, request)
	if allowed.Code != http.StatusCreated {
		t.Fatalf("with header status=%d body=%s", allowed.Code, allowed.Body.String())
	}
}
