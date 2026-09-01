package expenses

import (
	"context"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var (
	ErrInvalid  = errors.New("invalid expense")
	ErrNotFound = errors.New("expense not found")
	ErrConflict = errors.New("expense state conflict")
)

type Repository interface {
	Create(context.Context, string, Expense) (Expense, error)
	List(context.Context, string, string, string, int) ([]Expense, error)
	MarkPaid(context.Context, string, string, int64, time.Time) (Expense, error)
	MonthlySummary(context.Context, string, string) (MonthlySummary, error)
	Exists(context.Context, string, string) (bool, error)
	UpsertBudget(context.Context, string, string, Budget) (Budget, error)
	ListBudgets(context.Context, string, string, string) ([]Budget, error)
	PetSummary(context.Context, string, string, string) (PetSummary, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, input CreateExpense, userID string) (Expense, error) {
	input.Description = strings.TrimSpace(input.Description)
	input.BudgetCategory = strings.TrimSpace(input.BudgetCategory)
	if input.Description == "" || len(input.Description) > 300 || input.PlannedAmountCents < 0 || len(input.BudgetCategory) > 100 || !id.IsUUID(strings.TrimSpace(input.PetID)) {
		return Expense{}, ErrInvalid
	}
	if input.Currency == "" {
		input.Currency = "EUR"
	}
	input.Currency = strings.ToUpper(input.Currency)
	if len(input.Currency) != 3 {
		return Expense{}, ErrInvalid
	}
	if _, err := time.Parse(time.DateOnly, input.DueDate); err != nil {
		return Expense{}, ErrInvalid
	}
	if input.Status == "" {
		input.Status = StatusPlanned
	}
	if input.Status != StatusPlanned && input.Status != StatusOpen {
		return Expense{}, ErrInvalid
	}
	expenseID, err := id.New()
	if err != nil {
		return Expense{}, err
	}
	now := s.now()
	return s.repository.Create(ctx, userID, Expense{ID: expenseID, Description: input.Description,
		Merchant: input.Merchant, PlannedAmountCents: input.PlannedAmountCents,
		Currency: input.Currency, DueDate: input.DueDate, Category: input.Category,
		RecurrenceRule: input.RecurrenceRule, Status: input.Status, UserID: userID,
		PaymentMethod: input.PaymentMethod, Notes: input.Notes, PetID: input.PetID, BudgetCategory: input.BudgetCategory, CreatedAt: now, UpdatedAt: now})
}

func (s *Service) List(ctx context.Context, userID, month, petID string, limit int) ([]Expense, error) {
	if month != "" {
		if _, err := time.Parse("2006-01", month); err != nil {
			return nil, ErrInvalid
		}
	}
	if petID != "" && !id.IsUUID(petID) {
		return nil, ErrInvalid
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	return s.repository.List(ctx, userID, month, petID, limit)
}

func (s *Service) MarkPaid(ctx context.Context, expenseID, userID string, input MarkPaid) (Expense, error) {
	if expenseID == "" || input.ActualAmountCents < 0 {
		return Expense{}, ErrInvalid
	}
	if input.PaidAt.IsZero() {
		input.PaidAt = s.now()
	}
	return s.repository.MarkPaid(ctx, expenseID, userID, input.ActualAmountCents, input.PaidAt.UTC())
}

func (s *Service) MonthlySummary(ctx context.Context, userID, month string) (MonthlySummary, error) {
	if _, err := time.Parse("2006-01", month); err != nil {
		return MonthlySummary{}, ErrInvalid
	}
	return s.repository.MonthlySummary(ctx, userID, month)
}

func (s *Service) Exists(ctx context.Context, userID, expenseID string) (bool, error) {
	return s.repository.Exists(ctx, userID, expenseID)
}

func (s *Service) UpsertBudget(ctx context.Context, userID, petID string, input BudgetInput) (Budget, error) {
	input.Category = strings.TrimSpace(input.Category)
	input.Notes = strings.TrimSpace(input.Notes)
	if !id.IsUUID(petID) || input.Category == "" || len(input.Category) > 100 || input.AmountCents < 0 || len(input.Notes) > 2000 || (input.WarningThresholdCents != nil && *input.WarningThresholdCents < 0) {
		return Budget{}, ErrInvalid
	}
	if _, err := time.Parse("2006-01", input.Month); err != nil {
		return Budget{}, ErrInvalid
	}
	if input.Currency == "" {
		input.Currency = "EUR"
	}
	input.Currency = strings.ToUpper(input.Currency)
	if len(input.Currency) != 3 {
		return Budget{}, ErrInvalid
	}
	budgetID, err := id.New()
	if err != nil {
		return Budget{}, err
	}
	return s.repository.UpsertBudget(ctx, userID, userID, Budget{ID: budgetID, PetID: petID, Month: input.Month, Category: input.Category, AmountCents: input.AmountCents, Currency: input.Currency, WarningThresholdCents: input.WarningThresholdCents, Notes: input.Notes, UpdatedAt: s.now()})
}

func (s *Service) ListBudgets(ctx context.Context, userID, petID, month string) ([]Budget, error) {
	if !id.IsUUID(petID) {
		return nil, ErrInvalid
	}
	if _, err := time.Parse("2006-01", month); err != nil {
		return nil, ErrInvalid
	}
	return s.repository.ListBudgets(ctx, userID, petID, month)
}

func (s *Service) PetSummary(ctx context.Context, userID, petID, period string) (PetSummary, error) {
	if !id.IsUUID(petID) || (len(period) != 4 && len(period) != 7) {
		return PetSummary{}, ErrInvalid
	}
	layout := "2006"
	if len(period) == 7 {
		layout = "2006-01"
	}
	if _, err := time.Parse(layout, period); err != nil {
		return PetSummary{}, ErrInvalid
	}
	return s.repository.PetSummary(ctx, userID, petID, period)
}
