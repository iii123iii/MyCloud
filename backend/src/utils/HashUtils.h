#pragma once
#include <string>
#include <openssl/evp.h>
#include <openssl/rand.h>
#include <sstream>
#include <iomanip>
#include <stdexcept>

namespace utils {

// ─── Hex helpers ──────────────────────────────────────────────────────────────

inline std::string bytesToHex(const unsigned char* data, size_t len) {
    std::ostringstream oss;
    oss << std::hex << std::setfill('0');
    for (size_t i = 0; i < len; ++i)
        oss << std::setw(2) << static_cast<int>(data[i]);
    return oss.str();
}

inline std::vector<unsigned char> hexToBytes(const std::string& hex) {
    if (hex.size() % 2 != 0)
        throw std::invalid_argument("Hex string has odd length");
    std::vector<unsigned char> bytes;
    bytes.reserve(hex.size() / 2);
    for (size_t i = 0; i < hex.size(); i += 2) {
        unsigned char byte = static_cast<unsigned char>(std::stoi(hex.substr(i, 2), nullptr, 16));
        bytes.push_back(byte);
    }
    return bytes;
}

// ─── Random hex token ─────────────────────────────────────────────────────────

inline std::string randomHex(size_t numBytes) {
    std::vector<unsigned char> buf(numBytes);
    if (RAND_bytes(buf.data(), static_cast<int>(numBytes)) != 1)
        throw std::runtime_error("RAND_bytes failed");
    return bytesToHex(buf.data(), numBytes);
}

// ─── bcrypt-style password hashing via OpenSSL EVP (PBKDF2-SHA256) ────────────
// We use PBKDF2-HMAC-SHA256 with a random 16-byte salt and 310000 iterations.
// Stored format: "pbkdf2$<iterations>$<salt_hex>$<hash_hex>"

constexpr int PBKDF2_ITER   = 310000;
constexpr int PBKDF2_KEYLEN = 32;

inline std::string hashPassword(const std::string& password) {
    unsigned char salt[16];
    if (RAND_bytes(salt, sizeof(salt)) != 1)
        throw std::runtime_error("RAND_bytes failed for salt");

    unsigned char key[PBKDF2_KEYLEN];
    if (PKCS5_PBKDF2_HMAC(
            password.c_str(), static_cast<int>(password.size()),
            salt, sizeof(salt),
            PBKDF2_ITER,
            EVP_sha256(),
            PBKDF2_KEYLEN, key) != 1)
        throw std::runtime_error("PBKDF2 failed");

    return "pbkdf2$" + std::to_string(PBKDF2_ITER)
         + "$" + bytesToHex(salt, sizeof(salt))
         + "$" + bytesToHex(key, PBKDF2_KEYLEN);
}

inline bool verifyPassword(const std::string& password, const std::string& stored) {
    // Parse "pbkdf2$<iter>$<salt_hex>$<hash_hex>"
    auto p1 = stored.find('$');
    auto p2 = stored.find('$', p1 + 1);
    auto p3 = stored.find('$', p2 + 1);
    if (p1 == std::string::npos || p2 == std::string::npos || p3 == std::string::npos)
        return false;

    int iter       = std::stoi(stored.substr(p1 + 1, p2 - p1 - 1));
    auto saltBytes = hexToBytes(stored.substr(p2 + 1, p3 - p2 - 1));
    auto hashBytes = hexToBytes(stored.substr(p3 + 1));

    unsigned char key[PBKDF2_KEYLEN];
    if (PKCS5_PBKDF2_HMAC(
            password.c_str(), static_cast<int>(password.size()),
            saltBytes.data(), static_cast<int>(saltBytes.size()),
            iter,
            EVP_sha256(),
            PBKDF2_KEYLEN, key) != 1)
        return false;

    // Constant-time compare
    if (hashBytes.size() != PBKDF2_KEYLEN) return false;
    return CRYPTO_memcmp(key, hashBytes.data(), PBKDF2_KEYLEN) == 0;
}

} // namespace utils
