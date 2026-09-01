package shopping

import (
	"context"
	"sort"
	"sync"
	"time"
)

type MemoryRepository struct {
	mu    sync.RWMutex
	items map[string]Item
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{items: make(map[string]Item)}
}

func (r *MemoryRepository) Create(_ context.Context, _ string, item Item) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[item.ID] = item
	return item, nil
}

func (r *MemoryRepository) List(_ context.Context, _ string, listID string, includeCompleted bool, petID string, limit int) ([]Item, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Item, 0)
	for _, item := range r.items {
		matchesPet := petID == ""
		for _, candidate := range item.PetIDs {
			if candidate == petID {
				matchesPet = true
			}
		}
		if item.ListID == listID && (includeCompleted || !item.Completed) && matchesPet {
			items = append(items, item)
			if len(items) == limit {
				break
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].CreatedAt.Before(items[j].CreatedAt) })
	return items, nil
}

func (r *MemoryRepository) Lists(_ context.Context, _ string) ([]List, error) {
	return []List{{ID: SharedListID, Name: "Gemeinsame Einkaufsliste", Visibility: "shared"}}, nil
}

func (r *MemoryRepository) Complete(_ context.Context, itemID, userID string, completedAt time.Time) (Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	item, ok := r.items[itemID]
	if !ok {
		return Item{}, ErrNotFound
	}
	if item.Completed {
		return Item{}, ErrAlreadyDone
	}
	item.Completed = true
	item.PurchasedBy = userID
	item.CompletedAt = &completedAt
	item.UpdatedAt = completedAt
	r.items[itemID] = item
	return item, nil
}
