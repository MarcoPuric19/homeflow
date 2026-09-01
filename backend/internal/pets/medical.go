package pets

import "time"

type RecordType string

const (
	RecordDiagnosis   RecordType = "diagnosis"
	RecordTreatment   RecordType = "treatment"
	RecordProcedure   RecordType = "procedure"
	RecordExamination RecordType = "examination"
	RecordNote        RecordType = "note"
	RecordImaging     RecordType = "imaging"
	RecordReport      RecordType = "report"
)

type MedicalRecord struct {
	ID           string            `json:"id"`
	PetID        string            `json:"petId"`
	RecordType   RecordType        `json:"recordType"`
	OccurredOn   string            `json:"occurredOn"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary,omitempty"`
	Veterinarian string            `json:"veterinarian,omitempty"`
	Details      map[string]string `json:"details"`
	CreatedAt    time.Time         `json:"createdAt"`
	UpdatedAt    time.Time         `json:"updatedAt"`
}

type MedicalRecordInput struct {
	RecordType   RecordType        `json:"recordType"`
	OccurredOn   string            `json:"occurredOn"`
	Title        string            `json:"title"`
	Summary      string            `json:"summary"`
	Veterinarian string            `json:"veterinarian"`
	Details      map[string]string `json:"details"`
}

type MedicalRecordPage struct {
	Records    []MedicalRecord `json:"records"`
	NextCursor string          `json:"nextCursor,omitempty"`
}

type medicalCursor struct {
	OccurredOn string
	ID         string
}
