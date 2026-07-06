from __future__ import annotations

from datetime import datetime

from pydantic import BaseModel, Field


class HealthResponse(BaseModel):
    status: str = "ok"
    bridge_connected: bool = False
    bridge_attached: bool = False
    is_alive: bool = False
    pid: int = 0
    last_heartbeat_at: datetime | None = None
    pending_commands: int = 0


class PingResponse(BaseModel):
    trace_id: str
    result: str
    bridge_connected: bool


class EcuInfo(BaseModel):
    address: str
    name: str = ""
    protocol: str = ""
    present: bool = True


class DtcInfo(BaseModel):
    ecu_address: str = ""
    ecu_name: str = ""
    code: str
    status: str = ""
    description: str = ""
    raw: str = ""


class ParameterInfo(BaseModel):
    ecu_address: str = ""
    ecu_name: str = ""
    did: str
    value_hex: str = ""
    value_text: str = ""


class Phase1Snapshot(BaseModel):
    protocol: str
    vin: str
    battery_voltage: float | None = None
    ecus: list[EcuInfo] = Field(default_factory=list)
    dtcs: list[DtcInfo] = Field(default_factory=list)
    parameters: list[ParameterInfo] = Field(default_factory=list)


class Phase1SnapshotResponse(BaseModel):
    trace_id: str
    session_id: str
    snapshot: Phase1Snapshot


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
