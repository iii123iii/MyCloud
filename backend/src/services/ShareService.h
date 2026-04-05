#pragma once
#include <string>
#include "utils/HashUtils.h"

namespace services {

class ShareService {
public:
    // Generate a secure random 32-byte (64-char hex) share token
    static std::string generateToken() {
        return utils::randomHex(32);
    }

    // Hash an optional share password for storage
    static std::string hashSharePassword(const std::string& password) {
        return utils::hashPassword(password);
    }

    static bool verifySharePassword(const std::string& password, const std::string& hash) {
        return utils::verifyPassword(password, hash);
    }
};

} // namespace services
