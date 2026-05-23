package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// extractByMime is package-local; covers the dispatch switch.
func TestExtractByMime_TextFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(path, []byte("hello plain text"), 0o644)
	got, err := extractByMime(path, "text/plain")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if !strings.Contains(got, "hello") {
		t.Errorf("expected text contents, got %q", got)
	}
}

func TestExtractByMime_TextFileReadError(t *testing.T) {
	_, err := extractByMime("/no/such/file", "text/plain")
	if err == nil {
		t.Errorf("expected error from missing file")
	}
}

// Default branch: unsupported MIME returns empty + nil.
func TestExtractByMime_UnsupportedMime(t *testing.T) {
	got, err := extractByMime("anything", "application/octet-stream")
	if err != nil {
		t.Errorf("expected nil error for unsupported MIME, got %v", err)
	}
	if got != "" {
		t.Errorf("expected empty string, got %q", got)
	}
}

// PDF / DOCX / XLSX paths require their respective files; we test the
// error branch (file missing/invalid) to cover the early return.
func TestExtractPDF_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not.pdf")
	_ = os.WriteFile(path, []byte("not actually pdf"), 0o644)
	if _, err := extractPDF(path); err == nil {
		t.Errorf("expected error for non-PDF input")
	}
}

func TestExtractDocx_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not.docx")
	_ = os.WriteFile(path, []byte("not actually docx"), 0o644)
	if _, err := extractDocx(path); err == nil {
		t.Errorf("expected error for non-DOCX input")
	}
}

func TestExtractXlsx_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "not.xlsx")
	_ = os.WriteFile(path, []byte("not actually xlsx"), 0o644)
	if _, err := extractXlsx(path); err == nil {
		t.Errorf("expected error for non-XLSX input")
	}
}

// extractByMime routes pdf/docx/xlsx to the right helper — verify via
// MIME dispatch on a non-existent file (so we exercise the switch
// branches without needing real fixtures).
func TestExtractByMime_DispatchPDF(t *testing.T) {
	_, err := extractByMime("/no/such/file.pdf", "application/pdf")
	if err == nil {
		t.Errorf("expected error from missing PDF")
	}
}

func TestExtractByMime_DispatchDocx(t *testing.T) {
	_, err := extractByMime("/no/such/file.docx",
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document")
	if err == nil {
		t.Errorf("expected error from missing DOCX")
	}
}

func TestExtractByMime_DispatchXlsx(t *testing.T) {
	_, err := extractByMime("/no/such/file.xlsx",
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	if err == nil {
		t.Errorf("expected error from missing XLSX")
	}
}

func TestExtractByMime_DispatchMSWord(t *testing.T) {
	_, err := extractByMime("/no/such/file.doc", "application/msword")
	if err == nil {
		t.Errorf("expected error from missing DOC")
	}
}

func TestExtractByMime_DispatchMSExcel(t *testing.T) {
	_, err := extractByMime("/no/such/file.xls", "application/vnd.ms-excel")
	if err == nil {
		t.Errorf("expected error from missing XLS")
	}
}
