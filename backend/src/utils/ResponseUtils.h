#pragma once
#include <drogon/HttpResponse.h>
#include <json/json.h>
#include <string>

namespace utils {

inline drogon::HttpResponsePtr okJson(const Json::Value& body) {
    auto resp = drogon::HttpResponse::newHttpJsonResponse(body);
    resp->setStatusCode(drogon::k200OK);
    return resp;
}

inline drogon::HttpResponsePtr createdJson(const Json::Value& body) {
    auto resp = drogon::HttpResponse::newHttpJsonResponse(body);
    resp->setStatusCode(drogon::k201Created);
    return resp;
}

inline drogon::HttpResponsePtr errorJson(drogon::HttpStatusCode code,
                                          const std::string& message) {
    Json::Value body;
    body["error"] = message;
    auto resp = drogon::HttpResponse::newHttpJsonResponse(body);
    resp->setStatusCode(code);
    return resp;
}

inline drogon::HttpResponsePtr noContent() {
    auto resp = drogon::HttpResponse::newHttpResponse();
    resp->setStatusCode(drogon::k204NoContent);
    return resp;
}

} // namespace utils
