# MyCloud Desktop

The desktop app is a Windows Electron client for MyCloud background sync. It signs into the existing backend API, lets the user choose local folders, watches those folders for changes, and mirrors them to the remote server.

## Responsibilities

- sign in and sign out against the MyCloud backend
- pick local folders to sync
- watch filesystem changes with `chokidar`
- queue sync work in the background
- show sync status and progress in a compact desktop UI
- stay available from the system tray

## Tech stack

- Electron
- TypeScript
- plain renderer HTML/CSS/JS
- `chokidar` for filesystem watching
- `undici` for HTTP requests

## Project structure

```text
desktop/
├─ renderer/  Desktop window HTML, CSS, and renderer logic
├─ src/       Electron main process, preload, sync engine, API client, store
├─ build/     Compiled TypeScript output
└─ dist/      Packaged installers
```

Important files:

- [src/main.ts](/C:/Users/omrio/Desktop/Projects/mycloud/desktop/src/main.ts)
- [src/lib/sync-engine.ts](/C:/Users/omrio/Desktop/Projects/mycloud/desktop/src/lib/sync-engine.ts)
- [renderer/index.html](/C:/Users/omrio/Desktop/Projects/mycloud/desktop/renderer/index.html)

## Run locally

```bash
cd desktop
npm install
npm run dev
```

Available scripts:

- `npm run build`
- `npm run watch`
- `npm run dev`
- `npm run start`
- `npm run dist`

## Typical workflow

1. launch the desktop app
2. sign in with an existing MyCloud account
3. point the app at your backend URL, for example `http://localhost:8080`
4. add one or more local folders
5. let the tray app keep syncing in the background

## Packaging

Build a Windows installer:

```bash
cd desktop
npm run dist
```

Output goes to `desktop/dist/`.

The app is configured with:

- product name: `MyCloud Sync`
- target: Windows NSIS installer

## Sync behavior

- adding a root creates one matching remote root folder
- nested directories are created automatically as needed
- changed files are uploaded
- deleted local files and folders are removed remotely
- removing a watched root updates local UI immediately and performs remote cleanup in the background queue
- closing the window hides it to the tray instead of quitting

## State and storage

- app state is persisted locally in a JSON state file under Electron user data
- auth tokens are stored in that local app state
- sync mappings are kept locally so remote IDs can be tracked across updates

## Current limitation

This client is currently optimized for local-to-remote sync. It does not yet behave like a full Dropbox-style two-way sync client that continuously pulls remote edits back down to disk.
