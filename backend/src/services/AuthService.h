#pragma once
#include <string>
#include <functional>
#include <fstream>
#include <drogon/drogon.h>
#include <hiredis/hiredis.h>
#include "utils/JwtUtils.h"
#include "utils/HashUtils.h"
#include "utils/UuidUtils.h"

namespace services {

// Thin wrapper around Redis for token blacklisting and rate-limiting
class RedisClient {
public:
    static redisContext* get() {
        static redisContext* ctx = nullptr;
        if (!ctx || ctx->err) {
            // Free the old broken context before reconnecting to avoid leaking memory
            if (ctx) {
                redisFree(ctx);
                ctx = nullptr;
            }
            const char* host = std::getenv("REDIS_HOST") ? std::getenv("REDIS_HOST") : "redis";
            int port = std::getenv("REDIS_PORT") ? std::stoi(std::getenv("REDIS_PORT")) : 6379;
            ctx = redisConnect(host, port);
        }
        return ctx;
    }

    // Store a key with TTL (seconds)
    static void setex(const std::string& key, int ttl, const std::string& value = "1") {
        auto* c = get();
        freeReplyObject(redisCommand(c, "SETEX %s %d %s", key.c_str(), ttl, value.c_str()));
    }

    // Check if key exists (returns true if blacklisted)
    static bool exists(const std::string& key) {
        auto* c = get();
        auto* reply = static_cast<redisReply*>(redisCommand(c, "EXISTS %s", key.c_str()));
        bool result = reply && reply->integer == 1;
        freeReplyObject(reply);
        return result;
    }

    // Increment rate-limit counter, set TTL if new
    static long long incr(const std::string& key, int ttlSeconds) {
        auto* c = get();
        auto* r1 = static_cast<redisReply*>(redisCommand(c, "INCR %s", key.c_str()));
        long long val = r1 ? r1->integer : 0;
        freeReplyObject(r1);
        if (val == 1)
            freeReplyObject(redisCommand(c, "EXPIRE %s %d", key.c_str(), ttlSeconds));
        return val;
    }
};

class AuthService {
public:
    static std::string jwtSecret() {
        // Direct env var
        const char* s = std::getenv("JWT_SECRET");
        if (s && s[0] != '\0') return std::string(s);
        // File-based secret
        const char* f = std::getenv("JWT_SECRET_FILE");
        if (f) {
            std::ifstream in(f);
            if (in) { std::string val; std::getline(in, val); if (!val.empty()) return val; }
        }
        return "";
    }

    // Blacklist a refresh token so logout is effective
    static void blacklistToken(const std::string& token) {
        // 7 days TTL matches refresh token lifetime
        RedisClient::setex("bl:" + token, 7 * 24 * 3600);
    }

    static bool isBlacklisted(const std::string& token) {
        return RedisClient::exists("bl:" + token);
    }

    // Rate-limit: max 10 login attempts per IP per 15 minutes
    static bool checkRateLimit(const std::string& ip) {
        auto count = RedisClient::incr("rl:login:" + ip, 15 * 60);
        return count <= 10;
    }

    // Issue both tokens and return them as a JSON value
    static Json::Value issueTokenPair(const std::string& userId, const std::string& role) {
        auto secret = jwtSecret();
        Json::Value result;
        result["access_token"]  = utils::createAccessToken(userId, role, secret);
        result["refresh_token"] = utils::createRefreshToken(userId, secret);
        result["token_type"]    = "Bearer";
        return result;
    }
};

} // namespace services
