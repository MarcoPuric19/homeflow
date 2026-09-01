package pets

import "time"

type PetMedication struct {
	ID               string    `json:"id"`
	PetID            string    `json:"petId"`
	MedicationID     string    `json:"medicationId"`
	Name             string    `json:"name"`
	ActiveIngredient string    `json:"activeIngredient,omitempty"`
	DosageForm       string    `json:"dosageForm,omitempty"`
	DoseText         string    `json:"doseText"`
	FrequencyText    string    `json:"frequencyText"`
	StartsOn         string    `json:"startsOn"`
	EndsOn           *string   `json:"endsOn,omitempty"`
	StockQuantity    string    `json:"stockQuantity"`
	StockUnit        string    `json:"stockUnit,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	Active           bool      `json:"active"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

type PetMedicationInput struct {
	Name             string  `json:"name"`
	ActiveIngredient string  `json:"activeIngredient"`
	DosageForm       string  `json:"dosageForm"`
	DoseText         string  `json:"doseText"`
	FrequencyText    string  `json:"frequencyText"`
	StartsOn         string  `json:"startsOn"`
	EndsOn           *string `json:"endsOn"`
	StockQuantity    string  `json:"stockQuantity"`
	StockUnit        string  `json:"stockUnit"`
	Notes            string  `json:"notes"`
	Active           *bool   `json:"active,omitempty"`
}

type PetMedicationPage struct {
	Medications []PetMedication `json:"medications"`
}
