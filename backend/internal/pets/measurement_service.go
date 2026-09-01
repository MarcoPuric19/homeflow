package pets

import (
	"context"
	"errors"
	"homeflow/backend/internal/platform/id"
	"math/big"
	"regexp"
	"strings"
	"time"
)

var ErrInvalidMeasurement = errors.New("invalid pet measurement")

const MeasurementMaxList = 100

var positiveDecimal = regexp.MustCompile(`^(0\.[0-9]{1,3}|[1-9][0-9]{0,6}(\.[0-9]{1,3})?)$`)
var signedDecimal = regexp.MustCompile(`^-?(0|[1-9][0-9]{0,9})(\.[0-9]{1,4})?$`)

type MeasurementRepository interface {
	CreateWeight(context.Context, string, string, WeightEntry) (WeightEntry, error)
	ListWeights(context.Context, string, string, int) ([]WeightEntry, error)
	CreateLabResult(context.Context, string, string, LabResult) (LabResult, error)
	ListLabResults(context.Context, string, string, int) ([]LabResult, error)
}

func (s *Service) CreateWeight(ctx context.Context, userID, petID string, input WeightInput) (WeightEntry, error) {
	if !id.IsUUID(petID) {
		return WeightEntry{}, ErrInvalidID
	}
	input.MeasuredOn = strings.TrimSpace(input.MeasuredOn)
	input.WeightKg = strings.TrimSpace(input.WeightKg)
	input.Notes = strings.TrimSpace(input.Notes)
	date, err := time.Parse(time.DateOnly, input.MeasuredOn)
	if err != nil || date.After(s.now()) || !positiveDecimal.MatchString(input.WeightKg) || len(input.Notes) > 1000 {
		return WeightEntry{}, ErrInvalidMeasurement
	}
	entryID, err := id.New()
	if err != nil {
		return WeightEntry{}, err
	}
	entry := WeightEntry{ID: entryID, PetID: petID, MeasuredOn: input.MeasuredOn, WeightKg: input.WeightKg, Notes: input.Notes, CreatedAt: s.now()}
	repo, ok := s.repository.(MeasurementRepository)
	if !ok {
		return WeightEntry{}, errors.New("measurement repository unavailable")
	}
	return repo.CreateWeight(ctx, userID, userID, entry)
}
func (s *Service) ListWeights(ctx context.Context, userID, petID string, limit int) ([]WeightEntry, error) {
	if !id.IsUUID(petID) {
		return nil, ErrInvalidID
	}
	if limit == 0 {
		limit = MeasurementMaxList
	}
	if limit < 1 || limit > MeasurementMaxList {
		return nil, ErrInvalidMeasurement
	}
	repo, ok := s.repository.(MeasurementRepository)
	if !ok {
		return nil, errors.New("measurement repository unavailable")
	}
	return repo.ListWeights(ctx, userID, petID, limit)
}
func decimalPointer(value *string) (*string, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, true
	}
	v := strings.TrimSpace(*value)
	if !signedDecimal.MatchString(v) {
		return nil, false
	}
	return &v, true
}
func (s *Service) CreateLabResult(ctx context.Context, userID, petID string, input LabResultInput) (LabResult, error) {
	if !id.IsUUID(petID) {
		return LabResult{}, ErrInvalidID
	}
	input.MeasuredOn = strings.TrimSpace(input.MeasuredOn)
	input.TestName = strings.TrimSpace(input.TestName)
	input.Value = strings.TrimSpace(input.Value)
	input.Unit = strings.TrimSpace(input.Unit)
	input.Notes = strings.TrimSpace(input.Notes)
	date, err := time.Parse(time.DateOnly, input.MeasuredOn)
	low, lowOK := decimalPointer(input.ReferenceLow)
	high, highOK := decimalPointer(input.ReferenceHigh)
	if err != nil || date.After(s.now()) || input.TestName == "" || len(input.TestName) > 200 || !signedDecimal.MatchString(input.Value) || len(input.Unit) > 50 || len(input.Notes) > 1000 || !lowOK || !highOK {
		return LabResult{}, ErrInvalidMeasurement
	}
	if low != nil && high != nil {
		lowValue, lowValid := new(big.Rat).SetString(*low)
		highValue, highValid := new(big.Rat).SetString(*high)
		if !lowValid || !highValid || lowValue.Cmp(highValue) > 0 {
			return LabResult{}, ErrInvalidMeasurement
		}
	}
	resultID, err := id.New()
	if err != nil {
		return LabResult{}, err
	}
	result := LabResult{ID: resultID, PetID: petID, MeasuredOn: input.MeasuredOn, TestName: input.TestName, Value: input.Value, Unit: input.Unit, ReferenceLow: low, ReferenceHigh: high, Notes: input.Notes, CreatedAt: s.now()}
	repo, ok := s.repository.(MeasurementRepository)
	if !ok {
		return LabResult{}, errors.New("measurement repository unavailable")
	}
	return repo.CreateLabResult(ctx, userID, userID, result)
}
func (s *Service) ListLabResults(ctx context.Context, userID, petID string, limit int) ([]LabResult, error) {
	if !id.IsUUID(petID) {
		return nil, ErrInvalidID
	}
	if limit == 0 {
		limit = MeasurementMaxList
	}
	if limit < 1 || limit > MeasurementMaxList {
		return nil, ErrInvalidMeasurement
	}
	repo, ok := s.repository.(MeasurementRepository)
	if !ok {
		return nil, errors.New("measurement repository unavailable")
	}
	return repo.ListLabResults(ctx, userID, petID, limit)
}
