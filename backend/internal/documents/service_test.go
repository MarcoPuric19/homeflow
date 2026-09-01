package documents

import (
	"bytes"
	"context"
	"homeflow/backend/internal/receipts"
	"mime/multipart"
	"os"
	"testing"
)

type fakeRepository struct{ document Document }

func (r *fakeRepository) Create(_ context.Context, _ string, d Document) (Document, error) {
	r.document = d
	return d, nil
}
func (r *fakeRepository) List(context.Context, string, string, int) ([]Document, error) {
	return []Document{r.document}, nil
}
func (r *fakeRepository) Get(_ context.Context, _, _, id string) (Document, error) {
	if id != r.document.ID {
		return Document{}, ErrNotFound
	}
	return r.document, nil
}

type memoryFile struct{ *bytes.Reader }

func (*memoryFile) Close() error                            { return nil }
func (*memoryFile) ReadAt(p []byte, off int64) (int, error) { return 0, os.ErrInvalid }

func TestUploadListAndContent(t *testing.T) {
	repo := &fakeRepository{}
	storage, err := receipts.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, storage)
	file := struct{ multipart.File }{&memoryFile{bytes.NewReader([]byte("%PDF-1.7\ndocument"))}}
	doc, err := service.Upload(context.Background(), "user", "00000000-0000-4000-8000-000000000010", file, "../Befund.pdf", Metadata{Title: "Befund", DocumentType: "report"})
	if err != nil {
		t.Fatal(err)
	}
	if doc.OriginalFilename != "Befund.pdf" || doc.SHA256 == "" || doc.StorageKey == "" {
		t.Fatalf("document=%#v", doc)
	}
	_, body, err := service.Content(context.Background(), "user", doc.PetID, doc.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer body.Close()
}

func TestRejectInvalidMetadata(t *testing.T) {
	storage, _ := receipts.NewLocalStorage(t.TempDir())
	service := NewService(&fakeRepository{}, storage)
	file := struct{ multipart.File }{&memoryFile{bytes.NewReader([]byte("%PDF-1.7\nx"))}}
	if _, err := service.Upload(context.Background(), "user", "bad", file, "x.pdf", Metadata{Title: "x", DocumentType: "report"}); err != ErrInvalid {
		t.Fatalf("err=%v", err)
	}
}
