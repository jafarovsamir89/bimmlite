# Shared Contract

This package holds the canonical WebSocket envelope contract for BimmLite.

- `protocol.schema.json` is the source of truth for the message envelope shape.
- `src/protocol.ts` exposes TypeScript types and helpers for frontend and bridge code.

Phase 0 keeps the contract intentionally small:

- `auth`
- `command`
- `frame`
- `result`
- `log`
- `heartbeat`
