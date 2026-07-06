from __future__ import annotations

from fastapi import APIRouter, Depends, HTTPException, Request
from sqlalchemy.orm import Session

from app.core.db import get_db
from app.core.trace import current_session_id, current_trace_id
from app.schemas import ClearDtcRequest, EcuActionRequest, EcuActionResponse, Phase1SnapshotResponse
from app.services.phase1 import Phase1Service

router = APIRouter(prefix="/api/phase1", tags=["phase1"])


@router.post("/connect-read", response_model=Phase1SnapshotResponse)
async def connect_read(request: Request, db: Session = Depends(get_db)) -> Phase1SnapshotResponse:
    service = Phase1Service(request)
    trace_id = current_trace_id()
    session_id = current_session_id()
    snapshot = await service.connect_and_read(db, trace_id=trace_id, session_id=session_id)
    return Phase1SnapshotResponse(trace_id=trace_id, session_id=session_id, snapshot=snapshot)


@router.post("/ecu/dtc", response_model=EcuActionResponse)
async def read_ecu_dtc(
    request: Request,
    body: EcuActionRequest,
    db: Session = Depends(get_db),
) -> EcuActionResponse:
    service = Phase1Service(request)
    trace_id = current_trace_id()
    session_id = current_session_id()
    try:
        dtcs = await service.read_ecu_dtc(
            db,
            ecu_address=body.ecu_address,
            ecu_name=body.ecu_name,
            trace_id=trace_id,
            session_id=session_id,
        )
        return EcuActionResponse(
            trace_id=trace_id,
            session_id=session_id,
            ecu_address=body.ecu_address,
            ecu_name=body.ecu_name,
            result={"dtcs": dtcs},
        )
    except Exception as exc:
        await request.app.state.telemetry.emit(
            db,
            level="error",
            module="phase1",
            event="dtc.read.failed",
            trace_id=trace_id,
            session_id=session_id,
            ecu=body.ecu_name,
            error=str(exc),
            message="ecu dtc read failed",
            persist=False,
        )
        raise HTTPException(status_code=502, detail={"trace_id": trace_id, "error": str(exc)}) from exc


@router.post("/ecu/params", response_model=EcuActionResponse)
async def read_ecu_params(
    request: Request,
    body: EcuActionRequest,
    db: Session = Depends(get_db),
) -> EcuActionResponse:
    service = Phase1Service(request)
    trace_id = current_trace_id()
    session_id = current_session_id()
    try:
        params = await service.read_ecu_parameters(
            db,
            ecu_address=body.ecu_address,
            ecu_name=body.ecu_name,
            dids=body.dids,
            trace_id=trace_id,
            session_id=session_id,
        )
        return EcuActionResponse(
            trace_id=trace_id,
            session_id=session_id,
            ecu_address=body.ecu_address,
            ecu_name=body.ecu_name,
            result={"parameters": params},
        )
    except Exception as exc:
        await request.app.state.telemetry.emit(
            db,
            level="error",
            module="phase1",
            event="params.read.failed",
            trace_id=trace_id,
            session_id=session_id,
            ecu=body.ecu_name,
            error=str(exc),
            message="ecu parameter read failed",
            persist=False,
        )
        raise HTTPException(status_code=502, detail={"trace_id": trace_id, "error": str(exc)}) from exc


@router.post("/clear-dtc", response_model=EcuActionResponse)
async def clear_dtc(
    request: Request,
    body: ClearDtcRequest,
    db: Session = Depends(get_db),
) -> EcuActionResponse:
    if not body.confirmed:
        raise HTTPException(status_code=400, detail="clear-dtc requires confirmation")

    service = Phase1Service(request)
    telemetry = request.app.state.telemetry
    trace_id = current_trace_id()
    session_id = current_session_id()
    try:
        result = await service.clear_ecu_dtc(
            db,
            ecu_address=body.ecu_address,
            ecu_name=body.ecu_name,
            trace_id=trace_id,
            session_id=session_id,
        )
        await telemetry.audit(
            db,
            action="dtc.clear",
            target=body.ecu_address,
            details=f"ecu_name={body.ecu_name}",
            trace_id=trace_id,
            session_id=session_id,
        )
        return EcuActionResponse(
            trace_id=trace_id,
            session_id=session_id,
            ecu_address=body.ecu_address,
            ecu_name=body.ecu_name,
            result=result,
        )
    except Exception as exc:
        await telemetry.emit(
            db,
            level="error",
            module="phase1",
            event="dtc.clear.failed",
            trace_id=trace_id,
            session_id=session_id,
            ecu=body.ecu_name,
            error=str(exc),
            message="ecu dtc clear failed",
            persist=False,
        )
        raise HTTPException(status_code=502, detail={"trace_id": trace_id, "error": str(exc)}) from exc
