from __future__ import annotations

import os
from datetime import datetime, timedelta, timezone

os.environ.setdefault("BIMM_DATABASE_URL", "sqlite+pysqlite:///./.pytest-bimmlite.db")
os.environ.setdefault("BIMM_ALLOWED_ORIGINS", "http://localhost:5173")

import pytest

from app.services.bridge import BridgeManager


class DummyWebSocket:
    def __init__(self, name: str) -> None:
        self.name = name
        self.closed = False

    async def close(self) -> None:
        self.closed = True


@pytest.mark.asyncio
async def test_attach_replaces_only_current_socket() -> None:
    manager = BridgeManager()
    old_ws = DummyWebSocket("old")
    new_ws = DummyWebSocket("new")

    await manager.attach(old_ws, session_id="old-session")
    replaced = await manager.attach(new_ws, session_id="new-session")

    assert replaced is True
    assert old_ws.closed is True
    assert manager.websocket is new_ws
    assert manager.session_id == "new-session"
    assert manager.attached is True

    stale_detach = manager.detach(old_ws)
    assert stale_detach is False
    assert manager.websocket is new_ws
    assert manager.attached is True

    owner_detach = manager.detach(new_ws)
    assert owner_detach is True
    assert manager.websocket is None
    assert manager.attached is False


@pytest.mark.asyncio
async def test_touch_activity_refreshes_liveness() -> None:
    manager = BridgeManager()
    ws = DummyWebSocket("active")

    await manager.attach(ws, session_id="active-session")
    manager.last_heartbeat_at = datetime.now(timezone.utc) - timedelta(seconds=60)
    assert manager.is_alive is False

    manager.touch_activity()
    assert manager.last_heartbeat_at is not None
    assert manager.is_alive is True
