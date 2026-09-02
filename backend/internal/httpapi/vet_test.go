package httpapi

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/shopping"
	"homeflow/backend/internal/vets"
)

type referencedVetRepository struct{}

func (referencedVetRepository) Create(context.Context, string, vets.Veterinarian) (vets.Veterinarian, error) {
	return vets.Veterinarian{}, nil
}
func (referencedVetRepository) List(context.Context, string, string, int, string) (vets.Page, error) {
	return vets.Page{}, nil
}
func (referencedVetRepository) Get(context.Context, string, string) (vets.Veterinarian, error) {
	return vets.Veterinarian{}, vets.ErrNotFound
}
func (referencedVetRepository) Update(context.Context, string, vets.Veterinarian) (vets.Veterinarian, error) {
	return vets.Veterinarian{}, nil
}
func (referencedVetRepository) Delete(context.Context, string, string) error {
	return vets.ErrReferenced
}
func (referencedVetRepository) Candidates(context.Context, string, string) ([]vets.Veterinarian, error) {
	return nil, nil
}

func TestDeleteReferencedVeterinarianReturnsConflict(t *testing.T) {
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: aliceID, Username: "alice", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil, shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Vets: vets.NewService(referencedVetRepository{})})
	request := httptest.NewRequest(http.MethodDelete, "/api/v1/veterinarians/00000000-0000-4000-8000-000000000001", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
