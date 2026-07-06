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
            ecu_dtcs: list[dict[str, Any]] = []
            try:
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
            except Exception as exc:
                await self.telemetry.emit(
                    db,
                    level="warn",
                    module="phase1",
                    event="dtc.read.failed",
                    trace_id=trace_id,
                    session_id=session_id,
                    vin=vin,
                    ecu=str(ecu_name),
                    error=str(exc),
                    message="dtc read failed, continuing",
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
            try:
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
            except Exception as exc:
                await self.telemetry.emit(
                    db,
                    level="warn",
                    module="phase1",
                    event="params.read.failed",
                    trace_id=trace_id,
                    session_id=session_id,
                    vin=vin,
                    ecu=str(ecu_name),
                    error=str(exc),
                    message="parameter read failed, continuing",
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
            battery_voltage=float(battery_voltage) if battery_voltage is not None and float(battery_voltage) > 0 else None,
            ecus=ecus,
            dtcs=dtcs,
            parameters=parameters,
        )

    async def read_ecu_dtc(
        self,
        db: Session,
        *,
        ecu_address: str,
        ecu_name: str = "",
        trace_id: str | None = None,
        session_id: str | None = None,
    ) -> list[dict[str, Any]]:
        trace_id = trace_id or current_trace_id()
        session_id = session_id or current_session_id()
        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="dtc.read.start",
            trace_id=trace_id,
            session_id=session_id,
            ecu=ecu_name,
            message="reading DTCs",
        )
        ecu_dtcs: list[dict[str, Any]] = []
        try:
            dtc_result = self._unwrap_bridge_result(
                await self.bridge.send_command(
                    command="dtc.read",
                    payload={"ecu_address": ecu_address, "ecu_name": ecu_name},
                    trace_id=trace_id,
                    session_id=session_id,
                )
            )
            ecu_dtcs = list(dtc_result.get("dtcs", []))
        except Exception as exc:
            await self.telemetry.emit(
                db,
                level="warn",
                module="phase1",
                event="dtc.read.fallback",
                trace_id=trace_id,
                session_id=session_id,
                ecu=ecu_name,
                error=str(exc),
                message="falling back to connect-read snapshot for DTCs",
            )

        if not ecu_dtcs:
            await self.telemetry.emit(
                db,
                level="debug",
                module="phase1",
                event="dtc.read.fallback",
                trace_id=trace_id,
                session_id=session_id,
                ecu=ecu_name,
                message="falling back to cached connect-read snapshot for DTCs",
            )
            snapshot = getattr(self.request.app.state, "last_phase1_snapshot", None)
            if snapshot is not None:
                ecu_dtcs = [
                    dtc
                    for dtc in snapshot.dtcs
                    if str(dtc.ecu_address).upper() == str(ecu_address).upper()
                    or (ecu_name and str(dtc.ecu_name) == ecu_name)
                ]
        for dtc in ecu_dtcs:
            if not dtc.get("description"):
                dtc["description"] = self.dtc_catalog.describe(str(dtc.get("code", "")), ecu_name=ecu_name)
        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="dtc.read.done",
            trace_id=trace_id,
            session_id=session_id,
            ecu=ecu_name,
            result=f"{len(ecu_dtcs)} dtcs",
            message="dtc read completed",
        )
        return ecu_dtcs

    async def read_ecu_parameters(
        self,
        db: Session,
        *,
        ecu_address: str,
        ecu_name: str = "",
        dids: list[str] | None = None,
        trace_id: str | None = None,
        session_id: str | None = None,
    ) -> list[dict[str, Any]]:
        trace_id = trace_id or current_trace_id()
        session_id = session_id or current_session_id()
        dids = dids or DEFAULT_PARAMETER_DIDS
        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="params.read.start",
            trace_id=trace_id,
            session_id=session_id,
            ecu=ecu_name,
            message="reading standard parameters",
        )
        ecu_params: list[dict[str, Any]] = []
        try:
            params_result = self._unwrap_bridge_result(
                await self.bridge.send_command(
                    command="params.read",
                    payload={"ecu_address": ecu_address, "ecu_name": ecu_name, "dids": dids},
                    trace_id=trace_id,
                    session_id=session_id,
                )
            )
            ecu_params = list(params_result.get("parameters", []))
        except Exception as exc:
            await self.telemetry.emit(
                db,
                level="warn",
                module="phase1",
                event="params.read.fallback",
                trace_id=trace_id,
                session_id=session_id,
                ecu=ecu_name,
                error=str(exc),
                message="falling back to cached connect-read snapshot for parameters",
            )

        if not ecu_params:
            await self.telemetry.emit(
                db,
                level="debug",
                module="phase1",
                event="params.read.fallback",
                trace_id=trace_id,
                session_id=session_id,
                ecu=ecu_name,
                message="falling back to cached connect-read snapshot for parameters",
            )
            snapshot = getattr(self.request.app.state, "last_phase1_snapshot", None)
            if snapshot is not None:
                ecu_params = [
                    param
                    for param in snapshot.parameters
                    if str(param.ecu_address).upper() == str(ecu_address).upper()
                    or (ecu_name and str(param.ecu_name) == ecu_name)
                ]
        await self.telemetry.emit(
            db,
            level="info",
            module="phase1",
            event="params.read.done",
            trace_id=trace_id,
            session_id=session_id,
            ecu=ecu_name,
            result=f"{len(ecu_params)} parameters",
            message="parameter read completed",
        )
        return ecu_params

    async def clear_ecu_dtc(
        self,
        db: Session,
        *,
        ecu_address: str,
        ecu_name: str = "",
        trace_id: str | None = None,
        session_id: str | None = None,
    ) -> dict[str, Any]:
        trace_id = trace_id or current_trace_id()
        session_id = session_id or current_session_id()
        await self.telemetry.emit(
            db,
            level="warn",
            module="phase1",
            event="dtc.clear.start",
            trace_id=trace_id,
            session_id=session_id,
            ecu=ecu_name,
            message="clearing DTCs",
        )
        result = self._unwrap_bridge_result(
            await self.bridge.send_command(
                command="dtc.clear",
                payload={"ecu_address": ecu_address, "ecu_name": ecu_name},
                trace_id=trace_id,
                session_id=session_id,
            )
        )
        await self.telemetry.emit(
            db,
            level="warn",
            module="phase1",
            event="dtc.clear.done",
            trace_id=trace_id,
            session_id=session_id,
            ecu=ecu_name,
            result=str(result.get("result", "cleared")),
            message="dtc clear completed",
        )
        return result

    @staticmethod
    def _unwrap_bridge_result(message: dict[str, Any]) -> dict[str, Any]:
        payload = message.get("payload", {})
        if not isinstance(payload, dict):
            return {}
        if payload.get("ok") is False:
            raise RuntimeError(str(payload.get("error", "bridge command failed")))
        data = payload.get("data", payload)
        return data if isinstance(data, dict) else {}
