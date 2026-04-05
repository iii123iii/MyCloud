#pragma once
#include <drogon/HttpController.h>
#include <drogon/MultiPart.h>
#include "utils/ResponseUtils.h"
#include "utils/UuidUtils.h"
#include "services/StorageService.h"
#include <unordered_map>
#include <algorithm>

// Simple extension → MIME lookup
static std::string mimeFromFilename(const std::string& name) {
    auto dot = name.rfind('.');
    if (dot == std::string::npos) return "application/octet-stream";
    std::string ext = name.substr(dot + 1);
    std::transform(ext.begin(), ext.end(), ext.begin(), ::tolower);
    static const std::unordered_map<std::string, std::string> m = {
        {"jpg","image/jpeg"},{"jpeg","image/jpeg"},{"png","image/png"},
        {"gif","image/gif"},{"webp","image/webp"},{"svg","image/svg+xml"},
        {"pdf","application/pdf"},
        {"mp4","video/mp4"},{"webm","video/webm"},{"mov","video/quicktime"},
        {"avi","video/x-msvideo"},{"mkv","video/x-matroska"},
        {"mp3","audio/mpeg"},{"wav","audio/wav"},{"ogg","audio/ogg"},
        {"flac","audio/flac"},{"aac","audio/aac"},
        {"txt","text/plain"},{"md","text/markdown"},{"html","text/html"},
        {"css","text/css"},{"js","text/javascript"},{"ts","text/plain"},
        {"json","application/json"},{"xml","application/xml"},
        {"zip","application/zip"},{"tar","application/x-tar"},
        {"gz","application/gzip"},{"7z","application/x-7z-compressed"},
        {"docx","application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
        {"xlsx","application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
        {"pptx","application/vnd.openxmlformats-officedocument.presentationml.presentation"},
        {"doc","application/msword"},{"xls","application/vnd.ms-excel"},
    };
    auto it = m.find(ext);
    return it != m.end() ? it->second : "application/octet-stream";
}

// Safe integer parse with default
static int safeInt(const std::string& s, int def, int minVal, int maxVal) {
    if (s.empty()) return def;
    try {
        int v = std::stoi(s);
        return std::max(minVal, std::min(maxVal, v));
    } catch (...) { return def; }
}

class FileController : public drogon::HttpController<FileController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(FileController::listFiles,  "/api/files",             drogon::Get);
        ADD_METHOD_TO(FileController::upload,     "/api/files/upload",      drogon::Post);
        ADD_METHOD_TO(FileController::download,   "/api/files/{id}/download", drogon::Get);
        ADD_METHOD_TO(FileController::preview,    "/api/files/{id}/preview",  drogon::Get);
        ADD_METHOD_TO(FileController::getInfo,    "/api/files/{id}",          drogon::Get);
        ADD_METHOD_TO(FileController::updateFile, "/api/files/{id}",          drogon::Patch);
        ADD_METHOD_TO(FileController::deleteFile, "/api/files/{id}",          drogon::Delete);
    METHOD_LIST_END

    // ── GET /api/files?folder_id=&sort=&order=&all=1&page=1&page_size=50 ─
    void listFiles(const drogon::HttpRequestPtr& req,
                   std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId   = req->getAttributes()->get<std::string>("userId");
        std::string folderId = req->getParameter("folder_id");
        std::string sort     = req->getParameter("sort");
        std::string order    = req->getParameter("order");
        bool        all      = req->getParameter("all") == "1";
        bool        starredOnly = req->getParameter("starred_only") == "1";
        int page     = safeInt(req->getParameter("page"),      1, 1, 100000);
        int pageSize = safeInt(req->getParameter("page_size"), 50, 1, 200);
        int offset   = (page - 1) * pageSize;

        if (sort.empty())  sort  = "name";
        if (order.empty()) order = "asc";
        if (sort != "name" && sort != "size_bytes" && sort != "created_at" && sort != "updated_at")
            sort = "name";
        if (order != "asc" && order != "desc") order = "asc";

        auto db = drogon::app().getDbClient();

        // ── Build WHERE clause ────────────────────────────────────────
        std::string where = "WHERE user_id=? AND is_deleted=0";
        if (starredOnly)
            where += " AND is_starred=1";
        if (!all) {
            if (folderId.empty())
                where += " AND folder_id IS NULL";
            else
                where += " AND folder_id=?";
        }

        std::string countSql = "SELECT COUNT(*) AS total FROM files " + where;
        std::string dataSql  =
            "SELECT id,name,size_bytes,mime_type,folder_id,is_starred,created_at,updated_at "
            "FROM files " + where +
            " ORDER BY " + sort + " " + order +
            " LIMIT " + std::to_string(pageSize) +
            " OFFSET " + std::to_string(offset);

        auto buildResult = [cb, page, pageSize](
                const drogon::orm::Result& countR,
                const drogon::orm::Result& dataR) {
            long long total = countR[0]["total"].as<long long>();
            Json::Value arr(Json::arrayValue);
            for (const auto& row : dataR) {
                Json::Value f;
                f["id"]         = row["id"].as<std::string>();
                f["name"]       = row["name"].as<std::string>();
                f["size_bytes"] = static_cast<Json::Int64>(row["size_bytes"].as<long long>());
                f["mime_type"]  = row["mime_type"].as<std::string>();
                f["is_starred"] = row["is_starred"].as<bool>();
                f["created_at"] = row["created_at"].as<std::string>();
                f["updated_at"] = row["updated_at"].as<std::string>();
                if (!row["folder_id"].isNull())
                    f["folder_id"] = row["folder_id"].as<std::string>();
                arr.append(f);
            }
            Json::Value body;
            body["files"]     = arr;
            body["total"]     = static_cast<Json::Int64>(total);
            body["page"]      = page;
            body["page_size"] = pageSize;
            body["has_more"]  = (page * pageSize) < total;
            cb(utils::okJson(body));
        };

        auto errCb = [cb](const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        };

        // Execute count then data query
        if (!all && !folderId.empty()) {
            db->execSqlAsync(countSql,
                [cb, db, dataSql, folderId, userId, buildResult, errCb](const drogon::orm::Result& countR) {
                    db->execSqlAsync(dataSql,
                        [countR, buildResult](const drogon::orm::Result& dataR) {
                            buildResult(countR, dataR);
                        }, errCb, userId, folderId);
                }, errCb, userId, folderId);
        } else {
            db->execSqlAsync(countSql,
                [cb, db, dataSql, userId, buildResult, errCb](const drogon::orm::Result& countR) {
                    db->execSqlAsync(dataSql,
                        [countR, buildResult](const drogon::orm::Result& dataR) {
                            buildResult(countR, dataR);
                        }, errCb, userId);
                }, errCb, userId);
        }
    }

    // ── POST /api/files/upload (multipart) ───────────────────────────────
    void upload(const drogon::HttpRequestPtr& req,
                std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");

        drogon::MultiPartParser parser;
        if (parser.parse(req) != 0) {
            cb(utils::errorJson(drogon::k400BadRequest, "Multipart parse error"));
            return;
        }

        auto& files = parser.getFiles();
        if (files.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "No file in request"));
            return;
        }

        const auto& file = files[0];
        std::string filename = file.getFileName();
        if (filename.empty()) filename = "untitled";

        std::string folderId = parser.getParameters().count("folder_id")
                                ? parser.getParameters().at("folder_id") : "";

        // Copy file bytes before going async (parser is stack-local)
        const char*   rawPtr = file.fileData();
        std::size_t   rawLen = static_cast<std::size_t>(file.fileLength());
        long long     size   = static_cast<long long>(rawLen);
        std::vector<unsigned char> fileBytes(
            reinterpret_cast<const unsigned char*>(rawPtr),
            reinterpret_cast<const unsigned char*>(rawPtr) + rawLen);

        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT quota_bytes, used_bytes FROM users WHERE id=?",
            [cb, db, userId, folderId, filename, fileBytes, size]
            (const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "User not found"));
                    return;
                }
                long long quota = r[0]["quota_bytes"].as<long long>();
                long long used  = r[0]["used_bytes"].as<long long>();
                if (used + size > quota) {
                    cb(utils::errorJson(drogon::k413RequestEntityTooLarge,
                        "Storage quota exceeded"));
                    return;
                }

                std::string fileId = utils::generateUuid();
                std::string mime   = mimeFromFilename(filename);

                // Encrypt in chunks (4 MB at a time) and write to disk
                auto bundle = services::StorageService::storeFile(
                    userId, fileId, fileBytes.data(), fileBytes.size());

                std::string storagePath = services::StorageService::filePath(userId, fileId);
                auto insertCb = [cb, db, userId, size, fileId, filename, folderId](const drogon::orm::Result&) {
                    db->execSqlAsync(
                        "UPDATE users SET used_bytes=used_bytes+? WHERE id=?",
                        [cb, fileId, filename, folderId](const drogon::orm::Result&) {
                            Json::Value ok;
                            ok["message"] = "File uploaded";
                            ok["id"] = fileId;
                            ok["name"] = filename;
                            if (folderId.empty())
                                ok["folder_id"] = Json::nullValue;
                            else
                                ok["folder_id"] = folderId;
                            cb(utils::createdJson(ok));
                        },
                        [cb](const drogon::orm::DrogonDbException& e) {
                            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                        }, size, userId);
                };
                auto insertErr = [cb](const drogon::orm::DrogonDbException& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                };

                if (folderId.empty()) {
                    db->execSqlAsync(
                        "INSERT INTO files (id,name,storage_path,size_bytes,mime_type,user_id,folder_id,"
                        "encryption_key_enc,encryption_iv,encryption_tag) VALUES (?,?,?,?,?,?,NULL,?,?,?)",
                        insertCb, insertErr,
                        fileId, filename, storagePath, size, mime, userId,
                        bundle.encKeyHex, bundle.ivHex, bundle.tagHex);
                } else {
                    db->execSqlAsync(
                        "INSERT INTO files (id,name,storage_path,size_bytes,mime_type,user_id,folder_id,"
                        "encryption_key_enc,encryption_iv,encryption_tag) VALUES (?,?,?,?,?,?,?,?,?,?)",
                        insertCb, insertErr,
                        fileId, filename, storagePath, size, mime, userId, folderId,
                        bundle.encKeyHex, bundle.ivHex, bundle.tagHex);
                }
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            }, userId);
    }

    void download(const drogon::HttpRequestPtr& req,
                  std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                  std::string id) {
        serveFile(req, std::move(cb), id, "attachment");
    }

    void preview(const drogon::HttpRequestPtr& req,
                 std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                 std::string id) {
        serveFile(req, std::move(cb), id, "inline");
    }

    void getInfo(const drogon::HttpRequestPtr& req,
                 std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                 std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT id,name,size_bytes,mime_type,folder_id,is_starred,is_deleted,created_at,updated_at "
            "FROM files WHERE id=? AND user_id=?",
            [cb](const drogon::orm::Result& r) {
                if (r.empty()) { cb(utils::errorJson(drogon::k404NotFound, "File not found")); return; }
                auto row = r[0];
                Json::Value f;
                f["id"]         = row["id"].as<std::string>();
                f["name"]       = row["name"].as<std::string>();
                f["size_bytes"] = static_cast<Json::Int64>(row["size_bytes"].as<long long>());
                f["mime_type"]  = row["mime_type"].as<std::string>();
                f["is_starred"] = row["is_starred"].as<bool>();
                f["is_deleted"] = row["is_deleted"].as<bool>();
                f["created_at"] = row["created_at"].as<std::string>();
                f["updated_at"] = row["updated_at"].as<std::string>();
                cb(utils::okJson(f));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            }, id, userId);
    }

    void updateFile(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                    std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        auto db = drogon::app().getDbClient();
        auto okCb = [cb](const drogon::orm::Result&) {
            Json::Value ok; ok["message"] = "Updated";
            cb(utils::okJson(ok));
        };
        auto errCb = [cb](const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        };

        if ((*body).isMember("is_starred") && !(*body).isMember("name") && !(*body).isMember("folder_id")) {
            int val = (*body)["is_starred"].asBool() ? 1 : 0;
            db->execSqlAsync(
                "UPDATE files SET is_starred=? WHERE id=? AND user_id=? AND is_deleted=0",
                okCb, errCb, val, id, userId);
            return;
        }
        if ((*body).isMember("name") && !(*body)["name"].asString().empty()) {
            db->execSqlAsync(
                "UPDATE files SET name=? WHERE id=? AND user_id=? AND is_deleted=0",
                okCb, errCb, (*body)["name"].asString(), id, userId);
            return;
        }
        if ((*body).isMember("folder_id")) {
            if ((*body)["folder_id"].isNull()) {
                db->execSqlAsync(
                    "UPDATE files SET folder_id=NULL WHERE id=? AND user_id=? AND is_deleted=0",
                    okCb, errCb, id, userId);
            } else {
                db->execSqlAsync(
                    "UPDATE files SET folder_id=? WHERE id=? AND user_id=? AND is_deleted=0",
                    okCb, errCb, (*body)["folder_id"].asString(), id, userId);
            }
            return;
        }
        cb(utils::errorJson(drogon::k400BadRequest, "Nothing to update"));
    }

    void deleteFile(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                    std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "UPDATE files SET is_deleted=1, deleted_at=NOW() WHERE id=? AND user_id=? AND is_deleted=0",
            [cb](const drogon::orm::Result& r) {
                if (r.affectedRows() == 0) cb(utils::errorJson(drogon::k404NotFound, "File not found"));
                else cb(utils::noContent());
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            }, id, userId);
    }

private:
    // ── Streaming file download ──────────────────────────────────────────
    // Uses Drogon's newStreamResponse + DecryptingReader so memory stays ~4 MB
    // regardless of file size.
    void serveFile(const drogon::HttpRequestPtr& req,
                   std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                   const std::string& id,
                   const std::string& disposition) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT user_id,name,mime_type,size_bytes,"
            "encryption_key_enc,encryption_iv,encryption_tag "
            "FROM files WHERE id=? AND user_id=? AND is_deleted=0",
            [cb, id, disposition](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "File not found"));
                    return;
                }
                auto row     = r[0];
                std::string ownerId  = row["user_id"].as<std::string>();
                std::string fileName = row["name"].as<std::string>();
                std::string mime     = row["mime_type"].as<std::string>();

                services::EncryptedKeyBundle bundle{
                    row["encryption_iv"].as<std::string>(),
                    row["encryption_key_enc"].as<std::string>(),
                    row["encryption_tag"].as<std::string>()
                };

                try {
                    // Create a streaming reader (4 MB memory)
                    auto reader = services::StorageService::createReader(
                        ownerId, id, bundle);

                    // Use Drogon's streaming response — pulls decrypted chunks on demand
                    auto resp = drogon::HttpResponse::newStreamResponse(
                        [reader](char* buf, std::size_t maxLen) -> std::size_t {
                            return reader->read(buf, maxLen);
                        },
                        fileName,
                        drogon::CT_CUSTOM,
                        mime);

                    resp->addHeader("Content-Disposition",
                        disposition + "; filename=\"" + fileName + "\"");
                    cb(resp);
                } catch (const std::exception& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.what()));
                }
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            }, id, userId);
    }
};
