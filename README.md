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
