#pragma once
#include <drogon/HttpMiddleware.h>
#include "utils/JwtUtils.h"
#include "utils/ResponseUtils.h"
#include <cstdlib>
#include <string>

// Injects userId and role into request attributes after JWT verification.
class AuthMiddleware : public drogon::HttpMiddleware<AuthMiddleware> {
public:
    AuthMiddleware() = default;

    void invoke(const drogon::HttpRequestPtr& req,
                drogon::MiddlewareNextCallback&& nextCb,
                drogon::FilterCallback&& stopCb) override {
        const std::string secret = std::getenv("JWT_SECRET") ? std::getenv("JWT_SECRET") : "";
        const auto auth = req->getHeader("Authorization");

        if (auth.empty() || auth.rfind("Bearer ", 0) != 0) {
            stopCb(utils::errorJson(drogon::k401Unauthorized, "Missing or invalid Authorization header"));
            return;
        }

        const std::string token = auth.substr(7);
        try {
            auto claims = utils::verifyToken(token, secret);
            if (claims.type != "access") {
                stopCb(utils::errorJson(drogon::k401Unauthorized, "Expected access token"));
                return;
            }
            req->getAttributes()->insert("userId", claims.userId);
            req->getAttributes()->insert("userRole", claims.role);
            nextCb(std::move(stopCb));
        } catch (const std::exception& e) {
            stopCb(utils::errorJson(drogon::k401Unauthorized, std::string("Invalid token: ") + e.what()));
        }
    }
};
