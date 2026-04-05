#pragma once
#include <drogon/HttpController.h>
#include "utils/ResponseUtils.h"
#include "utils/UuidUtils.h"
#include "utils/HashUtils.h"

class SetupController : public drogon::HttpController<SetupController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(SetupController::getStatus,   "/api/setup/status",   drogon::Get);
        ADD_METHOD_TO(SetupController::complete,    "/api/setup/complete", drogon::Post);
    METHOD_LIST_END

    // GET /api/setup/status
    void getStatus(const drogon::HttpRequestPtr& req,
                   std::function<void(const drogon::HttpResponsePtr&)>&& callback) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT value FROM settings WHERE key_name='setup_complete'",
            [callback](const drogon::orm::Result& r) {
                Json::Value body;
                bool done = !r.empty() && r[0][0].as<std::string>() == "true";
                body["setup_complete"] = done;
                callback(utils::okJson(body));
            },
            [callback](const drogon::orm::DrogonDbException& e) {
                callback(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }

    // POST /api/setup/complete
    void complete(const drogon::HttpRequestPtr& req,
                  std::function<void(const drogon::HttpResponsePtr&)>&& callback) {
        auto db = drogon::app().getDbClient();

        // Check not already done
        db->execSqlAsync(
            "SELECT value FROM settings WHERE key_name='setup_complete'",
            [this, req, callback, db](const drogon::orm::Result& r) {
                if (!r.empty() && r[0][0].as<std::string>() == "true") {
                    callback(utils::errorJson(drogon::k400BadRequest, "Setup already completed"));
                    return;
                }

                auto body = req->getJsonObject();
                if (!body) {
                    callback(utils::errorJson(drogon::k400BadRequest, "Invalid JSON"));
                    return;
                }

                std::string username = (*body)["username"].asString();
                std::string email    = (*body)["email"].asString();
                std::string password = (*body)["password"].asString();

                if (username.empty() || email.empty() || password.size() < 8) {
                    callback(utils::errorJson(drogon::k400BadRequest,
                        "username, email and password (min 8 chars) are required"));
                    return;
                }

                std::string id   = utils::generateUuid();
                std::string hash = utils::hashPassword(password);

                db->execSqlAsync(
                    "INSERT INTO users (id,username,email,password_hash,role) VALUES (?,?,?,?,'admin')",
                    [callback, db](const drogon::orm::Result&) {
                        db->execSqlAsync(
                            "UPDATE settings SET value='true' WHERE key_name='setup_complete'",
                            [callback](const drogon::orm::Result&) {
                                Json::Value out;
                                out["message"] = "Setup complete. You can now log in.";
                                callback(utils::createdJson(out));
                            },
                            [callback](const drogon::orm::DrogonDbException& e) {
                                callback(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                            });
                    },
                    [callback](const drogon::orm::DrogonDbException& e) {
                        std::string msg = e.base().what();
                        if (msg.find("Duplicate") != std::string::npos)
                            callback(utils::errorJson(drogon::k409Conflict, "Username or email already taken"));
                        else
                            callback(utils::errorJson(drogon::k500InternalServerError, msg));
                    },
                    id, username, email, hash);
            },
            [callback](const drogon::orm::DrogonDbException& e) {
                callback(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            });
    }
};
