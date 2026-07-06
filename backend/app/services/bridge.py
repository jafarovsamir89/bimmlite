from __future__ import annotations

import asyncio
import json
import os
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from fastapi import WebSocket

from app.core.config import get_settings
from app.core.trace import current_session_id, current_trace_id
from app.services.telemetry import TelemetryService


BridgeEventHandler = Callable[[dict[str, Any]], Awaitable[None]]


@dataclass
class BridgeManager:
    websocket: WebSocket | None = None
    session_id: str = ""
    authenticated: bool = False
    last_heartbeat_at: datetime | None = None
    last_error: str = ""
    telemetry: TelemetryService | None = None
    _lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    _pending: dict[str, asyncio.Future[dict[str, Any]]] = field(default_factory=dict)
    _handlers: list[BridgeEventHandler] = field(default_factory=list)
    _connected_event: asyncio.Event = field(default_factory=asyncio.Event)

    def add_handler(self, handler: BridgeEventHandler) -> None:
        self._handlers.append(handler)

    def set_telemetry(self, telemetry: TelemetryService) -> None:
        self.telemetry = telemetry

    @property
    def pid(self) -> int:
        return os.getpid()

    @property
    def attached(self) -> bool:
        return self.websocket is not None

    @property
    def is_alive(self) -> bool:
        if not self.attached or self.last_heartbeat_at is None:
            return False
        settings = get_settings()
        heartbeat_age = (datetime.now(timezone.utc) - self.last_heartbeat_at).total_seconds()
        return heartbeat_age <= settings.bridge_heartbeat_seconds * 3

    @property
    def pending_commands(self) -> int:
        return len(self._pending)

    @property
    def connected(self) -> bool:
        return self.websocket is not None and self.authenticated

    def touch_activity(self) -> None:
        self.last_heartbeat_at = datetime.now(timezone.utc)

    async def attach(self, websocket: WebSocket, session_id: str) -> bool:
        replace = self.websocket is not None and self.websocket is not websocket
        if replace and self.websocket is not None:
            try:
                await self.websocket.close()
            except Exception:
                pass
        self.websocket = websocket
        self.session_id = session_id
        self.authenticated = True
        self.touch_activity()
        self.last_error = ""
        self._connected_event.set()
        return replace

    def detach(self, websocket: WebSocket | None = None) -> bool:
        owner = websocket is None or websocket is self.websocket
        if not owner:
            return False
        self.websocket = None
        self.session_id = ""
        self.authenticated = False
        self.last_error = ""
        self._connected_event.clear()
        for future in self._pending.values():
            if not future.done():
                future.cancel()
        self._pending.clear()
        return True

    async def send_command(
        self,
        *,
        command: str,
        payload: dict[str, Any] | None = None,
        trace_id: str | None = None,
        session_id: str | None = None,
    ) -> dict[str, Any]:
        settings = get_settings()
        trace_id = trace_id or current_trace_id()
        session_id = session_id or current_session_id()
        envelope = {
            "version": "1.0",
            "ts": datetime.now(timezone.utc).isoformat(),
            "trace_id": trace_id,
            "session_id": session_id,
            "msg_type": "command",
            "payload": {"command": command, "args": payload or {}},
        }

        future: asyncio.Future[dict[str, Any]] = asyncio.get_running_loop().create_future()
        self._pending[trace_id] = future
        deadline = asyncio.get_running_loop().time() + settings.bridge_command_timeout_seconds

        try:
            while True:
                if not self.connected or self.websocket is None:
                    remaining = deadline - asyncio.get_running_loop().time()
                    if remaining <= 0:
                        raise RuntimeError("bridge is not connected")
                    try:
                        await asyncio.wait_for(self._connected_event.wait(), timeout=remaining)
                    except TimeoutError as exc:
                        raise RuntimeError("bridge is not connected") from exc

                if self.telemetry is not None:
                    await self.telemetry.emit(
                        None,
                        level="debug",
                        module="bridge",
                        event="bridge.send_command",
                        trace_id=trace_id,
                        session_id=session_id,
                        message=f"send command {command}",
                        persist=False,
                        pid=self.pid,
                        command=command,
                        pending_commands=self.pending_commands + 1,
                    )

                async with self._lock:
                    if not self.connected or self.websocket is None:
                        continue
                    try:
                        await self.websocket.send_text(json.dumps(envelope))
                        break
                    except Exception as exc:
                        self.last_error = str(exc)
                        continue

            remaining = deadline - asyncio.get_running_loop().time()
            if remaining <= 0:
                raise RuntimeError("bridge command timed out")
            result = await asyncio.wait_for(future, timeout=remaining)
            if self.telemetry is not None:
                await self.telemetry.emit(
                    None,
                    level="debug",
                    module="bridge",
                    event="bridge.result.matched",
                    trace_id=trace_id,
                    session_id=session_id,
                    message=f"result matched for {command}",
                    persist=False,
                    pid=self.pid,
                    command=command,
                )
            return result
        except TimeoutError as exc:
            remaining_error = f"timeout after {settings.bridge_command_timeout_seconds}s"
            if self.telemetry is not None:
                await self.telemetry.emit(
                    None,
                    level="warn",
                    module="bridge",
                    event="bridge.command.timeout",
                    trace_id=trace_id,
                    session_id=session_id,
                    error=remaining_error,
                    message=f"command timeout for {command}",
                    persist=False,
                    pid=self.pid,
                    command=command,
                )
            raise RuntimeError("bridge command timed out") from exc
        finally:
            self._pending.pop(trace_id, None)

    async def handle_incoming(self, raw_message: str) -> dict[str, Any] | None:
        message = json.loads(raw_message)
        self.touch_activity()
        for handler in self._handlers:
            await handler(message)
        trace_id = message.get("trace_id", "")
        msg_type = message.get("msg_type")
        if msg_type == "result" and trace_id in self._pending:
            future = self._pending[trace_id]
            if not future.done():
                future.set_result(message)
            if self.telemetry is not None:
                payload = message.get("payload", {})
                command = ""
                if isinstance(payload, dict):
                    command = str(payload.get("command", ""))
                await self.telemetry.emit(
                    None,
                    level="debug",
                    module="bridge",
                    event="bridge.result.matched",
                    trace_id=trace_id,
                    session_id=str(message.get("session_id", "")),
                    message=f"result matched for {command or 'command'}",
                    persist=False,
                    pid=self.pid,
                    command=command,
                )
            return message
        if msg_type == "heartbeat":
            return message
        return None
