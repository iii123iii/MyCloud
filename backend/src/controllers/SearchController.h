#pragma once
#include <drogon/HttpController.h>
#include "utils/ResponseUtils.h"

class SearchController : public drogon::HttpController<SearchController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(SearchController::search, "/api/search", drogon::Get);
    METHOD_LIST_END

    void search(const drogon::HttpRequestPtr& req,
                std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        std::string userId = req->getAttributes()->get<std::string>("userId");
        std::string q      = req->getParameter("q");
        if (q.empty()) {
            cb(utils::errorJson(drogon::k400BadRequest, "q parameter required"));
            return;
        }
        // Escape SQL LIKE wildcards in user input to prevent wildcard injection
        std::string escaped;
        escaped.reserve(q.size());
        for (char c : q) {
            if (c == '%' || c == '_' || c == '\\') escaped += '\\';
            escaped += c;
        }
        std::string like = "%" + escaped + "%";

        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT 'file' AS type, id, name, size_bytes, mime_type, is_starred, updated_at "
            "FROM files WHERE user_id=? AND is_deleted=0 AND name LIKE ? "
            "UNION ALL "
            "SELECT 'folder' AS type, id, name, 0, '', 0, updated_at "
            "FROM folders WHERE user_id=? AND is_deleted=0 AND name LIKE ? "
            "ORDER BY updated_at DESC LIMIT 100",
            [cb](const drogon::orm::Result& r) {
                Json::Value arr(Json::arrayValue);
                for (const auto& row : r) {
                    Json::Value item;
                    item["type"]       = row["type"].as<std::string>();
                    item["id"]         = row["id"].as<std::string>();
                    item["name"]       = row["name"].as<std::string>();
                    item["updated_at"] = row["updated_at"].as<std::string>();
                    if (row["type"].as<std::string>() == "file") {
                        item["size_bytes"] = static_cast<Json::Int64>(row["size_bytes"].as<long long>());
                        item["mime_type"]  = row["mime_type"].as<std::string>();
                        item["is_starred"] = row["is_starred"].as<bool>();
                    }
                    arr.append(item);
                }
                Json::Value body; body["results"] = arr;
                cb(utils::okJson(body));
            },
            [cb](const drogon::orm::DrogonDbException& e) {
                cb(utils::errorJson(drogon::k500InternalServerError, e.base().what()));
            },
            userId, like, userId, like);
    }
};
