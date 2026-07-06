import os

os.environ.setdefault("BIMM_DATABASE_URL", "sqlite+pysqlite:///./.pytest-bimmlite.db")
os.environ.setdefault("BIMM_ALLOWED_ORIGINS", "http://localhost:5173")

from fastapi.testclient import TestClient

from app.main import create_app


class FakeBridge:
    connected = True

    async def send_command(self, **kwargs):
        return {"msg_type": "result", "payload": {"result": "pong"}}


def test_health_endpoint():
    app = create_app()
    app.state.bridge_manager = FakeBridge()
    with TestClient(app) as client:
        response = client.get("/health")
    assert response.status_code == 200
    assert response.json()["status"] == "ok"
    assert "is_alive" in response.json()


def test_ping_endpoint_uses_shared_trace():
    app = create_app()
    app.state.bridge_manager = FakeBridge()
    with TestClient(app) as client:
        response = client.post("/api/ops/ping", headers={"X-Trace-Id": "trace-123", "X-Session-Id": "session-1"})
    assert response.status_code == 200
    body = response.json()
    assert body["trace_id"] == "trace-123"
    assert body["result"] == "pong"
