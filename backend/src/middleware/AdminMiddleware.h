#pragma once
#include <drogon/HttpMiddleware.h>
#include "utils/ResponseUtils.h"
#include <string>

// Must run AFTER AuthMiddleware. Rejects non-admin roles.
class AdminMiddleware : public drogon::HttpMiddleware<AdminMiddleware> {
public:
    AdminMiddleware() = default;

    void invoke(const drogon::HttpRequestPtr& req,
                drogon::MiddlewareNextCallback&& nextCb,
                drogon::FilterCallback&& stopCb) override {
        const auto role = req->getAttributes()->get<std::string>("userRole");
        if (role != "admin") {
            stopCb(utils::errorJson(drogon::k403Forbidden, "Admin access required"));
            return;
        }
        nextCb(std::move(stopCb));
    }
};
