#pragma once
#include <drogon/drogon.h>
#include <filesystem>
#include <system_error>
#include <string>
#include <functional>
#include <utility>
#include "StorageService.h"

namespace fs = std::filesystem;

namespace services {

class UserDeletionService {
public:
    static void deleteUser(
        const std::string& userId,
        std::function<void()>&& onSuccess,
        std::function<void(drogon::HttpStatusCode, const std::string&)>&& onError) {
        auto db = drogon::app().getDbClient();
        db->execSqlAsync(
            "SELECT id FROM users WHERE id=? LIMIT 1",
            [db, userId, onSuccess = std::move(onSuccess), onError = std::move(onError)]
            (const drogon::orm::Result& r) mutable {
                if (r.empty()) {
                    onError(drogon::k404NotFound, "User not found");
                    return;
                }

                std::error_code ec;
                fs::remove_all(fs::path(StorageService::storagePath()) / userId, ec);
                if (ec) {
                    onError(drogon::k500InternalServerError,
                            "Failed to delete user storage: " + ec.message());
                    return;
                }

                db->execSqlAsync(
                    "DELETE FROM users WHERE id=?",
                    [onSuccess = std::move(onSuccess)](const drogon::orm::Result&) mutable {
                        onSuccess();
                    },
                    [onError = std::move(onError)](const drogon::orm::DrogonDbException& e) mutable {
                        onError(drogon::k500InternalServerError, e.base().what());
                    },
                    userId);
            },
            [onError = std::move(onError)](const drogon::orm::DrogonDbException& e) mutable {
                onError(drogon::k500InternalServerError, e.base().what());
            },
            userId);
    }
};

} // namespace services
