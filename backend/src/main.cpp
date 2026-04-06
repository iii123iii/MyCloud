#include <drogon/drogon.h>
#include <drogon/orm/DbConfig.h>
#include <cstdlib>
#include <string>
#include <fstream>
#include <iostream>
#include <thread>

// Controllers (header-only Drogon style)
#include "controllers/SetupController.h"
#include "controllers/AuthController.h"
#include "controllers/FileController.h"
#include "controllers/FolderController.h"
#include "controllers/ShareController.h"
#include "controllers/TrashController.h"
#include "controllers/SearchController.h"
#include "controllers/AdminController.h"
#include "controllers/UpdateController.h"

#include "utils/JwtUtils.h"
#include "utils/ResponseUtils.h"

int main() {
    // ── Read env var, or fall back to reading from a *_FILE path ─────────────
    auto envOrFile = [](const char* name, const char* def = "") -> std::string {
        const char* v = std::getenv(name);
        if (v && v[0] != '\0') return std::string(v);
        std::string fileKey = std::string(name) + "_FILE";
        const char* fp = std::getenv(fileKey.c_str());
        if (fp) {
            std::ifstream f(fp);
            if (f) {
                std::string s;
                std::getline(f, s);
                if (!s.empty()) return s;
            }
        }
        return std::string(def);
    };

    const std::string dbHost    = envOrFile("DB_HOST",   "mariadb");
    const std::string dbPort    = envOrFile("DB_PORT",   "3306");
    const std::string dbName    = envOrFile("DB_NAME",   "mycloud");
    const std::string dbUser    = envOrFile("DB_USER",   "mycloud");
    const std::string dbPass    = envOrFile("DB_PASSWORD", "");
    const std::string jwtSecret = envOrFile("JWT_SECRET", "");
    const std::string origins   = envOrFile("ALLOWED_ORIGINS", "https://localhost");

    // ── Database client ───────────────────────────────────────────────────────
    // DbConfig is a std::variant<PostgresConfig, MysqlConfig, Sqlite3Config>
    drogon::orm::MysqlConfig mysqlCfg;
    mysqlCfg.host         = dbHost;
    mysqlCfg.port         = static_cast<unsigned short>(std::stoi(dbPort));
    mysqlCfg.databaseName = dbName;
    mysqlCfg.username         = dbUser;
    mysqlCfg.password         = dbPass;
    // IMPORTANT: connectionNumber is per IO thread, not total.
    // setThreadNum(0) spawns hardware_concurrency() threads, so:
    //   total DB connections = connectionNumber × thread_count
    // We target ~80 total, staying well under MariaDB's max_connections (200).
    // This auto-scales: 8 cores → 10/thread, 16 cores → 5/thread, etc.
    mysqlCfg.connectionNumber = std::max(2u, 80u / std::max(1u, std::thread::hardware_concurrency()));
    mysqlCfg.characterSet  = "utf8mb4";
    mysqlCfg.timeout       = 60.0;
    mysqlCfg.name          = "default";
    drogon::app().addDbClient(drogon::orm::DbConfig{mysqlCfg});

    // ── CORS pre-handling advice (OPTIONS preflight) ────────────────────────────
    drogon::app().registerPreHandlingAdvice(
        [origins](const drogon::HttpRequestPtr& req,
                  std::function<void(const drogon::HttpResponsePtr&)>&& stop,
                  std::function<void()>&& next) {
            if (req->method() == drogon::Options) {
                auto resp = drogon::HttpResponse::newHttpResponse();
                resp->setStatusCode(drogon::k204NoContent);
                resp->addHeader("Access-Control-Allow-Origin",  origins);
                resp->addHeader("Access-Control-Allow-Methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS");
                resp->addHeader("Access-Control-Allow-Headers",
                                "Authorization,Content-Type,X-Share-Password");
                resp->addHeader("Access-Control-Max-Age", "86400");
                stop(resp);
                return;
            }
            next();
        });

    // ── CORS post-handling advice (add origin header to ALL responses) ────────
    drogon::app().registerPostHandlingAdvice(
        [origins](const drogon::HttpRequestPtr& req,
                  const drogon::HttpResponsePtr& resp) {
            resp->addHeader("Access-Control-Allow-Origin", origins);
        });

    // ── Auth pre-handling advice ──────────────────────────────────────────────
    // Routes that start with these prefixes require a valid Bearer JWT.
    // Routes starting with /api/admin also require role == "admin".
    drogon::app().registerPreHandlingAdvice(
        [jwtSecret](const drogon::HttpRequestPtr& req,
                    std::function<void(const drogon::HttpResponsePtr&)>&& stop,
                    std::function<void()>&& next) {
            const auto& path = req->path();

            // Public routes — skip auth
            auto isPublic = [&]() {
                if (path == "/api/setup/status")   return true;
                if (path == "/api/setup/complete") return true;
                if (path == "/api/auth/login")     return true;
                if (path == "/api/auth/register")  return true;
                if (path == "/api/auth/refresh")   return true;
                if (path == "/api/auth/logout")    return true;
                if (path.rfind("/api/s/", 0) == 0) return true;  // public shares
                return false;
            };

            if (!path.starts_with("/api/") || isPublic()) {
                next();
                return;
            }

            // Verify JWT
            const auto auth = req->getHeader("Authorization");
            if (auth.rfind("Bearer ", 0) != 0) {
                stop(utils::errorJson(drogon::k401Unauthorized,
                     "Missing or invalid Authorization header"));
                return;
            }
            try {
                auto claims = utils::verifyToken(auth.substr(7), jwtSecret);
                if (claims.type != "access") {
                    stop(utils::errorJson(drogon::k401Unauthorized, "Expected access token"));
                    return;
                }
                req->getAttributes()->insert("userId",   claims.userId);
                req->getAttributes()->insert("userRole", claims.role);

                // Admin-only routes
                if (path.starts_with("/api/admin/") && claims.role != "admin") {
                    stop(utils::errorJson(drogon::k403Forbidden, "Admin access required"));
                    return;
                }
                next();
            } catch (const std::exception& e) {
                stop(utils::errorJson(drogon::k401Unauthorized,
                     std::string("Invalid token: ") + e.what()));
            }
        });

    // ── App startup ───────────────────────────────────────────────────────────
    drogon::app()
        .setLogLevel(trantor::Logger::kInfo)
        .addListener("0.0.0.0", 8080)
        .setThreadNum(0)
        .setMaxConnectionNum(100000)
        .setClientMaxBodySize(10ULL * 1024 * 1024 * 1024)
        .run();

    return 0;
}
