; Custom NSIS hooks for MyCloud Sync.
;
; Electron-builder's stock uninstaller removes only the installed program
; files; it deliberately leaves per-user data behind. For a sync client that
; persists encrypted auth tokens, mapping state, Chromium caches, crash
; dumps and event logs, this surprises users — uninstall should clean up.
;
; The customUnInstall macro fires after the standard uninstall steps, so by
; the time it runs the app exe is gone and the user-data folders are free to
; delete. We clear both:
;   %APPDATA%\<app>         — Electron's userData (state.json, Cookies,
;                             Local Storage, tus fingerprints, logs)
;   %LOCALAPPDATA%\<app>    — Crashpad / cache that some Electron versions
;                             place under LocalAppData
;
; The folder name matches Electron's app.getName(), which derives from the
; package.json top-level `name` field ("mycloud-desktop") since we don't set a
; top-level productName ("MyCloud Sync" only appears under build.productName
; for electron-builder's installer copy, not Electron's runtime name).

!macro customUnInstall
  RMDir /r "$APPDATA\mycloud-desktop"
  RMDir /r "$LOCALAPPDATA\mycloud-desktop"
!macroend
