from __future__ import annotations

from fastapi import APIRouter, Request

from app.schemas import HealthResponse
from app.services.telemetry import TelemetryService

router = APIRouter(tags=["health"])


@router.get("/health", response_model=HealthResponse)
async def health(request: Request) -> HealthResponse:
    bridge = request.app.state.bridge_manager
    telemetry = getattr(request.app.state, "telemetry", None)
    bridge_attached = getattr(bridge, "attached", False)
    bridge_alive = getattr(bridge, "is_alive", False)
    if bridge_attached and not bridge_alive and isinstance(telemetry, TelemetryService):
        await telemetry.emit(
            None,
            level="warn",
            module="bridge",
            event="bridge.stale",
            message="bridge heartbeat is stale",
            persist=False,
            pid=getattr(bridge, "pid", 0),
            bridge_attached=getattr(bridge, "attached", False),
            pending_commands=getattr(bridge, "pending_commands", 0),
        )
    return HealthResponse(
        bridge_connected=getattr(bridge, "connected", False),
        bridge_attached=bridge_attached,
        is_alive=bridge_alive,
        pid=getattr(bridge, "pid", 0),
        last_heartbeat_at=getattr(bridge, "last_heartbeat_at", None),
        pending_commands=getattr(bridge, "pending_commands", 0),
    )
