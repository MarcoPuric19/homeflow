package receipts

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"

	"homeflow/backend/internal/platform/id"
)

const MaxUploadBytes int64 = 10 * 1024 * 1024

var (
	ErrInvalidFile = errors.New("invalid receipt file")
	ErrTooLarge    = errors.New("receipt file too large")
)

type StoredFile struct {
	StorageKey       string
	OriginalFilename string
	MIMEType         string
	SHA256           string
}

type LocalStorage struct{ root string }

func NewLocalStorage(root string) (*LocalStorage, error) {
	if root == "" {
		return nil, errors.New("receipt storage path is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, fmt.Errorf("create receipt storage: %w", err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, errors.New("receipt storage must be a real directory")
	}
	return &LocalStorage{root: abs}, nil
}

func (s *LocalStorage) Store(file multipart.File, originalFilename string) (stored StoredFile, err error) {
	name := filepath.Base(strings.ReplaceAll(originalFilename, "\\", "/"))
	name = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, name)
	if name == "" || name == "." {
		name = "receipt"
	}

	header := make([]byte, 512)
	n, readErr := io.ReadFull(file, header)
	if readErr != nil && !errors.Is(readErr, io.EOF) && !errors.Is(readErr, io.ErrUnexpectedEOF) {
		return StoredFile{}, ErrInvalidFile
	}
	header = header[:n]
	mimeType, extension, ok := detectType(header)
	if !ok {
		return StoredFile{}, ErrInvalidFile
	}
	fileID, err := id.New()
	if err != nil {
		return StoredFile{}, err
	}
	storageKey := fileID + extension
	temp, err := os.CreateTemp(s.root, ".upload-*")
	if err != nil {
		return StoredFile{}, err
	}
	tempName := temp.Name()
	defer func() {
		_ = temp.Close()
		if err != nil {
			_ = os.Remove(tempName)
		}
	}()
	if err = temp.Chmod(0o600); err != nil {
		return StoredFile{}, err
	}
	hash := sha256.New()
	limited := io.LimitReader(io.MultiReader(bytes.NewReader(header), file), MaxUploadBytes+1)
	written, copyErr := io.Copy(io.MultiWriter(temp, hash), limited)
	if copyErr != nil {
		return StoredFile{}, copyErr
	}
	if written > MaxUploadBytes {
		return StoredFile{}, ErrTooLarge
	}
	if err = temp.Sync(); err != nil {
		return StoredFile{}, err
	}
	if err = temp.Close(); err != nil {
		return StoredFile{}, err
	}
	finalPath := filepath.Join(s.root, storageKey)
	if err = os.Rename(tempName, finalPath); err != nil {
		return StoredFile{}, err
	}
	return StoredFile{StorageKey: storageKey, OriginalFilename: name, MIMEType: mimeType,
		SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func (s *LocalStorage) Remove(storageKey string) error {
	if !validStorageKey(storageKey) {
		return ErrInvalidFile
	}
	return os.Remove(filepath.Join(s.root, storageKey))
}

// Open returns a read handle for a stored file. Storage keys are produced by
// Store and never taken from a request, but they are validated again here so
// that a corrupted database row cannot reach outside the storage root. The
// caller closes the handle.
func (s *LocalStorage) Open(storageKey string) (*os.File, error) {
	if !validStorageKey(storageKey) {
		return nil, ErrInvalidFile
	}
	file, err := os.Open(filepath.Join(s.root, storageKey))
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, ErrInvalidFile
	}
	return file, nil
}

func validStorageKey(storageKey string) bool {
	return storageKey != "" && storageKey != "." && filepath.Base(storageKey) == storageKey &&
		!strings.Contains(storageKey, "..") && !strings.ContainsRune(storageKey, os.PathSeparator)
}

func detectType(header []byte) (string, string, bool) {
	switch {
	case len(header) >= 3 && bytes.Equal(header[:3], []byte{0xff, 0xd8, 0xff}):
		return "image/jpeg", ".jpg", true
	case len(header) >= 8 && bytes.Equal(header[:8], []byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}):
		return "image/png", ".png", true
	case len(header) >= 5 && bytes.Equal(header[:5], []byte("%PDF-")):
		return "application/pdf", ".pdf", true
	default:
		return "", "", false
	}
}
