import { useEffect, useMemo, useState } from "react";
import { connectRead, getHealth, getLogs, sendPing } from "./lib/api";

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

type EcuInfo = {
  address: string;
  name: string;
  protocol: string;
  present: boolean;
};

type DtcInfo = {
  ecu_address: string;
  ecu_name: string;
  code: string;
  status: string;
  description: string;
  raw: string;
};

type ParameterInfo = {
  ecu_address: string;
  ecu_name: string;
  did: string;
  value_hex: string;
  value_text: string;
};

type Phase1Snapshot = {
  protocol: string;
  vin: string;
  battery_voltage: number | null;
  ecus: EcuInfo[];
  dtcs: DtcInfo[];
  parameters: ParameterInfo[];
};

type Phase1Response = {
  trace_id: string;
  session_id: string;
  snapshot: Phase1Snapshot;
};

function randomId() {
  return crypto.randomUUID().replaceAll("-", "");
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [pingState, setPingState] = useState<string>("idle");
  const [phase1State, setPhase1State] = useState<string>("idle");
  const [currentTrace, setCurrentTrace] = useState<string>("");
  const [snapshot, setSnapshot] = useState<Phase1Snapshot | null>(null);
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

  async function handleConnectRead() {
    const traceId = randomId();
    setCurrentTrace(traceId);
    setPhase1State("running");
    try {
      const response = (await connectRead(traceId, sessionId)) as Phase1Response;
      setSnapshot(response.snapshot);
      setPhase1State(response.snapshot.ecus.length ? "read complete" : "connected");
      setHealth((current) =>
        current ? { ...current, bridge_connected: true } : { status: "ok", bridge_connected: true },
      );
    } catch (error) {
      setPhase1State(error instanceof Error ? error.message : "connect-read failed");
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
            <div className="hero__actions">
              <button className="ping-button" onClick={handleConnectRead} type="button">
                Connect & Read
              </button>
              <button className="secondary-button" onClick={handlePing} type="button">
                Ping stack
              </button>
            </div>
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
                <dt>Read</dt>
                <dd>{phase1State}</dd>
              </div>
              <div>
                <dt>Ping</dt>
                <dd>{pingState}</dd>
              </div>
            </dl>
          </div>
        </header>

        <section className="panel panel--snapshot">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">Vehicle snapshot</p>
              <h2>Connect & read results</h2>
            </div>
            <p className="panel__hint">{currentTrace || "trace pending"}</p>
          </div>
          {snapshot ? (
            <div className="snapshot-grid">
              <div className="snapshot-card">
                <span className="snapshot-label">Protocol</span>
                <strong>{snapshot.protocol}</strong>
              </div>
              <div className="snapshot-card">
                <span className="snapshot-label">VIN</span>
                <strong>{snapshot.vin || "unknown"}</strong>
              </div>
              <div className="snapshot-card">
                <span className="snapshot-label">Battery</span>
                <strong>{snapshot.battery_voltage?.toFixed(1) ?? "n/a"} V</strong>
              </div>
              <div className="snapshot-card">
                <span className="snapshot-label">ECUs</span>
                <strong>{snapshot.ecus.length}</strong>
              </div>
            </div>
          ) : (
            <div className="panel__empty">Run Connect & Read to populate the live vehicle snapshot.</div>
          )}
        </section>

        <section className="panel panel--data">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">ECU inventory</p>
              <h2>Discovered modules</h2>
            </div>
          </div>
          <div className="chip-grid">
            {(snapshot?.ecus ?? []).map((ecu) => (
              <div className="chip" key={ecu.address}>
                <span className="chip__title">{ecu.address}</span>
                <span className="chip__meta">{ecu.name || ecu.protocol || "ECU"}</span>
              </div>
            ))}
            {!snapshot?.ecus.length ? <div className="panel__empty">No ECUs discovered yet.</div> : null}
          </div>
        </section>

        <section className="panel panel--data">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">DTCs</p>
              <h2>Read-only fault table</h2>
            </div>
          </div>
          <div className="data-table">
            <div className="data-table__head">
              <span>ECU</span>
              <span>Code</span>
              <span>Status</span>
              <span>Description</span>
            </div>
            {(snapshot?.dtcs ?? []).map((dtc, index) => (
              <div className="data-table__row" key={`${dtc.ecu_address}-${dtc.code}-${index}`}>
                <span>{dtc.ecu_name || dtc.ecu_address}</span>
                <span className="mono">{dtc.code}</span>
                <span>{dtc.status || "n/a"}</span>
                <span>{dtc.description || dtc.raw || "n/a"}</span>
              </div>
            ))}
            {!snapshot?.dtcs.length ? <div className="panel__empty">No DTCs returned yet.</div> : null}
          </div>
        </section>

        <section className="panel panel--data">
          <div className="panel__header">
            <div>
              <p className="panel__eyebrow">Parameters</p>
              <h2>Standard reads</h2>
            </div>
          </div>
          <div className="data-table">
            <div className="data-table__head">
              <span>ECU</span>
              <span>DID</span>
              <span>Value</span>
              <span>Text</span>
            </div>
            {(snapshot?.parameters ?? []).map((parameter, index) => (
              <div className="data-table__row" key={`${parameter.ecu_address}-${parameter.did}-${index}`}>
                <span>{parameter.ecu_name || parameter.ecu_address}</span>
                <span className="mono">{parameter.did}</span>
                <span className="mono">{parameter.value_hex || "n/a"}</span>
                <span>{parameter.value_text || "n/a"}</span>
              </div>
            ))}
            {!snapshot?.parameters.length ? <div className="panel__empty">No parameters returned yet.</div> : null}
          </div>
        </section>

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
