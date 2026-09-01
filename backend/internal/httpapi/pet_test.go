package httpapi

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/pets"
	"homeflow/backend/internal/shopping"
)

func petHandler(t *testing.T) http.Handler {
	t.Helper()
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: aliceID, Username: "alice", Role: "admin"})
	if err != nil {
		t.Fatal(err)
	}
	return NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil,
		shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Pets: pets.NewService(pets.NewMemoryRepository())})
}

func petCall(t *testing.T, handler http.Handler, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	request.Header.Set("Authorization", "Bearer "+testToken)
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestPetAPIFlow(t *testing.T) {
	handler := petHandler(t)
	created := petCall(t, handler, http.MethodPost, "/api/v1/pets", `{"name":"Momo","species":"Katze","sex":"female"}`)
	if created.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", created.Code, created.Body.String())
	}
	location := created.Header().Get("Location")
	listed := petCall(t, handler, http.MethodGet, "/api/v1/pets?limit=50", "")
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("Momo")) {
		t.Fatalf("status=%d body=%s", listed.Code, listed.Body.String())
	}
	got := petCall(t, handler, http.MethodGet, location, "")
	if got.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", got.Code, got.Body.String())
	}
	updated := petCall(t, handler, http.MethodPatch, location, `{"name":"Momo","species":"Katze","sex":"female","active":false}`)
	if updated.Code != http.StatusOK || !bytes.Contains(updated.Body.Bytes(), []byte(`"active":false`)) {
		t.Fatalf("status=%d body=%s", updated.Code, updated.Body.String())
	}
	record := petCall(t, handler, http.MethodPost, location+"/medical-records", `{"recordType":"examination","occurredOn":"2026-08-31","title":"Jahreskontrolle","summary":"Vom Tierarzt dokumentiert"}`)
	if record.Code != http.StatusCreated {
		t.Fatalf("record status=%d body=%s", record.Code, record.Body.String())
	}
	records := petCall(t, handler, http.MethodGet, location+"/medical-records?limit=30", "")
	if records.Code != http.StatusOK || !bytes.Contains(records.Body.Bytes(), []byte("Jahreskontrolle")) {
		t.Fatalf("records status=%d body=%s", records.Code, records.Body.String())
	}
	vaccination := petCall(t, handler, http.MethodPost, location+"/vaccinations", `{"vaccineName":"Tollwut","administeredOn":"2026-08-30","nextDueDate":"2027-08-30"}`)
	if vaccination.Code != http.StatusCreated {
		t.Fatalf("vaccination status=%d body=%s", vaccination.Code, vaccination.Body.String())
	}
	vaccinations := petCall(t, handler, http.MethodGet, location+"/vaccinations?limit=30", "")
	if vaccinations.Code != http.StatusOK || !bytes.Contains(vaccinations.Body.Bytes(), []byte("Tollwut")) {
		t.Fatalf("vaccinations status=%d body=%s", vaccinations.Code, vaccinations.Body.String())
	}
	medication := petCall(t, handler, http.MethodPost, location+"/medications", `{"name":"Mittel","doseText":"laut Tierarzt","frequencyText":"morgens","startsOn":"2026-08-31","stockQuantity":"12.500","stockUnit":"Tabletten"}`)
	if medication.Code != http.StatusCreated {
		t.Fatalf("medication status=%d body=%s", medication.Code, medication.Body.String())
	}
	medications := petCall(t, handler, http.MethodGet, location+"/medications?limit=100", "")
	if medications.Code != http.StatusOK || !bytes.Contains(medications.Body.Bytes(), []byte("12.500")) {
		t.Fatalf("medications status=%d body=%s", medications.Code, medications.Body.String())
	}
	weight := petCall(t, handler, http.MethodPost, location+"/weights", `{"measuredOn":"2026-08-31","weightKg":"4.250"}`)
	if weight.Code != http.StatusCreated || !bytes.Contains(weight.Body.Bytes(), []byte("4.250")) {
		t.Fatalf("weight status=%d body=%s", weight.Code, weight.Body.String())
	}
	lab := petCall(t, handler, http.MethodPost, location+"/lab-results", `{"measuredOn":"2026-08-31","testName":"Glukose","value":"5.5000","unit":"mmol/L"}`)
	if lab.Code != http.StatusCreated || !bytes.Contains(lab.Body.Bytes(), []byte("5.5000")) {
		t.Fatalf("lab status=%d body=%s", lab.Code, lab.Body.String())
	}
	dashboard := petCall(t, handler, http.MethodGet, location+"/dashboard?month=2026-09", "")
	if dashboard.Code != http.StatusOK || !bytes.Contains(dashboard.Body.Bytes(), []byte(`"month":"2026-09"`)) {
		t.Fatalf("dashboard status=%d body=%s", dashboard.Code, dashboard.Body.String())
	}
	timeline := petCall(t, handler, http.MethodGet, location+"/timeline?limit=2", "")
	if timeline.Code != http.StatusOK || !bytes.Contains(timeline.Body.Bytes(), []byte(`"items"`)) || !bytes.Contains(timeline.Body.Bytes(), []byte(`"nextCursor"`)) {
		t.Fatalf("timeline status=%d body=%s", timeline.Code, timeline.Body.String())
	}
}

func TestPetAPIRejectsMalformedIDsAndUnboundedLists(t *testing.T) {
	handler := petHandler(t)
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets/not-a-uuid", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("id status=%d", response.Code)
	}
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets?limit=101", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("limit status=%d", response.Code)
	}
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets/not-a-uuid/medical-records", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("record id status=%d", response.Code)
	}
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets/not-a-uuid/vaccinations", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("vaccination id status=%d", response.Code)
	}
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets/not-a-uuid/medications", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("medication id status=%d", response.Code)
	}
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets/not-a-uuid/timeline", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("timeline id status=%d", response.Code)
	}
	if response := petCall(t, handler, http.MethodGet, "/api/v1/pets/00000000-0000-4000-8000-000000000010/timeline?limit=101", ""); response.Code != http.StatusBadRequest {
		t.Fatalf("timeline limit status=%d", response.Code)
	}
}
