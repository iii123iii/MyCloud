package storage

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"io"
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

// smallReader returns at most `max` bytes per Read, simulating a source that
// delivers data in small pieces (e.g. a network multipart part). Proves
// EncryptStream produces a uniform chunk layout regardless of read granularity
// — the property DecryptRangeToWriter's arithmetic seek depends on.
type smallReader struct {
	data []byte
	max  int
}

func (s *smallReader) Read(p []byte) (int, error) {
	if len(s.data) == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > s.max {
		n = s.max
	}
	if n > len(s.data) {
		n = len(s.data)
	}
	copy(p, s.data[:n])
	s.data = s.data[n:]
	return n, nil
}

// Even when the source dribbles bytes in odd-sized reads, every chunk except
// the last must be exactly chunkSize, so the on-disk size matches the uniform
// layout formula. Without io.ReadFull this would produce many tiny chunks.
func TestEncryptStream_UniformChunkLayout(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.enc")
	key := randBytes(t, 32)
	plain := randBytes(t, 2*chunkSize+12345) // two full chunks + partial tail

	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	n, err := EncryptStream(&smallReader{data: append([]byte(nil), plain...), max: 7919}, out, key)
	if err != nil {
		t.Fatalf("EncryptStream: %v", err)
	}
	out.Close()
	if n != int64(len(plain)) {
		t.Fatalf("returned size %d, want %d", n, len(plain))
	}
	st, _ := os.Stat(path)
	if st.Size() != expectedEncryptedSize(int64(len(plain))) {
		t.Errorf("on-disk size %d, expected uniform %d", st.Size(), expectedEncryptedSize(int64(len(plain))))
	}
	var got bytes.Buffer
	if err := DecryptFileToWriter(path, &got, key); err != nil {
		t.Fatalf("DecryptFileToWriter: %v", err)
	}
	if !bytes.Equal(got.Bytes(), plain) {
		t.Errorf("round-trip mismatch")
	}
}

// Byte ranges decrypt to exactly the corresponding plaintext slice, including
// chunk-boundary spans and the partial last chunk.
func TestDecryptRangeToWriter_Slices(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.enc")
	key := randBytes(t, 32)
	plain := randBytes(t, 2*chunkSize+5000)

	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := EncryptStream(bytes.NewReader(plain), out, key); err != nil {
		t.Fatalf("EncryptStream: %v", err)
	}
	out.Close()

	size := int64(len(plain))
	cases := []struct{ start, length int64 }{
		{0, 100},                // head of first chunk
		{chunkSize - 50, 100},   // spans chunk 0 → 1 boundary
		{chunkSize, 200},        // exact start of chunk 1
		{2*chunkSize + 1, 4999}, // into the partial last chunk
		{0, size},               // whole file via a single range
		{size - 1, 1},           // final byte
		{1234567, 1000000},      // arbitrary mid-file span
	}
	for _, c := range cases {
		var got bytes.Buffer
		if err := DecryptRangeToWriter(path, &got, key, c.start, c.length, size); err != nil {
			t.Fatalf("range [%d,+%d): %v", c.start, c.length, err)
		}
		want := plain[c.start : c.start+c.length]
		if !bytes.Equal(got.Bytes(), want) {
			t.Errorf("range [%d,+%d) mismatch: got %d bytes, want %d", c.start, c.length, got.Len(), len(want))
		}
	}
}

// A blob whose size doesn't match the uniform layout (legacy single-blob or
// pre-uniform-chunking) reports ErrRangeUnsupported so callers fall back to a
// full 200 stream.
func TestDecryptRangeToWriter_NonUniformFallback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.enc")
	if err := os.WriteFile(path, randBytes(t, 1234), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	var got bytes.Buffer
	err := DecryptRangeToWriter(path, &got, randBytes(t, 32), 0, 100, 5000)
	if !ErrRangeUnsupported(err) {
		t.Errorf("expected ErrRangeUnsupported, got %v", err)
	}
}

func TestDecryptRangeToWriter_OutOfBounds(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "blob.enc")
	key := randBytes(t, 32)
	out, _ := os.Create(path)
	_, _ = EncryptStream(bytes.NewReader(randBytes(t, 1000)), out, key)
	out.Close()
	var got bytes.Buffer
	if err := DecryptRangeToWriter(path, &got, key, 900, 200, 1000); err == nil {
		t.Errorf("expected out-of-bounds error for [900,1100) over size 1000")
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
