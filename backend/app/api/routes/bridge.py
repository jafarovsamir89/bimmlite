from __future__ import annotations

import json
from datetime import datetime, timezone

from fastapi import APIRouter, WebSocket, WebSocketDisconnect

from app.core.config import get_settings
from app.core.trace import bind_context, new_trace_id
from app.services.telemetry import TelemetryService

router = APIRouter(tags=["bridge"])


@router.websocket("/ws/bridge")
async def bridge_socket(websocket: WebSocket) -> None:
    settings = get_settings()
    await websocket.accept()
    telemetry: TelemetryService = websocket.app.state.telemetry
    bridge = websocket.app.state.bridge_manager
    db_session = websocket.app.state.db_session_factory()
    authenticated = False

    try:
        auth_raw = await websocket.receive_text()
        auth = json.loads(auth_raw)
        payload = auth.get("payload", {})
        token = payload.get("token", "")
        session_id = payload.get("session_id", "bridge")
        trace_id = auth.get("trace_id") or new_trace_id()

        if token != settings.bridge_session_token:
            await websocket.close(code=1008)
            return

        authenticated = True
        await bridge.attach(websocket, session_id=session_id)
        bind_context(trace_id=trace_id, session_id=session_id)
        await telemetry.emit(
            db_session,
            level="info",
            module="bridge",
            event="bridge.auth.ok",
            trace_id=trace_id,
            session_id=session_id,
            message="bridge authenticated",
        )

        while True:
            raw = await websocket.receive_text()
            message = json.loads(raw)
            trace_id = message.get("trace_id", new_trace_id())
            session_id = message.get("session_id", session_id)
            bind_context(trace_id=trace_id, session_id=session_id)
            msg_type = message.get("msg_type")
            payload = message.get("payload", {})

            if msg_type == "log":
                await telemetry.emit(
                    db_session,
                    level=str(payload.get("level", "info")),
                    module=str(payload.get("module", "bridge")),
                    event=str(payload.get("event", "bridge.log")),
                    trace_id=trace_id,
                    session_id=session_id,
                    user_id=str(payload.get("user_id", "bridge")),
                    vin=str(payload.get("vin", "")),
                    ecu=str(payload.get("ecu", "")),
                    message=str(payload.get("message", "")),
                    payload_hex=str(payload.get("payload_hex", "")),
                    result=str(payload.get("result", "")),
                    error=str(payload.get("error", "")),
                )
                continue

            if msg_type == "result":
                await bridge.handle_incoming(raw)
                continue

            if msg_type == "heartbeat":
                await telemetry.emit(
                    db_session,
                    level="debug",
                    module="bridge",
                    event="heartbeat.received",
                    trace_id=trace_id,
                    session_id=session_id,
                    message="heartbeat received from bridge",
                )
                await websocket.send_text(
                    json.dumps(
                        {
                            "version": "1.0",
                            "ts": datetime.now(timezone.utc).isoformat(),
                            "trace_id": trace_id,
                            "session_id": session_id,
                            "msg_type": "heartbeat",
                            "payload": {"status": "alive"},
                        }
                    )
                )
                continue

            await bridge.handle_incoming(raw)
    except WebSocketDisconnect:
        if authenticated:
            await telemetry.emit(
                db_session,
                level="warn",
                module="bridge",
                event="bridge.disconnected",
                message="bridge disconnected",
            )
    finally:
        bridge.detach()
        db_session.close()
