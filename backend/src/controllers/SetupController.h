#pragma once
#include <algorithm>
#include <cctype>
#include <drogon/HttpController.h>
#include <regex>
#include <string>
#include <thread>
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
        auto body = req->getJsonObject();
        if (!body) {
            callback(utils::errorJson(drogon::k400BadRequest, "Invalid JSON"));
            return;
        }

        std::string username = trim((*body)["username"].asString());
        std::string email = trim((*body)["email"].asString());
        std::string password = (*body)["password"].asString();

        const auto validationError = validateInput(username, email, password);
        if (!validationError.empty()) {
            callback(utils::errorJson(drogon::k400BadRequest, validationError));
            return;
        }

        auto db = drogon::app().getDbClient();
        std::string id = utils::generateUuid();
        std::string hash = utils::hashPassword(password);

        std::thread([db,
                     callback = std::move(callback),
                     id = std::move(id),
                     username = std::move(username),
                     email = std::move(email),
                     hash = std::move(hash)]() mutable {
            try {
                auto tx = db->newTransaction();
                const auto status = tx->execSqlSync(
                    "SELECT value FROM settings WHERE key_name='setup_complete' FOR UPDATE");
                if (!status.empty() && status[0][0].as<std::string>() == "true") {
                    tx.reset();
                    callback(utils::errorJson(drogon::k400BadRequest, "Setup already completed"));
                    return;
                }

                tx->execSqlSync(
                    "INSERT INTO users (id,username,email,password_hash,role) VALUES (?,?,?,?,'admin')",
                    id, username, email, hash);
                tx->execSqlSync(
                    "INSERT INTO settings (key_name,value) VALUES ('setup_complete','true') "
                    "ON DUPLICATE KEY UPDATE value=VALUES(value)");
                tx.reset();

                Json::Value out;
                out["message"] = "Setup complete. You can now log in.";
                callback(utils::createdJson(out));
            } catch (const drogon::orm::DrogonDbException& e) {
                std::string msg = e.base().what();
                if (msg.find("Duplicate") != std::string::npos) {
                    callback(utils::errorJson(drogon::k409Conflict, "Username or email already taken"));
                } else {
                    callback(utils::errorJson(drogon::k500InternalServerError, msg));
                }
            } catch (const std::exception& e) {
                callback(utils::errorJson(drogon::k500InternalServerError, e.what()));
            }
        }).detach();
    }

private:
    static constexpr std::size_t kMaxUsernameLength = 50;
    static constexpr std::size_t kMaxEmailLength = 255;

    static std::string trim(std::string value) {
        const auto notSpace = [](unsigned char ch) { return !std::isspace(ch); };
        value.erase(value.begin(), std::find_if(value.begin(), value.end(), notSpace));
        value.erase(std::find_if(value.rbegin(), value.rend(), notSpace).base(), value.end());
        return value;
    }

    static bool isValidEmail(const std::string& email) {
        static const std::regex kEmailPattern(
            R"(^[A-Z0-9.!#$%&'*+/=?^_`{|}~-]+@[A-Z0-9-]+(?:\.[A-Z0-9-]+)+$)",
            std::regex::icase);
        return std::regex_match(email, kEmailPattern);
    }

    static std::string validateInput(const std::string& username,
                                     const std::string& email,
                                     const std::string& password) {
        if (username.size() < 3 || username.size() > kMaxUsernameLength) {
            return "Username must be between 3 and 50 characters.";
        }
        if (email.empty() || email.size() > kMaxEmailLength) {
            return "Email must be between 1 and 255 characters.";
        }
        if (!isValidEmail(email)) {
            return "Email address is not valid.";
        }
        if (password.size() < 8) {
            return "Password must be at least 8 characters.";
        }
        return "";
    }
};
