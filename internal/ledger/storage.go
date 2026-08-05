package ledger

import (
	"crypto/rand"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type StoredFile struct {
	Key  string
	Path string
}

type Storage struct {
	root string
}

func NewStorage(root string) (*Storage, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(absRoot, 0o700); err != nil {
		return nil, err
	}
	return &Storage{root: absRoot}, nil
}

func (s *Storage) Save(userID int64, originalName string, source io.Reader) (StoredFile, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return StoredFile{}, err
	}
	name := filepath.Base(strings.ReplaceAll(originalName, "\\", "/"))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return StoredFile{}, fmt.Errorf("invalid upload filename")
	}
	key := filepath.Join("users", strconv.FormatInt(userID, 10), "imports", fmt.Sprintf("%x-%s", nonce, name))
	path := filepath.Join(s.root, key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return StoredFile{}, err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return StoredFile{}, err
	}
	_, copyErr := io.Copy(file, source)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = os.Remove(path)
		if copyErr != nil {
			return StoredFile{}, copyErr
		}
		return StoredFile{}, closeErr
	}
	return StoredFile{Key: key, Path: path}, nil
}

func (s *Storage) Remove(file StoredFile) error {
	return os.Remove(file.Path)
}
