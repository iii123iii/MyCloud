package app

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/tus/tusd/v2/pkg/filelocker"
	"github.com/tus/tusd/v2/pkg/filestore"
	tusd "github.com/tus/tusd/v2/pkg/handler"

	"mycloud/backend-go/internal/storage"
)

// mountTus wires the tus 1.0 server at /api/v2/files/tus/*. Auth is enforced
// by the surrounding chi middleware (requireAuth), so all hooks have a valid
// user ID in their context.
//
// On a successful finish, the assembled plaintext is encrypted via our
// existing pipeline (same per-file AES-GCM keys, same on-disk layout) and a
// row is added to `files` (or a version is created via commitUploadWithVersioning).
func (a *App) mountTus(basePath string) (http.Handler, error) {
	tusDir := filepath.Join(a.Config.StoragePath, "tus")
	if err := os.MkdirAll(tusDir, 0o755); err != nil {
		return nil, err
	}

	store := filestore.New(tusDir)
	locker := filelocker.New(tusDir)
	composer := tusd.NewStoreComposer()
	store.UseIn(composer)
	locker.UseIn(composer)

	handler, err := tusd.NewHandler(tusd.Config{
		BasePath:      basePath,
		StoreComposer: composer,
		// nginx terminates TLS and forwards X-Forwarded-Proto=https. Without
		// this flag, tusd would build the Location header from the backend's
		// own http://backend:8080 view and the browser would then try to
		// PATCH http:// from an https:// origin (CORS preflight + redirect).
		RespectForwardedHeaders: true,
		PreUploadCreateCallback: func(hook tusd.HookEvent) (tusd.HTTPResponse, tusd.FileInfoChanges, error) {
			userID, _ := hook.Context.Value(userIDKey).(string)
			if userID == "" {
				return tusd.HTTPResponse{StatusCode: http.StatusUnauthorized}, tusd.FileInfoChanges{},
					errors.New("unauthorized")
			}
			folderID := hook.Upload.MetaData["folder_id"]
			if folderID != "" {
				if err := a.canAccessFolder(hook.Context, userID, folderID, AccessEditor); err != nil {
					return tusd.HTTPResponse{StatusCode: http.StatusForbidden}, tusd.FileInfoChanges{},
						fmt.Errorf("folder not accessible")
				}
			}
			return tusd.HTTPResponse{}, tusd.FileInfoChanges{}, nil
		},
		PreFinishResponseCallback: func(hook tusd.HookEvent) (tusd.HTTPResponse, error) {
			userID, _ := hook.Context.Value(userIDKey).(string)
			if userID == "" {
				return tusd.HTTPResponse{}, errors.New("unauthorized")
			}
			filename := strings.TrimSpace(hook.Upload.MetaData["filename"])
			if filename == "" {
				filename = "untitled"
			}
			folderID := hook.Upload.MetaData["folder_id"]
			mimeType := detectMime(filename)

			// Locate the tus blob. filestore writes the assembled file at
			// {tusDir}/{id} with no extension.
			tusPath := filepath.Join(tusDir, hook.Upload.ID)
			plaintext, err := os.Open(tusPath)
			if err != nil {
				return tusd.HTTPResponse{}, fmt.Errorf("open tus file: %w", err)
			}
			defer plaintext.Close()

			fileKey, err := storage.GenerateFileKey()
			if err != nil {
				return tusd.HTTPResponse{}, err
			}
			bundle, err := storage.WrapKey(a.Config.MasterEncryptionKey, fileKey)
			if err != nil {
				return tusd.HTTPResponse{}, err
			}

			tmpPath := storage.TempPath(a.Config.StoragePath, "tus-"+hook.Upload.ID)
			if err := os.MkdirAll(filepath.Dir(tmpPath), 0o755); err != nil {
				return tusd.HTTPResponse{}, err
			}
			tmpFile, err := os.Create(tmpPath)
			if err != nil {
				return tusd.HTTPResponse{}, err
			}
			hasher := sha256.New()
			teed := io.TeeReader(plaintext, hasher)
			size, encErr := storage.EncryptStream(teed, tmpFile, fileKey)
			closeErr := tmpFile.Close()
			if encErr != nil {
				_ = os.Remove(tmpPath)
				return tusd.HTTPResponse{}, encErr
			}
			if closeErr != nil {
				_ = os.Remove(tmpPath)
				return tusd.HTTPResponse{}, closeErr
			}
			contentHash := hex.EncodeToString(hasher.Sum(nil))

			if _, err := storage.EnsureUserDir(a.Config.StoragePath, userID); err != nil {
				_ = os.Remove(tmpPath)
				return tusd.HTTPResponse{}, err
			}

			commit, err := a.commitUploadWithVersioning(hook.Context, userID, filename, folderID, mimeType, contentHash, size, bundle, tmpPath)
			if err != nil {
				_ = os.Remove(tmpPath)
				code := http.StatusInternalServerError
				if errors.Is(err, ErrQuotaExceeded) {
					code = http.StatusRequestEntityTooLarge
				}
				return tusd.HTTPResponse{StatusCode: code}, err
			}

			// Clean up tus state.
			_ = os.Remove(tusPath)
			_ = os.Remove(tusPath + ".info")

			uid := userID
			writeActivity(hook.Context, a.DB, &uid, "file.upload", "file", commit.fileID,
				hook.HTTPRequest.RemoteAddr, map[string]any{
					"name":       filename,
					"size":       size,
					"versioned":  commit.versioned,
					"version_no": commit.newVersionNo,
					"via":        "tus",
				})
			a.afterUploadCommit(hook.Context, commit.fileID, mimeType)

			return tusd.HTTPResponse{
				StatusCode: http.StatusCreated,
				Header:     tusd.HTTPHeader{"X-File-ID": commit.fileID},
			}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return handler, nil
}
