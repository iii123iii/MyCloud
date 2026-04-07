# MyCloud Go Backend

The Go backend replaces the original Drogon/C++ service with a versioned `/api/v2` API, MariaDB persistence, Redis-backed auth helpers, and constant-memory encrypted file streaming.

## Stack

- Go
- `net/http`
- `chi`
- MariaDB
- Redis
- JWT auth
- local encrypted file storage

## Run locally

```bash
cd backend-go
go run ./cmd/api
```

Required environment variables match the Docker stack:

- `DB_HOST`
- `DB_PORT`
- `DB_NAME`
- `DB_USER`
- `DB_PASSWORD` or `DB_PASSWORD_FILE`
- `JWT_SECRET` or `JWT_SECRET_FILE`
- `MASTER_ENCRYPTION_KEY` or `MASTER_ENCRYPTION_KEY_FILE`
- `STORAGE_PATH`
- `ALLOWED_ORIGINS`

## Docker

From repo root:

```bash
docker compose up --build backend mariadb redis init
```
