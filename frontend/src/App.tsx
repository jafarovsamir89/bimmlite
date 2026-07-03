import { useEffect, useMemo, useState } from "react";
import { getHealth, getLogs, sendPing } from "./lib/api";

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

type Health = {
  status: string;
  bridge_connected: boolean;
};

function randomId() {
  return crypto.randomUUID().replaceAll("-", "");
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [pingState, setPingState] = useState<string>("idle");
  const [currentTrace, setCurrentTrace] = useState<string>("");
  const sessionId = useMemo(() => {
    const existing = window.localStorage.getItem("bimmlite.session_id");
    if (existing) {
      return existing;
    }
    const created = `ui-${randomId()}`;
    window.localStorage.setItem("bimmlite.session_id", created);
    return created;
  }, []);

  useEffect(() => {
    let cancelled = false;
    getHealth()
      .then((value) => {
        if (!cancelled) {
          setHealth(value);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setHealth({ status: "offline", bridge_connected: false });
        }
      });

    getLogs()
      .then((items) => {
        if (!cancelled) {
          setLogs([...items].reverse());
        }
      })
      .catch(() => {
        if (!cancelled) {
          setLogs([]);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    const wsProtocol = window.location.protocol === "https:" ? "wss" : "ws";
    const socket = new WebSocket(`${wsProtocol}://${window.location.hostname}:8000/ws/logs`);
    socket.onmessage = (event) => {
      const item = JSON.parse(event.data) as LogItem;
      setLogs((current) => [...current, item].slice(-200));
    };
    return () => {
      socket.close();
    };
  }, []);

  async function handlePing() {
    const traceId = randomId();
    setCurrentTrace(traceId);
    setPingState("sending");
    try {
      const response = await sendPing(traceId, sessionId);
      setPingState(response.result ?? "ok");
      setHealth((current) =>
        current ? { ...current, bridge_connected: Boolean(response.bridge_connected) } : current,
      );
    } catch (error) {
      setPingState(error instanceof Error ? error.message : "ping failed");
    }
  }

  return (
    <div className="shell">
      <div className="shell__backdrop" />
      <main className="dashboard">
        <header className="hero">
          <div>
            <p className="eyebrow">BimmLite Phase 0</p>
            <h1>Smart server, thin bridge, one trace.</h1>
            <p className="lede">
              The first commit is about foundations: structured logs, bridge transport, and a live
              operational view.
            </p>
          </div>
          <div className="hero__panel">
            <span className={`badge ${health?.bridge_connected ? "badge--ok" : "badge--warn"}`}>
              {health?.bridge_connected ? "Bridge connected" : "Bridge waiting"}
            </span>
            <button className="ping-button" onClick={handlePing} type="button">
              Ping stack
            </button>
            <dl className="metrics">
              <div>
                <dt>Status</dt>
                <dd>{health?.status ?? "loading"}</dd>
              </div>
              <div>
                <dt>Trace</dt>
                <dd>{currentTrace || "none yet"}</dd>
              </div>
              <div>
                <dt>Session</dt>
                <dd>{sessionId}</dd>
              </div>
              <div>
                <dt>Result</dt>
                <dd>{pingState}</dd>
              </div>
            </dl>
          </div>
        </header>

        <section className="panel panel--logs">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">Live telemetry</p>
              <h2>Structured logs</h2>
            </div>
            <p className="panel__hint">UI stream from backend WebSocket</p>
          </div>
          <div className="log-table">
            <div className="log-table__head">
              <span>ts</span>
              <span>level</span>
              <span>module</span>
              <span>event</span>
              <span>trace_id</span>
            </div>
            {logs.map((item) => (
              <div className="log-table__row" key={`${item.ts}-${item.event}-${item.trace_id}`}>
                <span>{item.ts}</span>
                <span className={`level level--${item.level.toLowerCase()}`}>{item.level}</span>
                <span>{item.module}</span>
                <span>{item.event}</span>
                <span className="mono">{item.trace_id}</span>
              </div>
            ))}
            {logs.length === 0 ? <div className="log-table__empty">No log events yet.</div> : null}
          </div>
        </section>
      </main>
    </div>
  );
}
