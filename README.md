# MyCloud

MyCloud is a self-hosted cloud storage stack with three user-facing surfaces:

- `backend/`: a Drogon-based C++ API for auth, files, folders, shares, search, trash, and admin operations
- `frontend/`: a Next.js web app for browsing and managing files
- `desktop/`: an Electron tray app for background folder sync from Windows

The repository also includes:

- `docker-compose.yml` for the full local stack
- `init/` for one-time secret generation
- `nginx/` for TLS termination and reverse proxying
- MariaDB and Redis as supporting services

## Repo layout

```text
.
├─ backend/     API server
├─ desktop/     Electron desktop sync client
├─ frontend/    Next.js web app
├─ init/        Secret generation container
├─ nginx/       Reverse proxy and TLS config
└─ docker-compose.yml
```

## What the stack does

- user setup and authentication
- encrypted file upload and download
- folder creation, rename, move, and delete
- public share links
- trash and restore flows
- admin settings and basic admin views
- desktop background sync from local folders into the server

## Quick start with Docker Compose

This is the intended easiest way to run the whole project locally.

```bash
docker compose up --build
```

Services started by compose:

- `init`: generates secrets into a named Docker volume on first boot
- `mariadb`: primary relational database
- `redis`: supporting cache/store used by the backend stack
- `backend`: API server on container port `8080`
- `frontend`: Next.js app on container port `3000`
- `nginx`: public entrypoint on `http://localhost` and `https://localhost`

Default external ports:

- `80` -> nginx HTTP
- `443` -> nginx HTTPS

The compose file is wired so:

- the backend reads secrets from `/run/secrets/*`
- uploaded file content is stored in the `file_storage` volume
- SQL migrations are mounted into MariaDB at startup
- the frontend talks to the backend through `NEXT_PUBLIC_API_URL=https://localhost`

## Configuration

`.env` is optional. The stack auto-generates secrets on first boot through `init/`.

See [.env.example](/C:/Users/omrio/Desktop/Projects/mycloud/.env.example) for the supported overrides:

- `DB_ROOT_PASSWORD`
- `DB_PASSWORD`
- `JWT_SECRET`
- `MASTER_ENCRYPTION_KEY`

## Running apps individually

Each app has its own README:

- [backend/README.md](/C:/Users/omrio/Desktop/Projects/mycloud/backend/README.md)
- [frontend/README.md](/C:/Users/omrio/Desktop/Projects/mycloud/frontend/README.md)
- [desktop/README.md](/C:/Users/omrio/Desktop/Projects/mycloud/desktop/README.md)

In practice:

- run the backend if you need the API only
- run the frontend if you are working on the web UI
- run the desktop app if you are working on sync UX or file watching

## Versioning and updates

MyCloud keeps a default version string in the backend source, but Docker-based update builds can
override it from the latest reachable git tag so the running binary matches the deployed release.

### How to release a new version

1. **Set the release version source:**
   - Preferred for Docker deployments: create and push the git tag you want to ship.
   - Fallback/default: edit `backend/src/utils/Version.h` and change `MYCLOUD_VERSION` to the new tag:
   ```cpp
   #define MYCLOUD_VERSION "v1.1.0"
   ```
   The value must exactly match the GitHub release tag you will create (including the leading `v`).

2. **Commit and push the code:**
   ```bash
   git add .
   git commit -am "chore: bump version to v1.1.0"
   git push origin main
   ```

3. **Create a GitHub release** at `https://github.com/iii123iii/MyCloud/releases/new` tagged exactly `v1.1.0`. Add release notes â€” they appear in the admin update panel.

4. **Create a GitHub release** at `https://github.com/iii123iii/MyCloud/releases/new` tagged exactly `v1.1.0`. Add release notes — they appear in the admin update panel.

5. **Deploy:**
   - If using Watchtower, it will pick up the new image automatically within the configured interval.
   - If you have explicitly enabled admin apply support: go to **Admin → Updates**, click
     **Check for updates**, then **Update to v1.1.0**.

### Admin panel one-click updates

The backend exposes two admin-only endpoints:

| Endpoint | Purpose |
|---|---|
| `GET /api/admin/updates/check` | Compare running version against latest GitHub release |
| `POST /api/admin/updates/apply` | Pull the latest git checkout and rebuild the app |

### How one-click apply works

The default `docker-compose.yml` now enables one-click apply for the standard repo-based deployment.

When an admin clicks **Apply update**, the backend calls the dedicated `updater` container, which runs this flow:

```bash
cd /opt/mycloud
git fetch --tags
git pull --ff-only
docker compose build backend frontend nginx
docker compose up -d backend frontend nginx
```

The updater derives `MYCLOUD_VERSION` from the latest reachable git tag and passes it into the backend build.

This works because the updater container has:

- the repo checkout mounted at `/opt/mycloud`
- the host Docker socket mounted at `/var/run/docker.sock`
- `git`, `docker`, `docker compose`, and `mysql` installed in the updater image

### Deployment requirements

For the button to work reliably, deploy the app as a real git checkout of this repository on the
server. The updater modifies that checkout in place.

If the server directory is not a git clone, or if the working tree has local uncommitted changes,
the apply step will fail and the running containers will stay on the current version.

### Manual host update

You can still update manually with the same flow:

```bash
git pull --ff-only
docker compose up -d --build backend frontend nginx
```

### Optional overrides

`docker-compose.yml` still exposes these knobs if you want to customize the apply flow:

- `MYCLOUD_UPDATER_URL`
- `MYCLOUD_UPDATE_COMMAND`
- `MYCLOUD_UPDATE_LOG_PATH`
- `MYCLOUD_UPDATE_SERVICES`

## Development notes

- backend: C++26, Drogon, OpenSSL, MariaDB, hiredis
- frontend: Next.js 16, React 19, SWR, Tailwind CSS 4
- desktop: Electron, TypeScript, chokidar, undici

## Current limitations

- the desktop app currently focuses on local-to-remote sync rather than full two-way sync
- local non-Docker backend setup requires Drogon and related native dependencies
- TLS in local development is expected through the bundled nginx setup

## Verification

Useful per-app commands:

```bash
cd frontend && npm run build
cd desktop && npm run build
```

Backend verification depends on your local C++ toolchain and Drogon installation.
