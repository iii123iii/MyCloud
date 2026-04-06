#pragma once
#include <drogon/HttpController.h>
#include "utils/FolderTreeUtils.h"
#include "utils/ResponseUtils.h"
#include "services/StorageService.h"
#include <vector>

class TrashController : public drogon::HttpController<TrashController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(TrashController::listTrash,    "/api/trash",               drogon::Get);
        ADD_METHOD_TO(TrashController::restoreItem,  "/api/trash/{id}/restore",  drogon::Post);
        ADD_METHOD_TO(TrashController::deleteItem,   "/api/trash/{id}",          drogon::Delete);
        ADD_METHOD_TO(TrashController::emptyTrash,   "/api/trash/empty",         drogon::Delete);
    METHOD_LIST_END

    void listTrash(const drogon::HttpRequestPtr& req,
                   std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");

        // Parse pagination
        std::string pageStr = req->getParameter("page");
        std::string sizeStr = req->getParameter("page_size");
        int page     = 1, pageSize = 50;
        if (!pageStr.empty()) try { page = std::max(1, std::stoi(pageStr)); } catch (...) {}
        if (!sizeStr.empty()) try { pageSize = std::max(1, std::min(200, std::stoi(sizeStr))); } catch (...) {}
        int offset = (page - 1) * pageSize;

        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT 'file' AS type, id, name, size_bytes, mime_type, deleted_at FROM files "
            "WHERE user_id=? AND is_deleted=1 "
            "UNION ALL "
            "SELECT 'folder' AS type, id, name, 0, '', deleted_at FROM folders "
            "WHERE user_id=? AND is_deleted=1 "
            "ORDER BY deleted_at DESC "
            "LIMIT " + std::to_string(pageSize) + " OFFSET " + std::to_string(offset),
            [cb](const drogon::orm::Result& r) {
                Json::Value arr(Json::arrayValue);
                for (const auto& row : r) {
                    Json::Value item;
                    item["type"]       = row["type"].as<std::string>();
                    item["id"]         = row["id"].as<std::string>();
                    item["name"]       = row["name"].as<std::string>();
                    item["deleted_at"] = row["deleted_at"].as<std::string>();
                    if (row["type"].as<std::string>() == "file") {
                        item["size_bytes"] = static_cast<Json::Int64>(row["size_bytes"].as<long long>());
                        item["mime_type"]  = row["mime_type"].as<std::string>();
                    }
                    arr.append(item);
                }
                Json::Value body; body["items"] = arr;
                cb(utils::okJson(body));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            userId, userId);
    }

    void restoreItem(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                     std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        // Try files first, then folders
        db->execSqlAsync(
            "UPDATE files SET is_deleted=0, deleted_at=NULL WHERE id=? AND user_id=?",
            [cb, db, id, userId](const drogon::orm::Result& r) {
                if (r.affectedRows() > 0) {
                    Json::Value ok; ok["message"] = "Restored";
                    cb(utils::okJson(ok));
                    return;
                }
                try {
                    auto folderIds = collectFolderTree(db, id, userId, true);
                    if (folderIds.empty()) {
                        cb(utils::errorJson(drogon::k404NotFound, "Item not found in trash"));
                        return;
                    }
                    restoreFolderTree(db, folderIds, userId);

                    Json::Value ok; ok["message"] = "Restored";
                    cb(utils::okJson(ok));
                } catch (const drogon::orm::DrogonDbException& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                }
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            id, userId);
    }

    void deleteItem(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                    std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();

        // Fetch file size BEFORE deleting so we can decrement quota correctly
        db->execSqlAsync(
            "SELECT id, user_id, size_bytes FROM files WHERE id=? AND user_id=? AND is_deleted=1",
            [cb, db, id, userId](const drogon::orm::Result& r) {
                if (!r.empty()) {
                    long long size = r[0]["size_bytes"].as<long long>();
                    // Hard-delete from disk + DB
                    services::StorageService::deleteFile(userId, id);
                    db->execSqlAsync(
                        "DELETE FROM files WHERE id=? AND user_id=?",
                        [cb, db, id, userId, size](const drogon::orm::Result&) {
                            db->execSqlAsync(
                                "UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?",
                                [cb](const drogon::orm::Result&) { cb(utils::noContent()); },
                                [cb](const drogon::orm::DrogonDbException&) { cb(utils::noContent()); },
                                size, userId);
                        },
                        [cb](const drogon::orm::DrogonDbException& e) {
                            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                        },
                        id, userId);
                    return;
                }
                // Try folder
                try {
                    auto folderIds = collectFolderTree(db, id, userId, true);
                    if (folderIds.empty()) {
                        cb(utils::errorJson(drogon::k404NotFound, "Item not found in trash"));
                        return;
                    }

                    auto filesInTree = collectDeletedFilesForFolderTree(db, folderIds, userId);
                    long long totalSize = 0;
                    for (const auto& [fileId, sizeBytes] : filesInTree) {
                        services::StorageService::deleteFile(userId, fileId);
                        totalSize += sizeBytes;
                    }

                    hardDeleteFolderTreeFiles(db, folderIds, userId);
                    hardDeleteFolderTreeFolders(db, folderIds, userId);

                    db->execSqlSync(
                        "UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?",
                        totalSize, userId);

                    cb(utils::noContent());
                } catch (const drogon::orm::DrogonDbException& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                }
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            id, userId);
    }

    void emptyTrash(const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();

        // Fetch all deleted files to remove from disk and reclaim quota
        db->execSqlAsync(
            "SELECT id, size_bytes FROM files WHERE user_id=? AND is_deleted=1",
            [cb, db, userId](const drogon::orm::Result& r) {
                long long totalSize = 0;
                for (const auto& row : r) {
                    services::StorageService::deleteFile(userId, row["id"].as<std::string>());
                    totalSize += row["size_bytes"].as<long long>();
                }

                db->execSqlAsync(
                    "DELETE FROM files WHERE user_id=? AND is_deleted=1",
                    [cb, db, userId, totalSize](const drogon::orm::Result&) {
                        // Reclaim storage quota
                        db->execSqlAsync(
                            "UPDATE users SET used_bytes=GREATEST(0, used_bytes-?) WHERE id=?",
                            [cb, db, userId](const drogon::orm::Result&) {
                                db->execSqlAsync(
                                    "DELETE FROM folders WHERE user_id=? AND is_deleted=1",
                                    [cb](const drogon::orm::Result&) { cb(utils::noContent()); },
                                    [cb](const drogon::orm::DrogonDbException& e) {
                                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                                    },
                                    userId);
                            },
                            [cb](const drogon::orm::DrogonDbException&) {
                                // Still proceed even if quota update fails
                                cb(utils::noContent());
                            },
                            totalSize, userId);
                    },
                    [cb](const drogon::orm::DrogonDbException& e) {
                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                    },
                    userId);
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            userId);
    }
};
