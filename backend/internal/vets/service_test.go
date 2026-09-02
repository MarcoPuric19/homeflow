package vets

import (
	"context"
	"testing"
)

type memoryRepository struct{ values []Veterinarian }

func (r *memoryRepository) Create(_ context.Context, _ string, v Veterinarian) (Veterinarian, error) {
	r.values = append(r.values, v)
	return v, nil
}
func (r *memoryRepository) List(_ context.Context, _ string, _ string, _ int, _ string) (Page, error) {
	return Page{Veterinarians: r.values}, nil
}
func (r *memoryRepository) Get(_ context.Context, _ string, id string) (Veterinarian, error) {
	for _, v := range r.values {
		if v.ID == id {
			return v, nil
		}
	}
	return Veterinarian{}, ErrNotFound
}
func (r *memoryRepository) Update(_ context.Context, _ string, v Veterinarian) (Veterinarian, error) {
	return v, nil
}
func (r *memoryRepository) Delete(context.Context, string, string) error { return nil }
func (r *memoryRepository) Candidates(_ context.Context, _ string, _ string) ([]Veterinarian, error) {
	return r.values, nil
}

func TestMatchReturnsCandidatesWithoutDeciding(t *testing.T) {
	r := &memoryRepository{values: []Veterinarian{{ID: "1", Practice: "TIERZENTRUM AM SCHMELZBACH", PostalCode: "12345"}}}
	s := NewService(r)
	for _, name := range []string{"Tierzentrum am Schmelzbach", "ENBEL-E6- axisfür Kleintiermedizin"} {
		matches, err := s.Match(context.Background(), "user", name, "12345")
		if err != nil || len(matches) != 1 || matches[0].ID != "1" {
			t.Fatalf("name=%q matches=%#v err=%v", name, matches, err)
		}
	}
}
func TestNormalisationAndValidation(t *testing.T) {
	r := &memoryRepository{}
	s := NewService(r)
	v, err := s.Create(context.Background(), "user", Input{Practice: " Tierarztpraxis Momo ", PostalCode: " 12345 "})
	if err != nil || v.Practice != "Tierarztpraxis Momo" || v.PostalCode != "12345" {
		t.Fatalf("v=%#v err=%v", v, err)
	}
	if _, err := s.Create(context.Background(), "user", Input{}); err != ErrInvalid {
		t.Fatalf("err=%v", err)
	}
}
