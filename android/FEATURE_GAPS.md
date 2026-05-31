# Android ⟷ Web feature-parity gaps

Consolidated from a sweep of the Next.js `frontend/` vs the Android app. Grouped by
area, roughly prioritized. "Done this pass" notes what was just added.

## Search (entirely missing)
- No global search UI at all. Web has a ⌘K command palette + in-folder search.
- Backend `GET /api/v2/search?q&scope=name|content|both&folder_id` supports a DSL
  (`name:*.pdf`, `tag:`, `mime:image/*`, `size:>1mb`, `after:`, `owner:me`, `starred:`),
  fuzzy ranking, folder-scoped search. **Need a `SearchApi` + search screen.**

## Tags & smart folders (entirely missing)
- Tags CRUD + colors, attach/detach to files/folders, filter-by-tag. (`/api/v2/tags`, file/folder tag attach/detach)
- Smart folders (saved searches) with a query builder + results. (`/api/v2/smart-folders`)
- Sidebar tag/smart-folder groups.

## File browser
- **Context menu / per-item actions** (web right-click ≈ Android per-item overflow ⋮).
- **Move + Copy — DONE this pass**: selection-bar overflow → full-screen `FolderPicker`
  (navigate, new-folder, source folders greyed out). Files use `PATCH /files/{id}`
  with an explicit `{"folder_id":null}` for root / `:copy`; folders use `PATCH /folders/{id}`
  `{"parent_id":…}` / `:copy`. Raw `JsonObject` bodies because the global Json drops nulls.
- **Folder + mixed-selection zip download — DONE**: selecting any folder makes the selection-bar Download stream a single server-built zip (`POST /files:download-archive`) via a new `ArchiveDownloadWorker`; files-only selections keep the per-file downloads.
- Shift/range select; sort-direction toggle per column (only 3 preset sorts now).
- Inline rename (we have a dialog), drag-and-drop upload of folder trees.

## Sharing
- **Public-link create was broken** — `POST /shares` replies `{token,url}` (no id) but `ShareDto.id` was required, so every share failed with a bogus "can't reach the server". Fixed (id now defaults). **DONE this pass.**
- **Per-user grant creation — DONE this pass**: ShareDialog now has a "Share with a person" section (username/email + Viewer/Editor). Also fixed the grant DTOs — backend wants `grantee` (username/email) + `viewer|editor|owner` and returns `grantee_id`/`grantee_name`, which didn't match the old DTOs (so grant list/create were silently failing too). Still missing: edit permission (`PATCH /shares/grants/{id}`).
- **Shared-with-me**: browse incoming files/folders, download, copy-to-my-drive, leave. (`?shared_with_me=1`, grants `direction=incoming`)
- **Upload requests** (guest upload links): CRUD + copy. (`/api/v2/upload-requests`)
- Public link: `single_view` missing from the domain model; expired-links view; show password/expiry/limit badges.
- Public (unauthenticated) link-consumer screen + deep links (`/s/{token}`, `/u/{token}`).

## Photos
- **Date filter** (presets + custom range) — `PhotoApi` already accepts `from`/`to`, just unused.
- Day-level sub-grouping for dense months.
- Lightbox: download, filename + "x of y", filmstrip, swipe between photos, chrome auto-hide. **(zoom/pan — done this pass.)**

## Preview / viewers
- **Image zoom/pan — DONE this pass** (`ZoomableImage`).
- **PDF viewer — DONE this pass** (`PdfViewer` via `PdfRenderer`).
- Still missing: **video (HLS via Media3)**, audio, **text/code viewer**, CSV/spreadsheet, Word/office; version-aware preview; download/print from preview; prev/next file navigation; image annotation / text editing.

## Collaboration (file detail)
- Comment **edit** — **DONE this pass** (`PATCH /comments/{id}`; edit icon → dialog).
- Version **download** + **preview** + diff (we have list/restore). (`/versions/{vno}:download|:preview`)
- **File locks** (acquire/release/list). (`/files/{id}/lock`)
- **Retention policies** on folders. (`/folders/{id}/retention`)
- Ownership transfer (`/files/{id}:transfer`); per-folder activity feed (`/folders/{id}/activity`).

## Settings / developer
- **Voluntary change-password** entry in Settings — **DONE this pass** (Security section → dialog).
- **Delete account** (`DELETE /auth/account`) — **DONE this pass** (Danger zone → password-confirm dialog → logout).
- PAT create: **scope picker + expiry + presets** (currently created with empty scopes); token rename; last-used/IP display.
- **Webhooks** UI — entire feature missing: list/create/edit/delete/test/deliveries/rotate-secret. (`/me/webhooks*`)
- Activity log pagination (capped at 20); session detail (last-seen/expiry, "this device" emphasis); profile role + member-since.

## Navigation surfaces
- **Recent** view (`?all=1&sort=updated_at&order=desc`), **Starred** view (`?starred_only=1`).
- **Favorites** (pinned folders) + reorder. (`/me/favorites`)
- Sidebar **storage card**; **admin panel** link (role-gated).

## Transfers / admin / global
- **Archive (zip) download** (`:download-archive`) — **DONE** (see File browser). Still missing: **presigned links** (`:presign`).
- **Resumable tus upload** (current upload is one-shot multipart — no resume).
- In-app **upload queue panel** with per-file cancel/retry (we have notifications + a transfers sheet).
- **Admin panel** entirely missing: users CRUD, stats, logs, settings, in-app updates. (`/api/v2/admin/*`)
- Manual **dark-mode toggle** — **DONE this pass** (Settings → Appearance: System/Light/Dark, persisted in `SettingsStore`, applied in `MainActivity`).

---
## Bugs found by the 10-agent audit (2026-05-30)

**Fixed this pass (high-confidence, verified against backend):**
- **Token refresh was 100% broken** → forced logout at every ~15-min access-token expiry. `AuthTokensDto.username`/`email` were required but `POST /auth/refresh` omits them. Defaulted them. (This was the real cause of the "session expires and doesn't refresh" report.)
- **Paging refetch loop** — `GET /files` sends `next_cursor:""` (not null) at end-of-list; the mediator looped re-fetching page 1 and duplicating rows. Now treats blank as terminal.
- **Realtime auto-refresh was dead** — hub sends `type:"event"` but the client filtered for `file.*`/`folder.*`. Now matches `"event"`.
- **Grants "People" tab inverted** — `listGrants` sent no `direction`, so backend returned *incoming* grants under outgoing UI. Now sends `direction=outgoing`.
- **Move/Copy failures were silent** — now surfaced via snackbar (`BrowserViewModel.messages`).
- Proactive-refresh guard hardened against a 0/unknown expiry (was a potential refresh-every-request storm).

**Fixed in the UX/copy round:**
- **Folder copy 500 (backend)** — `copyFolderInto` inserted the copied subtree in Go map order, intermittently inserting a child before its new parent and violating `fk_folders_parent` → non-deterministic 500. Now inserts parents-first (BFS). *(backend-go/internal/app/handlers_copy.go — redeploy backend.)*
- **Removed "Use an access token instead"** from the login screen.
- **Auto-fetch on sign-in** — `BrowserViewModel` (Activity-scoped, survives sign-out) now resets to root + reloads when the signed-in user changes; `BrowserScreen` also refreshes paging on entry. No manual pull needed after login.
- **Pull-to-refresh everywhere** — added to Trash and Photos (Browser + Shared already had it); all data screens also refresh on (re)entry.

**Fixed in the follow-up round (cache-privacy + bug batch):**
- **Cache cleared on sign-out + encrypted at rest.** Room cache is now SQLCipher-encrypted (`DbKeyProvider` — a Keystore-wrapped random passphrase via Tink; DB renamed `mycloud-enc.db`). A new `SessionCacheCleaner` wipes Room + Coil (memory/disk) + the `previews` dir on logout, on involuntary sign-out (`onRefreshFailed`/`restoreSession` 401), and when a *different* user signs in (`SettingsStore.cachedUserId` guard in `login()`).
- **File detail stale state / PDF race / cache collision** — `load()` now resets state + cancels the prior `pdfJob` and ignores a late download; `downloadToCache` namespaces by file id.
- **SharedScreen** — loading (pull-to-refresh) + error state (distinct from empty) + refresh-on-entry.
- **Comments** — `editable` flag added (DTO/model/mapper); Edit/Delete gated on it; 4000-char limit enforced client-side.
- **Share double-tap** — `create()`/`addPerson()` guarded with `AtomicBoolean.compareAndSet`.

**Still open (lower priority / larger change):**
- `FileDetailViewModel`/`ShareViewModel` are still Activity-scoped (AppShell uses `hiltViewModel()` not a NavHost). The stale-state symptoms are fixed via explicit reset/guards, but a NavHost migration would be the cleaner root fix. Note: mobile gates comment delete on `editable` (author-only); a *file owner* can't delete others' comments from mobile (backend allows it) — minor gap.
- Share `created_at`/`expires_at` may not parse (`Instant.parse` vs MySQL space-separated DATETIME) — verify and add a tolerant parser.
- Upload progress stuck at 0% for unknown-size (`SIZE`-less) content URIs; terminal WorkManager transfers never pruned (sheet + notifications grow unbounded); download filename collisions overwrite.
- `single_view`/password-protected links not badged in the share list; `restoreSession` lets non-transient 4xx through as AUTHENTICATED.

**Done in earlier passes:** zoomable image preview + lightbox; PDF viewer; authenticated download-to-cache.
**Done in this pass:** Move/Copy (+ folder picker); dark-mode toggle; voluntary change-password + delete-account in Settings; comment edit; proactive token refresh (no visible 15-min lapse); fixed stuck sign-out; uniform folder/file grid tiles.
**Highest-impact next:** Recent/Starred/Favorites views · Tags/Smart-folders · Photos date filter + lightbox · Version download/preview · Sharing grants + shared-with-me + upload requests · Admin panel · Video (Media3 HLS) preview.
