package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// 32 bytes for AES-256.
func TestGenerateFileKey_Length(t *testing.T) {
	k, err := GenerateFileKey()
	if err != nil {
		t.Fatalf("GenerateFileKey: %v", err)
	}
	if len(k) != 32 {
		t.Errorf("expected 32-byte key, got %d", len(k))
	}
}

// Two successive calls must not collide — if they do, the RNG is broken.
func TestGenerateFileKey_Unique(t *testing.T) {
	a, _ := GenerateFileKey()
	b, _ := GenerateFileKey()
	if bytes.Equal(a, b) {
		t.Errorf("two random keys should not collide")
	}
}

// Round-trip wrap then unwrap yields the original file key. Foundational
// guarantee: a misconfig that broke this would silently corrupt every blob.
func TestWrapKey_UnwrapKey_RoundTrip(t *testing.T) {
	master := hex.EncodeToString(randBytes(t, 32))
	fileKey := randBytes(t, 32)
	bundle, err := WrapKey(master, fileKey)
	if err != nil {
		t.Fatalf("WrapKey: %v", err)
	}
	if bundle.IVHex == "" || bundle.EncKeyHex == "" || bundle.TagHex == "" {
		t.Errorf("bundle has empty fields: %+v", bundle)
	}
	got, err := UnwrapKey(master, bundle)
	if err != nil {
		t.Fatalf("UnwrapKey: %v", err)
	}
	if !bytes.Equal(got, fileKey) {
		t.Errorf("round-trip mismatch")
	}
}

// Tampering with the ciphertext, IV, or tag breaks GCM authentication —
// UnwrapKey must refuse. This is the property that lets us treat blob_refs
// as the integrity gate.
func TestUnwrapKey_TamperedCiphertext(t *testing.T) {
	master := hex.EncodeToString(randBytes(t, 32))
	fileKey := randBytes(t, 32)
	bundle, _ := WrapKey(master, fileKey)
	raw, _ := hex.DecodeString(bundle.EncKeyHex)
	raw[0] ^= 0xff
	bundle.EncKeyHex = hex.EncodeToString(raw)
	if _, err := UnwrapKey(master, bundle); err == nil {
		t.Errorf("UnwrapKey should reject tampered ciphertext")
	}
}

func TestUnwrapKey_TamperedTag(t *testing.T) {
	master := hex.EncodeToString(randBytes(t, 32))
	fileKey := randBytes(t, 32)
	bundle, _ := WrapKey(master, fileKey)
	raw, _ := hex.DecodeString(bundle.TagHex)
	raw[0] ^= 0x01
	bundle.TagHex = hex.EncodeToString(raw)
	if _, err := UnwrapKey(master, bundle); err == nil {
		t.Errorf("UnwrapKey should reject tampered tag")
	}
}

func TestUnwrapKey_WrongMasterKey(t *testing.T) {
	master := hex.EncodeToString(randBytes(t, 32))
	wrong := hex.EncodeToString(randBytes(t, 32))
	fileKey := randBytes(t, 32)
	bundle, _ := WrapKey(master, fileKey)
	if _, err := UnwrapKey(wrong, bundle); err == nil {
		t.Errorf("UnwrapKey should fail with wrong master key")
	}
}

func TestWrapKey_BadMasterKey(t *testing.T) {
	// Non-hex string fails decode.
	if _, err := WrapKey("not-hex", []byte("filekey")); err == nil {
		t.Errorf("WrapKey should reject non-hex master")
	}
	// Wrong length (must be 16/24/32 bytes for AES).
	if _, err := WrapKey(hex.EncodeToString([]byte{0x01, 0x02}), []byte("filekey")); err == nil {
		t.Errorf("WrapKey should reject short master")
	}
}

// Stream round-trip: encrypt to disk, decrypt back, compare.
func TestEncryptStream_DecryptFileToWriter_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.enc")
	plain := []byte("hello world, this is a streaming round-trip test of arbitrary length: " +
		strings.Repeat("abc-", 1000))

	key := randBytes(t, 32)
	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := EncryptStream(bytes.NewReader(plain), out, key)
	if err != nil {
		t.Fatalf("EncryptStream: %v", err)
	}
	out.Close()
	if n != int64(len(plain)) {
		t.Errorf("EncryptStream returned size %d, want %d", n, len(plain))
	}

	var got bytes.Buffer
	if err := DecryptFileToWriter(path, &got, key); err != nil {
		t.Fatalf("DecryptFileToWriter: %v", err)
	}
	if !bytes.Equal(got.Bytes(), plain) {
		t.Errorf("round-trip mismatch (got %d bytes, want %d)", got.Len(), len(plain))
	}
}

// A flipped ciphertext byte should make decryption fail — proves GCM auth
// runs at the per-chunk level, not just at the bundle level.
func TestDecryptFileToWriter_TamperedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.enc")
	key := randBytes(t, 32)

	out, _ := os.Create(path)
	_, _ = EncryptStream(bytes.NewReader([]byte("payload")), out, key)
	out.Close()

	// Read, flip a byte well past the magic prefix, write back.
	raw, _ := os.ReadFile(path)
	// Magic is 4 bytes; len prefix is next 4; nonce is next ~12. Flip in
	// the ciphertext region (offset 20+).
	if len(raw) > 25 {
		raw[25] ^= 0xff
	}
	_ = os.WriteFile(path, raw, 0o644)

	var got bytes.Buffer
	if err := DecryptFileToWriter(path, &got, key); err == nil {
		t.Errorf("expected decryption to fail on tampered file")
	}
}

func TestDecryptFileToWriter_MissingFile(t *testing.T) {
	var buf bytes.Buffer
	if err := DecryptFileToWriter(filepath.Join(t.TempDir(), "does-not-exist"), &buf, randBytes(t, 32)); err == nil {
		t.Errorf("expected error for missing file")
	}
}

// Empty input is a legal upload (a zero-byte file). The stream encoder
// should produce a header and decode back to empty.
func TestEncryptStream_EmptyInput(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.enc")
	key := randBytes(t, 32)
	out, _ := os.Create(path)
	n, err := EncryptStream(bytes.NewReader(nil), out, key)
	if err != nil {
		t.Fatalf("EncryptStream(empty): %v", err)
	}
	out.Close()
	if n != 0 {
		t.Errorf("expected 0 bytes encoded, got %d", n)
	}
	var got bytes.Buffer
	if err := DecryptFileToWriter(path, &got, key); err != nil {
		t.Fatalf("DecryptFileToWriter(empty): %v", err)
	}
	if got.Len() != 0 {
		t.Errorf("expected empty plaintext, got %d bytes", got.Len())
	}
}

// Bundle is unique per call because the IV is fresh. Wrapping the same
// file key twice must produce two distinct bundles — otherwise an attacker
// can detect duplicate file keys via the wrapped representation.
func TestWrapKey_BundlesAreUnique(t *testing.T) {
	master := hex.EncodeToString(randBytes(t, 32))
	key := randBytes(t, 32)
	a, _ := WrapKey(master, key)
	b, _ := WrapKey(master, key)
	if a.IVHex == b.IVHex {
		t.Errorf("WrapKey reused the IV — must be fresh per call")
	}
	if a.EncKeyHex == b.EncKeyHex {
		t.Errorf("WrapKey produced identical ciphertext for same input — IV is broken")
	}
}

// helper
func randBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}
	return b
}
