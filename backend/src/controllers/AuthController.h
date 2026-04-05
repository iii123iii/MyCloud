#pragma once
#include <drogon/HttpController.h>
#include "utils/ResponseUtils.h"
#include "utils/UuidUtils.h"
#include "utils/JwtUtils.h"
#include "utils/HashUtils.h"
#include "services/AuthService.h"
#include <cstdlib>

class AuthController : public drogon::HttpController<AuthController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(AuthController::login,          "/api/auth/login",           drogon::Post);
        ADD_METHOD_TO(AuthController::registerUser,   "/api/auth/register",        drogon::Post);
        ADD_METHOD_TO(AuthController::refresh,        "/api/auth/refresh",         drogon::Post);
        ADD_METHOD_TO(AuthController::logout,         "/api/auth/logout",          drogon::Post);
        ADD_METHOD_TO(AuthController::me,             "/api/auth/me",              drogon::Get);
        ADD_METHOD_TO(AuthController::changePassword, "/api/auth/change-password", drogon::Post);
    METHOD_LIST_END

    // POST /api/auth/login
    void login(const drogon::HttpRequestPtr& req,
               std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        // Rate limit by IP
        std::string ip = req->getPeerAddr().toIp();
        if (!services::AuthService::checkRateLimit(ip)) {
            cb(utils::errorJson(drogon::k429TooManyRequests, "Too many login attempts. Try again in 15 minutes."));
            return;
        }

        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string login_field = (*body)["email"].asString();
        if (login_field.empty()) login_field = (*body)["username"].asString();
        std::string password = (*body)["password"].asString();

        if (login_field.empty() || password.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "email/username and password required"));
            return;
        }

        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT id,username,email,password_hash,role,is_active,must_change_password "
            "FROM users WHERE email=? OR username=? LIMIT 1",
            [cb, password](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k401Unauthorized, "Invalid credentials"));
                    return;
                }
                auto row = r[0];
                if (!row["is_active"].as<bool>()) {
                    cb(utils::errorJson(drogon::k403Forbidden, "Account is disabled"));
                    return;
                }
                if (!utils::verifyPassword(password, row["password_hash"].as<std::string>())) {
                    cb(utils::errorJson(drogon::k401Unauthorized, "Invalid credentials"));
                    return;
                }
                std::string userId = row["id"].as<std::string>();
                std::string role   = row["role"].as<std::string>();

                auto tokens = services::AuthService::issueTokenPair(userId, role);
                tokens["user_id"]              = userId;
                tokens["username"]             = row["username"].as<std::string>();
                tokens["email"]                = row["email"].as<std::string>();
                tokens["role"]                 = role;
                tokens["must_change_password"] = row["must_change_password"].as<bool>();
                cb(utils::okJson(tokens));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            login_field, login_field);
    }

    // POST /api/auth/register
    void registerUser(const drogon::HttpRequestPtr& req,
                      std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        // Check if registration is enabled
        auto db   = drogon::app().getDbClient();
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string username = (*body)["username"].asString();
        std::string email    = (*body)["email"].asString();
        std::string password = (*body)["password"].asString();

        if (username.size() < 3 || email.empty() || password.size() < 8) {
            cb(utils::errorJson(drogon::k400BadRequest,
               "username (min 3), email, and password (min 8 chars) are required"));
            return;
        }

        db->execSqlAsync(
            "SELECT value FROM settings WHERE key_name='registration_enabled'",
            [cb, db, username, email, password](const drogon::orm::Result& r) {
                bool enabled = r.empty() || r[0][0].as<std::string>() == "true";
                if (!enabled) {
                    cb(utils::errorJson(drogon::k403Forbidden, "Registration is disabled by administrator"));
                    return;
                }

                // Get default quota
                db->execSqlAsync(
                    "SELECT value FROM settings WHERE key_name='default_quota_bytes'",
                    [cb, db, username, email, password](const drogon::orm::Result& qr) {
                        long long quota = qr.empty() ? 10737418240LL
                                                     : std::stoll(qr[0][0].as<std::string>());
                        std::string id   = utils::generateUuid();
                        std::string hash = utils::hashPassword(password);

                        db->execSqlAsync(
                            "INSERT INTO users (id,username,email,password_hash,quota_bytes) VALUES (?,?,?,?,?)",
                            [cb, id, username](const drogon::orm::Result&) {
                                auto tokens = services::AuthService::issueTokenPair(id, "user");
                                tokens["user_id"]  = id;
                                tokens["username"] = username;
                                tokens["role"]     = "user";
                                cb(utils::createdJson(tokens));
                            },
                            [cb](const drogon::orm::DrogonDbException& e) {
                                std::string msg = e.base().what();
                                if (msg.find("Duplicate") != std::string::npos)
                                    cb(utils::errorJson(drogon::k409Conflict, "Username or email already taken"));
                                else
                                    cb(utils::errorJson(drogon::k500InternalServerError, msg));
                            },
                            id, username, email, hash, quota);
                    },
                    [cb](const drogon::orm::DrogonDbException& e) {
                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                    });
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }

    // POST /api/auth/refresh
    void refresh(const drogon::HttpRequestPtr& req,
                 std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string token = (*body)["refresh_token"].asString();
        if (token.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "refresh_token required"));
            return;
        }
        if (services::AuthService::isBlacklisted(token)) {
            cb(utils::errorJson(drogon::k401Unauthorized, "Token revoked"));
            return;
        }

        try {
            auto claims = utils::verifyToken(token, services::AuthService::jwtSecret());
            if (claims.type != "refresh") {
                cb(utils::errorJson(drogon::k401Unauthorized, "Expected refresh token"));
                return;
            }

            // Fetch user role from DB (in case it changed)
            auto db = drogon::app().getDbClient();
            db->execSqlAsync(
                "SELECT role FROM users WHERE id=? AND is_active=1",
                [cb, userId = claims.userId](const drogon::orm::Result& r) {
                    if (r.empty()) {
                        cb(utils::errorJson(drogon::k401Unauthorized, "User not found"));
                        return;
                    }
                    auto tokens = services::AuthService::issueTokenPair(userId, r[0]["role"].as<std::string>());
                    cb(utils::okJson(tokens));
                },
                [cb](const drogon::orm::DrogonDbException& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                },
                claims.userId);
        } catch (...) {
            cb(utils::errorJson(drogon::k401Unauthorized, "Invalid or expired token"));
        }
    }

    // POST /api/auth/logout
    void logout(const drogon::HttpRequestPtr& req,
                std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto body = req->getJsonObject();
        if (body && (*body).isMember("refresh_token"))
            services::AuthService::blacklistToken((*body)["refresh_token"].asString());
        cb(utils::noContent());
    }

    // GET /api/auth/me  (requires AuthMiddleware)
    void me(const drogon::HttpRequestPtr& req,
            std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT id,username,email,role,quota_bytes,used_bytes,is_active,must_change_password,created_at "
            "FROM users WHERE id=?",
            [cb](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "User not found"));
                    return;
                }
                auto row = r[0];
                Json::Value u;
                u["id"]                   = row["id"].as<std::string>();
                u["username"]             = row["username"].as<std::string>();
                u["email"]                = row["email"].as<std::string>();
                u["role"]                 = row["role"].as<std::string>();
                u["quota_bytes"]          = static_cast<Json::Int64>(row["quota_bytes"].as<long long>());
                u["used_bytes"]           = static_cast<Json::Int64>(row["used_bytes"].as<long long>());
                u["is_active"]            = row["is_active"].as<bool>();
                u["must_change_password"] = row["must_change_password"].as<bool>();
                u["created_at"]           = row["created_at"].as<std::string>();
                cb(utils::okJson(u));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            userId);
    }

    // POST /api/auth/change-password  (requires AuthMiddleware)
    void changePassword(const drogon::HttpRequestPtr& req,
                        std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string oldPass = (*body)["old_password"].asString();
        std::string newPass = (*body)["new_password"].asString();
        if (newPass.size() < 8) {
            cb(utils::errorJson(drogon::k400BadRequest, "New password must be at least 8 characters"));
            return;
        }

        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT password_hash FROM users WHERE id=?",
            [cb, db, userId, oldPass, newPass](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "User not found"));
                    return;
                }
                if (!utils::verifyPassword(oldPass, r[0]["password_hash"].as<std::string>())) {
                    cb(utils::errorJson(drogon::k401Unauthorized, "Current password is incorrect"));
                    return;
                }
                std::string hash = utils::hashPassword(newPass);
                db->execSqlAsync(
                    "UPDATE users SET password_hash=?, must_change_password=0 WHERE id=?",
                    [cb](const drogon::orm::Result&) {
                        Json::Value ok;
                        ok["message"] = "Password changed successfully";
                        cb(utils::okJson(ok));
                    },
                    [cb](const drogon::orm::DrogonDbException& e) {
                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                    },
                    hash, userId);
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            userId);
    }
};
