package pets

import (
	"context"
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var ErrInvalidVaccination = errors.New("invalid vaccination")

const VaccinationDefaultLimit = 30
const VaccinationMaxLimit = 100

type VaccinationRepository interface {
	CreateVaccination(context.Context, string, string, Vaccination) (Vaccination, error)
	ListVaccinations(context.Context, string, string, int, *vaccinationCursor) (VaccinationPage, error)
}

func (s *Service) CreateVaccination(ctx context.Context, userID, petID string, input VaccinationInput) (Vaccination, error) {
	if !id.IsUUID(petID) {
		return Vaccination{}, ErrInvalidID
	}
	vaccination, err := normaliseVaccination(input, s.now())
	if err != nil {
		return Vaccination{}, err
	}
	vaccination.ID, err = id.New()
	if err != nil {
		return Vaccination{}, err
	}
	vaccination.PetID, vaccination.CreatedAt, vaccination.UpdatedAt = petID, s.now(), s.now()
	repository, ok := s.repository.(VaccinationRepository)
	if !ok {
		return Vaccination{}, errors.New("vaccination repository unavailable")
	}
	return repository.CreateVaccination(ctx, userID, userID, vaccination)
}

func (s *Service) ListVaccinations(ctx context.Context, userID, petID string, limit int, cursor string) (VaccinationPage, error) {
	if !id.IsUUID(petID) {
		return VaccinationPage{}, ErrInvalidID
	}
	if limit == 0 {
		limit = VaccinationDefaultLimit
	}
	if limit < 1 || limit > VaccinationMaxLimit {
		return VaccinationPage{}, ErrInvalidVaccination
	}
	parsed, err := decodeVaccinationCursor(cursor)
	if err != nil {
		return VaccinationPage{}, ErrInvalidVaccination
	}
	repository, ok := s.repository.(VaccinationRepository)
	if !ok {
		return VaccinationPage{}, errors.New("vaccination repository unavailable")
	}
	return repository.ListVaccinations(ctx, userID, petID, limit, parsed)
}

func normaliseVaccination(input VaccinationInput, now time.Time) (Vaccination, error) {
	input.VaccineName, input.Manufacturer, input.Veterinarian, input.BatchNumber, input.Notes = strings.TrimSpace(input.VaccineName), strings.TrimSpace(input.Manufacturer), strings.TrimSpace(input.Veterinarian), strings.TrimSpace(input.BatchNumber), strings.TrimSpace(input.Notes)
	administered, err := time.Parse(time.DateOnly, input.AdministeredOn)
	if err != nil || administered.After(now) || input.VaccineName == "" || len(input.VaccineName) > 200 || len(input.Manufacturer) > 200 || len(input.Veterinarian) > 200 || len(input.BatchNumber) > 100 || len(input.Notes) > 2000 || len(input.Diseases) > 20 {
		return Vaccination{}, ErrInvalidVaccination
	}
	diseases := make([]string, 0, len(input.Diseases))
	seen := map[string]bool{}
	for _, disease := range input.Diseases {
		disease = strings.TrimSpace(disease)
		if disease == "" || len(disease) > 100 || seen[disease] {
			return Vaccination{}, ErrInvalidVaccination
		}
		seen[disease] = true
		diseases = append(diseases, disease)
	}
	validUntil, err := optionalVaccinationDate(input.ValidUntil, administered)
	if err != nil {
		return Vaccination{}, err
	}
	var next *string
	if input.NextDueDate != nil && strings.TrimSpace(*input.NextDueDate) != "" {
		value := strings.TrimSpace(*input.NextDueDate)
		due, err := time.Parse(time.DateOnly, value)
		if err != nil || due.Before(administered) {
			return Vaccination{}, ErrInvalidVaccination
		}
		next = &value
	}
	var documentID *string
	if input.DocumentID != nil && strings.TrimSpace(*input.DocumentID) != "" {
		value := strings.TrimSpace(*input.DocumentID)
		if !id.IsUUID(value) {
			return Vaccination{}, ErrInvalidVaccination
		}
		documentID = &value
	}
	return Vaccination{VaccineName: input.VaccineName, Manufacturer: input.Manufacturer, Diseases: diseases, AdministeredOn: input.AdministeredOn, ValidUntil: validUntil, NextDueDate: next, DocumentID: documentID, Veterinarian: input.Veterinarian, BatchNumber: input.BatchNumber, Notes: input.Notes}, nil
}

func optionalVaccinationDate(value *string, earliest time.Time) (*string, error) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, nil
	}
	clean := strings.TrimSpace(*value)
	date, err := time.Parse(time.DateOnly, clean)
	if err != nil || date.Before(earliest) {
		return nil, ErrInvalidVaccination
	}
	return &clean, nil
}

func encodeVaccinationCursor(cursor vaccinationCursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(cursor.AdministeredOn + "|" + cursor.ID))
}
func decodeVaccinationCursor(value string) (*vaccinationCursor, error) {
	if value == "" {
		return nil, nil
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return nil, err
	}
	parts := strings.SplitN(string(decoded), "|", 2)
	if len(parts) != 2 || !id.IsUUID(parts[1]) {
		return nil, ErrInvalidVaccination
	}
	if _, err := time.Parse(time.DateOnly, parts[0]); err != nil {
		return nil, ErrInvalidVaccination
	}
	return &vaccinationCursor{AdministeredOn: parts[0], ID: parts[1]}, nil
}
