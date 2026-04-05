#pragma once
#include <string>
#include <vector>
#include <fstream>
#include <filesystem>
#include <functional>
#include <memory>
#include <cstring>
#include <stdexcept>
#include <cstdlib>
#include "EncryptionService.h"

namespace fs = std::filesystem;

namespace services {

// ─── Streaming reader for decrypting files chunk-by-chunk ────────────────────
// Used by Drogon's newStreamResponse() — fills a buffer on each pull call.
// Memory usage: ~4 MB regardless of file size.
class DecryptingReader {
    std::ifstream file_;
    std::vector<unsigned char> key_;
    bool isV2_  = false;
    bool done_  = false;

    // Buffered decrypted data waiting to be consumed
    std::vector<unsigned char> pending_;
    size_t pendingOff_ = 0;

    std::vector<unsigned char> cipherBuf_;
    std::vector<unsigned char> plainBuf_;

    bool readNextChunk() {
        if (done_) return false;

        if (!isV2_) {
            // V1 legacy: read entire file, decrypt, mark done
            file_.seekg(0, std::ios::end);
            auto sz = static_cast<size_t>(file_.tellg());
            file_.seekg(0);
            std::vector<unsigned char> buf(sz);
            file_.read(reinterpret_cast<char*>(buf.data()),
                       static_cast<std::streamsize>(sz));
            pending_ = EncryptionService::decryptBuffer(buf.data(), buf.size(), key_);
            pendingOff_ = 0;
            done_ = true;
            return true;
        }

        // V2 chunked: read one chunk
        uint32_t chunkLen = 0;
        file_.read(reinterpret_cast<char*>(&chunkLen), 4);
        if (!file_ || file_.gcount() < 4 || chunkLen == 0) { done_ = true; return false; }

        if (cipherBuf_.size() < chunkLen) {
            cipherBuf_.resize(chunkLen);
            plainBuf_.resize(chunkLen);
        }

        unsigned char nonce[GCM_NONCE_LEN];
        file_.read(reinterpret_cast<char*>(nonce), GCM_NONCE_LEN);
        file_.read(reinterpret_cast<char*>(cipherBuf_.data()), chunkLen);
        unsigned char tag[GCM_TAG_LEN];
        file_.read(reinterpret_cast<char*>(tag), GCM_TAG_LEN);
        if (!file_) throw std::runtime_error("Unexpected EOF in encrypted chunk");

        EvpCtxGuard ctx;
        EVP_DecryptInit_ex(ctx, EVP_aes_256_gcm(), nullptr, key_.data(), nonce);
        EVP_CIPHER_CTX_ctrl(ctx, EVP_CTRL_GCM_SET_TAG, GCM_TAG_LEN, tag);
        int outLen = 0;
        EVP_DecryptUpdate(ctx, plainBuf_.data(), &outLen,
                          cipherBuf_.data(), static_cast<int>(chunkLen));
        int finalLen = 0;
        int ret = EVP_DecryptFinal_ex(ctx, plainBuf_.data() + outLen, &finalLen);
        if (ret != 1) throw std::runtime_error("GCM auth failed on chunk");

        size_t totalLen = static_cast<size_t>(outLen + finalLen);
        pending_.assign(plainBuf_.begin(), plainBuf_.begin() + totalLen);
        pendingOff_ = 0;
        return true;
    }

public:
    DecryptingReader(const std::string& path,
                     std::vector<unsigned char> key)
        : file_(path, std::ios::binary), key_(std::move(key))
    {
        if (!file_) throw std::runtime_error("Cannot open: " + path);
        char magic[4] = {};
        file_.read(magic, 4);
        isV2_ = (std::memcmp(magic, FILE_MAGIC, 4) == 0);
        if (!isV2_) file_.seekg(0); // rewind for v1
    }

    // Pull-based read: fill `buf` with up to `maxLen` bytes, return count.
    // Returns 0 when the file is fully consumed.
    size_t read(char* buf, size_t maxLen) {
        // Drain pending buffer first
        while (pendingOff_ >= pending_.size()) {
            if (!readNextChunk()) return 0;
        }
        size_t avail = pending_.size() - pendingOff_;
        size_t n = std::min(avail, maxLen);
        std::memcpy(buf, pending_.data() + pendingOff_, n);
        pendingOff_ += n;
        return n;
    }
};

// ─── StorageService ──────────────────────────────────────────────────────────

class StorageService {
public:
    static std::string storagePath() {
        const char* p = std::getenv("STORAGE_PATH");
        return p ? std::string(p) : "/data/files";
    }

    static std::string filePath(const std::string& userId,
                                const std::string& fileId) {
        return storagePath() + "/" + userId + "/" + fileId + ".enc";
    }

    // ── Store (encrypt) ──────────────────────────────────────────────────
    // Encrypts `plainData` in 4 MB chunks and writes to disk.
    // Memory overhead: ~4 MB regardless of file size.
    static EncryptedKeyBundle storeFile(
            const std::string& userId,
            const std::string& fileId,
            const unsigned char* plainData,
            size_t plainLen) {
        auto fileKey = EncryptionService::generateFileKey();
        auto bundle  = EncryptionService::wrapKey(fileKey);

        std::string dir  = storagePath() + "/" + userId;
        fs::create_directories(dir);
        std::string path = filePath(userId, fileId);

        std::ofstream out(path, std::ios::binary);
        if (!out) throw std::runtime_error("Cannot open for writing: " + path);

        EncryptionService::encryptToStream(plainData, plainLen, fileKey, out);

        if (!out.good())
            throw std::runtime_error("Write failed: " + path);

        return bundle;
    }

    // ── Load (streaming decrypt) ─────────────────────────────────────────
    // Returns a shared DecryptingReader for pull-based streaming.
    // Memory usage: ~4 MB per active reader.
    static std::shared_ptr<DecryptingReader> createReader(
            const std::string& userId,
            const std::string& fileId,
            const EncryptedKeyBundle& bundle) {
        auto fileKey = EncryptionService::unwrapKey(bundle);
        std::string path = filePath(userId, fileId);
        return std::make_shared<DecryptingReader>(path, std::move(fileKey));
    }

    // ── Load entire file (convenience, for small files / backward compat) ─
    static std::vector<unsigned char> loadFile(
            const std::string& userId,
            const std::string& fileId,
            const EncryptedKeyBundle& bundle) {
        auto fileKey = EncryptionService::unwrapKey(bundle);
        std::string path = filePath(userId, fileId);

        std::vector<unsigned char> result;
        EncryptionService::decryptFileStream(path, fileKey,
            [&result](const unsigned char* data, size_t len) {
                result.insert(result.end(), data, data + len);
            });
        return result;
    }

    // ── Delete ciphertext from disk ──────────────────────────────────────
    static void deleteFile(const std::string& userId,
                           const std::string& fileId) {
        std::string path = filePath(userId, fileId);
        std::error_code ec;
        fs::remove(path, ec);
    }
};

} // namespace services
