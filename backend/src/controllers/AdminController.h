#pragma once
#include <drogon/HttpController.h>
#include "utils/ResponseUtils.h"
#include "utils/UuidUtils.h"
#include "utils/HashUtils.h"

class AdminController : public drogon::HttpController<AdminController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(AdminController::listUsers,   "/api/admin/users",         drogon::Get);
        ADD_METHOD_TO(AdminController::createUser,  "/api/admin/users",         drogon::Post);
        ADD_METHOD_TO(AdminController::updateUser,  "/api/admin/users/{id}",    drogon::Patch);
        ADD_METHOD_TO(AdminController::deleteUser,  "/api/admin/users/{id}",    drogon::Delete);
        ADD_METHOD_TO(AdminController::getStats,    "/api/admin/stats",         drogon::Get);
        ADD_METHOD_TO(AdminController::getLogs,     "/api/admin/logs",          drogon::Get);
        ADD_METHOD_TO(AdminController::getSettings, "/api/admin/settings",      drogon::Get);
        ADD_METHOD_TO(AdminController::putSettings, "/api/admin/settings",      drogon::Put);
    METHOD_LIST_END

    void listUsers(const drogon::HttpRequestPtr& req,
                   std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT id,username,email,role,quota_bytes,used_bytes,is_active,created_at FROM users ORDER BY created_at DESC",
            [cb](const drogon::orm::Result& r) {
                Json::Value arr(Json::arrayValue);
                for (const auto& row : r) {
                    Json::Value u;
                    u["id"]          = row["id"].as<std::string>();
                    u["username"]    = row["username"].as<std::string>();
                    u["email"]       = row["email"].as<std::string>();
                    u["role"]        = row["role"].as<std::string>();
                    u["quota_bytes"] = static_cast<Json::Int64>(row["quota_bytes"].as<long long>());
                    u["used_bytes"]  = static_cast<Json::Int64>(row["used_bytes"].as<long long>());
                    u["is_active"]   = row["is_active"].as<bool>();
                    u["created_at"]  = row["created_at"].as<std::string>();
                    arr.append(u);
                }
                Json::Value body; body["users"] = arr;
                cb(utils::okJson(body));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }

    void createUser(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string username = (*body)["username"].asString();
        std::string email    = (*body)["email"].asString();
        std::string password = (*body)["password"].asString();
        std::string role     = (*body)["role"].asString();
        if (role != "admin" && role != "user") role = "user";
        long long quota = (*body)["quota_bytes"].asInt64();
        if (quota <= 0) quota = 10737418240LL;

        if (username.empty() || email.empty() || password.size() < 8) {
            cb(utils::errorJson(drogon::k400BadRequest, "username, email, password(min 8) required"));
            return;
        }

        std::string id   = utils::generateUuid();
        std::string hash = utils::hashPassword(password);
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "INSERT INTO users (id,username,email,password_hash,role,quota_bytes) VALUES (?,?,?,?,?,?)",
            [cb, id, username](const drogon::orm::Result&) {
                Json::Value out; out["id"] = id; out["username"] = username;
                cb(utils::createdJson(out));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                std::string msg = e.base().what();
                if (msg.find("Duplicate") != std::string::npos)
                    cb(utils::errorJson(drogon::k409Conflict, "Username or email already taken"));
                else
                    cb(utils::errorJson(drogon::k500InternalServerError, msg));
            },
            id, username, email, hash, role, quota);
    }

    void updateUser(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                    std::string id) {
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        auto db = drogon::app().getDbClient();

        // Build SET clause using parameterized placeholders to prevent SQL injection
        std::vector<std::string> setClauses;
        std::vector<std::string> paramValues;

        if ((*body).isMember("is_active")) {
            setClauses.push_back("is_active=?");
            paramValues.push_back(std::to_string((*body)["is_active"].asBool() ? 1 : 0));
        }
        if ((*body).isMember("role")) {
            std::string role = (*body)["role"].asString();
            if (role == "admin" || role == "user") {
                setClauses.push_back("role=?");
                paramValues.push_back(role);
            }
        }
        if ((*body).isMember("quota_bytes")) {
            setClauses.push_back("quota_bytes=?");
            paramValues.push_back(std::to_string((*body)["quota_bytes"].asInt64()));
        }
        if ((*body).isMember("password") && (*body)["password"].asString().size() >= 8) {
            std::string hash = utils::hashPassword((*body)["password"].asString());
            setClauses.push_back("password_hash=?");
            paramValues.push_back(hash);
        }

        if (setClauses.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "Nothing to update")); return;
        }

        std::string sql = "UPDATE users SET ";
        for (size_t i = 0; i < setClauses.size(); ++i) {
            sql += setClauses[i];
            if (i + 1 < setClauses.size()) sql += ",";
        }
        sql += " WHERE id=?";
        paramValues.push_back(id);

        // Use execSqlAsync with the correct number of parameters.
        // Since the count is dynamic, we handle common cases explicitly.
        auto okCb = [cb](const drogon::orm::Result&) {
            Json::Value ok; ok["message"] = "User updated";
            cb(utils::okJson(ok));
        };
        auto errCb = [cb](const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        };

        switch (paramValues.size()) {
            case 2: db->execSqlAsync(sql, okCb, errCb, paramValues[0], paramValues[1]); break;
            case 3: db->execSqlAsync(sql, okCb, errCb, paramValues[0], paramValues[1], paramValues[2]); break;
            case 4: db->execSqlAsync(sql, okCb, errCb, paramValues[0], paramValues[1], paramValues[2], paramValues[3]); break;
            case 5: db->execSqlAsync(sql, okCb, errCb, paramValues[0], paramValues[1], paramValues[2], paramValues[3], paramValues[4]); break;
            default: cb(utils::errorJson(drogon::k400BadRequest, "Nothing to update")); break;
        }
    }

    void deleteUser(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                    std::string id) {
        std::string selfId = req->getAttributes()->get<std::string>("userId");
        if (id == selfId) {
            cb(utils::errorJson(drogon::k400BadRequest, "Cannot delete your own account"));
            return;
        }
        auto db = drogon::app().getDbClient();
        db->execSqlAsync("DELETE FROM users WHERE id=?",
            [cb](const drogon::orm::Result&) { cb(utils::noContent()); },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            }, id);
    }

    void getStats(const drogon::HttpRequestPtr& req,
                  std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT (SELECT COUNT(*) FROM users) AS total_users,"
            "(SELECT COUNT(*) FROM files WHERE is_deleted=0) AS total_files,"
            "(SELECT COALESCE(SUM(size_bytes),0) FROM files WHERE is_deleted=0) AS total_storage_used,"
            "(SELECT COALESCE(SUM(quota_bytes),0) FROM users) AS total_quota",
            [cb](const drogon::orm::Result& r) {
                Json::Value s;
                s["total_users"]         = r[0]["total_users"].as<int>();
                s["total_files"]         = r[0]["total_files"].as<int>();
                s["total_storage_used"]  = static_cast<Json::Int64>(r[0]["total_storage_used"].as<long long>());
                s["total_quota"]         = static_cast<Json::Int64>(r[0]["total_quota"].as<long long>());
                cb(utils::okJson(s));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }

    void getLogs(const drogon::HttpRequestPtr& req,
                 std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string limitStr = req->getParameter("limit");
        // Sanitize limit to a safe integer to prevent SQL injection
        int limit = 100;
        if (!limitStr.empty()) {
            try { limit = std::stoi(limitStr); } catch (...) { limit = 100; }
            if (limit <= 0 || limit > 10000) limit = 100;
        }
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT a.id,a.action,a.resource_type,a.resource_id,a.ip_address,a.created_at,u.username "
            "FROM activity_log a LEFT JOIN users u ON a.user_id=u.id "
            "ORDER BY a.created_at DESC LIMIT " + std::to_string(limit),
            [cb](const drogon::orm::Result& r) {
                Json::Value arr(Json::arrayValue);
                for (const auto& row : r) {
                    Json::Value log;
                    log["id"]            = static_cast<Json::Int64>(row["id"].as<long long>());
                    log["action"]        = row["action"].as<std::string>();
                    log["ip_address"]    = row["ip_address"].as<std::string>();
                    log["created_at"]    = row["created_at"].as<std::string>();
                    if (!row["username"].isNull())
                        log["username"] = row["username"].as<std::string>();
                    if (!row["resource_type"].isNull())
                        log["resource_type"] = row["resource_type"].as<std::string>();
                    arr.append(log);
                }
                Json::Value body; body["logs"] = arr;
                cb(utils::okJson(body));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }

    void getSettings(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync("SELECT key_name, value FROM settings",
            [cb](const drogon::orm::Result& r) {
                Json::Value s;
                for (const auto& row : r)
                    s[row["key_name"].as<std::string>()] = row["value"].as<std::string>();
                cb(utils::okJson(s));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }

    void putSettings(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        auto db = drogon::app().getDbClient();
        // Only allow updating specific safe keys
        static const std::vector<std::string> allowed = {
            "registration_enabled", "default_quota_bytes"
        };
        for (const auto& key : allowed) {
            if ((*body).isMember(key)) {
                db->execSqlAsync(
                    "INSERT INTO settings (key_name,value) VALUES (?,?) ON DUPLICATE KEY UPDATE value=?",
                    [](const drogon::orm::Result&){},
                    [](const drogon::orm::DrogonDbException&){},
                    key, (*body)[key].asString(), (*body)[key].asString());
            }
        }
        Json::Value ok; ok["message"] = "Settings updated";
        cb(utils::okJson(ok));
    }
};
