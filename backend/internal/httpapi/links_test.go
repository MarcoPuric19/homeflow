package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/links"
	"homeflow/backend/internal/shopping"
)

type linkRepo struct{ set links.Set }

func (r *linkRepo) GetDocument(context.Context, string, string, string) (links.Set, error) {
	return r.set, nil
}
func (r *linkRepo) ReplaceDocument(_ context.Context, _ string, _ string, _ string, set links.Set) (links.Set, error) {
	r.set = set
	return set, nil
}
func (r *linkRepo) GetExpense(context.Context, string, string) (links.Set, error) { return r.set, nil }
func (r *linkRepo) ReplaceExpense(_ context.Context, _ string, _ string, set links.Set) (links.Set, error) {
	r.set = set
	return set, nil
}

func TestRelationshipRoutesRequireAuthAndReplaceSet(t *testing.T) {
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: "00000000-0000-4000-8000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	repo := &linkRepo{}
	service := links.NewService(repo)
	handler := NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil, shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Links: service})
	id1, id2 := "00000000-0000-4000-8000-000000000010", "00000000-0000-4000-8000-000000000011"
	request := httptest.NewRequest(http.MethodPut, "/api/v1/expenses/"+id1+"/pet-links", strings.NewReader(`{"medicalRecordIds":["`+id2+`"],"appointmentIds":[]}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || len(repo.set.MedicalRecordIDs) != 1 {
		t.Fatalf("status=%d body=%s set=%#v", response.Code, response.Body.String(), repo.set)
	}
	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "/api/v1/expenses/"+id1+"/pet-links", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d", unauthenticated.Code)
	}
}
