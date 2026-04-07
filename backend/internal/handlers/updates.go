package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/iii123iii/mycloud/backend/internal/utils"
)

// UpdatesHandler handles /api/admin/updates/* routes.
type UpdatesHandler struct {
	version    string
	githubRepo string
	updateInProgress atomic.Bool
	stateMu    sync.Mutex
	state      updateState
}

type updateState struct {
	InProgress    bool
	Status        string
	Message       string
	LogPath       string
	TargetVersion string
}

func NewUpdatesHandler(version, githubRepo string) *UpdatesHandler {
	return &UpdatesHandler{
		version:    version,
		githubRepo: githubRepo,
		state:      updateState{Status: "idle"},
	}
}

type releaseInfo struct {
	Latest       string
	ReleaseURL   string
	ReleaseName  string
	PublishedAt  string
	ReleaseNotes string
}

type applyConfig struct {
	Supported         bool
	Command           string
	Message           string
	LogPath           string
	UpdaterURL        string
	UseRemoteUpdater  bool
}

// CheckUpdate handles GET /api/admin/updates/check
func (h *UpdatesHandler) CheckUpdate(w http.ResponseWriter, r *http.Request) {
	release, err := h.fetchLatestRelease()
	if err != nil {
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	cfg := h.getApplyConfig()
	state := h.snapshotState()

	if cfg.UseRemoteUpdater {
		remoteState, err := h.fetchRemoteUpdaterState(cfg.UpdaterURL)
		if err != nil {
			state.Status = "unknown"
			state.Message = err.Error()
		} else {
			state = *remoteState
		}
	}

	utils.OkJSON(w, h.buildResponse(release, cfg, state))
}

// ApplyUpdate handles POST /api/admin/updates/apply
func (h *UpdatesHandler) ApplyUpdate(w http.ResponseWriter, r *http.Request) {
	cfg := h.getApplyConfig()
	if !cfg.Supported {
		utils.ErrorJSON(w, http.StatusNotImplemented, cfg.Message)
		return
	}

	if !h.updateInProgress.CompareAndSwap(false, true) {
		utils.ErrorJSON(w, http.StatusConflict, "An update command is already running.")
		return
	}

	release, err := h.fetchLatestRelease()
	if err != nil {
		h.updateInProgress.Store(false)
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	if !isVersionNewer(release.Latest, h.version) {
		h.updateInProgress.Store(false)
		h.setState("idle", "No newer GitHub release is available for this server.", cfg.LogPath, "", false)
		utils.ErrorJSON(w, http.StatusConflict, "No newer GitHub release is available for this server.")
		return
	}

	h.setState("running",
		"Applying "+release.Latest+". Logs are being written to "+cfg.LogPath+".",
		cfg.LogPath, release.Latest, true)

	if cfg.UseRemoteUpdater {
		msg, logPath, err := h.triggerRemoteUpdate(cfg.UpdaterURL, release.Latest, h.version)
		h.updateInProgress.Store(false)
		if err != nil {
			h.setState("failed", err.Error(), cfg.LogPath, release.Latest, false)
			utils.ErrorJSON(w, http.StatusBadGateway, err.Error())
			return
		}
		if logPath == "" {
			logPath = cfg.LogPath
		}
		utils.AcceptedJSON(w, map[string]string{"message": msg, "log_path": logPath})
		return
	}

	// Launch detached shell command
	if err := h.launchDetachedCommand(cfg.Command, cfg.LogPath, map[string]string{
		"MYCLOUD_UPDATE_TARGET_VERSION":  release.Latest,
		"MYCLOUD_UPDATE_CURRENT_VERSION": h.version,
		"MYCLOUD_UPDATE_RELEASE_URL":     release.ReleaseURL,
	}); err != nil {
		h.updateInProgress.Store(false)
		h.setState("failed", err.Error(), cfg.LogPath, release.Latest, false)
		utils.ErrorJSON(w, http.StatusInternalServerError, err.Error())
		return
	}

	utils.AcceptedJSON(w, map[string]string{
		"message":  "Update started. Watch " + cfg.LogPath + " if the server does not come back.",
		"log_path": cfg.LogPath,
	})
}

// --- helpers ---

func (h *UpdatesHandler) setState(status, message, logPath, targetVersion string, inProgress bool) {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	h.state = updateState{
		InProgress:    inProgress,
		Status:        status,
		Message:       message,
		LogPath:       logPath,
		TargetVersion: targetVersion,
	}
}

func (h *UpdatesHandler) snapshotState() updateState {
	h.stateMu.Lock()
	defer h.stateMu.Unlock()
	return h.state
}

func (h *UpdatesHandler) buildResponse(release releaseInfo, cfg applyConfig, state updateState) map[string]any {
	return map[string]any{
		"current":               h.version,
		"latest":                release.Latest,
		"update_available":      isVersionNewer(release.Latest, h.version),
		"release_url":           release.ReleaseURL,
		"release_name":          release.ReleaseName,
		"published_at":          release.PublishedAt,
		"release_notes":         release.ReleaseNotes,
		"apply_supported":       cfg.Supported,
		"apply_message":         cfg.Message,
		"update_in_progress":    state.InProgress,
		"update_status":         state.Status,
		"update_status_message": state.Message,
		"update_log_path":       state.LogPath,
		"last_started_target":   state.TargetVersion,
	}
}

func (h *UpdatesHandler) getApplyConfig() applyConfig {
	cfg := applyConfig{
		LogPath: envOrDefaultUpdater("MYCLOUD_UPDATE_LOG_PATH", "/data/logs/update.log"),
	}
	if !isValidLogPath(cfg.LogPath) {
		cfg.Message = "MYCLOUD_UPDATE_LOG_PATH must be an absolute path under /tmp/ or /data/ with no path-traversal components."
		return cfg
	}

	if url := envOrDefaultUpdater("MYCLOUD_UPDATER_URL", ""); url != "" {
		cfg.Supported = true
		cfg.UseRemoteUpdater = true
		cfg.UpdaterURL = url
		cfg.Message = "One-click apply will call the dedicated updater container."
		return cfg
	}

	cmd := envOrDefaultUpdater("MYCLOUD_UPDATE_COMMAND", "")
	if cmd == "" {
		cfg.Message = "One-click apply is disabled because MYCLOUD_UPDATE_COMMAND is empty."
		return cfg
	}
	if _, err := os.Stat("/var/run/docker.sock"); err != nil {
		cfg.Message = "One-click apply is configured, but /var/run/docker.sock is not mounted."
		return cfg
	}
	projectDir := envOrDefaultUpdater("MYCLOUD_PROJECT_DIR", "/opt/mycloud")
	if _, err := os.Stat(projectDir + "/docker-compose.yml"); err != nil {
		cfg.Message = "One-click apply is configured, but docker-compose.yml is not available."
		return cfg
	}
	cfg.Supported = true
	cfg.Command = cmd
	cfg.Message = "One-click apply will pull the latest git changes and rebuild."
	return cfg
}

func isValidLogPath(path string) bool {
	if path == "" || path[0] != '/' {
		return false
	}
	if strings.Contains(path, "..") {
		return false
	}
	for _, prefix := range []string{"/tmp/", "/data/"} {
		if len(path) > len(prefix) && strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

func envOrDefaultUpdater(name, def string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return def
}

func (h *UpdatesHandler) fetchLatestRelease() (releaseInfo, error) {
	url := "https://api.github.com/repos/" + h.githubRepo + "/releases/latest"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return releaseInfo{}, err
	}
	req.Header.Set("User-Agent", "MyCloud-Server/"+h.version)
	req.Header.Set("Accept", "application/vnd.github+json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return releaseInfo{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return releaseInfo{}, &httpError{code: resp.StatusCode, msg: "GitHub API returned HTTP " + strconv.Itoa(resp.StatusCode)}
	}

	var body struct {
		TagName     string `json:"tag_name"`
		HTMLURL     string `json:"html_url"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
		Body        string `json:"body"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return releaseInfo{}, err
	}

	notes := body.Body
	if len(notes) > 800 {
		notes = notes[:800] + "..."
	}
	return releaseInfo{
		Latest:       body.TagName,
		ReleaseURL:   body.HTMLURL,
		ReleaseName:  body.Name,
		PublishedAt:  body.PublishedAt,
		ReleaseNotes: notes,
	}, nil
}

type httpError struct {
	code int
	msg  string
}

func (e *httpError) Error() string { return e.msg }

func (h *UpdatesHandler) fetchRemoteUpdaterState(updaterURL string) (*updateState, error) {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(updaterURL + "/status")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var body struct {
		InProgress    bool   `json:"in_progress"`
		Message       string `json:"message"`
		Status        string `json:"status"`
		LogPath       string `json:"log_path"`
		TargetVersion string `json:"target_version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return &updateState{
		InProgress:    body.InProgress,
		Message:       body.Message,
		Status:        body.Status,
		LogPath:       body.LogPath,
		TargetVersion: body.TargetVersion,
	}, nil
}

func (h *UpdatesHandler) triggerRemoteUpdate(updaterURL, targetVersion, currentVersion string) (string, string, error) {
	payload, _ := json.Marshal(map[string]string{
		"target_version":  targetVersion,
		"current_version": currentVersion,
	})
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Post(updaterURL+"/update", "application/json", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var result map[string]string
	_ = json.Unmarshal(body, &result)

	if resp.StatusCode != http.StatusAccepted {
		msg := "Updater container rejected the request."
		if v, ok := result["error"]; ok {
			msg = v
		}
		return "", "", &httpError{code: resp.StatusCode, msg: msg}
	}

	msg := result["message"]
	if msg == "" {
		msg = "Update started."
	}
	return msg, result["log_path"], nil
}

func (h *UpdatesHandler) launchDetachedCommand(command, logPath string, envVars map[string]string) error {
	cmd := exec.Command("sh", "-lc", command) // #nosec G204 — command comes from admin-set env var
	cmd.Dir = "/"

	// Set environment variables for the update script
	cmd.Env = os.Environ()
	for k, v := range envVars {
		cmd.Env = append(cmd.Env, k+"="+v)
	}

	// Redirect stdout/stderr to the log file
	logFile, err := os.OpenFile(logPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0644)
	if err != nil {
		// Fall back to /dev/null
		devNull, _ := os.Open(os.DevNull)
		cmd.Stdout = devNull
		cmd.Stderr = devNull
	} else {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	// Reap the child in a goroutine so it doesn't become a zombie,
	// and update state when it finishes.
	go func() {
		err := cmd.Wait()
		if logFile != nil {
			logFile.Close()
		}
		state := h.snapshotState()
		if err == nil {
			h.setState("succeeded",
				"Update finished successfully. Check "+logPath+" if you need the deployment log.",
				logPath, state.TargetVersion, false)
		} else {
			h.setState("failed",
				"Update failed. Check "+logPath+" for details.",
				logPath, state.TargetVersion, false)
		}
		h.updateInProgress.Store(false)
	}()

	return nil
}

// ─── Semantic version comparison (ported from C++) ───────────────────────────

type parsedVersion struct {
	major, minor, patch int
	prerelease          []string
}

func parseVersion(s string) (*parsedVersion, bool) {
	if s == "" {
		return nil, false
	}
	// Strip leading v/V
	if s[0] == 'v' || s[0] == 'V' {
		s = s[1:]
	}
	// Strip build metadata
	if i := strings.Index(s, "+"); i != -1 {
		s = s[:i]
	}
	// Split pre-release
	var prerelease []string
	if i := strings.Index(s, "-"); i != -1 {
		for _, id := range strings.Split(s[i+1:], ".") {
			if id == "" {
				return nil, false
			}
			prerelease = append(prerelease, id)
		}
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return nil, false
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(parts[1])
	patch, err3 := strconv.Atoi(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return nil, false
	}
	return &parsedVersion{major, minor, patch, prerelease}, true
}

func compareVersions(a, b *parsedVersion) int {
	if a.major != b.major {
		if a.major < b.major {
			return -1
		}
		return 1
	}
	if a.minor != b.minor {
		if a.minor < b.minor {
			return -1
		}
		return 1
	}
	if a.patch != b.patch {
		if a.patch < b.patch {
			return -1
		}
		return 1
	}
	// Pre-release: no pre-release > has pre-release
	if len(a.prerelease) == 0 && len(b.prerelease) == 0 {
		return 0
	}
	if len(a.prerelease) == 0 {
		return 1
	}
	if len(b.prerelease) == 0 {
		return -1
	}
	n := len(a.prerelease)
	if len(b.prerelease) < n {
		n = len(b.prerelease)
	}
	for i := 0; i < n; i++ {
		ai, aerr := strconv.Atoi(a.prerelease[i])
		bi, berr := strconv.Atoi(b.prerelease[i])
		if aerr == nil && berr == nil {
			if ai != bi {
				if ai < bi {
					return -1
				}
				return 1
			}
		} else if aerr == nil {
			return -1 // numeric < alphanumeric
		} else if berr == nil {
			return 1
		} else {
			if a.prerelease[i] != b.prerelease[i] {
				if a.prerelease[i] < b.prerelease[i] {
					return -1
				}
				return 1
			}
		}
	}
	if len(a.prerelease) == len(b.prerelease) {
		return 0
	}
	if len(a.prerelease) < len(b.prerelease) {
		return -1
	}
	return 1
}

func isVersionNewer(candidate, current string) bool {
	av, aok := parseVersion(candidate)
	bv, bok := parseVersion(current)
	if !aok || !bok {
		return false
	}
	return compareVersions(av, bv) > 0
}
