# MyCloud

MyCloud is a self-hosted cloud storage stack with three main surfaces:

- `backend-go/`: the active Go API server
- `frontend/`: a Next.js web app for browsing and managing files
- `desktop/`: an Electron tray app for background folder sync from Windows

The repository also includes:

- `docker-compose.yml` for the full local stack
- `init/` for one-time secret generation
- `nginx/` for TLS termination and reverse proxying
- MariaDB and Redis as supporting services

## Repo Layout

```text
.
├─ backend-go/  Go API server
├─ desktop/     Electron desktop sync client
├─ frontend/    Next.js web app
├─ init/        Secret generation container
├─ nginx/       Reverse proxy and TLS config
└─ docker-compose.yml
```

## What The Stack Does

- user setup and authentication
- encrypted file upload and download
- folder creation, rename, move, and delete
- public share links
- trash and restore flows
- admin settings and basic admin views
- desktop background sync from local folders into the server

## Quick Start With Docker Compose

```bash
sh ./scripts/compose-up.sh up --build
```

Services started by compose:

- `init`: generates secrets into a named Docker volume on first boot
- `mariadb`: primary relational database
- `redis`: supporting cache/store used by the backend stack
- `backend`: Go API server on container port `8080`
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

See [`.env.example`](/C:/Users/omrio/Desktop/Projects/mycloud/.env.example) for the supported overrides:

- `DB_ROOT_PASSWORD`
- `DB_PASSWORD`
- `JWT_SECRET`
- `MASTER_ENCRYPTION_KEY`

## Running Apps Individually

Each app has its own README:

- [`backend-go/README.md`](/C:/Users/omrio/Desktop/Projects/mycloud/backend-go/README.md)
- [`frontend/README.md`](/C:/Users/omrio/Desktop/Projects/mycloud/frontend/README.md)
- [`desktop/README.md`](/C:/Users/omrio/Desktop/Projects/mycloud/desktop/README.md)

## Versioning And Updates

For Docker deployments, the backend version is resolved from the Git tag on the checked-out commit. The Compose wrappers and updater scripts export `MYCLOUD_VERSION` automatically before they build.

### How To Release A New Version

1. Commit and push the code.
2. Create and push the Git tag you want to ship, then create the matching GitHub release.
3. Deploy with `sh ./scripts/compose-up.sh up -d --build backend frontend nginx`.

On Windows PowerShell, use `./scripts/compose-up.ps1 up -d --build backend frontend nginx`.

### Admin Panel One-Click Updates

The backend exposes:

- `GET /api/v2/admin/updates/check`
- `POST /api/v2/admin/updates/apply`

The default stack still uses the dedicated `updater` container for the apply step.

## Development Notes

- backend-go: Go, chi, MariaDB, Redis
- frontend: Next.js 16, React 19, SWR, Tailwind CSS 4
- desktop: Electron, TypeScript, chokidar, undici

## Verification

Useful commands:

```bash
cd backend-go && go test ./...
cd frontend && npm run build
cd desktop && npm run build
```
