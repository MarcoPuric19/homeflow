package receipts

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"time"

	"homeflow/backend/internal/expenses"
	"homeflow/backend/internal/platform/id"
)

var (
	ErrInvalid  = errors.New("invalid receipt")
	ErrNotFound = errors.New("receipt not found")
)

type Repository interface {
	Create(context.Context, Receipt) (Receipt, error)
	ListForExpense(context.Context, string) ([]Receipt, error)
	GetForExpense(ctx context.Context, expenseID, receiptID string) (Receipt, error)
}

type Service struct {
	repository Repository
	expenses   *expenses.Service
	storage    *LocalStorage
	now        func() time.Time
}

func NewService(repository Repository, expenseService *expenses.Service, storage *LocalStorage) *Service {
	return &Service{repository: repository, expenses: expenseService, storage: storage, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Upload(ctx context.Context, expenseID, userID string, file multipart.File, filename string, metadata Metadata) (Receipt, error) {
	if !id.IsUUID(expenseID) {
		return Receipt{}, expenses.ErrNotFound
	}
	exists, err := s.expenses.Exists(ctx, userID, expenseID)
	if err != nil {
		return Receipt{}, err
	}
	if !exists {
		return Receipt{}, expenses.ErrNotFound
	}
	if metadata.Currency == "" {
		metadata.Currency = "EUR"
	}
	metadata.Currency = strings.ToUpper(metadata.Currency)
	if len(metadata.Currency) != 3 || (metadata.TotalAmountCents != nil && *metadata.TotalAmountCents < 0) {
		return Receipt{}, ErrInvalid
	}
	if metadata.ReceiptDate != nil {
		if _, err := time.Parse(time.DateOnly, *metadata.ReceiptDate); err != nil {
			return Receipt{}, ErrInvalid
		}
	}
	stored, err := s.storage.Store(file, filename)
	if err != nil {
		return Receipt{}, err
	}
	receiptID, err := id.New()
	if err != nil {
		_ = s.storage.Remove(stored.StorageKey)
		return Receipt{}, err
	}
	receipt := Receipt{ID: receiptID, ExpenseID: expenseID, Merchant: metadata.Merchant,
		ReceiptDate: metadata.ReceiptDate, TotalAmountCents: metadata.TotalAmountCents,
		Currency: metadata.Currency, Category: metadata.Category, OriginalFilename: stored.OriginalFilename,
		MIMEType: stored.MIMEType, SHA256: stored.SHA256, StorageKey: stored.StorageKey,
		UploadedAt: s.now(), UploadedBy: userID}
	created, err := s.repository.Create(ctx, receipt)
	if err != nil {
		_ = s.storage.Remove(stored.StorageKey)
		return Receipt{}, err
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, userID, expenseID string) ([]Receipt, error) {
	if !id.IsUUID(expenseID) {
		return nil, ErrInvalid
	}
	exists, err := s.expenses.Exists(ctx, userID, expenseID)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, expenses.ErrNotFound
	}
	return s.repository.ListForExpense(ctx, expenseID)
}

// Content returns the metadata of a stored receipt together with an open
// handle to its bytes. The receipt must belong to the given expense, so a
// receipt id alone is not enough to read a file. The caller closes the handle.
func (s *Service) Content(ctx context.Context, userID, expenseID, receiptID string) (Receipt, io.ReadSeekCloser, error) {
	if !id.IsUUID(expenseID) || !id.IsUUID(receiptID) {
		return Receipt{}, nil, ErrInvalid
	}
	exists, err := s.expenses.Exists(ctx, userID, expenseID)
	if err != nil {
		return Receipt{}, nil, err
	}
	if !exists {
		return Receipt{}, nil, ErrNotFound
	}
	receipt, err := s.repository.GetForExpense(ctx, expenseID, receiptID)
	if err != nil {
		return Receipt{}, nil, err
	}
	file, err := s.storage.Open(receipt.StorageKey)
	if err != nil {
		return Receipt{}, nil, err
	}
	return receipt, file, nil
}
