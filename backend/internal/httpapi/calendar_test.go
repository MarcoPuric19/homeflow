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
	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/shopping"
)

const (
	aliceID    = "00000000-0000-4000-8000-000000000001"
	bobID      = "00000000-0000-4000-8000-000000000002"
	aliceToken = "0123456789abcdef0123456789abcdefalice"
	bobToken   = "0123456789abcdef0123456789abcdefbobbb"
	// A range wide enough for every event these tests create.
	septemberQuery = "?from=2026-09-01&to=2026-10-01"
)

// calendarHandlers returns one handler per household member, both backed by the
// same repository, so the visibility rules can be exercised across users.
func calendarHandlers(t *testing.T) (http.Handler, http.Handler) {
	t.Helper()
	repository := calendar.NewMemoryRepository()
	build := func(token, userID string) http.Handler {
		authenticator, err := auth.NewDevelopmentAuthenticator(token, auth.Principal{UserID: userID, Username: userID, Role: "member"})
		if err != nil {
			t.Fatal(err)
		}
		return NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil,
			shopping.NewService(shopping.NewMemoryRepository()), nil, nil,
			BrowserCookieConfig{Calendar: calendar.NewService(repository)})
	}
	return build(aliceToken, aliceID), build(bobToken, bobID)
}

func callCalendar(t *testing.T, handler http.Handler, token, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewBufferString(body)
	}
	request := httptest.NewRequest(method, target, reader)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		request.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func createEvent(t *testing.T, handler http.Handler, token, body string) calendar.Event {
	t.Helper()
	response := callCalendar(t, handler, token, http.MethodPost, "/api/v1/calendar/events", body)
	if response.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", response.Code, response.Body.String())
	}
	var event calendar.Event
	if err := json.Unmarshal(response.Body.Bytes(), &event); err != nil {
		t.Fatal(err)
	}
	return event
}

func TestCalendarCreateListAndDelete(t *testing.T) {
	alice, _ := calendarHandlers(t)
	event := createEvent(t, alice, aliceToken,
		`{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"Zahnarzt","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z","category":"Gesundheit"}`)
	if event.OwnerID != aliceID || event.Visibility != calendar.VisibilityShared || event.Status != calendar.StatusPlanned {
		t.Fatalf("event=%#v", event)
	}

	list := callCalendar(t, alice, aliceToken, http.MethodGet, "/api/v1/calendar/events"+septemberQuery, "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", list.Code, list.Body.String())
	}
	var listed struct {
		Events []calendar.Event `json:"events"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if len(listed.Events) != 1 || listed.Events[0].ID != event.ID {
		t.Fatalf("events=%#v", listed.Events)
	}

	deleted := callCalendar(t, alice, aliceToken, http.MethodDelete, "/api/v1/calendar/events/"+event.ID, "")
	if deleted.Code != http.StatusNoContent || deleted.Body.Len() != 0 {
		t.Fatalf("delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	gone := callCalendar(t, alice, aliceToken, http.MethodGet, "/api/v1/calendar/events/"+event.ID, "")
	if gone.Code != http.StatusNotFound {
		t.Fatalf("status after delete=%d, want 404", gone.Code)
	}
}

func TestCalendarUpdateKeepsTheOwner(t *testing.T) {
	alice, bob := calendarHandlers(t)
	event := createEvent(t, alice, aliceToken,
		`{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"Müllabfuhr","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z"}`)

	// A shared event belongs to the household, so the other member may edit it.
	updated := callCalendar(t, bob, bobToken, http.MethodPut, "/api/v1/calendar/events/"+event.ID,
		`{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"Müllabfuhr verschoben","startsAt":"2026-09-11T09:00:00Z","endsAt":"2026-09-11T10:00:00Z","status":"completed"}`)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var result calendar.Event
	if err := json.Unmarshal(updated.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.OwnerID != aliceID {
		t.Fatalf("ownerId=%q, an update must not transfer ownership", result.OwnerID)
	}
	if result.Title != "Müllabfuhr verschoben" || result.Status != calendar.StatusCompleted {
		t.Fatalf("event=%#v", result)
	}
}

func TestCalendarPrivateEventIsInvisibleToTheOtherMember(t *testing.T) {
	alice, bob := calendarHandlers(t)
	private := createEvent(t, alice, aliceToken,
		`{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"Überraschung","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z","visibility":"private"}`)

	list := callCalendar(t, bob, bobToken, http.MethodGet, "/api/v1/calendar/events"+septemberQuery, "")
	if list.Code != http.StatusOK {
		t.Fatalf("list status=%d", list.Code)
	}
	if bytes.Contains(list.Body.Bytes(), []byte("Überraschung")) {
		t.Fatalf("private event leaked into the other member's list: %s", list.Body.String())
	}

	for _, request := range []struct {
		method string
		body   string
	}{
		{http.MethodGet, ""},
		{http.MethodPut, `{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"Geklaut","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z"}`},
		{http.MethodDelete, ""},
	} {
		response := callCalendar(t, bob, bobToken, request.method, "/api/v1/calendar/events/"+private.ID, request.body)
		// 404 and not 403: the response must not confirm that the event exists.
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status=%d, want 404", request.method, response.Code)
		}
		if bytes.Contains(response.Body.Bytes(), []byte("Überraschung")) {
			t.Fatalf("%s leaked the event title", request.method)
		}
	}

	still := callCalendar(t, alice, aliceToken, http.MethodGet, "/api/v1/calendar/events/"+private.ID, "")
	if still.Code != http.StatusOK {
		t.Fatalf("owner lost access: status=%d", still.Code)
	}
}

func TestCalendarRejectsBadRequests(t *testing.T) {
	alice, _ := calendarHandlers(t)
	event := createEvent(t, alice, aliceToken,
		`{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"Zahnarzt","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z"}`)

	cases := map[string]struct {
		method string
		target string
		body   string
		want   int
	}{
		"missing range":    {http.MethodGet, "/api/v1/calendar/events", "", http.StatusBadRequest},
		"unparsable range": {http.MethodGet, "/api/v1/calendar/events?from=gestern&to=morgen", "", http.StatusBadRequest},
		"inverted range":   {http.MethodGet, "/api/v1/calendar/events?from=2026-10-01&to=2026-09-01", "", http.StatusBadRequest},
		"range too wide":   {http.MethodGet, "/api/v1/calendar/events?from=2020-01-01&to=2026-01-01", "", http.StatusBadRequest},
		"empty title":      {http.MethodPost, "/api/v1/calendar/events", `{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"  ","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z"}`, http.StatusUnprocessableEntity},
		"end before start": {http.MethodPost, "/api/v1/calendar/events", `{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"X","startsAt":"2026-09-10T10:00:00Z","endsAt":"2026-09-10T09:00:00Z"}`, http.StatusUnprocessableEntity},
		"unknown field":    {http.MethodPost, "/api/v1/calendar/events", `{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"X","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z","ownerId":"me"}`, http.StatusBadRequest},
		"broken json":      {http.MethodPost, "/api/v1/calendar/events", `{`, http.StatusBadRequest},
		"malformed get id": {http.MethodGet, "/api/v1/calendar/events/nonsense", "", http.StatusBadRequest},
		"malformed put id": {http.MethodPut, "/api/v1/calendar/events/nonsense", `{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"X","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z"}`, http.StatusBadRequest},
		"malformed del id": {http.MethodDelete, "/api/v1/calendar/events/nonsense", "", http.StatusBadRequest},
		"unknown event":    {http.MethodGet, "/api/v1/calendar/events/00000000-0000-4000-8000-0000000000ff", "", http.StatusNotFound},
		"invalid update":   {http.MethodPut, "/api/v1/calendar/events/" + event.ID, `{"petId":"9f1d2c3b-4a5e-4f60-8123-abcdefabcdef","title":"","startsAt":"2026-09-10T09:00:00Z","endsAt":"2026-09-10T10:00:00Z"}`, http.StatusUnprocessableEntity},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			response := callCalendar(t, alice, aliceToken, testCase.method, testCase.target, testCase.body)
			if response.Code != testCase.want {
				t.Fatalf("status=%d, want %d, body=%s", response.Code, testCase.want, response.Body.String())
			}
		})
	}
}

func TestCalendarRequiresAuthentication(t *testing.T) {
	alice, _ := calendarHandlers(t)
	for _, target := range []string{
		"/api/v1/calendar/events" + septemberQuery,
		"/api/v1/calendar/events/00000000-0000-4000-8000-000000000010",
	} {
		if response := callCalendar(t, alice, "", http.MethodGet, target, ""); response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status=%d, want 401", target, response.Code)
		}
	}
}

func TestCalendarRoutesAreAbsentWithoutTheService(t *testing.T) {
	// The calendar is optional in the same way expenses and receipts are; a
	// server built without it must not answer the routes at all.
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: aliceID})
	if err != nil {
		t.Fatal(err)
	}
	handler := New(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator,
		shopping.NewService(shopping.NewMemoryRepository()))
	response := callCalendar(t, handler, testToken, http.MethodGet, "/api/v1/calendar/events"+septemberQuery, "")
	if response.Code != http.StatusNotFound {
		t.Fatalf("status=%d, want 404", response.Code)
	}
}
