package shopping

import "time"

type Priority string

const (
	PriorityLow    Priority = "low"
	PriorityNormal Priority = "normal"
	PriorityHigh   Priority = "high"
)

type Item struct {
	ID                     string     `json:"id"`
	ListID                 string     `json:"listId"`
	Name                   string     `json:"name"`
	Quantity               string     `json:"quantity"`
	Unit                   string     `json:"unit,omitempty"`
	Category               string     `json:"category,omitempty"`
	Note                   string     `json:"note,omitempty"`
	Priority               Priority   `json:"priority"`
	Store                  string     `json:"store,omitempty"`
	AddedBy                string     `json:"addedBy"`
	Completed              bool       `json:"completed"`
	PurchasedBy            string     `json:"purchasedBy,omitempty"`
	CompletedAt            *time.Time `json:"completedAt,omitempty"`
	CreatedAt              time.Time  `json:"createdAt"`
	UpdatedAt              time.Time  `json:"updatedAt"`
	PetIDs                 []string   `json:"petIds"`
	SourcePetMedicationID  *string    `json:"sourcePetMedicationId,omitempty"`
	PreferredBrand         string     `json:"preferredBrand,omitempty"`
	EstimatedPriceCents    *int64     `json:"estimatedPriceCents,omitempty"`
	EstimatedPriceCurrency string     `json:"estimatedPriceCurrency"`
	Recurring              bool       `json:"recurring"`
	// Ein Artikel gehört seit HF-0116 zu einem, zu mehreren oder ausdrücklich
	// zu allen Tieren. "Alle" ist eine Markierung und nicht die leere Menge -
	// sonst ist es von "vergessen" nicht zu unterscheiden.
	ForAllPets bool `json:"forAllPets"`
}

type MedicationRestockPreview struct {
	PetID         string  `json:"petId"`
	MedicationID  string  `json:"medicationId"`
	Name          string  `json:"name"`
	StockQuantity string  `json:"stockQuantity"`
	MinimumStock  string  `json:"minimumStock"`
	StockUnit     string  `json:"stockUnit"`
	PackageSize   *string `json:"packageSize,omitempty"`
}

type MedicationRestockInput struct {
	ListID string `json:"listId"`
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
	// Seit HF-0116 muss ein Artikel Tierbezug haben: entweder PetIDs oder
	// ausdrücklich ForAllPets. Beides leer ist ungültig.
	ForAllPets             bool   `json:"forAllPets"`
	PreferredBrand         string `json:"preferredBrand"`
	EstimatedPriceCents    *int64 `json:"estimatedPriceCents"`
	EstimatedPriceCurrency string `json:"estimatedPriceCurrency"`
	Recurring              bool   `json:"recurring"`
}

type List struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Visibility string `json:"visibility"`
}
