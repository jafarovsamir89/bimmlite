from __future__ import annotations

import asyncio
import json
from collections.abc import Awaitable, Callable
from dataclasses import dataclass, field
from datetime import datetime, timezone
from typing import Any

from fastapi import WebSocket

from app.core.config import get_settings
from app.core.trace import current_session_id, current_trace_id


BridgeEventHandler = Callable[[dict[str, Any]], Awaitable[None]]


@dataclass
class BridgeManager:
    websocket: WebSocket | None = None
    session_id: str = ""
    authenticated: bool = False
    last_heartbeat_at: datetime | None = None
    last_error: str = ""
    _lock: asyncio.Lock = field(default_factory=asyncio.Lock)
    _pending: dict[str, asyncio.Future[dict[str, Any]]] = field(default_factory=dict)
    _handlers: list[BridgeEventHandler] = field(default_factory=list)
    _connected_event: asyncio.Event = field(default_factory=asyncio.Event)

    def add_handler(self, handler: BridgeEventHandler) -> None:
        self._handlers.append(handler)

    @property
    def connected(self) -> bool:
        return self.websocket is not None and self.authenticated

    async def attach(self, websocket: WebSocket, session_id: str) -> None:
        self.websocket = websocket
        self.session_id = session_id
        self.authenticated = True
        self.last_heartbeat_at = datetime.now(timezone.utc)
        self.last_error = ""
        self._connected_event.set()

    def detach(self) -> None:
        self.websocket = None
        self.session_id = ""
        self.authenticated = False
        self.last_error = ""
        self._connected_event.clear()
        for future in self._pending.values():
            if not future.done():
                future.cancel()
        self._pending.clear()

    async def send_command(
        self,
        *,
        command: str,
        payload: dict[str, Any] | None = None,
        trace_id: str | None = None,
        session_id: str | None = None,
    ) -> dict[str, Any]:
        settings = get_settings()
        if not self.connected or self.websocket is None:
            try:
                await asyncio.wait_for(self._connected_event.wait(), timeout=settings.bridge_command_timeout_seconds)
            except TimeoutError as exc:
                raise RuntimeError("bridge is not connected") from exc
        if not self.connected or self.websocket is None:
            raise RuntimeError("bridge is not connected")

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
        async with self._lock:
            await self.websocket.send_text(json.dumps(envelope))

        try:
            return await asyncio.wait_for(future, timeout=settings.bridge_command_timeout_seconds)
        finally:
            self._pending.pop(trace_id, None)

    async def handle_incoming(self, raw_message: str) -> dict[str, Any] | None:
        message = json.loads(raw_message)
        for handler in self._handlers:
            await handler(message)
        trace_id = message.get("trace_id", "")
        msg_type = message.get("msg_type")
        if msg_type == "result" and trace_id in self._pending:
            future = self._pending[trace_id]
            if not future.done():
                future.set_result(message)
            return message
        if msg_type == "heartbeat":
            return message
        return None
