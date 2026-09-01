package documents

import "time"

type Document struct {
	ID               string    `json:"id"`
	PetID            string    `json:"petId"`
	Title            string    `json:"title"`
	DocumentType     string    `json:"documentType"`
	OccurredOn       *string   `json:"occurredOn,omitempty"`
	Notes            string    `json:"notes,omitempty"`
	OriginalFilename string    `json:"originalFilename"`
	MIMEType         string    `json:"mimeType"`
	SHA256           string    `json:"sha256"`
	StorageKey       string    `json:"-"`
	UploadedBy       string    `json:"uploadedBy"`
	UploadedAt       time.Time `json:"uploadedAt"`
}

type Metadata struct {
	Title        string
	DocumentType string
	OccurredOn   *string
	Notes        string
}
