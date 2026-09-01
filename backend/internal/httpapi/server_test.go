package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/observability"
	"homeflow/backend/internal/shopping"
)

const testToken = "0123456789abcdef0123456789abcdef"

func newTestServer(t *testing.T) http.Handler {
	t.Helper()
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: "user-1", Username: "test", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return New(logger, authenticator, shopping.NewService(shopping.NewMemoryRepository()))
}

func TestHealthDoesNotRequireAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusOK)
	}
}

func TestShoppingRequiresAuthentication(t *testing.T) {
	recorder := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/shopping/items", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusUnauthorized)
	}
}

func TestCreateRejectsTrailingJSON(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/shopping/items",
		bytes.NewBufferString(`{"forAllPets":true,"name":"Milk","quantity":"1"} true`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	recorder := httptest.NewRecorder()
	newTestServer(t).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusBadRequest)
	}
}

func TestShoppingAPIFlow(t *testing.T) {
	handler := newTestServer(t)
	body := bytes.NewBufferString(`{"forAllPets":true,"name":"Cat litter","quantity":"2","unit":"packs","category":"pet supplies"}`)
	create := httptest.NewRequest(http.MethodPost, "/api/v1/shopping/items", body)
	create.Header.Set("Authorization", "Bearer "+testToken)
	create.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	var item shopping.Item
	if err := json.Unmarshal(created.Body.Bytes(), &item); err != nil {
		t.Fatal(err)
	}

	complete := httptest.NewRequest(http.MethodPost, "/api/v1/shopping/items/"+item.ID+"/complete", nil)
	complete.Header.Set("Authorization", "Bearer "+testToken)
	completed := httptest.NewRecorder()
	handler.ServeHTTP(completed, complete)
	if completed.Code != http.StatusOK {
		t.Fatalf("complete status=%d body=%s", completed.Code, completed.Body.String())
	}

	list := httptest.NewRequest(http.MethodGet, "/api/v1/shopping/items?includeCompleted=true", nil)
	list.Header.Set("Authorization", "Bearer "+testToken)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("Cat litter")) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestMetricsUseInnerRoutePattern(t *testing.T) {
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: "user-1"})
	if err != nil {
		t.Fatal(err)
	}
	metrics := observability.New()
	handler := NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil,
		shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Metrics: metrics})

	request := httptest.NewRequest(http.MethodGet, "/api/v1/shopping/items", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	expected := `route="GET /api/v1/shopping/items"`
	if !bytes.Contains(response.Body.Bytes(), []byte(expected)) {
		t.Fatalf("metrics do not contain %s", expected)
	}
}
