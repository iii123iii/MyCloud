#pragma once

#include <algorithm>
#include <atomic>
#include <cerrno>
#include <cctype>
#include <cstdlib>
#include <cstring>
#include <drogon/HttpClient.h>
#include <drogon/HttpController.h>
#include <fcntl.h>
#include <json/json.h>
#include <memory>
#include <mutex>
#include <optional>
#include <sstream>
#include <string>
#include <sys/wait.h>
#include <thread>
#include <unistd.h>
#include <utility>
#include <vector>

#include "utils/ResponseUtils.h"

class UpdateController : public drogon::HttpController<UpdateController> {
public:
    METHOD_LIST_BEGIN
        ADD_METHOD_TO(UpdateController::checkUpdate, "/api/admin/updates/check", drogon::Get);
        ADD_METHOD_TO(UpdateController::applyUpdate, "/api/admin/updates/apply", drogon::Post);
    METHOD_LIST_END

    void checkUpdate(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        (void)req;
        auto responseCb =
            std::make_shared<std::function<void(const drogon::HttpResponsePtr&)>>(std::move(cb));

        fetchLatestRelease(
            [responseCb](const ReleaseInfo& release) mutable {
                const ApplyConfig applyConfig = getApplyConfig();
                if (applyConfig.useRemoteUpdater) {
                    fetchRemoteUpdaterState(
                        applyConfig.updaterUrl,
                        [responseCb, release, applyConfig](const StateSnapshot& state) mutable {
                            Json::Value out;
                            fillResponse(out, release, applyConfig, state);
                            (*responseCb)(utils::okJson(out));
                        },
                        [responseCb, release, applyConfig](const std::string& errorMessage) mutable {
                            StateSnapshot state;
                            state.status = "unknown";
                            state.message = errorMessage;
                            state.logPath = applyConfig.logPath;

                            Json::Value out;
                            fillResponse(out, release, applyConfig, state);
                            (*responseCb)(utils::okJson(out));
                        });
                    return;
                }

                Json::Value out;
                fillResponse(out, release, applyConfig, snapshotState());
                (*responseCb)(utils::okJson(out));
            },
            [responseCb](const drogon::HttpResponsePtr& errorResp) mutable {
                (*responseCb)(errorResp);
            });
    }

    void applyUpdate(const drogon::HttpRequestPtr& req,
                     std::function<void(const drogon::HttpResponsePtr&)>&& cb) {
        (void)req;
        auto responseCb =
            std::make_shared<std::function<void(const drogon::HttpResponsePtr&)>>(std::move(cb));

        const ApplyConfig applyConfig = getApplyConfig();
        if (!applyConfig.supported) {
            (*responseCb)(utils::errorJson(drogon::k501NotImplemented, applyConfig.message));
            return;
        }

        bool expected = false;
        if (!updateInProgress_.compare_exchange_strong(expected, true)) {
            (*responseCb)(
                utils::errorJson(drogon::k409Conflict, "An update command is already running."));
            return;
        }

        fetchLatestRelease(
            [responseCb, applyConfig](const ReleaseInfo& release) mutable {
                const std::string current = MYCLOUD_VERSION;
                if (!isVersionNewer(release.latest, current)) {
                    updateInProgress_.store(false);
                    setState("idle",
                             "No newer GitHub release is available for this server.",
                             applyConfig.logPath,
                             "",
                             false);
                    (*responseCb)(utils::errorJson(
                        drogon::k409Conflict,
                        "No newer GitHub release is available for this server."));
                    return;
                }

                setState("running",
                         "Applying " + release.latest + ". Logs are being written to " +
                             applyConfig.logPath + ".",
                         applyConfig.logPath,
                         release.latest,
                         true);

                if (applyConfig.useRemoteUpdater) {
                    triggerRemoteUpdate(
                        applyConfig.updaterUrl,
                        release.latest,
                        current,
                        [responseCb, applyConfig](const std::string& message,
                                                  const std::string& logPath) mutable {
                            updateInProgress_.store(false);
                            Json::Value out;
                            out["message"] = message;
                            out["log_path"] = logPath.empty() ? applyConfig.logPath : logPath;

                            auto resp = drogon::HttpResponse::newHttpJsonResponse(out);
                            resp->setStatusCode(drogon::k202Accepted);
                            (*responseCb)(resp);
                        },
                        [responseCb, applyConfig, release](const drogon::HttpResponsePtr& errorResp,
                                                           const std::string& message) mutable {
                            updateInProgress_.store(false);
                            setState("failed", message, applyConfig.logPath, release.latest, false);
                            (*responseCb)(errorResp);
                        });
                    return;
                }

                const std::string launchError = launchDetachedCommand(
                    applyConfig.command,
                    applyConfig.logPath,
                    {{"MYCLOUD_UPDATE_TARGET_VERSION", release.latest},
                     {"MYCLOUD_UPDATE_CURRENT_VERSION", current},
                     {"MYCLOUD_UPDATE_RELEASE_URL", release.releaseUrl}});
                if (!launchError.empty()) {
                    updateInProgress_.store(false);
                    setState("failed", launchError, applyConfig.logPath, release.latest, false);
                    (*responseCb)(
                        utils::errorJson(drogon::k500InternalServerError, launchError));
                    return;
                }

                Json::Value out;
                out["message"] = "Update started. Watch " + applyConfig.logPath +
                                 " if the server does not come back.";
                out["log_path"] = applyConfig.logPath;

                auto resp = drogon::HttpResponse::newHttpJsonResponse(out);
                resp->setStatusCode(drogon::k202Accepted);
                (*responseCb)(resp);
            },
            [responseCb, applyConfig](const drogon::HttpResponsePtr& errorResp) mutable {
                updateInProgress_.store(false);
                setState("failed",
                         "Could not verify the latest GitHub release before applying the update.",
                         applyConfig.logPath,
                         "",
                         false);
                (*responseCb)(errorResp);
            });
    }

private:
    struct PreReleaseIdentifier {
        bool numeric = false;
        std::string value;
    };

    struct ParsedVersion {
        int major = 0;
        int minor = 0;
        int patch = 0;
        std::vector<PreReleaseIdentifier> prerelease;
    };

    struct ApplyConfig {
        bool supported = false;
        std::string command;
        std::string message;
        std::string logPath;
        std::string updaterUrl;
        bool useRemoteUpdater = false;
    };

    struct ReleaseInfo {
        std::string latest;
        std::string releaseUrl;
        std::string releaseName;
        std::string publishedAt;
        std::string releaseNotes;
    };

    struct StateSnapshot {
        StateSnapshot()
            : inProgress(false), status("idle") {}

        bool inProgress;
        std::string message;
        std::string status;
        std::string logPath;
        std::string targetVersion;
    };

    static inline std::atomic<bool> updateInProgress_{false};
    static inline std::mutex stateMutex_;
    static inline StateSnapshot state_;

    static std::string envOrDefault(const char* name, const char* fallback = "") {
        const char* value = std::getenv(name);
        return (value && value[0] != '\0') ? std::string(value) : std::string(fallback);
    }

    static void fillResponse(Json::Value& out,
                             const ReleaseInfo& release,
                             const ApplyConfig& applyConfig,
                             const StateSnapshot& state) {
        const std::string current = MYCLOUD_VERSION;
        const bool updateAvailable = isVersionNewer(release.latest, current);

        out["current"] = current;
        out["latest"] = release.latest;
        out["update_available"] = updateAvailable;
        out["release_url"] = release.releaseUrl;
        out["release_name"] = release.releaseName;
        out["published_at"] = release.publishedAt;
        out["release_notes"] = release.releaseNotes;
        out["apply_supported"] = applyConfig.supported;
        out["apply_message"] = applyConfig.message;
        out["update_in_progress"] = state.inProgress;
        out["update_status"] = state.status;
        out["update_status_message"] = state.message;
        out["update_log_path"] = state.logPath;
        out["last_started_target"] = state.targetVersion;
    }

    // Returns true only for absolute paths confined to safe directories,
    // with no path-traversal components.
    static bool isValidLogPath(const std::string& path) {
        if (path.empty() || path[0] != '/') {
            return false;
        }
        // Reject any path traversal segment.
        if (path.find("..") != std::string::npos) {
            return false;
        }
        // Must be under one of the known-safe directories.
        const std::string allowedPrefixes[] = {"/tmp/", "/data/"};
        for (const auto& prefix : allowedPrefixes) {
            if (path.size() > prefix.size() &&
                path.compare(0, prefix.size(), prefix) == 0) {
                return true;
            }
        }
        return false;
    }

    static void setState(const std::string& status,
                         const std::string& message,
                         const std::string& logPath,
                         const std::string& targetVersion,
                         bool inProgress) {
        std::lock_guard<std::mutex> lock(stateMutex_);
        state_.status = status;
        state_.message = message;
        state_.logPath = logPath;
        state_.targetVersion = targetVersion;
        state_.inProgress = inProgress;
    }

    static StateSnapshot snapshotState() {
        std::lock_guard<std::mutex> lock(stateMutex_);
        return state_;
    }

    static std::optional<StateSnapshot> parseRemoteState(const Json::Value& body) {
        if (!body.isObject()) {
            return std::nullopt;
        }

        StateSnapshot state;
        if (body.isMember("in_progress")) {
            state.inProgress = body["in_progress"].asBool();
        }
        if (body.isMember("message")) {
            state.message = body["message"].asString();
        }
        if (body.isMember("status")) {
            state.status = body["status"].asString();
        }
        if (body.isMember("log_path")) {
            state.logPath = body["log_path"].asString();
        }
        if (body.isMember("target_version")) {
            state.targetVersion = body["target_version"].asString();
        }
        return state;
    }

    static void fetchLatestRelease(
        std::function<void(const ReleaseInfo&)>&& onSuccess,
        std::function<void(const drogon::HttpResponsePtr&)>&& onError) {
        auto client = drogon::HttpClient::newHttpClient("https://api.github.com");
        auto ghReq = drogon::HttpRequest::newHttpRequest();
        ghReq->setPath("/repos/" MYCLOUD_GITHUB_REPO "/releases/latest");
        ghReq->setMethod(drogon::Get);
        ghReq->addHeader("User-Agent", "MyCloud-Server/" MYCLOUD_VERSION);
        ghReq->addHeader("Accept", "application/vnd.github+json");

        client->sendRequest(
            ghReq,
            [onSuccess = std::move(onSuccess),
             onError = std::move(onError)](drogon::ReqResult result,
                                           const drogon::HttpResponsePtr& resp) mutable {
                if (result != drogon::ReqResult::Ok || !resp) {
                    onError(utils::errorJson(
                        drogon::k500InternalServerError,
                        "Could not reach GitHub API. Check server internet access."));
                    return;
                }

                if (resp->statusCode() != drogon::k200OK) {
                    onError(utils::errorJson(
                        drogon::k502BadGateway,
                        "GitHub API returned HTTP " +
                            std::to_string(static_cast<int>(resp->statusCode())) + "."));
                    return;
                }

                auto body = resp->getJsonObject();
                if (!body || !body->isMember("tag_name")) {
                    onError(utils::errorJson(
                        drogon::k500InternalServerError,
                        "Unexpected response from GitHub API."));
                    return;
                }

                ReleaseInfo release;
                release.latest = (*body)["tag_name"].asString();
                release.releaseUrl = (*body)["html_url"].asString();
                release.releaseName = (*body)["name"].asString();
                release.publishedAt = (*body)["published_at"].asString();
                release.releaseNotes = (*body)["body"].asString();
                if (release.releaseNotes.size() > 800) {
                    release.releaseNotes = release.releaseNotes.substr(0, 800) + "...";
                }

                onSuccess(release);
            },
            10.0);
    }

    static void fetchRemoteUpdaterState(
        const std::string& updaterUrl,
        std::function<void(const StateSnapshot&)>&& onSuccess,
        std::function<void(const std::string&)>&& onError) {
        auto client = drogon::HttpClient::newHttpClient(updaterUrl);
        auto req = drogon::HttpRequest::newHttpRequest();
        req->setPath("/status");
        req->setMethod(drogon::Get);

        client->sendRequest(
            req,
            [onSuccess = std::move(onSuccess),
             onError = std::move(onError)](drogon::ReqResult result,
                                           const drogon::HttpResponsePtr& resp) mutable {
                if (result != drogon::ReqResult::Ok || !resp) {
                    onError("Updater container is configured, but /status is unreachable.");
                    return;
                }
                if (resp->statusCode() != drogon::k200OK) {
                    onError("Updater container returned HTTP " +
                            std::to_string(static_cast<int>(resp->statusCode())) + ".");
                    return;
                }
                auto body = resp->getJsonObject();
                if (!body) {
                    onError("Updater container returned malformed JSON.");
                    return;
                }
                const auto state = parseRemoteState(*body);
                if (!state) {
                    onError("Updater container returned an unexpected status payload.");
                    return;
                }
                onSuccess(*state);
            },
            5.0);
    }

    static void triggerRemoteUpdate(
        const std::string& updaterUrl,
        const std::string& targetVersion,
        const std::string& currentVersion,
        std::function<void(const std::string&, const std::string&)>&& onSuccess,
        std::function<void(const drogon::HttpResponsePtr&, const std::string&)>&& onError) {
        auto client = drogon::HttpClient::newHttpClient(updaterUrl);
        auto req = drogon::HttpRequest::newHttpRequest();
        req->setPath("/update");
        req->setMethod(drogon::Post);
        req->setContentTypeCode(drogon::CT_APPLICATION_JSON);

        Json::Value body;
        body["target_version"] = targetVersion;
        body["current_version"] = currentVersion;
        req->setBody(body.toStyledString());

        client->sendRequest(
            req,
            [onSuccess = std::move(onSuccess),
             onError = std::move(onError)](drogon::ReqResult result,
                                           const drogon::HttpResponsePtr& resp) mutable {
                if (result != drogon::ReqResult::Ok || !resp) {
                    onError(
                        utils::errorJson(drogon::k502BadGateway,
                                         "Could not reach the updater container."),
                        "Could not reach the updater container.");
                    return;
                }

                auto payload = resp->getJsonObject();
                if (resp->statusCode() != drogon::k202Accepted) {
                    std::string message = "Updater container rejected the request.";
                    if (payload && payload->isMember("error")) {
                        message = (*payload)["error"].asString();
                    }
                    onError(utils::errorJson(drogon::k502BadGateway, message), message);
                    return;
                }

                std::string message = "Update started.";
                std::string logPath;
                if (payload) {
                    if (payload->isMember("message")) {
                        message = (*payload)["message"].asString();
                    }
                    if (payload->isMember("log_path")) {
                        logPath = (*payload)["log_path"].asString();
                    }
                }
                onSuccess(message, logPath);
            },
            10.0);
    }

    static ApplyConfig getApplyConfig() {
        ApplyConfig config;
        config.logPath = envOrDefault("MYCLOUD_UPDATE_LOG_PATH", "/tmp/mycloud-update.log");
        if (!isValidLogPath(config.logPath)) {
            config.message =
                "MYCLOUD_UPDATE_LOG_PATH must be an absolute path under /tmp/ or /data/ "
                "with no path-traversal components.";
            return config;
        }

        config.updaterUrl = envOrDefault("MYCLOUD_UPDATER_URL");
        if (!config.updaterUrl.empty()) {
            config.supported = true;
            config.useRemoteUpdater = true;
            config.message =
                "One-click apply will call the dedicated updater container to pull, migrate, "
                "rebuild, and restart the configured services.";
            return config;
        }

        config.command = envOrDefault("MYCLOUD_UPDATE_COMMAND");
        if (config.command.empty()) {
            config.message =
                "One-click apply is disabled because MYCLOUD_UPDATE_COMMAND is empty.";
            return config;
        }

        if (access("/var/run/docker.sock", R_OK | W_OK) != 0) {
            config.message =
                "One-click apply is configured, but /var/run/docker.sock is not mounted into "
                "the backend container.";
            return config;
        }

        if (access("/opt/mycloud/docker-compose.yml", R_OK) != 0) {
            config.message =
                "One-click apply is configured, but /opt/mycloud/docker-compose.yml is not "
                "available inside the backend container.";
            return config;
        }

        config.supported = true;
        config.message =
            "One-click apply will pull the latest git changes and rebuild the configured "
            "services through the host Docker daemon.";
        return config;
    }

    static bool isDigitsOnly(const std::string& value) {
        if (value.empty()) {
            return false;
        }
        for (unsigned char ch : value) {
            if (!std::isdigit(ch)) {
                return false;
            }
        }
        return true;
    }

    static bool isAlphaNumHyphenOnly(const std::string& value) {
        if (value.empty()) {
            return false;
        }
        for (unsigned char ch : value) {
            if (!std::isalnum(ch) && ch != '-') {
                return false;
            }
        }
        return true;
    }

    static std::optional<int> parseNumericPart(const std::string& part) {
        if (!isDigitsOnly(part)) {
            return std::nullopt;
        }
        try {
            return std::stoi(part);
        } catch (...) {
            return std::nullopt;
        }
    }

    static std::optional<ParsedVersion> parseVersion(const std::string& version) {
        std::string normalized =
            (!version.empty() && (version[0] == 'v' || version[0] == 'V'))
                ? version.substr(1)
                : version;

        const std::size_t buildPos = normalized.find('+');
        if (buildPos != std::string::npos) {
            normalized = normalized.substr(0, buildPos);
        }

        std::string prereleasePart;
        const std::size_t prereleasePos = normalized.find('-');
        if (prereleasePos != std::string::npos) {
            prereleasePart = normalized.substr(prereleasePos + 1);
            normalized = normalized.substr(0, prereleasePos);
        }

        std::stringstream coreStream(normalized);
        std::string corePart;
        std::vector<std::string> coreParts;
        while (std::getline(coreStream, corePart, '.')) {
            coreParts.push_back(corePart);
        }
        if (coreParts.size() != 3) {
            return std::nullopt;
        }

        ParsedVersion parsed;
        const auto major = parseNumericPart(coreParts[0]);
        const auto minor = parseNumericPart(coreParts[1]);
        const auto patch = parseNumericPart(coreParts[2]);
        if (!major || !minor || !patch) {
            return std::nullopt;
        }

        parsed.major = *major;
        parsed.minor = *minor;
        parsed.patch = *patch;

        if (prereleasePart.empty()) {
            return parsed;
        }

        std::stringstream prereleaseStream(prereleasePart);
        std::string identifier;
        while (std::getline(prereleaseStream, identifier, '.')) {
            if (!isAlphaNumHyphenOnly(identifier)) {
                return std::nullopt;
            }

            PreReleaseIdentifier parsedIdentifier;
            parsedIdentifier.numeric = isDigitsOnly(identifier);
            parsedIdentifier.value = identifier;

            if (parsedIdentifier.numeric && identifier.size() > 1 && identifier[0] == '0') {
                return std::nullopt;
            }

            parsed.prerelease.push_back(std::move(parsedIdentifier));
        }

        if (parsed.prerelease.empty()) {
            return std::nullopt;
        }

        return parsed;
    }

    static int compareNumericStrings(const std::string& left, const std::string& right) {
        if (left.size() != right.size()) {
            return left.size() < right.size() ? -1 : 1;
        }
        if (left == right) {
            return 0;
        }
        return left < right ? -1 : 1;
    }

    static int comparePreRelease(const ParsedVersion& left, const ParsedVersion& right) {
        if (left.prerelease.empty() && right.prerelease.empty()) {
            return 0;
        }
        if (left.prerelease.empty()) {
            return 1;
        }
        if (right.prerelease.empty()) {
            return -1;
        }

        const std::size_t count = std::min(left.prerelease.size(), right.prerelease.size());
        for (std::size_t i = 0; i < count; ++i) {
            const auto& leftId = left.prerelease[i];
            const auto& rightId = right.prerelease[i];

            if (leftId.numeric && rightId.numeric) {
                const int numericCmp = compareNumericStrings(leftId.value, rightId.value);
                if (numericCmp != 0) {
                    return numericCmp;
                }
                continue;
            }

            if (leftId.numeric != rightId.numeric) {
                return leftId.numeric ? -1 : 1;
            }

            if (leftId.value != rightId.value) {
                return leftId.value < rightId.value ? -1 : 1;
            }
        }

        if (left.prerelease.size() == right.prerelease.size()) {
            return 0;
        }
        return left.prerelease.size() < right.prerelease.size() ? -1 : 1;
    }

    static int compareVersions(const ParsedVersion& left, const ParsedVersion& right) {
        if (left.major != right.major) {
            return left.major < right.major ? -1 : 1;
        }
        if (left.minor != right.minor) {
            return left.minor < right.minor ? -1 : 1;
        }
        if (left.patch != right.patch) {
            return left.patch < right.patch ? -1 : 1;
        }
        return comparePreRelease(left, right);
    }

    static bool isVersionNewer(const std::string& candidate, const std::string& current) {
        const auto candidateVersion = parseVersion(candidate);
        const auto currentVersion = parseVersion(current);
        if (!candidateVersion || !currentVersion) {
            return false;
        }
        return compareVersions(*candidateVersion, *currentVersion) > 0;
    }

    static void redirectDescriptor(int sourceFd, int targetFd) {
        if (sourceFd >= 0 && sourceFd != targetFd) {
            dup2(sourceFd, targetFd);
        }
    }

    static std::string launchDetachedCommand(
        const std::string& command,
        const std::string& logPath,
        const std::vector<std::pair<std::string, std::string>>& envVars) {
        int execPipe[2];
        if (pipe(execPipe) != 0) {
            return "Failed to create update command pipe: " + std::string(std::strerror(errno));
        }
        if (fcntl(execPipe[1], F_SETFD, FD_CLOEXEC) == -1) {
            close(execPipe[0]);
            close(execPipe[1]);
            return "Failed to prepare update command pipe: " + std::string(std::strerror(errno));
        }

        const pid_t pid = fork();
        if (pid < 0) {
            close(execPipe[0]);
            close(execPipe[1]);
            return "Failed to fork update command: " + std::string(std::strerror(errno));
        }

        if (pid == 0) {
            close(execPipe[0]);
            setsid();

            for (const auto& [key, value] : envVars) {
                setenv(key.c_str(), value.c_str(), 1);
            }

            const int stdinFd = open("/dev/null", O_RDONLY);
            const int logFd = open(logPath.c_str(), O_WRONLY | O_CREAT | O_APPEND, 0644);
            const int nullWriteFd = (logFd >= 0) ? -1 : open("/dev/null", O_WRONLY);

            redirectDescriptor(stdinFd, STDIN_FILENO);
            redirectDescriptor(logFd >= 0 ? logFd : nullWriteFd, STDOUT_FILENO);
            redirectDescriptor(logFd >= 0 ? logFd : nullWriteFd, STDERR_FILENO);

            if (stdinFd >= 0) close(stdinFd);
            if (logFd >= 0) close(logFd);
            if (nullWriteFd >= 0) close(nullWriteFd);

            execl("/bin/sh", "sh", "-lc", command.c_str(), static_cast<char*>(nullptr));

            const int execErrno = errno;
            (void)write(execPipe[1], &execErrno, sizeof(execErrno));
            close(execPipe[1]);
            _exit(127);
        }

        close(execPipe[1]);

        int execErrno = 0;
        ssize_t readBytes = -1;
        do {
            readBytes = read(execPipe[0], &execErrno, sizeof(execErrno));
        } while (readBytes == -1 && errno == EINTR);
        close(execPipe[0]);
        if (readBytes > 0) {
            int status = 0;
            while (waitpid(pid, &status, 0) == -1 && errno == EINTR) {
            }
            return "Failed to launch update command: " + std::string(std::strerror(execErrno));
        }
        if (readBytes == -1) {
            int status = 0;
            while (waitpid(pid, &status, 0) == -1 && errno == EINTR) {
            }
            return "Failed to read update command status: " + std::string(std::strerror(errno));
        }

        std::thread([pid, logPath]() {
            int status = 0;
            while (waitpid(pid, &status, 0) == -1 && errno == EINTR) {
            }

            if (WIFEXITED(status) && WEXITSTATUS(status) == 0) {
                setState("succeeded",
                         "Update finished successfully. Check " + logPath +
                             " if you need the deployment log.",
                         logPath,
                         snapshotState().targetVersion,
                         false);
            } else {
                std::string failureMessage =
                    "Update failed. Check " + logPath + " for details.";
                if (WIFEXITED(status)) {
                    failureMessage = "Update failed with exit code " +
                                     std::to_string(WEXITSTATUS(status)) + ". Check " + logPath +
                                     " for details.";
                } else if (WIFSIGNALED(status)) {
                    failureMessage = "Update terminated by signal " +
                                     std::to_string(WTERMSIG(status)) + ". Check " + logPath +
                                     " for details.";
                }
                setState("failed",
                         failureMessage,
                         logPath,
                         snapshotState().targetVersion,
                         false);
            }
            updateInProgress_.store(false);
        }).detach();

        return "";
    }
};
