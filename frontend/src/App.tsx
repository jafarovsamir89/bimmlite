import {
  AlertTriangle,
  CarFront,
  Cpu,
  LayoutDashboard,
  LayoutGrid,
  LogOut,
  Menu,
  Power,
  Settings2,
  SquareStack,
  X,
  Zap,
} from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";
import { connectRead, getHealth, getLogs } from "./lib/api";

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
  series?: string;
  fa_payload?: string;
};

type Phase1Response = {
  trace_id: string;
  session_id: string;
  snapshot: Phase1Snapshot;
};

type View = "workspace" | "dashboard" | "settings";

function randomId() {
  return crypto.randomUUID().replaceAll("-", "");
}

function formatVoltage(value: number | null) {
  if (value == null) return "0.0V";
  return `${value.toFixed(1)}V`;
}

function formatLogs(logs: LogItem[]) {
  return [...logs].reverse().slice(0, 120);
}

export default function App() {
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [view, setView] = useState<View>("workspace");
  const [health, setHealth] = useState<Health>({ status: "offline", bridge_connected: false });
  const [snapshot, setSnapshot] = useState<Phase1Snapshot | null>(null);
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [connectionState, setConnectionState] = useState<"disconnected" | "connecting" | "connected">(
    "disconnected",
  );
  const [isConnecting, setIsConnecting] = useState(false);
  const [logFilter, setLogFilter] = useState("");
  const sessionId = useMemo(() => {
    const existing = window.localStorage.getItem("bimmlite.session_id");
    if (existing) return existing;
    const created = `ui-${randomId()}`;
    window.localStorage.setItem("bimmlite.session_id", created);
    return created;
  }, []);
  const traceRef = useRef<string>("");

  useEffect(() => {
    let cancelled = false;
    getHealth()
      .then((value) => {
        if (cancelled) return;
        setHealth(value);
        setConnectionState(value.bridge_connected ? "connected" : "disconnected");
      })
      .catch(() => {
        if (!cancelled) {
          setHealth({ status: "offline", bridge_connected: false });
          setConnectionState("disconnected");
        }
      });

    getLogs()
      .then((items) => {
        if (cancelled) return;
        setLogs(items);
        const firstTrace = items[0]?.trace_id ?? "";
        traceRef.current = firstTrace;
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
    const socket = new WebSocket(`${wsProtocol}://${window.location.host}/ws/logs`);
    socket.onmessage = (event) => {
      const item = JSON.parse(event.data) as LogItem;
      setLogs((current) => [...current, item].slice(-250));
      if (!traceRef.current && item.trace_id) {
        traceRef.current = item.trace_id;
      }
    };
    return () => socket.close();
  }, []);

  const voltage = snapshot?.battery_voltage ?? 0;
  const ecuCount = snapshot?.ecus.length ?? 0;
  const dtcCount = snapshot?.dtcs.length ?? 0;

  async function handleConnect() {
    const traceId = randomId();
    setConnectionState("connecting");
    setIsConnecting(true);
    try {
      const response = (await connectRead(traceId, sessionId)) as Phase1Response;
      setSnapshot(response.snapshot);
      setHealth({ status: "ok", bridge_connected: true });
      setConnectionState("connected");
      traceRef.current = response.trace_id || traceId;
    } catch {
      setConnectionState("disconnected");
    } finally {
      setIsConnecting(false);
    }
  }

  function handleDisconnect() {
    setSnapshot(null);
    setConnectionState("disconnected");
    setHealth((current) => ({ ...current, bridge_connected: false }));
  }

  const visibleLogs = formatLogs(
    logs.filter((item) => !logFilter || item.trace_id.toLowerCase().includes(logFilter.toLowerCase())),
  );

  return (
    <div className="app-shell">
      <div className="app-bg" />

      <header className="topbar">
        <button
          className="icon-button"
          type="button"
          aria-label="Open menu"
          onClick={() => setDrawerOpen(true)}
        >
          <Menu size={18} />
        </button>

        <div className="topbar__status">
          <span className={`status-pill status-pill--${connectionState}`}>
            <span className="status-pill__dot" aria-hidden="true" />
            {connectionState === "connected"
              ? "CONNECTED"
              : connectionState === "connecting"
                ? "CONNECTING"
                : "DISCONNECTED"}
          </span>
          <span className="voltage-pill">
            <Zap size={12} />
            {formatVoltage(voltage)}
          </span>
        </div>

        <button className="connect-button" type="button" onClick={handleConnect} disabled={isConnecting}>
          <Power size={16} />
          <span>Connect</span>
        </button>
      </header>

      <main className="content">
        {view === "settings" ? (
          <section className="logs-panel">
            <div className="panel-card panel-card--logs">
              <div className="panel-card__header">
                <div>
                  <h2>Logs</h2>
                  <p>Live stream from the backend WebSocket.</p>
                </div>
                <input
                  className="log-filter"
                  value={logFilter}
                  onChange={(event) => setLogFilter(event.target.value)}
                  placeholder="trace_id"
                  aria-label="Filter logs by trace_id"
                />
              </div>
              <div className="logs-table">
                <div className="logs-table__head">
                  <span>ts</span>
                  <span>level</span>
                  <span>module</span>
                  <span>event</span>
                </div>
                {visibleLogs.length ? (
                  visibleLogs.map((item) => (
                    <div className="logs-table__row" key={`${item.ts}-${item.event}-${item.trace_id}`}>
                      <span className="mono">{item.ts.slice(11, 23)}</span>
                      <span className={`log-badge log-badge--${item.level.toLowerCase()}`}>{item.level}</span>
                      <span>{item.module}</span>
                      <span>{item.event}</span>
                    </div>
                  ))
                ) : (
                  <div className="empty-state">No log events yet.</div>
                )}
              </div>
            </div>
          </section>
        ) : (
          <>
            <section className="hero-card">
              <div className="hero-card__watermark" aria-hidden="true">
                <CarFront size={200} />
              </div>
              <h1>Waiting for connection</h1>
              <p>USE THE CONNECT BUTTON IN THE TOP-RIGHT CORNER</p>

              <div className="hero-grid">
                <div className="data-box">
                  <span>SESSION</span>
                  <strong>{health.bridge_connected ? "LIVE" : "OFFLINE"}</strong>
                </div>
                <div className="data-box">
                  <span>ECU MATRIX</span>
                  <strong>{ecuCount} Nodes</strong>
                </div>
                <div className="data-box">
                  <span>FA PAYLOAD</span>
                  <strong>{snapshot?.fa_payload ?? "--"}</strong>
                </div>
                <div className="data-box">
                  <span>DTC AUTH</span>
                  <strong>{dtcCount ? `${dtcCount} FAULTS` : "NO FAULTS"}</strong>
                </div>
              </div>
            </section>

            <section className="tile-grid">
              <article className="tile tile--warm">
                <div className="tile__icon tile__icon--warm">
                  <AlertTriangle size={20} />
                </div>
                <div className="tile__watermark">
                  <SquareStack size={132} />
                </div>
                <div className="tile__body">
                  <span className="tile__eyebrow">Diagnostic Matrix</span>
                  <h2>DTC FAULT MEMORY</h2>
                  <div className="tile__metric">
                    <strong>{dtcCount}</strong>
                    <span>{dtcCount ? "FAULTS" : "NO FAULTS"}</span>
                  </div>
                </div>
              </article>

              <article className="tile tile--cool">
                <div className="tile__icon tile__icon--cool">
                  <Cpu size={20} />
                </div>
                <div className="tile__watermark">
                  <LayoutGrid size={132} />
                </div>
                <div className="tile__body">
                  <span className="tile__eyebrow">Coding Panel</span>
                  <h2>FDL & DIAGNOSTICS</h2>
                  <div className="tile__metric">
                    <strong>{snapshot?.ecus.length ?? 0}</strong>
                    <span>{snapshot ? "SCANNED" : "NOT SCANNED"}</span>
                  </div>
                </div>
              </article>
            </section>
          </>
        )}
      </main>

      {drawerOpen ? (
        <div className="drawer-backdrop" role="presentation" onClick={() => setDrawerOpen(false)}>
          <aside className="drawer" role="dialog" aria-label="Navigation drawer" onClick={(event) => event.stopPropagation()}>
            <div className="drawer__top">
              <div className="drawer__brand">
                <div className="drawer__brand-icon">B</div>
                <span>BimmLite</span>
              </div>
              <button className="drawer__close" type="button" onClick={() => setDrawerOpen(false)} aria-label="Close menu">
                <X size={18} />
              </button>
            </div>

            <nav className="drawer__nav">
              <button
                type="button"
                className={`drawer__item ${view === "workspace" ? "drawer__item--active" : ""}`}
                onClick={() => {
                  setView("workspace");
                  setDrawerOpen(false);
                }}
              >
                <LayoutGrid size={16} />
                <span>Workspace</span>
              </button>
              <button
                type="button"
                className={`drawer__item ${view === "dashboard" ? "drawer__item--active" : ""}`}
                onClick={() => {
                  setView("dashboard");
                  setDrawerOpen(false);
                }}
              >
                <LayoutDashboard size={16} />
                <span>My Dashboard</span>
              </button>
              <button
                type="button"
                className={`drawer__item ${view === "settings" ? "drawer__item--active" : ""}`}
                onClick={() => {
                  setView("settings");
                  setDrawerOpen(false);
                }}
              >
                <Settings2 size={16} />
                <span>Settings</span>
              </button>
            </nav>

            <div className="drawer__spacer" />

            <div className="user-card">
              <div className="user-card__avatar">WO</div>
              <div className="user-card__meta">
                <strong>Workshop Owner</strong>
                <span>WORKSHOP OWNER</span>
              </div>
            </div>

            <button className="signout-button" type="button" onClick={handleDisconnect}>
              <LogOut size={16} />
              <span>Sign out</span>
            </button>
          </aside>
        </div>
      ) : null}
    </div>
  );
}
