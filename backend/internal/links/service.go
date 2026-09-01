package links

import (
	"context"
	"errors"

	"homeflow/backend/internal/platform/id"
)

var (
	ErrInvalid  = errors.New("invalid links")
	ErrNotFound = errors.New("link source or target not found")
)

const MaxLinksPerType = 100

type Repository interface {
	GetDocument(context.Context, string, string, string) (Set, error)
	ReplaceDocument(context.Context, string, string, string, Set) (Set, error)
	GetExpense(context.Context, string, string) (Set, error)
	ReplaceExpense(context.Context, string, string, Set) (Set, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) GetDocument(ctx context.Context, userID, petID, documentID string) (Set, error) {
	if !id.IsUUID(petID) || !id.IsUUID(documentID) {
		return Set{}, ErrInvalid
	}
	return s.repository.GetDocument(ctx, userID, petID, documentID)
}

func (s *Service) ReplaceDocument(ctx context.Context, userID, petID, documentID string, set Set) (Set, error) {
	if !id.IsUUID(petID) || !id.IsUUID(documentID) || !validSet(set) {
		return Set{}, ErrInvalid
	}
	return s.repository.ReplaceDocument(ctx, userID, petID, documentID, set)
}

func (s *Service) GetExpense(ctx context.Context, userID, expenseID string) (Set, error) {
	if !id.IsUUID(expenseID) {
		return Set{}, ErrInvalid
	}
	return s.repository.GetExpense(ctx, userID, expenseID)
}

func (s *Service) ReplaceExpense(ctx context.Context, userID, expenseID string, set Set) (Set, error) {
	if !id.IsUUID(expenseID) || !validSet(set) {
		return Set{}, ErrInvalid
	}
	return s.repository.ReplaceExpense(ctx, userID, expenseID, set)
}

func validSet(set Set) bool {
	return validIDs(set.MedicalRecordIDs) && validIDs(set.AppointmentIDs)
}

func validIDs(values []string) bool {
	if len(values) > MaxLinksPerType {
		return false
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !id.IsUUID(value) {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return true
}
