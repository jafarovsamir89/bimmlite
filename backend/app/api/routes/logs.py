from __future__ import annotations

from fastapi import APIRouter, Depends, WebSocket, WebSocketDisconnect
from sqlalchemy import select
from sqlalchemy.orm import Session

from app.core.db import get_db
from app.models import LogEntry

router = APIRouter(tags=["logs"])


@router.get("/api/logs")
async def recent_logs(limit: int = 100, db: Session = Depends(get_db)) -> list[dict[str, object]]:
    stmt = select(LogEntry).order_by(LogEntry.id.desc()).limit(limit)
    rows = db.execute(stmt).scalars().all()
    return [
        {
            "ts": row.ts.isoformat(),
            "level": row.level,
            "module": row.module,
            "event": row.event,
            "trace_id": row.trace_id,
            "session_id": row.session_id,
            "user_id": row.user_id,
            "vin": row.vin,
            "ecu": row.ecu,
            "duration_ms": row.duration_ms,
            "payload_hex": row.payload_hex,
            "result": row.result,
            "error": row.error,
            "message": row.message,
        }
        for row in rows
    ]


@router.websocket("/ws/logs")
async def live_logs(websocket: WebSocket) -> None:
    hub = websocket.app.state.telemetry.hub
    await hub.connect(websocket)
    try:
        while True:
            await websocket.receive_text()
    except WebSocketDisconnect:
        hub.disconnect(websocket)
