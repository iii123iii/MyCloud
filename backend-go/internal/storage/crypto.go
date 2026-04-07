package storage

import (
	"bufio"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	chunkSize = 4 * 1024 * 1024
)

var fileMagic = []byte{'M', 'C', 'v', '2'}

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

	buf := make([]byte, chunkSize)
	var total int64
	for {
		n, readErr := io.ReadAtLeast(src, buf, 1)
		if readErr == io.EOF {
			break
		}
		if readErr == io.ErrUnexpectedEOF {
			// n is valid
		} else if readErr != nil {
			if readErr == io.EOF {
				break
			}
			if readErr != io.ErrUnexpectedEOF {
				return total, readErr
			}
		}
		if n <= 0 {
			break
		}
		nonce := make([]byte, gcm.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return total, err
		}
		sealed := gcm.Seal(nil, nonce, buf[:n], nil)
		tagStart := len(sealed) - gcm.Overhead()
		if err := binary.Write(dst, binary.LittleEndian, uint32(n)); err != nil {
			return total, err
		}
		if _, err := dst.Write(nonce); err != nil {
			return total, err
		}
		if _, err := dst.Write(sealed[:tagStart]); err != nil {
			return total, err
		}
		if _, err := dst.Write(sealed[tagStart:]); err != nil {
			return total, err
		}
		total += int64(n)
		if readErr == io.ErrUnexpectedEOF {
			break
		}
	}
	return total, nil
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
