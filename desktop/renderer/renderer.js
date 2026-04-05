const els = {
  loginForm: document.getElementById("login-form"),
  loggedOutView: document.getElementById("logged-out-view"),
  loggedInView: document.getElementById("logged-in-view"),
  apiBaseUrl: document.getElementById("apiBaseUrl"),
  email: document.getElementById("email"),
  password: document.getElementById("password"),
  allowSelfSignedTls: document.getElementById("allowSelfSignedTls"),
  logoutBtn: document.getElementById("logout-btn"),
  accountUsername: document.getElementById("account-username"),
  accountEmail: document.getElementById("account-email"),
  accountApiUrl: document.getElementById("account-api-url"),
  pickFolderBtn: document.getElementById("pick-folder-btn"),
  syncNowBtn: document.getElementById("sync-now-btn"),
  openFolderBtn: document.getElementById("open-folder-btn"),
  viewOnlineBtn: document.getElementById("view-online-btn"),
  settingsToggle: document.getElementById("settings-toggle"),
  settingsPanel: document.getElementById("settings-panel"),
  primaryCta: document.getElementById("primary-cta"),
  topbarSubtitle: document.getElementById("topbar-subtitle"),
  heroTitle: document.getElementById("hero-title"),
  heroText: document.getElementById("hero-text"),
  statusDot: document.getElementById("status-dot"),
  statusLabel: document.getElementById("status-label"),
  statusText: document.getElementById("status-text"),
  foldersList: document.getElementById("folders-list"),
  folderTemplate: document.getElementById("folder-template"),
  // Progress bar elements
  progressSection: document.getElementById("progress-section"),
  progressLabel: document.getElementById("progress-label"),
  progressCount: document.getElementById("progress-count"),
  progressFill: document.getElementById("progress-fill"),
  progressFile: document.getElementById("progress-file"),
};

let currentState = null;

function formatDate(value) {
  return value ? new Date(value).toLocaleString() : "Never";
}

function toggleSettings(force) {
  const shouldOpen = typeof force === "boolean" ? force : els.settingsPanel.classList.contains("is-hidden");
  els.settingsPanel.classList.toggle("is-hidden", !shouldOpen);
  els.settingsToggle.textContent = shouldOpen ? "-" : "+";
}

function renderFolders(snapshot) {
  els.foldersList.innerHTML = "";
  if (!snapshot.roots.length) {
    els.foldersList.innerHTML = '<div class="empty-state">No folders selected.</div>';
    return;
  }

  for (const root of snapshot.roots) {
    const node = els.folderTemplate.content.firstElementChild.cloneNode(true);
    node.querySelector(".folder-title").textContent = root.label;
    node.querySelector(".folder-path").textContent = root.localPath;
    node.querySelector(".remove-folder-btn").addEventListener("click", async () => {
      try {
        await window.desktopApi.removeFolder(root.id);
      } catch (error) {
        alert(error.message || "Failed to remove folder");
      }
    });
    els.foldersList.appendChild(node);
  }
}

function renderProgress(syncStatus) {
  const progress = syncStatus.progress;
  const isSyncing = syncStatus.state === "syncing" && progress && progress.totalItems > 0;

  els.progressSection.classList.toggle("is-hidden", !isSyncing);

  if (!isSyncing) return;

  const percent = Math.min(progress.percent, 100);
  els.progressLabel.textContent = syncStatus.message || "Syncing...";
  els.progressCount.textContent = `${progress.completedItems} / ${progress.totalItems}`;
  els.progressFill.style.width = `${percent}%`;

  if (progress.currentFile) {
    els.progressFile.textContent = progress.currentFile;
    els.progressFile.title = progress.currentFile;
  } else {
    els.progressFile.textContent = "";
  }
}

function render(snapshot) {
  currentState = snapshot;
  const user = snapshot.auth.user;
  const status = snapshot.syncStatus;

  els.apiBaseUrl.value = snapshot.apiBaseUrl || "http://localhost:8080";
  els.allowSelfSignedTls.checked = Boolean(snapshot.allowSelfSignedTls);
  els.statusDot.className = "status-dot " + status.state;
  els.statusLabel.textContent = status.state[0].toUpperCase() + status.state.slice(1);
  els.statusText.textContent = status.lastSyncAt
    ? status.message + " | " + formatDate(status.lastSyncAt)
    : status.message;

  els.loggedOutView.classList.toggle("is-hidden", Boolean(user));
  els.loggedInView.classList.toggle("is-hidden", !user);

  // Render the progress bar
  renderProgress(status);

  if (user) {
    els.topbarSubtitle.textContent = user.username + " connected";
    els.heroTitle.textContent =
      (snapshot.roots.length || "No") + " folder" + (snapshot.roots.length === 1 ? "" : "s") + " selected";
    els.heroText.textContent = snapshot.roots.length
      ? "MyCloud Sync is running in the background and watching your selected folders."
      : "You are connected. Add a folder to start watching local changes and syncing them automatically.";
    els.primaryCta.textContent = snapshot.roots.length ? "Sync now" : "Add folder";
    els.accountUsername.textContent = user.username;
    els.accountEmail.textContent = user.email || "Signed in account";
    els.accountApiUrl.textContent = snapshot.apiBaseUrl || "http://localhost:8080";
  } else {
    els.topbarSubtitle.textContent = "Not connected";
    els.heroTitle.textContent = "Sign in to start syncing";
    els.heroText.textContent =
      "Connect the desktop app to your MyCloud account and keep selected folders synced in the background.";
    els.primaryCta.textContent = "Sign in";
    els.accountUsername.textContent = "-";
    els.accountEmail.textContent = "-";
    els.accountApiUrl.textContent = "-";
  }

  els.openFolderBtn.disabled = snapshot.roots.length === 0;
  els.viewOnlineBtn.disabled = !user;
  els.pickFolderBtn.disabled = !user;
  els.syncNowBtn.disabled = !user || status.state === "syncing";

  renderFolders(snapshot);
}

/* ---- Event listeners ---- */

els.settingsToggle.addEventListener("click", () => toggleSettings());

els.primaryCta.addEventListener("click", async () => {
  if (!currentState?.auth?.user) {
    toggleSettings(true);
    els.email.focus();
    return;
  }
  if (!currentState.roots.length) {
    try {
      const folderPath = await window.desktopApi.pickFolder();
      if (folderPath) {
        await window.desktopApi.addFolder(folderPath);
      }
    } catch (error) {
      alert(error.message || "Failed to add folder");
    }
    return;
  }
  try {
    await window.desktopApi.syncNow();
  } catch (error) {
    alert(error.message || "Sync failed");
  }
});

els.loginForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const form = new FormData(els.loginForm);
  try {
    await window.desktopApi.login({
      apiBaseUrl: form.get("apiBaseUrl"),
      email: form.get("email"),
      password: form.get("password"),
      allowSelfSignedTls: form.get("allowSelfSignedTls") === "on",
    });
    els.password.value = "";
    toggleSettings(false);
  } catch (error) {
    alert(error.message || "Login failed");
  }
});

els.logoutBtn.addEventListener("click", async () => {
  try {
    await window.desktopApi.logout();
    toggleSettings(true);
  } catch (error) {
    alert(error.message || "Logout failed");
  }
});

els.pickFolderBtn.addEventListener("click", async () => {
  try {
    const folderPath = await window.desktopApi.pickFolder();
    if (folderPath) {
      await window.desktopApi.addFolder(folderPath);
    }
  } catch (error) {
    alert(error.message || "Failed to add folder");
  }
});

els.syncNowBtn.addEventListener("click", async () => {
  try {
    await window.desktopApi.syncNow();
  } catch (error) {
    alert(error.message || "Sync failed");
  }
});

els.openFolderBtn.addEventListener("click", () => {
  if (!currentState?.roots?.length) return;
  window.desktopApi.openFolder(currentState.roots[0].id);
});

els.viewOnlineBtn.addEventListener("click", () => {
  window.desktopApi.openWeb();
});

/* ---- State subscription ---- */

window.desktopApi.onStateChanged((snapshot) => render(snapshot));
window.desktopApi.getState().then((snapshot) => {
  render(snapshot);
  toggleSettings(false);
});
