from __future__ import annotations

from contextlib import asynccontextmanager

from fastapi import FastAPI
from fastapi.middleware.cors import CORSMiddleware

from app.api.routes.bridge import router as bridge_router
from app.api.routes.health import router as health_router
from app.api.routes.phase1 import router as phase1_router
from app.api.routes.logs import router as logs_router
from app.api.routes.ops import router as ops_router
from app.core.config import get_settings
from app.core.db import Base, engine
from app.core.logging import configure_logging
from app.middleware import trace_middleware
from app import models  # noqa: F401
from app.services.bridge import BridgeManager
from app.services.telemetry import TelemetryService


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings = get_settings()
    configure_logging(settings.log_level)
    if settings.env.lower() == "test":
        Base.metadata.create_all(bind=engine)
    if not hasattr(app.state, "bridge_manager"):
        app.state.bridge_manager = BridgeManager()
    if not hasattr(app.state, "telemetry"):
        app.state.telemetry = TelemetryService(app)
    app.state.bridge_manager.set_telemetry(app.state.telemetry)
    if not hasattr(app.state, "db_session_factory"):
        from app.core.db import SessionLocal

        app.state.db_session_factory = SessionLocal
    yield


def create_app() -> FastAPI:
    settings = get_settings()
    app = FastAPI(title=settings.app_name, lifespan=lifespan)

    app.add_middleware(
        CORSMiddleware,
        allow_origins=settings.allowed_origins_list,
        allow_credentials=True,
        allow_methods=["*"],
        allow_headers=["*"],
    )
    app.middleware("http")(trace_middleware)

    app.include_router(health_router)
    app.include_router(phase1_router)
    app.include_router(ops_router)
    app.include_router(logs_router)
    app.include_router(bridge_router)
    return app


app = create_app()
