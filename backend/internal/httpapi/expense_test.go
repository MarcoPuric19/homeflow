package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"homeflow/backend/internal/auth"
	"homeflow/backend/internal/expenses"
	"homeflow/backend/internal/receipts"
	"homeflow/backend/internal/shopping"
)

type expenseRepo struct{ expense expenses.Expense }

func (r *expenseRepo) Create(_ context.Context, _ string, value expenses.Expense) (expenses.Expense, error) {
	r.expense = value
	return value, nil
}
func (r *expenseRepo) List(context.Context, string, string, string, int) ([]expenses.Expense, error) {
	return []expenses.Expense{r.expense}, nil
}
func (r *expenseRepo) MarkPaid(_ context.Context, id, _ string, amount int64, at time.Time) (expenses.Expense, error) {
	value := r.expense
	value.ID = id
	value.Status = expenses.StatusPaid
	value.ActualAmountCents = &amount
	value.PaidAt = &at
	return value, nil
}
func (r *expenseRepo) MonthlySummary(_ context.Context, _ string, month string) (expenses.MonthlySummary, error) {
	return expenses.MonthlySummary{Month: month, PlannedAmountCents: r.expense.PlannedAmountCents}, nil
}
func (r *expenseRepo) Exists(context.Context, string, string) (bool, error) { return true, nil }
func (r *expenseRepo) UpsertBudget(_ context.Context, _, _ string, b expenses.Budget) (expenses.Budget, error) {
	return b, nil
}
func (r *expenseRepo) ListBudgets(context.Context, string, string, string) ([]expenses.Budget, error) {
	return []expenses.Budget{}, nil
}
func (r *expenseRepo) PetSummary(_ context.Context, _, petID, period string) (expenses.PetSummary, error) {
	return expenses.PetSummary{PetID: petID, Period: period, Categories: []expenses.CategorySummary{}}, nil
}

type receiptRepo struct{ receipt receipts.Receipt }

func (r *receiptRepo) Create(_ context.Context, value receipts.Receipt) (receipts.Receipt, error) {
	r.receipt = value
	return value, nil
}
func (r *receiptRepo) ListForExpense(context.Context, string) ([]receipts.Receipt, error) {
	return []receipts.Receipt{r.receipt}, nil
}
func (r *receiptRepo) GetForExpense(_ context.Context, expenseID, receiptID string) (receipts.Receipt, error) {
	if r.receipt.ID != receiptID || r.receipt.ExpenseID != expenseID {
		return receipts.Receipt{}, receipts.ErrNotFound
	}
	return r.receipt, nil
}

func sprint3Handler(t *testing.T) http.Handler {
	t.Helper()
	authenticator, err := auth.NewDevelopmentAuthenticator(testToken, auth.Principal{UserID: "00000000-0000-4000-8000-000000000001"})
	if err != nil {
		t.Fatal(err)
	}
	expenseService := expenses.NewService(&expenseRepo{})
	storage, err := receipts.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	receiptService := receipts.NewService(&receiptRepo{}, expenseService, storage)
	return NewFull(slog.New(slog.NewTextHandler(io.Discard, nil)), authenticator, nil,
		shopping.NewService(shopping.NewMemoryRepository()), expenseService, receiptService)
}

func TestExpenseAndReceiptAPI(t *testing.T) {
	handler := sprint3Handler(t)
	create := httptest.NewRequest(http.MethodPost, "/api/v1/expenses", bytes.NewBufferString(`{"description":"Groceries","plannedAmountCents":12000,"dueDate":"2026-09-05"}`))
	create.Header.Set("Authorization", "Bearer "+testToken)
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, create)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "../../receipt.pdf")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("%PDF-1.7\nreceipt"))
	_ = writer.WriteField("totalAmountCents", "11500")
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	upload := httptest.NewRequest(http.MethodPost, "/api/v1/expenses/00000000-0000-4000-8000-000000000010/receipts", &body)
	upload.Header.Set("Authorization", "Bearer "+testToken)
	upload.Header.Set("Content-Type", writer.FormDataContentType())
	uploaded := httptest.NewRecorder()
	handler.ServeHTTP(uploaded, upload)
	if uploaded.Code != http.StatusCreated || bytes.Contains(uploaded.Body.Bytes(), []byte("storageKey")) {
		t.Fatalf("upload status=%d body=%s", uploaded.Code, uploaded.Body.String())
	}
}

func TestPetBudgetAndSummaryAPI(t *testing.T) {
	handler := sprint3Handler(t)
	petID := "00000000-0000-4000-8000-000000000020"
	put := httptest.NewRequest(http.MethodPut, "/api/v1/pets/"+petID+"/budgets", bytes.NewBufferString(`{"month":"2026-09","category":"Tierarzt","amountCents":5000}`))
	put.Header.Set("Authorization", "Bearer "+testToken)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, put)
	if response.Code != http.StatusOK {
		t.Fatalf("budget status=%d body=%s", response.Code, response.Body.String())
	}
	get := httptest.NewRequest(http.MethodGet, "/api/v1/pets/"+petID+"/expense-summary?period=2026-09", nil)
	get.Header.Set("Authorization", "Bearer "+testToken)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, get)
	if response.Code != http.StatusOK {
		t.Fatalf("summary status=%d body=%s", response.Code, response.Body.String())
	}
}

const downloadExpenseID = "00000000-0000-4000-8000-000000000010"

// uploadReceiptForTest stores a PDF and returns the created receipt id.
func uploadReceiptForTest(t *testing.T, handler http.Handler, filename string, content []byte) string {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/expenses/"+downloadExpenseID+"/receipts", &body)
	request.Header.Set("Authorization", "Bearer "+testToken)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" {
		t.Fatal("upload returned no receipt id")
	}
	return created.ID
}

func getReceiptContent(t *testing.T, handler http.Handler, expenseID, receiptID string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/expenses/"+expenseID+"/receipts/"+receiptID+"/content", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func TestReceiptDownloadServesStoredBytesAsAttachment(t *testing.T) {
	handler := sprint3Handler(t)
	content := []byte("%PDF-1.7\nreceipt body")
	receiptID := uploadReceiptForTest(t, handler, "../../Stromrechnung \"Mai\".pdf", content)

	response := getReceiptContent(t, handler, downloadExpenseID, receiptID)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if !bytes.Equal(response.Body.Bytes(), content) {
		t.Fatalf("body=%q, want %q", response.Body.Bytes(), content)
	}
	if got := response.Header().Get("Content-Type"); got != "application/pdf" {
		t.Fatalf("Content-Type=%q", got)
	}
	disposition := response.Header().Get("Content-Disposition")
	if !strings.HasPrefix(disposition, "attachment") {
		t.Fatalf("Content-Disposition=%q must be an attachment", disposition)
	}
	// The traversal prefix is dropped at upload time and must not come back.
	if strings.Contains(disposition, "..") || strings.Contains(disposition, "/") {
		t.Fatalf("Content-Disposition=%q leaks a path", disposition)
	}
	if got := response.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options=%q", got)
	}
	if got := response.Header().Get("Content-Security-Policy"); got != "default-src 'none'; sandbox" {
		t.Fatalf("Content-Security-Policy=%q", got)
	}
}

func TestReceiptDownloadRejectsUnknownAndMalformedIDs(t *testing.T) {
	handler := sprint3Handler(t)
	receiptID := uploadReceiptForTest(t, handler, "receipt.pdf", []byte("%PDF-1.7\nx"))
	otherExpense := "00000000-0000-4000-8000-000000000011"

	for name, testCase := range map[string]struct {
		expenseID string
		receiptID string
		want      int
	}{
		"unknown receipt":            {downloadExpenseID, "00000000-0000-4000-8000-0000000000ff", http.StatusNotFound},
		"receipt of another expense": {otherExpense, receiptID, http.StatusNotFound},
		"malformed receipt id":       {downloadExpenseID, "not-a-uuid", http.StatusBadRequest},
		"malformed expense id":       {"nonsense", receiptID, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			response := getReceiptContent(t, handler, testCase.expenseID, testCase.receiptID)
			if response.Code != testCase.want {
				t.Fatalf("status=%d, want %d, body=%s", response.Code, testCase.want, response.Body.String())
			}
		})
	}
}

func TestReceiptListRejectsMalformedExpenseID(t *testing.T) {
	handler := sprint3Handler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/expenses/nonsense/receipts", nil)
	request.Header.Set("Authorization", "Bearer "+testToken)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status=%d, want 400", recorder.Code)
	}
}

func TestReceiptDownloadRequiresAuthentication(t *testing.T) {
	handler := sprint3Handler(t)
	receiptID := uploadReceiptForTest(t, handler, "receipt.pdf", []byte("%PDF-1.7\nx"))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/expenses/"+downloadExpenseID+"/receipts/"+receiptID+"/content", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d, want 401", recorder.Code)
	}
}

func TestPinnedMIMEType(t *testing.T) {
	for stored, want := range map[string]string{
		"image/jpeg":               "image/jpeg",
		"image/png":                "image/png",
		"application/pdf":          "application/pdf",
		"text/html":                "application/octet-stream",
		"image/svg+xml":            "application/octet-stream",
		"":                         "application/octet-stream",
		"application/pdf; charset": "application/octet-stream",
	} {
		if got := pinnedMIMEType(stored); got != want {
			t.Fatalf("pinnedMIMEType(%q)=%q, want %q", stored, got, want)
		}
	}
}

func TestAttachmentDisposition(t *testing.T) {
	for name, filename := range map[string]string{
		"quotes":    `re"ceipt".pdf`,
		"semicolon": `receipt; rm -rf.pdf`,
		"umlaut":    "Stromrechnung März.pdf",
		"newline":   "receipt\n.pdf",
		"empty":     "",
	} {
		t.Run(name, func(t *testing.T) {
			got := attachmentDisposition(filename)
			if !strings.HasPrefix(got, "attachment") {
				t.Fatalf("disposition=%q", got)
			}
			if strings.ContainsAny(got, "\r\n") {
				t.Fatalf("disposition=%q must not contain a line break", got)
			}
		})
	}
}
