/* ------------------------------------------------------------------ */
/*  Element refs                                                       */
/* ------------------------------------------------------------------ */
const els = {
  // Header
  statusDot:       document.getElementById("status-dot"),
  statusLabel:     document.getElementById("status-label"),

  // Views
  mainView:        document.getElementById("main-view"),
  loginView:       document.getElementById("login-view"),
  settingsView:    document.getElementById("settings-view"),

  // Main view
  syncDetailMsg:   document.getElementById("sync-detail-msg"),
  lastSyncLabel:   document.getElementById("last-sync-label"),
  progressSection: document.getElementById("progress-section"),
  progressLabel:   document.getElementById("progress-label"),
  progressCount:   document.getElementById("progress-count"),
  progressFill:    document.getElementById("progress-fill"),
  progressFile:    document.getElementById("progress-file"),
  pickFolderBtn:   document.getElementById("pick-folder-btn"),
  foldersList:     document.getElementById("folders-list"),
  folderTemplate:  document.getElementById("folder-template"),
  activityList:    document.getElementById("activity-list"),
  activityCount:   document.getElementById("activity-count"),
  activityTemplate:document.getElementById("activity-template"),

  // Login view
  loginForm:       document.getElementById("login-form"),
  apiBaseUrl:      document.getElementById("apiBaseUrl"),
  email:           document.getElementById("email"),
  password:        document.getElementById("password"),
  allowSelfSigned: document.getElementById("allowSelfSignedTls"),
  loginBtn:        document.getElementById("login-btn"),
  loginError:      document.getElementById("login-error"),

  // Settings view
  accountUsername: document.getElementById("account-username"),
  accountEmail:    document.getElementById("account-email"),
  accountApiUrl:   document.getElementById("account-api-url"),
  storageText:     document.getElementById("storage-text"),
  storageFill:     document.getElementById("storage-fill"),
  autoSyncSelect:  document.getElementById("auto-sync-select"),
  logoutBtn:       document.getElementById("logout-btn"),

  // Footer
  syncNowBtn:        document.getElementById("sync-now-btn"),
  syncLabel:         document.getElementById("sync-label"),
  settingsBtn:       document.getElementById("settings-btn"),
  settingsGearIcon:  document.getElementById("settings-gear-icon"),
  settingsCloseIcon: document.getElementById("settings-close-icon"),
  settingsLabel:     document.getElementById("settings-label"),
};

/* ------------------------------------------------------------------ */
/*  State                                                              */
/* ------------------------------------------------------------------ */
let currentState = null;
let currentView = ""; // "main" | "login" | "settings"

/* ------------------------------------------------------------------ */
/*  View switching                                                     */
/* ------------------------------------------------------------------ */
function showView(view) {
  if (currentView === view) return;
  currentView = view;
  els.mainView.classList.toggle("is-hidden", view !== "main");
  els.loginView.classList.toggle("is-hidden", view !== "login");
  els.settingsView.classList.toggle("is-hidden", view !== "settings");

  const inMain = view === "main";
  const inSettings = view === "settings";

  els.syncNowBtn.classList.toggle("active", inMain);
  els.settingsBtn.classList.toggle("active", inSettings);
  els.settingsGearIcon.classList.remove("is-hidden");
  els.settingsCloseIcon.classList.add("is-hidden");
  els.settingsLabel.textContent = "Settings";
}

/* ------------------------------------------------------------------ */
/*  Formatting helpers                                                 */
/* ------------------------------------------------------------------ */
function timeAgo(isoString) {
  if (!isoString) return "";
  const seconds = Math.floor((Date.now() - new Date(isoString).getTime()) / 1000);
  if (seconds < 5)  return "just now";
  if (seconds < 60) return `${seconds}s ago`;
  const mins = Math.floor(seconds / 60);
  if (mins < 60)    return `${mins}m ago`;
  const hrs = Math.floor(mins / 60);
  if (hrs < 24)     return `${hrs}h ago`;
  return new Date(isoString).toLocaleDateString();
}

function formatBytes(bytes) {
  if (bytes === null || bytes === undefined) return "-";
  if (bytes < 1024)             return bytes + " B";
  if (bytes < 1024 * 1024)     return (bytes / 1024).toFixed(1) + " KB";
  if (bytes < 1024 ** 3)       return (bytes / 1024 ** 2).toFixed(1) + " MB";
  return (bytes / 1024 ** 3).toFixed(2) + " GB";
}

/* ------------------------------------------------------------------ */
/*  Render: progress bar                                               */
/* ------------------------------------------------------------------ */
function renderProgress(syncStatus) {
  const progress = syncStatus.progress;
  const isSyncing = syncStatus.state === "syncing" && progress && progress.totalItems > 0;

  els.progressSection.classList.toggle("is-hidden", !isSyncing);
  if (!isSyncing) return;

  const pct = Math.min(progress.percent ?? 0, 100);
  els.progressLabel.textContent = syncStatus.message || "Syncing...";
  els.progressCount.textContent = `${progress.completedItems} / ${progress.totalItems}`;
  els.progressFill.style.width = `${pct}%`;
  els.progressFile.textContent = progress.currentFile || "";
  els.progressFile.title = progress.currentFile || "";
}

/* ------------------------------------------------------------------ */
/*  Render: folder list                                                */
/* ------------------------------------------------------------------ */
function renderFolders(roots) {
  els.foldersList.innerHTML = "";

  if (!roots || roots.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.textContent = "No folders added. Click + to add one.";
    els.foldersList.appendChild(empty);
    return;
  }

  for (const root of roots) {
    const node = els.folderTemplate.content.firstElementChild.cloneNode(true);
    node.querySelector(".folder-title").textContent = root.label;

    const parts = [];
    if (root.fileCount != null) parts.push(`${root.fileCount} file${root.fileCount !== 1 ? "s" : ""}`);
    if (root.lastScanAt)        parts.push(timeAgo(root.lastScanAt));
    node.querySelector(".folder-meta").textContent = parts.join(" · ") || root.localPath;
    node.querySelector(".folder-meta").title = root.localPath;

    node.querySelector(".open-folder-btn").addEventListener("click", () => {
      window.desktopApi.openFolder(root.id);
    });
    node.querySelector(".remove-folder-btn").addEventListener("click", async () => {
      try {
        await window.desktopApi.removeFolder(root.id);
      } catch (err) {
        showError(err.message || "Failed to remove folder");
      }
    });

    els.foldersList.appendChild(node);
  }
}

/* ------------------------------------------------------------------ */
/*  Render: activity log                                               */
/* ------------------------------------------------------------------ */
function renderActivity(events) {
  els.activityList.innerHTML = "";

  if (!events || events.length === 0) {
    const empty = document.createElement("div");
    empty.className = "empty-state";
    empty.style.padding = "10px";
    empty.textContent = "No recent activity.";
    els.activityList.appendChild(empty);
    els.activityCount.textContent = "";
    return;
  }

  // Show max 15 events
  const shown = events.slice(0, 15);
  els.activityCount.textContent = events.length > 15 ? `${events.length}` : "";

  for (const evt of shown) {
    const node = els.activityTemplate.content.firstElementChild.cloneNode(true);
    const dot = node.querySelector(".activity-dot");
    dot.classList.add(evt.level || "info");
    node.querySelector(".activity-msg").textContent = evt.message || "";
    node.querySelector(".activity-time").textContent = timeAgo(evt.timestamp);
    els.activityList.appendChild(node);
  }
}

/* ------------------------------------------------------------------ */
/*  Render: storage bar                                                */
/* ------------------------------------------------------------------ */
function renderStorage(stats) {
  if (!stats || !stats.quota_bytes) {
    els.storageText.textContent = "- / -";
    els.storageFill.style.width = "0%";
    return;
  }
  const used = stats.used_bytes || 0;
  const quota = stats.quota_bytes;
  const pct = Math.min((used / quota) * 100, 100);
  els.storageText.textContent = `${formatBytes(used)} / ${formatBytes(quota)}`;
  els.storageFill.style.width = `${pct}%`;
  els.storageFill.className = "storage-fill" + (pct > 90 ? " full" : pct > 75 ? " warn" : "");
}

/* ------------------------------------------------------------------ */
/*  Main render                                                        */
/* ------------------------------------------------------------------ */
function render(snapshot) {
  currentState = snapshot;
  const user = snapshot.auth?.user;
  const status = snapshot.syncStatus;
  const stateKey = status?.state || "idle";

  // ── Header status chip ──────────────────────────────────────────
  els.statusDot.className = `status-dot ${stateKey}`;
  const labels = { idle: "Idle", syncing: "Syncing", error: "Error", paused: "Paused" };
  els.statusLabel.textContent = labels[stateKey] || stateKey;

  // ── View routing ────────────────────────────────────────────────
  if (!user) {
    showView("login");
    if (snapshot.apiBaseUrl) els.apiBaseUrl.value = snapshot.apiBaseUrl;
    els.allowSelfSigned.checked = Boolean(snapshot.allowSelfSignedTls);
    return;
  }

  // Logged in — switch away from login
  if (currentView === "login" || currentView === "") showView("main");

  // ── Sync info row ───────────────────────────────────────────────
  els.syncDetailMsg.textContent = status.message || "Idle";
  els.lastSyncLabel.textContent = status.lastSyncAt
    ? "Last: " + timeAgo(status.lastSyncAt)
    : "";

  // ── Progress ────────────────────────────────────────────────────
  renderProgress(status);

  // ── Folders ─────────────────────────────────────────────────────
  renderFolders(snapshot.roots);

  // ── Activity log ────────────────────────────────────────────────
  renderActivity(snapshot.events);

  // ── Settings (update even if not visible) ───────────────────────
  els.accountUsername.textContent = user.username || "-";
  els.accountEmail.textContent    = user.email || "-";
  els.accountApiUrl.textContent   = snapshot.apiBaseUrl || "http://localhost:8080";
  els.autoSyncSelect.value = String(snapshot.autoSyncMinutes ?? 5);
  renderStorage(snapshot.storageStats || null);

  // ── Footer button states ────────────────────────────────────────
  const isSyncing = stateKey === "syncing";
  const isPaused  = stateKey === "paused";
  els.syncNowBtn.disabled = isSyncing || isPaused;
  els.syncLabel.textContent = isSyncing ? "Syncing…" : "Sync now";
}

/* ------------------------------------------------------------------ */
/*  Error display                                                      */
/* ------------------------------------------------------------------ */
function showError(msg) {
  els.loginError.textContent = msg;
  els.loginError.classList.remove("is-hidden");
  setTimeout(() => els.loginError.classList.add("is-hidden"), 5000);
}

/* ------------------------------------------------------------------ */
/*  Event listeners                                                    */
/* ------------------------------------------------------------------ */

// Settings toggle
els.settingsBtn.addEventListener("click", () => {
  showView(currentView === "settings" ? "main" : "settings");
});

// Login form submit
els.loginForm.addEventListener("submit", async (e) => {
  e.preventDefault();
  els.loginError.classList.add("is-hidden");
  els.loginBtn.disabled = true;
  els.loginBtn.textContent = "Signing in...";
  const form = new FormData(els.loginForm);
  try {
    await window.desktopApi.login({
      apiBaseUrl:       form.get("apiBaseUrl"),
      email:            form.get("email"),
      password:         form.get("password"),
      allowSelfSignedTls: form.get("allowSelfSignedTls") === "on",
    });
    els.password.value = "";
  } catch (err) {
    showError(err.message || "Login failed");
  } finally {
    els.loginBtn.disabled = false;
    els.loginBtn.textContent = "Sign In";
  }
});

// Logout
els.logoutBtn.addEventListener("click", async () => {
  try {
    await window.desktopApi.logout();
    showView("login");
  } catch (err) {
    console.error(err);
  }
});

// Pick folder
els.pickFolderBtn.addEventListener("click", async () => {
  try {
    const folderPath = await window.desktopApi.pickFolder();
    if (folderPath) await window.desktopApi.addFolder(folderPath);
  } catch (err) {
    console.error("Add folder error:", err);
  }
});

// Sync now — jump to main view so progress is visible
els.syncNowBtn.addEventListener("click", async () => {
  if (currentView === "settings") showView("main");
  try {
    await window.desktopApi.syncNow();
  } catch (err) {
    console.error("Sync error:", err);
  }
});

// Auto-sync interval
els.autoSyncSelect.addEventListener("change", () => {
  const minutes = parseInt(els.autoSyncSelect.value, 10);
  window.desktopApi.setAutoSyncInterval(minutes);
});

/* ------------------------------------------------------------------ */
/*  Periodic relative-time refresh (every 30 s)                       */
/* ------------------------------------------------------------------ */
setInterval(() => {
  if (currentState) render(currentState);
}, 30_000);

/* ------------------------------------------------------------------ */
/*  Bootstrap                                                          */
/* ------------------------------------------------------------------ */
window.desktopApi.onStateChanged((snapshot) => render(snapshot));
window.desktopApi.getState().then((snapshot) => render(snapshot));
