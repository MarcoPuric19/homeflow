package httpapi

import (
	"bytes"
	"context"
	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/calendar"
	"homeflow/backend/internal/reminders"
	"homeflow/backend/internal/shopping"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type reminderRepo struct{}

func (reminderRepo) GetRules(context.Context, string) (reminders.Rules, error) {
	return reminders.Rules{LeadDays: []int{30, 7, 1}}, nil
}
func (reminderRepo) ReplaceRules(_ context.Context, _ string, values []int) (reminders.Rules, error) {
	return reminders.Rules{LeadDays: values}, nil
}
func (reminderRepo) CreateAppointment(_ context.Context, _ string, _ string, event calendar.Event) (calendar.Event, error) {
	petID := "00000000-0000-4000-8000-000000000099"
	event.PetID = &petID
	return event, nil
}

func (reminderRepo) List(_ context.Context, _ string, _, _ time.Time, _ int) ([]reminders.Reminder, error) {
	return []reminders.Reminder{{ID: "vaccination:x", Kind: "vaccination", PetID: "pet", Title: "Impfung fällig", DueDate: "2026-09-10", DaysUntil: 9}}, nil
}

func TestReminderRuleAPI(t *testing.T) {
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: aliceID})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil, shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Reminders: reminders.NewService(reminderRepo{})})
	for _, method := range []string{http.MethodGet, http.MethodPut} {
		request := httptest.NewRequest(method, "/api/v1/reminder-rules", bytes.NewBufferString(`{"leadDays":[30,7,1]}`))
		request.Header.Set("Authorization", "Bearer "+testToken)
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("method=%s status=%d body=%s", method, response.Code, response.Body.String())
		}
	}
}
func TestReminderAPI(t *testing.T) {
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: aliceID})
	if err != nil {
		t.Fatal(err)
	}
	service := reminders.NewService(reminderRepo{})
	handler := NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil, shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Reminders: service})
	until := time.Now().UTC().AddDate(0, 0, 30).Format(time.DateOnly)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/reminders?until="+until, nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestCreateReminderAppointmentAPI(t *testing.T) {
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: aliceID})
	if err != nil {
		t.Fatal(err)
	}
	handler := NewFullWithOptions(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil, shopping.NewService(shopping.NewMemoryRepository()), nil, nil, BrowserCookieConfig{Reminders: reminders.NewService(reminderRepo{})})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/reminders/vaccinations/00000000-0000-4000-8000-000000000010/appointment", bytes.NewBufferString(`{"title":"Impftermin","startsAt":"2026-09-10T09:00:00+02:00","endsAt":"2026-09-10T10:00:00+02:00","visibility":"shared"}`))
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusCreated || !bytes.Contains(response.Body.Bytes(), []byte(`"appointmentType":"vaccination"`)) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
