from __future__ import annotations

from fastapi import APIRouter, Request

from app.schemas import HealthResponse

router = APIRouter(tags=["health"])


@router.get("/health", response_model=HealthResponse)
async def health(request: Request) -> HealthResponse:
    bridge = request.app.state.bridge_manager
    return HealthResponse(
        bridge_connected=bridge.connected,
        bridge_attached=bridge.attached,
        pid=bridge.pid,
        last_heartbeat_at=bridge.last_heartbeat_at,
        pending_commands=bridge.pending_commands,
    )
