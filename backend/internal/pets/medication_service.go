package pets

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var ErrInvalidMedication = errors.New("invalid pet medication")

const PetMedicationMaxList = 100

var stockPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,8})(\.[0-9]{1,3})?$`)

type PetMedicationRepository interface {
	CreatePetMedication(context.Context, string, string, string, PetMedication) (PetMedication, error)
	ListPetMedications(context.Context, string, string, bool, int) (PetMedicationPage, error)
}

func (s *Service) CreatePetMedication(ctx context.Context, userID, petID string, input PetMedicationInput) (PetMedication, error) {
	if !id.IsUUID(petID) {
		return PetMedication{}, ErrInvalidID
	}
	medication, err := normalisePetMedication(input, s.now())
	if err != nil {
		return PetMedication{}, err
	}
	medication.ID, err = id.New()
	if err != nil {
		return PetMedication{}, err
	}
	medication.MedicationID, err = id.New()
	if err != nil {
		return PetMedication{}, err
	}
	medication.PetID, medication.CreatedAt, medication.UpdatedAt = petID, s.now(), s.now()
	repository, ok := s.repository.(PetMedicationRepository)
	if !ok {
		return PetMedication{}, errors.New("pet medication repository unavailable")
	}
	return repository.CreatePetMedication(ctx, userID, userID, medication.MedicationID, medication)
}

func (s *Service) ListPetMedications(ctx context.Context, userID, petID string, includeInactive bool, limit int) (PetMedicationPage, error) {
	if !id.IsUUID(petID) {
		return PetMedicationPage{}, ErrInvalidID
	}
	if limit == 0 {
		limit = PetMedicationMaxList
	}
	if limit < 1 || limit > PetMedicationMaxList {
		return PetMedicationPage{}, ErrInvalidMedication
	}
	repository, ok := s.repository.(PetMedicationRepository)
	if !ok {
		return PetMedicationPage{}, errors.New("pet medication repository unavailable")
	}
	return repository.ListPetMedications(ctx, userID, petID, includeInactive, limit)
}

func normalisePetMedication(input PetMedicationInput, now time.Time) (PetMedication, error) {
	input.Name, input.ActiveIngredient, input.DosageForm = strings.TrimSpace(input.Name), strings.TrimSpace(input.ActiveIngredient), strings.TrimSpace(input.DosageForm)
	input.DoseText, input.FrequencyText, input.StockQuantity, input.StockUnit, input.Notes = strings.TrimSpace(input.DoseText), strings.TrimSpace(input.FrequencyText), strings.TrimSpace(input.StockQuantity), strings.TrimSpace(input.StockUnit), strings.TrimSpace(input.Notes)
	start, err := time.Parse(time.DateOnly, input.StartsOn)
	if err != nil || start.After(now.AddDate(0, 0, 1)) || input.Name == "" || len(input.Name) > 200 || len(input.ActiveIngredient) > 200 || len(input.DosageForm) > 100 || input.DoseText == "" || len(input.DoseText) > 200 || input.FrequencyText == "" || len(input.FrequencyText) > 200 || !stockPattern.MatchString(input.StockQuantity) || len(input.StockUnit) > 50 || len(input.Notes) > 2000 {
		return PetMedication{}, ErrInvalidMedication
	}
	var end *string
	if input.EndsOn != nil && strings.TrimSpace(*input.EndsOn) != "" {
		value := strings.TrimSpace(*input.EndsOn)
		date, err := time.Parse(time.DateOnly, value)
		if err != nil || date.Before(start) {
			return PetMedication{}, ErrInvalidMedication
		}
		end = &value
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	return PetMedication{Name: input.Name, ActiveIngredient: input.ActiveIngredient, DosageForm: input.DosageForm, DoseText: input.DoseText, FrequencyText: input.FrequencyText, StartsOn: input.StartsOn, EndsOn: end, StockQuantity: input.StockQuantity, StockUnit: input.StockUnit, Notes: input.Notes, Active: active}, nil
}
