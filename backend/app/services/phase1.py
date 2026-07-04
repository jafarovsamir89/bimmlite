from __future__ import annotations

from typing import Any

from fastapi import Request
from sqlalchemy.orm import Session

from app.core.trace import current_session_id, current_trace_id
from app.schemas import Phase1Snapshot
from app.services.dtc_catalog import get_dtc_catalog
from app.services.telemetry import TelemetryService


DEFAULT_PARAMETER_DIDS = ["F186", "F190", "100A", "100E", "172A", "F18C"]


class Phase1Service:
    def __init__(self, request: Request) -> None:
        self.request = request
        self.bridge = request.app.state.bridge_manager
        self.telemetry: TelemetryService = request.app.state.telemetry
        self.dtc_catalog = get_dtc_catalog()

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
        discover = self._unwrap_bridge_result(
            await self.bridge.send_command(
            command="connect.discover",
            payload={"mode": "auto"},
            trace_id=trace_id,
            session_id=session_id,
        )
        )
        discover_payload = discover
        vehicle = discover_payload.get("vehicle", {})
        protocol = str(vehicle.get("protocol", discover_payload.get("protocol", "unknown")))
        vehicle_ip = str(vehicle.get("ip", discover_payload.get("ip", "")))
        vin = str(vehicle.get("vin", discover_payload.get("vin", "")))
        battery_voltage = vehicle.get("battery_voltage", discover_payload.get("battery_voltage"))

        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="connect.discover.found",
            trace_id=trace_id,
            session_id=session_id,
            vin=vin,
            result=protocol,
            message=f"vehicle discovery complete ip={vehicle_ip or 'unknown'} protocol={protocol}",
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
        scan = self._unwrap_bridge_result(
            await self.bridge.send_command(
            command="ecu.scan",
            payload={"vin": vin},
            trace_id=trace_id,
            session_id=session_id,
        )
        )
        ecus = list(scan.get("ecus", []))
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
            dtc_result = self._unwrap_bridge_result(
                await self.bridge.send_command(
                command="dtc.read",
                payload={"ecu_address": ecu_address, "ecu_name": ecu_name},
                trace_id=trace_id,
                session_id=session_id,
            )
            )
            ecu_dtcs = list(dtc_result.get("dtcs", []))
            for dtc in ecu_dtcs:
                if not dtc.get("description"):
                    dtc["description"] = self.dtc_catalog.describe(str(dtc.get("code", "")), ecu_name=str(ecu_name))
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
            params_result = self._unwrap_bridge_result(
                await self.bridge.send_command(
                command="params.read",
                payload={
                    "ecu_address": ecu_address,
                    "ecu_name": ecu_name,
                    "dids": dids,
                },
                trace_id=trace_id,
                session_id=session_id,
            )
            )
            ecu_params = list(params_result.get("parameters", []))
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

    @staticmethod
    def _unwrap_bridge_result(message: dict[str, Any]) -> dict[str, Any]:
        payload = message.get("payload", {})
        if not isinstance(payload, dict):
            return {}
        if payload.get("ok") is False:
            raise RuntimeError(str(payload.get("error", "bridge command failed")))
        data = payload.get("data", payload)
        return data if isinstance(data, dict) else {}
