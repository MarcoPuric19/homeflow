package receipts

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func uploadFile(t *testing.T, data []byte) *os.File {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "upload")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if _, err := file.Seek(0, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestStoreValidPNGWithOpaqueName(t *testing.T) {
	root := t.TempDir()
	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	png := append([]byte{0x89, 'P', 'N', 'G', 0x0d, 0x0a, 0x1a, 0x0a}, bytes.Repeat([]byte{0}, 32)...)
	stored, err := storage.Store(uploadFile(t, png), "../../private/receipt.png")
	if err != nil {
		t.Fatal(err)
	}
	if stored.OriginalFilename != "receipt.png" || stored.MIMEType != "image/png" || !strings.HasSuffix(stored.StorageKey, ".png") {
		t.Fatalf("unexpected result: %#v", stored)
	}
	if strings.Contains(stored.StorageKey, "receipt") {
		t.Fatal("client filename leaked into storage key")
	}
	info, err := os.Stat(filepath.Join(root, stored.StorageKey))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode=%o, want 600", info.Mode().Perm())
	}
}

func TestRejectUnsupportedAndOversizedFiles(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Store(uploadFile(t, []byte("plain text")), "receipt.jpg"); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("err=%v", err)
	}
	large := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{'x'}, int(MaxUploadBytes))...)
	if _, err := storage.Store(uploadFile(t, large), "large.pdf"); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err=%v", err)
	}
}

func TestOpenReturnsStoredBytes(t *testing.T) {
	storage, err := NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	pdf := []byte("%PDF-1.7\nstored payload")
	stored, err := storage.Store(uploadFile(t, pdf), "receipt.pdf")
	if err != nil {
		t.Fatal(err)
	}
	file, err := storage.Open(stored.StorageKey)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	content, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, pdf) {
		t.Fatalf("content=%q, want %q", content, pdf)
	}
}

func TestOpenRefusesKeysOutsideTheStorageRoot(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside.pdf")
	if err := os.WriteFile(outside, []byte("%PDF-1.7\nsecret"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{
		"", ".", "..", "../outside.pdf", "../" + filepath.Base(outside),
		"sub/receipt.pdf", "/etc/passwd", filepath.Join(root, "escape.pdf"),
	} {
		if _, err := storage.Open(key); !errors.Is(err, ErrInvalidFile) {
			t.Fatalf("Open(%q) err=%v, want ErrInvalidFile", key, err)
		}
	}
	// A syntactically valid key that simply does not exist stays a plain
	// os.ErrNotExist so the handler can turn it into a 404.
	if _, err := storage.Open("00000000-0000-4000-8000-000000000001.pdf"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err=%v, want os.ErrNotExist", err)
	}
}

func TestOpenRefusesDirectories(t *testing.T) {
	root := t.TempDir()
	storage, err := NewLocalStorage(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "adirectory"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.Open("adirectory"); !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("err=%v, want ErrInvalidFile", err)
	}
}
