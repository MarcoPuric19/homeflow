package vets

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode"

	"homeflow/backend/internal/platform/id"
)

var (
	ErrInvalid    = errors.New("invalid veterinarian")
	ErrNotFound   = errors.New("veterinarian not found")
	ErrReferenced = errors.New("veterinarian is referenced")
)

type Repository interface {
	Create(context.Context, string, Veterinarian) (Veterinarian, error)
	List(context.Context, string, string, int, string) (Page, error)
	Get(context.Context, string, string) (Veterinarian, error)
	Update(context.Context, string, Veterinarian) (Veterinarian, error)
	Delete(context.Context, string, string) error
	Candidates(context.Context, string, string) ([]Veterinarian, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, userID string, input Input) (Veterinarian, error) {
	vet, err := s.normalise(input)
	if err != nil {
		return Veterinarian{}, err
	}
	vet.ID, err = id.New()
	if err != nil {
		return Veterinarian{}, err
	}
	vet.CreatedAt, vet.UpdatedAt = s.now(), s.now()
	return s.repository.Create(ctx, userID, vet)
}
func (s *Service) List(ctx context.Context, userID, search string, limit int, after string) (Page, error) {
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 || after != "" && !id.IsUUID(after) || len(search) > 200 {
		return Page{}, ErrInvalid
	}
	return s.repository.List(ctx, userID, strings.TrimSpace(search), limit, after)
}
func (s *Service) Get(ctx context.Context, userID, vetID string) (Veterinarian, error) {
	if !id.IsUUID(vetID) {
		return Veterinarian{}, ErrInvalid
	}
	return s.repository.Get(ctx, userID, vetID)
}
func (s *Service) Update(ctx context.Context, userID, vetID string, input Input) (Veterinarian, error) {
	if !id.IsUUID(vetID) {
		return Veterinarian{}, ErrInvalid
	}
	vet, err := s.normalise(input)
	if err != nil {
		return Veterinarian{}, err
	}
	vet.ID, vet.UpdatedAt = vetID, s.now()
	return s.repository.Update(ctx, userID, vet)
}
func (s *Service) Delete(ctx context.Context, userID, vetID string) error {
	if !id.IsUUID(vetID) {
		return ErrInvalid
	}
	return s.repository.Delete(ctx, userID, vetID)
}
func (s *Service) Match(ctx context.Context, userID, practice, postalCode string) ([]Veterinarian, error) {
	practice, postalCode = strings.TrimSpace(practice), strings.TrimSpace(postalCode)
	if practice == "" || len(practice) > 200 || len(postalCode) > 20 {
		return nil, ErrInvalid
	}
	candidates, err := s.repository.Candidates(ctx, userID, postalCode)
	if err != nil {
		return nil, err
	}
	want := normalisePractice(practice)
	matches := make([]Veterinarian, 0)
	for _, candidate := range candidates {
		if normalisePractice(candidate.Practice) == want || postalCode != "" && candidate.PostalCode == postalCode || similarity(want, normalisePractice(candidate.Practice)) >= .35 {
			matches = append(matches, candidate)
		}
	}
	return matches, nil
}
func (s *Service) normalise(input Input) (Veterinarian, error) {
	v := Veterinarian{Practice: strings.TrimSpace(input.Practice), VetName: strings.TrimSpace(input.VetName), Street: strings.TrimSpace(input.Street), PostalCode: strings.TrimSpace(input.PostalCode), City: strings.TrimSpace(input.City), Phone: strings.TrimSpace(input.Phone), Email: strings.TrimSpace(input.Email), Notes: strings.TrimSpace(input.Notes)}
	if v.Practice == "" || len(v.Practice) > 200 || len(v.VetName) > 200 || len(v.Street) > 200 || len(v.PostalCode) > 20 || len(v.City) > 120 || len(v.Phone) > 60 || len(v.Email) > 200 || len(v.Notes) > 2000 {
		return Veterinarian{}, ErrInvalid
	}
	return v, nil
}
func normalisePractice(value string) string {
	value = strings.ToLower(strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss").Replace(value))
	for _, noise := range []string{"tierarztpraxis", "praxis", "dipl", "vet", "med"} {
		value = strings.ReplaceAll(value, noise, "")
	}
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return -1
	}, value)
}
func similarity(a, b string) float64 {
	if a == b {
		return 1
	}
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 || len(br) == 0 {
		return 0
	}
	common := 0
	seen := map[rune]bool{}
	for _, r := range ar {
		seen[r] = true
	}
	for _, r := range br {
		if seen[r] {
			common++
		}
	}
	longest := len(ar)
	if len(br) > longest {
		longest = len(br)
	}
	return float64(common) / float64(longest)
}
