package receipts

import "time"

type Receipt struct {
	ID               string    `json:"id"`
	ExpenseID        string    `json:"expenseId"`
	Merchant         string    `json:"merchant,omitempty"`
	ReceiptDate      *string   `json:"receiptDate,omitempty"`
	TotalAmountCents *int64    `json:"totalAmountCents,omitempty"`
	Currency         string    `json:"currency"`
	Category         string    `json:"category,omitempty"`
	OriginalFilename string    `json:"originalFilename"`
	MIMEType         string    `json:"mimeType"`
	SHA256           string    `json:"sha256"`
	StorageKey       string    `json:"-"`
	UploadedAt       time.Time `json:"uploadedAt"`
	UploadedBy       string    `json:"uploadedBy"`
}

type Metadata struct {
	Merchant         string
	ReceiptDate      *string
	TotalAmountCents *int64
	Currency         string
	Category         string
}
