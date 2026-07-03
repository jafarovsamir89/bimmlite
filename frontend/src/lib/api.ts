export async function getHealth() {
  const response = await fetch("/health");
  if (!response.ok) {
    throw new Error(`health check failed: ${response.status}`);
  }
  return response.json();
}

export async function getLogs() {
  const response = await fetch("/api/logs?limit=100");
  if (!response.ok) {
    throw new Error(`log fetch failed: ${response.status}`);
  }
  return response.json();
}

export async function sendPing(traceId: string, sessionId: string) {
  const response = await fetch("/api/ops/ping", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: JSON.stringify({}),
  });
  if (!response.ok) {
    throw new Error(`ping failed: ${response.status}`);
  }
  return response.json();
}

export async function connectRead(traceId: string, sessionId: string) {
  const response = await fetch("/api/phase1/connect-read", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "X-Trace-Id": traceId,
      "X-Session-Id": sessionId,
    },
    body: JSON.stringify({}),
  });
  if (!response.ok) {
    throw new Error(`connect-read failed: ${response.status}`);
  }
  return response.json();
}
