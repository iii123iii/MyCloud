# MyCloud — Android client

Native Android app (Jetpack Compose + **Material 3 Expressive**) for the MyCloud
backend (`/api/v2`). Online client with smart caching; email/password + biometric
auth. See the full design in `../.claude/plans/plan-a-jetpack-compose-*.md`.

## Status

Phase 0 (scaffolding) is in place:

- Multi-module Gradle project (`settings.gradle.kts`, version catalog in `gradle/libs.versions.toml`).
- `:app` — Hilt `Application`, splash, edge-to-edge, `MyCloudTheme`, launcher icon.
- `:core:designsystem` — brand-seeded Material 3 Expressive theme (`MyCloudTheme`) + preview gallery.
- `:core:model` / `:core:common` — domain models + `NetworkResult` and utilities.
- `:core:network` — `Envelope<T>`, `safeApiCall`, Retrofit/OkHttp/kotlinx.serialization wiring, `AuthApi`/`TokenRefreshApi`, auth interceptor + single-flight `TokenAuthenticator`.

Phase 1 (auth spine + app shell):

- `:core:datastore` — Tink/Keystore `SecureTokenStore`, `SettingsStore`, `AppLockManager`.
- `:data` — `AuthRepository`, `TokenProviderImpl` (bound into the OkHttp authenticator), `AuthStateHolder`, mappers.
- `:feature:auth` — Login, Change Password, and biometric App-Lock screens (MVVM + UDF state).
- `:core:database` — Room (`FileEntity`/`FolderEntity`/`RemoteKeyEntity` + DAOs), schema export, destructive-fallback for now.
- `:data` — `FileRepository` (Paging 3 `Pager` + `FileRemoteMediator` over Room), `FolderRepository`, `StorageRepository`, mappers.
- `:feature:browser` — file browser (breadcrumb, grid/list toggle, sort menu, pull-to-refresh, multi-select floating bar, new-folder FAB) with authenticated Coil thumbnails.
- `:app` — `RootViewModel` + auth gate (Loading→splash, LoggedOut→Login, NeedsPasswordChange→Change Password, Locked→App-Lock, Authenticated→shell); the `NavigationSuiteScaffold` shell (My Files → browser; Photos/Shared/Trash/Settings placeholders); a singleton Coil `ImageLoader` on the authenticated OkHttp client.

**Phase 1 is functionally complete** (a small file-detail / full-screen preview screen is a follow-up — `onOpenFile` is currently a stub; tapping is wired but opens nothing yet).

Phase 2 (transfers):

- `:core:work` — `UploadWorker` (multipart via a progress-aware `UriRequestBody`), `DownloadWorker` (streams to the app's Downloads dir, shareable via FileProvider), `TransferNotifications` (foreground + progress), `TransferManager` (enqueue + observe).
- `:app` — WorkManager via `Configuration.Provider` + `HiltWorkerFactory`; default initializer disabled; `dataSync` foreground-service type + permissions; FileProvider; runtime `POST_NOTIFICATIONS`.
- `:data` / `:feature:browser` — `TransferRepository`; the browser FAB now offers **Upload files** (SAF picker) + **New folder**, the selection bar adds **Download**, and a top-bar action opens a **Transfers** bottom sheet with live progress.

**Phase 2 status:** uploads/downloads + transfers UI are functional. Deferred follow-ups: tus *resumable* upload (currently multipart), MediaStore/SAF download destinations (currently app Downloads dir), and Media3/HLS + PDF viewers.

Phase 3 (sharing) + Phase 4 (trash + settings):

- `:core:network` / `:data` — `ShareApi` + `TrashApi`; `ShareRepository` (public links + grants), `TrashRepository`.
- `:feature:sharing` — `ShareDialog` (create a public link with permission/password/expiry/limit/single-view → copy) + `SharedScreen` (Links/People tabs with revoke). Triggered from the browser's selection bar (single-select **Share**), orchestrated by `:app`.
- `:feature:trash` — `TrashScreen` (restore, delete-forever, empty).
- `:app` — real `SettingsScreen` (account + storage quota bar + sign out); the shell's **Shared / Trash / Settings** destinations now render real screens.

**Phase 3/4 status:** public share links, grants list/revoke, trash management, and settings/storage are functional. Deferred: upload-requests, the public (unauthenticated) link-consumer deep link, shared-with-me browsing, and per-user grant *creation* UI.

Phase 5 (photos) + Phase 6 (collaboration):

- `:core:network` / `:data` — `PhotoApi`, `CommentApi`, `VersionApi`; `PhotoRepository`, `CommentRepository`, `VersionRepository`.
- `:feature:photos` — `PhotosScreen`: month-grouped grid (authenticated Coil thumbnails) + full-screen lightbox. Fills the last shell placeholder.
- `:feature:filedetail` — `FileDetailScreen`: image preview, file info, version history (restore), and comments (add/delete). Opened by tapping a file in the browser (`onOpenFile`), shown as a full-screen overlay orchestrated by `:app`.

**Phase 5/6 status:** photos timeline + lightbox, file preview, versions, and comments are functional. All five shell destinations are now real screens. Deferred: file locks, version download/diff, ownership transfer, per-folder activity.

Phase 7 (sessions/activity/developer) + Phase 8 (realtime):

- `:core:network` / `:data` — `AccountApi` + `AccountRepository` (sessions, activity log, personal access tokens); `RealtimeClient` (OkHttp WebSocket, in-band bare-token auth) + `RealtimeRepository` (connection lifecycle + a coarse "data changed" signal, with reconnect/backoff).
- `:app` — `SettingsScreen` expanded into sections: Account, Storage, **Sessions** (list + revoke), **Recent activity**, **Access tokens** (create dialog with one-time secret reveal + revoke), Sign out. `RootViewModel` holds the realtime socket open only while authenticated.
- `:feature:browser` — collects realtime change events to refresh the folder listing + Paging.

**Phase 7/8 status:** session management, activity log, PAT create/revoke, and realtime-driven list refresh are functional.

**All eight plan phases are now implemented** (0–8). Remaining cross-cutting / deferred items: organization extras (tags / smart folders / favorites / starred), webhooks UI, upload-requests + public link consumer, tus *resumable* upload, Media3/HLS video + PDF viewers, real Room migrations, the build-logic convention plugins, and the test suite. None are blocking — they're enhancements on a complete spine.

## Prerequisites

- **Android Studio** (latest stable) with the Android **SDK 36** installed. Android Studio's bundled JBR (JDK 17+) is used — no separate JDK needed.
- No global Gradle required; the project uses the Gradle wrapper.

## Open & run

1. Open the **`android/`** folder (not the repo root) in Android Studio.
2. On first sync, Android Studio generates the Gradle wrapper `.jar` (only `gradle-wrapper.properties` + scripts are committed). If you build from the CLI first, run `gradle wrapper` once to create it.
3. `local.properties` already points `sdk.dir` at `C:/Users/omrio/AppData/Local/Android/Sdk`; adjust if your SDK differs (this file is git-ignored).
4. Run the `app` configuration on an emulator/device.

> **Version sync:** the catalog pins plausible stable versions as of early 2026. Open `gradle/libs.versions.toml` and let Android Studio's inspections bump to the latest. Keep `material3` on a **1.4.x** line (Material 3 Expressive lives there).

## Backend URL

The server address is **entered on the login screen** and persisted (`ServerConfig` in `:core:datastore`). A `HostSelectionInterceptor` rewrites every request (and Coil image load) to the chosen host at runtime, so it can change without rebuilding Retrofit. It defaults to `http://10.0.2.2:8080/` (emulator host-loopback); the debug manifest allows cleartext HTTP for local dev. `BuildConfig.BASE_URL` now only serves as the Retrofit placeholder base.

Run the backend locally with the repo's `docker-compose.yml`. If CORS rejects, add the dev origin to `ALLOWED_ORIGINS`.

## Notes

- **Build-logic convention plugins** are intentionally deferred — module `build.gradle.kts` files configure AGP/Kotlin directly for now. Extract a `build-logic/` with `mycloud.android.*` convention plugins once the module count grows.
- Modules are added to `settings.gradle.kts` as they're created; keep that list in sync with directories that contain a `build.gradle.kts`.
