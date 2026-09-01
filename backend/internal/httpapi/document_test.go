package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/documents"
	"homeflow/backend/internal/receipts"
	"homeflow/backend/internal/shopping"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

type documentRepo struct{ document documents.Document }

func (r *documentRepo) Create(_ context.Context, _ string, d documents.Document) (documents.Document, error) {
	r.document = d
	return d, nil
}
func (r *documentRepo) List(context.Context, string, string, int) ([]documents.Document, error) {
	return []documents.Document{r.document}, nil
}
func (r *documentRepo) Get(_ context.Context, _, petID, documentID string) (documents.Document, error) {
	if r.document.ID != documentID || r.document.PetID != petID {
		return documents.Document{}, documents.ErrNotFound
	}
	return r.document, nil
}

func documentHandler(t *testing.T) http.Handler {
	t.Helper()
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: "00000000-0000-4000-8000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := receipts.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := documents.NewService(&documentRepo{}, storage)
	return NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil, shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Documents: service})
}

func TestDocumentUploadAndDownload(t *testing.T) {
	handler := documentHandler(t)
	petID := "00000000-0000-4000-8000-000000000010"
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, _ := writer.CreateFormFile("file", "../Befund.pdf")
	_, _ = part.Write([]byte("%PDF-1.7\nmedical"))
	_ = writer.WriteField("title", "Befund")
	_ = writer.WriteField("documentType", "report")
	_ = writer.Close()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/pets/"+petID+"/documents", &body)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", response.Code, response.Body.String())
	}
	repoID := response.Result().Header.Get("Location")
	_ = repoID
	// Obtain the id through the list response, then verify forced attachment bytes.
	list := httptest.NewRequest(http.MethodGet, "/api/v1/pets/"+petID+"/documents", nil)
	list.Header.Set("Authorization", "Bearer "+testToken)
	listed := httptest.NewRecorder()
	handler.ServeHTTP(listed, list)
	if listed.Code != http.StatusOK {
		t.Fatal(listed.Body.String())
	}
	var page struct {
		Documents []documents.Document `json:"documents"`
	}
	if err := json.Unmarshal(listed.Body.Bytes(), &page); err != nil || len(page.Documents) != 1 {
		t.Fatalf("list=%s err=%v", listed.Body.String(), err)
	}
	id := page.Documents[0].ID
	download := httptest.NewRequest(http.MethodGet, "/api/v1/pets/"+petID+"/documents/"+id+"/content", nil)
	download.Header.Set("Authorization", "Bearer "+testToken)
	got := httptest.NewRecorder()
	handler.ServeHTTP(got, download)
	if got.Code != http.StatusOK || got.Body.String() != "%PDF-1.7\nmedical" {
		t.Fatalf("download status=%d body=%q", got.Code, got.Body.String())
	}
	if got.Header().Get("Content-Disposition") == "" || got.Header().Get("Content-Security-Policy") != "default-src 'none'; sandbox" {
		t.Fatalf("headers=%v", got.Header())
	}
}
