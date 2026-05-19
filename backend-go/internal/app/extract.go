package app

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ledongthuc/pdf"
	"github.com/nguyenthenguyen/docx"
	"github.com/xuri/excelize/v2"

	"mycloud/backend-go/internal/storage"
)

// extractTextLimit caps how much text we store per file. Larger than this
// is rare in practice; truncation keeps MEDIUMTEXT rows manageable and
// FULLTEXT indexing fast.
const extractTextLimit = 1 << 20 // 1 MiB

// processExtract decrypts the file at fileID, extracts plain text from the
// supported formats, and upserts it into file_text. Called by the q:extract
// worker after a successful upload.
func (a *App) processExtract(ctx context.Context, fileID string) error {
	var mimeType, encKey, iv, tag, blobPath string
	err := a.DB.QueryRowContext(ctx, `
		SELECT mime_type, encryption_key_enc, encryption_iv, encryption_tag, storage_path
		FROM files WHERE id = ? AND is_deleted = 0`, fileID,
	).Scan(&mimeType, &encKey, &iv, &tag, &blobPath)
	if err != nil {
		return fmt.Errorf("load file %s: %w", fileID, err)
	}

	fileKey, err := storage.UnwrapKey(a.Config.MasterEncryptionKey, storage.EncryptedKeyBundle{
		IVHex: iv, EncKeyHex: encKey, TagHex: tag,
	})
	if err != nil {
		return fmt.Errorf("unwrap key: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "mycloud-extract-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	tmpPath := filepath.Join(tmpDir, fileID)
	tmpFile, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if err := storage.DecryptFileToWriter(blobPath, tmpFile, fileKey); err != nil {
		_ = tmpFile.Close()
		return fmt.Errorf("decrypt: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return err
	}

	text, err := extractByMime(tmpPath, mimeType)
	if err != nil {
		return fmt.Errorf("extract: %w", err)
	}
	if len(text) > extractTextLimit {
		text = text[:extractTextLimit]
	}
	if strings.TrimSpace(text) == "" {
		return nil
	}

	if _, err := a.DB.ExecContext(ctx, `
		INSERT INTO file_text (file_id, content, indexed_at)
		VALUES (?, ?, ?)
		ON DUPLICATE KEY UPDATE content = VALUES(content), indexed_at = VALUES(indexed_at)`,
		fileID, text, time.Now().UTC()); err != nil {
		return fmt.Errorf("upsert file_text: %w", err)
	}
	return nil
}

func extractByMime(path, mimeType string) (string, error) {
	mt := strings.ToLower(mimeType)
	switch {
	case strings.HasPrefix(mt, "text/"):
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		return string(b), nil
	case mt == "application/pdf":
		return extractPDF(path)
	case strings.Contains(mt, "wordprocessingml") || mt == "application/msword":
		return extractDocx(path)
	case strings.Contains(mt, "spreadsheetml") ||
		strings.Contains(mt, "ms-excel") ||
		strings.Contains(mt, "excel"):
		return extractXlsx(path)
	default:
		return "", nil // unsupported mime — silently skip
	}
}

func extractPDF(path string) (string, error) {
	f, r, err := pdf.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var buf bytes.Buffer
	for i := 1; i <= r.NumPage(); i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		s, err := p.GetPlainText(nil)
		if err != nil {
			continue
		}
		buf.WriteString(s)
		buf.WriteString("\n")
		if buf.Len() > extractTextLimit {
			break
		}
	}
	return buf.String(), nil
}

func extractDocx(path string) (string, error) {
	r, err := docx.ReadDocxFile(path)
	if err != nil {
		return "", err
	}
	defer r.Close()
	return r.Editable().GetContent(), nil
}

func extractXlsx(path string) (string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	var buf bytes.Buffer
	for _, sheet := range f.GetSheetList() {
		rows, err := f.GetRows(sheet)
		if err != nil {
			continue
		}
		for _, row := range rows {
			for _, cell := range row {
				if strings.TrimSpace(cell) == "" {
					continue
				}
				buf.WriteString(cell)
				buf.WriteByte(' ')
			}
			buf.WriteByte('\n')
			if buf.Len() > extractTextLimit {
				return buf.String(), nil
			}
		}
	}
	return buf.String(), nil
}

// noop reference so imports stick when extraction isn't compiled in some path.
var _ = io.Discard
