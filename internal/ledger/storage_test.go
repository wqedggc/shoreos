package ledger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStorageSavesInsideUserPrivateDirectory(t *testing.T) {
	storage, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	stored, err := storage.Save(42, "../../账单.csv", strings.NewReader("private content"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(filepath.ToSlash(stored.Key), "users/42/imports/") {
		t.Fatalf("unexpected private key: %s", stored.Key)
	}
	if strings.Contains(stored.Key, "..") {
		t.Fatalf("upload key contains traversal: %s", stored.Key)
	}
	content, err := os.ReadFile(stored.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "private content" {
		t.Fatalf("unexpected content: %q", content)
	}
	if err := storage.Remove(stored); err != nil {
		t.Fatal(err)
	}
}
