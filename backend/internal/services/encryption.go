package services

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/iii123iii/mycloud/backend/internal/utils"
)

const (
	aesKeyLen   = 32 // AES-256
	gcmNonceLen = 12
	gcmTagLen   = 16
	chunkSize   = 4 * 1024 * 1024 // 4 MB
)

var fileMagic = []byte{'M', 'C', 'v', '2'}

// EncryptedKeyBundle holds the per-file key wrapped with the master key.
type EncryptedKeyBundle struct {
	IVHex     string // 24 hex chars
	EncKeyHex string // 64 hex chars
	TagHex    string // 32 hex chars
}

// getMasterKey reads the master key from the environment.
func getMasterKey() ([]byte, error) {
	raw := ""
	if v := os.Getenv("MASTER_ENCRYPTION_KEY"); v != "" {
		raw = v
	} else if fp := os.Getenv("MASTER_ENCRYPTION_KEY_FILE"); fp != "" {
		b, err := os.ReadFile(fp)
		if err != nil {
			return nil, fmt.Errorf("cannot open key file: %w", err)
		}
		raw = string(bytes.TrimSpace(b))
	}
	if raw == "" {
		return nil, errors.New("MASTER_ENCRYPTION_KEY or MASTER_ENCRYPTION_KEY_FILE not set")
	}
	key, err := utils.HexToBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("invalid hex in MASTER_ENCRYPTION_KEY: %w", err)
	}
	if len(key) != aesKeyLen {
		return nil, fmt.Errorf("MASTER_ENCRYPTION_KEY must be 64 hex chars (32 bytes), got %d", len(key))
	}
	return key, nil
}

// GenerateFileKey returns a fresh 32-byte random key.
func GenerateFileKey() ([]byte, error) {
	key := make([]byte, aesKeyLen)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}

// WrapKey encrypts the per-file key with the master key using AES-256-GCM.
func WrapKey(fileKey []byte) (*EncryptedKeyBundle, error) {
	masterKey, err := getMasterKey()
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceLen)
	if _, err := rand.Read(nonce); err != nil {
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
	ciphertext := gcm.Seal(nil, nonce, fileKey, nil)
	// ciphertext includes 16-byte GCM tag appended by Go's Seal
	encKey := ciphertext[:aesKeyLen]
	tag := ciphertext[aesKeyLen:]
	return &EncryptedKeyBundle{
		IVHex:     utils.BytesToHex(nonce),
		EncKeyHex: utils.BytesToHex(encKey),
		TagHex:    utils.BytesToHex(tag),
	}, nil
}

// UnwrapKey decrypts a per-file key using the master key.
func UnwrapKey(bundle *EncryptedKeyBundle) ([]byte, error) {
	masterKey, err := getMasterKey()
	if err != nil {
		return nil, err
	}
	nonce, err := utils.HexToBytes(bundle.IVHex)
	if err != nil {
		return nil, err
	}
	encKey, err := utils.HexToBytes(bundle.EncKeyHex)
	if err != nil {
		return nil, err
	}
	tag, err := utils.HexToBytes(bundle.TagHex)
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
	// Reconstruct the combined ciphertext+tag that Go's GCM expects
	combined := append(encKey, tag...)
	fileKey, err := gcm.Open(nil, nonce, combined, nil)
	if err != nil {
		return nil, fmt.Errorf("key unwrap failed — bad master key or corrupted data: %w", err)
	}
	return fileKey, nil
}

// EncryptToFile writes the data to outPath using the V2 chunked format.
func EncryptToFile(data []byte, key []byte, outPath string) error {
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()

	// Write magic header
	if _, err := f.Write(fileMagic); err != nil {
		return err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	offset := 0
	for offset < len(data) {
		end := offset + chunkSize
		if end > len(data) {
			end = len(data)
		}
		chunk := data[offset:end]

		nonce := make([]byte, gcmNonceLen)
		if _, err := rand.Read(nonce); err != nil {
			return err
		}

		gcm, err := cipher.NewGCM(block)
		if err != nil {
			return err
		}
		ciphertext := gcm.Seal(nil, nonce, chunk, nil)
		// ciphertext = encrypted bytes + 16-byte tag
		encBytes := ciphertext[:len(chunk)]
		tag := ciphertext[len(chunk):]

		// Write chunk header: [4-byte LE payload_len]
		var lenBuf [4]byte
		binary.LittleEndian.PutUint32(lenBuf[:], uint32(len(chunk)))
		if _, err := f.Write(lenBuf[:]); err != nil {
			return err
		}
		if _, err := f.Write(nonce); err != nil {
			return err
		}
		if _, err := f.Write(encBytes); err != nil {
			return err
		}
		if _, err := f.Write(tag); err != nil {
			return err
		}

		offset = end
	}
	return nil
}

// DecryptFileStream opens the file at path, auto-detects V1/V2 format, and
// calls onChunk for each decrypted block. Memory usage stays ~4 MB.
func DecryptFileStream(path string, key []byte, onChunk func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer f.Close()

	// Peek at first 4 bytes to detect format
	var magic [4]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return fmt.Errorf("cannot read file header: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return err
	}

	if bytes.Equal(magic[:], fileMagic) {
		// ── V2 chunked ───────────────────────────────────────────────
		return decryptV2(f, block, onChunk)
	}
	// ── V1 legacy ─────────────────────────────────────────────────────
	// Seek back to start; magic bytes are the beginning of nonce+ciphertext+tag
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	return decryptV1(f, block, onChunk)
}

func decryptV2(r io.Reader, block cipher.Block, onChunk func([]byte) error) error {
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	for {
		var lenBuf [4]byte
		if _, err := io.ReadFull(r, lenBuf[:]); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
				return nil // normal end of chunks
			}
			return err
		}
		chunkLen := binary.LittleEndian.Uint32(lenBuf[:])
		if chunkLen == 0 {
			return nil
		}

		nonce := make([]byte, gcmNonceLen)
		if _, err := io.ReadFull(r, nonce); err != nil {
			return fmt.Errorf("unexpected EOF reading nonce: %w", err)
		}
		ciphertext := make([]byte, int(chunkLen)+gcmTagLen)
		if _, err := io.ReadFull(r, ciphertext); err != nil {
			return fmt.Errorf("unexpected EOF reading chunk: %w", err)
		}
		plain, err := gcm.Open(nil, nonce, ciphertext, nil)
		if err != nil {
			return fmt.Errorf("GCM auth failed — corrupted or tampered chunk: %w", err)
		}
		if err := onChunk(plain); err != nil {
			return err
		}
	}
}

func decryptV1(r io.Reader, block cipher.Block, onChunk func([]byte) error) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if len(data) < gcmNonceLen+gcmTagLen {
		return errors.New("V1 file too small")
	}
	nonce := data[:gcmNonceLen]
	ciphertext := data[gcmNonceLen:] // includes the 16-byte GCM tag at the end

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return err
	}
	plain, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return fmt.Errorf("GCM auth failed — file may be corrupted: %w", err)
	}
	return onChunk(plain)
}
