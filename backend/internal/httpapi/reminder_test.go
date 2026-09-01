package httpapi

import (
	"context"
	"homeflow/backend/internal/auth"
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

func (reminderRepo) List(_ context.Context, _ string, _, _ time.Time, _ int) ([]reminders.Reminder, error) {
	return []reminders.Reminder{{ID: "vaccination:x", Kind: "vaccination", PetID: "pet", Title: "Impfung fällig", DueDate: "2026-09-10", DaysUntil: 9}}, nil
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
