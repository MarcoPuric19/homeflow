package pets

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var ErrInvalidRecord = errors.New("invalid medical record")

const MedicalRecordDefaultLimit = 30
const MedicalRecordMaxLimit = 100

type MedicalRecordRepository interface {
	CreateMedicalRecord(context.Context, string, string, MedicalRecord) (MedicalRecord, error)
	ListMedicalRecords(context.Context, string, string, int, *medicalCursor) (MedicalRecordPage, error)
}

func (s *Service) CreateMedicalRecord(ctx context.Context, userID, petID string, input MedicalRecordInput) (MedicalRecord, error) {
	if !id.IsUUID(petID) {
		return MedicalRecord{}, ErrInvalidID
	}
	record, err := normaliseMedicalRecord(input, s.now())
	if err != nil {
		return MedicalRecord{}, err
	}
	record.ID, err = id.New()
	if err != nil {
		return MedicalRecord{}, err
	}
	record.PetID, record.CreatedAt, record.UpdatedAt = petID, s.now(), s.now()
	repository, ok := s.repository.(MedicalRecordRepository)
	if !ok {
		return MedicalRecord{}, errors.New("medical record repository unavailable")
	}
	return repository.CreateMedicalRecord(ctx, userID, userID, record)
}

func (s *Service) ListMedicalRecords(ctx context.Context, userID, petID string, limit int, cursor string) (MedicalRecordPage, error) {
	if !id.IsUUID(petID) {
		return MedicalRecordPage{}, ErrInvalidID
	}
	if limit == 0 {
		limit = MedicalRecordDefaultLimit
	}
	if limit < 1 || limit > MedicalRecordMaxLimit {
		return MedicalRecordPage{}, ErrInvalidRecord
	}
	parsed, err := decodeMedicalCursor(cursor)
	if err != nil {
		return MedicalRecordPage{}, ErrInvalidRecord
	}
	repository, ok := s.repository.(MedicalRecordRepository)
	if !ok {
		return MedicalRecordPage{}, errors.New("medical record repository unavailable")
	}
	return repository.ListMedicalRecords(ctx, userID, petID, limit, parsed)
}

func normaliseMedicalRecord(input MedicalRecordInput, now time.Time) (MedicalRecord, error) {
	input.Title, input.Summary, input.Veterinarian = strings.TrimSpace(input.Title), strings.TrimSpace(input.Summary), strings.TrimSpace(input.Veterinarian)
	date, err := time.Parse(time.DateOnly, input.OccurredOn)
	if err != nil || date.After(now) || input.Title == "" || len(input.Title) > 200 || len(input.Summary) > 4000 || len(input.Veterinarian) > 200 || !validRecordType(input.RecordType) {
		return MedicalRecord{}, ErrInvalidRecord
	}
	if input.Details == nil {
		input.Details = map[string]string{}
	}
	if len(input.Details) > 20 {
		return MedicalRecord{}, ErrInvalidRecord
	}
	for key, value := range input.Details {
		if strings.TrimSpace(key) == "" || len(key) > 80 || len(value) > 1000 {
			return MedicalRecord{}, ErrInvalidRecord
		}
	}
	return MedicalRecord{RecordType: input.RecordType, OccurredOn: input.OccurredOn, Title: input.Title, Summary: input.Summary, Veterinarian: input.Veterinarian, Details: input.Details}, nil
}

func validRecordType(value RecordType) bool {
	switch value {
	case RecordDiagnosis, RecordTreatment, RecordProcedure, RecordExamination, RecordNote, RecordImaging, RecordReport:
		return true
	}
	return false
}

func encodeMedicalCursor(cursor medicalCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursor.OccurredOn + "|" + cursor.ID))
}
func decodeMedicalCursor(value string) (*medicalCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || !id.IsUUID(parts[1]) {
		return nil, ErrInvalidRecord
	}
	if _, err := time.Parse(time.DateOnly, parts[0]); err != nil {
		return nil, ErrInvalidRecord
	}
	return &medicalCursor{OccurredOn: parts[0], ID: parts[1]}, nil
}
