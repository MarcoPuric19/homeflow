package pets

import "time"

type Vaccination struct {
	ID             string    `json:"id"`
	PetID          string    `json:"petId"`
	VaccineName    string    `json:"vaccineName"`
	Manufacturer   string    `json:"manufacturer,omitempty"`
	Diseases       []string  `json:"diseases"`
	AdministeredOn string    `json:"administeredOn"`
	ValidUntil     *string   `json:"validUntil,omitempty"`
	NextDueDate    *string   `json:"nextDueDate,omitempty"`
	DocumentID     *string   `json:"documentId,omitempty"`
	Veterinarian   string    `json:"veterinarian,omitempty"`
	BatchNumber    string    `json:"batchNumber,omitempty"`
	Notes          string    `json:"notes,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type VaccinationInput struct {
	VaccineName    string   `json:"vaccineName"`
	Manufacturer   string   `json:"manufacturer"`
	Diseases       []string `json:"diseases"`
	AdministeredOn string   `json:"administeredOn"`
	ValidUntil     *string  `json:"validUntil"`
	NextDueDate    *string  `json:"nextDueDate"`
	DocumentID     *string  `json:"documentId"`
	Veterinarian   string   `json:"veterinarian"`
	BatchNumber    string   `json:"batchNumber"`
	Notes          string   `json:"notes"`
}

type VaccinationPage struct {
	Vaccinations []Vaccination `json:"vaccinations"`
	NextCursor   string        `json:"nextCursor,omitempty"`
}

type vaccinationCursor struct {
	AdministeredOn string
	ID             string
}
