package storage

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Path-construction helpers — pure functions, just verify the layout.

func TestFinalPath(t *testing.T) {
	got := FinalPath("/data", "u-1", "file-123")
	want := filepath.Join("/data", "u-1", "file-123.enc")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestTempPath(t *testing.T) {
	got := TempPath("/data", "file-123")
	want := filepath.Join("/data", "tmp", "file-123.enc.tmp")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestVersionPath(t *testing.T) {
	got := VersionPath("/data", "u-1", "file-123", 5)
	want := filepath.Join("/data", "u-1", "file-123.v5.enc")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// EnsureUserDir creates the directory if missing and returns the path.
func TestEnsureUserDir_Creates(t *testing.T) {
	base := t.TempDir()
	dir, err := EnsureUserDir(base, "u-1")
	if err != nil {
		t.Fatalf("EnsureUserDir: %v", err)
	}
	if dir != filepath.Join(base, "u-1") {
		t.Errorf("returned path wrong: %q", dir)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Errorf("expected directory")
	}
}

// EnsureUserDir is idempotent — calling on an existing dir is OK.
func TestEnsureUserDir_Idempotent(t *testing.T) {
	base := t.TempDir()
	if _, err := EnsureUserDir(base, "u-1"); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if _, err := EnsureUserDir(base, "u-1"); err != nil {
		t.Fatalf("second call: %v", err)
	}
}

// decryptLegacy: read a file produced by the OLD format (no header,
// nonce + ciphertext + tag). Synthesise such a file and verify decryption
// works — covers the backward-compat branch of DecryptFileToWriter.
func TestDecryptLegacy_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.enc")
	plain := []byte("legacy plaintext content")
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}

	// Encrypt in the legacy single-shot format.
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		t.Fatalf("rand: %v", err)
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	// Legacy file format: nonce || sealed (sealed already includes tag).
	if err := os.WriteFile(path, append(nonce, sealed...), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// DecryptFileToWriter should auto-detect the legacy format (no
	// MCv2 magic) and route to decryptLegacy.
	var got bytes.Buffer
	if err := DecryptFileToWriter(path, &got, key); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got.Bytes(), plain) {
		t.Errorf("round-trip mismatch")
	}
}

// Legacy file too small (less than nonce + tag) → error.
func TestDecryptLegacy_FileTooSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tiny.enc")
	if err := os.WriteFile(path, []byte{1, 2, 3}, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	var buf bytes.Buffer
	if err := DecryptFileToWriter(path, &buf, key); err == nil {
		t.Errorf("expected error on too-small file")
	}
}

// Legacy file tampered ciphertext → auth fail.
func TestDecryptLegacy_TamperedCiphertext(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tampered.enc")
	plain := []byte("data")
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	block, _ := aes.NewCipher(key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	_, _ = rand.Read(nonce)
	sealed := gcm.Seal(nil, nonce, plain, nil)
	raw := append(nonce, sealed...)
	if len(raw) > 15 {
		raw[15] ^= 0xff
	}
	_ = os.WriteFile(path, raw, 0o644)

	var buf bytes.Buffer
	if err := DecryptFileToWriter(path, &buf, key); err == nil {
		t.Errorf("expected auth failure on tampered legacy file")
	}
}

// EncryptStream error paths: write to a closed file.
func TestEncryptStream_DstWriteError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.enc")
	f, _ := os.Create(path)
	f.Close() // closed before use → all writes fail
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	_, err := EncryptStream(bytes.NewReader([]byte("data")), f, key)
	if err == nil {
		t.Errorf("expected write error to propagate")
	}
}

// EncryptStream with wrong-length key returns AES init error.
func TestEncryptStream_BadKeyLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.enc")
	f, _ := os.Create(path)
	defer f.Close()
	_, err := EncryptStream(bytes.NewReader([]byte("data")), f, []byte{0x01, 0x02})
	if err == nil {
		t.Errorf("expected key-length error")
	}
}

// DecryptFileToWriter on non-existent file.
func TestDecryptFileToWriter_FileNotFound(t *testing.T) {
	var buf bytes.Buffer
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	if err := DecryptFileToWriter(filepath.Join(t.TempDir(), "missing"), &buf, key); err == nil {
		t.Errorf("expected file-not-found error")
	}
}

// DecryptFileToWriter with wrong-length key (AES init fails).
func TestDecryptFileToWriter_BadKeyLength(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.enc")
	// Make a syntactically-valid encrypted file first.
	out, _ := os.Create(path)
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	_, _ = EncryptStream(bytes.NewReader([]byte("d")), out, key)
	out.Close()

	var buf bytes.Buffer
	if err := DecryptFileToWriter(path, &buf, []byte{0x01, 0x02}); err == nil {
		t.Errorf("expected key-length error on decrypt")
	}
}

// Multi-chunk stream — verify the chunk loop handles content larger than
// one chunk (4 MB) correctly.
func TestEncryptStream_MultiChunk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.enc")
	out, _ := os.Create(path)
	defer out.Close()

	// 9 MB of varied content to span multiple chunks.
	plain := make([]byte, 9*1024*1024)
	if _, err := rand.Read(plain); err != nil {
		t.Fatalf("rand: %v", err)
	}
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	n, err := EncryptStream(bytes.NewReader(plain), out, key)
	if err != nil {
		t.Fatalf("EncryptStream: %v", err)
	}
	if n != int64(len(plain)) {
		t.Errorf("size mismatch: %d vs %d", n, len(plain))
	}
	out.Close()

	var got bytes.Buffer
	if err := DecryptFileToWriter(path, &got, key); err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if !bytes.Equal(got.Bytes(), plain) {
		t.Errorf("multi-chunk round-trip mismatch")
	}
}

// WrapKey with malformed hex master returns hex decode error.
func TestWrapKey_BadHex(t *testing.T) {
	if _, err := WrapKey("nothex!!!", make([]byte, 32)); err == nil {
		t.Errorf("expected hex decode error")
	}
}

// UnwrapKey: malformed bundle fields each return their own error.
func TestUnwrapKey_BadIVHex(t *testing.T) {
	master := strings.Repeat("a", 64) // 32 bytes hex
	if _, err := UnwrapKey(master, EncryptedKeyBundle{IVHex: "zzz", EncKeyHex: "00", TagHex: "00"}); err == nil {
		t.Errorf("expected error on bad IV hex")
	}
}

func TestUnwrapKey_BadEncKeyHex(t *testing.T) {
	master := strings.Repeat("a", 64)
	if _, err := UnwrapKey(master, EncryptedKeyBundle{IVHex: "00", EncKeyHex: "zzz", TagHex: "00"}); err == nil {
		t.Errorf("expected error on bad EncKey hex")
	}
}

func TestUnwrapKey_BadTagHex(t *testing.T) {
	master := strings.Repeat("a", 64)
	if _, err := UnwrapKey(master, EncryptedKeyBundle{IVHex: "00", EncKeyHex: "00", TagHex: "zzz"}); err == nil {
		t.Errorf("expected error on bad Tag hex")
	}
}

func TestUnwrapKey_BadMasterHex(t *testing.T) {
	if _, err := UnwrapKey("nothex!", EncryptedKeyBundle{IVHex: "00", EncKeyHex: "00", TagHex: "00"}); err == nil {
		t.Errorf("expected error on bad master hex")
	}
}

// Sanity: format string of VersionPath uses %s.v%d.enc shape.
func TestVersionPath_Format(t *testing.T) {
	got := VersionPath("/data", "u-1", "file-x", 42)
	if !strings.HasSuffix(got, ".v42.enc") {
		t.Errorf("expected .v42.enc suffix, got %q", got)
	}
	// Use fmt-printf to confirm independent of filepath.Join order on
	// different OSes.
	_ = fmt.Sprintf("just-here-so-fmt-stays-imported")
}
