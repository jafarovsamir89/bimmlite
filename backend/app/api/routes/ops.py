from __future__ import annotations

from fastapi import APIRouter, Depends, Request
from fastapi import HTTPException
from sqlalchemy.orm import Session

from app.core.db import get_db
from app.core.trace import current_session_id, current_trace_id
from app.schemas import PingResponse
from app.services.telemetry import TelemetryService

router = APIRouter(prefix="/api/ops", tags=["ops"])


@router.post("/ping", response_model=PingResponse)
async def ping(request: Request, db: Session = Depends(get_db)) -> PingResponse:
    telemetry: TelemetryService = request.app.state.telemetry
    bridge = request.app.state.bridge_manager
    trace_id = current_trace_id()
    session_id = current_session_id()

    await telemetry.emit(
        db,
        level="info",
        module="api",
        event="ping.start",
        trace_id=trace_id,
        session_id=session_id,
        message="ping request received",
    )

    try:
        result = await bridge.send_command(command="ping", payload={"message": "ping"}, trace_id=trace_id, session_id=session_id)
    except RuntimeError as exc:
        await telemetry.emit(
            db,
            level="error",
            module="api",
            event="ping.bridge_unavailable",
            trace_id=trace_id,
            session_id=session_id,
            error=str(exc),
            message="bridge unavailable",
        )
        raise HTTPException(status_code=503, detail="bridge unavailable") from exc

    await telemetry.emit(
        db,
        level="info",
        module="api",
        event="ping.complete",
        trace_id=trace_id,
        session_id=session_id,
        result=str(result.get("payload", {}).get("result", "pong")),
        message="ping request completed",
    )

    return PingResponse(
        trace_id=trace_id,
        result=str(result.get("payload", {}).get("result", "pong")),
        bridge_connected=bridge.connected,
    )
