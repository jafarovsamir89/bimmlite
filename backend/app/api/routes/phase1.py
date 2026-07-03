from __future__ import annotations

from fastapi import APIRouter, Depends, Request
from sqlalchemy.orm import Session

from app.core.db import get_db
from app.core.trace import current_session_id, current_trace_id
from app.schemas import Phase1SnapshotResponse
from app.services.phase1 import Phase1Service

router = APIRouter(prefix="/api/phase1", tags=["phase1"])


@router.post("/connect-read", response_model=Phase1SnapshotResponse)
async def connect_read(request: Request, db: Session = Depends(get_db)) -> Phase1SnapshotResponse:
    service = Phase1Service(request)
    trace_id = current_trace_id()
    session_id = current_session_id()
    snapshot = await service.connect_and_read(db, trace_id=trace_id, session_id=session_id)
    return Phase1SnapshotResponse(trace_id=trace_id, session_id=session_id, snapshot=snapshot)
