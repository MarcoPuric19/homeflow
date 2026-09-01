package pets

import (
	"context"
	"errors"
	"regexp"
	"strconv"
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
	UpdateMedicationStock(context.Context, string, string, string, string, time.Time) (PetMedication, error)
}

func (s *Service) UpdateMedicationStock(ctx context.Context, userID, petID, medicationID string, input MedicationStockInput) (PetMedication, error) {
	value := strings.TrimSpace(input.StockQuantity)
	if !id.IsUUID(petID) || !id.IsUUID(medicationID) || !stockPattern.MatchString(value) {
		return PetMedication{}, ErrInvalidMedication
	}
	repository, ok := s.repository.(PetMedicationRepository)
	if !ok {
		return PetMedication{}, errors.New("pet medication repository unavailable")
	}
	return repository.UpdateMedicationStock(ctx, userID, petID, medicationID, value, s.now())
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
	decimal := func(value *string, positive bool) (*string, error) {
		if value == nil || strings.TrimSpace(*value) == "" {
			return nil, nil
		}
		v := strings.TrimSpace(*value)
		number, parseErr := strconv.ParseFloat(v, 64)
		if !stockPattern.MatchString(v) || parseErr != nil || (positive && number <= 0) {
			return nil, ErrInvalidMedication
		}
		return &v, nil
	}
	packageSize, err := decimal(input.PackageSize, true)
	if err != nil {
		return PetMedication{}, err
	}
	minimumStock, err := decimal(input.MinimumStock, false)
	if err != nil {
		return PetMedication{}, err
	}
	dateValue := func(value *string) (*string, time.Time, error) {
		if value == nil || strings.TrimSpace(*value) == "" {
			return nil, time.Time{}, nil
		}
		v := strings.TrimSpace(*value)
		parsed, err := time.Parse(time.DateOnly, v)
		if err != nil {
			return nil, time.Time{}, ErrInvalidMedication
		}
		return &v, parsed, nil
	}
	purchasedOn, purchasedDate, err := dateValue(input.PurchasedOn)
	if err != nil || (!purchasedDate.IsZero() && purchasedDate.After(now)) {
		return PetMedication{}, ErrInvalidMedication
	}
	expiresOn, expiresDate, err := dateValue(input.ExpiresOn)
	if err != nil || (!expiresDate.IsZero() && !purchasedDate.IsZero() && expiresDate.Before(purchasedDate)) {
		return PetMedication{}, ErrInvalidMedication
	}
	if input.StockUnit == "" && (packageSize != nil || minimumStock != nil) {
		return PetMedication{}, ErrInvalidMedication
	}
	return PetMedication{Name: input.Name, ActiveIngredient: input.ActiveIngredient, DosageForm: input.DosageForm, DoseText: input.DoseText, FrequencyText: input.FrequencyText, StartsOn: input.StartsOn, EndsOn: end, StockQuantity: input.StockQuantity, StockUnit: input.StockUnit, PackageSize: packageSize, MinimumStock: minimumStock, PurchasedOn: purchasedOn, ExpiresOn: expiresOn, Notes: input.Notes, Active: active}, nil
}
