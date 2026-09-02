package pets

import "time"

type Sex string

const (
	SexFemale  Sex = "female"
	SexMale    Sex = "male"
	SexUnknown Sex = "unknown"
)

type Pet struct {
	ID                   string    `json:"id"`
	Name                 string    `json:"name"`
	Species              string    `json:"species"`
	Breed                string    `json:"breed,omitempty"`
	BirthDate            *string   `json:"birthDate,omitempty"`
	ApproximateBirthDate bool      `json:"approximateBirthDate"`
	Sex                  Sex       `json:"sex"`
	MicrochipNumber      *string   `json:"microchipNumber,omitempty"`
	Color                string    `json:"color,omitempty"`
	Insurance            string    `json:"insurance,omitempty"`
	InsuranceNumber      string    `json:"insuranceNumber,omitempty"`
	Veterinarian         string    `json:"veterinarian,omitempty"`
	PrimaryVetID         *string   `json:"primaryVetId,omitempty"`
	Notes                string    `json:"notes,omitempty"`
	Active               bool      `json:"active"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

type Input struct {
	Name                 string  `json:"name"`
	Species              string  `json:"species"`
	Breed                string  `json:"breed"`
	BirthDate            *string `json:"birthDate"`
	ApproximateBirthDate bool    `json:"approximateBirthDate"`
	Sex                  Sex     `json:"sex"`
	MicrochipNumber      *string `json:"microchipNumber"`
	Color                string  `json:"color"`
	Insurance            string  `json:"insurance"`
	InsuranceNumber      string  `json:"insuranceNumber"`
	Veterinarian         string  `json:"veterinarian"`
	PrimaryVetID         *string `json:"primaryVetId"`
	Notes                string  `json:"notes"`
	Active               *bool   `json:"active,omitempty"`
}

type Page struct {
	Pets       []Pet  `json:"pets"`
	NextCursor string `json:"nextCursor,omitempty"`
}
