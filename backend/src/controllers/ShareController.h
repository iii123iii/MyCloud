#pragma once
#include <drogon/HttpController.h>
#include <chrono>
#include <ctime>
#include "utils/ResponseUtils.h"
#include "utils/UuidUtils.h"
#include "services/ShareService.h"
#include "services/StorageService.h"

class ShareController : public drogon::HttpController<ShareController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(ShareController::listShares,    "/api/shares",          drogon::Get);
        ADD_METHOD_TO(ShareController::createShare,   "/api/shares",          drogon::Post);
        ADD_METHOD_TO(ShareController::deleteShare,   "/api/shares/{id}",     drogon::Delete);
        ADD_METHOD_TO(ShareController::resolveShare,  "/api/s/{token}",       drogon::Get);
        ADD_METHOD_TO(ShareController::downloadShare, "/api/s/{token}/download", drogon::Get);
    METHOD_LIST_END

    void listShares(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT s.id,s.token,s.permission,s.expires_at,s.created_at,"
            "s.file_id,s.folder_id,f.name AS file_name "
            "FROM shares s LEFT JOIN files f ON s.file_id=f.id "
            "WHERE s.created_by=? ORDER BY s.created_at DESC",
            [cb](const drogon::orm::Result& r) {
                Json::Value arr(Json::arrayValue);
                for (const auto& row : r) {
                    Json::Value s;
                    s["id"]         = row["id"].as<std::string>();
                    s["token"]      = row["token"].as<std::string>();
                    s["permission"] = row["permission"].as<std::string>();
                    s["created_at"] = row["created_at"].as<std::string>();
                    if (!row["expires_at"].isNull())
                        s["expires_at"] = row["expires_at"].as<std::string>();
                    if (!row["file_id"].isNull())
                        s["file_id"] = row["file_id"].as<std::string>();
                    if (!row["file_name"].isNull())
                        s["file_name"] = row["file_name"].as<std::string>();
                    arr.append(s);
                }
                Json::Value body; body["shares"] = arr;
                cb(utils::okJson(body));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            userId);
    }

    void createShare(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string fileId     = (*body)["file_id"].asString();
        std::string folderId   = (*body)["folder_id"].asString();
        std::string permission = (*body)["permission"].asString();
        std::string password   = (*body)["password"].asString();
        std::string expiresAt  = (*body)["expires_at"].asString();

        if (fileId.empty() && folderId.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "file_id or folder_id required"));
            return;
        }
        if (permission != "read" && permission != "write") permission = "read";

        std::string id    = utils::generateUuid();
        std::string token = services::ShareService::generateToken();
        std::string pwHash = password.empty() ? "" : services::ShareService::hashSharePassword(password);

        auto db = drogon::app().getDbClient();

        // Use parameterized queries to prevent SQL injection.
        // Pass all values as ?, using Drogon's << binder for variable arity.
        auto okCb = [cb, token](const drogon::orm::Result&) {
            Json::Value out;
            out["token"] = token;
            out["url"]   = "/s/" + token;
            cb(utils::createdJson(out));
        };
        auto errCb = [cb](const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        };

        // Shares always have either file_id or folder_id (validated above).
        // Use fully parameterized INSERT for the common file-share case.
        if (!fileId.empty() && folderId.empty()) {
            // File share
            if (pwHash.empty() && expiresAt.empty()) {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,?,NULL,?,?,NULL,NULL)",
                    okCb, errCb, id, token, fileId, userId, permission);
            } else if (!pwHash.empty() && expiresAt.empty()) {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,?,NULL,?,?,?,NULL)",
                    okCb, errCb, id, token, fileId, userId, permission, pwHash);
            } else if (pwHash.empty() && !expiresAt.empty()) {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,?,NULL,?,?,NULL,?)",
                    okCb, errCb, id, token, fileId, userId, permission, expiresAt);
            } else {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,?,NULL,?,?,?,?)",
                    okCb, errCb, id, token, fileId, userId, permission, pwHash, expiresAt);
            }
        } else {
            // Folder share
            if (pwHash.empty() && expiresAt.empty()) {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,NULL,?,?,?,NULL,NULL)",
                    okCb, errCb, id, token, folderId, userId, permission);
            } else if (!pwHash.empty() && expiresAt.empty()) {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,NULL,?,?,?,?,NULL)",
                    okCb, errCb, id, token, folderId, userId, permission, pwHash);
            } else if (pwHash.empty() && !expiresAt.empty()) {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,NULL,?,?,?,NULL,?)",
                    okCb, errCb, id, token, folderId, userId, permission, expiresAt);
            } else {
                db->execSqlAsync(
                    "INSERT INTO shares (id,token,file_id,folder_id,created_by,permission,password_hash,expires_at) "
                    "VALUES (?,?,NULL,?,?,?,?,?)",
                    okCb, errCb, id, token, folderId, userId, permission, pwHash, expiresAt);
            }
        }
    }

    void deleteShare(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                     std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "DELETE FROM shares WHERE id=? AND created_by=?",
            [cb](const drogon::orm::Result& r) {
                if (r.affectedRows() == 0)
                    cb(utils::errorJson(drogon::k404NotFound, "Share not found"));
                else
                    cb(utils::noContent());
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            id, userId);
    }

    // GET /api/s/:token  — public, no auth
    void resolveShare(const drogon::HttpRequestPtr& req,
                      std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                      std::string token) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT s.id,s.permission,s.expires_at,s.password_hash,"
            "s.file_id,s.folder_id,f.name,f.size_bytes,f.mime_type "
            "FROM shares s LEFT JOIN files f ON s.file_id=f.id "
            "WHERE s.token=?",
            [cb, req](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "Share not found"));
                    return;
                }
                auto row = r[0];
                // Check expiry — compare against current UTC time.
                // MariaDB DATETIME strings are "YYYY-MM-DD HH:MM:SS" which sort lexicographically.
                if (!row["expires_at"].isNull()) {
                    std::string exp = row["expires_at"].as<std::string>();
                    auto now = std::chrono::system_clock::now();
                    std::time_t t = std::chrono::system_clock::to_time_t(now);
                    std::tm utc{};
#ifdef _WIN32
                    gmtime_s(&utc, &t);
#else
                    gmtime_r(&t, &utc);
#endif
                    char buf[20];
                    std::strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", &utc);
                    if (exp < std::string(buf)) {
                        cb(utils::errorJson(drogon::k410Gone, "This share link has expired"));
                        return;
                    }
                }
                // If password protected, check header
                if (!row["password_hash"].isNull()) {
                    std::string provided = req->getHeader("X-Share-Password");
                    if (!services::ShareService::verifySharePassword(
                            provided, row["password_hash"].as<std::string>())) {
                        Json::Value err; err["error"] = "Password required";
                        err["password_required"] = true;
                        auto resp = drogon::HttpResponse::newHttpJsonResponse(err);
                        resp->setStatusCode(drogon::k401Unauthorized);
                        cb(resp);
                        return;
                    }
                }
                Json::Value out;
                out["permission"]  = row["permission"].as<std::string>();
                if (!row["file_id"].isNull()) {
                    out["file_id"]   = row["file_id"].as<std::string>();
                    out["file_name"] = row["name"].as<std::string>();
                    out["file_size"] = static_cast<Json::Int64>(row["size_bytes"].as<long long>());
                    out["mime_type"] = row["mime_type"].as<std::string>();
                }
                cb(utils::okJson(out));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            token);
    }

    // GET /api/s/:token/download  — public file download via share
    void downloadShare(const drogon::HttpRequestPtr& req,
                       std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                       std::string token) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT s.password_hash,s.expires_at,"
            "f.id AS file_id,f.user_id,f.name,f.mime_type,"
            "f.encryption_key_enc,f.encryption_iv,f.encryption_tag "
            "FROM shares s JOIN files f ON s.file_id=f.id "
            "WHERE s.token=? AND f.is_deleted=0",
            [cb, req](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "Share not found"));
                    return;
                }
                auto row = r[0];
                // Check expiry
                if (!row["expires_at"].isNull()) {
                    std::string exp = row["expires_at"].as<std::string>();
                    auto now = std::chrono::system_clock::now();
                    std::time_t t = std::chrono::system_clock::to_time_t(now);
                    std::tm utc{};
#ifdef _WIN32
                    gmtime_s(&utc, &t);
#else
                    gmtime_r(&t, &utc);
#endif
                    char buf[20];
                    std::strftime(buf, sizeof(buf), "%Y-%m-%d %H:%M:%S", &utc);
                    if (exp < std::string(buf)) {
                        cb(utils::errorJson(drogon::k410Gone, "This share link has expired"));
                        return;
                    }
                }
                if (!row["password_hash"].isNull()) {
                    std::string provided = req->getHeader("X-Share-Password");
                    if (!services::ShareService::verifySharePassword(
                            provided, row["password_hash"].as<std::string>())) {
                        cb(utils::errorJson(drogon::k401Unauthorized, "Invalid share password"));
                        return;
                    }
                }
                services::EncryptedKeyBundle bundle{
                    row["encryption_iv"].as<std::string>(),
                    row["encryption_key_enc"].as<std::string>(),
                    row["encryption_tag"].as<std::string>()
                };
                try {
                    std::string ownerId  = row["user_id"].as<std::string>();
                    std::string fileId   = row["file_id"].as<std::string>();
                    std::string fileName = row["name"].as<std::string>();
                    std::string mime     = row["mime_type"].as<std::string>();

                    // Stream the download using a DecryptingReader (~4 MB memory)
                    auto reader = services::StorageService::createReader(
                        ownerId, fileId, bundle);

                    auto resp = drogon::HttpResponse::newStreamResponse(
                        [reader](char* buf, std::size_t maxLen) -> std::size_t {
                            return reader->read(buf, maxLen);
                        },
                        fileName,
                        drogon::CT_CUSTOM,
                        mime);
                    // Sanitize filename to prevent header injection
                    std::string safeName;
                    safeName.reserve(fileName.size());
                    for (char c : fileName) {
                        if (c == '"' || c == '\\' || c == '\r' || c == '\n' || c == '\0')
                            continue;
                        safeName += c;
                    }
                    if (safeName.empty()) safeName = "download";
                    resp->addHeader("Content-Disposition",
                        "attachment; filename=\"" + safeName + "\"");
                    cb(resp);
                } catch (const std::exception& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.what()));
                }
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            token);
    }
};
