from __future__ import annotations

from pydantic import BaseModel, Field


class HealthResponse(BaseModel):
    status: str = "ok"
    bridge_connected: bool = False


class PingResponse(BaseModel):
    trace_id: str
    result: str
    bridge_connected: bool


class LogItem(BaseModel):
    ts: str
    level: str
    module: str
    event: str
    trace_id: str
    session_id: str
    user_id: str
    vin: str
    ecu: str
    duration_ms: int | None = None
    payload_hex: str = ""
    result: str = ""
    error: str = ""
    message: str = ""


class LogStreamPayload(BaseModel):
    kind: str = Field(default="log")
    item: LogItem
