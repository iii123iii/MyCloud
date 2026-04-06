#pragma once
#include <string>
#include <functional>
#include <fstream>
#include <mutex>
#include <drogon/drogon.h>
#include <hiredis/hiredis.h>
#include "utils/JwtUtils.h"
#include "utils/HashUtils.h"
#include "utils/UuidUtils.h"

namespace services {

// Thread-safe wrapper around Redis for token blacklisting and rate-limiting.
// Drogon runs multiple event-loop threads — a bare static pointer is a data race.
class RedisClient {
public:
    static redisContext* get() {
        // Lock protects both the pointer and the connection state check.
        // hiredis is NOT thread-safe; every call on the same redisContext
        // must be serialized.
        std::lock_guard<std::mutex> lock(mutex());
        static redisContext* ctx = nullptr;
        if (!ctx || ctx->err) {
            if (ctx) {
                redisFree(ctx);
                ctx = nullptr;
            }
            const char* host = std::getenv("REDIS_HOST") ? std::getenv("REDIS_HOST") : "redis";
            int port = std::getenv("REDIS_PORT") ? std::stoi(std::getenv("REDIS_PORT")) : 6379;
            struct timeval tv = {2, 0}; // 2 second connect timeout
            ctx = redisConnectWithTimeout(host, port, tv);
        }
        return ctx;
    }

    // Store a key with TTL (seconds)
    static void setex(const std::string& key, int ttl, const std::string& value = "1") {
        std::lock_guard<std::mutex> lock(mutex());
        auto* c = getUnlocked();
        if (!c) return;
        freeReplyObject(redisCommand(c, "SETEX %s %d %s", key.c_str(), ttl, value.c_str()));
    }

    // Check if key exists (returns true if blacklisted)
    static bool exists(const std::string& key) {
        std::lock_guard<std::mutex> lock(mutex());
        auto* c = getUnlocked();
        if (!c) return false;
        auto* reply = static_cast<redisReply*>(redisCommand(c, "EXISTS %s", key.c_str()));
        bool result = reply && reply->integer == 1;
        freeReplyObject(reply);
        return result;
    }

    // Increment rate-limit counter, set TTL if new (atomic via Lua script)
    static long long incr(const std::string& key, int ttlSeconds) {
        std::lock_guard<std::mutex> lock(mutex());
        auto* c = getUnlocked();
        if (!c) return 0;
        // Use a Lua script to make INCR + EXPIRE atomic — no race window
        const char* lua =
            "local v = redis.call('INCR', KEYS[1]) "
            "if v == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end "
            "return v";
        std::string ttlStr = std::to_string(ttlSeconds);
        auto* reply = static_cast<redisReply*>(
            redisCommand(c, "EVAL %s 1 %s %s", lua, key.c_str(), ttlStr.c_str()));
        long long val = (reply && reply->type == REDIS_REPLY_INTEGER) ? reply->integer : 0;
        freeReplyObject(reply);
        return val;
    }

private:
    static std::mutex& mutex() {
        static std::mutex m;
        return m;
    }
    // Unsynchronized getter — caller must already hold the mutex
    static redisContext* getUnlocked() {
        static redisContext* ctx = nullptr;
        if (!ctx || ctx->err) {
            if (ctx) { redisFree(ctx); ctx = nullptr; }
            const char* host = std::getenv("REDIS_HOST") ? std::getenv("REDIS_HOST") : "redis";
            int port = std::getenv("REDIS_PORT") ? std::stoi(std::getenv("REDIS_PORT")) : 6379;
            struct timeval tv = {2, 0};
            ctx = redisConnectWithTimeout(host, port, tv);
        }
        return (ctx && !ctx->err) ? ctx : nullptr;
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
