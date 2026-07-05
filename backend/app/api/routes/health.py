from __future__ import annotations

from fastapi import APIRouter, Request

from app.schemas import HealthResponse

router = APIRouter(tags=["health"])


@router.get("/health", response_model=HealthResponse)
async def health(request: Request) -> HealthResponse:
    bridge = request.app.state.bridge_manager
    return HealthResponse(
        bridge_connected=getattr(bridge, "connected", False),
        bridge_attached=getattr(bridge, "attached", False),
        pid=getattr(bridge, "pid", 0),
        last_heartbeat_at=getattr(bridge, "last_heartbeat_at", None),
        pending_commands=getattr(bridge, "pending_commands", 0),
    )
