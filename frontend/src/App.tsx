import { AnimatePresence, motion } from "framer-motion";
import {
  AlertTriangle,
  ArrowRight,
  BadgeInfo,
  Brain,
  Cable,
  CarFront,
  ChevronRight,
  Database,
  Languages,
  LogOut,
  Network,
  Play,
  Radar,
  Search,
  Sparkles,
  TimerReset,
  X,
} from "lucide-react";
import type { ReactNode } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
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
  series?: string;
  fa_payload?: string;
};

type Phase1Response = {
  trace_id: string;
  session_id: string;
  snapshot: Phase1Snapshot;
};

type Locale = "en" | "ru";
type PanelTab = "system" | "ai";
type ModalKind = "dtc" | "ecu" | "passport" | null;

const STRINGS: Record<Locale, Record<string, string>> = {
  en: {
    system: "System",
    aiLab: "AI Lab",
    connect: "Connect",
    disconnect: "Disconnect",
    openApp: "Open App",
    logs: "Logs",
    liveLogs: "Live Logs",
    waiting: "Waiting for connection",
    connected: "Connected",
    disconnected: "Disconnected",
    connecting: "Connecting",
    diagnosticMatrix: "Diagnostic Matrix",
    controlUnitExplorer: "Control Unit Explorer",
    passportStatus: "Passport Status",
    series: "Series",
    connection: "Connection",
    battery: "Battery",
    latency: "Latency",
    errors: "Errors",
    session: "Session",
    ecuMatrix: "ECU Matrix",
    faPayload: "FA payload",
    dtc: "DTC",
    vin: "VIN",
    trace: "Trace",
    filterTrace: "Filter by trace_id",
    noLogs: "No log events yet.",
    noSnapshot: "Run Connect to populate the live vehicle snapshot.",
    noEcus: "No ECUs discovered yet.",
    noDtcs: "No DTCs returned yet.",
    noParams: "No parameters returned yet.",
    aiPrompt: "Ask the AI assistant about the current fault set.",
    aiPlaceholder: "Explain the fault matrix and suggest next reads...",
    badgeConnected: "Bridge online",
    badgeDisconnected: "Bridge waiting",
    statusPending: "Awaiting connect",
    statusReady: "Ready",
    read: "Read",
    clear: "Clear",
    aiFix: "AI-fix",
    diagnostics: "Diagnostics",
    fdl: "FDL",
    seriesHint: "OEM look, server data, one trace.",
  },
  ru: {
    system: "Система",
    aiLab: "AI Лаб",
    connect: "Подключить",
    disconnect: "Отключить",
    openApp: "Открыть приложение",
    logs: "Логи",
    liveLogs: "Живые логи",
    waiting: "Ожидание подключения",
    connected: "Подключено",
    disconnected: "Нет связи",
    connecting: "Подключение",
    diagnosticMatrix: "Диагностическая матрица",
    controlUnitExplorer: "Обозреватель ЭБУ",
    passportStatus: "Статус паспорта",
    series: "Серия",
    connection: "Связь",
    battery: "АКБ",
    latency: "Латентность",
    errors: "Ошибки",
    session: "Сессия",
    ecuMatrix: "Матрица ЭБУ",
    faPayload: "FA payload",
    dtc: "DTC",
    vin: "VIN",
    trace: "Трасса",
    filterTrace: "Фильтр по trace_id",
    noLogs: "Пока нет событий.",
    noSnapshot: "Нажмите Подключить, чтобы получить snapshot автомобиля.",
    noEcus: "ЭБУ пока не найдены.",
    noDtcs: "DTC пока не получены.",
    noParams: "Параметры пока не получены.",
    aiPrompt: "Спросите AI о текущей матрице ошибок.",
    aiPlaceholder: "Объясни матрицу ошибок и предложи следующий read...",
    badgeConnected: "Мост онлайн",
    badgeDisconnected: "Мост ждёт",
    statusPending: "Ожидание connect",
    statusReady: "Готово",
    read: "Читать",
    clear: "Очистить",
    aiFix: "AI-fix",
    diagnostics: "Диагностика",
    fdl: "FDL",
    seriesHint: "ОЕМ-стиль, данные сервера, одна трасса.",
  },
};

function randomId() {
  return crypto.randomUUID().replaceAll("-", "");
}

function formatLatency(latencyMs: number | null) {
  if (latencyMs == null) return "n/a";
  return `${Math.round(latencyMs)} ms`;
}

function groupDtcs(dtcs: DtcInfo[]) {
  return dtcs.reduce<Record<string, DtcInfo[]>>((acc, dtc) => {
    const key = dtc.ecu_name || dtc.ecu_address || "Unknown ECU";
    (acc[key] ??= []).push(dtc);
    return acc;
  }, {});
}

export default function App() {
  const [health, setHealth] = useState<Health | null>(null);
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [currentTrace, setCurrentTrace] = useState<string>("");
  const [snapshot, setSnapshot] = useState<Phase1Snapshot | null>(null);
  const [locale, setLocale] = useState<Locale>(() => {
    const stored = window.localStorage.getItem("bimmlite.locale");
    return stored === "ru" ? "ru" : "en";
  });
  const [tab, setTab] = useState<PanelTab>("system");
  const [modal, setModal] = useState<ModalKind>(null);
  const [traceFilter, setTraceFilter] = useState<string>("");
  const [pingState, setPingState] = useState<string>("idle");
  const [phaseState, setPhaseState] = useState<string>("idle");
  const [latencyMs, setLatencyMs] = useState<number | null>(null);
  const [connectionState, setConnectionState] = useState<"disconnected" | "connecting" | "connected">(
    "disconnected",
  );
  const [aiDraft, setAiDraft] = useState("");
  const [aiMessages, setAiMessages] = useState<
    Array<{ role: "assistant" | "user"; text: string; ts: string }>
  >([
    {
      role: "assistant",
      text: "Drop a DTC / ECU question here. The AI panel is wired for future server inference.",
      ts: new Date().toISOString(),
    },
  ]);
  const sessionId = useMemo(() => {
    const existing = window.localStorage.getItem("bimmlite.session_id");
    if (existing) return existing;
    const created = `ui-${randomId()}`;
    window.localStorage.setItem("bimmlite.session_id", created);
    return created;
  }, []);
  const t = STRINGS[locale];
  const traceFilterRef = useRef(traceFilter);

  useEffect(() => {
    window.localStorage.setItem("bimmlite.locale", locale);
  }, [locale]);

  useEffect(() => {
    traceFilterRef.current = traceFilter;
  }, [traceFilter]);

  useEffect(() => {
    let cancelled = false;
    getHealth()
      .then((value) => {
        if (!cancelled) {
          setHealth(value);
          setConnectionState(value.bridge_connected ? "connected" : "disconnected");
        }
      })
      .catch(() => {
        if (!cancelled) {
          setHealth({ status: "offline", bridge_connected: false });
          setConnectionState("disconnected");
        }
      });

    getLogs()
      .then((items) => {
        if (!cancelled) {
          setLogs([...items].reverse());
          const newestTrace = [...items].find((item) => item.trace_id)?.trace_id ?? "";
          setTraceFilter(newestTrace);
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
    const socket = new WebSocket(`${wsProtocol}://${window.location.host}/ws/logs`);
    socket.onmessage = (event) => {
      const item = JSON.parse(event.data) as LogItem;
      setLogs((current) => [...current, item].slice(-300));
      if (!traceFilterRef.current && item.trace_id) {
        setTraceFilter(item.trace_id);
      }
    };
    return () => {
      socket.close();
    };
  }, []);

  async function handleConnectRead() {
    const traceId = randomId();
    setCurrentTrace(traceId);
    setPhaseState("connecting");
    setConnectionState("connecting");
    const startedAt = performance.now();
    try {
      const response = (await connectRead(traceId, sessionId)) as Phase1Response;
      setSnapshot(response.snapshot);
      setTraceFilter(response.trace_id || traceId);
      setPhaseState(response.snapshot.ecus.length ? "connected" : "ready");
      setConnectionState("connected");
      setHealth((current) =>
        current ? { ...current, bridge_connected: true } : { status: "ok", bridge_connected: true },
      );
      setLatencyMs(performance.now() - startedAt);
    } catch (error) {
      setPhaseState(error instanceof Error ? error.message : "connect-read failed");
      setConnectionState("disconnected");
    }
  }

  async function handlePing() {
    const traceId = randomId();
    setCurrentTrace(traceId);
    setPingState("sending");
    const startedAt = performance.now();
    try {
      const response = await sendPing(traceId, sessionId);
      setPingState(response.result ?? "ok");
      setLatencyMs(performance.now() - startedAt);
      setHealth((current) =>
        current ? { ...current, bridge_connected: Boolean(response.bridge_connected) } : current,
      );
    } catch (error) {
      setPingState(error instanceof Error ? error.message : "ping failed");
    }
  }

  function handleDisconnect() {
    setConnectionState("disconnected");
    setSnapshot(null);
    setPhaseState("idle");
    setLatencyMs(null);
  }

  function handleSubmitAi() {
    if (!aiDraft.trim()) return;
    setAiMessages((current) => [
      ...current,
      { role: "user", text: aiDraft.trim(), ts: new Date().toISOString() },
      {
        role: "assistant",
        text: "AI integration is reserved for the next phase. This panel is live UI-ready.",
        ts: new Date().toISOString(),
      },
    ]);
    setAiDraft("");
  }

  const visibleLogs = logs.filter((log) => !traceFilter || log.trace_id === traceFilter);
  const dtcGroups = snapshot ? groupDtcs(snapshot.dtcs) : {};
  const errorCount = snapshot?.dtcs.length ?? 0;
  const ecuCount = snapshot?.ecus.length ?? 0;

  return (
    <div className="app-shell">
      <div className="bg-grid" />
      <div className="bg-orb bg-orb--one" />
      <div className="bg-orb bg-orb--two" />

      <div className="app-layout">
        <aside className="glass-sidebar">
          <div className="brand-block">
            <div className="brand-mark">B</div>
            <div>
              <div className="brand-title">BimmLite</div>
              <div className="brand-subtitle">{t.seriesHint}</div>
            </div>
          </div>

          <nav className="nav-rail">
            <button className={`nav-item ${tab === "system" ? "nav-item--active" : ""}`} onClick={() => setTab("system")} type="button">
              <CarFront size={18} />
              <span>{t.system}</span>
            </button>
            <button className={`nav-item ${tab === "ai" ? "nav-item--active" : ""}`} onClick={() => setTab("ai")} type="button">
              <Brain size={18} />
              <span>{t.aiLab}</span>
            </button>
          </nav>

          <div className="sidebar-card">
            <div className="sidebar-card__label">{t.trace}</div>
            <div className="sidebar-card__value mono">{currentTrace || "—"}</div>
          </div>

          <div className="sidebar-card">
            <div className="sidebar-card__label">UI Locale</div>
            <button className="primary-button primary-button--small" onClick={() => setLocale(locale === "en" ? "ru" : "en")} type="button">
              <Languages size={16} />
              <span>{locale.toUpperCase()}</span>
            </button>
          </div>

          <div className="sidebar-card">
            <div className="sidebar-card__label">Session State</div>
            <div className="sidebar-mini">
              <div>
                <span>Ping</span>
                <strong>{pingState}</strong>
              </div>
              <div>
                <span>Read</span>
                <strong>{phaseState}</strong>
              </div>
            </div>
          </div>
        </aside>

        <main className="main-stack">
          <header className="glass-header">
            <div className="header-status">
              <div className={`status-ring status-ring--${connectionState}`} />
              <div>
                <div className="header-kicker">{connectionState === "connected" ? t.connected : connectionState === "connecting" ? t.connecting : t.disconnected}</div>
                <div className="header-title">{health?.status ?? "offline"}</div>
              </div>
            </div>

            <div className="header-metrics">
              <div className="mini-metric">
                <span>{t.battery}</span>
                <strong>{snapshot?.battery_voltage ? `${snapshot.battery_voltage.toFixed(1)} V` : "n/a"}</strong>
              </div>
              <div className="mini-metric">
                <span>{t.latency}</span>
                <strong>{formatLatency(latencyMs)}</strong>
              </div>
              <div className="mini-metric">
                <span>{t.errors}</span>
                <strong>{errorCount}</strong>
              </div>
            </div>

            <div className="header-actions">
              <button className="primary-button" onClick={handleConnectRead} type="button">
                <Play size={16} />
                <span>{t.connect}</span>
              </button>
              <button className="btn-minimal" onClick={handleDisconnect} type="button">
                <LogOut size={16} />
                <span>{t.disconnect}</span>
              </button>
              <button className="btn-minimal" onClick={handlePing} type="button">
                <Radar size={16} />
                <span>Ping</span>
              </button>
            </div>
          </header>

          <section className="hero-grid">
            <motion.article
              className="apple-card hero-card floating-3d"
              initial={{ opacity: 0, y: 14 }}
              animate={{ opacity: 1, y: 0 }}
              transition={{ duration: 0.35 }}
            >
              <div className="hero-card__watermark" aria-hidden="true">
                <svg viewBox="0 0 240 180" role="presentation">
                  <defs>
                    <linearGradient id="bmwGrid" x1="0%" y1="0%" x2="100%" y2="100%">
                      <stop offset="0%" stopColor="rgba(0,0,0,0.13)" />
                      <stop offset="100%" stopColor="rgba(0,0,0,0)" />
                    </linearGradient>
                  </defs>
                  <rect x="24" y="30" width="192" height="120" rx="22" fill="url(#bmwGrid)" />
                  <g opacity="0.2" stroke="currentColor" strokeWidth="2">
                    <path d="M36 58h168" />
                    <path d="M36 90h168" />
                    <path d="M36 122h168" />
                    <path d="M60 34v112" />
                    <path d="M100 34v112" />
                    <path d="M140 34v112" />
                    <path d="M180 34v112" />
                  </g>
                </svg>
              </div>

              <div className="hero-card__topline">
                <span className="eyebrow">{t.waiting}</span>
                <span className={`pill ${health?.bridge_connected ? "pill--success" : "pill--muted"}`}>
                  {health?.bridge_connected ? t.badgeConnected : t.badgeDisconnected}
                </span>
              </div>

              <div className="hero-card__vin">
                <span className="vin-label">{t.vin}</span>
                <h1>{snapshot?.vin ?? "—"}</h1>
              </div>

              <div className="hero-card__meta">
                <div className="data-box">
                  <span>{t.series}</span>
                  <strong>{snapshot?.series ?? "BMW F/G"}</strong>
                </div>
                <div className="data-box">
                  <span>{t.connection}</span>
                  <strong>{snapshot?.protocol ?? "ENET / DoIP"}</strong>
                </div>
                <div className="data-box">
                  <span>{t.session}</span>
                  <strong className="mono">{sessionId}</strong>
                </div>
                <div className="data-box">
                  <span>{t.ecuMatrix}</span>
                  <strong>{ecuCount}</strong>
                </div>
                <div className="data-box">
                  <span>{t.faPayload}</span>
                  <strong className="mono">{snapshot?.fa_payload ?? "pending"}</strong>
                </div>
                <div className="data-box">
                  <span>{t.dtc}</span>
                  <strong>{errorCount}</strong>
                </div>
              </div>

              <div className="hero-card__actions">
                <button className="btn-action btn-action--warm" onClick={() => setModal("dtc")} type="button">
                  <AlertTriangle size={16} />
                  <span>{t.diagnosticMatrix}</span>
                </button>
                <button className="btn-action btn-action--cool" onClick={() => setModal("ecu")} type="button">
                  <Network size={16} />
                  <span>{t.controlUnitExplorer}</span>
                </button>
                <button className="btn-action btn-action--neutral" onClick={() => setModal("passport")} type="button">
                  <BadgeInfo size={16} />
                  <span>{t.passportStatus}</span>
                </button>
              </div>
            </motion.article>

            <div className="stacked-panels">
              <motion.button
                className="pro-card pro-card--warm tile-button"
                onClick={() => setModal("dtc")}
                whileHover={{ y: -3, scale: 1.01 }}
                type="button"
              >
                <div>
                  <span className="tile-label">Diagnostic Matrix</span>
                  <strong>{errorCount}</strong>
                </div>
                <AlertTriangle size={42} />
              </motion.button>
              <motion.button
                className="pro-card pro-card--cool tile-button"
                onClick={() => setModal("ecu")}
                whileHover={{ y: -3, scale: 1.01 }}
                type="button"
              >
                <div>
                  <span className="tile-label">Control Unit Explorer</span>
                  <strong>{ecuCount}</strong>
                </div>
                <Cable size={42} />
              </motion.button>
            </div>
          </section>

          <section className="grid-panels">
            <motion.section className="apple-card section-card" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
              <div className="section-card__header">
                <div>
                  <div className="eyebrow">VehicleHero</div>
                  <h2>{snapshot?.vin ? "Passport ready" : t.noSnapshot}</h2>
                </div>
                <button className="btn-minimal" onClick={() => setModal("passport")} type="button">
                  <span>{t.passportStatus}</span>
                  <ChevronRight size={16} />
                </button>
              </div>

              {snapshot ? (
                <div className="hero-specs">
                  <div className="data-box data-box--accent">
                    <span>{t.vin}</span>
                    <strong className="mono">{snapshot.vin}</strong>
                  </div>
                  <div className="data-box">
                    <span>{t.series}</span>
                    <strong>{snapshot.series ?? "BMW F/G"}</strong>
                  </div>
                  <div className="data-box">
                    <span>{t.connection}</span>
                    <strong>{snapshot.protocol}</strong>
                  </div>
                  <div className="data-box">
                    <span>FA</span>
                    <strong className="mono">{snapshot.fa_payload ?? "pending"}</strong>
                  </div>
                </div>
              ) : (
                <div className="shimmer-state">
                  <div className="shimmer-line shimmer-line--wide" />
                  <div className="shimmer-line" />
                  <div className="shimmer-line shimmer-line--short" />
                </div>
              )}
            </motion.section>

            {tab === "system" ? (
              <motion.section className="apple-card section-card" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
                <div className="section-card__header">
                  <div>
                    <div className="eyebrow">{t.liveLogs}</div>
                    <h2>{t.trace}</h2>
                  </div>
                  <div className="section-card__tools">
                    <div className="input-shell">
                      <Search size={14} />
                      <input
                        aria-label={t.filterTrace}
                        value={traceFilter}
                        onChange={(event) => setTraceFilter(event.target.value)}
                        placeholder={t.filterTrace}
                      />
                    </div>
                  </div>
                </div>
                <LiveLogViewer logs={visibleLogs} />
              </motion.section>
            ) : (
              <motion.section className="apple-card section-card" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
                <div className="section-card__header">
                  <div>
                    <div className="eyebrow">{t.aiLab}</div>
                    <h2>{t.aiPrompt}</h2>
                  </div>
                </div>

                <div className="ai-thread">
                  {aiMessages.map((message, index) => (
                    <div className={`ai-bubble ai-bubble--${message.role}`} key={`${message.ts}-${index}`}>
                      <div className="ai-bubble__meta">
                        <span>{message.role === "assistant" ? "AI" : "You"}</span>
                        <span>{message.ts.slice(11, 16)}</span>
                      </div>
                      <p>{message.text}</p>
                    </div>
                  ))}
                </div>

                <div className="ai-compose">
                  <textarea
                    value={aiDraft}
                    onChange={(event) => setAiDraft(event.target.value)}
                    placeholder={t.aiPlaceholder}
                    rows={4}
                  />
                  <button className="primary-button" onClick={handleSubmitAi} type="button">
                    <Sparkles size={16} />
                    <span>Send</span>
                  </button>
                </div>
              </motion.section>
            )}
          </section>

          <section className="grid-panels grid-panels--dense">
            <motion.section className="apple-card section-card" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
              <div className="section-card__header">
                <div>
                  <div className="eyebrow">{t.controlUnitExplorer}</div>
                  <h2>{ecuCount ? `${ecuCount} modules` : t.noEcus}</h2>
                </div>
                <button className="btn-minimal" onClick={() => setModal("ecu")} type="button">
                  <span>{t.diagnostics}</span>
                  <ArrowRight size={16} />
                </button>
              </div>

              <div className="ecu-grid">
                {(snapshot?.ecus ?? []).slice(0, 8).map((ecu) => (
                  <div className="ecu-chip" key={ecu.address}>
                    <span className="ecu-chip__name">{ecu.name || ecu.address}</span>
                    <span className="ecu-chip__meta mono">{ecu.address}</span>
                  </div>
                ))}
                {!snapshot?.ecus.length ? <div className="empty-note">{t.noEcus}</div> : null}
              </div>
            </motion.section>

            <motion.section className="apple-card section-card" initial={{ opacity: 0 }} animate={{ opacity: 1 }}>
              <div className="section-card__header">
                <div>
                  <div className="eyebrow">{t.diagnosticMatrix}</div>
                  <h2>{errorCount ? `${errorCount} faults` : t.noDtcs}</h2>
                </div>
                <button className="btn-minimal" onClick={() => setModal("dtc")} type="button">
                  <span>{t.read}</span>
                  <ArrowRight size={16} />
                </button>
              </div>

              <div className="dtc-summary">
                {(snapshot?.dtcs ?? []).slice(0, 6).map((dtc, index) => (
                  <div className="dtc-pill" key={`${dtc.ecu_address}-${dtc.code}-${index}`}>
                    <strong className="mono">{dtc.code}</strong>
                    <span>{dtc.ecu_name || dtc.ecu_address}</span>
                  </div>
                ))}
                {!snapshot?.dtcs.length ? <div className="empty-note">{t.noDtcs}</div> : null}
              </div>
            </motion.section>
          </section>
        </main>
      </div>

      <AnimatePresence>
        {modal ? (
          <Modal onClose={() => setModal(null)}>
            {modal === "dtc" ? (
              <DiagnosticMatrixModal
                dtcGroups={dtcGroups}
                onRead={handleConnectRead}
                onClear={() => setSnapshot((current) => (current ? { ...current, dtcs: [] } : current))}
              />
            ) : null}
            {modal === "ecu" ? <EcuExplorerModal ecus={snapshot?.ecus ?? []} /> : null}
            {modal === "passport" ? <PassportStatusModal snapshot={snapshot} /> : null}
          </Modal>
        ) : null}
      </AnimatePresence>
    </div>
  );
}

function LiveLogViewer({ logs }: { logs: LogItem[] }) {
  return (
    <div className="live-log">
      {logs.length === 0 ? <div className="empty-note">No log events yet.</div> : null}
      {logs.map((item) => (
        <div className="log-row" key={`${item.ts}-${item.event}-${item.trace_id}`}>
          <span className="log-row__ts">{item.ts.slice(11, 23)}</span>
          <span className={`log-level log-level--${item.level.toLowerCase()}`}>{item.level}</span>
          <span className="log-row__module">{item.module}</span>
          <span className="log-row__event">{item.event}</span>
          <span className="log-row__trace mono">{item.trace_id}</span>
        </div>
      ))}
    </div>
  );
}

function Modal({ children, onClose }: { children: ReactNode; onClose: () => void }) {
  return (
    <motion.div
      className="modal-backdrop"
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      onClick={onClose}
      role="presentation"
    >
      <motion.div
        className="modal-shell"
        initial={{ scale: 0.96, y: 14, opacity: 0 }}
        animate={{ scale: 1, y: 0, opacity: 1 }}
        exit={{ scale: 0.98, y: 10, opacity: 0 }}
        onClick={(event) => event.stopPropagation()}
        role="dialog"
        aria-modal="true"
      >
        <button className="modal-close" onClick={onClose} type="button" aria-label="Close modal">
          <X size={18} />
        </button>
        {children}
      </motion.div>
    </motion.div>
  );
}

function DiagnosticMatrixModal({
  dtcGroups,
  onRead,
  onClear,
}: {
  dtcGroups: Record<string, DtcInfo[]>;
  onRead: () => void;
  onClear: () => void;
}) {
  const sections = Object.entries(dtcGroups);
  return (
    <div className="modal-content">
      <div className="modal-hero modal-hero--warm">
        <AlertTriangle size={28} />
        <div>
          <h3>Diagnostic Matrix</h3>
          <p>Grouped by ECU with severity-aware summaries and read-only actions.</p>
        </div>
      </div>
      <div className="modal-actions">
        <button className="btn-action btn-action--warm" onClick={onRead} type="button">
          <TimerReset size={16} />
          <span>Read</span>
        </button>
        <button className="btn-action btn-action--neutral" onClick={onClear} type="button">
          <X size={16} />
          <span>Clear</span>
        </button>
        <button className="btn-action btn-action--cool" type="button">
          <Sparkles size={16} />
          <span>AI-fix</span>
        </button>
      </div>
      <div className="modal-list">
        {sections.length ? (
          sections.map(([ecu, dtcs]) => (
            <div className="modal-list__group" key={ecu}>
              <div className="modal-list__group-title">
                <strong>{ecu}</strong>
                <span>{dtcs.length} items</span>
              </div>
              {dtcs.map((dtc, index) => (
                <div className="modal-list__row" key={`${dtc.ecu_address}-${dtc.code}-${index}`}>
                  <span className="mono">{dtc.code}</span>
                  <span>{dtc.status || "n/a"}</span>
                  <span>{dtc.description || dtc.raw || "n/a"}</span>
                </div>
              ))}
            </div>
          ))
        ) : (
          <div className="empty-note">No DTCs returned yet.</div>
        )}
      </div>
    </div>
  );
}

function EcuExplorerModal({ ecus }: { ecus: EcuInfo[] }) {
  return (
    <div className="modal-content">
      <div className="modal-hero modal-hero--cool">
        <Network size={28} />
        <div>
          <h3>Control Unit Explorer</h3>
          <p>Module names, addresses, and bus information from the live connect-read snapshot.</p>
        </div>
      </div>
      <div className="modal-actions">
        <button className="btn-action btn-action--cool" type="button">
          <Search size={16} />
          <span>Diagnostics</span>
        </button>
        <button className="btn-action btn-action--neutral" type="button">
          <Database size={16} />
          <span>FDL</span>
        </button>
      </div>
      <div className="modal-list modal-list--dense">
        {ecus.length ? (
          ecus.map((ecu) => (
            <div className="ecu-row" key={`${ecu.address}-${ecu.name}`}>
              <div>
                <strong>{ecu.name || ecu.address}</strong>
                <span>{ecu.protocol}</span>
              </div>
              <div className="mono">{ecu.address}</div>
            </div>
          ))
        ) : (
          <div className="empty-note">No ECUs discovered yet.</div>
        )}
      </div>
    </div>
  );
}

function PassportStatusModal({ snapshot }: { snapshot: Phase1Snapshot | null }) {
  return (
    <div className="modal-content">
      <div className="modal-hero modal-hero--neutral">
        <BadgeInfo size={28} />
        <div>
          <h3>Passport Status</h3>
          <p>VIN, series and FA snapshot from the live server read.</p>
        </div>
      </div>
      <div className="passport-grid">
        <div className="data-box">
          <span>VIN</span>
          <strong className="mono">{snapshot?.vin ?? "—"}</strong>
        </div>
        <div className="data-box">
          <span>Series</span>
          <strong>{snapshot?.series ?? "BMW F/G"}</strong>
        </div>
        <div className="data-box">
          <span>Protocol</span>
          <strong>{snapshot?.protocol ?? "—"}</strong>
        </div>
        <div className="data-box">
          <span>FA</span>
          <strong className="mono">{snapshot?.fa_payload ?? "pending"}</strong>
        </div>
      </div>
    </div>
  );
}
