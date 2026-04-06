# MyCloud Backend

The backend is a Drogon-based C++ API server that powers authentication, file storage, folders, shares, trash, search, setup, and admin operations for MyCloud.

## Responsibilities

- initial setup flow
- login, refresh, logout, and password change
- file upload, download, preview, rename, move, delete, and starring
- folder list, create, rename, move, and delete
- public share link creation and resolution
- trash and restore workflows
- search and admin endpoints

Main entrypoint: [main.cpp](/C:/Users/omrio/Desktop/Projects/mycloud/backend/src/main.cpp)

## Tech stack

- C++26
- Drogon
- OpenSSL
- MariaDB
- hiredis
- jwt-cpp

## Project structure

```text
backend/
├─ config/
├─ migrations/
├─ src/
│  ├─ controllers/
│  ├─ middleware/
│  ├─ services/
│  └─ utils/
├─ CMakeLists.txt
└─ Dockerfile
```

## API surface

Implemented controller areas:

- `SetupController`
- `AuthController`
- `FileController`
- `FolderController`
- `ShareController`
- `TrashController`
- `SearchController`
- `AdminController`

## Runtime configuration

The backend reads configuration from either direct env vars or matching `*_FILE` env vars.

Supported values visible in [main.cpp](/C:/Users/omrio/Desktop/Projects/mycloud/backend/src/main.cpp#L35):

- `DB_HOST`
- `DB_PORT`
- `DB_NAME`
- `DB_USER`
- `DB_PASSWORD` or `DB_PASSWORD_FILE`
- `JWT_SECRET` or `JWT_SECRET_FILE`
- `ALLOWED_ORIGINS`

Other values used by the Docker setup:

- `MASTER_ENCRYPTION_KEY` or `MASTER_ENCRYPTION_KEY_FILE`
- `STORAGE_PATH`
- `MYCLOUD_UPDATE_COMMAND` for the admin apply command
- `MYCLOUD_UPDATE_LOG_PATH` to control where detached update command output is written
- `MYCLOUD_UPDATE_SERVICES` to control which compose services are rebuilt on apply

Default listener:

- `0.0.0.0:8080`

## Docker

The repository Docker flow builds and runs the backend through [backend/Dockerfile](/C:/Users/omrio/Desktop/Projects/mycloud/backend/Dockerfile).

From the repo root:

```bash
docker compose up --build backend mariadb redis init
```

Or start the full stack:

```bash
docker compose up --build
```

## Local build

You need Drogon and the native dependencies installed on your machine first.

Build with CMake:

```bash
cd backend
cmake -S . -B build -DCMAKE_BUILD_TYPE=Release
cmake --build build
```

Run the compiled binary:

```bash
./build/mycloud_backend
```

On Windows without WSL or a Linux-like toolchain, Docker is the more practical path.

## Database

MariaDB migrations live in:

- [001_initial.sql](/C:/Users/omrio/Desktop/Projects/mycloud/backend/migrations/001_initial.sql)
- [002_performance_indexes.sql](/C:/Users/omrio/Desktop/Projects/mycloud/backend/migrations/002_performance_indexes.sql)

In Docker Compose, these are mounted into MariaDB automatically.

## Security model

- access and refresh token auth
- JWT verification on protected `/api/*` routes
- admin-only guard on `/api/admin/*`
- CORS allowlist via `ALLOWED_ORIGINS`
- encrypted file storage with backend-side key management

## Notes

- public share routes under `/api/s/*` are intentionally unauthenticated
- uploads allow large bodies and stream through the backend
- the backend assumes MariaDB and local file storage are available
- one-click admin updates in the stock Docker deployment depend on the host repo being mounted at
  `/opt/mycloud` and the Docker socket being mounted into the backend container
