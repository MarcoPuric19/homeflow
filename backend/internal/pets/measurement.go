package pets

import "time"

type WeightEntry struct {
	ID         string    `json:"id"`
	PetID      string    `json:"petId"`
	MeasuredOn string    `json:"measuredOn"`
	WeightKg   string    `json:"weightKg"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}
type WeightInput struct {
	MeasuredOn string `json:"measuredOn"`
	WeightKg   string `json:"weightKg"`
	Notes      string `json:"notes"`
}
type LabResult struct {
	ID            string    `json:"id"`
	PetID         string    `json:"petId"`
	MeasuredOn    string    `json:"measuredOn"`
	TestName      string    `json:"testName"`
	Value         string    `json:"value"`
	Unit          string    `json:"unit,omitempty"`
	ReferenceLow  *string   `json:"referenceLow,omitempty"`
	ReferenceHigh *string   `json:"referenceHigh,omitempty"`
	Notes         string    `json:"notes,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}
type LabResultInput struct {
	MeasuredOn    string  `json:"measuredOn"`
	TestName      string  `json:"testName"`
	Value         string  `json:"value"`
	Unit          string  `json:"unit"`
	ReferenceLow  *string `json:"referenceLow"`
	ReferenceHigh *string `json:"referenceHigh"`
	Notes         string  `json:"notes"`
}
