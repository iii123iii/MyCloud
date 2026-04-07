package services

import (
	"fmt"
	"os"
	"path/filepath"
)

// StorageService manages file storage on disk.
type StorageService struct {
	basePath string
}

func NewStorageService(basePath string) *StorageService {
	return &StorageService{basePath: basePath}
}

// FilePath returns the on-disk path for a stored file.
func (s *StorageService) FilePath(userID, fileID string) string {
	return filepath.Join(s.basePath, userID, fileID)
}

// StoreFile encrypts the given bytes and writes them to disk.
// Returns the EncryptedKeyBundle to be stored in the database.
func (s *StorageService) StoreFile(userID, fileID string, data []byte) (*EncryptedKeyBundle, error) {
	dir := filepath.Join(s.basePath, userID)
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("cannot create storage dir: %w", err)
	}

	fileKey, err := GenerateFileKey()
	if err != nil {
		return nil, err
	}

	path := filepath.Join(dir, fileID)
	if err := EncryptToFile(data, fileKey, path); err != nil {
		return nil, err
	}

	bundle, err := WrapKey(fileKey)
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return bundle, nil
}

// DeleteFile removes the on-disk file (best effort; ignores not-found errors).
func (s *StorageService) DeleteFile(userID, fileID string) {
	_ = os.Remove(s.FilePath(userID, fileID))
}
