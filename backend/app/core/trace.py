from __future__ import annotations

from contextvars import ContextVar
from uuid import uuid4

trace_id_var: ContextVar[str] = ContextVar("trace_id", default="")
session_id_var: ContextVar[str] = ContextVar("session_id", default="")
user_id_var: ContextVar[str] = ContextVar("user_id", default="")
vin_var: ContextVar[str] = ContextVar("vin", default="")
ecu_var: ContextVar[str] = ContextVar("ecu", default="")


def new_trace_id() -> str:
    return uuid4().hex


def bind_context(
    *,
    trace_id: str | None = None,
    session_id: str | None = None,
    user_id: str | None = None,
    vin: str | None = None,
    ecu: str | None = None,
) -> dict[str, str]:
    values: dict[str, str] = {}
    if trace_id is not None:
        trace_id_var.set(trace_id)
        values["trace_id"] = trace_id
    if session_id is not None:
        session_id_var.set(session_id)
        values["session_id"] = session_id
    if user_id is not None:
        user_id_var.set(user_id)
        values["user_id"] = user_id
    if vin is not None:
        vin_var.set(vin)
        values["vin"] = vin
    if ecu is not None:
        ecu_var.set(ecu)
        values["ecu"] = ecu
    return values


def current_trace_id() -> str:
    return trace_id_var.get() or new_trace_id()


def current_session_id() -> str:
    return session_id_var.get() or "ui"


def current_user_id() -> str:
    return user_id_var.get() or "anonymous"


def current_vin() -> str:
    return vin_var.get() or ""


def current_ecu() -> str:
    return ecu_var.get() or ""
