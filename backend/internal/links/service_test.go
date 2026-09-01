package links

import (
	"context"
	"testing"
)

type stubRepository struct{ set Set }

func (r *stubRepository) GetDocument(context.Context, string, string, string) (Set, error) {
	return r.set, nil
}
func (r *stubRepository) ReplaceDocument(_ context.Context, _ string, _ string, _ string, set Set) (Set, error) {
	r.set = set
	return set, nil
}
func (r *stubRepository) GetExpense(context.Context, string, string) (Set, error) { return r.set, nil }
func (r *stubRepository) ReplaceExpense(_ context.Context, _ string, _ string, set Set) (Set, error) {
	r.set = set
	return set, nil
}

func TestReplaceValidatesBoundedUniqueUUIDs(t *testing.T) {
	s := NewService(&stubRepository{})
	id1 := "00000000-0000-4000-8000-000000000001"
	if _, err := s.ReplaceExpense(context.Background(), "user", id1, Set{MedicalRecordIDs: []string{id1}}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReplaceExpense(context.Background(), "user", id1, Set{MedicalRecordIDs: []string{id1, id1}}); err != ErrInvalid {
		t.Fatalf("duplicate err=%v", err)
	}
	if _, err := s.GetDocument(context.Background(), "user", "bad", id1); err != ErrInvalid {
		t.Fatalf("invalid err=%v", err)
	}
}
