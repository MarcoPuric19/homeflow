package shopping

import (
	"context"
	"errors"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
)

var (
	ErrNotFound    = errors.New("shopping item not found")
	ErrInvalid     = errors.New("invalid shopping item")
	ErrAlreadyDone = errors.New("shopping item already completed")
)

type Repository interface {
	Create(ctx context.Context, userID string, item Item) (Item, error)
	List(ctx context.Context, userID, listID string, includeCompleted bool, petID string, limit int) ([]Item, error)
	Lists(ctx context.Context, userID string) ([]List, error)
	Complete(ctx context.Context, itemID, userID string, completedAt time.Time) (Item, error)
}

type Service struct {
	repository Repository
	now        func() time.Time
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, input CreateItem, userID string) (Item, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Quantity = strings.TrimSpace(input.Quantity)
	if input.Name == "" || len(input.Name) > 200 || input.Quantity == "" || len(input.Quantity) > 40 {
		return Item{}, ErrInvalid
	}
	if input.ListID == "" {
		input.ListID = "shared"
	}
	if input.Priority == "" {
		input.Priority = PriorityNormal
	}
	if input.Priority != PriorityLow && input.Priority != PriorityNormal && input.Priority != PriorityHigh {
		return Item{}, ErrInvalid
	}
	if len(input.PetIDs) > 20 {
		return Item{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, petID := range input.PetIDs {
		if !id.IsUUID(petID) || seen[petID] {
			return Item{}, ErrInvalid
		}
		seen[petID] = true
	}
	itemID, err := id.New()
	if err != nil {
		return Item{}, err
	}
	now := s.now()
	return s.repository.Create(ctx, userID, Item{
		ID: itemID, ListID: input.ListID, Name: input.Name, Quantity: input.Quantity,
		Unit: input.Unit, Category: input.Category, Note: input.Note, Priority: input.Priority,
		Store: input.Store, AddedBy: userID, CreatedAt: now, UpdatedAt: now, PetIDs: input.PetIDs,
	})
}

func (s *Service) List(ctx context.Context, userID, listID string, includeCompleted bool, petID string, limit int) ([]Item, error) {
	if listID == "" {
		listID = "shared"
	}
	if petID != "" && !id.IsUUID(petID) {
		return nil, ErrInvalid
	}
	if limit == 0 {
		limit = 100
	}
	if limit < 1 || limit > 100 {
		return nil, ErrInvalid
	}
	return s.repository.List(ctx, userID, listID, includeCompleted, petID, limit)
}

func (s *Service) Lists(ctx context.Context, userID string) ([]List, error) {
	return s.repository.Lists(ctx, userID)
}

func (s *Service) Complete(ctx context.Context, itemID, userID string) (Item, error) {
	if !id.IsUUID(itemID) {
		return Item{}, ErrInvalid
	}
	return s.repository.Complete(ctx, itemID, userID, s.now())
}
