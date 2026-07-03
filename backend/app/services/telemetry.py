from __future__ import annotations

from datetime import datetime, timezone
from typing import Any

import structlog
from fastapi import FastAPI, WebSocket
from sqlalchemy.orm import Session

from app.core.trace import current_ecu, current_session_id, current_trace_id, current_user_id, current_vin
from app.models import AuditLog, LogEntry
from app.schemas import LogItem


class LiveLogHub:
    def __init__(self) -> None:
        self._clients: set[WebSocket] = set()

    async def connect(self, websocket: WebSocket) -> None:
        await websocket.accept()
        self._clients.add(websocket)

    def disconnect(self, websocket: WebSocket) -> None:
        self._clients.discard(websocket)

    async def broadcast(self, item: dict[str, Any]) -> None:
        stale: list[WebSocket] = []
        for websocket in self._clients:
            try:
                await websocket.send_json(item)
            except Exception:
                stale.append(websocket)
        for websocket in stale:
            self.disconnect(websocket)


class TelemetryService:
    def __init__(self, app: FastAPI) -> None:
        self.app = app
        self.logger = structlog.get_logger("bimmlite")
        self.hub = LiveLogHub()

    async def emit(
        self,
        session: Session | None,
        *,
        level: str,
        module: str,
        event: str,
        message: str = "",
        duration_ms: int | None = None,
        payload_hex: str = "",
        result: str = "",
        error: str = "",
        trace_id: str | None = None,
        session_id: str | None = None,
        user_id: str | None = None,
        vin: str | None = None,
        ecu: str | None = None,
        persist: bool = True,
        **extra: Any,
    ) -> dict[str, Any]:
        trace_id = trace_id or current_trace_id()
        session_id = session_id or current_session_id()
        user_id = user_id or current_user_id()
        vin = vin if vin is not None else current_vin()
        ecu = ecu if ecu is not None else current_ecu()
        ts = datetime.now(timezone.utc)
        payload = {
            "ts": ts.isoformat(),
            "level": level.upper(),
            "module": module,
            "event": event,
            "trace_id": trace_id,
            "session_id": session_id,
            "user_id": user_id,
            "vin": vin,
            "ecu": ecu,
            "duration_ms": duration_ms,
            "payload_hex": payload_hex,
            "result": result,
            "error": error,
            "message": message,
            **extra,
        }

        bound = self.logger.bind(module=module, event=event, trace_id=trace_id, session_id=session_id)
        method_name = level.lower()
        if method_name == "trace":
            bound.log(5, message or event, **extra)
            return payload
        if method_name == "warn":
            method_name = "warning"
        log_method = getattr(bound, method_name, bound.info)
        log_method(message or event, **extra)

        if persist and session is not None:
            session.add(
                LogEntry(
                    ts=ts,
                    level=level.upper(),
                    module=module,
                    event=event,
                    trace_id=trace_id,
                    session_id=session_id,
                    user_id=user_id,
                    vin=vin,
                    ecu=ecu,
                    duration_ms=duration_ms,
                    payload_hex=payload_hex,
                    result=result,
                    error=error,
                    message=message,
                )
            )
            session.commit()

        await self.hub.broadcast(payload)
        return payload

    async def audit(
        self,
        session: Session | None,
        *,
        action: str,
        target: str = "",
        details: str = "",
        actor_id: str | None = None,
        trace_id: str | None = None,
        session_id: str | None = None,
    ) -> None:
        if session is None:
            return
        session.add(
            AuditLog(
                actor_id=actor_id or current_user_id(),
                trace_id=trace_id or current_trace_id(),
                session_id=session_id or current_session_id(),
                action=action,
                target=target,
                details=details,
            )
        )
        session.commit()


def serialize_log(item: dict[str, Any]) -> LogItem:
    return LogItem(**item)
