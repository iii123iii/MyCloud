package storage

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	chunkSize = 4 * 1024 * 1024
	// On-disk per-chunk framing produced by EncryptStream:
	//   [lenPrefix(4)] [nonce(12)] [ciphertext(plainLen)] [tag(16)]
	// These match AES-GCM's standard nonce/tag sizes (cipher.NewGCM defaults).
	lenPrefixSize = 4
	gcmNonceSize  = 12
	gcmTagSize    = 16
	chunkOverhead = lenPrefixSize + gcmNonceSize + gcmTagSize // 32 bytes / chunk
)

var fileMagic = []byte{'M', 'C', 'v', '2'}

// errNonUniformLayout signals that a blob's on-disk size doesn't match the
// uniform fixed-size chunk layout, so byte offsets can't be computed
// arithmetically. Callers fall back to a full (non-range) decrypt. Legacy
// single-blob files and any pre-uniform-chunking uploads land here.
var errNonUniformLayout = errors.New("storage: non-uniform chunk layout; range unsupported")

// ErrRangeUnsupported reports whether err means the blob can't be served as a
// byte range (the caller should stream the whole file with a 200 instead).
func ErrRangeUnsupported(err error) bool { return errors.Is(err, errNonUniformLayout) }

type EncryptedKeyBundle struct {
	IVHex     string
	EncKeyHex string
	TagHex    string
}

func GenerateFileKey() ([]byte, error) {
	key := make([]byte, 32)
	_, err := rand.Read(key)
	return key, err
}

func WrapKey(masterKeyHex string, fileKey []byte) (EncryptedKeyBundle, error) {
	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return EncryptedKeyBundle{}, err
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return EncryptedKeyBundle{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return EncryptedKeyBundle{}, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return EncryptedKeyBundle{}, err
	}
	sealed := gcm.Seal(nil, nonce, fileKey, nil)
	tagStart := len(sealed) - gcm.Overhead()
	return EncryptedKeyBundle{
		IVHex:     hex.EncodeToString(nonce),
		EncKeyHex: hex.EncodeToString(sealed[:tagStart]),
		TagHex:    hex.EncodeToString(sealed[tagStart:]),
	}, nil
}

func UnwrapKey(masterKeyHex string, bundle EncryptedKeyBundle) ([]byte, error) {
	masterKey, err := hex.DecodeString(masterKeyHex)
	if err != nil {
		return nil, err
	}
	nonce, err := hex.DecodeString(bundle.IVHex)
	if err != nil {
		return nil, err
	}
	encKey, err := hex.DecodeString(bundle.EncKeyHex)
	if err != nil {
		return nil, err
	}
	tag, err := hex.DecodeString(bundle.TagHex)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(masterKey)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, append(encKey, tag...), nil)
}

func EncryptStream(src io.Reader, dst io.Writer, key []byte) (int64, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return 0, err
	}
	if _, err := dst.Write(fileMagic); err != nil {
		return 0, err
	}

	// io.ReadFull fills the whole buffer per chunk, so every chunk except the
	// last is exactly chunkSize plaintext. That uniform layout is what lets
	// DecryptRangeToWriter seek to an arbitrary byte offset arithmetically. It
	// also avoids the per-chunk 32-byte overhead exploding when src delivers
	// data in small reads (e.g. a network multipart part).
	buf := make([]byte, chunkSize)
	var total int64
	for {
		n, readErr := io.ReadFull(src, buf)
		if n > 0 {
			if err := writeSealedChunk(dst, gcm, buf[:n]); err != nil {
				return total, err
			}
			total += int64(n)
		}
		if readErr != nil {
			// EOF: clean end (n==0). ErrUnexpectedEOF: final short chunk already
			// written above. Anything else is a real read error.
			if readErr == io.EOF || readErr == io.ErrUnexpectedEOF {
				return total, nil
			}
			return total, readErr
		}
	}
}

// writeSealedChunk seals plain under a fresh random nonce and writes one framed
// chunk: lenPrefix | nonce | ciphertext | tag.
func writeSealedChunk(dst io.Writer, gcm cipher.AEAD, plain []byte) error {
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return err
	}
	sealed := gcm.Seal(nil, nonce, plain, nil)
	tagStart := len(sealed) - gcm.Overhead()
	if err := binary.Write(dst, binary.LittleEndian, uint32(len(plain))); err != nil {
		return err
	}
	if _, err := dst.Write(nonce); err != nil {
		return err
	}
	if _, err := dst.Write(sealed[:tagStart]); err != nil {
		return err
	}
	if _, err := dst.Write(sealed[tagStart:]); err != nil {
		return err
	}
	return nil
}

func DecryptFileToWriter(path string, dst io.Writer, key []byte) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return err
	}
	if string(header) != string(fileMagic) {
		return decryptLegacy(path, dst, key)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	reader := bufio.NewReader(file)
	for {
		var plainLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &plainLen); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(reader, nonce); err != nil {
			return err
		}
		ciphertext := make([]byte, plainLen)
		if _, err := io.ReadFull(reader, ciphertext); err != nil {
			return err
		}
		tag := make([]byte, gcm.Overhead())
		if _, err := io.ReadFull(reader, tag); err != nil {
			return err
		}
		plain, err := gcm.Open(nil, nonce, append(ciphertext, tag...), nil)
		if err != nil {
			return err
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}
	}
}

// expectedEncryptedSize returns the on-disk byte size a blob of the given
// plaintext size MUST have under the uniform fixed-chunk layout. If the actual
// file size differs, the blob isn't uniformly chunked and can't be seeked
// arithmetically.
func expectedEncryptedSize(plaintextSize int64) int64 {
	size := int64(len(fileMagic))
	if plaintextSize == 0 {
		return size
	}
	nFull := plaintextSize / chunkSize
	rem := plaintextSize % chunkSize
	size += nFull * (int64(chunkSize) + chunkOverhead)
	if rem > 0 {
		size += rem + chunkOverhead
	}
	return size
}

// SupportsRange reports whether the blob at path uses the uniform chunk layout
// required for byte-range serving (a single cheap Stat, no decrypt). A false
// result without error means the caller should stream the whole file (200)
// rather than attempt a range. plaintextSize is the file's decrypted size.
func SupportsRange(path string, plaintextSize int64) (bool, error) {
	st, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return st.Size() == expectedEncryptedSize(plaintextSize), nil
}

// DecryptRangeToWriter writes plaintext bytes [start, start+length) of the
// encrypted blob at path to dst, decrypting only the chunks the range touches.
// plaintextSize is the file's decrypted size (from the DB), used both to bound
// the range and to validate the on-disk layout.
//
// It requires the uniform layout produced by EncryptStream (every chunk except
// the last is exactly chunkSize). When the blob predates uniform chunking or is
// a legacy single-blob file, it returns errNonUniformLayout (test with
// ErrRangeUnsupported) so the caller can fall back to a full 200 stream.
func DecryptRangeToWriter(path string, dst io.Writer, key []byte, start, length, plaintextSize int64) error {
	if start < 0 || length < 0 || start+length > plaintextSize {
		return fmt.Errorf("range [%d,%d) out of bounds for size %d", start, start+length, plaintextSize)
	}
	if length == 0 {
		return nil
	}

	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	// One Stat decides uniform-vs-not: if the on-disk size matches the layout
	// formula exactly, every full chunk is chunkSize and offsets are computable.
	st, err := file.Stat()
	if err != nil {
		return err
	}
	if st.Size() != expectedEncryptedSize(plaintextSize) {
		return errNonUniformLayout
	}

	header := make([]byte, len(fileMagic))
	if _, err := io.ReadFull(file, header); err != nil {
		return err
	}
	if string(header) != string(fileMagic) {
		return errNonUniformLayout
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}

	onDiskFullChunk := int64(chunkOverhead) + int64(chunkSize)
	startChunk := start / chunkSize
	skip := start % chunkSize // bytes to drop from the first decrypted chunk

	// Seek straight to the length prefix of the chunk that contains `start`.
	diskPos := int64(len(fileMagic)) + startChunk*onDiskFullChunk
	if _, err := file.Seek(diskPos, io.SeekStart); err != nil {
		return err
	}

	reader := bufio.NewReaderSize(file, 1<<20)
	remaining := length
	first := true
	for remaining > 0 {
		var plainLen uint32
		if err := binary.Read(reader, binary.LittleEndian, &plainLen); err != nil {
			return err
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := io.ReadFull(reader, nonce); err != nil {
			return err
		}
		ciphertext := make([]byte, plainLen)
		if _, err := io.ReadFull(reader, ciphertext); err != nil {
			return err
		}
		tag := make([]byte, gcm.Overhead())
		if _, err := io.ReadFull(reader, tag); err != nil {
			return err
		}
		plain, err := gcm.Open(nil, nonce, append(ciphertext, tag...), nil)
		if err != nil {
			return err
		}
		if first {
			if skip > int64(len(plain)) {
				return fmt.Errorf("range skip %d exceeds chunk length %d", skip, len(plain))
			}
			plain = plain[skip:]
			first = false
		}
		if int64(len(plain)) > remaining {
			plain = plain[:remaining]
		}
		if _, err := dst.Write(plain); err != nil {
			return err
		}
		remaining -= int64(len(plain))
	}
	return nil
}

func decryptLegacy(path string, dst io.Writer, key []byte) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	if len(data) < gcm.NonceSize()+gcm.Overhead() {
		return fmt.Errorf("legacy file too small")
	}
	nonce := data[:gcm.NonceSize()]
	ciphertextWithTag := data[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, ciphertextWithTag, nil)
	if err != nil {
		return err
	}
	_, err = dst.Write(plain)
	return err
}

func EnsureUserDir(storagePath, userID string) (string, error) {
	dir := filepath.Join(storagePath, userID)
	return dir, os.MkdirAll(dir, 0o755)
}

func FinalPath(storagePath, userID, fileID string) string {
	return filepath.Join(storagePath, userID, fileID+".enc")
}

func TempPath(storagePath, fileID string) string {
	return filepath.Join(storagePath, "tmp", fileID+".enc.tmp")
}

// VersionPath returns the on-disk path for version N of a file, used by
// versioning. Versions are sibling blobs to the current version with a
// .v{n}.enc suffix.
func VersionPath(storagePath, userID, fileID string, versionNo int) string {
	return filepath.Join(storagePath, userID, fmt.Sprintf("%s.v%d.enc", fileID, versionNo))
}
