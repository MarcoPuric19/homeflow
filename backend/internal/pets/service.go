package pets

import (
	"context"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var (
	ErrInvalid   = errors.New("invalid pet")
	ErrInvalidID = errors.New("invalid pet id")
	ErrNotFound  = errors.New("pet not found")
)

const DefaultLimit = 50
const MaxLimit = 100

type Repository interface {
	Create(context.Context, string, string, Pet) (Pet, error)
	List(context.Context, string, bool, int, string) (Page, error)
	Get(context.Context, string, string) (Pet, error)
	Update(context.Context, string, Pet) (Pet, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, userID string, input Input) (Pet, error) {
	pet, err := s.normalise(input, true)
	if err != nil {
		return Pet{}, err
	}
	pet.ID, err = id.New()
	if err != nil {
		return Pet{}, err
	}
	pet.CreatedAt, pet.UpdatedAt = s.now(), s.now()
	return s.repository.Create(ctx, userID, userID, pet)
}

func (s *Service) List(ctx context.Context, userID string, includeInactive bool, limit int, after string) (Page, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit || (after != "" && !id.IsUUID(after)) {
		return Page{}, ErrInvalid
	}
	return s.repository.List(ctx, userID, includeInactive, limit, after)
}

func (s *Service) Get(ctx context.Context, userID, petID string) (Pet, error) {
	if !id.IsUUID(petID) {
		return Pet{}, ErrInvalidID
	}
	return s.repository.Get(ctx, userID, petID)
}

func (s *Service) Update(ctx context.Context, userID, petID string, input Input) (Pet, error) {
	if !id.IsUUID(petID) {
		return Pet{}, ErrInvalidID
	}
	pet, err := s.normalise(input, false)
	if err != nil {
		return Pet{}, err
	}
	pet.ID, pet.UpdatedAt = petID, s.now()
	return s.repository.Update(ctx, userID, pet)
}

func (s *Service) normalise(input Input, creating bool) (Pet, error) {
	name, species := strings.TrimSpace(input.Name), strings.TrimSpace(input.Species)
	breed, notes := strings.TrimSpace(input.Breed), strings.TrimSpace(input.Notes)
	color, insurance := strings.TrimSpace(input.Color), strings.TrimSpace(input.Insurance)
	insuranceNumber, veterinarian := strings.TrimSpace(input.InsuranceNumber), strings.TrimSpace(input.Veterinarian)
	var primaryVetID *string
	if input.PrimaryVetID != nil && strings.TrimSpace(*input.PrimaryVetID) != "" {
		value := strings.TrimSpace(*input.PrimaryVetID)
		if !id.IsUUID(value) {
			return Pet{}, ErrInvalid
		}
		primaryVetID = &value
	}
	sex := input.Sex
	if sex == "" {
		sex = SexUnknown
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	if name == "" || len(name) > 120 || species == "" || len(species) > 80 || len(breed) > 120 || len(notes) > 2000 || len(color) > 100 || len(insurance) > 200 || len(insuranceNumber) > 100 || len(veterinarian) > 200 {
		return Pet{}, ErrInvalid
	}
	if sex != SexFemale && sex != SexMale && sex != SexUnknown {
		return Pet{}, ErrInvalid
	}
	var birthDate *string
	if input.BirthDate != nil && strings.TrimSpace(*input.BirthDate) != "" {
		value := strings.TrimSpace(*input.BirthDate)
		date, err := time.Parse(time.DateOnly, value)
		if err != nil || date.After(s.now()) {
			return Pet{}, ErrInvalid
		}
		birthDate = &value
	}
	if input.ApproximateBirthDate && birthDate == nil {
		return Pet{}, ErrInvalid
	}
	var chip *string
	if input.MicrochipNumber != nil && strings.TrimSpace(*input.MicrochipNumber) != "" {
		value := strings.TrimSpace(*input.MicrochipNumber)
		if len(value) < 5 || len(value) > 50 {
			return Pet{}, ErrInvalid
		}
		chip = &value
	}
	_ = creating
	return Pet{Name: name, Species: species, Breed: breed, BirthDate: birthDate, ApproximateBirthDate: input.ApproximateBirthDate, Sex: sex, MicrochipNumber: chip, Color: color, Insurance: insurance, InsuranceNumber: insuranceNumber, Veterinarian: veterinarian, PrimaryVetID: primaryVetID, Notes: notes, Active: active}, nil
}
