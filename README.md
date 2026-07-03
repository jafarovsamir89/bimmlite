# BimmLite

BimmLite is a server-native BMW F/G-series diagnostics and coding platform built around a thin desktop bridge and a smart backend.

Phase 0 establishes the foundation only:

- FastAPI backend with PostgreSQL, Alembic, structured logging, and trace propagation
- Go bridge skeleton over WebSocket + TLS with session-token auth
- Shared protocol contract for backend-to-bridge messages
- React + TypeScript + Vite + Tailwind frontend shell with live log viewer
- Docker and CI minimum for local development

Scope boundaries for Phase 0:

- No vehicle business logic
- No UDS coding or flashing logic
- No RSA or coding keys on the bridge or client
- No duplicated sources of truth for protocol, services, or schema definitions

## Repository Layout

- `backend/` FastAPI application, database models, logging, and migrations
- `bridge/` Go desktop bridge skeleton
- `frontend/` Vite React UI shell
- `shared/` protocol contract and message envelopes
- `data/` static data placeholders
- `docs/` product and architecture documentation

## Quick Start

1. Copy `.env.example` to `.env` and adjust values.
2. Start the database and backend stack:

```powershell
docker compose up --build
```

3. Run the frontend locally:

```powershell
cd frontend
npm install
npm run dev
```

## Phase 0 Goal

The `ping` operation should travel through the stack with one shared `trace_id`:

`UI -> backend -> bridge -> backend -> UI`

Logs are structured JSON and are visible in the UI live viewer.
