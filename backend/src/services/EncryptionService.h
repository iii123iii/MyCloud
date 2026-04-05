#pragma once
#include <string>
#include <vector>
#include <fstream>
#include <functional>
#include <cstring>
#include <stdexcept>
#include <openssl/evp.h>
#include <openssl/rand.h>
#include "utils/HashUtils.h"

// ─── AES-256-GCM encryption ─────────────────────────────────────────────────
//
// V1 (legacy) file layout:
//   [12-byte nonce][ciphertext][16-byte GCM auth tag]
//
// V2 (chunked) file layout — supports streaming and multi-GB files:
//   [4 bytes: "MCv2" magic]
//   Repeated chunks until EOF:
//     [4 bytes LE: payload_len — plaintext/ciphertext bytes in this chunk]
//     [12 bytes:   nonce]
//     [payload_len bytes: ciphertext]
//     [16 bytes:   GCM auth tag]
//
// Per-file key wrapping (unchanged):
//   masterKey (32 bytes) → AES-256-GCM encrypt the per-file key
//   → store: iv_hex, enc_key_hex, tag_hex in the database

namespace services {

constexpr size_t AES_KEY_LEN    = 32;   // 256-bit
constexpr size_t GCM_NONCE_LEN  = 12;
constexpr size_t GCM_TAG_LEN    = 16;
constexpr size_t CHUNK_SIZE     = 4u * 1024 * 1024;  // 4 MB per chunk

static constexpr char FILE_MAGIC[4] = {'M', 'C', 'v', '2'};

// RAII wrapper for EVP_CIPHER_CTX to prevent memory leaks on exceptions
struct EvpCtxGuard {
    EVP_CIPHER_CTX* ctx;
    EvpCtxGuard() : ctx(EVP_CIPHER_CTX_new()) {
        if (!ctx) throw std::runtime_error("EVP_CIPHER_CTX_new failed");
    }
    ~EvpCtxGuard() { if (ctx) EVP_CIPHER_CTX_free(ctx); }
    operator EVP_CIPHER_CTX*() const { return ctx; }
    EvpCtxGuard(const EvpCtxGuard&) = delete;
    EvpCtxGuard& operator=(const EvpCtxGuard&) = delete;
};

struct EncryptedKeyBundle {
    std::string ivHex;       // 24 hex chars
    std::string encKeyHex;   // 64 hex chars
    std::string tagHex;      // 32 hex chars
};

class EncryptionService {
public:
    // ── Master key ───────────────────────────────────────────────────────────
    static std::vector<unsigned char> getMasterKey() {
        std::string raw;
        const char* direct = std::getenv("MASTER_ENCRYPTION_KEY");
        if (direct && direct[0] != '\0') {
            raw = std::string(direct);
        } else {
            const char* fp = std::getenv("MASTER_ENCRYPTION_KEY_FILE");
            if (!fp) throw std::runtime_error("MASTER_ENCRYPTION_KEY or MASTER_ENCRYPTION_KEY_FILE not set");
            std::ifstream f(fp);
            if (!f) throw std::runtime_error(std::string("Cannot open key file: ") + fp);
            std::getline(f, raw);
        }
        if (raw.empty()) throw std::runtime_error("MASTER_ENCRYPTION_KEY is empty");
        auto bytes = utils::hexToBytes(raw);
        if (bytes.size() != AES_KEY_LEN)
            throw std::runtime_error("MASTER_ENCRYPTION_KEY must be 64 hex chars (32 bytes)");
        return bytes;
    }

    // ── Per-file key generation ──────────────────────────────────────────────
    static std::vector<unsigned char> generateFileKey() {
        std::vector<unsigned char> key(AES_KEY_LEN);
        if (RAND_bytes(key.data(), AES_KEY_LEN) != 1)
            throw std::runtime_error("RAND_bytes failed");
        return key;
    }

    // ── Key wrapping (encrypt per-file key with master key) ──────────────────
    static EncryptedKeyBundle wrapKey(const std::vector<unsigned char>& fileKey) {
        auto masterKey = getMasterKey();
        unsigned char nonce[GCM_NONCE_LEN];
        if (RAND_bytes(nonce, GCM_NONCE_LEN) != 1)
            throw std::runtime_error("RAND_bytes failed for nonce");

        std::vector<unsigned char> ciphertext(AES_KEY_LEN);
        unsigned char tag[GCM_TAG_LEN];

        EvpCtxGuard ctx;
        EVP_EncryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, nullptr, nullptr);
        EVP_EncryptInit_ex(ctx, nullptr, nullptr, masterKey.data(), nonce);
        int outLen = 0;
        EVP_EncryptUpdate(ctx, ciphertext.data(), &outLen, fileKey.data(), AES_KEY_LEN);
        EVP_EncryptFinal_ex(ctx, ciphertext.data() + outLen, &outLen);
        EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_GET_TAG, GCM_TAG_LEN, tag);

        return {
            utils::bytesToHex(nonce,             GCM_NONCE_LEN),
            utils::bytesToHex(ciphertext.data(),  AES_KEY_LEN),
            utils::bytesToHex(tag,                GCM_TAG_LEN)
        };
    }

    // ── Key unwrapping ───────────────────────────────────────────────────────
    static std::vector<unsigned char> unwrapKey(const EncryptedKeyBundle& bundle) {
        auto masterKey = getMasterKey();
        auto nonce     = utils::hexToBytes(bundle.ivHex);
        auto encKey    = utils::hexToBytes(bundle.encKeyHex);
        auto tag       = utils::hexToBytes(bundle.tagHex);

        std::vector<unsigned char> fileKey(AES_KEY_LEN);
        EvpCtxGuard ctx;
        EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, nullptr, nullptr);
        EVP_DecryptInit_ex(ctx, nullptr, nullptr, masterKey.data(), nonce.data());
        EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, GCM_TAG_LEN,
                            const_cast<unsigned char*>(tag.data()));
        int outLen = 0;
        EVP_DecryptUpdate(ctx, fileKey.data(), &outLen, encKey.data(), AES_KEY_LEN);
        int ret = EVP_DecryptFinal_ex(ctx, fileKey.data() + outLen, &outLen);
        if (ret != 1)
            throw std::runtime_error("Key unwrap failed — bad master key or corrupted data");
        return fileKey;
    }

    // ═════════════════════════════════════════════════════════════════════════
    // V2 CHUNKED FILE ENCRYPTION — constant-memory for any file size
    // ═════════════════════════════════════════════════════════════════════════

    // Encrypt `data` (up to multi-GB) in 4 MB chunks, writing directly to `out`.
    // Memory usage: ~4 MB regardless of file size.
    static void encryptToStream(
            const unsigned char* data, size_t dataLen,
            const std::vector<unsigned char>& key,
            std::ofstream& out) {
        // Write v2 magic header
        out.write(FILE_MAGIC, 4);

        size_t offset = 0;
        std::vector<unsigned char> cipherBuf(CHUNK_SIZE);

        while (offset < dataLen) {
            uint32_t thisChunk = static_cast<uint32_t>(
                std::min(CHUNK_SIZE, dataLen - offset));

            unsigned char nonce[GCM_NONCE_LEN];
            if (RAND_bytes(nonce, GCM_NONCE_LEN) != 1)
                throw std::runtime_error("RAND_bytes failed");

            unsigned char tag[GCM_TAG_LEN];
            EvpCtxGuard ctx;
            EVP_EncryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, key.data(), nonce);
            int outLen = 0;
            EVP_EncryptUpdate(ctx, cipherBuf.data(), &outLen,
                              data + offset, static_cast<int>(thisChunk));
            int finalLen = 0;
            EVP_EncryptFinal_ex(ctx, cipherBuf.data() + outLen, &finalLen);
            EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_GET_TAG, GCM_TAG_LEN, tag);

            // Write chunk: [len][nonce][ciphertext][tag]
            out.write(reinterpret_cast<const char*>(&thisChunk), 4);
            out.write(reinterpret_cast<const char*>(nonce), GCM_NONCE_LEN);
            out.write(reinterpret_cast<const char*>(cipherBuf.data()),
                      static_cast<std::streamsize>(thisChunk));
            out.write(reinterpret_cast<const char*>(tag), GCM_TAG_LEN);

            if (!out.good())
                throw std::runtime_error("Write failed during chunked encryption");

            offset += thisChunk;
        }
        out.flush();
    }

    // Decrypt a file (v1 or v2 format) in streaming fashion.
    // Calls `onChunk(data, len)` for each decrypted chunk.
    // Memory usage: ~4 MB regardless of file size.
    static void decryptFileStream(
            const std::string& path,
            const std::vector<unsigned char>& key,
            std::function<void(const unsigned char*, size_t)> onChunk) {
        std::ifstream in(path, std::ios::binary);
        if (!in) throw std::runtime_error("Cannot open file: " + path);

        // Peek at first 4 bytes to detect format
        char magic[4] = {};
        in.read(magic, 4);

        if (std::memcmp(magic, FILE_MAGIC, 4) == 0) {
            // ── V2 chunked format ────────────────────────────────────────
            std::vector<unsigned char> cipherBuf;
            std::vector<unsigned char> plainBuf;

            while (true) {
                uint32_t chunkLen = 0;
                in.read(reinterpret_cast<char*>(&chunkLen), 4);
                if (!in || in.gcount() < 4) break;   // EOF — no more chunks
                if (chunkLen == 0) break;

                // Ensure buffers are large enough
                if (cipherBuf.size() < chunkLen) {
                    cipherBuf.resize(chunkLen);
                    plainBuf.resize(chunkLen);
                }

                unsigned char nonce[GCM_NONCE_LEN];
                in.read(reinterpret_cast<char*>(nonce), GCM_NONCE_LEN);
                in.read(reinterpret_cast<char*>(cipherBuf.data()), chunkLen);

                unsigned char tag[GCM_TAG_LEN];
                in.read(reinterpret_cast<char*>(tag), GCM_TAG_LEN);

                if (!in)
                    throw std::runtime_error("Unexpected EOF reading encrypted chunk");

                EvpCtxGuard ctx;
                EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, key.data(), nonce);
                EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, GCM_TAG_LEN, tag);
                int outLen = 0;
                EVP_DecryptUpdate(ctx, plainBuf.data(), &outLen,
                                  cipherBuf.data(), static_cast<int>(chunkLen));
                int finalLen = 0;
                int ret = EVP_DecryptFinal_ex(ctx, plainBuf.data() + outLen, &finalLen);
                if (ret != 1)
                    throw std::runtime_error("GCM auth failed — corrupted or tampered chunk");

                onChunk(plainBuf.data(), static_cast<size_t>(outLen + finalLen));
            }
        } else {
            // ── V1 legacy format: [nonce][ciphertext][tag] ───────────────
            in.seekg(0, std::ios::end);
            auto fileSize = static_cast<size_t>(in.tellg());
            in.seekg(0);

            if (fileSize < GCM_NONCE_LEN + GCM_TAG_LEN)
                throw std::runtime_error("V1 file too small");

            // For v1 we must load the entire file (no chunking in old format)
            std::vector<unsigned char> buf(fileSize);
            in.read(reinterpret_cast<char*>(buf.data()),
                    static_cast<std::streamsize>(fileSize));

            size_t cipherLen = fileSize - GCM_NONCE_LEN - GCM_TAG_LEN;
            const unsigned char* nonce      = buf.data();
            const unsigned char* ciphertext = buf.data() + GCM_NONCE_LEN;
            const unsigned char* tagPtr     = buf.data() + GCM_NONCE_LEN + cipherLen;

            std::vector<unsigned char> plain(cipherLen);
            EvpCtxGuard ctx;
            EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, key.data(), nonce);
            EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, GCM_TAG_LEN,
                                const_cast<unsigned char*>(tagPtr));
            int outLen = 0;
            EVP_DecryptUpdate(ctx, plain.data(), &outLen,
                              ciphertext, static_cast<int>(cipherLen));
            int finalLen = 0;
            int ret = EVP_DecryptFinal_ex(ctx, plain.data() + outLen, &finalLen);
            if (ret != 1)
                throw std::runtime_error("GCM auth failed — file may be corrupted");

            onChunk(plain.data(), static_cast<size_t>(outLen + finalLen));
        }
    }

    // ── Legacy in-memory encrypt (kept for small buffers / key material) ─────
    static std::vector<unsigned char> encryptBuffer(
            const unsigned char* plaintext, size_t len,
            const std::vector<unsigned char>& key) {
        unsigned char nonce[GCM_NONCE_LEN];
        if (RAND_bytes(nonce, GCM_NONCE_LEN) != 1)
            throw std::runtime_error("RAND_bytes failed");

        std::vector<unsigned char> output(GCM_NONCE_LEN + len + GCM_TAG_LEN);
        std::copy(nonce, nonce + GCM_NONCE_LEN, output.begin());

        EvpCtxGuard ctx;
        EVP_EncryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, key.data(), nonce);
        int outLen = 0;
        EVP_EncryptUpdate(ctx, output.data() + GCM_NONCE_LEN, &outLen,
                          plaintext, static_cast<int>(len));
        EVP_EncryptFinal_ex(ctx, output.data() + GCM_NONCE_LEN + outLen, &outLen);
        EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_GET_TAG, GCM_TAG_LEN,
                            output.data() + GCM_NONCE_LEN + len);
        return output;
    }

    static std::vector<unsigned char> decryptBuffer(
            const unsigned char* data, size_t len,
            const std::vector<unsigned char>& key) {
        if (len < GCM_NONCE_LEN + GCM_TAG_LEN)
            throw std::runtime_error("Buffer too small to contain nonce+tag");
        size_t cipherLen = len - GCM_NONCE_LEN - GCM_TAG_LEN;
        const unsigned char* nonce      = data;
        const unsigned char* ciphertext = data + GCM_NONCE_LEN;
        const unsigned char* tag        = data + GCM_NONCE_LEN + cipherLen;

        std::vector<unsigned char> plain(cipherLen);
        EvpCtxGuard ctx;
        EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, key.data(), nonce);
        EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, GCM_TAG_LEN,
                            const_cast<unsigned char*>(tag));
        int outLen = 0;
        EVP_DecryptUpdate(ctx, plain.data(), &outLen,
                          ciphertext, static_cast<int>(cipherLen));
        int ret = EVP_DecryptFinal_ex(ctx, plain.data() + outLen, &outLen);
        if (ret != 1)
            throw std::runtime_error("GCM auth failed — data corrupted or tampered");
        return plain;
    }
};

} // namespace services
