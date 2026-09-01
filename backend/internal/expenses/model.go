package expenses

import "time"

type Status string

const (
	StatusPlanned   Status = "planned"
	StatusOpen      Status = "open"
	StatusPaid      Status = "paid"
	StatusCancelled Status = "cancelled"
)

type Expense struct {
	ID                 string     `json:"id"`
	Description        string     `json:"description"`
	Merchant           string     `json:"merchant,omitempty"`
	PlannedAmountCents int64      `json:"plannedAmountCents"`
	ActualAmountCents  *int64     `json:"actualAmountCents,omitempty"`
	Currency           string     `json:"currency"`
	DueDate            string     `json:"dueDate"`
	PaidAt             *time.Time `json:"paidAt,omitempty"`
	Category           string     `json:"category,omitempty"`
	RecurrenceRule     string     `json:"recurrenceRule,omitempty"`
	Status             Status     `json:"status"`
	UserID             string     `json:"userId"`
	PetID              *string    `json:"petId,omitempty"`
	BudgetCategory     string     `json:"budgetCategory,omitempty"`
	PaymentMethod      string     `json:"paymentMethod,omitempty"`
	Notes              string     `json:"notes,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
	UpdatedAt          time.Time  `json:"updatedAt"`
}

type CreateExpense struct {
	Description        string  `json:"description"`
	Merchant           string  `json:"merchant"`
	PlannedAmountCents int64   `json:"plannedAmountCents"`
	Currency           string  `json:"currency"`
	DueDate            string  `json:"dueDate"`
	Category           string  `json:"category"`
	RecurrenceRule     string  `json:"recurrenceRule"`
	Status             Status  `json:"status"`
	PaymentMethod      string  `json:"paymentMethod"`
	Notes              string  `json:"notes"`
	PetID              *string `json:"petId,omitempty"`
	BudgetCategory     string  `json:"budgetCategory"`
}

type Budget struct {
	ID          string    `json:"id"`
	PetID       string    `json:"petId"`
	Month       string    `json:"month"`
	Category    string    `json:"category"`
	AmountCents int64     `json:"amountCents"`
	Currency    string    `json:"currency"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

type BudgetInput struct {
	Month       string `json:"month"`
	Category    string `json:"category"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

type CategorySummary struct {
	Category           string `json:"category"`
	PlannedAmountCents int64  `json:"plannedAmountCents"`
	ActualAmountCents  int64  `json:"actualAmountCents"`
	BudgetAmountCents  int64  `json:"budgetAmountCents"`
}

type PetSummary struct {
	PetID      string            `json:"petId"`
	Period     string            `json:"period"`
	Categories []CategorySummary `json:"categories"`
}

type MarkPaid struct {
	ActualAmountCents int64     `json:"actualAmountCents"`
	PaidAt            time.Time `json:"paidAt"`
}

type MonthlySummary struct {
	Month                string `json:"month"`
	PlannedAmountCents   int64  `json:"plannedAmountCents"`
	ActualAmountCents    int64  `json:"actualAmountCents"`
	OpenAmountCents      int64  `json:"openAmountCents"`
	PlanActualDeltaCents int64  `json:"planActualDeltaCents"`
}
