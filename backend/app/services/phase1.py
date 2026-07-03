from __future__ import annotations

from typing import Any

from fastapi import Request
from sqlalchemy.orm import Session

from app.core.trace import current_session_id, current_trace_id
from app.schemas import Phase1Snapshot
from app.services.telemetry import TelemetryService


DEFAULT_PARAMETER_DIDS = ["F190", "F187", "F188", "F189"]


class Phase1Service:
    def __init__(self, request: Request) -> None:
        self.request = request
        self.bridge = request.app.state.bridge_manager
        self.telemetry: TelemetryService = request.app.state.telemetry

    async def connect_and_read(
        self,
        db: Session,
        *,
        trace_id: str | None = None,
        session_id: str | None = None,
        dids: list[str] | None = None,
    ) -> Phase1Snapshot:
        trace_id = trace_id or current_trace_id()
        session_id = session_id or current_session_id()
        dids = dids or DEFAULT_PARAMETER_DIDS

        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="connect.discover.start",
            trace_id=trace_id,
            session_id=session_id,
            message="starting vehicle discovery",
        )
        discover = await self.bridge.send_command(
            command="connect.discover",
            payload={"mode": "auto"},
            trace_id=trace_id,
            session_id=session_id,
        )
        discover_payload = discover.get("payload", {})
        vehicle = discover_payload.get("vehicle", {})
        protocol = str(vehicle.get("protocol", discover_payload.get("protocol", "unknown")))
        vin = str(vehicle.get("vin", discover_payload.get("vin", "")))
        battery_voltage = vehicle.get("battery_voltage", discover_payload.get("battery_voltage"))

        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="connect.vin.ok",
            trace_id=trace_id,
            session_id=session_id,
            vin=vin,
            result=protocol,
            message="vehicle discovery complete",
        )

        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="ecu.scan.start",
            trace_id=trace_id,
            session_id=session_id,
            vin=vin,
            message="starting ECU scan",
        )
        scan = await self.bridge.send_command(
            command="ecu.scan",
            payload={"vin": vin},
            trace_id=trace_id,
            session_id=session_id,
        )
        ecus = list(scan.get("payload", {}).get("ecus", []))
        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="ecu.scan.done",
            trace_id=trace_id,
            session_id=session_id,
            vin=vin,
            result=f"{len(ecus)} ecus",
            message="ecu scan completed",
        )

        dtcs: list[dict[str, Any]] = []
        parameters: list[dict[str, Any]] = []
        for ecu in ecus:
            ecu_address = ecu.get("address", "")
            ecu_name = ecu.get("name", "")
            await self.telemetry.emit(
                db,
                level="info",
                module="phase1",
                event="dtc.read.start",
                trace_id=trace_id,
                session_id=session_id,
                vin=vin,
                ecu=str(ecu_name),
                message="reading DTCs",
            )
            dtc_result = await self.bridge.send_command(
                command="dtc.read",
                payload={"ecu_address": ecu_address, "ecu_name": ecu_name},
                trace_id=trace_id,
                session_id=session_id,
            )
            ecu_dtcs = list(dtc_result.get("payload", {}).get("dtcs", []))
            dtcs.extend(ecu_dtcs)
            await self.telemetry.emit(
                db,
                level="info",
                module="phase1",
                event="dtc.read.done",
                trace_id=trace_id,
                session_id=session_id,
                vin=vin,
                ecu=str(ecu_name),
                result=f"{len(ecu_dtcs)} dtcs",
                message="dtc read completed",
            )

            await self.telemetry.emit(
                db,
                level="info",
                module="phase1",
                event="params.read.start",
                trace_id=trace_id,
                session_id=session_id,
                vin=vin,
                ecu=str(ecu_name),
                message="reading standard parameters",
            )
            params_result = await self.bridge.send_command(
                command="params.read",
                payload={
                    "ecu_address": ecu_address,
                    "ecu_name": ecu_name,
                    "dids": dids,
                },
                trace_id=trace_id,
                session_id=session_id,
            )
            ecu_params = list(params_result.get("payload", {}).get("parameters", []))
            parameters.extend(ecu_params)
            await self.telemetry.emit(
                db,
                level="info",
                module="phase1",
                event="params.read.done",
                trace_id=trace_id,
                session_id=session_id,
                vin=vin,
                ecu=str(ecu_name),
                result=f"{len(ecu_params)} parameters",
                message="parameter read completed",
            )

        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="connect.read.complete",
            trace_id=trace_id,
            session_id=session_id,
            vin=vin,
            result=f"{len(ecus)} ecus",
            message="phase1 read flow complete",
        )
        return Phase1Snapshot(
            protocol=protocol,
            vin=vin,
            battery_voltage=float(battery_voltage) if battery_voltage is not None else None,
            ecus=ecus,
            dtcs=dtcs,
            parameters=parameters,
        )
