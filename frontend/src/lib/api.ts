type HealthResponse = {
  status: string;
  bridge_connected: boolean;
  bridge_attached?: boolean;
  is_alive?: boolean;
  pid?: number;
  last_heartbeat_at?: string | null;
  pending_commands?: number;
};

type LogItem = {
  ts: string;
  level: string;
  module: string;
  event: string;
  trace_id: string;
  session_id: string;
  user_id: string;
  vin: string;
  ecu: string;
  duration_ms?: number | null;
  payload_hex: string;
  result: string;
  error: string;
  message: string;
};

type RequestInitJson = {
  method?: string;
  headers?: Record<string, string>;
  body?: unknown;
};

async function requestJson<T>(url: string, init?: RequestInitJson): Promise<T> {
  const response = await fetch(url, {
    ...init,
    headers: {
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...(init?.headers ?? {}),
    },
    body: init?.body == null ? undefined : JSON.stringify(init.body),
  });
  if (!response.ok) {
    throw new Error(`${url} failed: ${response.status}`);
  }
  return response.json() as Promise<T>;
}

export async function getHealth(): Promise<HealthResponse> {
  return requestJson<HealthResponse>("/health");
}

export async function getLogs(): Promise<LogItem[]> {
  return requestJson<LogItem[]>("/api/logs?limit=150");
}

export async function sendPing(traceId: string, sessionId: string) {
  return requestJson("/api/ops/ping", {
    method: "POST",
    headers: {
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: {},
  });
}

export async function connectRead(traceId: string, sessionId: string) {
  return requestJson("/api/phase1/connect-read", {
    method: "POST",
    headers: {
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: {},
  });
}

export async function readEcuDtc(traceId: string, sessionId: string, ecuAddress: string, ecuName = "") {
  return requestJson("/api/phase1/ecu/dtc", {
    method: "POST",
    headers: {
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: { ecu_address: ecuAddress, ecu_name: ecuName },
  });
}

export async function readEcuParams(
  traceId: string,
  sessionId: string,
  ecuAddress: string,
  ecuName = "",
  dids: string[] = [],
) {
  return requestJson("/api/phase1/ecu/params", {
    method: "POST",
    headers: {
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: { ecu_address: ecuAddress, ecu_name: ecuName, dids },
  });
}

export async function clearEcuDtc(traceId: string, sessionId: string, ecuAddress: string, ecuName = "") {
  return requestJson("/api/phase1/clear-dtc", {
    method: "POST",
    headers: {
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: { ecu_address: ecuAddress, ecu_name: ecuName, confirmed: true },
  });
}
