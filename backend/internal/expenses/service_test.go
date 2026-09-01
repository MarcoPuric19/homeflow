package expenses

import (
	"context"
	"testing"
	"time"
)

type fakeRepository struct{ created Expense }

func (r *fakeRepository) Create(_ context.Context, _ string, expense Expense) (Expense, error) {
	r.created = expense
	return expense, nil
}
func (r *fakeRepository) List(context.Context, string, string, string, int) ([]Expense, error) {
	return []Expense{r.created}, nil
}
func (r *fakeRepository) MarkPaid(_ context.Context, id, _ string, amount int64, paidAt time.Time) (Expense, error) {
	expense := r.created
	expense.ID = id
	expense.Status = StatusPaid
	expense.ActualAmountCents = &amount
	expense.PaidAt = &paidAt
	return expense, nil
}
func (r *fakeRepository) MonthlySummary(_ context.Context, _ string, month string) (MonthlySummary, error) {
	return MonthlySummary{Month: month}, nil
}
func (r *fakeRepository) Exists(context.Context, string, string) (bool, error) { return true, nil }
func (r *fakeRepository) UpsertBudget(_ context.Context, _, _ string, budget Budget) (Budget, error) {
	return budget, nil
}
func (r *fakeRepository) ListBudgets(context.Context, string, string, string) ([]Budget, error) {
	return nil, nil
}
func (r *fakeRepository) PetSummary(_ context.Context, _, petID, period string) (PetSummary, error) {
	return PetSummary{PetID: petID, Period: period}, nil
}

func TestCreateAndPayExpense(t *testing.T) {
	repository := &fakeRepository{}
	service := NewService(repository)
	now := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	expense, err := service.Create(context.Background(), CreateExpense{Description: "Rent", PlannedAmountCents: 120000, DueDate: "2026-09-01"}, "user-1")
	if err != nil {
		t.Fatal(err)
	}
	if expense.Currency != "EUR" || expense.Status != StatusPlanned || expense.PlannedAmountCents != 120000 {
		t.Fatalf("unexpected expense: %#v", expense)
	}
	paid, err := service.MarkPaid(context.Background(), expense.ID, "user-1", MarkPaid{ActualAmountCents: 120000})
	if err != nil {
		t.Fatal(err)
	}
	if paid.Status != StatusPaid || paid.ActualAmountCents == nil {
		t.Fatalf("unexpected paid expense: %#v", paid)
	}
}

func TestBudgetAndSummaryValidation(t *testing.T) {
	service := NewService(&fakeRepository{})
	petID := "00000000-0000-4000-8000-000000000010"
	threshold := int64(1000)
	budget, err := service.UpsertBudget(context.Background(), "user", petID, BudgetInput{Month: "2026-09", Category: "Tierarzt", AmountCents: 5000, WarningThresholdCents: &threshold, Notes: " Nur intern "})
	if err != nil || budget.AmountCents != 5000 || budget.Currency != "EUR" || budget.WarningThresholdCents == nil || *budget.WarningThresholdCents != 1000 || budget.Notes != "Nur intern" {
		t.Fatalf("budget=%#v err=%v", budget, err)
	}
	negative := int64(-1)
	if _, err := service.UpsertBudget(context.Background(), "user", petID, BudgetInput{Month: "2026-09", Category: "Tierarzt", AmountCents: 5000, WarningThresholdCents: &negative}); err != ErrInvalid {
		t.Fatalf("negative threshold err=%v", err)
	}
	if _, err := service.PetSummary(context.Background(), "user", petID, "2026"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.PetSummary(context.Background(), "user", petID, "all"); err != ErrInvalid {
		t.Fatalf("err=%v", err)
	}
}

func TestRejectInvalidExpense(t *testing.T) {
	service := NewService(&fakeRepository{})
	for _, input := range []CreateExpense{
		{Description: "", PlannedAmountCents: 1, DueDate: "2026-09-01"},
		{Description: "Bad", PlannedAmountCents: -1, DueDate: "2026-09-01"},
		{Description: "Bad", PlannedAmountCents: 1, DueDate: "09/01/2026"},
	} {
		if _, err := service.Create(context.Background(), input, "user"); err != ErrInvalid {
			t.Fatalf("input=%#v err=%v", input, err)
		}
	}
}
