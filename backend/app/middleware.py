from __future__ import annotations

from collections.abc import Awaitable, Callable

from fastapi import Request, Response

from app.core.trace import bind_context, new_trace_id


async def trace_middleware(request: Request, call_next: Callable[[Request], Awaitable[Response]]) -> Response:
    trace_id = request.headers.get("x-trace-id") or new_trace_id()
    session_id = request.headers.get("x-session-id") or request.headers.get("x-session") or "ui"
    user_id = request.headers.get("x-user-id") or "anonymous"
    vin = request.headers.get("x-vin") or ""
    ecu = request.headers.get("x-ecu") or ""

    bind_context(trace_id=trace_id, session_id=session_id, user_id=user_id, vin=vin, ecu=ecu)
    response = await call_next(request)
    response.headers["X-Trace-Id"] = trace_id
    response.headers["X-Session-Id"] = session_id
    return response
