#pragma once
#include <drogon/HttpController.h>
#include "utils/FolderTreeUtils.h"
#include "utils/ResponseUtils.h"
#include "utils/UuidUtils.h"
#include <vector>

class FolderController : public drogon::HttpController<FolderController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(FolderController::listFolders,  "/api/folders",       drogon::Get);
        ADD_METHOD_TO(FolderController::createFolder, "/api/folders",       drogon::Post);
        ADD_METHOD_TO(FolderController::getFolder,    "/api/folders/{id}",  drogon::Get);
        ADD_METHOD_TO(FolderController::updateFolder, "/api/folders/{id}",  drogon::Patch);
        ADD_METHOD_TO(FolderController::deleteFolder, "/api/folders/{id}",  drogon::Delete);
    METHOD_LIST_END

    void listFolders(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId   = req->getAttributes()->get<std::string>("userId");
        std::string parentId = req->getParameter("parent_id");

        auto db  = drogon::app().getDbClient();
        std::string where = "FROM folders WHERE user_id=? AND is_deleted=0";
        if (parentId.empty())
            where += " AND parent_id IS NULL";
        else
            where += " AND parent_id=?";

        std::string countSql = "SELECT COUNT(*) AS total " + where;
        std::string dataSql  = "SELECT id,name,parent_id,created_at,updated_at "
                               + where + " ORDER BY name ASC";

        auto resultCb = [cb](const drogon::orm::Result& countR,
                             const drogon::orm::Result& dataR) {
            Json::Value arr(Json::arrayValue);
            for (const auto& row : dataR) {
                Json::Value f;
                f["id"]         = row["id"].as<std::string>();
                f["name"]       = row["name"].as<std::string>();
                f["created_at"] = row["created_at"].as<std::string>();
                if (!row["updated_at"].isNull())
                    f["updated_at"] = row["updated_at"].as<std::string>();
                if (!row["parent_id"].isNull())
                    f["parent_id"] = row["parent_id"].as<std::string>();
                arr.append(f);
            }
            Json::Value body;
            body["folders"] = arr;
            body["total"]   = static_cast<Json::Int64>(countR[0]["total"].as<long long>());
            cb(utils::okJson(body));
        };
        auto errCb = [cb](const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        };

        if (parentId.empty()) {
            db->execSqlAsync(countSql,
                [cb, db, dataSql, userId, resultCb, errCb](const drogon::orm::Result& countR) {
                    db->execSqlAsync(dataSql,
                        [countR, resultCb](const drogon::orm::Result& dataR) { resultCb(countR, dataR); },
                        errCb, userId);
                }, errCb, userId);
        } else {
            db->execSqlAsync(countSql,
                [cb, db, dataSql, userId, parentId, resultCb, errCb](const drogon::orm::Result& countR) {
                    db->execSqlAsync(dataSql,
                        [countR, resultCb](const drogon::orm::Result& dataR) { resultCb(countR, dataR); },
                        errCb, userId, parentId);
                }, errCb, userId, parentId);
        }
    }

    // GET /api/folders/:id
    void getFolder(const drogon::HttpRequestPtr& req,
                   std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                   std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT id,name,parent_id,created_at,updated_at "
            "FROM folders WHERE id=? AND user_id=? AND is_deleted=0",
            [cb](const drogon::orm::Result& r) {
                if (r.empty()) {
                    cb(utils::errorJson(drogon::k404NotFound, "Folder not found"));
                    return;
                }
                auto row = r[0];
                Json::Value f;
                f["id"]         = row["id"].as<std::string>();
                f["name"]       = row["name"].as<std::string>();
                f["created_at"] = row["created_at"].as<std::string>();
                if (!row["updated_at"].isNull())
                    f["updated_at"] = row["updated_at"].as<std::string>();
                if (!row["parent_id"].isNull())
                    f["parent_id"] = row["parent_id"].as<std::string>();
                cb(utils::okJson(f));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            id, userId);
    }

    void createFolder(const drogon::HttpRequestPtr& req,
                      std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        std::string name     = (*body)["name"].asString();
        std::string parentId = (*body)["parent_id"].asString();
        if (name.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "name is required"));
            return;
        }

        std::string id = utils::generateUuid();
        auto db = drogon::app().getDbClient();

        auto okCb = [cb, id, name](const drogon::orm::Result&) {
            Json::Value out;
            out["id"]   = id;
            out["name"] = name;
            cb(utils::createdJson(out));
        };
        auto errCb = [cb](const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        };

        if (parentId.empty()) {
            db->execSqlAsync(
                "INSERT INTO folders (id,name,user_id,parent_id) VALUES (?,?,?,NULL)",
                okCb, errCb, id, name, userId);
        } else {
            db->execSqlAsync(
                "INSERT INTO folders (id,name,user_id,parent_id) VALUES (?,?,?,?)",
                okCb, errCb, id, name, userId, parentId);
        }
    }

    void updateFolder(const drogon::HttpRequestPtr& req,
                      std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                      std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto body = req->getJsonObject();
        if (!body) { cb(utils::errorJson(drogon::k400BadRequest, "Invalid JSON")); return; }

        auto db = drogon::app().getDbClient();

        // Rename
        if ((*body).isMember("name")) {
            std::string name = (*body)["name"].asString();
            if (name.empty()) {
                cb(utils::errorJson(drogon::k400BadRequest, "name cannot be empty"));
                return;
            }
            db->execSqlAsync(
                "UPDATE folders SET name=? WHERE id=? AND user_id=? AND is_deleted=0",
                [cb](const drogon::orm::Result& r) {
                    if (r.affectedRows() == 0)
                        cb(utils::errorJson(drogon::k404NotFound, "Folder not found"));
                    else { Json::Value ok; ok["message"] = "Updated"; cb(utils::okJson(ok)); }
                },
                [cb](const drogon::orm::DrogonDbException& e) {
                    cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                },
                name, id, userId);
            return;
        }

        // Move (update parent_id)
        if ((*body).isMember("parent_id")) {
            if ((*body)["parent_id"].isNull()) {
                db->execSqlAsync(
                    "UPDATE folders SET parent_id=NULL WHERE id=? AND user_id=? AND is_deleted=0",
                    [cb](const drogon::orm::Result& r) {
                        if (r.affectedRows() == 0)
                            cb(utils::errorJson(drogon::k404NotFound, "Folder not found"));
                        else { Json::Value ok; ok["message"] = "Updated"; cb(utils::okJson(ok)); }
                    },
                    [cb](const drogon::orm::DrogonDbException& e) {
                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                    },
                    id, userId);
            } else {
                std::string pid = (*body)["parent_id"].asString();
                if (pid == id) {
                    cb(utils::errorJson(drogon::k400BadRequest, "Cannot move folder into itself"));
                    return;
                }
                // Fully async cycle-check — no blocking calls on the event loop
                db->execSqlAsync(
                    "SELECT id FROM folders WHERE id=? AND user_id=? AND is_deleted=0",
                    [cb, db, id, userId, pid](const drogon::orm::Result& targetParent) {
                        if (targetParent.empty()) {
                            cb(utils::errorJson(drogon::k404NotFound, "Destination folder not found"));
                            return;
                        }
                        db->execSqlAsync(
                            "WITH RECURSIVE folder_tree AS ("
                            " SELECT id FROM folders WHERE id=? AND user_id=? AND is_deleted=0"
                            " UNION ALL "
                            " SELECT f.id FROM folders f "
                            " INNER JOIN folder_tree ft ON f.parent_id=ft.id "
                            " WHERE f.user_id=? AND f.is_deleted=0"
                            ") "
                            "SELECT id FROM folder_tree WHERE id=? LIMIT 1",
                            [cb, db, id, userId, pid](const drogon::orm::Result& cycleCheck) {
                                if (!cycleCheck.empty()) {
                                    cb(utils::errorJson(drogon::k400BadRequest,
                                       "Cannot move folder into one of its descendants"));
                                    return;
                                }
                                db->execSqlAsync(
                                    "UPDATE folders SET parent_id=? WHERE id=? AND user_id=? AND is_deleted=0",
                                    [cb](const drogon::orm::Result& r) {
                                        if (r.affectedRows() == 0)
                                            cb(utils::errorJson(drogon::k404NotFound, "Folder not found"));
                                        else { Json::Value ok; ok["message"] = "Updated"; cb(utils::okJson(ok)); }
                                    },
                                    [cb](const drogon::orm::DrogonDbException& e) {
                                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                                    },
                                    pid, id, userId);
                            },
                            [cb](const drogon::orm::DrogonDbException& e) {
                                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                            },
                            id, userId, userId, pid);
                    },
                    [cb](const drogon::orm::DrogonDbException& e) {
                        cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
                    },
                    pid, userId);
            }
            return;
        }

        cb(utils::errorJson(drogon::k400BadRequest, "Nothing to update"));
    }

    void deleteFolder(const drogon::HttpRequestPtr& req,
                      std::function<void(const drogon::HttpResponsePtr&)>&& cb,
                      std::string id) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        auto db = drogon::app().getDbClient();
        try {
            auto folderIds = collectFolderTree(db, id, userId);
            if (folderIds.empty()) {
                cb(utils::errorJson(drogon::k404NotFound, "Folder not found"));
                return;
            }
            softDeleteFolderTree(db, folderIds, userId);

            cb(utils::noContent());
        } catch (const drogon::orm::DrogonDbException& e) {
            cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
        }
    }
};
