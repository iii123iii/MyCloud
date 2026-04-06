#pragma once
#include <algorithm>
#include <string>
#include <utility>
#include <vector>
#include <drogon/orm/DbClient.h>

inline std::vector<std::string> collectFolderTree(
    const drogon::orm::DbClientPtr& db,
    const std::string& rootId,
    const std::string& userId,
    bool onlyDeleted = false) {
    std::string sql =
        "WITH RECURSIVE folder_tree AS ("
        " SELECT id FROM folders WHERE id=? AND user_id=? ";
    sql += onlyDeleted ? "AND is_deleted=1" : "AND is_deleted=0";
    sql +=
        " UNION ALL "
        " SELECT f.id FROM folders f "
        " INNER JOIN folder_tree ft ON f.parent_id=ft.id "
        " WHERE f.user_id=? ";
    sql += onlyDeleted ? "AND f.is_deleted=1" : "AND f.is_deleted=0";
    sql +=
        ") "
        "SELECT id FROM folder_tree";

    std::vector<std::string> ids;
    auto result = db->execSqlSync(sql, rootId, userId, userId);
    ids.reserve(result.size());
    for (const auto& row : result) ids.push_back(row["id"].as<std::string>());
    return ids;
}

inline void softDeleteFolderTree(
    const drogon::orm::DbClientPtr& db,
    const std::vector<std::string>& folderIds,
    const std::string& userId) {
    if (folderIds.empty()) return;

    for (const auto& folderId : folderIds) {
        db->execSqlSync(
            "UPDATE folders SET is_deleted=1, deleted_at=NOW() "
            "WHERE id=? AND user_id=? AND is_deleted=0",
            folderId, userId);
        db->execSqlSync(
            "UPDATE files SET is_deleted=1, deleted_at=NOW() "
            "WHERE user_id=? AND is_deleted=0 AND folder_id=?",
            userId, folderId);
    }
}

inline void restoreFolderTree(
    const drogon::orm::DbClientPtr& db,
    const std::vector<std::string>& folderIds,
    const std::string& userId) {
    if (folderIds.empty()) return;

    for (const auto& folderId : folderIds) {
        db->execSqlSync(
            "UPDATE folders SET is_deleted=0, deleted_at=NULL "
            "WHERE id=? AND user_id=? AND is_deleted=1",
            folderId, userId);
        db->execSqlSync(
            "UPDATE files SET is_deleted=0, deleted_at=NULL "
            "WHERE user_id=? AND is_deleted=1 AND folder_id=?",
            userId, folderId);
    }
}

inline std::vector<std::pair<std::string, long long>> collectDeletedFilesForFolderTree(
    const drogon::orm::DbClientPtr& db,
    const std::vector<std::string>& folderIds,
    const std::string& userId) {
    std::vector<std::pair<std::string, long long>> files;
    if (folderIds.empty()) return files;

    for (const auto& folderId : folderIds) {
        auto result = db->execSqlSync(
            "SELECT id, size_bytes FROM files "
            "WHERE user_id=? AND is_deleted=1 AND folder_id=?",
            userId, folderId);
        for (const auto& row : result) {
            files.emplace_back(
                row["id"].as<std::string>(),
                row["size_bytes"].as<long long>());
        }
    }
    return files;
}

inline void hardDeleteFolderTreeFiles(
    const drogon::orm::DbClientPtr& db,
    const std::vector<std::string>& folderIds,
    const std::string& userId) {
    if (folderIds.empty()) return;

    for (const auto& folderId : folderIds) {
        db->execSqlSync(
            "DELETE FROM files WHERE user_id=? AND is_deleted=1 AND folder_id=?",
            userId, folderId);
    }
}

inline void hardDeleteFolderTreeFolders(
    const drogon::orm::DbClientPtr& db,
    const std::vector<std::string>& folderIds,
    const std::string& userId) {
    if (folderIds.empty()) return;

    // Reverse order: children before parents to satisfy FK constraints
    for (auto it = folderIds.rbegin(); it != folderIds.rend(); ++it) {
        db->execSqlSync(
            "DELETE FROM folders WHERE id=? AND user_id=? AND is_deleted=1",
            *it, userId);
    }
}
