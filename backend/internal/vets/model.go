package vets

import "time"

type Veterinarian struct {
	ID         string    `json:"id"`
	Practice   string    `json:"practice"`
	VetName    string    `json:"vetName,omitempty"`
	Street     string    `json:"street,omitempty"`
	PostalCode string    `json:"postalCode,omitempty"`
	City       string    `json:"city,omitempty"`
	Phone      string    `json:"phone,omitempty"`
	Email      string    `json:"email,omitempty"`
	Notes      string    `json:"notes,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Input struct {
	Practice   string `json:"practice"`
	VetName    string `json:"vetName"`
	Street     string `json:"street"`
	PostalCode string `json:"postalCode"`
	City       string `json:"city"`
	Phone      string `json:"phone"`
	Email      string `json:"email"`
	Notes      string `json:"notes"`
}

type Page struct {
	Veterinarians []Veterinarian `json:"veterinarians"`
	NextCursor    string         `json:"nextCursor,omitempty"`
}
