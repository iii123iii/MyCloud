#pragma once
#include <string>
#include <chrono>
#include <stdexcept>
#include <jwt-cpp/jwt.h>

namespace utils {

struct JwtClaims {
    std::string userId;
    std::string role;
    std::string type; // "access" or "refresh"
};

inline std::string createAccessToken(const std::string& userId,
                                     const std::string& role,
                                     const std::string& secret) {
    auto now = std::chrono::system_clock::now();
    return jwt::create()
        .set_issuer("mycloud")
        .set_subject(userId)
        .set_payload_claim("role", jwt::claim(role))
        .set_payload_claim("type", jwt::claim(std::string("access")))
        .set_issued_at(now)
        .set_expires_at(now + std::chrono::minutes(15))
        .sign(jwt::algorithm::hs256{secret});
}

inline std::string createRefreshToken(const std::string& userId,
                                      const std::string& secret) {
    auto now = std::chrono::system_clock::now();
    return jwt::create()
        .set_issuer("mycloud")
        .set_subject(userId)
        .set_payload_claim("type", jwt::claim(std::string("refresh")))
        .set_issued_at(now)
        .set_expires_at(now + std::chrono::days(7))
        .sign(jwt::algorithm::hs256{secret});
}

inline JwtClaims verifyToken(const std::string& token,
                              const std::string& secret) {
    auto verifier = jwt::verify()
        .allow_algorithm(jwt::algorithm::hs256{secret})
        .with_issuer("mycloud");

    auto decoded = jwt::decode(token);
    verifier.verify(decoded);

    JwtClaims claims;
    claims.userId = decoded.get_subject();
    if (decoded.has_payload_claim("role"))
        claims.role = decoded.get_payload_claim("role").as_string();
    if (decoded.has_payload_claim("type"))
        claims.type = decoded.get_payload_claim("type").as_string();
    return claims;
}

} // namespace utils
