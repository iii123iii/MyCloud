# MyCloud Frontend

The frontend is the web UI for MyCloud. It is built with Next.js App Router and talks to the backend API for auth, file browsing, shares, trash, admin pages, and account settings.

## Tech stack

- Next.js 16
- React 19
- TypeScript
- SWR and `useSWRInfinite`
- Tailwind CSS 4
- shadcn/ui-style component primitives

## App areas

Routes currently present in `app/`:

- `/login`
- `/register`
- `/setup`
- `/dashboard`
- `/recent`
- `/starred`
- `/shared`
- `/trash`
- `/settings`
- `/admin`
- `/s/[token]`

## Local development

Install dependencies and start the dev server:

```bash
cd frontend
npm install
npm run dev
```

Open:

```text
http://localhost:3000
```

## Environment

The frontend uses:

- `NEXT_PUBLIC_API_URL`

Example:

```bash
NEXT_PUBLIC_API_URL=http://localhost:8080
```

In the Docker stack, it is set to `https://localhost` because nginx fronts the API.

## Build and run

Production build:

```bash
cd frontend
npm run build
npm run start
```

Available scripts from [package.json](/C:/Users/omrio/Desktop/Projects/mycloud/frontend/package.json):

- `npm run dev`
- `npm run build`
- `npm run start`
- `npm run lint`

## Docker

The frontend Docker image is a multi-stage build that outputs a standalone Next.js server.

Run it as part of the full stack:

```bash
docker compose up --build frontend backend nginx
```

Or run the full repo stack:

```bash
docker compose up --build
```

## Important directories

```text
frontend/
├─ app/         Route segments and pages
├─ components/  Explorer UI, sidebar, modals, shared UI primitives
├─ hooks/       Client hooks
├─ lib/         API client and shared types
└─ public/      Static assets
```

## File explorer behavior

The dashboard explorer:

- loads folders and files from the backend
- supports pagination and incremental loading
- supports upload by file picker and drag-and-drop
- supports preview, rename, move, share, and delete flows through modals and context menus

## Notes

- auth tokens are stored client-side for API calls
- the frontend expects the backend to be reachable from the browser
- image remote patterns are intentionally empty in [next.config.ts](/C:/Users/omrio/Desktop/Projects/mycloud/frontend/next.config.ts)
