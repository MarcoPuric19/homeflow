package shopping

import "time"

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

type Item struct {
	ID          string     `json:"id"`
	ListID      string     `json:"listId"`
	Name        string     `json:"name"`
	Quantity    string     `json:"quantity"`
	Unit        string     `json:"unit,omitempty"`
	Category    string     `json:"category,omitempty"`
	Note        string     `json:"note,omitempty"`
	Priority    Priority   `json:"priority"`
	Store       string     `json:"store,omitempty"`
	AddedBy     string     `json:"addedBy"`
	Completed   bool       `json:"completed"`
	PurchasedBy string     `json:"purchasedBy,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	UpdatedAt   time.Time  `json:"updatedAt"`
	PetIDs      []string   `json:"petIds"`
}

type CreateItem struct {
	ListID   string   `json:"listId"`
	Name     string   `json:"name"`
	Quantity string   `json:"quantity"`
	Unit     string   `json:"unit"`
	Category string   `json:"category"`
	Note     string   `json:"note"`
	Priority Priority `json:"priority"`
	Store    string   `json:"store"`
	PetIDs   []string `json:"petIds"`
}

type List struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}
