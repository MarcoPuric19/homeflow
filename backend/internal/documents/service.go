package documents

import (
	"context"
	"errors"
	"io"
	"mime/multipart"
	"strings"
	"time"

	"homeflow/backend/internal/platform/id"
	"homeflow/backend/internal/receipts"
)

var (
	ErrInvalid  = errors.New("invalid document")
	ErrNotFound = errors.New("document not found")
)

const MaxList = 100

type Repository interface {
	Create(context.Context, string, Document) (Document, error)
	List(context.Context, string, string, int) ([]Document, error)
	Get(context.Context, string, string, string) (Document, error)
}

type Service struct {
	repository Repository
	storage    *receipts.LocalStorage
	now        func() time.Time
}

func NewService(repository Repository, storage *receipts.LocalStorage) *Service {
	return &Service{repository: repository, storage: storage, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Upload(ctx context.Context, userID, petID string, file multipart.File, filename string, metadata Metadata) (Document, error) {
	metadata.Title = strings.TrimSpace(metadata.Title)
	metadata.DocumentType = strings.TrimSpace(metadata.DocumentType)
	metadata.Notes = strings.TrimSpace(metadata.Notes)
	if !id.IsUUID(petID) || metadata.Title == "" || len(metadata.Title) > 200 || len(metadata.Notes) > 2000 || !validType(metadata.DocumentType) {
		return Document{}, ErrInvalid
	}
	if metadata.OccurredOn != nil {
		value := strings.TrimSpace(*metadata.OccurredOn)
		date, err := time.Parse(time.DateOnly, value)
		if err != nil || date.After(s.now()) {
			return Document{}, ErrInvalid
		}
		metadata.OccurredOn = &value
	}
	stored, err := s.storage.Store(file, filename)
	if err != nil {
		return Document{}, err
	}
	documentID, err := id.New()
	if err != nil {
		_ = s.storage.Remove(stored.StorageKey)
		return Document{}, err
	}
	document := Document{ID: documentID, PetID: petID, Title: metadata.Title, DocumentType: metadata.DocumentType, OccurredOn: metadata.OccurredOn, Notes: metadata.Notes, OriginalFilename: stored.OriginalFilename, MIMEType: stored.MIMEType, SHA256: stored.SHA256, StorageKey: stored.StorageKey, UploadedBy: userID, UploadedAt: s.now()}
	created, err := s.repository.Create(ctx, userID, document)
	if err != nil {
		_ = s.storage.Remove(stored.StorageKey)
		return Document{}, err
	}
	return created, nil
}

func (s *Service) List(ctx context.Context, userID, petID string, limit int) ([]Document, error) {
	if !id.IsUUID(petID) {
		return nil, ErrInvalid
	}
	if limit == 0 {
		limit = MaxList
	}
	if limit < 1 || limit > MaxList {
		return nil, ErrInvalid
	}
	return s.repository.List(ctx, userID, petID, limit)
}

func (s *Service) Content(ctx context.Context, userID, petID, documentID string) (Document, io.ReadSeekCloser, error) {
	if !id.IsUUID(petID) || !id.IsUUID(documentID) {
		return Document{}, nil, ErrInvalid
	}
	document, err := s.repository.Get(ctx, userID, petID, documentID)
	if err != nil {
		return Document{}, nil, err
	}
	file, err := s.storage.Open(document.StorageKey)
	if err != nil {
		return Document{}, nil, err
	}
	return document, file, nil
}

func validType(value string) bool {
	switch value {
	case "report", "lab", "vaccination", "imaging", "insurance", "other":
		return true
	default:
		return false
	}
}
